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
		tableName := aws.ToString(k.selected.TableName)
		items, err := app.APIStore().ScanFirstPage(context.Background(), tableName, 25)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		flex := renderScanResults(tableName, items)
		return app.SwitchView(
			&pseudoKind{name: "scan:" + tableName},
			newTextSubView(app, flex),
		)
	}
}

// renderScanResults turns a slice of DynamoDB items into a tview Flex. Items
// have a different attribute set in general; we union all attribute names
// across items, prefer key attributes (selected.KeySchema) as the leftmost
// columns, then alphabetical for the rest. Cells render the underlying
// AttributeValue as a short string. Long values are truncated with "…".
func renderScanResults(tableName string, items []map[string]ddbTypes.AttributeValue) *tview.Flex {
	if len(items) == 0 {
		tv := tview.NewTextView().SetText("(no items)")
		tv.SetBorder(true).SetTitle(" scan " + tableName + " (first 25) ")
		return tview.NewFlex().AddItem(tv, 0, 1, true)
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
	scanTable.SetSelectable(true, false)
	scanTable.SetFixed(1, 0)
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
	scanTable.SetTitle(" scan " + tableName + " (" + fmt.Sprintf("%d", len(items)) + " items, first 25) ")
	scanTable.SetBorder(true)
	return tview.NewFlex().AddItem(scanTable, 0, 1, true)
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
