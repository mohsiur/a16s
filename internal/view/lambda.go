package view

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// lambdaKind is the cross-process inventory cache for Lambda. It implements
// kindpkg.Kind so the palette/preload registry can find it, but the actual
// rendering goes through showLambdasPage + the legacy ECS chrome.
func init() {
	kindpkg.Register(&lambdaKind{})
	bindKind(LambdaKind, "lambda", "lambdas")
}

type lambdaKind struct {
	kindpkg.BaseKind
	selected *lambdaTypes.FunctionConfiguration
	mu       sync.RWMutex
	fns      []lambdaTypes.FunctionConfiguration
	loaded   bool
	loadDone chan struct{}
	loadErr  error
}

func (k *lambdaKind) Name() string  { return "lambda" }
func (k *lambdaKind) Title() string { return "lambdas" }

func (k *lambdaKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showLambdasPage(reload)
	}
	return nil
}

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

// BrowserURL returns the AWS console URL for the function under the cursor.
// The selection passed in is the row reference picked up by openInBrowser;
// it falls back to k.selected when nil so this also works for callers that
// route through the cached selection.
func (k *lambdaKind) BrowserURL(region string) (string, error) {
	fn := k.selected
	if fn == nil || fn.FunctionName == nil {
		return "", nil
	}
	return utils.LambdaFunctionURL(region, aws.ToString(fn.FunctionName)), nil
}

// FooterItem describes the lambda kind's footer summary cell.
func (k *lambdaKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "lambdas"}
}

// Traits flag the affordances Lambda opts into.
func (k *lambdaKind) Traits() kindpkg.Traits {
	return kindpkg.Traits{
		Filterable:  true,
		Refreshable: true,
		Browsable:   true,
		WideTable:   true,
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

func (k *lambdaKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app, false)
}

// loadInventory fetches the function list and caches the result. Concurrent
// callers single-flight on k.loadDone. When reload is true, the cache is
// invalidated before the fetch so refresh keys (`r`) and the auto-refresh
// ticker actually re-hit the AWS API; selection state is preserved.
func (k *lambdaKind) loadInventory(app kindpkg.App, reload bool) error {
	k.mu.Lock()
	if reload {
		k.loaded = false
		k.fns = nil
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

	fns, err := app.AWSClients().ListFunctions(context.Background())

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
		hotKeyMap["i"],
		hotKeyMap["D"],
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
		if err := lk.loadInventory(app, reload); err != nil {
			return err
		}
		lk.mu.RLock()
		fns := append([]lambdaTypes.FunctionConfiguration(nil), lk.fns...)
		lk.mu.RUnlock()
		return buildResourcePage(fns, app, nil, func() resourceViewBuilder {
			return newLambdaView(fns, app)
		})
	}
	fns, err := app.Clients.ListFunctions(context.Background())
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
	return &v.view, v.footer.middle
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
