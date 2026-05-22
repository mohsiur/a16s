package kind

import (
	"errors"
	"testing"
)

// resourceStub embeds BaseKind so it inherits the wider defaults, then
// implements only the narrow Kind methods. If BaseKind ever stops covering
// the wider Resource surface this file fails to compile, which is the test.
type resourceStub struct{ BaseKind }

func (resourceStub) Name() string     { return "stub" }
func (resourceStub) Reset()           {}
func (resourceStub) Selection() any   { return nil }
func (resourceStub) SetSelection(any) {}

var _ Resource = resourceStub{}

func TestBaseKindDefaults(t *testing.T) {
	var s resourceStub

	if got := s.PageHandle(nil); got != "" {
		t.Errorf("PageHandle = %q; want empty", got)
	}
	if err := s.Show(nil, false); !errors.Is(err, ErrShowUnimplemented) {
		t.Errorf("Show err = %v; want ErrShowUnimplemented (the BaseKind default — embedding kinds override Show)", err)
	}
	if got := s.DescribePayload(); got != nil {
		t.Errorf("DescribePayload = %v; want nil", got)
	}
	url, err := s.BrowserURL("us-east-1")
	if url != "" || err != nil {
		t.Errorf("BrowserURL = (%q, %v); want (\"\", nil)", url, err)
	}
	if got := s.Drilldown(); got != nil {
		t.Errorf("Drilldown = %v; want nil", got)
	}
	if got := s.BackTo(); got != nil {
		t.Errorf("BackTo = %v; want nil", got)
	}
	if got := s.FooterItem(); got != (FooterItem{}) {
		t.Errorf("FooterItem = %#v; want zero", got)
	}
	if got := s.Traits(); got != (Traits{}) {
		t.Errorf("Traits = %#v; want zero", got)
	}
	if got := s.Actions(); got != nil {
		t.Errorf("Actions = %v; want nil", got)
	}
}
