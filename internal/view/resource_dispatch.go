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
