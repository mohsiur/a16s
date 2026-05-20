package kind

import "testing"

type stubKind struct {
	name       string
	resetCalls int
}

func (s *stubKind) Name() string                { return s.name }
func (s *stubKind) Build(App) (View, error)     { return nil, nil }
func (s *stubKind) Reset()                      { s.resetCalls++ }
func (s *stubKind) Selection() any              { return nil }
func (s *stubKind) SetSelection(any)            {}
func (s *stubKind) Breadcrumb() string          { return s.name }
func (s *stubKind) PrimaryAction() Action       { return nil }
func (s *stubKind) SecondaryActions() []Binding { return nil }

func TestRegisterAndGet(t *testing.T) {
	resetRegistryForTest()
	k := &stubKind{name: "lambda"}
	Register(k)

	got, ok := Get("lambda")
	if !ok || got != k {
		t.Fatalf("Get(\"lambda\") = %v, %v; want %v, true", got, ok, k)
	}
}

func TestGetUnknownReturnsFalse(t *testing.T) {
	resetRegistryForTest()
	if _, ok := Get("nope"); ok {
		t.Fatal("Get(\"nope\") returned ok=true; want false")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	resetRegistryForTest()
	Register(&stubKind{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register; got nil")
		}
	}()
	Register(&stubKind{name: "dup"})
}

func TestResetAllCallsResetOnEveryKind(t *testing.T) {
	resetRegistryForTest()
	a := &stubKind{name: "a"}
	b := &stubKind{name: "b"}
	Register(a)
	Register(b)

	ResetAll()

	if a.resetCalls != 1 || b.resetCalls != 1 {
		t.Fatalf("Reset calls a=%d b=%d; want 1,1", a.resetCalls, b.resetCalls)
	}
}

func TestAllReturnsSortedByName(t *testing.T) {
	resetRegistryForTest()
	Register(&stubKind{name: "sqs"})
	Register(&stubKind{name: "ddb"})
	Register(&stubKind{name: "lambda"})

	got := All()
	want := []string{"ddb", "lambda", "sqs"}
	for i, k := range got {
		if k.Name() != want[i] {
			t.Fatalf("All()[%d].Name() = %q; want %q", i, k.Name(), want[i])
		}
	}
}
