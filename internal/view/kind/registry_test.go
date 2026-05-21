package kind

import "testing"

type stubKind struct {
	name       string
	resetCalls int
}

func (s *stubKind) Name() string     { return s.name }
func (s *stubKind) Reset()           { s.resetCalls++ }
func (s *stubKind) Selection() any   { return nil }
func (s *stubKind) SetSelection(any) {}

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

type aliasedStub struct {
	stubKind
	aliases []string
}

func (a *aliasedStub) Aliases() []string { return a.aliases }

func TestAliasResolvesToSameKind(t *testing.T) {
	resetRegistryForTest()
	k := &aliasedStub{stubKind: stubKind{name: "ddb"}, aliases: []string{"dynamodb"}}
	Register(k)

	canonical, ok := Get("ddb")
	if !ok || canonical != k {
		t.Fatalf("Get(\"ddb\") = %v, %v; want %v, true", canonical, ok, k)
	}
	alias, ok := Get("dynamodb")
	if !ok || alias != k {
		t.Fatalf("Get(\"dynamodb\") = %v, %v; want %v, true", alias, ok, k)
	}
}

func TestAllDedupesAliases(t *testing.T) {
	resetRegistryForTest()
	Register(&aliasedStub{stubKind: stubKind{name: "ddb"}, aliases: []string{"dynamodb"}})
	Register(&stubKind{name: "lambda"})

	got := All()
	if len(got) != 2 {
		t.Fatalf("All() len = %d; want 2", len(got))
	}
	names := []string{got[0].Name(), got[1].Name()}
	want := []string{"ddb", "lambda"}
	for i, n := range names {
		if n != want[i] {
			t.Fatalf("All()[%d].Name() = %q; want %q", i, n, want[i])
		}
	}
}

func TestNamesIncludesAliases(t *testing.T) {
	resetRegistryForTest()
	Register(&aliasedStub{stubKind: stubKind{name: "ddb"}, aliases: []string{"dynamodb"}})
	Register(&stubKind{name: "lambda"})

	got := Names()
	want := []string{"ddb", "dynamodb", "lambda"}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v; want %v", got, want)
	}
	for i, n := range got {
		if n != want[i] {
			t.Fatalf("Names()[%d] = %q; want %q", i, n, want[i])
		}
	}
}

func TestRegisterAliasCollisionPanics(t *testing.T) {
	resetRegistryForTest()
	Register(&stubKind{name: "dynamodb"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when alias collides with existing name; got nil")
		}
	}()
	Register(&aliasedStub{stubKind: stubKind{name: "ddb"}, aliases: []string{"dynamodb"}})
}
