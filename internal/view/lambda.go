package view

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kindpkg.Register(&lambdaKind{}) }

type lambdaKind struct {
	selected *lambdaTypes.FunctionConfiguration
}

func (k *lambdaKind) Name() string { return "lambda" }

func (k *lambdaKind) Reset() { k.selected = nil }

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
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(" invoke result ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&logsPseudoKind{name: "invoke:" + aws.ToString(k.selected.FunctionName)}, &simpleKindView{flex: flex, focus: tv})
	}
}

func (k *lambdaKind) dlqAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		// Filled in during Phase 6. For now: flash.
		app.FlashError("DLQ jump not implemented yet")
		return nil
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
		// Render the full config as JSON for now (reuse json.go in a later pass).
		body := fmt.Sprintf("%+v", out.Configuration)
		tv := tview.NewTextView().SetText(body)
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.FunctionName) + " config ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(&logsPseudoKind{name: "config:" + aws.ToString(k.selected.FunctionName)}, &simpleKindView{flex: flex, focus: tv})
	}
}

func (k *lambdaKind) Build(app kindpkg.App) (kindpkg.View, error) {
	fns, err := app.APIStore().ListFunctions(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
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
				cell.SetReference(&copyFn) // picked up by tableSelectionForActiveKind
			}
			table.SetCell(row+1, col, cell)
		}
	}

	flex := tview.NewFlex().AddItem(table, 0, 1, true)
	view := &simpleKindView{flex: flex, focus: table, app: app, source: k}
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if view.OnKey(event) {
			return nil
		}
		return event
	})
	return view, nil
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
	tv := tview.NewTextView().SetDynamicColors(true).SetText(joinLines(logs))
	tv.SetBorder(true).SetTitle(" " + logGroup + " ")
	flex := tview.NewFlex().AddItem(tv, 0, 1, true)
	return app.SwitchView(&logsPseudoKind{name: logGroup}, &simpleKindView{flex: flex, focus: tv})
}

func joinLines(in []string) string {
	out := ""
	for _, s := range in {
		out += s + "\n"
	}
	return out
}

// logsPseudoKind is a transient kind for the log-tail screen. Not registered.
type logsPseudoKind struct{ name string }

func (l *logsPseudoKind) Name() string                            { return "logs:" + l.name }
func (l *logsPseudoKind) Build(kindpkg.App) (kindpkg.View, error) { return nil, nil }
func (l *logsPseudoKind) Reset()                                  {}
func (l *logsPseudoKind) Selection() any                          { return nil }
func (l *logsPseudoKind) SetSelection(any)                        {}
func (l *logsPseudoKind) Breadcrumb() string                      { return l.name }
func (l *logsPseudoKind) PrimaryAction() kindpkg.Action           { return nil }
func (l *logsPseudoKind) SecondaryActions() []kindpkg.Binding     { return nil }
