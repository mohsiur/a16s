package view

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gdamore/tcell/v2"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
	"golang.org/x/sync/errgroup"
)

func init() { kindpkg.Register(&ddbKind{}) }

type ddbKind struct {
	selected *ddbTypes.TableDescription
	// inventory captured during Build / Preload so Informer methods can
	// compute aggregate + per-row detail without re-querying DynamoDB.
	// `descs` and `descErrs` are parallel to `names` — a nil desc with a
	// non-nil err means the DescribeTable for that name failed.
	// `loadDone` is the single-flight latch: nil before the first load, set
	// to a fresh channel when a load starts, closed when it finishes. This
	// guarantees concurrent Preload + Build only fetch once and the second
	// caller waits.
	mu       sync.RWMutex
	names    []string
	descs    []*ddbTypes.TableDescription
	descErrs []error
	loaded   bool
	loadDone chan struct{}
	loadErr  error
}

func (k *ddbKind) Name() string      { return "ddb" }
func (k *ddbKind) Aliases() []string { return []string{"dynamodb"} }
func (k *ddbKind) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.selected = nil
	k.names = nil
	k.descs = nil
	k.descErrs = nil
	k.loaded = false
	k.loadDone = nil
	k.loadErr = nil
}

func (k *ddbKind) Selection() any {
	if k.selected == nil {
		return nil
	}
	return k.selected
}

func (k *ddbKind) SetSelection(s any) {
	if td, ok := s.(*ddbTypes.TableDescription); ok {
		k.selected = td
	}
}

func (k *ddbKind) Breadcrumb() string {
	if k.selected == nil || k.selected.TableName == nil {
		return "ddb"
	}
	return "ddb > " + aws.ToString(k.selected.TableName)
}

func (k *ddbKind) PrimaryAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		return openIndexList(app, k.selected)
	}
}

// ddbIndex is a flat representation of either the base table or a GSI/LSI:
// `name` is "" for the base table, otherwise the index name; partitionKey is
// the hash key attribute name (used by the query action).
type ddbIndex struct {
	name         string
	kind         string // "BASE", "GSI", "LSI"
	partitionKey string
	sortKey      string
}

// collectIndexes returns the base table first, then GSIs (alphabetical), then
// LSIs (alphabetical). The base-first ordering is the user-visible ask: scan
// shows indexes with the base table on top.
func collectIndexes(td *ddbTypes.TableDescription) []ddbIndex {
	out := []ddbIndex{{
		name:         "",
		kind:         "BASE",
		partitionKey: keyAttr(td.KeySchema, ddbTypes.KeyTypeHash),
		sortKey:      keyAttr(td.KeySchema, ddbTypes.KeyTypeRange),
	}}
	gsis := make([]ddbIndex, 0, len(td.GlobalSecondaryIndexes))
	for _, gsi := range td.GlobalSecondaryIndexes {
		gsis = append(gsis, ddbIndex{
			name:         aws.ToString(gsi.IndexName),
			kind:         "GSI",
			partitionKey: keyAttr(gsi.KeySchema, ddbTypes.KeyTypeHash),
			sortKey:      keyAttr(gsi.KeySchema, ddbTypes.KeyTypeRange),
		})
	}
	sort.Slice(gsis, func(i, j int) bool { return gsis[i].name < gsis[j].name })
	out = append(out, gsis...)
	lsis := make([]ddbIndex, 0, len(td.LocalSecondaryIndexes))
	for _, lsi := range td.LocalSecondaryIndexes {
		lsis = append(lsis, ddbIndex{
			name:         aws.ToString(lsi.IndexName),
			kind:         "LSI",
			partitionKey: keyAttr(lsi.KeySchema, ddbTypes.KeyTypeHash),
			sortKey:      keyAttr(lsi.KeySchema, ddbTypes.KeyTypeRange),
		})
	}
	sort.Slice(lsis, func(i, j int) bool { return lsis[i].name < lsis[j].name })
	out = append(out, lsis...)
	return out
}

func keyAttr(schema []ddbTypes.KeySchemaElement, kt ddbTypes.KeyType) string {
	for _, e := range schema {
		if e.KeyType == kt {
			return aws.ToString(e.AttributeName)
		}
	}
	return ""
}

// openIndexList renders Index | Type | PartitionKey | SortKey for the table's
// base + GSIs + LSIs. Enter scans the chosen index; `q` queries it.
func openIndexList(app kindpkg.App, td *ddbTypes.TableDescription) error {
	tableName := aws.ToString(td.TableName)
	idxs := collectIndexes(td)
	table := tview.NewTable().SetBorders(false)
	headers := []string{"Index", "Type", "PartitionKey", "SortKey"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for r, idx := range idxs {
		display := idx.name
		if idx.name == "" {
			display = "(base table)"
		}
		cells := []string{display, idx.kind, idx.partitionKey, idx.sortKey}
		copyIdx := idx
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyIdx)
			}
			table.SetCell(r+1, col, cell)
		}
	}
	view := newTableSubView(app, table, "indexes "+tableName, func(row int) {
		idx, _ := table.GetCell(row, 0).GetReference().(ddbIndex)
		_ = openScanResults(app, tableName, idx)
	})
	view.bindings = []kindpkg.Binding{{Key: 'q', Label: "query", Run: func(app kindpkg.App) error {
		row, _ := table.GetSelection()
		if row < 1 {
			app.FlashError("no index selected")
			return nil
		}
		idx, _ := table.GetCell(row, 0).GetReference().(ddbIndex)
		return openQueryPrompt(app, tableName, idx)
	}}}
	return app.SwitchView(&pseudoKind{name: "indexes:" + tableName}, view)
}

func openScanResults(app kindpkg.App, tableName string, idx ddbIndex) error {
	items, err := app.APIStore().ScanIndexFirstPage(context.Background(), tableName, idx.name, 25)
	if err != nil {
		app.FlashError(err.Error())
		return err
	}
	title := "scan " + tableName
	if idx.name != "" {
		title += " / " + idx.name
	}
	view := buildScanResultsView(app, title, items)
	return app.SwitchView(&pseudoKind{name: "scan:" + tableName + ":" + idx.name}, view)
}

// openQueryPrompt mounts a one-line input above the empty results area, asking
// for the partition-key value. Submitting runs QueryEquality and replaces the
// flex with a results table. Esc returns to the index list.
func openQueryPrompt(app kindpkg.App, tableName string, idx ddbIndex) error {
	if idx.partitionKey == "" {
		app.FlashError("index has no partition key")
		return nil
	}
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	prompt := tview.NewInputField().
		SetLabel(idx.partitionKey + " = ").
		SetFieldWidth(0)
	status := tview.NewTextView().SetText("Enter to run, Esc to cancel")
	flex.AddItem(prompt, 1, 0, true)
	flex.AddItem(status, 1, 0, false)
	view := newTextSubView(app, flex)
	prompt.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		val := prompt.GetText()
		items, err := app.APIStore().QueryEquality(context.Background(), tableName, idx.name, idx.partitionKey, val, 25)
		if err != nil {
			app.FlashError(err.Error())
			return
		}
		title := fmt.Sprintf("query %s [%s = %q]", tableName, idx.partitionKey, val)
		if idx.name != "" {
			title = fmt.Sprintf("query %s/%s [%s = %q]", tableName, idx.name, idx.partitionKey, val)
		}
		results := buildScanResultsView(app, title, items)
		_ = app.SwitchView(&pseudoKind{name: "query:" + tableName + ":" + idx.name}, results)
	})
	return app.SwitchView(&pseudoKind{name: "query-prompt:" + tableName + ":" + idx.name}, view)
}

// buildScanResultsView turns a slice of DynamoDB items into a sortable table
// sub-view (or a "(no items)" TextView when the result set is empty). Header
// row + each item row preserve item attributes; missing attributes render as
// empty cells.
func buildScanResultsView(app kindpkg.App, title string, items []map[string]ddbTypes.AttributeValue) *simpleKindView {
	if len(items) == 0 {
		tv := tview.NewTextView().SetText("(no items)")
		tv.SetBorder(true).SetTitle(" " + title + " ")
		return newTextSubView(app, tview.NewFlex().AddItem(tv, 0, 1, true))
	}
	attrSet := map[string]struct{}{}
	for _, it := range items {
		for k := range it {
			attrSet[k] = struct{}{}
		}
	}
	attrs := make([]string, 0, len(attrSet))
	for a := range attrSet {
		attrs = append(attrs, a)
	}
	sort.Strings(attrs)

	scanTable := tview.NewTable().SetBorders(false)
	for col, h := range attrs {
		scanTable.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for r, it := range items {
		for col, attr := range attrs {
			val := ""
			if av, ok := it[attr]; ok {
				val = ddbAttrToString(av)
			}
			scanTable.SetCell(r+1, col, tview.NewTableCell(val).SetMaxWidth(40))
		}
	}
	return newTableSubView(app, scanTable, fmt.Sprintf("%s (%d items, first 25)", title, len(items)), nil)
}

// ddbAttrToString renders a DynamoDB AttributeValue as a short string
// suitable for a table cell. Maps and lists are summarised by their length —
// drilling in is a follow-up.
func ddbAttrToString(av ddbTypes.AttributeValue) string {
	switch v := av.(type) {
	case *ddbTypes.AttributeValueMemberS:
		return v.Value
	case *ddbTypes.AttributeValueMemberN:
		return v.Value
	case *ddbTypes.AttributeValueMemberBOOL:
		if v.Value {
			return "true"
		}
		return "false"
	case *ddbTypes.AttributeValueMemberNULL:
		return "null"
	case *ddbTypes.AttributeValueMemberSS:
		return "[" + fmt.Sprintf("%d strs", len(v.Value)) + "]"
	case *ddbTypes.AttributeValueMemberNS:
		return "[" + fmt.Sprintf("%d nums", len(v.Value)) + "]"
	case *ddbTypes.AttributeValueMemberBS:
		return "[" + fmt.Sprintf("%d bins", len(v.Value)) + "]"
	case *ddbTypes.AttributeValueMemberL:
		return fmt.Sprintf("list(%d)", len(v.Value))
	case *ddbTypes.AttributeValueMemberM:
		return fmt.Sprintf("map(%d)", len(v.Value))
	case *ddbTypes.AttributeValueMemberB:
		return fmt.Sprintf("bin(%d B)", len(v.Value))
	}
	return ""
}

func (k *ddbKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'c', Label: "describe", Run: k.describeAction()},
	}
}

func (k *ddbKind) describeAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selected == nil {
			app.FlashError("no table selected")
			return nil
		}
		body, _ := json.MarshalIndent(k.selected, "", "  ")
		tv := tview.NewTextView().SetText(string(body))
		tv.SetBorder(true).SetTitle(" " + aws.ToString(k.selected.TableName) + " describe ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&pseudoKind{name: "describe:" + aws.ToString(k.selected.TableName)},
			newTextSubView(app, flex),
		)
	}
}

// Preload satisfies kindpkg.Preloader. Fired in a goroutine on app start so
// the first `:ddb` is instant. Safe to call concurrently with Build —
// loadInventory uses RWMutex.
func (k *ddbKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app)
}

// loadInventory fetches the table list + descriptions once and caches the
// result. Concurrent callers single-flight on k.loadDone — the first caller
// runs the fetch and closes the channel; subsequent callers (including
// Preload + a fast `:ddb`) block on the channel and read the shared result.
// After Reset() the cycle restarts.
func (k *ddbKind) loadInventory(app kindpkg.App) error {
	k.mu.Lock()
	if k.loaded {
		k.mu.Unlock()
		return nil
	}
	if k.loadDone != nil {
		done := k.loadDone
		k.mu.Unlock()
		<-done
		k.mu.RLock()
		err := k.loadErr
		k.mu.RUnlock()
		return err
	}
	done := make(chan struct{})
	k.loadDone = done
	k.mu.Unlock()

	names, err := app.APIStore().ListTables(context.Background())
	var descs []*ddbTypes.TableDescription
	var errs []error
	if err == nil {
		// Fan out DescribeTable. Sequential is unusable on accounts with >50
		// tables. Per-row errors are kept parallel to names so Build can flash
		// a last-error notice; cache stores both descs + errs.
		descs = make([]*ddbTypes.TableDescription, len(names))
		errs = make([]error, len(names))
		g, ctx := errgroup.WithContext(context.Background())
		g.SetLimit(10)
		for i, name := range names {
			i, name := i, name
			g.Go(func() error {
				td, derr := app.APIStore().DescribeTable(ctx, name)
				descs[i] = td
				errs[i] = derr
				return nil
			})
		}
		_ = g.Wait()
	}

	k.mu.Lock()
	k.loadErr = err
	if err == nil {
		k.names = names
		k.descs = descs
		k.descErrs = errs
		k.loaded = true
	} else {
		// Reset loadDone on error so a future call can retry; closing the
		// channel below still wakes anyone currently waiting.
		k.loadDone = nil
	}
	k.mu.Unlock()
	close(done)
	return err
}

func (k *ddbKind) buildTable(app kindpkg.App) *tview.Table {
	k.mu.RLock()
	names := k.names
	descs := k.descs
	errs := k.descErrs
	k.mu.RUnlock()

	table := tview.NewTable().SetBorders(false)
	headers := []string{"TableName", "Status", "ItemCount", "SizeBytes", "BillingMode", "Streams"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}

	rowOut := 0
	var lastErr error
	var lastErrName string
	for i, name := range names {
		td := descs[i]
		if errs[i] != nil || td == nil {
			lastErr = errs[i]
			lastErrName = name
			continue
		}
		billing := ""
		if td.BillingModeSummary != nil {
			billing = string(td.BillingModeSummary.BillingMode)
		}
		streams := "no"
		if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
			streams = "yes"
		}
		cells := []string{
			aws.ToString(td.TableName),
			string(td.TableStatus),
			fmt.Sprintf("%d", aws.ToInt64(td.ItemCount)),
			fmt.Sprintf("%d", aws.ToInt64(td.TableSizeBytes)),
			billing,
			streams,
		}
		copyTD := td
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyTD)
			}
			table.SetCell(rowOut+1, col, cell)
		}
		rowOut++
	}
	if lastErr != nil {
		app.FlashError("describe " + lastErrName + ": " + lastErr.Error())
	}

	return table
}

func (k *ddbKind) Build(app kindpkg.App) (kindpkg.View, error) {
	k.mu.RLock()
	loaded := k.loaded
	k.mu.RUnlock()
	if loaded {
		return newTableKindView(app, k, k.buildTable(app)), nil
	}
	return newLoadingTableKindView(app, k, func() error {
		return k.loadInventory(app)
	}, func() *tview.Table { return k.buildTable(app) }), nil
}

// AggregateInfo / SelectionDetail satisfy kindpkg.Informer.
func (k *ddbKind) AggregateInfo() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	successful := 0
	var totalItems, totalBytes int64
	streamed := 0
	for _, td := range k.descs {
		if td == nil {
			continue
		}
		successful++
		totalItems += aws.ToInt64(td.ItemCount)
		totalBytes += aws.ToInt64(td.TableSizeBytes)
		if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
			streamed++
		}
	}
	if successful == 0 {
		return "No tables"
	}
	return fmt.Sprintf(
		"Tables: %d\nTotal items: %d\nTotal size: %s\nWith streams: %d",
		successful, totalItems, humanBytes(totalBytes), streamed,
	)
}

func (k *ddbKind) SelectionDetail() string {
	if k.selected == nil {
		return ""
	}
	td := k.selected
	billing := ""
	if td.BillingModeSummary != nil {
		billing = string(td.BillingModeSummary.BillingMode)
	}
	streams := "no"
	if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
		streams = "yes"
	}
	return fmt.Sprintf(
		"Name: %s\nStatus: %s\nItems: %d\nSize: %s\nBilling: %s\nStreams: %s",
		aws.ToString(td.TableName),
		td.TableStatus,
		aws.ToInt64(td.ItemCount),
		humanBytes(aws.ToInt64(td.TableSizeBytes)),
		billing,
		streams,
	)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---- Legacy-style ECS chrome integration ----

type ddbView struct {
	view
	tables []*ddbTypes.TableDescription
}

func newDDBView(tables []*ddbTypes.TableDescription, app *App) *ddbView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
	}...)
	return &ddbView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		tables: tables,
	}
}

func (app *App) showTablesPage(reload bool) error {
	app.kind = DynamoDBKind
	if switched := app.switchPage(reload); switched {
		return nil
	}
	dk := getDDBKind()
	if dk != nil {
		if err := dk.loadInventory(app); err != nil {
			return err
		}
		dk.mu.RLock()
		tables := make([]*ddbTypes.TableDescription, 0, len(dk.descs))
		for _, td := range dk.descs {
			if td != nil {
				tables = append(tables, td)
			}
		}
		dk.mu.RUnlock()
		return buildResourcePage(tables, app, nil, func() resourceViewBuilder {
			return newDDBView(tables, app)
		})
	}
	names, err := app.Store.ListTables(context.Background())
	if err != nil {
		return buildResourcePage([]*ddbTypes.TableDescription(nil), app, err, func() resourceViewBuilder {
			return newDDBView(nil, app)
		})
	}
	tables := make([]*ddbTypes.TableDescription, 0, len(names))
	for _, n := range names {
		td, derr := app.Store.DescribeTable(context.Background(), n)
		if derr != nil || td == nil {
			continue
		}
		tables = append(tables, td)
	}
	return buildResourcePage(tables, app, nil, func() resourceViewBuilder {
		return newDDBView(tables, app)
	})
}

func getDDBKind() *ddbKind {
	k, ok := kindpkg.Get("ddb")
	if !ok {
		return nil
	}
	dk, _ := k.(*ddbKind)
	return dk
}

func (v *ddbView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.dynamodb
}

func (v *ddbView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.tables))
	for i, td := range v.tables {
		params = append(params, headerPageParam{
			title:      aws.ToString(td.TableName),
			entityName: aws.ToString(td.TableName),
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *ddbView) headerPageItems(index int) []headerItem {
	td := v.tables[index]
	billing := utils.EmptyText
	if td.BillingModeSummary != nil {
		billing = string(td.BillingModeSummary.BillingMode)
	}
	streams := "no"
	if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
		streams = "yes"
	}
	pk := keyAttr(td.KeySchema, ddbTypes.KeyTypeHash)
	sk := keyAttr(td.KeySchema, ddbTypes.KeyTypeRange)
	if sk == "" {
		sk = utils.EmptyText
	}
	return []headerItem{
		{name: "Name", value: aws.ToString(td.TableName)},
		{name: "Status", value: string(td.TableStatus)},
		{name: "Items", value: fmt.Sprintf("%d", aws.ToInt64(td.ItemCount))},
		{name: "Size", value: humanBytes(aws.ToInt64(td.TableSizeBytes))},
		{name: "Billing", value: billing},
		{name: "Streams", value: streams},
		{name: "PartitionKey", value: pk},
		{name: "SortKey", value: sk},
		{name: "GSIs", value: fmt.Sprintf("%d", len(td.GlobalSecondaryIndexes))},
		{name: "LSIs", value: fmt.Sprintf("%d", len(td.LocalSecondaryIndexes))},
	}
}

func (v *ddbView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, "all", len(v.tables))
	headers = []string{"TableName", "Status", "ItemCount", "SizeBytes", "BillingMode", "Streams"}
	rowsBuilder = func() (data [][]string) {
		for _, td := range v.tables {
			copyTD := td
			billing := ""
			if td.BillingModeSummary != nil {
				billing = string(td.BillingModeSummary.BillingMode)
			}
			streams := "no"
			if td.StreamSpecification != nil && aws.ToBool(td.StreamSpecification.StreamEnabled) {
				streams = "yes"
			}
			row := []string{
				aws.ToString(td.TableName),
				string(td.TableStatus),
				fmt.Sprintf("%d", aws.ToInt64(td.ItemCount)),
				fmt.Sprintf("%d", aws.ToInt64(td.TableSizeBytes)),
				billing,
				streams,
			}
			data = append(data, row)
			entity := Entity{
				ddbTable:   copyTD,
				entityName: aws.ToString(td.TableName),
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}

// ---- DDB Indexes (per table) ----

type ddbIndexView struct {
	view
	tableName string
	indexes   []ddbIndex
}

func newDDBIndexView(tableName string, indexes []ddbIndex, app *App) *ddbIndexView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
		hotKeyMap["q"],
	}...)
	return &ddbIndexView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		tableName: tableName,
		indexes:   indexes,
	}
}

func (app *App) showTableIndexesPage(reload bool) error {
	app.kind = DynamoDBIndexKind
	if app.ddbTable == nil {
		app.Notice.Warn("no table selected")
		app.back()
		return nil
	}
	if switched := app.switchPage(reload); switched {
		return nil
	}
	tableName := aws.ToString(app.ddbTable.TableName)
	indexes := collectIndexes(app.ddbTable)
	return buildResourcePage(indexes, app, nil, func() resourceViewBuilder {
		return newDDBIndexView(tableName, indexes, app)
	})
}

func (v *ddbIndexView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.ddbIndex
}

func (v *ddbIndexView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.indexes))
	for i, idx := range v.indexes {
		display := idx.name
		if display == "" {
			display = "(base table)"
		}
		params = append(params, headerPageParam{
			title:      v.tableName + " > " + display,
			entityName: v.tableName + "." + idx.name,
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *ddbIndexView) headerPageItems(index int) []headerItem {
	idx := v.indexes[index]
	display := idx.name
	if display == "" {
		display = "(base table)"
	}
	sk := idx.sortKey
	if sk == "" {
		sk = utils.EmptyText
	}
	return []headerItem{
		{name: "Table", value: v.tableName},
		{name: "Index", value: display},
		{name: "Type", value: idx.kind},
		{name: "PartitionKey", value: idx.partitionKey},
		{name: "SortKey", value: sk},
	}
}

func (v *ddbIndexView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, v.tableName, len(v.indexes))
	headers = []string{"Index", "Type", "PartitionKey", "SortKey"}
	rowsBuilder = func() (data [][]string) {
		for _, idx := range v.indexes {
			copyIdx := idx
			display := idx.name
			if display == "" {
				display = "(base table)"
			}
			sk := idx.sortKey
			if sk == "" {
				sk = utils.EmptyText
			}
			row := []string{display, idx.kind, idx.partitionKey, sk}
			data = append(data, row)
			entity := Entity{
				ddbIndex:   &copyIdx,
				entityName: v.tableName + "." + idx.name,
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}

// ---- DDB Scan items (per index) ----

type ddbScanView struct {
	view
	tableName string
	indexName string
	items     []map[string]ddbTypes.AttributeValue
	attrs     []string
}

func newDDBScanView(tableName, indexName string, items []map[string]ddbTypes.AttributeValue, app *App) *ddbScanView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
	}...)
	attrSet := map[string]struct{}{}
	for _, it := range items {
		for k := range it {
			attrSet[k] = struct{}{}
		}
	}
	attrs := make([]string, 0, len(attrSet))
	for a := range attrSet {
		attrs = append(attrs, a)
	}
	sort.Strings(attrs)
	return &ddbScanView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		tableName: tableName,
		indexName: indexName,
		items:     items,
		attrs:     attrs,
	}
}

func (app *App) showIndexItemsPage(reload bool) error {
	app.kind = DynamoDBScanKind
	if app.ddbTable == nil || app.ddbIndex == nil {
		app.Notice.Warn("no index selected")
		app.back()
		return nil
	}
	if switched := app.switchPage(reload); switched {
		return nil
	}
	tableName := aws.ToString(app.ddbTable.TableName)
	indexName := app.ddbIndex.name
	items, err := app.Store.ScanIndexFirstPage(context.Background(), tableName, indexName, 25)
	return buildResourcePage(items, app, err, func() resourceViewBuilder {
		return newDDBScanView(tableName, indexName, items, app)
	})
}

func (v *ddbScanView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.ddbScan
}

func (v *ddbScanView) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.items))
	for i := range v.items {
		entity := fmt.Sprintf("%s.%s.%d", v.tableName, v.indexName, i)
		title := v.tableName
		if v.indexName != "" {
			title += " / " + v.indexName
		}
		params = append(params, headerPageParam{
			title:      title,
			entityName: entity,
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *ddbScanView) headerPageItems(index int) []headerItem {
	it := v.items[index]
	items := []headerItem{
		{name: "Table", value: v.tableName},
		{name: "Index", value: indexLabel(v.indexName)},
		{name: "Attributes", value: fmt.Sprintf("%d", len(it))},
	}
	limit := len(v.attrs)
	if limit > 9 {
		limit = 9
	}
	for i := 0; i < limit; i++ {
		attr := v.attrs[i]
		val := utils.EmptyText
		if av, ok := it[attr]; ok {
			val = ddbAttrToString(av)
		}
		items = append(items, headerItem{name: attr, value: val})
	}
	return items
}

func indexLabel(name string) string {
	if name == "" {
		return "(base table)"
	}
	return name
}

func (v *ddbScanView) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, v.tableName+"."+v.indexName, len(v.items))
	headers = make([]string, len(v.attrs))
	copy(headers, v.attrs)
	rowsBuilder = func() (data [][]string) {
		for i, it := range v.items {
			row := make([]string, len(v.attrs))
			for col, attr := range v.attrs {
				if av, ok := it[attr]; ok {
					row[col] = ddbAttrToString(av)
				}
			}
			data = append(data, row)
			entity := Entity{
				entityName: fmt.Sprintf("%s.%s.%d", v.tableName, v.indexName, i),
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}
