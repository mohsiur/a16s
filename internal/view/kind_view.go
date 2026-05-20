package view

import (
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// newLoadingTableKindView shows a centered "Loading <name>…" placeholder,
// then in a goroutine calls `load`. On success, `buildTable` runs on the
// tview event loop and its result populates the same root Flex via
// populateTableKindView. On error, the placeholder is replaced with an
// error message. Esc on the placeholder calls app.Back().
//
// `load` runs OFF the event loop and must not touch tview state.
// `buildTable` runs ON the event loop and may freely build/mutate widgets.
func newLoadingTableKindView(app kindpkg.App, source kindpkg.Kind, load func() error, buildTable func() *tview.Table) *simpleKindView {
	root := tview.NewFlex().SetDirection(tview.FlexRow)
	loading := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("\nLoading " + source.Name() + "…\n")
	root.AddItem(loading, 0, 1, true)

	view := &simpleKindView{flex: root, app: app, source: source}
	root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if view.OnKey(event) {
			return nil
		}
		return event
	})

	go func() {
		err := load()
		app.QueueUpdateDraw(func() {
			if err != nil {
				root.Clear()
				errView := tview.NewTextView().
					SetTextAlign(tview.AlignCenter).
					SetText("\nFailed to load " + source.Name() + ":\n" + err.Error())
				root.AddItem(errView, 0, 1, true)
				return
			}
			populateTableKindView(root, app, source, buildTable())
		})
	}()

	return view
}

// newTextSubView wraps a tview.Primitive (typically a TextView, optionally
// inside its own Flex) into a simpleKindView whose Esc handler calls
// app.Back(). Sub-views (logs, peek, scan, config, invoke result) need this
// because the inner TextView's default input capture would otherwise swallow
// Esc before simpleKindView's OnKey ever sees it.
func newTextSubView(app kindpkg.App, body tview.Primitive) *simpleKindView {
	flex, isFlex := body.(*tview.Flex)
	if !isFlex {
		flex = tview.NewFlex().AddItem(body, 0, 1, true)
	}
	view := &simpleKindView{flex: flex, app: app}
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if view.OnKey(event) {
			return nil
		}
		return event
	})
	return view
}

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
// Table. See populateTableKindView for the wiring contract — this is the
// from-scratch entrypoint used by synchronous Build paths.
func newTableKindView(app kindpkg.App, source kindpkg.Kind, table *tview.Table) *simpleKindView {
	root := tview.NewFlex().SetDirection(tview.FlexRow)
	return populateTableKindView(root, app, source, table)
}

// populateTableKindView clears `root` and populates it with the standard
// flat-kind layout: optional Informer header (8 rows fixed) + the populated
// table (flex 1). Wires:
//   - selectable mode (defensive — table.go already does this for legacy ECS)
//   - SetFixed(1, 0) so the header row stays visible while scrolling
//   - SelectionChangedFunc that pushes the row's column-0 reference back to
//     the source Kind via SetSelection (this is how flat kinds learn about
//     the user's cursor without going through the legacy ECS view struct)
//   - SetInputCapture on both root and table that delegates to the
//     simpleKindView's OnKey (PrimaryAction / SecondaryActions / Esc->Back).
//
// Async Build paths reuse this to swap a loading placeholder root for the
// real table without rebuilding the parent Flex.
func populateTableKindView(root *tview.Flex, app kindpkg.App, source kindpkg.Kind, table *tview.Table) *simpleKindView {
	table.SetSelectable(true, false)
	table.SetFixed(1, 0)

	root.Clear()
	root.SetDirection(tview.FlexRow)

	informer, hasInfo := source.(kindpkg.Informer)
	var detailView *tview.TextView
	if hasInfo {
		aggView := tview.NewTextView().SetDynamicColors(true).SetText(informer.AggregateInfo())
		aggView.SetBorder(true).SetTitle(" " + source.Breadcrumb() + " ")
		detailView = tview.NewTextView().SetDynamicColors(true).SetText(informer.SelectionDetail())
		detailView.SetBorder(true).SetTitle(" selection ")
		header := tview.NewFlex().
			AddItem(aggView, 0, 1, false).
			AddItem(detailView, 0, 1, false)
		root.AddItem(header, 8, 0, false)
	}
	root.AddItem(table, 0, 1, true)

	view := &simpleKindView{flex: root, app: app, source: source}

	table.SetSelectionChangedFunc(func(row, _ int) {
		if row <= 0 || source == nil {
			return
		}
		cell := table.GetCell(row, 0)
		if cell == nil {
			return
		}
		source.SetSelection(cell.GetReference())
		if detailView != nil {
			detailView.SetText(informer.SelectionDetail())
		}
	})
	capture := func(event *tcell.EventKey) *tcell.EventKey {
		if view.OnKey(event) {
			return nil
		}
		return event
	}
	table.SetInputCapture(capture)
	root.SetInputCapture(capture)
	return view
}
