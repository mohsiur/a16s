package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// Compile-time pins: every kind in resourceRegistryName must satisfy the
// wider Resource interface so the dispatcher can route through it.
var (
	_ kindpkg.Resource = (*lambdaKind)(nil)
	_ kindpkg.Resource = (*sqsKind)(nil)
	_ kindpkg.Resource = (*sqsPeekKind)(nil)
	_ kindpkg.Resource = (*ddbKind)(nil)
	_ kindpkg.Resource = (*ddbIndexKind)(nil)
	_ kindpkg.Resource = (*ddbScanKind)(nil)
	_ kindpkg.Resource = (*clusterKind)(nil)
	_ kindpkg.Resource = (*serviceKind)(nil)
	_ kindpkg.Resource = (*taskKind)(nil)
	_ kindpkg.Resource = (*containerKind)(nil)
	_ kindpkg.Resource = (*taskDefinitionKind)(nil)
	_ kindpkg.Resource = (*serviceDeploymentKind)(nil)
)

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
	if got := resolveResource(InstanceKind); got != nil {
		t.Fatalf("resolveResource(InstanceKind) = %T; want nil — instance not migrated", got)
	}
}

func TestResolveResource_ECSChainMigrated(t *testing.T) {
	for _, k := range []kind{ClusterKind, ServiceKind, TaskKind, ContainerKind, TaskDefinitionKind, ServiceDeploymentKind} {
		if r := resolveResource(k); r == nil {
			t.Errorf("resolveResource(%v) = nil; want non-nil — ECS chain migrated in Phase 3", k)
		}
	}
}

func TestClusterKind_BrowserURL(t *testing.T) {
	ck := getClusterKind()
	if ck == nil {
		t.Fatal("getClusterKind() = nil; init() should have registered it")
	}
	t.Cleanup(ck.Reset)

	if got, _ := ck.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL with no selection = %q; want empty", got)
	}

	arn := "arn:aws:ecs:us-east-1:111111111111:cluster/my-cluster"
	ck.SetSelection(&ecsTypes.Cluster{ClusterArn: aws.String(arn)})
	got, err := ck.BrowserURL("us-east-1")
	if err != nil {
		t.Fatalf("BrowserURL err = %v; want nil", err)
	}
	want := utils.ArnToUrl(arn, "")
	if got != want {
		t.Errorf("BrowserURL = %q; want %q", got, want)
	}
}

func TestTaskKind_BrowserURL(t *testing.T) {
	tk := getTaskKind()
	sk := getServiceKind()
	if tk == nil || sk == nil {
		t.Fatal("task/service kind not registered")
	}
	t.Cleanup(tk.Reset)
	t.Cleanup(sk.Reset)

	if got, _ := tk.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL with no selection = %q; want empty", got)
	}

	taskArn := "arn:aws:ecs:us-east-1:111111111111:task/my-cluster/abc123"
	tk.SetSelection(&ecsTypes.Task{TaskArn: aws.String(taskArn)})
	if got, _ := tk.BrowserURL("us-east-1"); got != "" {
		t.Errorf("BrowserURL without service context = %q; want empty (legacy fallthrough)", got)
	}

	sk.SetSelection(&ecsTypes.Service{
		ServiceName: aws.String("my-service"),
		ServiceArn:  aws.String("arn:aws:ecs:us-east-1:111111111111:service/my-cluster/my-service"),
	})
	got, err := tk.BrowserURL("us-east-1")
	if err != nil {
		t.Fatalf("BrowserURL err = %v; want nil", err)
	}
	want := utils.ArnToUrl(taskArn, "my-service")
	if got != want {
		t.Errorf("BrowserURL = %q; want %q", got, want)
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
// the registry. DynamoDBKind goes to the parent ddb Resource; the index and
// scan leaves get their own Resource so they can return their own
// FooterItem ("indexes", "items") while delegating BrowserURL upward.
func TestResolveResource_DDBMigrated(t *testing.T) {
	for _, k := range []kind{DynamoDBKind, DynamoDBIndexKind, DynamoDBScanKind} {
		r := resolveResource(k)
		if r == nil {
			t.Fatalf("resolveResource(%s) = nil; want non-nil — Phase 3 migrated ddb", k)
		}
	}
}

// TestResolveResource_LeavesHaveOwnResource pins that SQSPeek, DDBIndex, and
// DDBScan resolve to a *different* Resource than their parents. Phase 4
// split them so they can return distinct FooterItem labels — collapsing
// them back onto the parent would regress the footer.
func TestResolveResource_LeavesHaveOwnResource(t *testing.T) {
	cases := []struct {
		parent, leaf kind
	}{
		{SQSKind, SQSPeekKind},
		{DynamoDBKind, DynamoDBIndexKind},
		{DynamoDBKind, DynamoDBScanKind},
	}
	for _, c := range cases {
		p, l := resolveResource(c.parent), resolveResource(c.leaf)
		if p == nil || l == nil {
			t.Fatalf("resolveResource(%s)=%v, resolveResource(%s)=%v; want both non-nil", c.parent, p, c.leaf, l)
		}
		if p == l {
			t.Errorf("%s and %s resolved to same Resource; want distinct so FooterItem differs", c.parent, c.leaf)
		}
	}
}

// TestFooterItem_AllMigratedKinds pins the label every migrated kind shows
// in the middle footer slot. These match what the legacy enum switch in
// addFooterItems used to render via the kind enum's String(); migrating the
// labels onto kindpkg.Resource must preserve them exactly.
func TestFooterItem_AllMigratedKinds(t *testing.T) {
	cases := []struct {
		k    kind
		want string
	}{
		{LambdaKind, "lambdas"},
		{SQSKind, "queues"},
		{SQSPeekKind, "messages"},
		{DynamoDBKind, "tables"},
		{DynamoDBIndexKind, "indexes"},
		{DynamoDBScanKind, "items"},
		{ClusterKind, "clusters"},
		{ServiceKind, "services"},
		{TaskKind, "tasks"},
		{ContainerKind, "containers"},
		{TaskDefinitionKind, "task definitions"},
		{ServiceDeploymentKind, "service deployments"},
	}
	for _, c := range cases {
		r := resolveResource(c.k)
		if r == nil {
			t.Errorf("resolveResource(%s) = nil; want non-nil", c.k)
			continue
		}
		got := r.FooterItem().Label
		if got != c.want {
			t.Errorf("FooterItem(%s).Label = %q; want %q", c.k, got, c.want)
		}
	}
}

// TestMiddleFooterLabel pins the middle-cell label for every kind that
// reaches addFooterItems. The four ECS chain kinds whose labels live in
// the always-shown left row return "" so the if/else falls through to the
// empty-stretch branch — matching legacy behaviour.
func TestMiddleFooterLabel(t *testing.T) {
	cases := []struct {
		k    kind
		want string
	}{
		// ECS chain kinds: middle slot stays empty, label is in the left row.
		{ClusterKind, ""},
		{ServiceKind, ""},
		{TaskKind, ""},
		{ContainerKind, ""},
		// Migrated middle-slot kinds: routed through Resource.FooterItem.
		{LambdaKind, "lambdas"},
		{SQSKind, "queues"},
		{SQSPeekKind, "messages"},
		{DynamoDBKind, "tables"},
		{DynamoDBIndexKind, "indexes"},
		{DynamoDBScanKind, "items"},
		{TaskDefinitionKind, "task definitions"},
		{ServiceDeploymentKind, "service deployments"},
		// Unmigrated middle-slot kinds: still via the kind enum's String().
		{ProfileKind, "profiles"},
		{RegionKind, "regions"},
		{HelpKind, "help"},
		{InstanceKind, "instances"},
	}
	for _, c := range cases {
		got := middleFooterLabel(c.k)
		if got != c.want {
			t.Errorf("middleFooterLabel(%s) = %q; want %q", c.k, got, c.want)
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
