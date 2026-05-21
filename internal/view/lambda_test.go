package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
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

func TestLambdaKindRegistered(t *testing.T) {
	k, ok := kindpkg.Get("lambda")
	if !ok {
		t.Fatal("lambda kind not registered")
	}
	if _, ok := k.(*lambdaKind); !ok {
		t.Fatalf("registered kind is %T; want *lambdaKind", k)
	}
}

