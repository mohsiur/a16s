package view

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/gdamore/tcell/v2"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
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
		queueName := queueNameFromURL(k.selectedURL)
		view := buildPeekTableView(app, queueName, msgs)
		return app.SwitchView(&pseudoKind{name: "peek:" + queueName}, view)
	}
}

// buildPeekTableView renders peeked SQS messages as a sortable column table.
// Empty queues fall back to a TextView so the user gets a clear "(no
// messages)" instead of a header-only table. Enter on a row opens a sub-view
// with the full body, JSON-pretty-printed when parseable.
func buildPeekTableView(app kindpkg.App, queueName string, msgs []sqsTypes.Message) *simpleKindView {
	if len(msgs) == 0 {
		tv := tview.NewTextView().SetText("(no messages)")
		tv.SetBorder(true).SetTitle(" peek " + queueName + " ")
		return newTextSubView(app, tview.NewFlex().AddItem(tv, 0, 1, true))
	}

	table := tview.NewTable().SetBorders(false)
	headers := []string{"MessageId", "Sent", "Size", "Body"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for r, m := range msgs {
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
		cells := []string{
			id,
			peekSentAge(m.Attributes),
			fmt.Sprintf("%d", len(body)),
			preview,
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c).SetMaxWidth(80)
			if col == 0 {
				// Capture body on the first cell so onEnter can resolve it
				// without re-walking the message slice.
				cell.SetReference(body)
			}
			table.SetCell(r+1, col, cell)
		}
	}

	title := fmt.Sprintf("peek %s (%d)", queueName, len(msgs))
	return newTableSubView(app, table, title, func(row int) {
		ref, _ := table.GetCell(row, 0).GetReference().(string)
		showPeekBody(app, queueName, ref)
	})
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

// showPeekBody opens a TextView with the full message body, pretty-printed
// when the body parses as JSON. Esc returns to the peek table.
func showPeekBody(app kindpkg.App, queueName, body string) {
	display := body
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(body), "", "  ") == nil && pretty.Len() > 0 {
		display = pretty.String()
	}
	tv := tview.NewTextView().SetText(display)
	tv.SetBorder(true).SetTitle(" peek " + queueName + " body ")
	_ = app.SwitchView(
		&pseudoKind{name: "peek-body:" + queueName},
		newTextSubView(app, tview.NewFlex().AddItem(tv, 0, 1, true)),
	)
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
		if err := app.APIStore().SendMessage(context.Background(), k.selectedURL, `{"a16s":"test"}`); err != nil {
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

// ---- Legacy-style ECS chrome integration ----

type sqsView struct {
	view
	urls       []string
	attrsByURL map[string]map[string]string
}

func newSQSView(urls []string, attrsByURL map[string]map[string]string, app *App) *sqsView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
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
		if err := sk.loadInventory(app); err != nil {
			return err
		}
		sk.mu.RLock()
		urls := append([]string(nil), sk.urls...)
		attrs := make(map[string]map[string]string, len(sk.attrsByURL))
		for k, v := range sk.attrsByURL {
			attrs[k] = v
		}
		sk.mu.RUnlock()
		return buildResourcePage(urls, app, nil, func() resourceViewBuilder {
			return newSQSView(urls, attrs, app)
		})
	}
	urls, err := app.Store.ListQueues(context.Background())
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

func (v *sqsView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.sqs
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
	if app.sqsQueueName == "" {
		app.Notice.Warn("no queue selected")
		app.back()
		return nil
	}
	if switched := app.switchPage(reload); switched {
		return nil
	}
	msgs, err := app.Store.PeekMessages(context.Background(), app.sqsQueueName)
	return buildResourcePage(msgs, app, err, func() resourceViewBuilder {
		return newSQSPeekView(app.sqsQueueName, msgs, app)
	})
}

func (v *sqsPeekView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.sqsPeek
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
