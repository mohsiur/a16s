package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// reserved palette verbs that don't map to a Kind.
var paletteExitVerbs = map[string]struct{}{
	"exit": {},
	"quit": {},
	"q":    {},
}

// showPalette mounts a one-line input as an extra row at the bottom of
// mainScreen. The current page (cluster table or flat-kind table) stays
// visible behind it — k9s-style. On Enter the typed name is dispatched
// through the palette. Escape cancels. Either path removes the input row
// and restores focus.
//
// Tab cycles through registered kind names (canonical + aliases) that share
// the typed prefix — e.g. `:c<Tab>` cycles cluster, container.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kindpkg.NewPalette(app)
	}

	input := tview.NewInputField().
		SetLabel(":").
		SetFieldWidth(0)

	// k9s-style prefix completion. Returning the full set of matches makes
	// tview render a popup; Tab moves through it. Empty input -> no popup
	// (returning nil from the autocomplete func suppresses it).
	input.SetAutocompleteFunc(func(currentText string) []string {
		prefix := strings.ToLower(strings.TrimSpace(currentText))
		if prefix == "" {
			return nil
		}
		var matches []string
		for _, n := range kindpkg.Names() {
			if strings.HasPrefix(n, prefix) {
				matches = append(matches, n)
			}
		}
		return matches
	})

	input.SetDoneFunc(func(key tcell.Key) {
		name := strings.TrimSpace(input.GetText())
		app.mainScreen.RemoveItem(input)
		app.SetFocus(app.Pages)
		if key != tcell.KeyEnter {
			return
		}
		if _, isExit := paletteExitVerbs[name]; isExit {
			app.Stop()
			return
		}
		app.palette.Submit(name)
	})

	app.mainScreen.AddItem(input, 1, 0, true)
	app.SetFocus(input)
}
