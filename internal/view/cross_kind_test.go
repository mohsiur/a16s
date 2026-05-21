package view

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	smithymiddleware "github.com/aws/smithy-go/middleware"

	"github.com/mohsiur/a16s/internal/api"
)

func newStoreServingQueues(t *testing.T, queueURLs []string) *api.Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				switch smithymiddleware.GetOperationName(ctx) {
				case "ListQueues":
					return smithymiddleware.FinalizeOutput{
						Result: &sqs.ListQueuesOutput{QueueUrls: queueURLs},
					}, smithymiddleware.Metadata{}, nil
				case "GetQueueAttributes":
					return smithymiddleware.FinalizeOutput{
						Result: &sqs.GetQueueAttributesOutput{Attributes: map[string]string{}},
					}, smithymiddleware.Metadata{}, nil
				default:
					return smithymiddleware.FinalizeOutput{}, smithymiddleware.Metadata{}, nil
				}
			}), smithymiddleware.Before)
		})
	})
	return api.StoreWithSqsForTest(&cfg, c)
}

// TestCrossKindLambdaToDLQ asserts the cross-kind contract: when Lambda's DLQ
// action sets a bare queue name on sqsKind, showQueuesPage's promotion logic
// upgrades that bare name to the full URL from cached inventory. The legacy
// chrome (showPrimaryKindPage(SQSKind)) needs the full URL because all the
// per-row references key off it.
func TestCrossKindLambdaToDLQ(t *testing.T) {
	fullURL := "https://sqs.us-east-1.amazonaws.com/111/my-dlq"
	store := newStoreServingQueues(t, []string{fullURL})

	sk := &sqsKind{}
	app := &fakeApp{store: store}
	if err := sk.loadInventory(app); err != nil {
		t.Fatalf("loadInventory err = %v", err)
	}

	// Lambda's openLambdaDLQ does this: SetSelection(bareName) on sqsKind.
	sk.SetSelection("my-dlq")

	// showQueuesPage calls promoteSelectedURL to upgrade bare-name to full URL.
	got := sk.promoteSelectedURL()
	if got != fullURL {
		t.Fatalf("promoteSelectedURL = %q; want %q", got, fullURL)
	}
}
