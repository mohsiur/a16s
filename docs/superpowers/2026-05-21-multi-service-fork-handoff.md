# Multi-Service Fork — Session Handoff (2026-05-21)

## What This Doc Is

Snapshot of the `mohsiur/a16s` fork after Phases 1–7 of the multi-service fork
work. New session should be able to pick up here without re-deriving anything.

Authoritative references that already exist in the repo:
- Spec: `docs/superpowers/specs/2026-05-20-multi-service-fork-design.md`
- Plan: `docs/superpowers/plans/2026-05-20-multi-service-fork.md`

This file is the **delta from those + every architectural decision made during
implementation that isn't captured in the spec/plan/code comments.**

## North-Star Goal

Fork `keidarcy/e1s` into `mohsiur/a16s` and make it browse Lambda, SQS, and
DynamoDB alongside ECS — accessible via a k9s-style `:` palette — using the
**exact same chrome** as the existing ECS pages.

## Critical User Feedback (Preserved Verbatim Across Sessions)

These constraints were re-stated multiple times. Treat as non-negotiable:

1. **Identical chrome.** Lambda/SQS/DDB pages must render with the same
   structure as cluster/service/task: info pane on left with `Name:` / counts,
   keyboard legend on right with `<key> description` format, `info(<name>)`
   titled border, `<kind>all(N)` titled table, kind-tabs at the bottom.

2. **No parallel rendering system.** The `:` palette and `/` filter must use
   the same code paths as ECS pages — `buildResourcePage`,
   `resourceViewBuilder`, `view` struct, `headerPages`, `tablePages`. Earlier
   prototypes built a parallel `simpleKindView` system; that has been retired.

3. **No top bar.** The `:` input mounts inline above `Pages` only when
   active, then disappears. No always-visible command row.

4. **Kind tabs at the bottom must include the new kinds.** Cluster, Service,
   Task, Lambda, SQS, DynamoDB are all reachable via the footer hint row.

## Final Architecture (After Phase 7)

### Two Separate Roles for `kindpkg`

`kindpkg` is **not** the rendering system. After Phase 7 it has one job: hold
inventory caches that survive across page navigations and can be reset on
profile/region switch.

```
┌──────────────────────────────────────────────────────────┐
│  kindpkg (registry only — ~140 LOC)                       │
│  • Kind interface: Name/Reset/Selection/SetSelection     │
│  • Aliaser: optional Aliases() for "dynamodb" → ddbKind  │
│  • Preloader: optional Preload(app) called at startup    │
│  • App interface: APIStore / FlashError / QueueUpdateDraw│
│    / SetFocus  (4 methods, no SwitchView, no Back)       │
│  • PreloadAll, ResetAll, Get, Names                      │
└──────────────────────────────────────────────────────────┘
                            ▲
                            │ each *Kind struct caches
                            │ inventory + selection
┌──────────────────────────────────────────────────────────┐
│  view package (rendering — uses ECS chrome)               │
│  • lambdaKind / sqsKind / ddbKind: cache + selection only │
│  • showLambdasPage / showQueuesPage / showTablesPage:     │
│    read cache, build *lambdaView / *sqsView / *ddbView,   │
│    call buildResourcePage                                 │
│  • flat_actions.go: action-key handlers (`l`, `i`, `d`,   │
│    `p`, `s`, `c`, `enter`) wired into table.go's          │
│    handleInputCapture / handleSelected                    │
│  • palette_view.go: `:` input → paletteKinds map →        │
│    showPrimaryKindPage                                    │
└──────────────────────────────────────────────────────────┘
```

### The `kind` Enum (in `internal/view/kind.go`)

This is the **legacy** enum used by the ECS chrome; it's separate from
`kindpkg.Kind`. Both names exist; both are necessary.

```go
const (
    ClusterKind kind = iota
    ServiceKind
    TaskKind
    ContainerKind
    InstanceKind
    TaskDefinitionKind
    DescriptionKind
    ServiceDeploymentKind
    ProfileKind
    RegionKind
    EmptyKind
    // Flat-kind additions (Phase 2-4):
    LambdaKind
    SQSKind
    SQSPeekKind
    DynamoDBKind
    DynamoDBIndexKind
    DynamoDBScanKind
)
```

`showPrimaryKindPage(k kind, reload bool)` dispatches each enum value to its
`show*Page` function. This is the single entry point used by:

- `paletteKinds[":foo"] → showPrimaryKindPage(...)`
- `back()` for sub-kinds
- Auto-refresh ticker
- Cross-kind nav from Lambda DLQ → SQS

### `kindpkg.Kind` vs `kind` Enum — Why Both?

- **`kind` enum**: drives `app.kind`, page routing, sort columns, footer chrome.
  Tightly coupled to the legacy ECS code paths.
- **`kindpkg.Kind` interface**: lets `kindpkg.PreloadAll` and `kindpkg.ResetAll`
  iterate over registered kinds without knowing about `view` types. The
  registry is the **single source of truth for cross-cutting startup/teardown**.

The two systems intersect at `getLambdaKind()` / `getSQSKind()` /
`getDDBKind()` helpers in each kind's `view/*.go` file: they pull the cache out
of `kindpkg` for the legacy chrome to read.

## Phase-by-Phase Decision Log

### Phase 1: Top bar removal + `:` mount inline (DONE)
- **Decision:** No always-visible command bar. The `:` input mounts at the top
  of `mainScreen` only while the user is typing, then dismisses.
- **Why:** k9s-style ergonomics; matches user feedback that the previous top
  bar was visual noise.
- **Files:** `palette_view.go` (showPalette / dismissPalette /
  rebuildMainScreen), `app.go` (mainScreen + mainScreenFooter wiring).

### Phase 2-4: Lambda, SQS, DynamoDB use legacy chrome (DONE)
- **Decision:** Each new kind is a `*<kind>View` that implements
  `resourceViewBuilder` and goes through `buildResourcePage[T]`. Identical
  chrome to ECS — info-flex on left, key legend on right, table below.
- **Why:** Critical user feedback — no parallel rendering systems.
- **Sub-kinds:**
  - SQS: `SQSPeekKind` for messages.
  - DynamoDB: `DynamoDBIndexKind` for GSI/LSI list, `DynamoDBScanKind` for
    items in an index.
- **Inventory caching:** Each `*Kind` struct uses RWMutex + a `loadDone`
  channel for single-flight `loadInventory(app)`. Concurrent `:lambda` and
  `Preload` only fetch once.

### Phase 5: Sub-screens use ECS chrome too (DONE)
- **Decision:** Drilling into a queue (`enter` from SQS list) lands on
  `showQueueMessagesPage` which builds an `sqsPeekView`. Same for DynamoDB
  index list and scan results.
- **Why:** Consistency. Users get the same keybindings (`/` filter, `?` help,
  Esc back) at every level.

### Phase 6: Action keys wired into legacy chrome (DONE)
- **Decision:** Action keys (`l` for logs, `i` for invoke, `d` for DLQ, `p`
  for purge, `s` for send, `c` for describe/config, `q` for query) are
  installed in `table.go`'s `handleInputCapture` and call methods on
  `*view`. Each method lives in `flat_actions.go`.
- **Why:** Same keybinding plumbing as ECS — no kind-specific input router.
- **Aux pages:** Transient screens (logs, invoke result, peek body, DDB query
  results) use `showAuxText` / `showAuxPrimitive` in flat_actions.go.
  Single page name `a16s.aux` mounted above `Pages`; Esc dismisses.

### Phase 7: Retire `simpleKindView` + thin out kindpkg (DONE — this session)
The full cleanup sweep. Earlier phases left a parallel rendering system
behind; Phase 7 deleted it.

#### 7a — Strip kindpkg.Kind to minimum + remove orphan action wiring

**Removed from `kindpkg`:**
- `View`, `Action`, `Binding`, `Informer` interfaces
- `Palette` type and `NewPalette` constructor
- `palette.go`, `palette_test.go`

**Slimmed `kindpkg.Kind` to:**
```go
type Kind interface {
    Name() string
    Reset()
    Selection() any
    SetSelection(any)
}
```

**Slimmed `kindpkg.App` to (no `SwitchView`/`Back`):**
```go
type App interface {
    APIStore() *api.Store
    FlashError(msg string)
    QueueUpdateDraw(f func()) *tview.Application
    SetFocus(p tview.Primitive) *tview.Application
}
```

**Removed from each `*Kind` (now dead):**
- `lambdaKind`: PrimaryAction, SecondaryActions, invokeAction, dlqAction,
  configAction, openLogGroupTail, buildLegacyTable, Build, Breadcrumb,
  AggregateInfo, SelectionDetail
- `sqsKind`: PrimaryAction, SecondaryActions, purgeAction, sendAction,
  buildPeekTableView, showPeekBody, buildTable, Build, Breadcrumb,
  AggregateInfo, SelectionDetail, atoiOrZero (also dead)
- `ddbKind`: PrimaryAction, SecondaryActions, describeAction, openIndexList,
  openScanResults, openQueryPrompt, buildScanResultsView, buildTable, Build,
  Breadcrumb, AggregateInfo, SelectionDetail

**Added to `sqsKind`:** `promoteSelectedURL()` — extracted from old
`buildTable`. Upgrades a bare-name selection (set by Lambda DLQ cross-kind
nav) to the full URL by matching against cached inventory.

#### 7b — Delete `kind_view.go` and tests
Files deleted:
- `internal/view/kind_view.go` (simpleKindView, pseudoKind, newTextSubView,
  newTableSubView, newTableKindView, newLoadingTableKindView,
  populateTableKindView, OnKey)
- `internal/view/kind_view_filter_test.go`
- `internal/view/kind_view_sort_test.go`
- `internal/view/switch_view_test.go`

#### 7c — Strip activeKind / SwitchView / Back from app.go
- `*App` no longer holds `activeKind kindpkg.Kind` or `palette *kindpkg.Palette`.
- `App.SwitchView` and `App.Back` deleted (only callers were simpleKindView).
- `app.Pages.SetChangedFunc` no longer resets activeKind on page change.
- `auto-refresh` no longer guards on `activeKind == nil`.
- `view.handleSelectionChanged` no longer short-circuits to
  `activeKind.SetSelection(...)`.
- `tableSelectionForActiveKind` deleted.

#### 7d — Rework cross-kind test
Old `cross_kind_test.go` asserted on `dlqAction` + `SwitchView`; both gone.
New version asserts on the actual contract: `sqsKind.SetSelection(bareName)`
followed by `promoteSelectedURL()` returns the full URL. The `view.openLambdaDLQ`
integration would require real tview state and isn't worth the scaffolding —
covered indirectly by the SQS legacy chrome tests.

`lambda_dlq_test.go` was reduced to just `TestParseQueueNameFromArn` (the
`dlqAction`-specific tests were deleted along with `dlqAction`).

#### 7e — Cleanup
- `test_helpers_test.go` — `fakeApp` shrunk to match new App interface
  (4 methods: APIStore / FlashError / QueueUpdateDraw / SetFocus).
- `palette_view.go` — renamed `paletteLegacyKinds` → `paletteKinds` (no
  legacy fallback path anymore). Unknown verbs flash a warning instead of
  routing through deleted `kindpkg.Palette`.
- Deleted `ecs_kinds.go` + test + `noop_view.go` — these were adapter shims
  routing palette → showPrimaryKindPage; now `paletteKinds` does it directly.

### Verification (Phase 7 exit)
- `go build ./...` clean
- `go test ./...` all green (api, utils, view, view/kind)
- `go vet ./...` clean
- Manual TUI smoke test by user — no UI regressions (UI was already done in
  Phases 1-6; Phase 7 was internal cleanup only)

## Current File Layout

```
internal/view/
├── app.go                       492 LOC, no activeKind/SwitchView/Back
├── kind.go                      kind enum + name/prevKind/getAppPageName
├── palette_view.go              119 LOC, paletteKinds map only
├── flat_actions.go              241 LOC, all flat-kind action handlers
├── lambda.go                    260 LOC, lambdaKind + lambdaView
├── sqs.go                       476 LOC, sqsKind + sqsView + sqsPeekView
├── dynamodb.go                  614 LOC, ddbKind + ddbView + ddbIndexView + ddbScanView
├── kind/
│   ├── registry.go              144 LOC — registry only
│   └── registry_test.go         alias / preloader / ResetAll tests
└── (cluster/service/task/etc — unchanged from upstream e1s)
```

## Cross-Kind Nav Contract (Lambda DLQ → SQS)

This was the trickiest piece. Final wiring:

1. User presses `d` on a Lambda row in the Lambda list.
2. `table.handleInputCapture` calls `v.openLambdaDLQ()` (in `flat_actions.go`).
3. `openLambdaDLQ`:
   - Reads selected `*lambdaTypes.FunctionConfiguration` from row reference.
   - Parses the DLQ ARN → bare queue name (`my-dlq`).
   - Calls `getSQSKind().SetSelection("my-dlq")`.
   - Calls `app.showPrimaryKindPage(SQSKind, false)`.
4. `showQueuesPage`:
   - Calls `loadInventory(app)` (cached, instant if Preload ran).
   - Calls `sk.promoteSelectedURL()` to upgrade `"my-dlq"` → full URL.
   - Sets `app.sqsQueueName = full` and `app.rowIndex` to the matching row.
   - `buildResourcePage` lands on the right row with the right header.

The bare-name → full-URL promotion is the **only** reason `sqsKind` keeps a
`SetSelection(any)` method that accepts a string. Future cross-kind callers
should follow the same pattern: set bare identifier, let the destination
kind's `show*Page` resolve.

## Known Constraints / Gotchas for Next Session

1. **`*Kind` structs MUST stay registered** — `Preload` only fires for
   registered kinds. If you add a new kind, do `init() { kindpkg.Register(...) }`.

2. **`kindpkg.ResetAll` is wired to `api.OnConfigSwitch`** — when the user
   changes profile/region via Ctrl+P / Ctrl+R, every `*Kind`'s `Reset()` runs.
   Per-kind state (`selected`, `urls`, `descs`, `loaded`, `loadDone`) MUST
   be cleared in `Reset()` or stale data will leak across profiles.

3. **No new files in `kindpkg`** — adding rendering surface back to
   `kindpkg` would re-introduce the parallel system Phase 7 deleted. New
   chrome goes in `view/`.

4. **`paletteKinds` is the routing table** — adding `:foo` autocomplete +
   dispatch means adding to this map AND adding a `kind` enum value AND a
   `show*Page`.

5. **`view.openLambdaDLQ` style is the cross-kind pattern** — set a bare
   selection on the destination kind, then `showPrimaryKindPage`. Don't try
   to call into another kind's view directly.

6. **Aux pages are single-slot** — `auxPageName = "a16s.aux"` is a constant.
   If two aux pages need to coexist, that needs work.

## Possible Next-Phase Topics

The plan document has more, but unfinished threads I noticed:

- **Phase 8 (if planned):** SQS send-message modal — current
  `sendTestMessageToQueue` hard-codes `{"a16s":"test"}`.
- **DynamoDB query results table** — `queryDDBIndex` currently dumps results
  as JSON in an aux text page. Spec called for tabular display.
- **Refresh interaction** — auto-refresh now no longer guards on activeKind
  (which is gone), but flat-kind pages call `loadInventory` which is single-
  flight + cached. Needs a `Reset()` + reload path on Ctrl+R-style refresh
  for flat kinds, otherwise the table never updates. (Unverified — worth
  testing.)
- **Help page (`?`)** — should now list all flat-kind keybindings; verify it
  was updated in Phase 6.

## Operational / Workflow Constraints (Preserve Across Sessions)

These are from the user's global CLAUDE.md and per-project memory. The
new session will inherit them via memory + CLAUDE.md, but listing here
defensively:

- Branch → commit → push → PR for any code change
- Conventional commits (`feat:`, `fix:`, `chore:`, etc.)
- Do NOT add `Co-Authored-By` lines to commits
- Always create new commits, never amend
- Never `cd /path && git ...` — use `git -C /absolute/path ...` instead
- Compound bash commands (`&&`, `;`) are fine without per-command approval
- Never run destructive git commands (push --force, reset --hard) without
  explicit user request
- `nvm use` before yarn/node commands; prefer yarn over npm

## Quick Restart Recipe for Next Session

1. Read `docs/superpowers/specs/2026-05-20-multi-service-fork-design.md` —
   the original design.
2. Read `docs/superpowers/plans/2026-05-20-multi-service-fork.md` — the
   phased plan.
3. Read this handoff to understand what's actually done vs what the plan says.
4. Run `go build ./... && go test ./...` to confirm green starting state.
5. Ask user which thread to pick up — Phase 8, refresh fix, query results
   table, or something else.
