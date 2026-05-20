package view

import (
	"github.com/keidarcy/e1s/internal/api"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
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
