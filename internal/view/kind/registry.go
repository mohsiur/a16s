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
	// Back removes the current front kind page (if any) so the previously
	// visible page is re-shown. Used by Esc on a flat-kind view.
	Back()
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

	// Build constructs the kind's view. May be called multiple times across
	// a session (every `:` invocation rebuilds). Implementations should be
	// cheap to call or cache internally.
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
