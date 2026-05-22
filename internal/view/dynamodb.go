package view

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
	"golang.org/x/sync/errgroup"
)

func init() {
	kindpkg.Register(&ddbKind{})
	kindpkg.Register(&ddbIndexKind{})
	kindpkg.Register(&ddbScanKind{})
	bindKind(DynamoDBKind, "ddb", "tables", "ddb", "dynamodb")
	bindKind(DynamoDBIndexKind, "ddb-indexes")
	bindKind(DynamoDBScanKind, "ddb-items")
}

type ddbKind struct {
	kindpkg.BaseKind
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
func (k *ddbKind) Title() string     { return "tables" }
func (k *ddbKind) Aliases() []string { return []string{"dynamodb"} }

func (k *ddbKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showTablesPage(reload)
	}
	return nil
}
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

// BrowserURL returns the AWS console URL for the table under the cursor.
// All three DDB enum values (DynamoDBKind, DynamoDBIndexKind, DynamoDBScanKind)
// share this Resource: the legacy switch in openInBrowser already collapsed
// index/scan pages onto the parent table URL, and the dispatcher keys all
// three enums onto "ddb" so they answer through this single method.
// Returns "" when no table is selected.
func (k *ddbKind) BrowserURL(region string) (string, error) {
	td := k.selected
	if td == nil || td.TableName == nil {
		return "", nil
	}
	return utils.DynamoDBTableURL(region, aws.ToString(td.TableName)), nil
}

// FooterItem describes the ddb kind's footer summary cell.
func (k *ddbKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "tables"}
}

// Traits flag the affordances DynamoDB opts into. Drillable covers the base
// table → indexes → scan-items chain.
func (k *ddbKind) Traits() kindpkg.Traits {
	return kindpkg.Traits{
		Filterable:  true,
		Refreshable: true,
		Drillable:   true,
		Browsable:   true,
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

// Preload satisfies kindpkg.Preloader. Fired in a goroutine on app start so
// the first `:ddb` is instant. Safe to call concurrently with Build —
// loadInventory uses RWMutex.
func (k *ddbKind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app, false)
}

// loadInventory fetches the table list + descriptions and caches the result.
// Concurrent callers single-flight on k.loadDone — the first caller runs the
// fetch and closes the channel; subsequent callers (including Preload + a
// fast `:ddb`) block on the channel and read the shared result. When reload
// is true, the cache is invalidated before the fetch so refresh keys (`r`)
// and the auto-refresh ticker actually re-hit the AWS API; selection state
// is preserved.
func (k *ddbKind) loadInventory(app kindpkg.App, reload bool) error {
	k.mu.Lock()
	if reload {
		k.loaded = false
		k.names = nil
		k.descs = nil
		k.descErrs = nil
		k.loadErr = nil
		k.loadDone = nil
	}
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

	names, err := app.AWSClients().ListTables(context.Background())
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
				td, derr := app.AWSClients().DescribeTable(ctx, name)
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
		if err := dk.loadInventory(app, reload); err != nil {
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
	names, err := app.Clients.ListTables(context.Background())
	if err != nil {
		return buildResourcePage([]*ddbTypes.TableDescription(nil), app, err, func() resourceViewBuilder {
			return newDDBView(nil, app)
		})
	}
	tables := make([]*ddbTypes.TableDescription, 0, len(names))
	for _, n := range names {
		td, derr := app.Clients.DescribeTable(context.Background(), n)
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

// ddbIndexKind and ddbScanKind are Resource adapters for the index and
// scan-items leaf pages. BrowserURL delegates to the parent ddbKind (the AWS
// console collapses both views onto the parent table's URL); FooterItem
// returns the leaf-specific label.
type ddbIndexKind struct {
	kindpkg.BaseKind
}

func (k *ddbIndexKind) Name() string     { return "ddb-indexes" }
func (k *ddbIndexKind) Title() string    { return "indexes" }
func (k *ddbIndexKind) Reset()           {}
func (k *ddbIndexKind) Selection() any   { return nil }
func (k *ddbIndexKind) SetSelection(any) {}

func (k *ddbIndexKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showTableIndexesPage(reload)
	}
	return nil
}
func (k *ddbIndexKind) BrowserURL(region string) (string, error) {
	if dk := getDDBKind(); dk != nil {
		return dk.BrowserURL(region)
	}
	return "", nil
}
func (k *ddbIndexKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "indexes"}
}

type ddbScanKind struct {
	kindpkg.BaseKind
}

func (k *ddbScanKind) Name() string     { return "ddb-items" }
func (k *ddbScanKind) Title() string    { return "items" }
func (k *ddbScanKind) Reset()           {}
func (k *ddbScanKind) Selection() any   { return nil }
func (k *ddbScanKind) SetSelection(any) {}

func (k *ddbScanKind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showIndexItemsPage(reload)
	}
	return nil
}
func (k *ddbScanKind) BrowserURL(region string) (string, error) {
	if dk := getDDBKind(); dk != nil {
		return dk.BrowserURL(region)
	}
	return "", nil
}
func (k *ddbScanKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "items"}
}
func (k *ddbScanKind) Traits() kindpkg.Traits {
	return kindpkg.Traits{WideTable: true}
}

func (v *ddbView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.middle
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
	return &v.view, v.footer.middle
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
	tableName    string
	indexName    string
	partitionKey string
	sortKey      string
	items        []map[string]ddbTypes.AttributeValue
	attrs        []string
}

func newDDBScanView(tableName, indexName, partitionKey, sortKey string, items []map[string]ddbTypes.AttributeValue, app *App) *ddbScanView {
	keys := append(basicKeyInputs, []keyDescriptionPair{
		hotKeyMap["enter"],
	}...)
	attrSet := map[string]struct{}{}
	for _, it := range items {
		for k := range it {
			attrSet[k] = struct{}{}
		}
	}
	// Order: partition key first, sort key second (when present), then the
	// rest alphabetically. The pk-first ordering matches what users expect
	// from the AWS console and makes scrolling tall result sets readable.
	rest := make([]string, 0, len(attrSet))
	for a := range attrSet {
		if a == partitionKey || a == sortKey {
			continue
		}
		rest = append(rest, a)
	}
	sort.Strings(rest)
	attrs := make([]string, 0, len(attrSet))
	if partitionKey != "" {
		if _, ok := attrSet[partitionKey]; ok {
			attrs = append(attrs, partitionKey)
		}
	}
	if sortKey != "" && sortKey != partitionKey {
		if _, ok := attrSet[sortKey]; ok {
			attrs = append(attrs, sortKey)
		}
	}
	attrs = append(attrs, rest...)
	return &ddbScanView{
		view: *newView(app, keys, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		tableName:    tableName,
		indexName:    indexName,
		partitionKey: partitionKey,
		sortKey:      sortKey,
		items:        items,
		attrs:        attrs,
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
	pk := app.ddbIndex.partitionKey
	sk := app.ddbIndex.sortKey
	items, err := app.Clients.ScanIndexFirstPage(context.Background(), tableName, indexName, 25)
	return buildResourcePage(items, app, err, func() resourceViewBuilder {
		return newDDBScanView(tableName, indexName, pk, sk, items, app)
	})
}

func (v *ddbScanView) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.middle
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
