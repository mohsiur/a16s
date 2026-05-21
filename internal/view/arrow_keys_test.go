package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// On leaf flat-kind tables (DDB scan, SQS peek, Lambda list) horizontal
// arrow keys must scroll the table, not navigate. tview moves the column
// offset on Left/Right when the event reaches the table widget — so the
// app's input capture must let those events pass through unchanged for
// these kinds. h/Esc still navigate back; only the literal arrow keys
// fall through.
func TestArrowKeysFallThroughOnLeafFlatKinds(t *testing.T) {
	leafKinds := []kind{DynamoDBScanKind, SQSPeekKind, LambdaKind}
	arrowKeys := []tcell.Key{tcell.KeyLeft, tcell.KeyRight}

	for _, k := range leafKinds {
		for _, key := range arrowKeys {
			app, err := newApp(Option{})
			if err != nil {
				t.Fatalf("newApp: %v", err)
			}
			app.kind = k
			v := newView(app, nil, nil)

			before := app.kind
			ev := tcell.NewEventKey(key, 0, tcell.ModNone)
			got := v.handleInputCapture(ev)

			if app.kind != before {
				t.Errorf("kind=%v key=%v: app.kind changed to %v (back was triggered)", k, key, app.kind)
			}
			if got != ev {
				t.Errorf("kind=%v key=%v: event was swallowed; tview will not see it", k, key)
			}
		}
	}
}

// Sanity: on non-leaf kinds (e.g. ClusterKind) the arrow keys keep their
// current behavior — Right drills in, Left goes back. We assert the back
// path because drilling requires a table populated with rows.
func TestLeftArrowStillBacksOnNonLeafKinds(t *testing.T) {
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	app.kind = ServiceKind
	v := newView(app, nil, nil)

	ev := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
	v.handleInputCapture(ev)

	if app.kind == ServiceKind {
		t.Fatalf("expected back() to change kind from ServiceKind, got %v", app.kind)
	}
}
