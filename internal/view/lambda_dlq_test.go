package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func TestParseQueueNameFromArn(t *testing.T) {
	cases := map[string]string{
		"arn:aws:sqs:us-east-1:111:my-dlq":      "my-dlq",
		"arn:aws:sqs:eu-west-2:222:another-dlq": "another-dlq",
		"":                                      "",
		"not-an-arn":                            "",
	}
	for in, want := range cases {
		if got := parseQueueNameFromArn(in); got != want {
			t.Fatalf("parseQueueNameFromArn(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestDLQActionFlashesWhenNoSelection(t *testing.T) {
	k := &lambdaKind{}
	app := &fakeApp{}
	if err := k.dlqAction()(app); err != nil {
		t.Fatalf("err = %v", err)
	}
	if app.flashedMsg == "" {
		t.Fatal("expected flash for no selection")
	}
}

func TestDLQActionFlashesWhenNoDLQ(t *testing.T) {
	k := &lambdaKind{selected: &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("x")}}
	app := &fakeApp{}
	if err := k.dlqAction()(app); err != nil {
		t.Fatalf("err = %v", err)
	}
	if app.flashedMsg != "no DLQ configured" {
		t.Fatalf("flashedMsg = %q", app.flashedMsg)
	}
}
