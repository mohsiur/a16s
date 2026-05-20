package view

import (
	"github.com/mohsiur/a16s/internal/api"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// fakeApp implements kindpkg.App for tests in this package. It records the
// last switch + flash so tests can assert on cross-kind navigation behaviour
// without spinning up tview.
type fakeApp struct {
	store        *api.Store
	switchedTo   kindpkg.Kind
	switchedView kindpkg.View
	switchErr    error
	flashedMsg   string
}

func (f *fakeApp) APIStore() *api.Store { return f.store }

func (f *fakeApp) SwitchView(k kindpkg.Kind, v kindpkg.View) error {
	f.switchedTo = k
	f.switchedView = v
	return f.switchErr
}

func (f *fakeApp) FlashError(msg string) { f.flashedMsg = msg }

func (f *fakeApp) Back() {}

func (f *fakeApp) QueueUpdateDraw(fn func()) *tview.Application {
	if fn != nil {
		fn()
	}
	return nil
}

func (f *fakeApp) SetFocus(tview.Primitive) *tview.Application { return nil }
