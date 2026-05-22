package view

import (
	"github.com/mohsiur/a16s/internal/api"
	"github.com/rivo/tview"
)

// fakeApp implements kindpkg.App for tests in this package. It records the
// last flash so tests can assert on warning/error paths without spinning up
// tview.
type fakeApp struct {
	clients    *api.Clients
	flashedMsg string
}

func (f *fakeApp) AWSClients() *api.Clients { return f.clients }

func (f *fakeApp) FlashError(msg string) { f.flashedMsg = msg }

func (f *fakeApp) QueueUpdateDraw(fn func()) *tview.Application {
	if fn != nil {
		fn()
	}
	return nil
}

func (f *fakeApp) SetFocus(tview.Primitive) *tview.Application { return nil }
