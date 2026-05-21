package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// Compile-time pin: lambdaKind must satisfy the wider Resource interface so
// the dispatcher can route through it. Phase 3 PRs add equivalent assertions
// for sqsKind / ddbKind.
var _ kindpkg.Resource = (*lambdaKind)(nil)
var _ kindpkg.Resource = (*sqsKind)(nil)
var _ kindpkg.Resource = (*ddbKind)(nil)

func TestResolveResource_LambdaMigrated(t *testing.T) {
	r := resolveResource(LambdaKind)
	if r == nil {
		t.Fatal("resolveResource(LambdaKind) = nil; want non-nil — Phase 2 migrated lambda")
	}
}

func TestResolveResource_SQSMigrated(t *testing.T) {
	if got := resolveResource(SQSKind); got == nil {
		t.Fatal("resolveResource(SQSKind) = nil; want non-nil — Phase 3 migrated sqs")
	}
	if got := resolveResource(SQSPeekKind); got == nil {
		t.Fatal("resolveResource(SQSPeekKind) = nil; want non-nil — SQSPeek shares sqs kind")
	}
}

func TestResolveResource_UnmigratedKindReturnsNil(t *testing.T) {
	if got := resolveResource(ClusterKind); got != nil {
		t.Fatalf("resolveResource(ClusterKind) = %T; want nil — ECS chain migrates later", got)
	}
}

func TestLambdaKind_BrowserURL(t *testing.T) {
	lk := getLambdaKind()
	if lk == nil {
		t.Fatal("getLambdaKind() = nil; lambdaKind init() should have registered it")
	}
	t.Cleanup(lk.Reset)

	if got, _ := lk.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL with no selection = %q; want empty", got)
	}

	lk.SetSelection(&lambdaTypes.FunctionConfiguration{FunctionName: aws.String("my-fn")})
	got, err := lk.BrowserURL("us-east-1")
	if err != nil {
		t.Fatalf("BrowserURL err = %v; want nil", err)
	}
	want := "https://us-east-1.console.aws.amazon.com/lambda/home?region=us-east-1#/functions/my-fn"
	if got != want {
		t.Errorf("BrowserURL = %q; want %q", got, want)
	}
}

func TestSQSKind_BrowserURL(t *testing.T) {
	sk := getSQSKind()
	if sk == nil {
		t.Fatal("getSQSKind() = nil; sqsKind init() should have registered it")
	}
	t.Cleanup(sk.Reset)

	if got, _ := sk.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL with no selection = %q; want empty", got)
	}

	queueURL := "https://sqs.us-east-1.amazonaws.com/111122223333/my-queue"
	sk.SetSelection(queueURL)
	got, err := sk.BrowserURL("us-east-1")
	if err != nil {
		t.Fatalf("BrowserURL err = %v; want nil", err)
	}
	want := "https://us-east-1.console.aws.amazon.com/sqs/v3/home?region=us-east-1#/queues/https%3A%2F%2Fsqs.us-east-1.amazonaws.com%2F111122223333%2Fmy-queue"
	if got != want {
		t.Errorf("BrowserURL = %q; want %q", got, want)
	}
}

// TestResolveResource_DDBMigrated pins that all three DDB enums route through
// the same Resource. The legacy openInBrowser switch collapsed index/scan
// pages onto the parent table URL; the dispatcher must preserve that by
// keying every DDB enum onto "ddb".
func TestResolveResource_DDBMigrated(t *testing.T) {
	for _, k := range []kind{DynamoDBKind, DynamoDBIndexKind, DynamoDBScanKind} {
		r := resolveResource(k)
		if r == nil {
			t.Fatalf("resolveResource(%s) = nil; want non-nil — Phase 3 migrated ddb", k)
		}
	}
}

func TestDDBKind_BrowserURL(t *testing.T) {
	dk := getDDBKind()
	if dk == nil {
		t.Fatal("getDDBKind() = nil; ddbKind init() should have registered it")
	}
	t.Cleanup(dk.Reset)

	if got, _ := dk.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL with no selection = %q; want empty", got)
	}

	dk.SetSelection(&ddbTypes.TableDescription{TableName: aws.String("my-table")})
	got, err := dk.BrowserURL("us-east-1")
	if err != nil {
		t.Fatalf("BrowserURL err = %v; want nil", err)
	}
	want := "https://us-east-1.console.aws.amazon.com/dynamodbv2/home?region=us-east-1#table?name=my-table"
	if got != want {
		t.Errorf("BrowserURL = %q; want %q", got, want)
	}
}
