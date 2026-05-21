package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (store *Store) ListQueues(ctx context.Context) ([]string, error) {
	c := store.initSqsClient()
	slog.Debug("api ListQueues")
	out, err := c.ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		slog.Error("ListQueues failed", "error", err)
		return nil, err
	}
	return out.QueueUrls, nil
}

func (store *Store) GetQueueAttributes(ctx context.Context, queueURL string) (map[string]string, error) {
	c := store.initSqsClient()
	out, err := c.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURL,
		AttributeNames: []sqsTypes.QueueAttributeName{sqsTypes.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

// PeekMessages reads up to 10 messages with VisibilityTimeout=0 so real
// consumers are not affected. MessageSystemAttributeNameAll asks SQS to
// include the SentTimestamp/SenderId/etc attributes — without it the peek
// table can't render the "Sent" age column.
func (store *Store) PeekMessages(ctx context.Context, queueURL string) ([]sqsTypes.Message, error) {
	c := store.initSqsClient()
	out, err := c.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    &queueURL,
		MaxNumberOfMessages:         10,
		VisibilityTimeout:           0,
		WaitTimeSeconds:             0,
		MessageSystemAttributeNames: []sqsTypes.MessageSystemAttributeName{sqsTypes.MessageSystemAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (store *Store) SendMessage(ctx context.Context, queueURL, body string) error {
	c := store.initSqsClient()
	_, err := c.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(body),
	})
	return err
}

func (store *Store) PurgeQueue(ctx context.Context, queueURL string) error {
	c := store.initSqsClient()
	_, err := c.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: &queueURL})
	return err
}

// StoreWithSqsForTest constructs a Store with a pre-configured SQS client.
// Used by tests in any package that need to mock SQS at the SDK middleware
// layer; the Store.sqs field is unexported so this is the only safe entry
// point.
func StoreWithSqsForTest(cfg *aws.Config, c *sqs.Client) *Store {
	return &Store{Config: cfg, sqs: c}
}
