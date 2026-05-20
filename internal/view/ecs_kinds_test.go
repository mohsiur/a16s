package view

import (
	"testing"

	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

func TestECSKindNames(t *testing.T) {
	cases := []struct {
		kind interface{ Name() string }
		want string
	}{
		{&ecsClusterKind{}, "cluster"},
		{&ecsServiceKind{}, "service"},
		{&ecsTaskKind{}, "task"},
	}
	for _, c := range cases {
		if got := c.kind.Name(); got != c.want {
			t.Errorf("Name = %q; want %q", got, c.want)
		}
	}
}

func TestECSKindBreadcrumbs(t *testing.T) {
	cases := []struct {
		kind interface{ Breadcrumb() string }
		want string
	}{
		{&ecsClusterKind{}, "cluster"},
		{&ecsServiceKind{}, "service"},
		{&ecsTaskKind{}, "task"},
	}
	for _, c := range cases {
		if got := c.kind.Breadcrumb(); got != c.want {
			t.Errorf("Breadcrumb = %q; want %q", got, c.want)
		}
	}
}

// TestECSKindsRegistered verifies the init() block registered all three
// adapters under their canonical palette names.
func TestECSKindsRegistered(t *testing.T) {
	cases := []struct {
		name string
		want kindpkg.Kind
	}{
		{"cluster", &ecsClusterKind{}},
		{"service", &ecsServiceKind{}},
		{"task", &ecsTaskKind{}},
	}
	for _, c := range cases {
		k, ok := kindpkg.Get(c.name)
		if !ok {
			t.Errorf("%s kind not registered", c.name)
			continue
		}
		switch c.name {
		case "cluster":
			if _, ok := k.(*ecsClusterKind); !ok {
				t.Errorf("registered %q kind is %T; want *ecsClusterKind", c.name, k)
			}
		case "service":
			if _, ok := k.(*ecsServiceKind); !ok {
				t.Errorf("registered %q kind is %T; want *ecsServiceKind", c.name, k)
			}
		case "task":
			if _, ok := k.(*ecsTaskKind); !ok {
				t.Errorf("registered %q kind is %T; want *ecsTaskKind", c.name, k)
			}
		}
	}
}
