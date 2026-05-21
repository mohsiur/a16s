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
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// lambdaKind is retained as the cross-process inventory cache + secondary
// action source. It still implements kindpkg.Kind so tests and the existing
// kindpkg layer keep compiling, but the palette now dispatches through
// showLambdasPage instead of going through kindpkg.Build.
func init() { kindpkg.Register(&lambdaKind{}) }

type lambdaKind struct {
	selected *lambdaTypes.FunctionConfiguration
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

func (k *lambdaKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app)
}

// loadInventory fetches the function list once and caches the result.
// Concurrent callers single-flight on k.loadDone.
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
		k.loadDone = nil
	}
	k.mu.Unlock()
	close(done)
	return err
}

func (k *lambdaKind) buildLegacyTable() *tview.Table {
	k.mu.RLock()
	fns := k.fns
	k.mu.RUnlock()

	table := tview.NewTable().SetBorders(false)
	headers := []string{"Name", "Runtime", "Memory", "Timeout", "LastModified", "State"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false))
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
		return newTableKindView(app, k, k.buildLegacyTable()), nil
	}
	return newLoadingTableKindView(app, k, func() error {
		return k.loadInventory(app)
	}, k.buildLegacyTable), nil
}

// openLogGroupTail opens a read-only log-tail view for the given CloudWatch
// log group.
func openLogGroupTail(app kindpkg.App, logGroup string) error {
	logs, err := app.APIStore().GetLogGroupTail(context.Background(), logGroup, 100)
	if err != nil {
		app.FlashError(err.Error())
		return err
	}
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

// ---- Legacy-style ECS chrome integration ----

// lambdaView wraps the lambdaKind cache as a resourceViewBuilder so the page
// uses the same buildHeaderFlex + buildTable + footer chrome as ClusterKind.
type lambdaView struct {
	view
	fns []lambdaTypes.FunctionConfiguration
}

func newLambdaView(fns []lambdaTypes.FunctionConfiguration, app *App) *lambdaView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["L"],
	}...)
	return &lambdaView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		fns: fns,
	}
}

// showLambdasPage is the LambdaKind entry point reachable from
// showPrimaryKindPage. It uses the legacy buildResourcePage flow so chrome
// matches ECS pages exactly.
func (app *App) showLambdasPage(reload bool) error {
	app.kind = LambdaKind
	if switched := app.switchPage(reload); switched {
		return nil
	}
	// Reuse lambdaKind's cache when available so first paint after a `:lambda`
	// is instant. Otherwise list synchronously — buildResourcePage assumes the
	// data is already in hand.
	lk := getLambdaKind()
	if lk != nil {
		if err := lk.loadInventory(app); err != nil {
			return err
		}
		lk.mu.RLock()
		fns := append([]lambdaTypes.FunctionConfiguration(nil), lk.fns...)
		lk.mu.RUnlock()
		return buildResourcePage(fns, app, nil, func() resourceViewBuilder {
			return newLambdaView(fns, app)
		})
	}
	fns, err := app.Store.ListFunctions(context.Background())
	return buildResourcePage(fns, app, err, func() resourceViewBuilder {
		return newLambdaView(fns, app)
	})
}

// getLambdaKind retrieves the registered lambdaKind cache. Returns nil if the
// kind isn't in the registry (shouldn't happen given init() above, but the
// fallback keeps the page resilient to registry changes).
func getLambdaKind() *lambdaKind {
	k, ok := kindpkg.Get("lambda")
	if !ok {
		return nil
	}
	lk, _ := k.(*lambdaKind)
	return lk
}

func (v *lambdaView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.lambda
}

func (v *lambdaView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.fns))
	for i, fn := range v.fns {
		params = append(params, headerPageParam{
			title:      aws.ToString(fn.FunctionName),
			entityName: aws.ToString(fn.FunctionArn),
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *lambdaView) headerPageItems(index int) []headerItem {
	fn := v.fns[index]
	dlq := "none"
	if fn.DeadLetterConfig != nil && fn.DeadLetterConfig.TargetArn != nil {
		name := parseQueueNameFromArn(aws.ToString(fn.DeadLetterConfig.TargetArn))
		if name == "" {
			dlq = aws.ToString(fn.DeadLetterConfig.TargetArn)
		} else {
			dlq = name
		}
	}
	return []headerItem{
		{name: "Name", value: aws.ToString(fn.FunctionName)},
		{name: "Runtime", value: string(fn.Runtime)},
		{name: "Memory", value: fmt.Sprintf("%d MB", aws.ToInt32(fn.MemorySize))},
		{name: "Timeout", value: fmt.Sprintf("%ds", aws.ToInt32(fn.Timeout))},
		{name: "State", value: string(fn.State)},
		{name: "Last modified", value: aws.ToString(fn.LastModified)},
		{name: "Handler", value: aws.ToString(fn.Handler)},
		{name: "Role", value: aws.ToString(fn.Role)},
		{name: "DLQ", value: dlq},
		{name: "Architecture", value: archList(fn.Architectures)},
	}
}

func archList(arches []lambdaTypes.Architecture) string {
	if len(arches) == 0 {
		return utils.EmptyText
	}
	out := make([]string, 0, len(arches))
	for _, a := range arches {
		out = append(out, string(a))
	}
	return strings.Join(out, ",")
}

func (v *lambdaView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, "all", len(v.fns))
	headers = []string{
		"Name",
		"Runtime",
		"Memory",
		"Timeout",
		"State",
		"LastModified",
	}
	rowsBuilder = func() (data [][]string) {
		for _, fn := range v.fns {
			row := []string{
				aws.ToString(fn.FunctionName),
				string(fn.Runtime),
				fmt.Sprintf("%d", aws.ToInt32(fn.MemorySize)),
				fmt.Sprintf("%ds", aws.ToInt32(fn.Timeout)),
				utils.ShowGreenGrey(stringPtr(string(fn.State)), "Active"),
				aws.ToString(fn.LastModified),
			}
			data = append(data, row)
			copyFn := fn
			entity := Entity{
				lambdaFunction: &copyFn,
				entityName:     aws.ToString(fn.FunctionArn),
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}

func stringPtr(s string) *string { return &s }
