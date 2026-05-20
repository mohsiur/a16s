package view

import (
	"testing"

	"github.com/rivo/tview"
)

// TestSwitchViewNoopLeavesActiveKindNil locks in the contract that ECS
// adapter kinds (which return *noopView from Build) must NOT set activeKind.
// If activeKind were set, table.go's row-change handler would short-circuit
// the legacy ECS header-pane refresh, regressing pre-branch behaviour.
func TestSwitchViewNoopLeavesActiveKindNil(t *testing.T) {
	app := &App{Pages: tview.NewPages()}
	app.activeKind = &lambdaKind{} // simulate prior flat-kind state
	if err := app.SwitchView(&ecsClusterKind{}, &noopView{}); err != nil {
		t.Fatalf("SwitchView err = %v", err)
	}
	if app.activeKind != nil {
		t.Fatalf("activeKind = %T; want nil after noop SwitchView", app.activeKind)
	}
}

// TestSwitchViewFlatKindSetsActiveKind locks in the inverse: a flat kind
// returning a real view must set activeKind so row-change events flow to it.
func TestSwitchViewFlatKindSetsActiveKind(t *testing.T) {
	app := &App{Pages: tview.NewPages()}
	flat := &lambdaKind{}
	v := &simpleKindView{flex: tview.NewFlex(), app: app, source: flat}
	if err := app.SwitchView(flat, v); err != nil {
		t.Fatalf("SwitchView err = %v", err)
	}
	if app.activeKind == nil {
		t.Fatal("activeKind = nil; want flat kind after non-noop SwitchView")
	}
	if _, ok := app.activeKind.(*lambdaKind); !ok {
		t.Fatalf("activeKind = %T; want *lambdaKind", app.activeKind)
	}
}
