package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (c *Clients) ListQueues(ctx context.Context) ([]string, error) {
	slog.Debug("api ListQueues")
	out, err := c.SQS().ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		slog.Error("ListQueues failed", "error", err)
		return nil, err
	}
	return out.QueueUrls, nil
}

func (c *Clients) GetQueueAttributes(ctx context.Context, queueURL string) (map[string]string, error) {
	out, err := c.SQS().GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
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
func (c *Clients) PeekMessages(ctx context.Context, queueURL string) ([]sqsTypes.Message, error) {
	out, err := c.SQS().ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
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

func (c *Clients) SendMessage(ctx context.Context, queueURL, body string) error {
	_, err := c.SQS().SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(body),
	})
	return err
}

func (c *Clients) PurgeQueue(ctx context.Context, queueURL string) error {
	_, err := c.SQS().PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: &queueURL})
	return err
}

