package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// Compile-time pin: lambdaKind must satisfy the wider Resource interface so
// the dispatcher can route through it. Phase 3 PRs add equivalent assertions
// for sqsKind / ddbKind.
var _ kindpkg.Resource = (*lambdaKind)(nil)

func TestResolveResource_LambdaMigrated(t *testing.T) {
	r := resolveResource(LambdaKind)
	if r == nil {
		t.Fatal("resolveResource(LambdaKind) = nil; want non-nil — Phase 2 migrated lambda")
	}
}

func TestResolveResource_UnmigratedKindReturnsNil(t *testing.T) {
	if got := resolveResource(SQSKind); got != nil {
		t.Fatalf("resolveResource(SQSKind) = %T; want nil — SQS migrates in Phase 3", got)
	}
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
