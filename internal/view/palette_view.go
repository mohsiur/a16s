package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// reserved palette verbs that don't map to a Kind.
var paletteExitVerbs = map[string]struct{}{
	"exit": {},
	"quit": {},
	"q":    {},
}

// showPalette swaps the persistent top bar from its context label into a `:`
// InputField. The current page stays visible behind it — k9s-style. On Enter
// the typed name is dispatched through the palette; Escape cancels. Either
// path swaps the bar back to the label and restores focus.
//
// Tab cycles through registered kind names (canonical + aliases) that share
// the typed prefix — e.g. `:c<Tab>` cycles cluster, container.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kindpkg.NewPalette(app)
	}

	autocomplete := func(currentText string) []string {
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
	}

	input := app.topBar.EnterPalette(autocomplete, func(name string, key tcell.Key) {
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
	app.SetFocus(input)
}
