package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// topBarWidget is the persistent k9s-style chrome row above Pages. In its
// default state it renders a one-line context summary
// (`profile · region · breadcrumb`). When the user presses `:`, the same row
// switches to an InputField wired to autocomplete + Enter/Esc handlers; on
// dismiss the row swaps back to the context label.
//
// The widget owns a single flex container so the parent (mainScreen) only
// reserves one row regardless of mode. Callers must run all mutations on the
// tview event loop — this struct is not goroutine-safe.
type topBarWidget struct {
	flex    *tview.Flex
	label   *tview.TextView
	input   *tview.InputField
	context string
}

func newTopBarWidget() *topBarWidget {
	t := &topBarWidget{
		flex:  tview.NewFlex(),
		label: tview.NewTextView().SetDynamicColors(true),
	}
	t.flex.AddItem(t.label, 0, 1, false)
	return t
}

// SetContext repaints the default label. Inputs use the same separator
// convention k9s uses; an empty crumb collapses gracefully so we don't render
// trailing punctuation.
func (t *topBarWidget) SetContext(profile, region, breadcrumb string) {
	parts := []string{}
	if profile != "" {
		parts = append(parts, "[yellow]"+profile+"[-]")
	}
	if region != "" {
		parts = append(parts, "[aqua]"+region+"[-]")
	}
	if breadcrumb != "" {
		parts = append(parts, breadcrumb)
	}
	t.context = strings.Join(parts, " · ")
	t.label.SetText(" " + t.context)
}

// EnterPalette swaps the label out for an InputField with a `:` label. The
// caller supplies the autocomplete + submit handlers; we wire Enter/Esc to
// call onSubmit (with the trimmed text and the terminating key) and then
// always restore the label. Returns the input so the caller can SetFocus on
// it after the swap.
func (t *topBarWidget) EnterPalette(autocomplete func(string) []string, onSubmit func(text string, key tcell.Key)) *tview.InputField {
	if t.input != nil {
		return t.input
	}
	in := tview.NewInputField().SetLabel(":").SetFieldWidth(0)
	if autocomplete != nil {
		in.SetAutocompleteFunc(autocomplete)
	}
	in.SetDoneFunc(func(key tcell.Key) {
		text := strings.TrimSpace(in.GetText())
		t.exitPalette()
		onSubmit(text, key)
	})
	t.flex.Clear()
	t.flex.AddItem(in, 0, 1, true)
	t.input = in
	return in
}

// exitPalette restores the label. Idempotent — safe to call after a manual
// dismiss path (Esc without a focus swap, etc).
func (t *topBarWidget) exitPalette() {
	if t.input == nil {
		return
	}
	t.flex.Clear()
	t.flex.AddItem(t.label, 0, 1, false)
	t.input = nil
}
