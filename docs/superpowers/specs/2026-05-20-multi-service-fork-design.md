# e1s Multi-Service Fork — Design

**Date:** 2026-05-20
**Scope:** Internal fork of e1s (https://github.com/keidarcy/e1s) extending it from ECS-only to a multi-service AWS TUI. MVP adds Lambda, SQS, and DynamoDB as flat (non-hierarchical) services accessible via a `:`-driven command palette.

## Goals

- Add Lambda, SQS, DynamoDB as first-class kinds in the e1s TUI.
- Introduce a `:` command palette for switching between kinds (k9s-style).
- Establish a `Kind` abstraction so adding a 4th, 5th, Nth flat service is a one-file addition with no edits to existing files.
- Keep ECS's existing internal drill-down behavior (cluster → service → task → container) untouched in this MVP. ECS kinds register with the new palette so `:cluster`, `:service`, `:task` work, but their internal `nextKind()`/`prevKind()` chain is preserved.

## Non-goals

- Upstreaming to keidarcy/e1s. This is an internal fork.
- Multi-region simultaneous browsing.
- Full ECS refactor: drill-down within ECS keeps its current `nextKind()` chain.
- Pagination beyond the first API page.
- Throttling / rate-limit handling (let SDK errors surface).
- Live AWS integration tests (mocks only, matching the existing repo).

## Background

e1s today is ECS-only. The code structure makes the API layer easy to extend (`internal/api/store.go` cleanly separates clients, lazy-instantiation pattern), but the **view layer is wired tightly around ECS's tree**:

- `internal/view/kind.go:64-96` — `nextKind()`/`prevKind()` hard-code Cluster→Service→Task→Container as a switch statement. No concept of a "leaf" or flat resource.
- `internal/view/app.go:28-42` — `Entity` struct embeds typed ECS fields (`*types.Cluster`, `*types.Service`, etc.). Adding a kind = adding a field.
- `internal/view/table.go:381-450` — `changeSelectedValues()` is a 50-line switch on the ECS kind enum.
- `internal/view/footer.go` — 9 hardcoded `TextView` fields, one per ECS resource type.

The `resourceViewBuilder` interface (`internal/view/resource_view.go:18`) is a clean foundation we extend rather than replace.

## Architecture

### Package layout

```
internal/
├── api/
│   ├── store.go              [edit: add Lambda, SQS, DDB clients to Store; add to SwitchAwsConfig nil-reset list]
│   ├── lambda.go             [new] — ListFunctions, GetFunction, InvokeFunction, ListEventSourceMappings
│   ├── sqs.go                [new] — ListQueues, GetQueueAttributes, ReceiveMessage(peek), SendMessage, PurgeQueue
│   └── dynamodb.go           [new] — ListTables, DescribeTable, Scan, Query
├── view/
│   ├── kind/                 [new package]
│   │   ├── registry.go       — Kind interface, registry, Register/Get/All/ResetAll
│   │   └── palette.go        — `:` command-mode input + Submit dispatch
│   ├── lambda.go             [new] — implements Kind
│   ├── sqs.go                [new] — implements Kind
│   ├── dynamodb.go           [new] — implements Kind
│   ├── kind.go               [edit: legacy ECS enum stays; flat kinds bypass it entirely]
│   ├── app.go                [edit: route ":" to palette; wire active-kind selection updates from table row changes; ECS kinds register with the new palette]
│   ├── footer.go             [edit: ask active Kind for breadcrumb + secondary-action keymap instead of switching on ECS enum]
│   └── table.go              [edit: row-change handler calls activeKind.SetSelection() before existing ECS switch]
```

### The `Kind` interface

```go
// internal/view/kind/registry.go
package kind

// App is the minimal surface a Kind needs from the host application. It is
// declared here (not imported from the `view` package) to avoid a circular
// import: `view` imports `kind` to use the registry; `kind` must not import
// `view`. The concrete `view.App` struct will satisfy this interface.
type App interface {
    Store() *api.Store               // for kinds that need AWS clients
    SwitchView(k Kind, v View) error // palette + cross-kind navigation
    FlashError(msg string)           // user-visible error toast
    // Additional methods (e.g., region/profile accessors) added as needed.
}

type Kind interface {
    // Identity
    Name() string                  // canonical palette name: "lambda", "sqs", "ddb"

    // Lifecycle
    Build(app App) (View, error)   // construct the tview view; called when palette dispatches
    Reset()                        // drop cached selection — called on profile/region switch

    // State (kind owns its selection)
    Selection() any
    SetSelection(any)

    // Display
    Breadcrumb() string            // e.g. "lambda > auth-handler"; footer reads this

    // Actions
    PrimaryAction() Action         // Enter handler; operates on current Selection()
    SecondaryActions() []Binding   // letter-key actions; also operate on current Selection()
}

type Action func(app App) error

type Binding struct {
    Key   rune    // e.g. 'i' for invoke
    Label string  // shown in footer keymap
    Run   Action
}

type View interface {
    Render() *tview.Flex
    Focus()
    OnKey(event *tcell.EventKey) (handled bool)
}
```

`View` is a minimal interface so flat kinds aren't forced to inherit ECS's `view` struct (which carries header pages and "secondary kinds" like LogKind/AutoScalingKind that flat services don't need). ECS kinds wrap their existing views to satisfy `View`; flat kinds build leaner ones.

**Import direction:** `view` → `kind` (one-way). `kind` only imports `internal/api` (for the `Store` type in the `App` interface) and `tview`/`tcell`. ECS view code imports `kind` to register itself; flat-kind files (`view/lambda.go`, etc.) import `kind` for the same reason. Nothing imports back into `view` from `kind`.

### Registry

```go
var registry = map[string]Kind{}

func Register(k Kind) {
    if _, exists := registry[k.Name()]; exists {
        panic("kind already registered: " + k.Name())
    }
    registry[k.Name()] = k
}

func Get(name string) (Kind, bool) { k, ok := registry[name]; return k, ok }
func All() []Kind                  { /* sorted by Name */ }
func ResetAll()                    { for _, k := range registry { k.Reset() } }
```

Registration uses Go's `init()` per-file pattern:
```go
// internal/view/lambda.go
func init() { kind.Register(&lambdaKind{}) }
```

This is what makes adding service #4 a no-edit-to-existing-files operation.

### Palette

k9s-style: press `:`, type the kind name, press Enter. No autocomplete dropdown, no aliases, no fuzzy matching in MVP.

```go
// internal/view/kind/palette.go
func (p *Palette) Submit(name string) {
    k, ok := registry.Get(name)
    if !ok {
        p.app.flashError("unknown kind: " + name)
        return
    }
    v, err := k.Build(p.app)
    if err != nil {
        p.app.flashError(err.Error())
        return
    }
    p.app.SwitchView(k, v)  // updates active kind + footer + main pane
}
```

### Cross-profile / cross-region behavior

`internal/api/config.go:18-43` (`SwitchAwsConfig`) already re-loads the AWS config and nil-resets ECS clients for lazy re-instantiation. Lambda/SQS/DDB clients follow the same pattern: nilable fields on `Store`, lazy-init on first call, added to the nil-reset list in `SwitchAwsConfig`.

`SwitchAwsConfig` also calls `kind.ResetAll()` after the client reset, so per-kind selection state (e.g., "auth-handler" was selected in the `lambda` kind under the `dev` profile) doesn't leak across profile changes.

## Data flow: `:lambda` end-to-end

```
User presses ":" → palette opens
User types "lambda" + Enter
  → palette.Submit("lambda")
  → registry.Get("lambda") returns &lambdaKind{}
  → lambdaKind.Build(app) called
      → api.ListFunctions(ctx) — uses store.lambda (lazy-init on first call)
      → builds tview.Table, populates rows
      → returns View
  → app.SwitchView(lambdaKind, view)
      → main pane swaps
      → footer asks lambdaKind.Breadcrumb() → "lambda"
      → footer asks lambdaKind.SecondaryActions() → renders "i:invoke d:dlq c:config"

User selects "auth-handler" row, table fires row-change handler
  → app.activeKind.SetSelection(fnConfig)  [NEW: two-line addition in table.go before existing ECS switch]
  → footer breadcrumb refreshes → "lambda > auth-handler"

User presses Enter
  → lambdaKind.PrimaryAction()(app)
      → reads Selection().(*types.FunctionConfiguration)
      → reuses existing api.GetLogs (CloudWatch Logs already in Store)
      → opens log-tailing view (reuses existing logs.go)

User presses 'd' for DLQ jump
  → lambdaKind.SecondaryActions()['d'].Run(app)
      → reads Selection().DeadLetterConfig.TargetArn
      → if nil: app.flashError("no DLQ configured"); return
      → parses queue name from ARN
      → sqsKind, _ := registry.Get("sqs")
      → sqsKind.SetSelection(queueName) — pre-select before Build
      → app.SwitchView(sqsKind, sqsKind.Build(app))
```

## Per-service specifics

### Lambda (`internal/view/lambda.go`)
- **Columns:** Name, Runtime, Memory, Timeout, LastModified, State
- **Enter (primary):** tail logs (reuses existing `logs.go`; log group is `/aws/lambda/<name>` by convention — no extra API call)
- **Secondary actions:**
  - `i` — invoke with payload prompt (modal input → `InvokeFunction` → response in scrollable text view)
  - `d` — jump to DLQ in SQS (parse ARN from `DeadLetterConfig.TargetArn`; cross-kind nav)
  - `c` — open config view (env vars, layers, VPC config — read-only `tview.TextView`)
  - `/` — filter (reuses existing `filter.go`)

### SQS (`internal/view/sqs.go`)
- **Columns:** Name, ApproxMessages, ApproxInFlight, ApproxDelayed, DLQ
- **Enter (primary):** peek — `ReceiveMessage` with `VisibilityTimeout=0` so peek does not hide messages from real consumers. Show 10 messages; JSON pretty-print via existing `json.go`.
- **Secondary actions:**
  - `p` — purge (modal confirm requiring user to type queue name; `PurgeQueue`)
  - `s` — send test message (modal input → `SendMessage`)
  - `/` — filter
- **Pre-selection:** if `SetSelection(name)` called before `Build`, Build positions cursor on that row (used by Lambda → DLQ jump).

### DynamoDB (`internal/view/dynamodb.go`)
- **Columns:** TableName, Status, ItemCount, SizeBytes, BillingMode, Streams
- **Enter (primary):** scan first 25 items (`Scan` with `Limit=25`; render as JSON tree via existing `json.go`)
- **Secondary actions:**
  - `q` — query (modal for partition key value; `DescribeTable` first to get key schema, then `Query`)
  - `c` — describe table (full schema, GSIs, streams config — read-only)
  - `/` — filter

## Reused infrastructure (no new code)

- `internal/view/logs.go` — Lambda log tailing
- `internal/view/json.go` — SQS message bodies, DDB items
- `internal/view/filter.go` — `/` filtering on any tview.Table-based view
- `internal/view/modal.go` — invoke payloads, purge confirms, query inputs
- `internal/view/table.go` — sorting, selection, keymap (its existing ECS-enum switch simply doesn't fire for flat kinds; flat-kind selection updates flow through `Kind.SetSelection()` instead)

## State model

Each kind owns its own selection (Section 2 / option C in the brainstorm).

`app.go`'s `App` struct gains one new field: `activeKind kind.Kind` (nil when the user is in legacy ECS-only flow before any palette use). `SwitchView` sets it; `table.go`'s row-change handler reads it to decide whether to call `activeKind.SetSelection(row)` or fall through to the existing ECS-enum switch.

ECS legacy kinds keep their typed fields on `Entity` (`*types.Cluster`, `*types.Service`, etc.) for this MVP. They are not migrated to the per-kind state model — that's a follow-up once the abstraction is proven on flat kinds.

## Cross-kind navigation

The mechanism is free in this design — `SecondaryActions` can return an action that calls `app.SwitchView(otherKind, otherKind.Build(app))`. Each cross-reference is per-relationship code (~10 lines).

**MVP includes 1 cross-kind jump:**
- Lambda `d` → SQS DLQ (parse ARN from `DeadLetterConfig.TargetArn`, pre-select queue, switch view).

Plus 1 implicit jump that doesn't require cross-kind machinery:
- Lambda Enter → CloudWatch Logs (uses existing logs.go view, not a separate registered kind in MVP).

**Explicitly deferred:**
- Reverse lookups (SQS → consumer Lambdas via paginated `ListEventSourceMappings`).
- DDB → stream consumers.
- Lambda → event source mappings list view.

These are post-MVP follow-ups once real usage tells us which reverse jumps matter.

## Error handling

1. **AWS API errors → flash bar, no crash.** Every AWS SDK call returns `(result, error)`. The kind's action calls `app.flashError(err.Error())`. No retry, no backoff. User sees the AWS error verbatim.
2. **Missing/expired credentials → identical to ECS today.** Lambda/SQS/DDB clients lazy-init using `*store.Config`; an expired SSO session fails the same way ECS sees on `ListClusters`. No new handling.
3. **Empty / nil-field cases → flash, don't navigate.** Cross-kind jumps check the prerequisite field; if nil, flash and return without switching views.

Logging: every action logs at `slog.Debug` on entry and `slog.Error` on failure, matching `api/cluster.go`.

## Testing

Following the existing e1s pattern (`*_test.go` colocated; mocked SDK clients via `aws-sdk-go-v2`'s middleware mock pattern; `tests/` for cross-cutting integration).

**Unit tests per new kind file (`internal/view/lambda_test.go`, `sqs_test.go`, `dynamodb_test.go`):**
- `Name()` returns expected string.
- `SetSelection / Selection` round-trip.
- `Reset()` clears selection.
- `Breadcrumb()` formats correctly with and without selection.
- `SecondaryActions()` returns expected key bindings (count, keys, labels).

**Unit tests per new API file (`internal/api/lambda_test.go`, `sqs_test.go`, `dynamodb_test.go`):**
- Mock SDK happy path: expected slice/struct returned.
- Mock SDK error path: `AccessDeniedException` → wrapped error returned.

**Registry tests (`internal/view/kind/registry_test.go`):**
- `Register` panics on duplicate name.
- `Get` returns `(nil, false)` for unknown name.
- `ResetAll` calls `Reset` on every registered kind.
- `All` returns kinds in stable sort order.

**Palette tests (`internal/view/kind/palette_test.go`):**
- Submit with known name → calls `Build` and `SwitchView`.
- Submit with unknown name → calls `flashError`, no view change.
- Empty input → no-op.

**Cross-kind integration test (`tests/cross_kind_navigation_test.go`):**
- Register Lambda + SQS kinds with mocked clients.
- Build Lambda view, populate with one function whose `DeadLetterConfig.TargetArn` points to a known queue.
- Trigger the `d` secondary action.
- Assert: SQS kind is now active; its selection equals the parsed queue name; main view swapped.

This is the architecturally critical test — if it passes, adding service #4 follows the same pattern.

**Explicitly skipped:**
- End-to-end terminal-rendering tests (e1s doesn't have them today).
- Live AWS tests in CI (mocks only).
- Performance / load tests.

Coverage target: match e1s's existing coverage on the new files. No specific gate.

## Migration / rollout

This is an internal fork. No staged rollout, no flags. The MVP either ships and gets used, or doesn't.

Ordering for the implementation plan:
1. `Kind` interface + registry + palette (additive, no behavior change).
2. Add Lambda — first flat kind, validates the abstraction.
3. Add SQS — second flat kind, stresses the abstraction with a different action shape (peek, purge confirm).
4. Add DynamoDB — third flat kind.
5. Wire ECS kinds to the registry so `:cluster`/`:service`/`:task` work as palette entries (drill-down behavior unchanged).
6. Cross-kind jump: Lambda → SQS DLQ.

Each step is independently mergeable.

## Open questions

None at design time. Implementation-time questions (e.g., exact tcell key for opening palette — likely `:` matching existing filter `/` precedent) are resolved during the writing-plans step.
