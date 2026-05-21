package view

import (
	"testing"
	"time"
)

// TestOnCloseCancelsContext asserts the contract the auto-refresh ticker
// goroutine relies on: app.onClose cancels app.ctx so the goroutine's
// `select { case <-ctx.Done(): return; case <-ticker.C: ... }` exits cleanly
// once Application.Run() has returned. Without this, the ticker would outlive
// shutdown and race with tview teardown when calling QueueUpdateDraw.
func TestOnCloseCancelsContext(t *testing.T) {
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	if app.ctx == nil {
		t.Fatal("expected app.ctx to be non-nil after newApp")
	}
	select {
	case <-app.ctx.Done():
		t.Fatal("app.ctx already cancelled before onClose")
	default:
	}

	app.onClose()

	select {
	case <-app.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("app.ctx not cancelled within 1s of onClose")
	}
}

// TestRefreshGoroutineExitsOnCancel mirrors the auto-refresh ticker structure
// from app.start() and asserts that cancelling app.ctx causes the goroutine to
// return promptly even if the ticker keeps firing. This guards against future
// edits that might drop the ctx.Done() case from the select.
func TestRefreshGoroutineExitsOnCancel(t *testing.T) {
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	ticker := time.NewTicker(time.Millisecond)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ticker.Stop()
		ctx := app.ctx
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	app.onClose()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ticker goroutine did not exit within 1s of onClose")
	}
}
