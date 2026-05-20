package view

import (
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// showPalette mounts a one-line input as an extra row at the bottom of
// mainScreen. The current page (cluster table or flat-kind table) stays
// visible behind it — k9s-style. On Enter the typed name is dispatched
// through the palette. Escape cancels. Either path removes the input row
// and restores focus.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kindpkg.NewPalette(app)
	}

	input := tview.NewInputField().
		SetLabel(":").
		SetFieldWidth(0)

	input.SetDoneFunc(func(key tcell.Key) {
		name := input.GetText()
		app.mainScreen.RemoveItem(input)
		app.SetFocus(app.Pages)
		if key == tcell.KeyEnter {
			app.palette.Submit(name)
		}
	})

	app.mainScreen.AddItem(input, 1, 0, true)
	app.SetFocus(input)
}
