package view

import (
	"testing"

	"github.com/rivo/tview"
)

// buildFilterFixture wires a simpleKindView against a populated table the
// way populateTableKindView would, without involving the kind registry or
// app.SetFocus. The view's source is left nil — the filter path doesn't
// touch it, only the table.
func buildFilterFixture(t *testing.T) *simpleKindView {
	t.Helper()
	table := tview.NewTable()
	headers := []string{"Name", "Runtime"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h))
	}
	rows := [][]string{
		{"alpha-fn", "go1.x"},
		{"beta-fn", "python3.11"},
		{"gamma-svc", "nodejs20.x"},
	}
	for r, row := range rows {
		for c, v := range row {
			table.SetCell(r+1, c, tview.NewTableCell(v))
		}
	}
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(table, 0, 1, true)
	return &simpleKindView{flex: flex, table: table}
}

func TestApplyFilterKeepsHeaderAndMatchingRows(t *testing.T) {
	v := buildFilterFixture(t)
	v.snapshotCells()

	v.applyFilter("fn")

	if got := v.table.GetRowCount(); got != 3 {
		t.Fatalf("row count = %d; want 3 (header + alpha + beta)", got)
	}
	if got := v.table.GetCell(0, 0).Text; got != "Name" {
		t.Fatalf("row 0 = %q; want header preserved", got)
	}
	if got := v.table.GetCell(1, 0).Text; got != "alpha-fn" {
		t.Fatalf("row 1 = %q; want \"alpha-fn\"", got)
	}
	if got := v.table.GetCell(2, 0).Text; got != "beta-fn" {
		t.Fatalf("row 2 = %q; want \"beta-fn\"", got)
	}
}

func TestApplyFilterEmptyTextRestoresAllRows(t *testing.T) {
	v := buildFilterFixture(t)
	v.snapshotCells()

	v.applyFilter("fn")
	v.applyFilter("")

	if got := v.table.GetRowCount(); got != 4 {
		t.Fatalf("row count = %d; want 4 after clearing filter", got)
	}
}

func TestApplyFilterIsCaseInsensitive(t *testing.T) {
	v := buildFilterFixture(t)
	v.snapshotCells()

	v.applyFilter("ALPHA")

	if got := v.table.GetRowCount(); got != 2 {
		t.Fatalf("row count = %d; want 2", got)
	}
	if got := v.table.GetCell(1, 0).Text; got != "alpha-fn" {
		t.Fatalf("row 1 = %q; want \"alpha-fn\"", got)
	}
}

func TestApplyFilterNoMatchesLeavesOnlyHeader(t *testing.T) {
	v := buildFilterFixture(t)
	v.snapshotCells()

	v.applyFilter("zzz")

	if got := v.table.GetRowCount(); got != 1 {
		t.Fatalf("row count = %d; want 1 (header only)", got)
	}
}
