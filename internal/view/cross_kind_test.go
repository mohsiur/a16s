package view

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	smithymiddleware "github.com/aws/smithy-go/middleware"

	"github.com/keidarcy/e1s/internal/api"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
)

func newStoreServingQueues(t *testing.T, queueURLs []string) *api.Store {
	t.Helper()
	cfg := aws.Config{Region: "us-east-1"}
	c := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *smithymiddleware.Stack) error {
			return stack.Finalize.Add(smithymiddleware.FinalizeMiddlewareFunc("mock", func(ctx context.Context, in smithymiddleware.FinalizeInput, next smithymiddleware.FinalizeHandler) (smithymiddleware.FinalizeOutput, smithymiddleware.Metadata, error) {
				// Type-discriminate by SDK operation name. The SDK does an
				// unconditional type assertion on the result, so a single
				// catch-all output panics on the second op (e.g.
				// GetQueueAttributes after ListQueues). Return the matching
				// output type per op, with empty payloads for ops we don't
				// care about — pre-selection only reads the row's
				// Reference (the queue URL), not the rendered cell text.
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

func TestCrossKindLambdaToDLQ(t *testing.T) {
	fullURL := "https://sqs.us-east-1.amazonaws.com/111/my-dlq"
	store := newStoreServingQueues(t, []string{fullURL})
	app := &fakeApp{store: store}

	// Reset the registered kinds so prior-test state doesn't bleed in.
	if k, ok := kindpkg.Get("lambda"); ok {
		k.Reset()
	}
	if k, ok := kindpkg.Get("sqs"); ok {
		k.Reset()
	}

	// Use the registered lambdaKind so dlqAction's closure reads the right
	// selected function.
	lk, ok := kindpkg.Get("lambda")
	if !ok {
		t.Fatal("lambda kind not registered")
	}
	concrete := lk.(*lambdaKind)
	concrete.selected = &lambdaTypes.FunctionConfiguration{
		FunctionName: aws.String("auth-handler"),
		DeadLetterConfig: &lambdaTypes.DeadLetterConfig{
			TargetArn: aws.String("arn:aws:sqs:us-east-1:111:my-dlq"),
		},
	}
	// Restore lambda + sqs state after the test so other tests aren't affected.
	t.Cleanup(func() {
		concrete.selected = nil
		if k, ok := kindpkg.Get("sqs"); ok {
			k.Reset()
		}
	})

	if err := concrete.dlqAction()(app); err != nil {
		t.Fatalf("dlqAction err = %v", err)
	}

	if app.switchedTo == nil || app.switchedTo.Name() != "sqs" {
		t.Fatalf("expected SwitchView to sqs; got %v", app.switchedTo)
	}

	sk, _ := kindpkg.Get("sqs")
	got, _ := sk.Selection().(string)
	if got != fullURL {
		t.Fatalf("sqs selection = %q; want %q", got, fullURL)
	}
}
