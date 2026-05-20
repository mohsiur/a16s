package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
)

func TestLambdaKindName(t *testing.T) {
	k := &lambdaKind{}
	if k.Name() != "lambda" {
		t.Fatalf("Name = %q; want %q", k.Name(), "lambda")
	}
}

func TestLambdaKindSelectionRoundTrip(t *testing.T) {
	k := &lambdaKind{}
	fn := &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("auth-handler")}
	k.SetSelection(fn)
	if k.Selection() != fn {
		t.Fatalf("Selection round-trip failed")
	}
}

func TestLambdaKindResetClearsSelection(t *testing.T) {
	k := &lambdaKind{}
	k.SetSelection(&lambdaTypes.FunctionConfiguration{FunctionName: aws.String("x")})
	k.Reset()
	if k.Selection() != nil {
		t.Fatalf("Selection after Reset = %v; want nil", k.Selection())
	}
}

func TestLambdaKindBreadcrumb(t *testing.T) {
	k := &lambdaKind{}
	if got := k.Breadcrumb(); got != "lambda" {
		t.Fatalf("Breadcrumb (no selection) = %q; want %q", got, "lambda")
	}
	k.SetSelection(&lambdaTypes.FunctionConfiguration{FunctionName: aws.String("auth-handler")})
	if got := k.Breadcrumb(); got != "lambda > auth-handler" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "lambda > auth-handler")
	}
}

func TestLambdaKindRegistered(t *testing.T) {
	k, ok := kindpkg.Get("lambda")
	if !ok {
		t.Fatal("lambda kind not registered")
	}
	if _, ok := k.(*lambdaKind); !ok {
		t.Fatalf("registered kind is %T; want *lambdaKind", k)
	}
}

func TestLambdaKindSecondaryActions(t *testing.T) {
	k := &lambdaKind{}
	got := k.SecondaryActions()
	if len(got) != 3 {
		t.Fatalf("len(SecondaryActions) = %d; want 3", len(got))
	}
	wantKeys := map[rune]string{'i': "invoke", 'd': "dlq", 'c': "config"}
	for _, b := range got {
		if wantKeys[b.Key] != b.Label {
			t.Fatalf("binding %c => %q; want %q", b.Key, b.Label, wantKeys[b.Key])
		}
	}
}
