// Package kind provides a registry for AWS resource kinds rendered in the
// a16s TUI. Each Kind is registered once at init() time and dispatched via
// the `:` palette. The package is the single source of truth for how flat
// (non-hierarchical) services slot into the app.
package kind

import (
	"sort"

	"github.com/mohsiur/a16s/internal/api"
	"github.com/rivo/tview"
)

// App is the minimal surface a Kind needs from the host application. Declared
// here (not imported from `view`) to avoid a circular import: `view` imports
// `kind`; `kind` must not import `view`. The concrete *view.App will satisfy
// this interface.
type App interface {
	AWSClients() *api.Clients
	FlashError(msg string)
	// QueueUpdateDraw queues f on the tview event loop and forces a redraw
	// after it returns. Used by background loaders that need to update UI.
	QueueUpdateDraw(f func()) *tview.Application
	// SetFocus moves keyboard focus to p. Used by background loaders when
	// the placeholder primitive that originally held focus is removed.
	SetFocus(p tview.Primitive) *tview.Application
}

// Kind is the interface every browseable resource implements. The kind layer
// owns the inventory cache (Reset / Selection / SetSelection); rendering and
// keybindings are wired in the legacy `view` package.
type Kind interface {
	Name() string
	Reset()
	Selection() any
	SetSelection(any)
}

// Aliaser is an optional companion interface a Kind can implement to expose
// extra names that resolve to the same Kind via Get(). For example, ddb
// returns []string{"dynamodb"}. Aliases are also surfaced by Names() so the
// palette autocomplete cycles through them.
type Aliaser interface {
	Aliases() []string
}

// Preloader is an optional companion interface a Kind can implement to fetch
// inventory in the background at app start. PreloadAll runs each Preload in
// its own goroutine.
type Preloader interface {
	Preload(app App)
}

// PreloadAll fans out Preload across every registered Kind that opts in.
// Each Preload runs in its own goroutine; this function does not wait. Call
// from app startup right after AWS config resolves.
func PreloadAll(app App) {
	for _, k := range registry {
		if pl, ok := k.(Preloader); ok {
			go pl.Preload(app)
		}
	}
}

// registry maps every name (canonical Name() and Aliases()) to its Kind.
// aliasOwner records the canonical name for a given alias so All()
// deduplicates and Names() can include aliases without double-counting.
var registry = map[string]Kind{}
var aliasOwner = map[string]string{}

// Register adds a Kind under its Name() and any Aliases(). Panics on
// duplicate registration — duplicates are a programming error caught at
// startup.
func Register(k Kind) {
	name := k.Name()
	if _, exists := registry[name]; exists {
		panic("kind already registered: " + name)
	}
	registry[name] = k
	aliasOwner[name] = name
	if a, ok := k.(Aliaser); ok {
		for _, alias := range a.Aliases() {
			if _, exists := registry[alias]; exists {
				panic("kind alias collides: " + alias)
			}
			registry[alias] = k
			aliasOwner[alias] = name
		}
	}
}

// Get returns the Kind registered under name (canonical or alias).
func Get(name string) (Kind, bool) {
	k, ok := registry[name]
	return k, ok
}

// All returns every registered Kind exactly once (deduped by canonical
// Name()), sorted alphabetically. Aliases do not produce extra entries.
func All() []Kind {
	seen := map[string]struct{}{}
	out := make([]Kind, 0, len(registry))
	for n, k := range registry {
		canonical := aliasOwner[n]
		if _, dup := seen[canonical]; dup {
			continue
		}
		if n != canonical {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names returns every name the palette accepts — canonical names plus
// aliases — sorted alphabetically. Used by the palette's autocomplete.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ResetAll calls Reset() on every registered Kind. Called from
// api.Clients.SwitchAwsConfig so per-kind selection state doesn't leak across
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
	aliasOwner = map[string]string{}
}
