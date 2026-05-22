package view

import (
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

// Typed selection accessors backed by the kindpkg registry.
//
// Migrated kinds own their selection inside their Resource struct; these
// accessors expose that registry-backed selection as a typed value so
// callers read App state through Cluster(), Service(), etc. rather than
// touching kind internals directly.
//
// Each accessor returns nil when:
//   - the kind isn't registered (shouldn't happen at runtime; bug if it does)
//   - the kind hasn't seen a SetSelection yet (first paint, post-Reset)
//
// Always nil-guard the accessor's return value before dereferencing — these
// are the only path to the active selection.

// Cluster returns the active ECS cluster selection, or nil when none.
func (app *App) Cluster() *ecsTypes.Cluster {
	if k := getClusterKind(); k != nil {
		return k.selected
	}
	return nil
}

// Service returns the active ECS service selection, or nil when none.
func (app *App) Service() *ecsTypes.Service {
	if k := getServiceKind(); k != nil {
		return k.selected
	}
	return nil
}

// Task returns the active ECS task selection, or nil when none.
func (app *App) Task() *ecsTypes.Task {
	if k := getTaskKind(); k != nil {
		return k.selected
	}
	return nil
}

// Container returns the active ECS container selection, or nil when none.
func (app *App) Container() *ecsTypes.Container {
	if k := getContainerKind(); k != nil {
		return k.selected
	}
	return nil
}

// TaskDefinition returns the active ECS task definition selection, or nil
// when none.
func (app *App) TaskDefinition() *ecsTypes.TaskDefinition {
	if k := getTaskDefinitionKind(); k != nil {
		return k.selected
	}
	return nil
}

// ServiceDeployment returns the active ECS service deployment selection,
// or nil when none.
func (app *App) ServiceDeployment() *ecsTypes.ServiceDeployment {
	if k := getServiceDeploymentKind(); k != nil {
		return k.selected
	}
	return nil
}

// LambdaFunction returns the active Lambda function selection, or nil
// when none.
func (app *App) LambdaFunction() *lambdaTypes.FunctionConfiguration {
	if k := getLambdaKind(); k != nil {
		return k.selected
	}
	return nil
}

// SQSQueueURL returns the active SQS queue URL (full URL, not bare name),
// or "" when none. SQS uniquely keys selection by string, not pointer —
// the AWS SDK exposes queues only as string URLs at the registry layer.
func (app *App) SQSQueueURL() string {
	if k := getSQSKind(); k != nil {
		return k.selectedURL
	}
	return ""
}

// DDBTable returns the active DynamoDB table selection, or nil when none.
func (app *App) DDBTable() *ddbTypes.TableDescription {
	if k := getDDBKind(); k != nil {
		return k.selected
	}
	return nil
}

// DDBIndex returns the active DynamoDB index selection (base table or
// secondary index), or nil when none.
func (app *App) DDBIndex() *ddbIndex {
	if k := getDDBIndexKind(); k != nil {
		return k.selected
	}
	return nil
}
