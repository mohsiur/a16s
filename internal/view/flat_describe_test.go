package view

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func TestFlatKindDescribeJsonString(t *testing.T) {
	app, _ := newApp(Option{})
	v := newView(app, nil, secondaryPageKeyMap{})

	fn := &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("auth-handler")}
	fnBytes, _ := json.MarshalIndent(fn, "", "  ")

	td := &ddbTypes.TableDescription{TableName: aws.String("Users")}
	tdBytes, _ := json.MarshalIndent(td, "", "  ")

	msg := &sqsTypes.Message{MessageId: aws.String("m-1"), Body: aws.String(`{"x":1}`)}
	msgBytes, _ := json.MarshalIndent(msg, "", "  ")

	cases := []struct {
		name       string
		setup      func()
		entity     Entity
		wantSubstr string
	}{
		{
			name: "lambda",
			setup: func() {
				app.kind = LambdaKind
				app.secondaryKind = DescriptionKind
			},
			entity:     Entity{lambdaFunction: fn},
			wantSubstr: colorizeJSON(fnBytes),
		},
		{
			name: "ddb",
			setup: func() {
				app.kind = DynamoDBKind
				app.secondaryKind = DescriptionKind
			},
			entity:     Entity{ddbTable: td},
			wantSubstr: colorizeJSON(tdBytes),
		},
		{
			name: "sqs message",
			setup: func() {
				app.kind = SQSPeekKind
				app.secondaryKind = DescriptionKind
			},
			entity:     Entity{sqsMessage: msg},
			wantSubstr: colorizeJSON(msgBytes),
		},
		{
			name: "sqs queue",
			setup: func() {
				app.kind = SQSKind
				app.secondaryKind = DescriptionKind
			},
			entity:     Entity{sqsQueueName: "https://sqs.us-east-1.amazonaws.com/111/orders"},
			wantSubstr: `name[-:-:-]": "orders"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			got, _, err := v.getJsonString(tc.entity)
			if err != nil {
				t.Fatalf("getJsonString: %v", err)
			}
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("output missing substring\n  got: %s\n  want substr: %s", got, tc.wantSubstr)
			}
		})
	}
}
