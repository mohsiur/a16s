package view

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

func init() { kindpkg.Register(&sqsKind{}) }

type sqsKind struct {
	// selectedURL is set in two ways:
	//   1. Row-change in Build's table fires SetSelection(fullURL).
	//   2. Cross-kind nav from Lambda DLQ calls SetSelection(bareQueueName).
	// Build's pre-selection block promotes (2) to a full URL after listing.
	selectedURL string
}

func (k *sqsKind) Name() string   { return "sqs" }
func (k *sqsKind) Reset()         { k.selectedURL = "" }
func (k *sqsKind) Selection() any { return k.selectedURL }

func (k *sqsKind) SetSelection(s any) {
	if str, ok := s.(string); ok {
		k.selectedURL = str
	}
}

func (k *sqsKind) Breadcrumb() string {
	if k.selectedURL == "" {
		return "sqs"
	}
	return "sqs > " + queueNameFromURL(k.selectedURL)
}

// queueNameFromURL returns the last path segment of an SQS queue URL. Bare
// names (no `/`) pass through unchanged so cross-kind nav from Lambda DLQ
// (which only knows the queue name) doesn't need special-casing in callers.
func queueNameFromURL(url string) string {
	idx := strings.LastIndex(url, "/")
	if idx < 0 {
		return url
	}
	return url[idx+1:]
}

func (k *sqsKind) PrimaryAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		msgs, err := app.APIStore().PeekMessages(context.Background(), k.selectedURL)
		if err != nil {
			app.FlashError(err.Error())
			return err
		}
		var body strings.Builder
		if len(msgs) == 0 {
			body.WriteString("(no messages)")
		}
		for _, m := range msgs {
			body.WriteString("---\n")
			if m.Body != nil {
				body.WriteString(*m.Body)
			}
			body.WriteString("\n")
		}
		tv := tview.NewTextView().SetText(body.String())
		tv.SetBorder(true).SetTitle(" peek " + queueNameFromURL(k.selectedURL) + " ")
		flex := tview.NewFlex().AddItem(tv, 0, 1, true)
		return app.SwitchView(
			&pseudoKind{name: "peek:" + queueNameFromURL(k.selectedURL)},
			&simpleKindView{flex: flex, app: app, source: k},
		)
	}
}

func (k *sqsKind) SecondaryActions() []kindpkg.Binding {
	return []kindpkg.Binding{
		{Key: 'p', Label: "purge", Run: k.purgeAction()},
		{Key: 's', Label: "send", Run: k.sendAction()},
	}
}

func (k *sqsKind) purgeAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: no confirm modal — typing `p` purges immediately. A modal
		// requiring the user to type the queue name is a follow-up.
		if err := app.APIStore().PurgeQueue(context.Background(), k.selectedURL); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("purged " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

func (k *sqsKind) sendAction() kindpkg.Action {
	return func(app kindpkg.App) error {
		if k.selectedURL == "" {
			app.FlashError("no queue selected")
			return nil
		}
		// MVP: fixed test payload. Modal input is a follow-up.
		if err := app.APIStore().SendMessage(context.Background(), k.selectedURL, `{"e1s":"test"}`); err != nil {
			app.FlashError(err.Error())
			return err
		}
		app.FlashError("sent test message to " + queueNameFromURL(k.selectedURL))
		return nil
	}
}

func (k *sqsKind) Build(app kindpkg.App) (kindpkg.View, error) {
	urls, err := app.APIStore().ListQueues(context.Background())
	if err != nil {
		return nil, err
	}

	table := tview.NewTable().SetBorders(false)
	headers := []string{"Name", "ApproxMessages", "ApproxInFlight", "ApproxDelayed", "DLQ"}
	for col, h := range headers {
		table.SetCell(0, col, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	for row, url := range urls {
		copyURL := url
		attrs, _ := app.APIStore().GetQueueAttributes(context.Background(), url)
		cells := []string{
			queueNameFromURL(url),
			attrs["ApproximateNumberOfMessages"],
			attrs["ApproximateNumberOfMessagesNotVisible"],
			attrs["ApproximateNumberOfMessagesDelayed"],
			fmt.Sprint(attrs["RedrivePolicy"] != ""),
		}
		for col, c := range cells {
			cell := tview.NewTableCell(c)
			if col == 0 {
				cell.SetReference(copyURL)
			}
			table.SetCell(row+1, col, cell)
		}
	}

	v := newTableKindView(app, k, table)

	// Pre-selection: SetSelection may have been called with either a full URL
	// (from a previous row-change) or a bare queue name (from cross-kind nav
	// from Lambda DLQ). Match against the row's reference (full URL) directly,
	// or against queueNameFromURL(reference) for the bare-name case.
	if k.selectedURL != "" {
		for r := 1; r <= len(urls); r++ {
			cell := table.GetCell(r, 0)
			if cell == nil {
				continue
			}
			ref, _ := cell.GetReference().(string)
			if ref == k.selectedURL || queueNameFromURL(ref) == k.selectedURL {
				table.Select(r, 0)
				k.selectedURL = ref // promote bare name to full URL
				break
			}
		}
	}

	return v, nil
}
