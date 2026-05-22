package view

import kindpkg "github.com/mohsiur/a16s/internal/view/kind"

// resolveResource returns the kindpkg.Resource for k, or nil when k has not
// been migrated. Callers prefer the Resource method when non-nil and fall
// through to the legacy enum switch otherwise.
//
// Returning nil rather than an empty BaseKind is deliberate: it keeps the
// "have we migrated this kind yet?" signal explicit at every call site so
// stragglers are easy to find with `grep resolveResource`.
//
// The kind→registry-name binding is populated by each kind file's init()
// via bindKind, so adding a new kind doesn't touch a centralized table.
func resolveResource(k kind) kindpkg.Resource {
	name, ok := kindRegistryName[k]
	if !ok {
		return nil
	}
	r, ok := kindpkg.Get(name)
	if !ok {
		return nil
	}
	res, _ := r.(kindpkg.Resource)
	return res
}

// selectionFromEntity returns the selection value the kind's SetSelection
// expects, picked out of the per-row Entity payload. Returning the right type
// per kind keeps this map at the view layer (kindpkg can't import view, and
// the Entity struct lives in view), so each kind's SetSelection stays
// strongly-typed at the call site below.
//
// Kinds without a row-backed selection (Profile, Region, ddb scan leaf,
// sqs-messages leaf) return nil so the dispatcher in changeSelectedValues
// becomes a no-op for them.
func selectionFromEntity(k kind, e Entity) any {
	switch k {
	case ClusterKind:
		if e.cluster != nil {
			return e.cluster
		}
	case ServiceKind:
		if e.service != nil {
			return e.service
		}
	case TaskKind:
		if e.task != nil {
			return e.task
		}
	case ContainerKind:
		if e.container != nil {
			return e.container
		}
	case TaskDefinitionKind:
		if e.taskDefinition != nil {
			return e.taskDefinition
		}
	case ServiceDeploymentKind:
		if e.serviceDeployment != nil {
			return e.serviceDeployment
		}
	case InstanceKind:
		if e.instance != nil {
			return e.instance
		}
	case LambdaKind:
		if e.lambdaFunction != nil {
			return e.lambdaFunction
		}
	case SQSKind:
		if e.sqsQueueName != "" {
			return e.sqsQueueName
		}
	case DynamoDBKind:
		if e.ddbTable != nil {
			return e.ddbTable
		}
	case DynamoDBIndexKind:
		if e.ddbIndex != nil {
			return e.ddbIndex
		}
	case S3Kind:
		if e.s3Bucket != nil {
			return e.s3Bucket
		}
	}
	return nil
}
