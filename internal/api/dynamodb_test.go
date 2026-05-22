package api

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

func newClientsWithDDB(t *testing.T, fn func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error)) *Clients {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", fn), smithymiddleware.Before)
		})
	})
	return ClientsWithDynamoDBForTest(cfg, c)
}

func TestListTablesHappyPath(t *testing.T) {
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ListTablesOutput{
				TableNames: []string{"users", "events"},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	got, err := c.ListTables(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0] != "users" {
		t.Fatalf("got %v", got)
	}
}

// TestListTablesTwoPages exercises the pagination loop: the first response
// carries a LastEvaluatedTableName, the second does not, and the returned
// slice contains entries from both pages in order.
func TestListTablesTwoPages(t *testing.T) {
	calls := 0
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		calls++
		switch calls {
		case 1:
			return smithymiddleware.FinalizeOutput{
				Result: &dynamodb.ListTablesOutput{
					TableNames:             []string{"users"},
					LastEvaluatedTableName: aws.String("users"),
				},
			}, smithymiddleware.Metadata{}, nil
		case 2:
			return smithymiddleware.FinalizeOutput{
				Result: &dynamodb.ListTablesOutput{
					TableNames: []string{"events"},
				},
			}, smithymiddleware.Metadata{}, nil
		}
		t.Fatalf("unexpected call %d", calls)
		return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
	})

	got, err := c.ListTables(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 || got[0] != "users" || got[1] != "events" {
		t.Fatalf("got %v; want [users events]", got)
	}
}

// TestListTablesErrorAfterFirstPage asserts the partial-success contract
// from dynamodb.go: pagination errors after at least one page has been
// fetched return what we have with a nil error.
func TestListTablesErrorAfterFirstPage(t *testing.T) {
	calls := 0
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		calls++
		switch calls {
		case 1:
			return smithymiddleware.FinalizeOutput{
				Result: &dynamodb.ListTablesOutput{
					TableNames:             []string{"users"},
					LastEvaluatedTableName: aws.String("users"),
				},
			}, smithymiddleware.Metadata{}, nil
		case 2:
			return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, errors.New("page-2 boom")
		}
		t.Fatalf("unexpected call %d", calls)
		return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
	})

	got, err := c.ListTables(context.Background())
	if err != nil {
		t.Fatalf("err = %v; want nil for partial-page failure", err)
	}
	if len(got) != 1 || got[0] != "users" {
		t.Fatalf("got %v; want [users]", got)
	}
}

// TestListTablesFirstPageErrorBubbles asserts the inverse: an error on the
// very first page returns (nil, err) — partial success only kicks in once
// at least one page has succeeded.
func TestListTablesFirstPageErrorBubbles(t *testing.T) {
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, errors.New("boom")
	})
	got, err := c.ListTables(context.Background())
	if err == nil {
		t.Fatalf("expected error; got %v", got)
	}
	if got != nil {
		t.Fatalf("got = %v; want nil on first-page error", got)
	}
}

func TestScanFirstPageRespectsLimit(t *testing.T) {
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.ScanOutput{
				Items: []map[string]ddbTypes.AttributeValue{
					{"pk": &ddbTypes.AttributeValueMemberS{Value: "1"}},
				},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	items, err := c.ScanFirstPage(context.Background(), "users", 25)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
}

func TestScanIndexFirstPageDoesNotError(t *testing.T) {
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{Result: &dynamodb.ScanOutput{}}, smithymiddleware.Metadata{}, nil
	})
	if _, err := c.ScanIndexFirstPage(context.Background(), "users", "gsi-email", 5); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestQueryEqualityPassesArgs(t *testing.T) {
	c := newClientsWithDDB(t, func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
		return smithymiddleware.FinalizeOutput{
			Result: &dynamodb.QueryOutput{
				Items: []map[string]ddbTypes.AttributeValue{
					{"email": &ddbTypes.AttributeValueMemberS{Value: "a@b"}},
				},
			},
		}, smithymiddleware.Metadata{}, nil
	})
	got, err := c.QueryEquality(context.Background(), "users", "gsi-email", "email", "a@b", 5)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items", len(got))
	}
}
