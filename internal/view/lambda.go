package view

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kindpkg.Register(&lambdaKind{}) }

type lambdaKind struct {
	selected *lambdaTypes.FunctionConfiguration
	// inventory captured during Build / Preload so Informer methods can
	// compute aggregate + per-row detail without re-listing functions.
	// `loadDone` is the single-flight latch: nil before the first load, set
	// to a fresh channel when a load starts, closed when it finishes. This
	// guarantees concurrent Preload + Build only fetch once and the second
	// caller waits.
	mu       sync.RWMutex
	fns      []lambdaTypes.FunctionConfiguration
	loaded   bool
	loadDone chan struct{}
	loadErr  error
}

func (k *lambdaKind) Name() string { return "lambda" }

func (k *lambdaKind) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.selected = nil
	k.fns = nil
	k.loaded = false
	k.loadDone = nil
	k.loadErr = nil
}

func (k *lambdaKind) Selection() any {
	if k.selected == nil {
		return nil
	}
	return k.selected
}

func (k *lambdaKind) SetSelection(s any) {
	if fn, ok := s.(*lambdaTypes.FunctionConfiguration); ok {
		k.selected = fn
	}
}

func (k *lambdaKind) Breadcrumb() string {
	if k.selected == nil || k.selected.FunctionName == nil {
		return "lambda"
	}
	return "lambda > " + aws.ToString(k.selected.FunctionName)
}

func (k *lambdaKind) PrimaryAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		logGroup := "/aws/lambda/" + aws.ToString(k.selected.FunctionName)
		return openLogGroupTail(app, logGroup)
	}
}

func (k *lambdaKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'i', Label: "invoke", Run: k.invokeAction()},
		{Key: 'd', Label: "dlq", Run: k.dlqAction()},
		{Key: 'c', Label: "config", Run: k.configAction()},
	}
}

func (k *lambdaKind) invokeAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		// MVP: empty payload — invoke as-is, show response. A modal input is a
		// follow-up.
		out, err := app.APIStore().InvokeFunction(context.Background(), aws.ToString(k.selected.FunctionName), []byte("{}"))
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		body := string(out.Payload)
		title := " invoke result "
		if out.FunctionError != nil {
			title = " invoke result (error) "
		}
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(title)
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&pseudoKind{name: "invoke:" + aws.ToString(k.selected.FunctionName)}, newTextSubView(app, flex))
	}
}

// parseQueueNameFromArn returns the queue name from an SQS ARN like
// "arn:aws:sqs:us-east-1:111:my-dlq". Returns "" for non-SQS ARNs or
// malformed input.
func parseQueueNameFromArn(arn string) string {
	if !strings.HasPrefix(arn, "arn:aws:sqs:") {
		return ""
	}
	idx := strings.LastIndex(arn, ":")
	if idx < 0 || idx == len(arn)-1 {
		return ""
	}
	return arn[idx+1:]
}

func (k *lambdaKind) dlqAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		if k.selected.DeadLetterConfig == nil || k.selected.DeadLetterConfig.TargetArn == nil {
			app.FlashError("no DLQ configured")
			return nil
		}
		queueName := parseQueueNameFromArn(aws.ToString(k.selected.DeadLetterConfig.TargetArn))
		if queueName == "" {
			app.FlashError("could not parse DLQ ARN")
			return nil
		}
		sqsK, ok := kindpkg.Get("sqs")
		if !ok {
			app.FlashError("sqs kind not registered")
			return nil
		}
		sqsK.SetSelection(queueName)
		v, err := sqsK.Build(app)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		return app.SwitchView(sqsK, v)
	}
}

func (k *lambdaKind) configAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no function selected")
			return nil
		}
		out, err := app.APIStore().GetFunction(context.Background(), aws.ToString(k.selected.FunctionName))
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		// Render the full config as JSON; fall back to %+v if marshalling fails
		// (e.g. an unexpected non-marshalable field).
		var body string
		if data, mErr := json.MarshalIndent(out.Configuration, "", "  "); mErr == nil {
			body = string(data)
		} else {
			app.FlashError("config marshal failed: " + mErr.Error())
			body = fmt.Sprintf("%+v", out.Configuration)
		}
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.FunctionName) + " config ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&pseudoKind{name: "config:" + aws.ToString(k.selected.FunctionName)}, newTextSubView(app, flex))
	}
}

// Preload satisfies kindpkg.Preloader. Fired in a goroutine on app start so
// the first `:lambda` is instant. Safe to call concurrently with Build —
// loadInventory uses RWMutex.
func (k *lambdaKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app)
}

// loadInventory fetches the function list once and caches the result.
// Concurrent callers single-flight on k.loadDone — the first caller runs
// the fetch and closes the channel; subsequent callers (including Preload
// + a fast `:lambda`) block on the channel and read the shared result.
// After Reset() the cycle restarts.
func (k *lambdaKind) loadInventory(app kindpkg.App) error {
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

	fns, err := app.APIStore().ListFunctions(context.Background())

	k.mu.Lock()
	k.loadErr = err
	if err == nil {
		k.fns = fns
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

func (k *lambdaKind) buildTable() *tview.Table {
	k.mu.RLock()
	fns := k.fns
	k.mu.RUnlock()

	table := tview.NewTable().SetBorders(false)
	headers := []string{"Name", "Runtime", "Memory", "Timeout", "LastModified", "State"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, fn := range fns {
		copyFn := fn
		cells := []string{
			aws.ToString(fn.FunctionName),
			string(fn.Runtime),
			fmt.Sprintf("%d", aws.ToInt32(fn.MemorySize)),
			fmt.Sprintf("%ds", aws.ToInt32(fn.Timeout)),
			aws.ToString(fn.LastModified),
			string(fn.State),
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(&copyFn)
			}
			table.SetCell(row+1, col, cell)
		}
	}
	return table
}

func (k *lambdaKind) Build(app kindpkg.App) (kindpkg.View, error) {
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

// openLogGroupTail opens a read-only log-tail view for the given CloudWatch
// log group. Reuses GetServiceLogs-style flow but for a single named group.
func openLogGroupTail(app kindpkg.App, logGroup string) error {
	// MVP: fetch latest 100 events synchronously, render in a TextView. A
	// follow-up can swap in true tail-by-polling.
	logs, err := app.APIStore().GetLogGroupTail(context.Background(), logGroup, 100)
	if err != nil {
		app.FlashError(err.Error())
		return err
	}
	// Each log line already has a trailing \n via api.logFmt, so strings.Join
	// with an empty separator is the right concatenation.
	tv := tview.NewTextView().SetDynamicColors(true).SetText(strings.Join(logs, ""))
	tv.SetBorder(true).SetTitle(" " + logGroup + " ")
	flex := tview.NewFlex().AddItem(tv, 0, 1, true)
	return app.SwitchView(&pseudoKind{name: "logs:" + logGroup}, newTextSubView(app, flex))
}

// AggregateInfo / SelectionDetail satisfy kindpkg.Informer.
func (k *lambdaKind) AggregateInfo() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if len(k.fns) == 0 {
		return "No functions"
	}
	runtimes := map[string]int{}
	withDLQ := 0
	for _, fn := range k.fns {
		runtimes[string(fn.Runtime)]++
		if fn.DeadLetterConfig != nil && fn.DeadLetterConfig.TargetArn != nil {
			withDLQ++
		}
	}
	// Top 3 runtimes by count, alphabetised within ties for determinism.
	type rc struct {
		name  string
		count int
	}
	rcs := make([]rc, 0, len(runtimes))
	for r, c := range runtimes {
		rcs = append(rcs, rc{r, c})
	}
	sort.Slice(rcs, func(i, j int) bool {
		if rcs[i].count != rcs[j].count {
			return rcs[i].count > rcs[j].count
		}
		return rcs[i].name < rcs[j].name
	})
	limit := len(rcs)
	if limit > 3 {
		limit = 3
	}
	var rtSummary strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			rtSummary.WriteString(", ")
		}
		rtSummary.WriteString(fmt.Sprintf("%s (%d)", rcs[i].name, rcs[i].count))
	}
	return fmt.Sprintf(
		"Functions: %d\nRuntimes: %s\nWith DLQ: %d",
		len(k.fns), rtSummary.String(), withDLQ,
	)
}

func (k *lambdaKind) SelectionDetail() string {
	if k.selected == nil {
		return ""
	}
	fn := k.selected
	dlq := "none"
	if fn.DeadLetterConfig != nil && fn.DeadLetterConfig.TargetArn != nil {
		dlq = parseQueueNameFromArn(aws.ToString(fn.DeadLetterConfig.TargetArn))
		if dlq == "" {
			dlq = aws.ToString(fn.DeadLetterConfig.TargetArn)
		}
	}
	return fmt.Sprintf(
		"Name: %s\nRuntime: %s\nMemory: %d MB\nTimeout: %ds\nState: %s\nDLQ: %s",
		aws.ToString(fn.FunctionName),
		fn.Runtime,
		aws.ToInt32(fn.MemorySize),
		aws.ToInt32(fn.Timeout),
		fn.State,
		dlq,
	)
}
