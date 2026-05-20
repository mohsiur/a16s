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

func (k *lambdaKind) PrimaryAction() kindpkg.Action { return nil } // wired in next task

func (k *lambdaKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'i', Label: "invoke", Run: nil}, // wired in next task
		{Key: 'd', Label: "dlq", Run: nil},
		{Key: 'c', Label: "config", Run: nil},
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
	return &simpleKindView{flex: flex, focus: table}, nil
}
