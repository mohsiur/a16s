package view

// kindRegistryName maps the view-package `kind` enum to the canonical name
// the kind was registered under in kindpkg. resolveResource(k) looks up
// here, then queries kindpkg.Get for the actual Resource.
//
// paletteVerbs maps user-typed `:` palette verbs to their target kind.
// Both maps are populated by per-kind `bindKind` calls from each file's
// init() so adding a new kind doesn't touch any centralized table.
var (
	kindRegistryName = map[kind]string{}
	paletteVerbs     = map[string]kind{}
)

// bindKind wires a kind enum into the registry-name and palette tables.
// Pass an empty registryName for palette-only kinds (Profile, Region) that
// don't have a kindpkg.Resource implementation yet.
//
// Panics on collision so duplicate registrations are caught at startup.
// Called from each kind's init() block, alongside kindpkg.Register.
func bindKind(k kind, registryName string, verbs ...string) {
	if registryName != "" {
		if existing, dup := kindRegistryName[k]; dup {
			panic("kind already bound to registry name: " + existing)
		}
		kindRegistryName[k] = registryName
	}
	for _, v := range verbs {
		if _, dup := paletteVerbs[v]; dup {
			panic("palette verb collision: " + v)
		}
		paletteVerbs[v] = k
	}
}
