package view

import (
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// pseudoKind is a transient, non-registered kind used as the "host" of an
// auxiliary screen launched by an action (log tail, invoke result, config
// dump, etc). It implements kind.Kind with no-op behaviour so the page can
// be tracked under a unique pseudo-name without polluting the registry.
type pseudoKind struct{ name string }

func (p *pseudoKind) Name() string                            { return p.name }
func (p *pseudoKind) Build(kindpkg.App) (kindpkg.View, error) { return nil, nil }
func (p *pseudoKind) Reset()                                  {}
func (p *pseudoKind) Selection() any                          { return nil }
func (p *pseudoKind) SetSelection(any)                        {}
func (p *pseudoKind) Breadcrumb() string                      { return p.name }
func (p *pseudoKind) PrimaryAction() kindpkg.Action           { return nil }
func (p *pseudoKind) SecondaryActions() []kindpkg.Binding     { return nil }

// joinLines concatenates a slice of strings into one, appending a newline
// after each entry. Used by log-tail action helpers.
func joinLines(in []string) string {
	out := ""
	for _, s := range in {
		out += s + "\n"
	}
	return out
}

// simpleKindView is the default kind.View for table-based flat kinds.
type simpleKindView struct {
	flex   *tview.Flex
	app    kindpkg.App
	source kindpkg.Kind
}

func (s *simpleKindView) Render() *tview.Flex { return s.flex }
func (s *simpleKindView) Focus()              { /* tview handles focus via SetFocus */ }
func (s *simpleKindView) OnKey(event *tcell.EventKey) (handled bool) {
	if event.Key() == tcell.KeyEscape {
		if s.app != nil {
			s.app.Back()
			return true
		}
	}
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

// newTableKindView builds a simpleKindView around an already-populated tview
// Table. It wires:
//   - selectable mode (defensive — table.go already does this for legacy ECS)
//   - SetFixed(1, 0) so the header row stays visible while scrolling
//   - SelectionChangedFunc that pushes the row's column-0 reference back to
//     the source Kind via SetSelection (this is how flat kinds learn about
//     the user's cursor without going through the legacy ECS view struct)
//   - SetInputCapture that delegates to the simpleKindView's OnKey, which in
//     turn calls the kind's PrimaryAction / SecondaryActions / Esc -> Back.
//
// This is the single place flat kinds get their selection + key wiring; SQS
// and DynamoDB phases pick it up for free.
func newTableKindView(app kindpkg.App, source kindpkg.Kind, table *tview.Table) *simpleKindView {
	table.SetSelectable(true, false)
	table.SetFixed(1, 0)

	flex := tview.NewFlex().AddItem(table, 0, 1, true)
	view := &simpleKindView{flex: flex, app: app, source: source}

	table.SetSelectionChangedFunc(func(row, _ int) {
		if row <= 0 || source == nil {
			return
		}
		cell := table.GetCell(row, 0)
		if cell == nil {
			return
		}
		source.SetSelection(cell.GetReference())
	})
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if view.OnKey(event) {
			return nil
		}
		return event
	})
	return view
}
