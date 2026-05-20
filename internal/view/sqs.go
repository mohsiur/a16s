package view

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
	"golang.org/x/sync/errgroup"
)

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func init() { kindpkg.Register(&sqsKind{}) }

type sqsKind struct {
	// selectedURL is set in two ways:
	//   1. Row-change in Build's table fires SetSelection(fullURL).
	//   2. Cross-kind nav from Lambda DLQ calls SetSelection(bareQueueName).
	// Build's pre-selection block promotes (2) to a full URL after listing.
	selectedURL string
	// inventory is captured during Build / Preload so Informer methods can
	// compute aggregate + per-row detail without re-querying SQS.
	// `loadDone` is the single-flight latch: nil before the first load, set
	// to a fresh channel when a load starts, closed when it finishes. This
	// guarantees concurrent Preload + Build only fetch once and the second
	// caller waits.
	mu         sync.RWMutex
	urls       []string
	attrsByURL map[string]map[string]string
	loaded     bool
	loadDone   chan struct{}
	loadErr    error
}

func (k *sqsKind) Name() string {
	return "sqs"
}

func (k *sqsKind) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.selectedURL = ""
	k.urls = nil
	k.attrsByURL = nil
	k.loaded = false
	k.loadDone = nil
	k.loadErr = nil
}

func (k *sqsKind) Selection() any { return k.selectedURL }

func (k *sqsKind) SetSelection(s any) {
	if str, ok := s.(string); ok {
		k.selectedURL = str
	}
}

func (k *sqsKind) Breadcrumb() string {
	if k.selectedURL == "" {
		return "sqs"
	}
	return "sqs > " + queueNameFromURL(k.selectedURL)
}

// queueNameFromURL returns the last path segment of an SQS queue URL. Bare
// names (no `/`) pass through unchanged so cross-kind nav from Lambda DLQ
// (which only knows the queue name) doesn't need special-casing in callers.
func queueNameFromURL(url string) string {
	idx := strings.LastIndex(url, "/")
	if idx < 0 {
		return url
	}
	return url[idx+1:]
}

func (k *sqsKind) PrimaryAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		msgs, err := app.APIStore().PeekMessages(context.Background(), k.selectedURL)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		var body strings.Builder
		if len(msgs) == 0 {
			body.WriteString("(no messages)")
		}
		for _, m := range msgs {
			body.WriteString("---\n")
			if m.Body != nil {
				body.WriteString(*m.Body)
			}
			body.WriteString("\n")
		}
		tv := tview.NewTextView().SetText(body.String())
		tv.SetBorder(true).SetTitle(" peek " + queueNameFromURL(k.selectedURL) + " ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&pseudoKind{name: "peek:" + queueNameFromURL(k.selectedURL)},
			newTextSubView(app, flex),
		)
	}
}

func (k *sqsKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'p', Label: "purge", Run: k.purgeAction()},
		{Key: 's', Label: "send", Run: k.sendAction()},
	}
}

func (k *sqsKind) purgeAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: no confirm modal — typing `p` purges immediately. A modal
		// requiring the user to type the queue name is a follow-up.
		if err := app.APIStore().PurgeQueue(context.Background(), k.selectedURL); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("purged " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

func (k *sqsKind) sendAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: fixed test payload. Modal input is a follow-up.
		if err := app.APIStore().SendMessage(context.Background(), k.selectedURL, `{"e1s":"test"}`); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("sent test message to " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

// Preload satisfies kindpkg.Preloader. Fired in a goroutine on app start so
// the first `:sqs` is instant. Safe to call concurrently with Build —
// loadInventory uses RWMutex.
func (k *sqsKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app)
}

// loadInventory fetches the queue list + attributes once and caches the
// result. Concurrent callers single-flight on k.loadDone — the first caller
// runs the fetch and closes the channel; subsequent callers (including
// Preload + a fast `:sqs`) block on the channel and read the shared result.
// After Reset() the cycle restarts.
func (k *sqsKind) loadInventory(app kindpkg.App) error {
	k.mu.Lock()
	if k.loaded {
		k.mu.Unlock()
		return nil
	}
	if k.loadDone != nil {
		done := k.loadDone
		k.mu.Unlock()
		<-done
		k.mu.RLock()
		err := k.loadErr
		k.mu.RUnlock()
		return err
	}
	done := make(chan struct{})
	k.loadDone = done
	k.mu.Unlock()

	urls, err := app.APIStore().ListQueues(context.Background())
	var attrsByURL map[string]map[string]string
	if err == nil {
		attrs := make([]map[string]string, len(urls))
		g, ctx := errgroup.WithContext(context.Background())
		g.SetLimit(10)
		for i, url := range urls {
			i, url := i, url
			g.Go(func() error {
				a, _ := app.APIStore().GetQueueAttributes(ctx, url)
				attrs[i] = a
				return nil
			})
		}
		_ = g.Wait()
		attrsByURL = make(map[string]map[string]string, len(urls))
		for i, url := range urls {
			attrsByURL[url] = attrs[i]
		}
	}

	k.mu.Lock()
	k.loadErr = err
	if err == nil {
		k.urls = urls
		k.attrsByURL = attrsByURL
		k.loaded = true
	} else {
		// Reset loadDone on error so a future call can retry; closing the
		// channel below still wakes anyone currently waiting.
		k.loadDone = nil
	}
	k.mu.Unlock()
	close(done)
	return err
}

func (k *sqsKind) buildTable() *tview.Table {
	k.mu.RLock()
	urls := k.urls
	attrsByURL := k.attrsByURL
	k.mu.RUnlock()

	table := tview.NewTable().SetBorders(false)
	headers := []string{"Name", "ApproxMessages", "ApproxInFlight", "ApproxDelayed", "DLQ"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}

	for row, url := range urls {
		copyURL := url
		a := attrsByURL[url]
		cells := []string{
			queueNameFromURL(url),
			a["ApproximateNumberOfMessages"],
			a["ApproximateNumberOfMessagesNotVisible"],
			a["ApproximateNumberOfMessagesDelayed"],
			fmt.Sprint(a["RedrivePolicy"] != ""),
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyURL)
			}
			table.SetCell(row+1, col, cell)
		}
	}

	// Pre-selection: SetSelection may have been called with either a full URL
	// (from a previous row-change) or a bare queue name (from cross-kind nav
	// from Lambda DLQ). Match against the row's reference (full URL) directly,
	// or against queueNameFromURL(reference) for the bare-name case.
	if k.selectedURL != "" {
		for r := 1; r <= len(urls); r++ {
			cell := table.GetCell(r, 0)
			if cell == nil {
				continue
			}
			ref, _ := cell.GetReference().(string)
			if ref == k.selectedURL || queueNameFromURL(ref) == k.selectedURL {
				table.Select(r, 0)
				k.selectedURL = ref // promote bare name to full URL
				break
			}
		}
	}

	return table
}

func (k *sqsKind) Build(app kindpkg.App) (kindpkg.View, error) {
	k.mu.RLock()
	loaded := k.loaded
	k.mu.RUnlock()
	if loaded {
		return newTableKindView(app, k, k.buildTable()), nil
	}
	return newLoadingTableKindView(app, k, func() error {
		return k.loadInventory(app)
	}, k.buildTable), nil
}

// AggregateInfo / SelectionDetail satisfy kindpkg.Informer.
func (k *sqsKind) AggregateInfo() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if len(k.urls) == 0 {
		return "No queues"
	}
	var totalMsgs, totalInflight, dlqCount int
	for _, url := range k.urls {
		a := k.attrsByURL[url]
		totalMsgs += atoiOrZero(a["ApproximateNumberOfMessages"])
		totalInflight += atoiOrZero(a["ApproximateNumberOfMessagesNotVisible"])
		if a["RedrivePolicy"] != "" {
			dlqCount++
		}
	}
	return fmt.Sprintf(
		"Queues: %d\nTotal messages: %d\nIn flight: %d\nWith DLQ: %d",
		len(k.urls), totalMsgs, totalInflight, dlqCount,
	)
}

func (k *sqsKind) SelectionDetail() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.selectedURL == "" {
		return ""
	}
	a := k.attrsByURL[k.selectedURL]
	if a == nil {
		return queueNameFromURL(k.selectedURL)
	}
	dlq := "no"
	if a["RedrivePolicy"] != "" {
		dlq = "yes"
	}
	return fmt.Sprintf(
		"Name: %s\nMessages: %s\nIn flight: %s\nDelayed: %s\nDLQ: %s",
		queueNameFromURL(k.selectedURL),
		a["ApproximateNumberOfMessages"],
		a["ApproximateNumberOfMessagesNotVisible"],
		a["ApproximateNumberOfMessagesDelayed"],
		dlq,
	)
}
