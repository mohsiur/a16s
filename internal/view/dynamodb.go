package view

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kindpkg.Register(&ddbKind{}) }

type ddbKind struct {
	selected *ddbTypes.TableDescription
}

func (k *ddbKind) Name() string { return "ddb" }
func (k *ddbKind) Reset()       { k.selected = nil }

func (k *ddbKind) Selection() any {
	if k.selected == nil {
		return nil
	}
	return k.selected
}

func (k *ddbKind) SetSelection(s any) {
	if td, ok := s.(*ddbTypes.TableDescription); ok {
		k.selected = td
	}
}

func (k *ddbKind) Breadcrumb() string {
	if k.selected == nil || k.selected.TableName == nil {
		return "ddb"
	}
	return "ddb > " + aws.ToString(k.selected.TableName)
}

func (k *ddbKind) PrimaryAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		items, err := app.APIStore().ScanFirstPage(context.Background(), aws.ToString(k.selected.TableName), 25)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		body, _ := json.MarshalIndent(items, "", "  ")
		tv := tview.NewTextView().SetText(string(body))
		tv.SetBorder(true).SetTitle(" scan " + aws.ToString(k.selected.TableName) + " (first 25) ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&pseudoKind{name: "scan:" + aws.ToString(k.selected.TableName)},
			&simpleKindView{flex: flex, app: app, source: k},
		)
	}
}

func (k *ddbKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'c', Label: "describe", Run: k.describeAction()},
	}
}

func (k *ddbKind) describeAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		body, _ := json.MarshalIndent(k.selected, "", "  ")
		tv := tview.NewTextView().SetText(string(body))
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.TableName) + " describe ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&pseudoKind{name: "describe:" + aws.ToString(k.selected.TableName)},
			&simpleKindView{flex: flex, app: app, source: k},
		)
	}
}

func (k *ddbKind) Build(app kindpkg.App) (kindpkg.View, error) {
	names, err := app.APIStore().ListTables(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false)
	headers := []string{"TableName", "Status", "ItemCount", "SizeBytes", "BillingMode", "Streams"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, name := range names {
		td, derr := app.APIStore().DescribeTable(context.Background(), name)
		if derr != nil {
			// Skip rows we can't describe — log via flash and continue. Other
			// rows are still useful.
			app.FlashError("describe " + name + ": " + derr.Error())
			continue
		}
		billing := ""
		if td.BillingModeSummary != nil {
			billing = string(td.BillingModeSummary.BillingMode)
		}
		streams := "no"
		if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
			streams = "yes"
		}
		cells := []string{
			aws.ToString(td.TableName),
			string(td.TableStatus),
			fmt.Sprintf("%d", aws.ToInt64(td.ItemCount)),
			fmt.Sprintf("%d", aws.ToInt64(td.TableSizeBytes)),
			billing,
			streams,
		}
		copyTD := td
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyTD)
			}
			table.SetCell(row+1, col, cell)
		}
	}

	return newTableKindView(app, k, table), nil
}
