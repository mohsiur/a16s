package api

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newStoreWithLambda(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return &Store{Config: &cfg, lambda: c}
}

func TestListFunctionsHappyPath(t *testing.T) {
	store := newStoreWithLambda(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &lambda.ListFunctionsOutput{
				Functions: []lambdaTypes.FunctionConfiguration{
					{FunctionName: aws.String("auth-handler"), Runtime: lambdaTypes.RuntimeNodejs20x},
				},
			},
		}, smithymiddleware.Metadata{}, nil
	})

	got, err := store.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || aws.ToString(got[0].FunctionName) != "auth-handler" {
		t.Fatalf("got %+v", got)
	}
}

// TestListFunctionsTwoPages exercises the pagination loop in ListFunctions:
// the first response carries a NextMarker, the second does not, and the
// returned slice should contain functions from both pages.
func TestListFunctionsTwoPages(t *testing.T) {
	calls := 0
	store := newStoreWithLambda(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		calls++
		switch calls {
		case 1:
			return smithymiddleware.FinalizeOutput{
				Result: &lambda.ListFunctionsOutput{
					Functions: []lambdaTypes.FunctionConfiguration{
						{FunctionName: aws.String("fn-1")},
					},
					NextMarker: aws.String("p2"),
				},
			}, smithymiddleware.Metadata{}, nil
		case 2:
			return smithymiddleware.FinalizeOutput{
				Result: &lambda.ListFunctionsOutput{
					Functions: []lambdaTypes.FunctionConfiguration{
						{FunctionName: aws.String("fn-2")},
					},
				},
			}, smithymiddleware.Metadata{}, nil
		}
		t.Fatalf("unexpected call %d", calls)
		return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
	})

	got, err := store.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d; want 2 (got: %+v)", len(got), got)
	}
	if aws.ToString(got[0].FunctionName) != "fn-1" || aws.ToString(got[1].FunctionName) != "fn-2" {
		t.Fatalf("unexpected function names: %+v", got)
	}
}

// TestListFunctionsErrorAfterFirstPage asserts the partial-success contract
// from cluster.go: if pagination errors after at least one page has been
// fetched, return what we have with a nil error.
func TestListFunctionsErrorAfterFirstPage(t *testing.T) {
	calls := 0
	store := newStoreWithLambda(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		calls++
		switch calls {
		case 1:
			return smithymiddleware.FinalizeOutput{
				Result: &lambda.ListFunctionsOutput{
					Functions: []lambdaTypes.FunctionConfiguration{
						{FunctionName: aws.String("fn-1")},
					},
					NextMarker: aws.String("p2"),
				},
			}, smithymiddleware.Metadata{}, nil
		case 2:
			return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, errors.New("page-2 boom")
		}
		t.Fatalf("unexpected call %d", calls)
		return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
	})

	got, err := store.ListFunctions(context.Background())
	if err != nil {
		t.Fatalf("err = %v; want nil for partial-page failure", err)
	}
	if len(got) != 1 || aws.ToString(got[0].FunctionName) != "fn-1" {
		t.Fatalf("got %+v; want [fn-1]", got)
	}
}
