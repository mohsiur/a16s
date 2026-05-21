package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func newTestSQSView(t *testing.T) *sqsView {
	t.Helper()
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	app.kind = SQSKind
	urls := []string{
		"https://sqs.us-east-1.amazonaws.com/111/orders",
		"https://sqs.us-east-1.amazonaws.com/111/orders-dlq",
	}
	attrs := map[string]map[string]string{
		urls[0]: {
			"ApproximateNumberOfMessages":           "12",
			"ApproximateNumberOfMessagesNotVisible": "3",
			"ApproximateNumberOfMessagesDelayed":    "0",
			"RedrivePolicy":                         `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:111:orders-dlq"}`,
			"VisibilityTimeout":                     "30",
		},
		urls[1]: {
			"ApproximateNumberOfMessages":           "0",
			"ApproximateNumberOfMessagesNotVisible": "0",
			"ApproximateNumberOfMessagesDelayed":    "0",
		},
	}
	return newSQSView(urls, attrs, app)
}

func TestSQSViewHeaderPageItems(t *testing.T) {
	v := newTestSQSView(t)
	items := v.headerPageItems(0)
	got := map[string]string{}
	for _, it := range items {
		got[it.name] = it.value
	}
	if got["Name"] != "orders" {
		t.Errorf("Name = %q; want %q", got["Name"], "orders")
	}
	if got["Messages"] != "12" {
		t.Errorf("Messages = %q; want %q", got["Messages"], "12")
	}
	if got["DLQ"] != "yes" {
		t.Errorf("DLQ = %q; want %q", got["DLQ"], "yes")
	}
}

func TestSQSViewTableParamsBuilder(t *testing.T) {
	v := newTestSQSView(t)
	_, headers, rowsBuilder := v.tableParamsBuilder()
	if headers[0] != "Name" || headers[4] != "DLQ" {
		t.Fatalf("headers = %v", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 2 {
		t.Fatalf("rows = %d; want 2", len(rows))
	}
	if rows[0][0] != "orders" || rows[0][1] != "12" || rows[0][4] != "yes" {
		t.Errorf("row 0 = %v", rows[0])
	}
	if rows[1][0] != "orders-dlq" || rows[1][4] != "no" {
		t.Errorf("row 1 = %v", rows[1])
	}
	if len(v.originalRowReferences) != 2 {
		t.Fatalf("rowRefs = %d; want 2", len(v.originalRowReferences))
	}
	if v.originalRowReferences[0].sqsQueueName == "" {
		t.Errorf("sqsQueueName not wired")
	}
}

func newTestSQSPeekView(t *testing.T) *sqsPeekView {
	t.Helper()
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	app.kind = SQSPeekKind
	msgs := []sqsTypes.Message{
		{
			MessageId: aws.String("m-1"),
			Body:      aws.String(`{"event":"order.created"}`),
			Attributes: map[string]string{
				"SentTimestamp": "1700000000000",
			},
		},
	}
	return newSQSPeekView("https://sqs.us-east-1.amazonaws.com/111/orders", msgs, app)
}

func TestSQSPeekViewTableParamsBuilder(t *testing.T) {
	v := newTestSQSPeekView(t)
	_, headers, rowsBuilder := v.tableParamsBuilder()
	if len(headers) != 4 || headers[0] != "MessageId" {
		t.Fatalf("headers = %v", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	if rows[0][0] != "m-1" {
		t.Errorf("MessageId = %q; want m-1", rows[0][0])
	}
	if len(v.originalRowReferences) != 1 || v.originalRowReferences[0].sqsMessage == nil {
		t.Errorf("sqsMessage reference not wired: %+v", v.originalRowReferences)
	}
}
