package view

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
	"golang.org/x/sync/errgroup"
)

func init() {
	kindpkg.Register(&sqsKind{})
	kindpkg.Register(&sqsPeekKind{})
	bindKind(SQSKind, "sqs", "queues", "sqs")
	bindKind(SQSPeekKind, "sqs-messages")
}

type sqsKind struct {
	kindpkg.BaseKind
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

func (k *sqsKind) Name() string  { return "sqs" }
func (k *sqsKind) Title() string { return "queues" }

func (k *sqsKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showQueuesPage(reload)
	}
	return nil
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

// BrowserURL returns the AWS console URL for the queue under the cursor. Both
// SQSKind (queue list) and SQSPeekKind (per-queue messages page) route here
// because they open the same console URL today; resource_dispatch.go maps both
// enum values to "sqs". Returns "" when no queue is selected so openInBrowser
// falls through to its legacy switch.
func (k *sqsKind) BrowserURL(region string) (string, error) {
	if k.selectedURL == "" {
		return "", nil
	}
	return utils.SQSQueueURL(region, k.selectedURL), nil
}

// FooterItem describes the sqs kind's footer summary cell.
func (k *sqsKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "queues"}
}

// Traits flag the affordances SQS opts into. Drillable covers Enter→messages
// (SQSPeekKind) and Browsable covers `o`→AWS console.
func (k *sqsKind) Traits() kindpkg.Traits {
	return kindpkg.Traits{
		Filterable:  true,
		Refreshable: true,
		Drillable:   true,
		Browsable:   true,
	}
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

// peekSentAge converts SentTimestamp (epoch milliseconds, set by SQS for
// every message) into a relative age string. Falls back to "" when the
// attribute is missing or unparsable so sort still groups them together.
func peekSentAge(attrs map[string]string) string {
	raw, ok := attrs["SentTimestamp"]
	if !ok {
		return ""
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	t := time.UnixMilli(ms)
	return utils.Age(&t)
}

// Preload satisfies kindpkg.Preloader. Fired in a goroutine on app start so
// the first `:sqs` is instant. Safe to call concurrently with Build —
// loadInventory uses RWMutex.
func (k *sqsKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app, false)
}

// Refresh satisfies kindpkg.Refresher. Called off the tview event loop by
// the auto-refresh ticker so the AWS round-trip never blocks scroll input.
func (k *sqsKind) Refresh(app kindpkg.App) error {
	return k.loadInventory(app, true)
}

// loadInventory fetches the queue list + attributes and caches the result.
// Concurrent callers single-flight on k.loadDone — the first caller runs the
// fetch and closes the channel; subsequent callers (including Preload + a
// fast `:sqs`) block on the channel and read the shared result. When reload
// is true, the cache is invalidated before the fetch so refresh keys (`r`)
// and the auto-refresh ticker actually re-hit the AWS API; selectedURL is
// preserved.
func (k *sqsKind) loadInventory(app kindpkg.App, reload bool) error {
	k.mu.Lock()
	if reload {
		k.loaded = false
		k.urls = nil
		k.attrsByURL = nil
		k.loadErr = nil
		k.loadDone = nil
	}
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

	urls, err := app.AWSClients().ListQueues(context.Background())
	var attrsByURL map[string]map[string]string
	if err == nil {
		attrs := make([]map[string]string, len(urls))
		g, ctx := errgroup.WithContext(context.Background())
		g.SetLimit(10)
		for i, url := range urls {
			i, url := i, url
			g.Go(func() error {
				a, _ := app.AWSClients().GetQueueAttributes(ctx, url)
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

// promoteSelectedURL upgrades a bare queue-name selection (set by cross-kind
// navigation from Lambda DLQ) to a full queue URL using the cached inventory.
// Returns the full URL or "" if no match. Called by showQueuesPage so the
// legacy chrome's pre-selection lands on the correct row.
func (k *sqsKind) promoteSelectedURL() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.selectedURL == "" {
		return ""
	}
	for _, url := range k.urls {
		if url == k.selectedURL || queueNameFromURL(url) == k.selectedURL {
			return url
		}
	}
	return ""
}

// ---- Legacy-style ECS chrome integration ----

type sqsView struct {
	view
	urls       []string
	attrsByURL map[string]map[string]string
}

func newSQSView(urls []string, attrsByURL map[string]map[string]string, app *App) *sqsView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
		hotKeyMap["sqsP"],
		hotKeyMap["sqsS"],
	}...)
	return &sqsView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		urls:       urls,
		attrsByURL: attrsByURL,
	}
}

// showQueuesPage is the SQS list page. Mirrors lambda.go: prefer the cached
// inventory from sqsKind so first paint after `:sqs` is instant; fall back to
// a synchronous list.
func (app *App) showQueuesPage(reload bool) error {
	app.kind = SQSKind
	if switched := app.switchPage(reload); switched {
		return nil
	}
	sk := getSQSKind()
	if sk != nil {
		if err := sk.loadInventory(app, reload); err != nil {
			return err
		}
		// Promote any bare-name selection (e.g. cross-kind nav from Lambda DLQ)
		// to the matching full URL before buildResourcePage seeds the cursor.
		if full := sk.promoteSelectedURL(); full != "" {
			sk.SetSelection(full)
		}
		sk.mu.RLock()
		urls := append([]string(nil), sk.urls...)
		attrs := make(map[string]map[string]string, len(sk.attrsByURL))
		for k, v := range sk.attrsByURL {
			attrs[k] = v
		}
		sk.mu.RUnlock()
		if selected := app.SQSQueueURL(); selected != "" {
			for i, url := range urls {
				if url == selected {
					app.rowIndex = i + 1
					break
				}
			}
		}
		return buildResourcePage(urls, app, nil, func() resourceViewBuilder {
			return newSQSView(urls, attrs, app)
		})
	}
	urls, err := app.Clients.ListQueues(context.Background())
	return buildResourcePage(urls, app, err, func() resourceViewBuilder {
		return newSQSView(urls, nil, app)
	})
}

func getSQSKind() *sqsKind {
	k, ok := kindpkg.Get("sqs")
	if !ok {
		return nil
	}
	sk, _ := k.(*sqsKind)
	return sk
}

// sqsPeekKind is the Resource adapter for SQSPeekKind (per-queue messages).
// BrowserURL delegates to the parent sqsKind so `o` opens the queue's console
// page; FooterItem returns "messages" so the footer reflects the leaf view.
type sqsPeekKind struct {
	kindpkg.BaseKind
}

func (k *sqsPeekKind) Name() string     { return "sqs-messages" }
func (k *sqsPeekKind) Title() string    { return "messages" }
func (k *sqsPeekKind) Reset()           {}
func (k *sqsPeekKind) Selection() any   { return nil }
func (k *sqsPeekKind) SetSelection(any) {}

func (k *sqsPeekKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showQueueMessagesPage(reload)
	}
	return nil
}
func (k *sqsPeekKind) BrowserURL(region string) (string, error) {
	if sk := getSQSKind(); sk != nil {
		return sk.BrowserURL(region)
	}
	return "", nil
}
func (k *sqsPeekKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "messages"}
}
func (k *sqsPeekKind) Traits() kindpkg.Traits {
	return kindpkg.Traits{WideTable: true}
}

// PageHandle returns the parent queue's URL so message pages stay scoped to
// the active queue.
func (k *sqsPeekKind) PageHandle() string {
	if sk := getSQSKind(); sk != nil {
		return sk.selectedURL
	}
	return ""
}

func (v *sqsView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.middle
}

func (v *sqsView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.urls))
	for i, url := range v.urls {
		params = append(params, headerPageParam{
			title:      queueNameFromURL(url),
			entityName: url,
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *sqsView) headerPageItems(index int) []headerItem {
	url := v.urls[index]
	a := v.attrsByURL[url]
	dlq := "no"
	if a["RedrivePolicy"] != "" {
		dlq = "yes"
	}
	return []headerItem{
		{name: "Name", value: queueNameFromURL(url)},
		{name: "URL", value: url},
		{name: "Messages", value: orEmpty(a["ApproximateNumberOfMessages"])},
		{name: "In flight", value: orEmpty(a["ApproximateNumberOfMessagesNotVisible"])},
		{name: "Delayed", value: orEmpty(a["ApproximateNumberOfMessagesDelayed"])},
		{name: "DLQ", value: dlq},
		{name: "VisibilityTimeout", value: orEmpty(a["VisibilityTimeout"])},
		{name: "RetentionPeriod", value: orEmpty(a["MessageRetentionPeriod"])},
	}
}

func (v *sqsView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, "all", len(v.urls))
	headers = []string{"Name", "Messages", "InFlight", "Delayed", "DLQ"}
	rowsBuilder = func() (data [][]string) {
		for _, url := range v.urls {
			copyURL := url
			a := v.attrsByURL[url]
			dlq := "no"
			if a["RedrivePolicy"] != "" {
				dlq = "yes"
			}
			row := []string{
				queueNameFromURL(url),
				orEmpty(a["ApproximateNumberOfMessages"]),
				orEmpty(a["ApproximateNumberOfMessagesNotVisible"]),
				orEmpty(a["ApproximateNumberOfMessagesDelayed"]),
				dlq,
			}
			data = append(data, row)
			entity := Entity{
				sqsQueueName: copyURL,
				entityName:   copyURL,
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}

func orEmpty(s string) string {
	if s == "" {
		return utils.EmptyText
	}
	return s
}

// sqsDescribeData returns a JSON-serialisable view of the selected queue:
// its URL plus whatever attributes the cache has for it. Used by the legacy
// `d` describe action so SQS describe lands in the same chrome as ECS
// describe.
func sqsDescribeData(v *view, entity Entity) any {
	out := struct {
		URL        string            `json:"url"`
		Name       string            `json:"name"`
		Attributes map[string]string `json:"attributes,omitempty"`
	}{
		URL:  entity.sqsQueueName,
		Name: queueNameFromURL(entity.sqsQueueName),
	}
	if sk := getSQSKind(); sk != nil {
		sk.mu.RLock()
		if a, ok := sk.attrsByURL[entity.sqsQueueName]; ok {
			out.Attributes = a
		}
		sk.mu.RUnlock()
	}
	return out
}

// ---- SQS Peek (messages) ----

type sqsPeekView struct {
	view
	queueURL string
	queue    string
	messages []sqsTypes.Message
}

func newSQSPeekView(queueURL string, msgs []sqsTypes.Message, app *App) *sqsPeekView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
	}...)
	return &sqsPeekView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		queueURL: queueURL,
		queue:    queueNameFromURL(queueURL),
		messages: msgs,
	}
}

func (app *App) showQueueMessagesPage(reload bool) error {
	app.kind = SQSPeekKind
	queueURL := app.SQSQueueURL()
	if queueURL == "" {
		app.Notice.Warn("no queue selected")
		app.back()
		return nil
	}
	if switched := app.switchPage(reload); switched {
		return nil
	}
	msgs, err := app.Clients.PeekMessages(context.Background(), queueURL)
	return buildResourcePage(msgs, app, err, func() resourceViewBuilder {
		return newSQSPeekView(queueURL, msgs, app)
	})
}

func (v *sqsPeekView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.middle
}

func (v *sqsPeekView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.messages))
	for i, m := range v.messages {
		id := ""
		if m.MessageId != nil {
			id = *m.MessageId
		}
		params = append(params, headerPageParam{
			title:      v.queue + " > " + id,
			entityName: id,
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *sqsPeekView) headerPageItems(index int) []headerItem {
	m := v.messages[index]
	body := ""
	if m.Body != nil {
		body = *m.Body
	}
	preview := body
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}
	id := ""
	if m.MessageId != nil {
		id = *m.MessageId
	}
	receipt := ""
	if m.ReceiptHandle != nil {
		r := *m.ReceiptHandle
		if len(r) > 30 {
			receipt = r[:30] + "…"
		} else {
			receipt = r
		}
	}
	return []headerItem{
		{name: "MessageId", value: id},
		{name: "Queue", value: v.queue},
		{name: "Sent", value: peekSentAge(m.Attributes)},
		{name: "Size", value: fmt.Sprintf("%d", len(body))},
		{name: "ReceiptHandle", value: receipt},
		{name: "Body preview", value: strings.ReplaceAll(preview, "\n", " ")},
	}
}

func (v *sqsPeekView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, v.queue, len(v.messages))
	headers = []string{"MessageId", "Sent", "Size", "Body"}
	rowsBuilder = func() (data [][]string) {
		for _, m := range v.messages {
			copyM := m
			body := ""
			if m.Body != nil {
				body = *m.Body
			}
			preview := body
			if len(preview) > 80 {
				preview = strings.ReplaceAll(preview[:80], "\n", " ") + "…"
			} else {
				preview = strings.ReplaceAll(preview, "\n", " ")
			}
			id := ""
			if m.MessageId != nil {
				id = *m.MessageId
			}
			row := []string{
				id,
				peekSentAge(m.Attributes),
				fmt.Sprintf("%d", len(body)),
				preview,
			}
			data = append(data, row)
			entity := Entity{
				sqsMessage: &copyM,
				entityName: id,
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}
