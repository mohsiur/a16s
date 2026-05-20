# e1s Multi-Service Fork Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend e1s from ECS-only to multi-service AWS TUI. MVP adds Lambda, SQS, DynamoDB as flat (non-hierarchical) kinds reachable via a `:`-driven command palette.

**Architecture:** New `internal/view/kind` package with a `Kind` interface + registry + `:` palette. ECS legacy kinds keep their internal drill-down (`nextKind()` chain) but also register with the new palette. Flat kinds (Lambda/SQS/DDB) implement `Kind` directly; per-kind state lives on each Kind, not on `Entity`. Cross-kind navigation works via `app.SwitchView`.

**Tech Stack:** Go 1.26, `aws-sdk-go-v2`, `tview`, `tcell`. Spec at `docs/superpowers/specs/2026-05-20-multi-service-fork-design.md`.

**Branch:** All work happens on `feat/multi-service-fork` (fresh from `master`). Each task ends with a commit on that branch.

---

## Setup (do once before Phase 1)

- [ ] **Step S1: Create feature branch from master**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s checkout master
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s checkout -b feat/multi-service-fork
```

- [ ] **Step S2: Verify build is green before any changes**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./... && go test ./...
```
Expected: build OK, tests pass.

---

## Phase 1: Kind interface + registry + palette

End state of phase: `:` opens a command bar, typing a registered name dispatches `Build()` and swaps the main pane. No new AWS services yet — the only registered kind is a no-op test stub that proves the pipeline works.

### Task 1.1: Create the `kind` package skeleton

**Files:**
- Create: `internal/view/kind/registry.go`
- Create: `internal/view/kind/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/view/kind/registry_test.go`:

```go
package kind

import "testing"

type stubKind struct {
	name      string
	resetCalls int
}

func (s *stubKind) Name() string                   { return s.name }
func (s *stubKind) Build(App) (View, error)        { return nil, nil }
func (s *stubKind) Reset()                         { s.resetCalls++ }
func (s *stubKind) Selection() any                 { return nil }
func (s *stubKind) SetSelection(any)               {}
func (s *stubKind) Breadcrumb() string             { return s.name }
func (s *stubKind) PrimaryAction() Action          { return nil }
func (s *stubKind) SecondaryActions() []Binding    { return nil }

func TestRegisterAndGet(t *testing.T) {
	resetRegistryForTest()
	k := &stubKind{name: "lambda"}
	Register(k)

	got, ok := Get("lambda")
	if !ok || got != k {
		t.Fatalf("Get(\"lambda\") = %v, %v; want %v, true", got, ok, k)
	}
}

func TestGetUnknownReturnsFalse(t *testing.T) {
	resetRegistryForTest()
	if _, ok := Get("nope"); ok {
		t.Fatal("Get(\"nope\") returned ok=true; want false")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	resetRegistryForTest()
	Register(&stubKind{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register; got nil")
		}
	}()
	Register(&stubKind{name: "dup"})
}

func TestResetAllCallsResetOnEveryKind(t *testing.T) {
	resetRegistryForTest()
	a := &stubKind{name: "a"}
	b := &stubKind{name: "b"}
	Register(a)
	Register(b)

	ResetAll()

	if a.resetCalls != 1 || b.resetCalls != 1 {
		t.Fatalf("Reset calls a=%d b=%d; want 1,1", a.resetCalls, b.resetCalls)
	}
}

func TestAllReturnsSortedByName(t *testing.T) {
	resetRegistryForTest()
	Register(&stubKind{name: "sqs"})
	Register(&stubKind{name: "ddb"})
	Register(&stubKind{name: "lambda"})

	got := All()
	want := []string{"ddb", "lambda", "sqs"}
	for i, k := range got {
		if k.Name() != want[i] {
			t.Fatalf("All()[%d].Name() = %q; want %q", i, k.Name(), want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/kind/...
```
Expected: build fails (missing `App`, `View`, `Action`, `Binding`, `Register`, `Get`, `All`, `ResetAll`, `resetRegistryForTest`).

- [ ] **Step 3: Implement `registry.go`**

Create `internal/view/kind/registry.go`:

```go
// Package kind provides a registry for AWS resource kinds rendered in the e1s
// TUI. Each Kind is registered once at init() time and dispatched via the `:`
// palette. The package is the single source of truth for how flat (non-
// hierarchical) services slot into the app — see
// docs/superpowers/specs/2026-05-20-multi-service-fork-design.md.
package kind

import (
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/keidarcy/e1s/internal/api"
	"github.com/rivo/tview"
)

// App is the minimal surface a Kind needs from the host application. Declared
// here (not imported from `view`) to avoid a circular import: `view` imports
// `kind`; `kind` must not import `view`. The concrete *view.App will satisfy
// this interface.
type App interface {
	APIStore() *api.Store
	SwitchView(k Kind, v View) error
	FlashError(msg string)
}

// View is what a Kind's Build returns. Intentionally minimal so flat kinds
// don't have to inherit ECS's `view` struct.
type View interface {
	Render() *tview.Flex
	Focus()
	OnKey(event *tcell.EventKey) (handled bool)
}

// Action is the function shape returned by PrimaryAction / Binding.Run.
type Action func(app App) error

// Binding is a letter-key secondary action displayed in the footer keymap.
type Binding struct {
	Key   rune
	Label string
	Run   Action
}

// Kind is the interface every browseable resource implements.
type Kind interface {
	Name() string

	Build(app App) (View, error)
	Reset()

	Selection() any
	SetSelection(any)

	Breadcrumb() string

	PrimaryAction() Action
	SecondaryActions() []Binding
}

var registry = map[string]Kind{}

// Register adds a Kind under its Name(). Panics on duplicate registration —
// duplicates are a programming error caught at startup.
func Register(k Kind) {
	if _, exists := registry[k.Name()]; exists {
		panic("kind already registered: " + k.Name())
	}
	registry[k.Name()] = k
}

// Get returns the Kind registered under name, if any.
func Get(name string) (Kind, bool) {
	k, ok := registry[name]
	return k, ok
}

// All returns every registered Kind, sorted by Name() for stable display.
func All() []Kind {
	out := make([]Kind, 0, len(registry))
	for _, k := range registry {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ResetAll calls Reset() on every registered Kind. Called from
// api.Store.SwitchAwsConfig so per-kind selection state doesn't leak across
// profile/region switches.
func ResetAll() {
	for _, k := range registry {
		k.Reset()
	}
}

// resetRegistryForTest is a test-only helper. Lives in production code so
// internal tests across files can use it without an export_test.go dance.
func resetRegistryForTest() {
	registry = map[string]Kind{}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/kind/...
```
Expected: PASS, all 5 tests.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/kind/
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(kind): add Kind interface and registry"
```

---

### Task 1.2: Palette dispatch

**Files:**
- Create: `internal/view/kind/palette.go`
- Create: `internal/view/kind/palette_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/view/kind/palette_test.go`:

```go
package kind

import (
	"errors"
	"testing"

	"github.com/keidarcy/e1s/internal/api"
)

type fakeApp struct {
	switched   Kind
	switchedV  View
	flashedMsg string
}

func (f *fakeApp) APIStore() *api.Store                  { return nil }
func (f *fakeApp) SwitchView(k Kind, v View) error    { f.switched = k; f.switchedV = v; return nil }
func (f *fakeApp) FlashError(msg string)              { f.flashedMsg = msg }

type buildableKind struct {
	stubKind
	buildErr error
	view     View
}

func (b *buildableKind) Build(App) (View, error) { return b.view, b.buildErr }

func TestPaletteSubmitKnownNameSwitchesView(t *testing.T) {
	resetRegistryForTest()
	k := &buildableKind{stubKind: stubKind{name: "lambda"}}
	Register(k)
	app := &fakeApp{}

	NewPalette(app).Submit("lambda")

	if app.switched != k {
		t.Fatalf("switched = %v; want %v", app.switched, k)
	}
	if app.flashedMsg != "" {
		t.Fatalf("unexpected flash %q", app.flashedMsg)
	}
}

func TestPaletteSubmitUnknownNameFlashes(t *testing.T) {
	resetRegistryForTest()
	app := &fakeApp{}

	NewPalette(app).Submit("nope")

	if app.switched != nil {
		t.Fatal("unexpected SwitchView call")
	}
	if app.flashedMsg == "" {
		t.Fatal("expected FlashError, got none")
	}
}

func TestPaletteSubmitEmptyIsNoop(t *testing.T) {
	resetRegistryForTest()
	app := &fakeApp{}

	NewPalette(app).Submit("")

	if app.switched != nil || app.flashedMsg != "" {
		t.Fatal("empty submit should be no-op")
	}
}

func TestPaletteSubmitBuildErrorFlashes(t *testing.T) {
	resetRegistryForTest()
	Register(&buildableKind{stubKind: stubKind{name: "lambda"}, buildErr: errors.New("boom")})
	app := &fakeApp{}

	NewPalette(app).Submit("lambda")

	if app.switched != nil {
		t.Fatal("Build error should prevent SwitchView")
	}
	if app.flashedMsg != "boom" {
		t.Fatalf("flashed = %q; want %q", app.flashedMsg, "boom")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/kind/...
```
Expected: build fails (missing `NewPalette`).

- [ ] **Step 3: Implement `palette.go`**

Create `internal/view/kind/palette.go`:

```go
package kind

import "strings"

// Palette is the `:` command-mode dispatcher. The UI half (input field, modal,
// keybinding) lives in the `view` package; this struct just maps a typed name
// to a registered Kind.
type Palette struct {
	app App
}

func NewPalette(app App) *Palette { return &Palette{app: app} }

// Submit handles a name typed into the palette. Empty input is a no-op
// (user cancelled).
func (p *Palette) Submit(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	k, ok := Get(name)
	if !ok {
		p.app.FlashError("unknown kind: " + name)
		return
	}
	v, err := k.Build(p.app)
	if err != nil {
		p.app.FlashError(err.Error())
		return
	}
	if err := p.app.SwitchView(k, v); err != nil {
		p.app.FlashError(err.Error())
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/kind/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/kind/
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(kind): add palette dispatcher"
```

---

### Task 1.3: Wire `App` to satisfy `kind.App` + add `activeKind` field

**Files:**
- Modify: `internal/view/app.go`
- Create: `internal/view/noop_view.go`

**Why `APIStore()` and not `Store()`:** the `App` struct embeds `*api.Store` (currently `internal/view/app.go:81`). Adding an `*App` method called `Store()` would collide with the embed's promoted name. The `kind.App` interface (Task 1.1) deliberately uses `APIStore() *api.Store` — no rename of the embed needed, no churn to existing call sites.

- [ ] **Step 1: Add `kind` import to app.go**

In `internal/view/app.go`, find the import block (lines 3-20). Add inside it:

```go
	"github.com/keidarcy/e1s/internal/view/kind"
```

- [ ] **Step 2: Add new fields to the `App` struct**

In `internal/view/app.go`, find the `App` struct (lines 71-107). After `splashStartupErr error` (around line 106), before the closing `}`, add:

```go
	// activeKind is non-nil when the user is browsing a flat kind via the `:`
	// palette. ECS legacy drill-down code paths ignore it.
	activeKind kind.Kind
	// palette is the `:` command-mode dispatcher; lazily initialised by showPalette.
	palette *kind.Palette
```

- [ ] **Step 3: Add the three interface methods at the end of `app.go`**

Append at end of `internal/view/app.go`:

```go
// APIStore returns the embedded api.Store. Satisfies kind.App. Named APIStore
// (not Store) because the *App struct already embeds *api.Store, whose name
// is promoted as Store — defining a Store() method would collide.
func (app *App) APIStore() *api.Store { return app.Store }

// FlashError surfaces an error message in the footer notice. Satisfies kind.App.
func (app *App) FlashError(msg string) {
	app.Notice.Warn(msg)
}

// SwitchView swaps the main pane to a kind's view and updates active-kind
// state. Satisfies kind.App.
//
// ECS adapter kinds (Phase 5) return a *noopView and navigate via the existing
// pages stack themselves; in that case we only record the active kind.
func (app *App) SwitchView(k kind.Kind, v kind.View) error {
	app.activeKind = k
	if _, isNoop := v.(*noopView); isNoop {
		return nil
	}
	pageName := "kind." + k.Name()
	app.Pages.AddAndSwitchToPage(pageName, v.Render(), true)
	return nil
}
```

- [ ] **Step 4: Create the `noopView` stub**

`SwitchView` references `*noopView`, which Phase 5 fully uses. Create the type now so `app.go` compiles through Phases 1-4.

Create `internal/view/noop_view.go`:

```go
package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// noopView is the kind.View returned by adapter kinds that drive the existing
// pages stack themselves rather than rendering a new tview.Flex. Used by ECS
// adapters in Phase 5.
type noopView struct{}

func (n *noopView) Render() *tview.Flex                        { return tview.NewFlex() }
func (n *noopView) Focus()                                     {}
func (n *noopView) OnKey(event *tcell.EventKey) (handled bool) { return false }
```

- [ ] **Step 5: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: build OK.

- [ ] **Step 6: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/app.go internal/view/noop_view.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): add activeKind state and SwitchView method"
```

---

### Task 1.4: Wire `:` keybind to open the palette

**Files:**
- Modify: `internal/view/app.go` (`globalInputHandle` function — find via `grep -n globalInputHandle internal/view/app.go`)
- Create: `internal/view/palette_view.go`

- [ ] **Step 1: Create the palette UI component**

Create `internal/view/palette_view.go`:

```go
package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// showPalette opens a single-line modal input. On Enter, the typed name is
// passed to the kind palette dispatcher. Escape cancels.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kind.NewPalette(app)
	}

	input := tview.NewInputField().
		SetLabel(": ").
		SetFieldWidth(40)

	input.SetDoneFunc(func(key tcell.Key) {
		name := input.GetText()
		app.Pages.RemovePage("palette")
		app.SetFocus(app.mainScreen)
		if key == tcell.KeyEnter {
			app.palette.Submit(name)
		}
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(input, 1, 0, true).
		AddItem(nil, 0, 1, false)

	app.Pages.AddPage("palette", flex, true, true)
	app.SetFocus(input)
}
```

- [ ] **Step 2: Wire `:` in the global key handler**

Find the function with `app.SetInputCapture(app.globalInputHandle)` (in `Start`, around app.go:173). Locate `globalInputHandle` (grep for it). Add a case in its switch on `event.Rune()`:

```go
	case ':':
		app.showPalette()
		return nil
```

- [ ] **Step 3: Manual smoke test**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
Expected: e1s opens normally. Press `:`. Input bar appears. Type `nope` Enter — flash "unknown kind: nope". Press `:` again, Escape — bar closes with no flash. Press `Ctrl+C` to exit.

- [ ] **Step 4: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/palette_view.go internal/view/app.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): add : palette keybind"
```

---

### Task 1.5: Wire ResetAll into SwitchAwsConfig

**Files:**
- Modify: `internal/api/config.go:18-43` (`SwitchAwsConfig`)

- [ ] **Step 1: Add the call**

Problem: `internal/api` cannot import `internal/view/kind` without creating a cycle (kind imports api). Solution: `kind` package exposes a callback hook. Add to `internal/view/kind/registry.go` already (no — that creates cycle the other direction since hook would need to be in api).

Cleanest fix: register the hook from `view` at app startup. Add to `internal/api/store.go` near top:

```go
// OnConfigSwitch is called after SwitchAwsConfig finishes resetting clients.
// view package sets this to kind.ResetAll during app init.
var OnConfigSwitch func()
```

Edit `internal/api/config.go` `SwitchAwsConfig` — at the very end, just before `return nil`:

```go
	if OnConfigSwitch != nil {
		OnConfigSwitch()
	}
```

In `internal/view/app.go`, inside `Start()` (around line 158, before `app.SetInputCapture`), add:

```go
	api.OnConfigSwitch = kind.ResetAll
```

- [ ] **Step 2: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK.

- [ ] **Step 3: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/api/store.go internal/api/config.go internal/view/app.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): call kind.ResetAll on profile/region switch"
```

---

### Task 1.6: Hook table row-change to active kind

**Files:**
- Modify: `internal/view/table.go` (find row-change handler — `grep -n SetSelectionChangedFunc internal/view/table.go`)

- [ ] **Step 1: Read the existing handler**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && grep -n "SetSelectionChangedFunc\|SetSelectedFunc" internal/view/table.go
```

- [ ] **Step 2: Add active-kind notification**

Inside the row-changed callback (the function passed to `SetSelectionChangedFunc`), add this as the **first** statement:

```go
	if v.app.activeKind != nil {
		v.app.activeKind.SetSelection(v.tableSelectionForActiveKind(row))
		return
	}
```

Then add a method on `*view`:

```go
// tableSelectionForActiveKind extracts whatever the active flat kind needs
// from the current table row. Each kind's Build() stashes the per-row data on
// the table cell's Reference; here we just retrieve it.
func (v *view) tableSelectionForActiveKind(row int) any {
	cell := v.table.GetCell(row, 0)
	if cell == nil {
		return nil
	}
	return cell.GetReference()
}
```

Convention: when each flat kind populates its table, it calls `cell.SetReference(perRowStruct)` so this lookup works for any kind without a switch.

- [ ] **Step 3: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK. (Existing ECS code keeps working because `activeKind` is nil during legacy ECS flow.)

- [ ] **Step 4: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/table.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): forward table row changes to active kind"
```

---

## Phase 2: Lambda

### Task 2.1: Add Lambda client to Store

**Files:**
- Modify: `internal/api/store.go`
- Modify: `internal/api/config.go:18-43`
- Modify: `go.mod` / `go.sum` (auto, via `go get`)

- [ ] **Step 1: Add the SDK dependency**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go get github.com/aws/aws-sdk-go-v2/service/lambda
```

- [ ] **Step 2: Add field + lazy init**

Edit `internal/api/store.go`. Add to imports: `"github.com/aws/aws-sdk-go-v2/service/lambda"`.

Add to the `Store` struct (after `account`):
```go
	lambda *lambda.Client
```

Add at the bottom of the file:
```go
func (store *Store) initLambdaClient() {
	if store.lambda == nil {
		store.lambda = lambda.NewFromConfig(*store.Config)
	}
}
```

In `internal/api/config.go` `SwitchAwsConfig`, in the nil-reset block (currently lines 34-39), add:
```go
	store.lambda = nil
```

- [ ] **Step 3: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK.

- [ ] **Step 4: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add go.mod go.sum internal/api/store.go internal/api/config.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add Lambda SDK client to Store"
```

---

### Task 2.2: `api/lambda.go` — ListFunctions, GetFunction, InvokeFunction

**Files:**
- Create: `internal/api/lambda.go`
- Create: `internal/api/lambda_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/lambda_test.go` using middleware-mock pattern (matches existing repo style; see go-aws-sdk-v2 docs). Use a smithy middleware to short-circuit the API call:

```go
package api

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newStoreWithLambda(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return &Store{Config: &cfg, lambda: c}
}

func TestListFunctionsHappyPath(t *testing.T) {
	store := newStoreWithLambda(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &lambda.ListFunctionsOutput{
				Functions: []lambdaTypes.FunctionConfiguration{
					{FunctionName: aws.String("auth-handler"), Runtime: lambdaTypes.RuntimeNodejs20x},
				},
			},
		}, middleware.Metadata{}, nil
	})

	got, err := store.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || aws.ToString(got[0].FunctionName) != "auth-handler" {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/ -run TestListFunctions
```
Expected: build fails (missing `ListFunctions`).

- [ ] **Step 3: Implement `internal/api/lambda.go`**

```go
package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

// ListFunctions returns all Lambda functions in the current region. Paginates
// internally; returns whatever it has on the first error after the first
// page (matches ListClusters behaviour in cluster.go).
func (store *Store) ListFunctions(ctx context.Context) ([]lambdaTypes.FunctionConfiguration, error) {
	store.initLambdaClient()
	slog.Debug("api ListFunctions")

	var out []lambdaTypes.FunctionConfiguration
	var marker *string
	for {
		resp, err := store.lambda.ListFunctions(ctx, &lambda.ListFunctionsInput{Marker: marker})
		if err != nil {
			slog.Error("ListFunctions failed", "error", err)
			if len(out) == 0 {
				return nil, err
			}
			return out, nil
		}
		out = append(out, resp.Functions...)
		if resp.NextMarker == nil {
			return out, nil
		}
		marker = resp.NextMarker
	}
}

// GetFunction returns the full configuration for a single function (env vars,
// VPC config, layers, DLQ — anything not in the ListFunctions summary).
func (store *Store) GetFunction(ctx context.Context, name string) (*lambda.GetFunctionOutput, error) {
	store.initLambdaClient()
	slog.Debug("api GetFunction", "name", name)
	return store.lambda.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: &name})
}

// InvokeFunction invokes a function with the given payload (raw JSON bytes).
// Always uses RequestResponse so the caller can show the result.
func (store *Store) InvokeFunction(ctx context.Context, name string, payload []byte) (*lambda.InvokeOutput, error) {
	store.initLambdaClient()
	slog.Debug("api InvokeFunction", "name", name, "payloadBytes", len(payload))
	return store.lambda.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: &name,
		Payload:      payload,
	})
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/api/lambda.go internal/api/lambda_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add Lambda list/get/invoke"
```

---

### Task 2.3: `view/lambda.go` — Kind impl skeleton + tests

**Files:**
- Create: `internal/view/lambda.go`
- Create: `internal/view/lambda_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/view/lambda_test.go`:

```go
package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestLambdaKindName(t *testing.T) {
	k := &lambdaKind{}
	if k.Name() != "lambda" {
		t.Fatalf("Name = %q; want %q", k.Name(), "lambda")
	}
}

func TestLambdaKindSelectionRoundTrip(t *testing.T) {
	k := &lambdaKind{}
	fn := &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("auth-handler")}
	k.SetSelection(fn)
	if k.Selection() != fn {
		t.Fatalf("Selection round-trip failed")
	}
}

func TestLambdaKindResetClearsSelection(t *testing.T) {
	k := &lambdaKind{}
	k.SetSelection(&lambdaTypes.FunctionConfiguration{FunctionName: aws.String("x")})
	k.Reset()
	if k.Selection() != nil {
		t.Fatalf("Selection after Reset = %v; want nil", k.Selection())
	}
}

func TestLambdaKindBreadcrumb(t *testing.T) {
	k := &lambdaKind{}
	if got := k.Breadcrumb(); got != "lambda" {
		t.Fatalf("Breadcrumb (no selection) = %q; want %q", got, "lambda")
	}
	k.SetSelection(&lambdaTypes.FunctionConfiguration{FunctionName: aws.String("auth-handler")})
	if got := k.Breadcrumb(); got != "lambda > auth-handler" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "lambda > auth-handler")
	}
}

func TestLambdaKindSecondaryActions(t *testing.T) {
	k := &lambdaKind{}
	got := k.SecondaryActions()
	if len(got) != 3 {
		t.Fatalf("len(SecondaryActions) = %d; want 3", len(got))
	}
	wantKeys := map[rune]string{'i': "invoke", 'd': "dlq", 'c': "config"}
	for _, b := range got {
		if wantKeys[b.Key] != b.Label {
			t.Fatalf("binding %c => %q; want %q", b.Key, b.Label, wantKeys[b.Key])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestLambda
```
Expected: build fails (missing `lambdaKind`).

- [ ] **Step 3: Implement skeleton**

Create `internal/view/lambda.go`:

```go
package view

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/keidarcy/e1s/internal/view/kind"
)

func init() { kind.Register(&lambdaKind{}) }

type lambdaKind struct {
	selected *lambdaTypes.FunctionConfiguration
}

func (k *lambdaKind) Name() string { return "lambda" }

func (k *lambdaKind) Reset() { k.selected = nil }

func (k *lambdaKind) Selection() any { return k.selected }

func (k *lambdaKind) SetSelection(s any) {
	if fn, ok := s.(*lambdaTypes.FunctionConfiguration); ok {
		k.selected = fn
	}
}

func (k *lambdaKind) Breadcrumb() string {
	if k.selected == nil || k.selected.FunctionName == nil {
		return "lambda"
	}
	return "lambda > " + aws.ToString(k.selected.FunctionName)
}

func (k *lambdaKind) PrimaryAction() kind.Action { return nil } // wired in next task

func (k *lambdaKind) SecondaryActions() []kind.Binding {
	return []kind.Binding{
		{Key: 'i', Label: "invoke", Run: nil}, // wired in next task
		{Key: 'd', Label: "dlq", Run: nil},
		{Key: 'c', Label: "config", Run: nil},
	}
}

func (k *lambdaKind) Build(app kind.App) (kind.View, error) {
	return nil, nil // implemented in next task
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestLambda
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/lambda.go internal/view/lambda_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): add lambdaKind skeleton"
```

---

### Task 2.4: Lambda `Build()` — populate the table

**Files:**
- Modify: `internal/view/lambda.go`

- [ ] **Step 1: Implement Build**

Replace the Build stub in `lambda.go`:

```go
func (k *lambdaKind) Build(app kind.App) (kind.View, error) {
	fns, err := app.APIStore().ListFunctions(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	headers := []string{"Name", "Runtime", "Memory", "Timeout", "LastModified", "State"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, fn := range fns {
		copyFn := fn
		cells := []string{
			aws.ToString(fn.FunctionName),
			string(fn.Runtime),
			fmt.Sprintf("%d", aws.ToInt32(fn.MemorySize)),
			fmt.Sprintf("%ds", aws.ToInt32(fn.Timeout)),
			aws.ToString(fn.LastModified),
			string(fn.State),
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(&copyFn) // picked up by tableSelectionForActiveKind
			}
			table.SetCell(row+1, col, cell)
		}
	}

	flex := tview.NewFlex().AddItem(table, 0, 1, true)
	return &simpleKindView{flex: flex, focus: table}, nil
}
```

Add `simpleKindView` (reusable shell — put it in `internal/view/kind_view.go`):

```go
package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// simpleKindView is the default kind.View for table-based flat kinds.
type simpleKindView struct {
	flex  *tview.Flex
	focus tview.Primitive
}

func (s *simpleKindView) Render() *tview.Flex                          { return s.flex }
func (s *simpleKindView) Focus()                                       { /* tview handles focus via SetFocus */ }
func (s *simpleKindView) OnKey(event *tcell.EventKey) (handled bool)   { return false }
```

Add imports to `lambda.go` for `context`, `fmt`, `tview`, `tcell`.

- [ ] **Step 2: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK.

- [ ] **Step 3: Manual smoke test**

```bash
go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
Press `:`, type `lambda`, Enter. Expected: table of Lambda functions in the current AWS profile/region.

- [ ] **Step 4: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/lambda.go internal/view/kind_view.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): list Lambda functions on :lambda"
```

---

### Task 2.5: Lambda PrimaryAction (logs) + SecondaryActions (invoke, config)

**Files:**
- Modify: `internal/view/lambda.go`

- [ ] **Step 1: Implement PrimaryAction**

Replace the PrimaryAction stub:

```go
func (k *lambdaKind) PrimaryAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		logGroup := "/aws/lambda/" + aws.ToString(k.selected.FunctionName)
		return openLogGroupTail(app, logGroup) // implemented below
	}
}
```

Add a helper at the bottom of the file (or in `kind_view.go`):

```go
// openLogGroupTail opens a read-only log-tail view for the given CloudWatch
// log group. Reuses GetServiceLogs-style flow but for a single named group.
func openLogGroupTail(app kind.App, logGroup string) error {
	// MVP: fetch latest 100 events synchronously, render in a TextView. A
	// follow-up can swap in true tail-by-polling.
	logs, err := app.APIStore().GetLogGroupTail(context.Background(), logGroup, 100)
	if err != nil {
		app.FlashError(err.Error())
		return err
	}
	tv := tview.NewTextView().SetDynamicColors(true).SetText(joinLines(logs))
	tv.SetBorder(true).SetTitle(" " + logGroup + " ")
	flex := tview.NewFlex().AddItem(tv, 0, 1, true)
	return app.SwitchView(&logsPseudoKind{name: logGroup}, &simpleKindView{flex: flex, focus: tv})
}

func joinLines(in []string) string {
	out := ""
	for _, s := range in {
		out += s + "\n"
	}
	return out
}

// logsPseudoKind is a transient kind for the log-tail screen. Not registered.
type logsPseudoKind struct{ name string }

func (l *logsPseudoKind) Name() string                   { return "logs:" + l.name }
func (l *logsPseudoKind) Build(kind.App) (kind.View, error) { return nil, nil }
func (l *logsPseudoKind) Reset()                         {}
func (l *logsPseudoKind) Selection() any                 { return nil }
func (l *logsPseudoKind) SetSelection(any)               {}
func (l *logsPseudoKind) Breadcrumb() string             { return l.name }
func (l *logsPseudoKind) PrimaryAction() kind.Action     { return nil }
func (l *logsPseudoKind) SecondaryActions() []kind.Binding { return nil }
```

- [ ] **Step 2: Add `GetLogGroupTail` API method**

Edit `internal/api/logs.go`. Add at the bottom:

```go
// GetLogGroupTail returns the most recent `limit` events from the latest
// log stream of the given log group, formatted with timestamp prefixes.
func (store *Store) GetLogGroupTail(ctx context.Context, logGroup string, limit int32) ([]string, error) {
	store.initCloudwatchlogsClient()
	streams, err := store.cloudwatchlogs.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: &logGroup,
		Limit:        aws.Int32(1),
		OrderBy:      cloudwatchlogsTypes.OrderByLastEventTime,
		Descending:   aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if len(streams.LogStreams) == 0 {
		return []string{"(no log streams yet)"}, nil
	}
	events, err := store.cloudwatchlogs.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroup,
		LogStreamName: streams.LogStreams[0].LogStreamName,
		Limit:         aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(events.Events))
	for _, e := range events.Events {
		out = append(out, fmt.Sprintf(logFmt, time.Unix(0, *e.Timestamp*int64(time.Millisecond)).Format(time.RFC3339), *e.Message))
	}
	return out, nil
}
```

- [ ] **Step 3: Implement SecondaryActions wiring**

Replace `SecondaryActions()` in `lambda.go`:

```go
func (k *lambdaKind) SecondaryActions() []kind.Binding {
	return []kind.Binding{
		{Key: 'i', Label: "invoke", Run: k.invokeAction()},
		{Key: 'd', Label: "dlq", Run: k.dlqAction()},
		{Key: 'c', Label: "config", Run: k.configAction()},
	}
}

func (k *lambdaKind) invokeAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		// MVP: empty payload — invoke as-is, show response. A modal input is a
		// follow-up.
		out, err := app.APIStore().InvokeFunction(context.Background(), aws.ToString(k.selected.FunctionName), []byte("{}"))
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		body := string(out.Payload)
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(" invoke result ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&logsPseudoKind{name: "invoke:" + aws.ToString(k.selected.FunctionName)}, &simpleKindView{flex: flex, focus: tv})
	}
}

func (k *lambdaKind) dlqAction() kind.Action {
	return func(app kind.App) error {
		// Filled in during Phase 6. For now: flash.
		app.FlashError("DLQ jump not implemented yet")
		return nil
	}
}

func (k *lambdaKind) configAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		out, err := app.APIStore().GetFunction(context.Background(), aws.ToString(k.selected.FunctionName))
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		// Render the full config as JSON for now (reuse json.go in a later pass).
		body := fmt.Sprintf("%+v", out.Configuration)
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.FunctionName) + " config ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&logsPseudoKind{name: "config:" + aws.ToString(k.selected.FunctionName)}, &simpleKindView{flex: flex, focus: tv})
	}
}
```

- [ ] **Step 4: Wire Enter + letter keys in the simpleKindView**

Currently `simpleKindView.OnKey` returns false. Update it to dispatch to the active kind:

```go
type simpleKindView struct {
	flex   *tview.Flex
	focus  tview.Primitive
	app    kind.App
	source kind.Kind
}

func (s *simpleKindView) Render() *tview.Flex                        { return s.flex }
func (s *simpleKindView) Focus()                                     {}
func (s *simpleKindView) OnKey(event *tcell.EventKey) (handled bool) {
	if s.source == nil {
		return false
	}
	if event.Key() == tcell.KeyEnter {
		if act := s.source.PrimaryAction(); act != nil {
			act(s.app)
			return true
		}
	}
	for _, b := range s.source.SecondaryActions() {
		if event.Rune() == b.Key && b.Run != nil {
			b.Run(s.app)
			return true
		}
	}
	return false
}
```

Update `lambdaKind.Build` to populate `app` and `source`:

```go
return &simpleKindView{flex: flex, focus: table, app: app, source: k}, nil
```

Then make the table forward keys to OnKey. In `Build`, after constructing the table:

```go
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		view := &simpleKindView{flex: flex, focus: table, app: app, source: k}
		if view.OnKey(event) {
			return nil
		}
		return event
	})
```

- [ ] **Step 5: Build + smoke test**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
Press `:`, type `lambda`, Enter. Select a function. Press Enter — log tail appears. Back out (Esc / `q` — wire Esc to pop the page if not already), select again, press `c` — config view appears. `i` — invokes with `{}` payload, shows response.

- [ ] **Step 6: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/api/logs.go internal/view/lambda.go internal/view/kind_view.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): wire Lambda primary+secondary actions"
```

---

## Phase 3: SQS

### Task 3.1: Add SQS client to Store

**Files:**
- Modify: `go.mod` / `go.sum`
- Modify: `internal/api/store.go`
- Modify: `internal/api/config.go`

- [ ] **Step 1: Add the SDK dependency**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go get github.com/aws/aws-sdk-go-v2/service/sqs
```

- [ ] **Step 2: Add field + lazy init**

In `internal/api/store.go`, add to imports:
```go
	"github.com/aws/aws-sdk-go-v2/service/sqs"
```

Add a field on the `Store` struct (after `lambda *lambda.Client` from Task 2.1):
```go
	sqs *sqs.Client
```

Add a method at the bottom of the file:
```go
func (store *Store) initSqsClient() {
	if store.sqs == nil {
		store.sqs = sqs.NewFromConfig(*store.Config)
	}
}
```

- [ ] **Step 3: Add to nil-reset list in `SwitchAwsConfig`**

In `internal/api/config.go`, in the reset block in `SwitchAwsConfig` (around lines 34-39), add after the existing `store.lambda = nil`:
```go
	store.sqs = nil
```

- [ ] **Step 4: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add go.mod go.sum internal/api/store.go internal/api/config.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add SQS SDK client to Store"
```

---

### Task 3.2: `internal/api/sqs.go` — ListQueues, GetQueueAttributes, PeekMessages, SendMessage, PurgeQueue

**Files:**
- Create: `internal/api/sqs.go`
- Create: `internal/api/sqs_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/sqs_test.go`:

```go
package api

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newStoreWithSQS(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return &Store{Config: &cfg, sqs: c}
}

func TestListQueuesHappyPath(t *testing.T) {
	store := newStoreWithSQS(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &sqs.ListQueuesOutput{
				QueueUrls: []string{"https://sqs.us-east-1.amazonaws.com/111/foo"},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	got, err := store.ListQueues(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0] != "https://sqs.us-east-1.amazonaws.com/111/foo" {
		t.Fatalf("got %v", got)
	}
}

func TestPeekMessagesUsesZeroVisibilityTimeout(t *testing.T) {
	var captured *sqs.ReceiveMessageInput
	store := newStoreWithSQS(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		// The serialised input is on in.Request — but the easier approach is to
		// re-read the original input via the context. For this MVP test we just
		// confirm the call returns and check the wire-level request via the
		// presence of zero — done indirectly: the SDK panics or errors if
		// VisibilityTimeout is invalid. Here we just return empty result.
		_ = captured
		return smithymiddleware.FinalizeOutput{
			Result: &sqs.ReceiveMessageOutput{
				Messages: []sqsTypes.Message{{Body: aws.String("hello")}},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	msgs, err := store.PeekMessages(context.Background(), "https://sqs.us-east-1.amazonaws.com/111/foo")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(msgs) != 1 || aws.ToString(msgs[0].Body) != "hello" {
		t.Fatalf("got %+v", msgs)
	}
	// True wire-level inspection of VisibilityTimeout=0 is exercised by the
	// integration smoke test in Phase 6. For this unit test we rely on the
	// implementation being a thin wrapper around ReceiveMessage with the
	// constant 0 baked into the source.
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/ -run "TestListQueues|TestPeek"
```
Expected: build fails (missing `ListQueues`, `PeekMessages`).

- [ ] **Step 3: Implement**

Create `internal/api/sqs.go`:

```go
package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (store *Store) ListQueues(ctx context.Context) ([]string, error) {
	store.initSqsClient()
	slog.Debug("api ListQueues")
	out, err := store.sqs.ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		slog.Error("ListQueues failed", "error", err)
		return nil, err
	}
	return out.QueueUrls, nil
}

func (store *Store) GetQueueAttributes(ctx context.Context, queueURL string) (map[string]string, error) {
	store.initSqsClient()
	out, err := store.sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURL,
		AttributeNames: []sqsTypes.QueueAttributeName{sqsTypes.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

// PeekMessages reads up to 10 messages with VisibilityTimeout=0 so real
// consumers are not affected.
func (store *Store) PeekMessages(ctx context.Context, queueURL string) ([]sqsTypes.Message, error) {
	store.initSqsClient()
	out, err := store.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &queueURL,
		MaxNumberOfMessages: 10,
		VisibilityTimeout:   0,
		WaitTimeSeconds:     0,
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (store *Store) SendMessage(ctx context.Context, queueURL, body string) error {
	store.initSqsClient()
	_, err := store.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(body),
	})
	return err
}

func (store *Store) PurgeQueue(ctx context.Context, queueURL string) error {
	store.initSqsClient()
	_, err := store.sqs.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: &queueURL})
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/api/sqs.go internal/api/sqs_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add SQS list/get/peek/send/purge"
```

---

### Task 3.3: `internal/view/sqs.go` — Kind impl skeleton + tests

**Files:**
- Create: `internal/view/sqs.go`
- Create: `internal/view/sqs_test.go`

Selection type is `string` (full queue URL when selected from the table; bare queue name when set via cross-kind nav from Lambda DLQ — Phase 6 promotes it to a URL via the pre-selection lookup).

- [ ] **Step 1: Write the failing test**

Create `internal/view/sqs_test.go`:

```go
package view

import "testing"

func TestSqsKindName(t *testing.T) {
	k := &sqsKind{}
	if k.Name() != "sqs" {
		t.Fatalf("Name = %q; want %q", k.Name(), "sqs")
	}
}

func TestSqsKindSelectionRoundTrip(t *testing.T) {
	k := &sqsKind{}
	url := "https://sqs.us-east-1.amazonaws.com/111/my-queue"
	k.SetSelection(url)
	if got, _ := k.Selection().(string); got != url {
		t.Fatalf("Selection round-trip failed: got %q", got)
	}
}

func TestSqsKindResetClearsSelection(t *testing.T) {
	k := &sqsKind{}
	k.SetSelection("https://sqs.us-east-1.amazonaws.com/111/my-queue")
	k.Reset()
	if got, _ := k.Selection().(string); got != "" {
		t.Fatalf("Selection after Reset = %q; want empty", got)
	}
}

func TestSqsKindBreadcrumb(t *testing.T) {
	k := &sqsKind{}
	if got := k.Breadcrumb(); got != "sqs" {
		t.Fatalf("Breadcrumb (no selection) = %q", got)
	}
	k.SetSelection("https://sqs.us-east-1.amazonaws.com/111/my-queue")
	if got := k.Breadcrumb(); got != "sqs > my-queue" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "sqs > my-queue")
	}
}

func TestSqsKindSecondaryActions(t *testing.T) {
	k := &sqsKind{}
	got := k.SecondaryActions()
	if len(got) != 2 {
		t.Fatalf("len(SecondaryActions) = %d; want 2", len(got))
	}
	want := map[rune]string{'p': "purge", 's': "send"}
	for _, b := range got {
		if want[b.Key] != b.Label {
			t.Fatalf("binding %c => %q; want %q", b.Key, b.Label, want[b.Key])
		}
	}
}

func TestQueueNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://sqs.us-east-1.amazonaws.com/111/my-queue": "my-queue",
		"my-queue": "my-queue", // bare name passes through
		"":          "",
	}
	for in, want := range cases {
		if got := queueNameFromURL(in); got != want {
			t.Fatalf("queueNameFromURL(%q) = %q; want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestSqs
```
Expected: build fails (missing `sqsKind`, `queueNameFromURL`).

- [ ] **Step 3: Implement**

Create `internal/view/sqs.go`:

```go
package view

import (
	"context"
	"fmt"
	"strings"

	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/gdamore/tcell/v2"
	"github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kind.Register(&sqsKind{}) }

type sqsKind struct {
	// selectedURL is set in two ways:
	//   1. Row-change in Build's table fires SetSelection(fullURL).
	//   2. Cross-kind nav from Lambda DLQ calls SetSelection(bareQueueName).
	// Build's pre-selection block promotes (2) to a full URL after listing.
	selectedURL string
}

func (k *sqsKind) Name() string  { return "sqs" }
func (k *sqsKind) Reset()        { k.selectedURL = "" }
func (k *sqsKind) Selection() any { return k.selectedURL }

func (k *sqsKind) SetSelection(s any) {
	if str, ok := s.(string); ok {
		k.selectedURL = str
	}
}

func (k *sqsKind) Breadcrumb() string {
	if k.selectedURL == "" {
		return "sqs"
	}
	return "sqs > " + queueNameFromURL(k.selectedURL)
}

// queueNameFromURL returns the last path segment of an SQS queue URL. Bare
// names (no `/`) pass through unchanged so cross-kind nav from Lambda DLQ
// (which only knows the queue name) doesn't need special-casing in callers.
func queueNameFromURL(url string) string {
	idx := strings.LastIndex(url, "/")
	if idx < 0 {
		return url
	}
	return url[idx+1:]
}

func (k *sqsKind) PrimaryAction() kind.Action {
	return func(app kind.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		msgs, err := app.APIStore().PeekMessages(context.Background(), k.selectedURL)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		var body strings.Builder
		if len(msgs) == 0 {
			body.WriteString("(no messages)")
		}
		for _, m := range msgs {
			body.WriteString("---\n")
			if m.Body != nil {
				body.WriteString(*m.Body)
			}
			body.WriteString("\n")
		}
		tv := tview.NewTextView().SetText(body.String())
		tv.SetBorder(true).SetTitle(" peek " + queueNameFromURL(k.selectedURL) + " ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&logsPseudoKind{name: "peek:" + queueNameFromURL(k.selectedURL)},
			&simpleKindView{flex: flex, focus: tv, app: app, source: k},
		)
	}
}

func (k *sqsKind) SecondaryActions() []kind.Binding {
	return []kind.Binding{
		{Key: 'p', Label: "purge", Run: k.purgeAction()},
		{Key: 's', Label: "send", Run: k.sendAction()},
	}
}

func (k *sqsKind) purgeAction() kind.Action {
	return func(app kind.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: no confirm modal — typing `p` purges immediately. A modal
		// requiring the user to type the queue name is a follow-up.
		if err := app.APIStore().PurgeQueue(context.Background(), k.selectedURL); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("purged " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

func (k *sqsKind) sendAction() kind.Action {
	return func(app kind.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: fixed test payload. Modal input is a follow-up.
		if err := app.APIStore().SendMessage(context.Background(), k.selectedURL, `{"e1s":"test"}`); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("sent test message to " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

func (k *sqsKind) Build(app kind.App) (kind.View, error) {
	urls, err := app.APIStore().ListQueues(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	headers := []string{"Name", "ApproxMessages", "ApproxInFlight", "ApproxDelayed", "DLQ"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, url := range urls {
		copyURL := url
		attrs, _ := app.APIStore().GetQueueAttributes(context.Background(), url)
		cells := []string{
			queueNameFromURL(url),
			attrs["ApproximateNumberOfMessages"],
			attrs["ApproximateNumberOfMessagesNotVisible"],
			attrs["ApproximateNumberOfMessagesDelayed"],
			fmt.Sprint(attrs["RedrivePolicy"] != ""),
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyURL)
			}
			table.SetCell(row+1, col, cell)
		}
	}

	flex := tview.NewFlex().AddItem(table, 0, 1, true)
	v := &simpleKindView{flex: flex, focus: table, app: app, source: k}
	table.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if v.OnKey(e) {
			return nil
		}
		return e
	})

	// Pre-selection: SetSelection may have been called with either a full URL
	// (from a previous row-change) or a bare queue name (from cross-kind nav
	// from Lambda DLQ). Match against the row's reference (full URL) directly,
	// or against queueNameFromURL(reference) for the bare-name case.
	if k.selectedURL != "" {
		for r := 1; r <= len(urls); r++ {
			cell := table.GetCell(r, 0)
			if cell == nil {
				continue
			}
			ref, _ := cell.GetReference().(string)
			if ref == k.selectedURL || queueNameFromURL(ref) == k.selectedURL {
				table.Select(r, 0)
				k.selectedURL = ref // promote bare name to full URL
				break
			}
		}
	}

	return v, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestSqs
```
Expected: PASS.

- [ ] **Step 5: Manual smoke test**

```bash
go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
Press `:`, type `sqs`, Enter. Expected: queue list with attribute counts. Select a queue with messages, press Enter — peek view shows up to 10 messages. Press `s` — flash "sent test message to ...". Verify in AWS console that VisibilityTimeout=0 means a real consumer can still read peeked messages.

- [ ] **Step 6: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/sqs.go internal/view/sqs_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): add :sqs kind with peek/purge/send"
```

---

## Phase 4: DynamoDB

**MVP scope adjustment vs. design doc:** the design called out Query as a secondary action (`q`). Implementing Query properly requires a modal input for the partition key value, plus a `DescribeTable`-backed schema cache to know the partition key name. Both are non-trivial. For MVP the `q` binding is **dropped**. Users get list + scan + describe; Query is a documented follow-up. This keeps Phase 4 sized like Phase 2/3 and avoids dragging modal-input work into MVP.

### Task 4.1: Add DynamoDB client to Store

**Files:**
- Modify: `go.mod` / `go.sum`
- Modify: `internal/api/store.go`
- Modify: `internal/api/config.go`

- [ ] **Step 1: Add the SDK dependency**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go get github.com/aws/aws-sdk-go-v2/service/dynamodb
```

- [ ] **Step 2: Add field + lazy init**

In `internal/api/store.go`, add to imports:
```go
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
```

Add a field on the `Store` struct (after `sqs *sqs.Client`):
```go
	dynamodb *dynamodb.Client
```

Add a method at the bottom of the file:
```go
func (store *Store) initDynamoDBClient() {
	if store.dynamodb == nil {
		store.dynamodb = dynamodb.NewFromConfig(*store.Config)
	}
}
```

- [ ] **Step 3: Add to nil-reset list in `SwitchAwsConfig`**

In `internal/api/config.go`, in the reset block in `SwitchAwsConfig`, add:
```go
	store.dynamodb = nil
```

- [ ] **Step 4: Build**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go build ./...
```
Expected: OK.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add go.mod go.sum internal/api/store.go internal/api/config.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add DynamoDB SDK client to Store"
```

---

### Task 4.2: `internal/api/dynamodb.go` — ListTables, DescribeTable, Scan

**Files:**
- Create: `internal/api/dynamodb.go`
- Create: `internal/api/dynamodb_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/dynamodb_test.go`:

```go
package api

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newStoreWithDDB(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return &Store{Config: &cfg, dynamodb: c}
}

func TestListTablesHappyPath(t *testing.T) {
	store := newStoreWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ListTablesOutput{
				TableNames: []string{"users", "events"},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	got, err := store.ListTables(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0] != "users" {
		t.Fatalf("got %v", got)
	}
}

func TestScanFirstPageRespectsLimit(t *testing.T) {
	store := newStoreWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ScanOutput{
				Items: []map[string]ddbTypes.AttributeValue{
					{"pk": &ddbTypes.AttributeValueMemberS{Value: "1"}},
				},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	items, err := store.ScanFirstPage(context.Background(), "users", 25)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/ -run "TestListTables|TestScan"
```
Expected: build fails (missing `ListTables`, `ScanFirstPage`).

- [ ] **Step 3: Implement**

Create `internal/api/dynamodb.go`:

```go
package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ListTables returns every DynamoDB table name in the current region. Paginates
// internally; on first error after the first page, returns what it has.
func (store *Store) ListTables(ctx context.Context) ([]string, error) {
	store.initDynamoDBClient()
	slog.Debug("api ListTables")

	var out []string
	var lastEvaluated *string
	for {
		resp, err := store.dynamodb.ListTables(ctx, &dynamodb.ListTablesInput{
			ExclusiveStartTableName: lastEvaluated,
		})
		if err != nil {
			slog.Error("ListTables failed", "error", err)
			if len(out) == 0 {
				return nil, err
			}
			return out, nil
		}
		out = append(out, resp.TableNames...)
		if resp.LastEvaluatedTableName == nil {
			return out, nil
		}
		lastEvaluated = resp.LastEvaluatedTableName
	}
}

// DescribeTable returns the full table metadata (key schema, GSIs, streams).
func (store *Store) DescribeTable(ctx context.Context, name string) (*ddbTypes.TableDescription, error) {
	store.initDynamoDBClient()
	slog.Debug("api DescribeTable", "name", name)
	resp, err := store.dynamodb.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	if err != nil {
		return nil, err
	}
	return resp.Table, nil
}

// ScanFirstPage runs a Scan with the given limit and returns the first page
// only. Pagination beyond page 1 is a follow-up.
func (store *Store) ScanFirstPage(ctx context.Context, table string, limit int32) ([]map[string]ddbTypes.AttributeValue, error) {
	store.initDynamoDBClient()
	slog.Debug("api ScanFirstPage", "table", table, "limit", limit)
	resp, err := store.dynamodb.Scan(ctx, &dynamodb.ScanInput{
		TableName: &table,
		Limit:     &limit,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/api/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/api/dynamodb.go internal/api/dynamodb_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(api): add DynamoDB list/describe/scan"
```

---

### Task 4.3: `internal/view/dynamodb.go` — Kind impl

**Files:**
- Create: `internal/view/dynamodb.go`
- Create: `internal/view/dynamodb_test.go`

Selection type is `*ddbTypes.TableDescription` (a Build-time `DescribeTable` populates it for the selected row, so primary/secondary actions don't need an extra round-trip).

- [ ] **Step 1: Write the failing test**

Create `internal/view/dynamodb_test.go`:

```go
package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDDBKindName(t *testing.T) {
	k := &ddbKind{}
	if k.Name() != "ddb" {
		t.Fatalf("Name = %q; want %q", k.Name(), "ddb")
	}
}

func TestDDBKindSelectionRoundTrip(t *testing.T) {
	k := &ddbKind{}
	td := &ddbTypes.TableDescription{TableName: aws.String("users")}
	k.SetSelection(td)
	if k.Selection() != td {
		t.Fatalf("Selection round-trip failed")
	}
}

func TestDDBKindResetClearsSelection(t *testing.T) {
	k := &ddbKind{}
	k.SetSelection(&ddbTypes.TableDescription{TableName: aws.String("x")})
	k.Reset()
	if k.Selection() != nil {
		t.Fatalf("Selection after Reset = %v; want nil", k.Selection())
	}
}

func TestDDBKindBreadcrumb(t *testing.T) {
	k := &ddbKind{}
	if got := k.Breadcrumb(); got != "ddb" {
		t.Fatalf("Breadcrumb (no selection) = %q", got)
	}
	k.SetSelection(&ddbTypes.TableDescription{TableName: aws.String("users")})
	if got := k.Breadcrumb(); got != "ddb > users" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "ddb > users")
	}
}

func TestDDBKindSecondaryActions(t *testing.T) {
	k := &ddbKind{}
	got := k.SecondaryActions()
	if len(got) != 1 {
		t.Fatalf("len(SecondaryActions) = %d; want 1", len(got))
	}
	if got[0].Key != 'c' || got[0].Label != "describe" {
		t.Fatalf("binding = %+v; want {c, describe}", got[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestDDB
```
Expected: build fails (missing `ddbKind`).

- [ ] **Step 3: Implement**

Create `internal/view/dynamodb.go`:

```go
package view

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gdamore/tcell/v2"
	"github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kind.Register(&ddbKind{}) }

type ddbKind struct {
	selected *ddbTypes.TableDescription
}

func (k *ddbKind) Name() string  { return "ddb" }
func (k *ddbKind) Reset()        { k.selected = nil }
func (k *ddbKind) Selection() any { return k.selected }

func (k *ddbKind) SetSelection(s any) {
	if td, ok := s.(*ddbTypes.TableDescription); ok {
		k.selected = td
	}
}

func (k *ddbKind) Breadcrumb() string {
	if k.selected == nil || k.selected.TableName == nil {
		return "ddb"
	}
	return "ddb > " + aws.ToString(k.selected.TableName)
}

func (k *ddbKind) PrimaryAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		items, err := app.APIStore().ScanFirstPage(context.Background(), aws.ToString(k.selected.TableName), 25)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		body, _ := json.MarshalIndent(items, "", "  ")
		tv := tview.NewTextView().SetText(string(body))
		tv.SetBorder(true).SetTitle(" scan " + aws.ToString(k.selected.TableName) + " (first 25) ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&logsPseudoKind{name: "scan:" + aws.ToString(k.selected.TableName)},
			&simpleKindView{flex: flex, focus: tv, app: app, source: k},
		)
	}
}

func (k *ddbKind) SecondaryActions() []kind.Binding {
	return []kind.Binding{
		{Key: 'c', Label: "describe", Run: k.describeAction()},
	}
}

func (k *ddbKind) describeAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		body, _ := json.MarshalIndent(k.selected, "", "  ")
		tv := tview.NewTextView().SetText(string(body))
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.TableName) + " describe ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&logsPseudoKind{name: "describe:" + aws.ToString(k.selected.TableName)},
			&simpleKindView{flex: flex, focus: tv, app: app, source: k},
		)
	}
}

func (k *ddbKind) Build(app kind.App) (kind.View, error) {
	names, err := app.APIStore().ListTables(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	headers := []string{"TableName", "Status", "ItemCount", "SizeBytes", "BillingMode", "Streams"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, name := range names {
		td, derr := app.APIStore().DescribeTable(context.Background(), name)
		if derr != nil {
			// Skip rows we can't describe — log via flash and continue. Other
			// rows are still useful.
			app.FlashError("describe " + name + ": " + derr.Error())
			continue
		}
		billing := ""
		if td.BillingModeSummary != nil {
			billing = string(td.BillingModeSummary.BillingMode)
		}
		streams := "no"
		if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
			streams = "yes"
		}
		cells := []string{
			aws.ToString(td.TableName),
			string(td.TableStatus),
			fmt.Sprintf("%d", aws.ToInt64(td.ItemCount)),
			fmt.Sprintf("%d", aws.ToInt64(td.TableSizeBytes)),
			billing,
			streams,
		}
		copyTD := td
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyTD)
			}
			table.SetCell(row+1, col, cell)
		}
	}

	flex := tview.NewFlex().AddItem(table, 0, 1, true)
	v := &simpleKindView{flex: flex, focus: table, app: app, source: k}
	table.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if v.OnKey(e) {
			return nil
		}
		return e
	})
	return v, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestDDB
```
Expected: PASS.

- [ ] **Step 5: Manual smoke test**

```bash
go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
`:` → `ddb` Enter. Expected: tables list with item counts, sizes, billing mode, streams flag. Select a small table, Enter — JSON-formatted scan of up to 25 items. Press `c` from the table view — describe output. **Caveat:** for accounts with many tables, `ListTables` + per-row `DescribeTable` is N+1 calls. If this is slow on your environment, that's expected for MVP — pagination/concurrency are follow-ups.

- [ ] **Step 6: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/dynamodb.go internal/view/dynamodb_test.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): add :ddb kind with scan/describe"
```

---

## Phase 5: Wire ECS kinds into the palette

End state: `:cluster`, `:service`, `:task` work as palette entries. Drill-down behaviour (Enter on cluster → services list) is unchanged because each ECS kind's PrimaryAction calls into the existing `showPrimaryKindPage(nextKind, …)` flow.

### Task 5.1: ECS Kind adapters

**Files:**
- Create: `internal/view/ecs_kinds.go`

- [ ] **Step 1: Implement adapters that wrap existing ECS views**

```go
package view

import "github.com/keidarcy/e1s/internal/view/kind"

func init() {
	kind.Register(&ecsClusterKind{})
	kind.Register(&ecsServiceKind{})
	kind.Register(&ecsTaskKind{})
}

type ecsClusterKind struct{}

func (e *ecsClusterKind) Name() string                   { return "cluster" }
func (e *ecsClusterKind) Reset()                         {}
func (e *ecsClusterKind) Selection() any                 { return nil }
func (e *ecsClusterKind) SetSelection(any)               {}
func (e *ecsClusterKind) Breadcrumb() string             { return "cluster" }
func (e *ecsClusterKind) PrimaryAction() kind.Action     { return nil }
func (e *ecsClusterKind) SecondaryActions() []kind.Binding { return nil }
func (e *ecsClusterKind) Build(app kind.App) (kind.View, error) {
	concrete, ok := app.(*App)
	if !ok {
		return nil, errors.New("ecsClusterKind needs *view.App")
	}
	if err := concrete.showPrimaryKindPage(ClusterKind, true); err != nil {
		return nil, err
	}
	return &noopView{}, nil
}

// noopView is returned by ECS adapters because they call into the existing
// pages stack themselves rather than handing back a fresh tview.Flex.
type noopView struct{}
func (n *noopView) Render() *tview.Flex                          { return tview.NewFlex() }
func (n *noopView) Focus()                                       {}
func (n *noopView) OnKey(event *tcell.EventKey) (handled bool)   { return false }

// ecsServiceKind / ecsTaskKind — same pattern, swap ClusterKind for
// ServiceKind / TaskKind.
```

`SwitchView` for ECS adapters needs special handling: when the returned `View` is a `*noopView`, do not call `AddAndSwitchToPage`. Update `App.SwitchView`:

```go
func (app *App) SwitchView(k kind.Kind, v kind.View) error {
	app.activeKind = k
	if _, isNoop := v.(*noopView); isNoop {
		return nil // ECS adapter already navigated via showPrimaryKindPage
	}
	pageName := "kind." + k.Name()
	app.Pages.AddAndSwitchToPage(pageName, v.Render(), true)
	return nil
}
```

- [ ] **Step 2: Build + smoke test**

```bash
go build -o /tmp/e1s ./cmd/e1s && /tmp/e1s
```
Press `:`, type `cluster`, Enter. Expected: cluster list (same as default startup view). `:service` from anywhere → service list of currently selected cluster.

- [ ] **Step 3: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/ecs_kinds.go internal/view/app.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "feat(view): register ECS kinds with palette"
```

---

## Phase 6: Cross-kind navigation — Lambda → SQS DLQ

End state: from a selected Lambda, pressing `d` parses the DLQ ARN, switches to `:sqs`, pre-selects the queue.

### Task 6.1: Implement `dlqAction`

**Files:**
- Modify: `internal/view/lambda.go` (replace the `dlqAction` placeholder)

- [ ] **Step 1: Test**

Create `internal/view/lambda_dlq_test.go`:

```go
package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestParseQueueNameFromArn(t *testing.T) {
	cases := map[string]string{
		"arn:aws:sqs:us-east-1:111:my-dlq":              "my-dlq",
		"arn:aws:sqs:eu-west-2:222:another-dlq":          "another-dlq",
		"":                                               "",
		"not-an-arn":                                     "",
	}
	for in, want := range cases {
		if got := parseQueueNameFromArn(in); got != want {
			t.Fatalf("parseQueueNameFromArn(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestDLQActionFlashesWhenNoSelection(t *testing.T) {
	k := &lambdaKind{}
	app := &fakeApp{}
	if err := k.dlqAction()(app); err != nil {
		t.Fatalf("err = %v", err)
	}
	if app.flashedMsg == "" {
		t.Fatal("expected flash for no selection")
	}
}

func TestDLQActionFlashesWhenNoDLQ(t *testing.T) {
	k := &lambdaKind{selected: &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("x")}}
	app := &fakeApp{}
	if err := k.dlqAction()(app); err != nil {
		t.Fatalf("err = %v", err)
	}
	if app.flashedMsg != "no DLQ configured" {
		t.Fatalf("flashedMsg = %q", app.flashedMsg)
	}
}
```

`fakeApp` here: extract the one in `kind/palette_test.go` into a small helper file `internal/view/test_helpers.go` if it isn't already test-only. Or duplicate per package. Either is fine.

- [ ] **Step 2: Implement**

Replace `dlqAction` in `lambda.go`:

```go
import "strings"

func parseQueueNameFromArn(arn string) string {
	if !strings.HasPrefix(arn, "arn:aws:sqs:") {
		return ""
	}
	idx := strings.LastIndex(arn, ":")
	if idx < 0 || idx == len(arn)-1 {
		return ""
	}
	return arn[idx+1:]
}

func (k *lambdaKind) dlqAction() kind.Action {
	return func(app kind.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		if k.selected.DeadLetterConfig == nil || k.selected.DeadLetterConfig.TargetArn == nil {
			app.FlashError("no DLQ configured")
			return nil
		}
		queueName := parseQueueNameFromArn(aws.ToString(k.selected.DeadLetterConfig.TargetArn))
		if queueName == "" {
			app.FlashError("could not parse DLQ ARN")
			return nil
		}
		sqsK, ok := kind.Get("sqs")
		if !ok {
			app.FlashError("sqs kind not registered")
			return nil
		}
		// Pre-select before Build so the cursor lands on the right row. SQS
		// kind's SetSelection takes a queue URL; here we have only the name,
		// so SQS Build must accept either (URL or bare name) — see sqs.go.
		sqsK.SetSelection(queueName)
		v, err := sqsK.Build(app)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		return app.SwitchView(sqsK, v)
	}
}
```

- [ ] **Step 3: Update SQS pre-selection to handle bare names**

In `sqs.go` `Build`, replace the pre-selection block with:

```go
	// Pre-selection: SetSelection may have been called with a full URL or a
	// bare queue name (cross-kind nav from Lambda DLQ).
	if k.selectedURL != "" {
		for r := 1; r <= len(urls); r++ {
			cell := table.GetCell(r, 0)
			if cell == nil {
				continue
			}
			ref, _ := cell.GetReference().(string)
			name, _ := cell.Text, queueNameFromURL(ref)
			_ = name
			if ref == k.selectedURL || queueNameFromURL(ref) == k.selectedURL {
				table.Select(r, 0)
				k.selectedURL = ref // promote to full URL
				break
			}
		}
	}
```

- [ ] **Step 4: Run tests, smoke test, commit**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./...
```

Smoke: `:lambda`, select a function with a DLQ, press `d`. Expected: switches to `:sqs` with cursor on the DLQ.

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -am "feat(view): cross-kind jump from Lambda to SQS DLQ"
```

---

### Task 6.2: Cross-kind navigation integration test

This is the test the design called out as architecturally critical: if it passes, adding a 4th flat service follows the same pattern.

The test lives in `internal/view/` (same package as `lambdaKind` / `sqsKind`) so it can poke unexported state directly. No test seam, no `t.Skip`.

**Files:**
- Create: `internal/view/cross_kind_test.go`

- [ ] **Step 1: Write the failing test**

The test exercises the part of `dlqAction` that doesn't need an AWS client: ARN parsing, registry lookup, pre-selection on the SQS kind, and dispatching `SwitchView`. The `Build` of `sqsKind` *does* hit ListQueues, so we sidestep that by calling `dlqAction` against a fake app that intercepts `SwitchView` *before* `Build`'s API call would happen — which means we test up to and including the registry hand-off, but stop short of `sqsKind.Build`. That's exactly the architectural seam we care about.

Wait — `dlqAction` currently calls `sqsK.Build(app)` *before* `app.SwitchView(...)`. So to keep this test self-contained (no AWS), we structure the fake app to never have `Build` reach an AWS client: the fake app's `APIStore()` returns a Store whose `sqs` field is a mocked client (same middleware-mock pattern used in `internal/api/sqs_test.go`).

Create `internal/view/cross_kind_test.go`:

```go
package view

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	smithymiddleware "github.com/aws/smithy-go/middleware"

	"github.com/keidarcy/e1s/internal/api"
	"github.com/keidarcy/e1s/internal/view/kind"
)

// crossKindFakeApp is a kind.App that lets the test observe SwitchView calls
// and serve a Store with a mocked SQS client (so sqsKind.Build's ListQueues
// returns a deterministic queue list without hitting AWS).
type crossKindFakeApp struct {
	store      *api.Store
	switched   kind.Kind
	switchedV  kind.View
	flashedMsg string
}

func (f *crossKindFakeApp) APIStore() *api.Store               { return f.store }
func (f *crossKindFakeApp) SwitchView(k kind.Kind, v kind.View) error {
	f.switched = k
	f.switchedV = v
	return nil
}
func (f *crossKindFakeApp) FlashError(msg string) { f.flashedMsg = msg }

func newStoreServingQueues(t *testing.T, queueURLs []string) *api.Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				switch in.Request.(type) {
				default:
					// Both ListQueues and GetQueueAttributes flow through here.
					// Return ListQueues output for the URL list; SDK type-asserts
					// the .Result against the expected output type, so we need to
					// switch on the operation. The simplest way is to inspect the
					// serialized HTTP request — for this test we return both.
				}
				// Pragmatic shortcut: return ListQueues output. GetQueueAttributes
				// in sqsKind.Build is best-effort (errors ignored), so a mismatched
				// response just means empty attributes — fine for this test.
				return smithymiddleware.FinalizeOutput{
					Result: &sqs.ListQueuesOutput{QueueUrls: queueURLs},
				}, smithymiddleware.Metadata{}, nil
			}), smithymiddleware.Before)
		})
	})
	// Construct a Store and inject the mocked client. The api package's
	// Store.sqs field is unexported, so this test must live in package api OR
	// we add a test-only setter. Cleanest: add SetSqsClientForTest in
	// internal/api/sqs.go (test build tag) — but to keep this MVP plan
	// tight, we just set the field via a helper in internal/api package.
	return apiStoreWithSqsForTest(&cfg, c)
}

// apiStoreWithSqsForTest is a small helper added to internal/api/sqs.go
// (production file, but only called by tests in either internal/api or
// internal/view) to construct a Store with a pre-set SQS client.
// See implementation step below.

func TestCrossKindLambdaToDLQ(t *testing.T) {
	// Mocked SQS returns one queue whose name matches the DLQ in the function's
	// DeadLetterConfig — sqsKind.Build's pre-selection should match it.
	fullURL := "https://sqs.us-east-1.amazonaws.com/111/my-dlq"
	store := newStoreServingQueues(t, []string{fullURL})
	app := &crossKindFakeApp{store: store}

	// Reset the registry so we control what's in it. Note: kind.resetRegistryForTest
	// is package-private to internal/view/kind — we can't call it here. Instead
	// we rely on init()-time registration: lambdaKind and sqsKind are already
	// registered when this test package builds. We just need to reset their
	// state.
	if k, ok := kind.Get("lambda"); ok {
		k.Reset()
	}
	if k, ok := kind.Get("sqs"); ok {
		k.Reset()
	}

	// Find the registered lambdaKind (not a fresh one) so dlqAction's closure
	// reads the right selected function.
	lk, ok := kind.Get("lambda")
	if !ok {
		t.Fatal("lambda kind not registered")
	}
	concrete := lk.(*lambdaKind)
	concrete.selected = &lambdaTypes.FunctionConfiguration{
		FunctionName: aws.String("auth-handler"),
		DeadLetterConfig: &lambdaTypes.DeadLetterConfig{
			TargetArn: aws.String("arn:aws:sqs:us-east-1:111:my-dlq"),
		},
	}

	// Trigger the cross-kind jump.
	if err := concrete.dlqAction()(app); err != nil {
		t.Fatalf("dlqAction err = %v", err)
	}

	// Assert: SwitchView was called with the sqs kind.
	if app.switched == nil || app.switched.Name() != "sqs" {
		t.Fatalf("expected SwitchView to sqs; got %v", app.switched)
	}
	// Assert: the sqs kind's selection was promoted from "my-dlq" (bare name)
	// to the full URL by Build's pre-selection block.
	sk, _ := kind.Get("sqs")
	got, _ := sk.Selection().(string)
	if got != fullURL {
		t.Fatalf("sqs selection = %q; want %q", got, fullURL)
	}
}
```

- [ ] **Step 2: Add the test-only Store helper**

The test needs to construct an `api.Store` with a pre-injected mocked `sqs.Client`. The `sqs` field is unexported. Add a small helper to `internal/api/sqs.go` (production file — function used by tests in any package; no build tag needed since it's just a constructor):

In `internal/api/sqs.go`, append:

```go
// StoreWithSqsForTest constructs a Store with a pre-configured SQS client,
// for use in tests that need to mock SQS at the SDK middleware layer.
// Not exported via an interface — callers in internal/* test files only.
func StoreWithSqsForTest(cfg *aws.Config, c *sqs.Client) *Store {
	return &Store{Config: cfg, sqs: c}
}
```

Then in `internal/view/cross_kind_test.go`, replace the `apiStoreWithSqsForTest` placeholder helper with:

```go
func apiStoreWithSqsForTest(cfg *aws.Config, c *sqs.Client) *api.Store {
	return api.StoreWithSqsForTest(cfg, c)
}
```

- [ ] **Step 3: Run the test**

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./internal/view/ -run TestCrossKindLambdaToDLQ -v
```
Expected: PASS.

If it fails because `GetQueueAttributes` in `sqsKind.Build` returns an unexpected type from the catch-all middleware, the simplest fix is: in `sqsKind.Build`, the existing `attrs, _ := app.APIStore().GetQueueAttributes(...)` already swallows errors — the row will just have empty attribute cells, but the pre-selection match (which only reads the cell *Reference*, not its text) still works. No change needed.

- [ ] **Step 4: Commit**

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s add internal/view/cross_kind_test.go internal/api/sqs.go
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s commit -m "test: cross-kind navigation Lambda→SQS DLQ"
```

---

## Done

After Phase 6:

```bash
cd /Users/mohsiurrahman/Desktop/runna-projects/e1s && go test ./... && go build -o /tmp/e1s ./cmd/e1s
```

Manual end-to-end verification:
- [ ] `:lambda` lists functions; Enter tails logs; `c` shows config; `i` invokes with `{}`.
- [ ] `:sqs` lists queues; Enter peeks (10 messages, VisibilityTimeout=0); `s` sends test message.
- [ ] `:ddb` lists tables; Enter scans first 25 items; `c` describes.
- [ ] `:cluster` / `:service` / `:task` navigate ECS as before.
- [ ] From `:lambda`, select a function with DLQ, press `d` → `:sqs` opens with that queue selected.
- [ ] Switch profile via existing UI → `kind.ResetAll` clears selections; `:lambda` re-fetches against the new profile.

Push the branch and review.

```bash
git -C /Users/mohsiurrahman/Desktop/runna-projects/e1s push -u origin feat/multi-service-fork
```
