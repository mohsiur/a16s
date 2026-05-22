package view

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"

	"github.com/mohsiur/a16s/internal/api"
)

// newClientsServingFunctions returns a *api.Clients whose Lambda client is
// mocked at the SDK middleware layer. Each ListFunctions call returns the
// next slice from `pages`; calls past len(pages) return an empty list.
func newClientsServingFunctions(t *testing.T, pages [][]lambdaTypes.FunctionConfiguration) *api.Clients {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	calls := 0
	c := lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				if smithymiddleware.GetOperationName(ctx) != "ListFunctions" {
					return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
				}
				idx := calls
				calls++
				var fns []lambdaTypes.FunctionConfiguration
				if idx < len(pages) {
					fns = pages[idx]
				}
				return smithymiddleware.FinalizeOutput{
					Result: &lambda.ListFunctionsOutput{Functions: fns},
				}, smithymiddleware.Metadata{}, nil
			}), smithymiddleware.Before)
		})
	})
	return api.ClientsWithLambdaForTest(cfg, c)
}

// TestLambdaLoadInventoryReloadRefetches asserts the contract that a
// reload-flagged loadInventory bypasses the cache and re-fetches. This is the
// hook that makes `r` and the auto-refresh ticker actually update the Lambda
// table — without it, both fall through to the cached fns from the first
// `:lambda` and the screen never reflects new/removed functions.
func TestLambdaLoadInventoryReloadRefetches(t *testing.T) {
	clients := newClientsServingFunctions(t, [][]lambdaTypes.FunctionConfiguration{
		{{FunctionName: aws.String("first")}},
		{{FunctionName: aws.String("second")}},
	})
	app := &fakeApp{clients: clients}
	k := &lambdaKind{}

	if err := k.loadInventory(app, false); err != nil {
		t.Fatalf("first loadInventory err = %v", err)
	}
	k.mu.RLock()
	got1 := append([]lambdaTypes.FunctionConfiguration(nil), k.fns...)
	k.mu.RUnlock()
	if len(got1) != 1 || aws.ToString(got1[0].FunctionName) != "first" {
		t.Fatalf("after first load: fns = %+v; want [first]", got1)
	}

	if err := k.loadInventory(app, true); err != nil {
		t.Fatalf("reload loadInventory err = %v", err)
	}
	k.mu.RLock()
	got2 := append([]lambdaTypes.FunctionConfiguration(nil), k.fns...)
	k.mu.RUnlock()
	if len(got2) != 1 || aws.ToString(got2[0].FunctionName) != "second" {
		t.Fatalf("after reload: fns = %+v; want [second] (cache was not refreshed)", got2)
	}
}

// TestLambdaLoadInventoryNoReloadUsesCache asserts the inverse: without the
// reload flag, a second loadInventory call must NOT re-hit the SDK. Preload's
// instant-paint contract relies on this.
func TestLambdaLoadInventoryNoReloadUsesCache(t *testing.T) {
	clients := newClientsServingFunctions(t, [][]lambdaTypes.FunctionConfiguration{
		{{FunctionName: aws.String("first")}},
		{{FunctionName: aws.String("second")}}, // should never be reached
	})
	app := &fakeApp{clients: clients}
	k := &lambdaKind{}

	if err := k.loadInventory(app, false); err != nil {
		t.Fatalf("first loadInventory err = %v", err)
	}
	if err := k.loadInventory(app, false); err != nil {
		t.Fatalf("second loadInventory err = %v", err)
	}

	k.mu.RLock()
	got := append([]lambdaTypes.FunctionConfiguration(nil), k.fns...)
	k.mu.RUnlock()
	if len(got) != 1 || aws.ToString(got[0].FunctionName) != "first" {
		t.Fatalf("fns = %+v; want [first] (cache should have served second call)", got)
	}
}
