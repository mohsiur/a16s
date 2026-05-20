package api

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newStoreWithDDB(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return &Store{Config: &cfg, dynamodb: c}
}

func TestListTablesHappyPath(t *testing.T) {
	store := newStoreWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ListTablesOutput{
				TableNames: []string{"users", "events"},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	got, err := store.ListTables(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0] != "users" {
		t.Fatalf("got %v", got)
	}
}

func TestScanFirstPageRespectsLimit(t *testing.T) {
	store := newStoreWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ScanOutput{
				Items: []map[string]ddbTypes.AttributeValue{
					{"pk": &ddbTypes.AttributeValueMemberS{Value: "1"}},
				},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	items, err := store.ScanFirstPage(context.Background(), "users", 25)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
}
