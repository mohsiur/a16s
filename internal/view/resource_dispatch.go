package view

import kindpkg "github.com/mohsiur/a16s/internal/view/kind"

// resourceRegistryName maps the legacy `kind` enum values to the canonical
// names registered in kindpkg. Only kinds that have been migrated to
// kindpkg.Resource appear here — call sites use resolveResource to opt into
// the new path one method at a time, with the enum switches as fallback.
//
// Phase 2: Lambda. Phase 3 (this PR) adds SQS — both SQSKind (queue list) and
// SQSPeekKind (per-queue messages) resolve to the same registered "sqs" kind
// because they open the same console URL today; the cached selection on
// sqsKind covers both pages.
var resourceRegistryName = map[kind]string{
	LambdaKind:  "lambda",
	SQSKind:     "sqs",
	SQSPeekKind: "sqs",
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
