package view

import (
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// showPalette opens a single-line modal input. On Enter, the typed name is
// passed to the kind palette dispatcher. Escape cancels.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kindpkg.NewPalette(app)
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
