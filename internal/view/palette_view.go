package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mohsiur/a16s/internal/color"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// reserved palette verbs that don't map to a Kind.
var paletteExitVerbs = map[string]struct{}{
	"exit": {},
	"quit": {},
	"q":    {},
}

// paletteLegacyKinds maps palette verbs to legacy `kind` enum values whose
// pages now go through showPrimaryKindPage (full ECS chrome). Verbs not
// listed here fall through to the kindpkg Palette flow, preserving any
// auxiliary kinds we haven't migrated yet.
var paletteLegacyKinds = map[string]kind{
	"clusters": ClusterKind,
	"cluster":  ClusterKind,
	"lambda":   LambdaKind,
	"lambdas":  LambdaKind,
	"sqs":      SQSKind,
	"queues":   SQSKind,
	"ddb":      DynamoDBKind,
	"dynamodb": DynamoDBKind,
	"tables":   DynamoDBKind,
}

// showPalette mounts a `:` InputField as a 1-row item at the top of mainScreen
// (above Pages, below where a top bar would be). Enter dispatches the typed
// name through the palette; Esc cancels. Both paths remove the input row and
// restore focus to Pages. Tab cycles through registered Kind names that share
// the typed prefix.
func (app *App) showPalette() {
	if app.palette == nil {
		app.palette = kindpkg.NewPalette(app)
	}
	if app.paletteInput != nil {
		app.SetFocus(app.paletteInput)
		return
	}

	input := tview.NewInputField().
		SetLabel("[gray]:[-] ").
		SetLabelColor(color.Color(theme.Cyan)).
		SetFieldWidth(0)
	input.SetBackgroundColor(color.Color(theme.BgColor))
	input.SetFieldBackgroundColor(color.Color(theme.BgColor))
	input.SetFieldTextColor(color.Color(theme.FgColor))
	input.SetBorder(true).SetBorderColor(color.Color(theme.Blue))
	input.SetBorderPadding(0, 0, 1, 0)

	input.SetAutocompleteFunc(func(currentText string) []string {
		prefix := strings.ToLower(strings.TrimSpace(currentText))
		if prefix == "" {
			return nil
		}
		seen := map[string]struct{}{}
		var matches []string
		for n := range paletteLegacyKinds {
			if strings.HasPrefix(n, prefix) {
				if _, dup := seen[n]; !dup {
					seen[n] = struct{}{}
					matches = append(matches, n)
				}
			}
		}
		for _, n := range kindpkg.Names() {
			if strings.HasPrefix(n, prefix) {
				if _, dup := seen[n]; !dup {
					seen[n] = struct{}{}
					matches = append(matches, n)
				}
			}
		}
		return matches
	})

	input.SetDoneFunc(func(key tcell.Key) {
		text := strings.TrimSpace(input.GetText())
		app.dismissPalette()
		if key != tcell.KeyEnter {
			return
		}
		if _, isExit := paletteExitVerbs[text]; isExit {
			app.Stop()
			return
		}
		if k, ok := paletteLegacyKinds[strings.ToLower(text)]; ok {
			if err := app.showPrimaryKindPage(k, false); err != nil {
				app.Notice.Warn(err.Error())
			}
			return
		}
		app.palette.Submit(text)
	})

	// Mount as the top item in mainScreen. Use AddItem at index 0 by clearing
	// and re-adding — tview.Flex doesn't expose insert.
	app.paletteInput = input
	app.rebuildMainScreen()
	app.SetFocus(input)
}

// dismissPalette removes the `:` input row and refocuses Pages.
func (app *App) dismissPalette() {
	if app.paletteInput == nil {
		return
	}
	app.paletteInput = nil
	app.rebuildMainScreen()
	app.SetFocus(app.Pages)
}

// rebuildMainScreen re-lays mainScreen's children: optional palette input on
// top, then Pages, then the footer. Called whenever the palette mounts or
// dismisses; cheap because tview just re-attaches the same primitives.
func (app *App) rebuildMainScreen() {
	app.mainScreen.Clear()
	app.mainScreen.SetDirection(tview.FlexRow)
	if app.paletteInput != nil {
		app.mainScreen.AddItem(app.paletteInput, 3, 0, true)
	}
	app.mainScreen.AddItem(app.Pages, 0, 2, true)
	app.mainScreen.AddItem(app.mainScreenFooter, 1, 1, false)
}
