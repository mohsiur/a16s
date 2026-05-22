package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// TestAccessors_NilWhenUnset pins that every typed accessor returns the
// zero value before any SetSelection call. Callers historically nil-checked
// app.cluster directly — the accessors must preserve that semantic so the
// legacy guards keep working when readers migrate.
func TestAccessors_NilWhenUnset(t *testing.T) {
	t.Cleanup(kindpkg.ResetAll)
	kindpkg.ResetAll()

	app := &App{}
	if got := app.Cluster(); got != nil {
		t.Errorf("Cluster() = %v; want nil", got)
	}
	if got := app.Service(); got != nil {
		t.Errorf("Service() = %v; want nil", got)
	}
	if got := app.Task(); got != nil {
		t.Errorf("Task() = %v; want nil", got)
	}
	if got := app.Container(); got != nil {
		t.Errorf("Container() = %v; want nil", got)
	}
	if got := app.TaskDefinition(); got != nil {
		t.Errorf("TaskDefinition() = %v; want nil", got)
	}
	if got := app.ServiceDeployment(); got != nil {
		t.Errorf("ServiceDeployment() = %v; want nil", got)
	}
	if got := app.LambdaFunction(); got != nil {
		t.Errorf("LambdaFunction() = %v; want nil", got)
	}
	if got := app.SQSQueueURL(); got != "" {
		t.Errorf("SQSQueueURL() = %q; want empty", got)
	}
	if got := app.DDBTable(); got != nil {
		t.Errorf("DDBTable() = %v; want nil", got)
	}
	if got := app.DDBIndex(); got != nil {
		t.Errorf("DDBIndex() = %v; want nil", got)
	}
}

// TestAccessors_ReadThroughRegistry pins that each accessor reflects the
// value SetSelection wrote to the registry. This is the contract that lets
// readers replace `v.app.cluster` with `v.app.Cluster()` without behaviour
// changing.
func TestAccessors_ReadThroughRegistry(t *testing.T) {
	t.Cleanup(kindpkg.ResetAll)
	kindpkg.ResetAll()

	app := &App{}

	cluster := &ecsTypes.Cluster{ClusterArn: aws.String("c-arn")}
	getClusterKind().SetSelection(cluster)
	if got := app.Cluster(); got != cluster {
		t.Errorf("Cluster() = %v; want %v", got, cluster)
	}

	service := &ecsTypes.Service{ServiceArn: aws.String("s-arn")}
	getServiceKind().SetSelection(service)
	if got := app.Service(); got != service {
		t.Errorf("Service() = %v; want %v", got, service)
	}

	task := &ecsTypes.Task{TaskArn: aws.String("t-arn")}
	getTaskKind().SetSelection(task)
	if got := app.Task(); got != task {
		t.Errorf("Task() = %v; want %v", got, task)
	}

	container := &ecsTypes.Container{ContainerArn: aws.String("ctr-arn")}
	getContainerKind().SetSelection(container)
	if got := app.Container(); got != container {
		t.Errorf("Container() = %v; want %v", got, container)
	}

	td := &ecsTypes.TaskDefinition{TaskDefinitionArn: aws.String("td-arn")}
	getTaskDefinitionKind().SetSelection(td)
	if got := app.TaskDefinition(); got != td {
		t.Errorf("TaskDefinition() = %v; want %v", got, td)
	}

	sd := &ecsTypes.ServiceDeployment{ServiceDeploymentArn: aws.String("sd-arn")}
	getServiceDeploymentKind().SetSelection(sd)
	if got := app.ServiceDeployment(); got != sd {
		t.Errorf("ServiceDeployment() = %v; want %v", got, sd)
	}

	fn := &lambdaTypes.FunctionConfiguration{FunctionName: aws.String("fn")}
	getLambdaKind().SetSelection(fn)
	if got := app.LambdaFunction(); got != fn {
		t.Errorf("LambdaFunction() = %v; want %v", got, fn)
	}

	getSQSKind().SetSelection("https://sqs.us-east-1.amazonaws.com/111/q")
	if got := app.SQSQueueURL(); got != "https://sqs.us-east-1.amazonaws.com/111/q" {
		t.Errorf("SQSQueueURL() = %q; want full URL", got)
	}

	tbl := &ddbTypes.TableDescription{TableName: aws.String("t1")}
	getDDBKind().SetSelection(tbl)
	if got := app.DDBTable(); got != tbl {
		t.Errorf("DDBTable() = %v; want %v", got, tbl)
	}

	idx := &ddbIndex{name: "gsi1"}
	getDDBIndexKind().SetSelection(idx)
	if got := app.DDBIndex(); got != idx {
		t.Errorf("DDBIndex() = %v; want %v", got, idx)
	}
}
