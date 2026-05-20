package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDDBKindName(t *testing.T) {
	k := &ddbKind{}
	if k.Name() != "ddb" {
		t.Fatalf("Name = %q; want %q", k.Name(), "ddb")
	}
}

func TestDDBKindSelectionRoundTrip(t *testing.T) {
	k := &ddbKind{}
	td := &ddbTypes.TableDescription{TableName: aws.String("users")}
	k.SetSelection(td)
	if k.Selection() != td {
		t.Fatalf("Selection round-trip failed")
	}
}

func TestDDBKindResetClearsSelection(t *testing.T) {
	k := &ddbKind{}
	k.SetSelection(&ddbTypes.TableDescription{TableName: aws.String("x")})
	k.Reset()
	if k.Selection() != nil {
		t.Fatalf("Selection after Reset = %v; want nil", k.Selection())
	}
}

func TestDDBKindBreadcrumb(t *testing.T) {
	k := &ddbKind{}
	if got := k.Breadcrumb(); got != "ddb" {
		t.Fatalf("Breadcrumb (no selection) = %q", got)
	}
	k.SetSelection(&ddbTypes.TableDescription{TableName: aws.String("users")})
	if got := k.Breadcrumb(); got != "ddb > users" {
		t.Fatalf("Breadcrumb = %q; want %q", got, "ddb > users")
	}
}

func TestDDBKindSecondaryActions(t *testing.T) {
	k := &ddbKind{}
	got := k.SecondaryActions()
	if len(got) != 1 {
		t.Fatalf("len(SecondaryActions) = %d; want 1", len(got))
	}
	if got[0].Key != 'c' || got[0].Label != "describe" {
		t.Fatalf("binding = %+v; want {c, describe}", got[0])
	}
}
