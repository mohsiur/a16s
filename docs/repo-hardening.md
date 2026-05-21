# Repo hardening checklist

A practical lockdown for `mohsiur/a16s`. Each step lists the goal, the `gh` CLI command (or the web-UI path), and why it matters. Run them top-to-bottom — earlier steps protect the work you'll do in later steps.

> **Prereq:** `gh auth status` returns "Logged in to github.com" with `repo` scope. If it doesn't, run `gh auth login` and pick `repo`, `read:org`, `workflow`.

## 1. Lock the default branch

Goal: prevent accidental force-push, deletion, or unreviewed commits to `main`.

```bash
# Branch protection on main via REST API
gh api -X PUT repos/mohsiur/a16s/branches/main/protection \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["Test"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "block_creations": false,
  "required_conversation_resolution": true
}
JSON
```

Why each setting:

- `required_status_checks` — every PR must pass the `Test` workflow before merging. The context name must match the job's `name:` field in `.github/workflows/test.yml` (currently `Test`).
- `strict: true` — branch must be up-to-date with main before merging. Avoids semantic merge conflicts where two PRs are fine alone but break together.
- `enforce_admins: false` — you can still bypass in an emergency. Set to `true` once you have a co-maintainer.
- `required_approving_review_count: 0` — solo project, so reviews aren't required. Bump to `1` when you add collaborators.
- `dismiss_stale_reviews: true` — re-approval needed if anything changes after approval.
- `required_linear_history: true` — no merge commits on main. Keeps `git log` readable. Use squash or rebase merges.
- `allow_force_pushes: false`, `allow_deletions: false` — main is not rewritable.
- `required_conversation_resolution: true` — open PR comments must be resolved before merge.

Verify:

```bash
gh api repos/mohsiur/a16s/branches/main/protection | jq '.required_pull_request_reviews, .allow_force_pushes, .required_linear_history'
```

## 2. Default merge strategy

Goal: clean history, no merge commits.

```bash
gh api -X PATCH repos/mohsiur/a16s \
  -F allow_merge_commit=false \
  -F allow_squash_merge=true \
  -F allow_rebase_merge=true \
  -F delete_branch_on_merge=true \
  -F squash_merge_commit_title=PR_TITLE \
  -F squash_merge_commit_message=PR_BODY
```

`delete_branch_on_merge: true` keeps the branch list tidy. The squash settings make the commit message use the PR title/body verbatim instead of the auto-generated `* commit a * commit b` blob.

## 3. Secret scanning + push protection

Goal: GitHub blocks commits that contain credentials before they reach the remote.

Both are free for public repos and require Advanced Security ($$) for private repos. If your repo is public:

```bash
gh api -X PATCH repos/mohsiur/a16s \
  -F security_and_analysis.secret_scanning.status=enabled \
  -F security_and_analysis.secret_scanning_push_protection.status=enabled
```

If push-protection ever blocks a legitimate commit (rare — it only fires on known credential patterns), you can mark it as a false positive in the PR UI.

## 4. Dependabot

Already configured (`.github/dependabot.yml`). Add auto-merge for patch updates so security fixes don't sit in your inbox:

Create `.github/workflows/dependabot-auto-merge.yml`:

```yaml
name: Dependabot auto-merge
on: pull_request

permissions:
  contents: write
  pull-requests: write

jobs:
  auto-merge:
    runs-on: ubuntu-latest
    if: github.actor == 'dependabot[bot]'
    steps:
      - name: Fetch metadata
        id: meta
        uses: dependabot/fetch-metadata@v2
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
      - name: Auto-approve patch updates
        if: steps.meta.outputs.update-type == 'version-update:semver-patch'
        run: gh pr review --approve "$PR_URL"
        env:
          PR_URL: ${{ github.event.pull_request.html_url }}
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - name: Enable auto-merge for patch updates
        if: steps.meta.outputs.update-type == 'version-update:semver-patch'
        run: gh pr merge --auto --squash "$PR_URL"
        env:
          PR_URL: ${{ github.event.pull_request.html_url }}
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Minor and major updates still need your review — this only auto-merges patches.

## 5. Restrict who can push tags

Tags drive releases (your `release.yml` runs on `v*` tag pushes). Once you have collaborators, you don't want anyone tagging arbitrarily.

```bash
gh api -X POST repos/mohsiur/a16s/tags/protection \
  -F pattern='v*'
```

Until then, your single-user account is the only one who can tag — but it's worth setting up the protection rule so it's already enforced when you add a contributor.

## 6. Workflow permissions — least privilege

Most jobs read repo content and don't need write access. Default permissions for `GITHUB_TOKEN` should be read-only; jobs that need more (release, dependabot-merge) opt in.

```bash
gh api -X PUT repos/mohsiur/a16s/actions/permissions/workflow \
  -F default_workflow_permissions=read \
  -F can_approve_pull_request_reviews=false
```

Check each existing workflow has an explicit `permissions:` block declaring only what it needs:

- `.github/workflows/test.yml` → `permissions: contents: read`
- `.github/workflows/release.yml` → `permissions: contents: write` (already has this)
- `.github/workflows/publish-image.yml` → `permissions: contents: read, packages: write`

## 7. Restrict the actions registry

Only allow Actions from verified creators + the repos in your organisation. Blocks supply-chain attacks via random third-party actions.

```bash
gh api -X PUT repos/mohsiur/a16s/actions/permissions \
  -F enabled=true \
  -F allowed_actions=selected
gh api -X PUT repos/mohsiur/a16s/actions/permissions/selected-actions \
  -F github_owned_allowed=true \
  -F verified_allowed=true \
  -F patterns_allowed='["goreleaser/*","docker/*","actions/*","dependabot/*"]'
```

Adjust the `patterns_allowed` list to match the actions your workflows actually reference.

## 8. Two-factor + signed commits

Account-level (do once, applies to all your repos):

- Settings → Password and authentication → enable 2FA (TOTP or hardware key, not SMS).
- Settings → SSH and GPG keys → register a signing key.

Local config (per-machine):

```bash
git config --global user.signingkey <KEY-ID>
git config --global commit.gpgsign true
git config --global tag.gpgsign true
```

Then enforce signed commits on main:

```bash
gh api -X POST repos/mohsiur/a16s/branches/main/protection/required_signatures
```

Solo-project caveat: if you ever push from a machine without your signing key, this blocks you. Defer until you've got the keychain set up everywhere.

## 9. Secrets rotation

Audit the secrets configured in repo settings:

```bash
gh secret list
```

Currently the release workflow uses `PUBLISHER_TOKEN` (likely a personal access token with `repo` + `write:packages`). Replace it with a fine-scoped GitHub App or a fine-grained PAT (Settings → Developer settings → Personal access tokens → Fine-grained):

- Repo access: only `mohsiur/a16s` and (if you create one) `mohsiur/homebrew-tap`.
- Permissions: `contents: write`, `metadata: read`. Nothing else.
- Expiration: 90 days (rotate quarterly).

Update the secret:

```bash
gh secret set PUBLISHER_TOKEN
# paste the new token when prompted
```

## 10. Enable vulnerability alerts

```bash
gh api -X PUT repos/mohsiur/a16s/vulnerability-alerts
gh api -X PUT repos/mohsiur/a16s/automated-security-fixes
```

GitHub will open a PR whenever a known CVE matches your `go.mod`.

## 11. Optional: CODEOWNERS

Sets a default reviewer for PRs. Useful even solo — it's the answer to "wait, who owns this folder again?" months later.

Create `.github/CODEOWNERS`:

```
# Default owner for everything
* @mohsiur

# Workflows + release config — extra-careful changes
/.github/ @mohsiur
/.goreleaser.yml @mohsiur
```

Then flip `require_code_owner_reviews: true` in the branch protection from step 1.

## 12. Verify with a dry-run

```bash
# create a test branch
git checkout -b test/protection-check
echo '' >> README.md
git commit -am 'test: protection dry-run'
git push -u origin test/protection-check

# attempt the things you want blocked
git push --force-with-lease origin test/protection-check  # should succeed (not main)
gh pr create --base main --title 'test' --body 'test'    # PR opens
# In the PR UI: confirm that "Test" status check is required and the merge button is disabled until green.

# clean up
gh pr close --delete-branch
git branch -D test/protection-check
```

## Reference: undo

Each step is reversible. Branch protection:

```bash
gh api -X DELETE repos/mohsiur/a16s/branches/main/protection
```

Workflow permissions back to write:

```bash
gh api -X PUT repos/mohsiur/a16s/actions/permissions/workflow \
  -F default_workflow_permissions=write
```

Tag protection:

```bash
gh api repos/mohsiur/a16s/tags/protection | jq '.[].id'
gh api -X DELETE repos/mohsiur/a16s/tags/protection/<id>
```
