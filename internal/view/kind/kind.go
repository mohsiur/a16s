package kind

// Resource is the wider interface kinds migrate to as the refactor lands.
// It embeds the narrow Kind so a kind that satisfies Resource is also
// registry-compatible.
//
// Today most of these behaviors live as switches in the view package keyed
// off the kind enum (see internal/view). Phase 1 only declares the
// interface; no kind has been migrated yet. Per-kind migrations move
// behavior off the switches and onto methods, one method at a time, using
// BaseKind to fill the rest.
type Resource interface {
	Kind

	// PageHandle returns the unique page key this kind uses for tview.Pages
	// given the current selection (cluster ARN, queue name, table name, ...).
	// Empty when the kind has no parent context.
	PageHandle(selection any) string

	// Show mounts the kind's primary list page on host. reload forces a
	// fresh inventory fetch even when a cache is warm.
	Show(host Host, reload bool) error

	// DescribePayload returns the structured value rendered by the
	// description (json/yaml) pane for the current selection. nil when the
	// kind has no describe view.
	DescribePayload() any

	// BrowserURL returns the AWS console URL for the current selection.
	// Empty string + nil error when the kind has no browser mapping.
	BrowserURL(region string) (string, error)

	// Drilldown returns the kind navigated to when the user presses Enter
	// on a row. nil when the kind is a leaf.
	Drilldown() Resource

	// BackTo returns the kind navigated to when the user presses Esc/back.
	// nil falls through to the registry's default back chain.
	BackTo() Resource

	// FooterItem describes the kind's footer summary row.
	FooterItem() FooterItem

	// Traits describe optional capabilities a kind opts into. Used by the
	// host to decide which UI affordances to mount (filter input, refresh
	// ticker, browser hotkey, ...).
	Traits() Traits

	// Actions lists key-bound commands surfaced in the kind's footer/menu.
	Actions() []Action
}

// Action is one key-bound command surfaced in a kind's footer/menu.
// Handler is bound at mount time and may close over host state.
type Action struct {
	Key         string
	Description string
	Handler     func()
}

// FooterItem describes the kind's footer summary cell. Label is the static
// prefix (e.g. "Tables"); Hint is appended when non-empty.
type FooterItem struct {
	Label string
	Hint  string
}

// Traits flag optional capabilities a kind opts into. The host inspects
// these to decide which affordances to mount; defaults are zero-valued.
type Traits struct {
	Filterable  bool
	Refreshable bool
	Drillable   bool
	Browsable   bool
}

// Host is the surface a Resource needs during Show. It embeds App so
// existing registry callers keep working; Phase 2 widens Host with
// title/notice/mount accessors as kinds migrate. Implemented by *view.App.
type Host interface {
	App
}

// BaseKind supplies no-op defaults for the wider Resource methods. Embed
// in a concrete kind to satisfy Resource without implementing every method
// up front. The narrow Kind methods (Name, Reset, Selection, SetSelection)
// must still be provided by the embedding kind — they are kind-specific
// and have no sensible default.
type BaseKind struct{}

func (BaseKind) PageHandle(any) string              { return "" }
func (BaseKind) Show(Host, bool) error              { return nil }
func (BaseKind) DescribePayload() any               { return nil }
func (BaseKind) BrowserURL(string) (string, error)  { return "", nil }
func (BaseKind) Drilldown() Resource                { return nil }
func (BaseKind) BackTo() Resource                   { return nil }
func (BaseKind) FooterItem() FooterItem             { return FooterItem{} }
func (BaseKind) Traits() Traits                     { return Traits{} }
func (BaseKind) Actions() []Action                  { return nil }
