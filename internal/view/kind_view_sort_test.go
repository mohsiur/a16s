package view

import (
	"testing"

	"github.com/rivo/tview"
)

func buildSortFixture(t *testing.T) *simpleKindView {
	t.Helper()
	table := tview.NewTable()
	headers := []string{"Name", "ApproxMessages"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h))
	}
	rows := [][]string{
		{"alpha", "10"},
		{"beta", "2"},
		{"gamma", "100"},
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

func TestSortByColumnStringDescAscToggle(t *testing.T) {
	v := buildSortFixture(t)

	v.sortByColumn(0)
	if got := v.table.GetCell(1, 0).Text; got != "gamma" {
		t.Fatalf("after first sort row 1 = %q; want gamma (desc)", got)
	}
	if got := v.table.GetCell(3, 0).Text; got != "alpha" {
		t.Fatalf("after first sort row 3 = %q; want alpha (desc)", got)
	}

	v.sortByColumn(0)
	if got := v.table.GetCell(1, 0).Text; got != "alpha" {
		t.Fatalf("after toggle row 1 = %q; want alpha (asc)", got)
	}
}

func TestSortByColumnNumeric(t *testing.T) {
	v := buildSortFixture(t)

	v.sortByColumn(1) // ApproxMessages
	// desc: 100, 10, 2
	if got := v.table.GetCell(1, 1).Text; got != "100" {
		t.Fatalf("row 1 = %q; want 100", got)
	}
	if got := v.table.GetCell(3, 1).Text; got != "2" {
		t.Fatalf("row 3 = %q; want 2", got)
	}

	v.sortByColumn(1) // toggle to asc
	if got := v.table.GetCell(1, 1).Text; got != "2" {
		t.Fatalf("asc row 1 = %q; want 2", got)
	}
}

func TestSortByColumnOutOfRangeNoop(t *testing.T) {
	v := buildSortFixture(t)

	v.sortByColumn(10)

	if got := v.table.GetCell(1, 0).Text; got != "alpha" {
		t.Fatalf("rows reordered after out-of-range sort: row 1 = %q", got)
	}
}
