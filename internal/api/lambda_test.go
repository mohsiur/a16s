package api

import (
	"context"
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
