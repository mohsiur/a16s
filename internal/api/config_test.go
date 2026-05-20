package api

import (
	"testing"
)

func TestSwitchAwsConfigInvokesOnConfigSwitch(t *testing.T) {
	called := 0
	prev := OnConfigSwitch
	OnConfigSwitch = func() { called++ }
	defer func() { OnConfigSwitch = prev }()

	s := &Store{}
	// SwitchAwsConfig with empty profile/region uses defaults; it may fail to
	// load AWS config in a sandbox, which is fine — we only assert that the
	// callback fires AFTER successful config load. So if it returns an error,
	// skip the test.
	if err := s.SwitchAwsConfig("", ""); err != nil {
		t.Skip("SwitchAwsConfig requires AWS config; skipping in sandbox: " + err.Error())
	}
	if called != 1 {
		t.Fatalf("OnConfigSwitch called %d times; want 1", called)
	}
}
