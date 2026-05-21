package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func newTestLambdaView(t *testing.T) *lambdaView {
	t.Helper()
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	app.kind = LambdaKind
	fns := []lambdaTypes.FunctionConfiguration{
		{
			FunctionName:  aws.String("auth-handler"),
			FunctionArn:   aws.String("arn:aws:lambda:us-east-1:111:function:auth-handler"),
			Runtime:       lambdaTypes.RuntimeNodejs20x,
			MemorySize:    aws.Int32(512),
			Timeout:       aws.Int32(30),
			LastModified:  aws.String("2026-04-01T00:00:00Z"),
			State:         lambdaTypes.StateActive,
			Handler:       aws.String("index.handler"),
			Role:          aws.String("arn:aws:iam::111:role/auth"),
			Architectures: []lambdaTypes.Architecture{lambdaTypes.ArchitectureArm64},
			DeadLetterConfig: &lambdaTypes.DeadLetterConfig{
				TargetArn: aws.String("arn:aws:sqs:us-east-1:111:auth-dlq"),
			},
		},
	}
	return newLambdaView(fns, app)
}

func TestLambdaViewHeaderPageItems(t *testing.T) {
	v := newTestLambdaView(t)
	items := v.headerPageItems(0)
	want := map[string]string{
		"Name":         "auth-handler",
		"Runtime":      "nodejs20.x",
		"Memory":       "512 MB",
		"Timeout":      "30s",
		"State":        "Active",
		"Handler":      "index.handler",
		"DLQ":          "auth-dlq",
		"Architecture": "arm64",
	}
	got := map[string]string{}
	for _, it := range items {
		got[it.name] = it.value
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q; want %q", k, got[k], w)
		}
	}
}

func TestLambdaViewTableParamsBuilder(t *testing.T) {
	v := newTestLambdaView(t)
	title, headers, rowsBuilder := v.tableParamsBuilder()
	if title == "" {
		t.Fatal("empty title")
	}
	if len(headers) != 6 || headers[0] != "Name" {
		t.Fatalf("headers = %v", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	if rows[0][0] != "auth-handler" {
		t.Errorf("Name = %q; want %q", rows[0][0], "auth-handler")
	}
	if rows[0][1] != "nodejs20.x" {
		t.Errorf("Runtime = %q; want %q", rows[0][1], "nodejs20.x")
	}
	if rows[0][2] != "512" {
		t.Errorf("Memory = %q; want %q", rows[0][2], "512")
	}
	if len(v.originalRowReferences) != 1 {
		t.Fatalf("rowRefs = %d; want 1", len(v.originalRowReferences))
	}
	ref := v.originalRowReferences[0]
	if ref.lambdaFunction == nil || aws.ToString(ref.lambdaFunction.FunctionName) != "auth-handler" {
		t.Errorf("lambdaFunction reference not wired correctly: %+v", ref)
	}
}

func TestLambdaViewGetViewAndFooterReturnsLambdaTab(t *testing.T) {
	v := newTestLambdaView(t)
	_, footer := v.getViewAndFooter()
	if footer == nil {
		t.Fatal("footer.lambda is nil")
	}
}
