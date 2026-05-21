package view

import kindpkg "github.com/mohsiur/a16s/internal/view/kind"

// resourceRegistryName maps the legacy `kind` enum values to the canonical
// names registered in kindpkg. Only kinds that have been migrated to
// kindpkg.Resource appear here — call sites use resolveResource to opt into
// the new path one method at a time, with the enum switches as fallback.
//
// Phase 2 added Lambda. Phase 3 adds SQS and DynamoDB. SQSKind/SQSPeekKind
// both resolve to "sqs" and DynamoDBKind/DynamoDBIndexKind/DynamoDBScanKind
// all resolve to "ddb" because the legacy openInBrowser switch already
// collapsed those pages onto the parent resource's console URL — sharing
// one Resource keeps that behavior in a single BrowserURL implementation.
// Map keys are canonical Name() values; aliases (e.g. "dynamodb" on ddbKind)
// would confuse kind.All()'s dedupe.
var resourceRegistryName = map[kind]string{
	LambdaKind:        "lambda",
	SQSKind:           "sqs",
	SQSPeekKind:       "sqs",
	DynamoDBKind:      "ddb",
	DynamoDBIndexKind: "ddb",
	DynamoDBScanKind:  "ddb",
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
