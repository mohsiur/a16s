package view

import kindpkg "github.com/mohsiur/a16s/internal/view/kind"

// resourceRegistryName maps the legacy `kind` enum values to the canonical
// names registered in kindpkg. Only kinds that have been migrated to
// kindpkg.Resource appear here — call sites prefer the Resource method and
// fall through to the legacy enum switch when it returns nil.
//
// SQS-peek and DDB index/scan get their own Resources rather than sharing
// with the parent: BrowserURL is a passthrough (the AWS console collapses
// these views onto the parent's URL), but FooterItem must differ
// ("messages", "indexes", "items" vs. "queues", "tables").
var resourceRegistryName = map[kind]string{
	LambdaKind:            "lambda",
	SQSKind:               "sqs",
	SQSPeekKind:           "sqs-messages",
	DynamoDBKind:          "ddb",
	DynamoDBIndexKind:     "ddb-indexes",
	DynamoDBScanKind:      "ddb-items",
	ClusterKind:           "clusters",
	ServiceKind:           "services",
	TaskKind:              "tasks",
	ContainerKind:         "containers",
	TaskDefinitionKind:    "task-definitions",
	ServiceDeploymentKind: "service-deployments",
}

// resolveResource returns the kindpkg.Resource for k, or nil when k has not
// been migrated. Callers prefer the Resource method when non-nil and fall
// through to the legacy enum switch otherwise.
//
// Returning nil rather than an empty BaseKind is deliberate: it keeps the
// "have we migrated this kind yet?" signal explicit at every call site so
// stragglers are easy to find with `grep resolveResource`.
func resolveResource(k kind) kindpkg.Resource {
	name, ok := resourceRegistryName[k]
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
