package view

import "testing"

func TestSqsKindName(t *testing.T) {
	k := &sqsKind{}
	if k.Name() != "sqs" {
		t.Fatalf("Name = %q; want %q", k.Name(), "sqs")
	}
}

func TestSqsKindSelectionRoundTrip(t *testing.T) {
	k := &sqsKind{}
	url := "https://sqs.us-east-1.amazonaws.com/111/my-queue"
	k.SetSelection(url)
	if got, _ := k.Selection().(string); got != url {
		t.Fatalf("Selection round-trip failed: got %q", got)
	}
}

func TestSqsKindResetClearsSelection(t *testing.T) {
	k := &sqsKind{}
	k.SetSelection("https://sqs.us-east-1.amazonaws.com/111/my-queue")
	k.Reset()
	if got, _ := k.Selection().(string); got != "" {
		t.Fatalf("Selection after Reset = %q; want empty", got)
	}
}

func TestSqsKindBreadcrumb(t *testing.T) {
	k := &sqsKind{}
	if got := k.Breadcrumb(); got != "sqs" {
		t.Fatalf("Breadcrumb (no selection) = %q", got)
	}
	k.SetSelection("https://sqs.us-east-1.amazonaws.com/111/my-queue")
	if got := k.Breadcrumb(); got != "sqs > my-queue" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "sqs > my-queue")
	}
}

func TestSqsKindSecondaryActions(t *testing.T) {
	k := &sqsKind{}
	got := k.SecondaryActions()
	if len(got) != 2 {
		t.Fatalf("len(SecondaryActions) = %d; want 2", len(got))
	}
	want := map[rune]string{'p': "purge", 's': "send"}
	for _, b := range got {
		if want[b.Key] != b.Label {
			t.Fatalf("binding %c => %q; want %q", b.Key, b.Label, want[b.Key])
		}
	}
}

func TestQueueNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://sqs.us-east-1.amazonaws.com/111/my-queue": "my-queue",
		"my-queue": "my-queue", // bare name passes through
		"":         "",
	}
	for in, want := range cases {
		if got := queueNameFromURL(in); got != want {
			t.Fatalf("queueNameFromURL(%q) = %q; want %q", in, got, want)
		}
	}
}
