package view

import (
	"strings"

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
			table := buildTable()
			populateTableKindView(root, app, source, table)
			// The loading placeholder held focus; after the swap, hand focus
			// to the table so arrow keys move the row cursor immediately.
			app.SetFocus(table)
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
//
// Filter state: when the user presses `/`, showFilter mounts a one-line
// InputField above the table. originalCells captures the unfiltered table
// cells once at filter-show time (rebuilds on every populateTableKindView)
// so backspace can restore rows. Enter dismisses the input but keeps the
// filter applied; Esc clears + dismisses.
type simpleKindView struct {
	flex   *tview.Flex
	app    kindpkg.App
	source kindpkg.Kind

	table         *tview.Table
	filterInput   *tview.InputField
	filterActive  bool
	originalCells [][]*tview.TableCell
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
	if event.Rune() == '/' && s.table != nil {
		s.showFilter()
		return true
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

// showFilter mounts a one-line InputField at the top of the view and
// applies a column-0 substring match on every keystroke. Enter keeps the
// filter applied and returns focus to the table; Esc clears it.
func (s *simpleKindView) showFilter() {
	if s.filterActive || s.table == nil {
		return
	}
	s.snapshotCells()
	s.filterActive = true
	s.filterInput = tview.NewInputField().SetLabel("/").SetFieldWidth(0)
	s.filterInput.SetChangedFunc(func(text string) {
		s.applyFilter(text)
	})
	s.filterInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			s.dismissFilter(false)
		case tcell.KeyEsc:
			s.dismissFilter(true)
		}
	})
	// Mount the input as a 1-row item at the top of the view's flex root.
	// AddItem appends, so we have to rebuild the children list — easier to
	// just stash the existing children and re-add them after the input.
	children := s.collectChildren()
	s.flex.Clear()
	s.flex.AddItem(s.filterInput, 1, 0, true)
	for _, c := range children {
		s.flex.AddItem(c.item, c.fixed, c.proportion, c.focus)
	}
	if s.app != nil {
		s.app.SetFocus(s.filterInput)
	}
}

// snapshotCells walks the current table and stores a row-major snapshot of
// every TableCell pointer. We re-attach these pointers (not copies) when
// rebuilding the table, so cell references — which the row-change handler
// uses to push selection back to the source Kind — survive the filter.
func (s *simpleKindView) snapshotCells() {
	rows := s.table.GetRowCount()
	cols := s.table.GetColumnCount()
	cells := make([][]*tview.TableCell, rows)
	for r := 0; r < rows; r++ {
		row := make([]*tview.TableCell, cols)
		for c := 0; c < cols; c++ {
			row[c] = s.table.GetCell(r, c)
		}
		cells[r] = row
	}
	s.originalCells = cells
}

// applyFilter rebuilds the visible table by skipping rows whose first-column
// text doesn't contain `text` (case-insensitive). The header row (row 0) is
// always preserved.
func (s *simpleKindView) applyFilter(text string) {
	if s.originalCells == nil {
		return
	}
	needle := strings.ToLower(strings.TrimSpace(text))
	s.table.Clear()
	if len(s.originalCells) == 0 {
		return
	}
	header := s.originalCells[0]
	for c, cell := range header {
		if cell != nil {
			s.table.SetCell(0, c, cell)
		}
	}
	out := 1
	for r := 1; r < len(s.originalCells); r++ {
		row := s.originalCells[r]
		if len(row) == 0 {
			continue
		}
		first := ""
		if row[0] != nil {
			first = strings.ToLower(row[0].Text)
		}
		if needle != "" && !strings.Contains(first, needle) {
			continue
		}
		for c, cell := range row {
			if cell != nil {
				s.table.SetCell(out, c, cell)
			}
		}
		out++
	}
	if out > 1 {
		s.table.Select(1, 0)
	}
}

// dismissFilter removes the input row and restores focus to the table. If
// reset is true, the filter is cleared first so the user sees all rows.
func (s *simpleKindView) dismissFilter(reset bool) {
	if !s.filterActive {
		return
	}
	if reset {
		s.applyFilter("")
	}
	s.flex.RemoveItem(s.filterInput)
	s.filterActive = false
	s.filterInput = nil
	if s.app != nil && s.table != nil {
		s.app.SetFocus(s.table)
	}
}

// flexChild captures one tview.Flex child slot so showFilter can re-add
// children in order when prepending the filter input. tview doesn't expose
// the existing AddItem args, so we recreate them from the layout we know
// populateTableKindView built (header 8 fixed + table flex 1).
type flexChild struct {
	item       tview.Primitive
	fixed      int
	proportion int
	focus      bool
}

func (s *simpleKindView) collectChildren() []flexChild {
	count := s.flex.GetItemCount()
	out := make([]flexChild, 0, count)
	for i := 0; i < count; i++ {
		item := s.flex.GetItem(i)
		// Mirror the layout populateTableKindView produces: optional 8-row
		// header, then a flex-1 table. If the source had no informer the
		// loop just yields the table.
		if item == s.table {
			out = append(out, flexChild{item: item, fixed: 0, proportion: 1, focus: true})
		} else {
			out = append(out, flexChild{item: item, fixed: 8, proportion: 0, focus: false})
		}
	}
	return out
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

	view := &simpleKindView{flex: root, app: app, source: source, table: table}

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
