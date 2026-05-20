package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (store *Store) ListQueues(ctx context.Context) ([]string, error) {
	store.initSqsClient()
	slog.Debug("api ListQueues")
	out, err := store.sqs.ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		slog.Error("ListQueues failed", "error", err)
		return nil, err
	}
	return out.QueueUrls, nil
}

func (store *Store) GetQueueAttributes(ctx context.Context, queueURL string) (map[string]string, error) {
	store.initSqsClient()
	out, err := store.sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURL,
		AttributeNames: []sqsTypes.QueueAttributeName{sqsTypes.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

// PeekMessages reads up to 10 messages with VisibilityTimeout=0 so real
// consumers are not affected.
func (store *Store) PeekMessages(ctx context.Context, queueURL string) ([]sqsTypes.Message, error) {
	store.initSqsClient()
	out, err := store.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &queueURL,
		MaxNumberOfMessages: 10,
		VisibilityTimeout:   0,
		WaitTimeSeconds:     0,
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (store *Store) SendMessage(ctx context.Context, queueURL, body string) error {
	store.initSqsClient()
	_, err := store.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(body),
	})
	return err
}

func (store *Store) PurgeQueue(ctx context.Context, queueURL string) error {
	store.initSqsClient()
	_, err := store.sqs.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: &queueURL})
	return err
}
