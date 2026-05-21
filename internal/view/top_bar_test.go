package view

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestTopBarSetContextRendersAllParts(t *testing.T) {
	tb := newTopBarWidget()
	tb.SetContext("dev", "eu-west-1", "sqs > orders-dlq")
	got := tb.label.GetText(true)
	if !strings.Contains(got, "dev") || !strings.Contains(got, "eu-west-1") || !strings.Contains(got, "sqs > orders-dlq") {
		t.Fatalf("label = %q; want all of profile/region/breadcrumb", got)
	}
}

func TestTopBarSetContextSkipsEmptyParts(t *testing.T) {
	tb := newTopBarWidget()
	tb.SetContext("dev", "", "")
	got := tb.label.GetText(true)
	if strings.Contains(got, "·") {
		t.Fatalf("label = %q; should not contain separator when only profile is set", got)
	}
}

// TestTopBarEnterPaletteSwapsAndRestores locks in the swap behaviour:
// EnterPalette replaces the label with an InputField; exitPalette restores
// the label so the row continues to occupy a single child slot.
func TestTopBarEnterPaletteSwapsAndRestores(t *testing.T) {
	tb := newTopBarWidget()
	tb.SetContext("dev", "us-east-1", "cluster")
	in := tb.EnterPalette(nil, func(string, tcell.Key) {})
	if in == nil {
		t.Fatal("EnterPalette returned nil input")
	}
	if tb.flex.GetItemCount() != 1 || tb.flex.GetItem(0) != in {
		t.Fatalf("flex did not swap to input field")
	}
	tb.exitPalette()
	if tb.flex.GetItemCount() != 1 || tb.flex.GetItem(0) != tb.label {
		t.Fatalf("flex did not restore label after exitPalette")
	}
}

func TestTopBarEnterPaletteIdempotent(t *testing.T) {
	tb := newTopBarWidget()
	first := tb.EnterPalette(nil, func(string, tcell.Key) {})
	second := tb.EnterPalette(nil, func(string, tcell.Key) {})
	if first != second {
		t.Fatalf("EnterPalette returned a new field on a re-entry; should be idempotent")
	}
}
