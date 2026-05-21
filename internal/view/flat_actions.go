package view

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Flat-kind action wiring used by table.handleInputCapture / handleSelected.
// Each action operates on the row currently selected in the legacy `view`'s
// table, mirroring the kindpkg actions but rendering output as legacy chrome
// pages.

// openLambdaLogs tails the selected Lambda function's CloudWatch log group.
// Renders in an aux text page with wrap enabled; `f` suspends the TUI and
// dumps the full log to stdout so the user can copy from terminal scrollback.
func (v *view) openLambdaLogs() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.lambdaFunction == nil {
		v.app.Notice.Warn("no function selected")
		return
	}
	logGroup := "/aws/lambda/" + aws.ToString(selected.lambdaFunction.FunctionName)
	logs, err := v.app.Store.GetLogGroupTail(context.Background(), logGroup, 100)
	if err != nil {
		v.app.Notice.Warn(err.Error())
		return
	}
	v.showAuxLogs(" "+logGroup+" ", strings.Join(logs, ""))
}

// invokeLambda runs InvokeFunction against the selected function and renders
// the result body as an aux page. FunctionError sets an alternate title.
func (v *view) invokeLambda() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.lambdaFunction == nil {
		v.app.Notice.Warn("no function selected")
		return
	}
	name := aws.ToString(selected.lambdaFunction.FunctionName)
	out, err := v.app.Store.InvokeFunction(context.Background(), name, []byte("{}"))
	if err != nil {
		v.app.Notice.Warn(err.Error())
		return
	}
	title := " " + name + " invoke result "
	if out.FunctionError != nil {
		title = " " + name + " invoke result (error) "
	}
	v.showAuxText(title, string(out.Payload))
}

// openLambdaDLQ navigates to the SQS queue page with the function's DLQ
// pre-selected. Mirrors kindpkg lambdaKind.dlqAction but goes through the
// legacy showQueuesPage so chrome stays consistent.
func (v *view) openLambdaDLQ() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.lambdaFunction == nil {
		v.app.Notice.Warn("no function selected")
		return
	}
	fn := selected.lambdaFunction
	if fn.DeadLetterConfig == nil || fn.DeadLetterConfig.TargetArn == nil {
		v.app.Notice.Warn("no DLQ configured")
		return
	}
	queueName := parseQueueNameFromArn(aws.ToString(fn.DeadLetterConfig.TargetArn))
	if queueName == "" {
		v.app.Notice.Warn("could not parse DLQ ARN")
		return
	}
	if sk := getSQSKind(); sk != nil {
		sk.SetSelection(queueName)
	}
	if err := v.app.showPrimaryKindPage(SQSKind, false); err != nil {
		v.app.Notice.Warn(err.Error())
	}
}

// purgeSelectedQueue purges the queue under the cursor. MVP: no confirm modal.
func (v *view) purgeSelectedQueue() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.sqsQueueName == "" {
		v.app.Notice.Warn("no queue selected")
		return
	}
	if err := v.app.Store.PurgeQueue(context.Background(), selected.sqsQueueName); err != nil {
		v.app.Notice.Warn(err.Error())
		return
	}
	v.app.Notice.Info("purged " + queueNameFromURL(selected.sqsQueueName))
}

// sendTestMessageToQueue sends a fixed test payload to the selected queue.
// MVP: hard-coded body — modal input is a follow-up.
func (v *view) sendTestMessageToQueue() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.sqsQueueName == "" {
		v.app.Notice.Warn("no queue selected")
		return
	}
	if err := v.app.Store.SendMessage(context.Background(), selected.sqsQueueName, `{"a16s":"test"}`); err != nil {
		v.app.Notice.Warn(err.Error())
		return
	}
	v.app.Notice.Info("sent test message to " + queueNameFromURL(selected.sqsQueueName))
}

// openSQSMessageBody opens the selected message's body in an aux text page,
// pretty-printed when it parses as JSON.
func (v *view) openSQSMessageBody() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.sqsMessage == nil {
		v.app.Notice.Warn("no message selected")
		return
	}
	body := ""
	if selected.sqsMessage.Body != nil {
		body = *selected.sqsMessage.Body
	}
	display := body
	var pretty []byte
	if pretty, err = prettyJSON(body); err == nil {
		display = string(pretty)
	}
	id := ""
	if selected.sqsMessage.MessageId != nil {
		id = *selected.sqsMessage.MessageId
	}
	v.showAuxText(" message "+id+" ", display)
}

// queryDDBIndex prompts for a partition-key value and runs QueryEquality
// against the selected index, rendering results as a scrollable text dump.
// (Full table rendering for query results is a follow-up; the existing
// DynamoDBScanKind page handles tabular display for scans.)
func (v *view) queryDDBIndex() {
	selected, err := v.getCurrentSelection()
	if err != nil || selected.ddbIndex == nil || v.app.ddbTable == nil {
		v.app.Notice.Warn("no index selected")
		return
	}
	idx := *selected.ddbIndex
	if idx.partitionKey == "" {
		v.app.Notice.Warn("index has no partition key")
		return
	}
	tableName := aws.ToString(v.app.ddbTable.TableName)

	prompt := tview.NewInputField().
		SetLabel(idx.partitionKey + " = ").
		SetFieldWidth(0)
	prompt.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			v.closeAuxPage()
			return
		}
		val := prompt.GetText()
		items, err := v.app.Store.QueryEquality(context.Background(), tableName, idx.name, idx.partitionKey, val, 25)
		if err != nil {
			v.app.Notice.Warn(err.Error())
			v.closeAuxPage()
			return
		}
		title := fmt.Sprintf(" query %s [%s = %q] ", tableName, idx.partitionKey, val)
		if idx.name != "" {
			title = fmt.Sprintf(" query %s/%s [%s = %q] ", tableName, idx.name, idx.partitionKey, val)
		}
		v.showAuxText(title, formatDDBItems(items))
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(prompt, 1, 0, true).
		AddItem(tview.NewTextView().SetText("Enter to run, Esc to cancel"), 1, 0, false)
	v.showAuxPrimitive(" query "+tableName+" ", flex)
}

func prettyJSON(body string) ([]byte, error) {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	return json.MarshalIndent(v, "", "  ")
}

func formatDDBItems(items []map[string]ddbTypes.AttributeValue) string {
	if len(items) == 0 {
		return "(no items)"
	}
	flat := make([]map[string]string, 0, len(items))
	for _, it := range items {
		row := make(map[string]string, len(it))
		for k, v := range it {
			row[k] = ddbAttrToString(v)
		}
		flat = append(flat, row)
	}
	out, err := json.MarshalIndent(flat, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", flat)
	}
	return string(out)
}

const auxPageName = "a16s.aux"

// showAuxText pushes a bordered text page above the main pages stack and
// captures Esc to dismiss. Replaces the simpleKindView/pseudoKind path used
// for transient screens (logs, invoke result, peek body, query results).
func (v *view) showAuxText(title, body string) {
	tv := tview.NewTextView().SetDynamicColors(true).SetText(body).SetScrollable(true)
	tv.SetBorder(true).SetTitle(title)
	v.showAuxPrimitive(title, tv)
}

// showAuxLogs is showAuxText with wrap on and an `f` shortcut that suspends
// the TUI and pipes the full body through `less -R` so users can copy long
// log content using their terminal's normal selection. Falls back to a plain
// stdout dump when less isn't available.
func (v *view) showAuxLogs(title, body string) {
	tv := tview.NewTextView().SetDynamicColors(true).SetText(body).SetScrollable(true).SetWrap(true)
	tv.SetBorder(true).SetTitle(title + "(f: full screen)")
	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'f':
			v.dumpToTerminal(body)
			return nil
		case 'c':
			v.app.copyToClipboard("logs", body)
			return nil
		}
		return event
	})
	v.showAuxPrimitive(title, tv)
}

// dumpToTerminal suspends tview, writes the body to stdout (piped through
// less -R if present so users can scroll/search), then resumes tview. Falls
// back to plain Fprintln when no pager is available.
func (v *view) dumpToTerminal(body string) {
	v.app.Suspend(func() {
		v.app.isSuspended = true
		defer func() { v.app.isSuspended = false }()
		if err := runPager(body); err != nil {
			fmt.Fprintln(os.Stdout, body)
			fmt.Fprintln(os.Stdout, "[press Enter to return]")
			fmt.Fscanln(os.Stdin)
		}
	})
}

// runPager pipes body through $PAGER (or `less -R`) so terminal selection
// works for long log dumps. Returns an error when no pager runs successfully
// — caller falls back to a plain stdout dump.
func runPager(body string) error {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less -R"
	}
	parts := strings.Fields(pager)
	if len(parts) == 0 {
		return fmt.Errorf("empty pager")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// showAuxPrimitive accepts an arbitrary primitive (e.g. an input + status flex)
// and mounts it as the aux page. Useful for the query prompt.
func (v *view) showAuxPrimitive(title string, p tview.Primitive) {
	flex := tview.NewFlex().AddItem(p, 0, 1, true)
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			v.closeAuxPage()
			return nil
		}
		return event
	})
	v.app.Pages.AddPage(auxPageName, flex, true, true)
	v.app.SetFocus(p)
}

// closeAuxPage tears down the aux page and returns focus to the main pages
// stack. Safe to call whether or not the aux page is currently mounted.
func (v *view) closeAuxPage() {
	if v.app.Pages.HasPage(auxPageName) {
		v.app.Pages.RemovePage(auxPageName)
	}
}
