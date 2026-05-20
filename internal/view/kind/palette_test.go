package kind

import (
	"errors"
	"testing"

	"github.com/keidarcy/e1s/internal/api"
	"github.com/rivo/tview"
)

type fakeApp struct {
	switched   Kind
	switchedV  View
	flashedMsg string
	switchErr  error
	backCalls  int
}

func (f *fakeApp) APIStore() *api.Store { return nil }
func (f *fakeApp) SwitchView(k Kind, v View) error {
	if f.switchErr != nil {
		return f.switchErr
	}
	f.switched = k
	f.switchedV = v
	return nil
}
func (f *fakeApp) FlashError(msg string) { f.flashedMsg = msg }
func (f *fakeApp) Back()                 { f.backCalls++ }
func (f *fakeApp) QueueUpdateDraw(fn func()) *tview.Application {
	if fn != nil {
		fn()
	}
	return nil
}
func (f *fakeApp) SetFocus(tview.Primitive) *tview.Application { return nil }

type buildableKind struct {
	stubKind
	buildErr error
	view     View
}

func (b *buildableKind) Build(App) (View, error) { return b.view, b.buildErr }

func TestPaletteSubmitKnownNameSwitchesView(t *testing.T) {
	resetRegistryForTest()
	k := &buildableKind{stubKind: stubKind{name: "lambda"}}
	Register(k)
	app := &fakeApp{}

	NewPalette(app).Submit("lambda")

	if app.switched != k {
		t.Fatalf("switched = %v; want %v", app.switched, k)
	}
	if app.flashedMsg != "" {
		t.Fatalf("unexpected flash %q", app.flashedMsg)
	}
}

func TestPaletteSubmitUnknownNameFlashes(t *testing.T) {
	resetRegistryForTest()
	app := &fakeApp{}

	NewPalette(app).Submit("nope")

	if app.switched != nil {
		t.Fatal("unexpected SwitchView call")
	}
	if app.flashedMsg == "" {
		t.Fatal("expected FlashError, got none")
	}
}

func TestPaletteSubmitEmptyIsNoop(t *testing.T) {
	resetRegistryForTest()
	app := &fakeApp{}

	NewPalette(app).Submit("")

	if app.switched != nil || app.flashedMsg != "" {
		t.Fatal("empty submit should be no-op")
	}
}

func TestPaletteSubmitBuildErrorFlashes(t *testing.T) {
	resetRegistryForTest()
	Register(&buildableKind{stubKind: stubKind{name: "lambda"}, buildErr: errors.New("boom")})
	app := &fakeApp{}

	NewPalette(app).Submit("lambda")

	if app.switched != nil {
		t.Fatal("Build error should prevent SwitchView")
	}
	if app.flashedMsg != "boom" {
		t.Fatalf("flashed = %q; want %q", app.flashedMsg, "boom")
	}
}

func TestPaletteSubmitSwitchViewErrorFlashes(t *testing.T) {
	resetRegistryForTest()
	Register(&buildableKind{stubKind: stubKind{name: "lambda"}})
	app := &fakeApp{switchErr: errors.New("kaboom")}

	NewPalette(app).Submit("lambda")

	if app.flashedMsg != "kaboom" {
		t.Fatalf("flashed = %q; want %q", app.flashedMsg, "kaboom")
	}
}
