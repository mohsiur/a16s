package view

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
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
	return nil, nil // implemented in next task
}
