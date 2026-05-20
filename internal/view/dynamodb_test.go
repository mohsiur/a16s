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

func TestCollectIndexesBaseFirstThenGSIThenLSI(t *testing.T) {
	td := &ddbTypes.TableDescription{
		KeySchema: []ddbTypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: ddbTypes.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: ddbTypes.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []ddbTypes.GlobalSecondaryIndexDescription{
			{
				IndexName: aws.String("gsi-by-email"),
				KeySchema: []ddbTypes.KeySchemaElement{
					{AttributeName: aws.String("email"), KeyType: ddbTypes.KeyTypeHash},
				},
			},
			{
				IndexName: aws.String("gsi-by-org"),
				KeySchema: []ddbTypes.KeySchemaElement{
					{AttributeName: aws.String("org_id"), KeyType: ddbTypes.KeyTypeHash},
				},
			},
		},
		LocalSecondaryIndexes: []ddbTypes.LocalSecondaryIndexDescription{
			{
				IndexName: aws.String("lsi-by-created"),
				KeySchema: []ddbTypes.KeySchemaElement{
					{AttributeName: aws.String("pk"), KeyType: ddbTypes.KeyTypeHash},
					{AttributeName: aws.String("created_at"), KeyType: ddbTypes.KeyTypeRange},
				},
			},
		},
	}
	got := collectIndexes(td)
	if len(got) != 4 {
		t.Fatalf("len = %d; want 4", len(got))
	}
	if got[0].kind != "BASE" || got[0].partitionKey != "pk" || got[0].sortKey != "sk" {
		t.Fatalf("base = %+v", got[0])
	}
	if got[1].kind != "GSI" || got[1].name != "gsi-by-email" {
		t.Fatalf("gsi[0] = %+v; want gsi-by-email first", got[1])
	}
	if got[2].kind != "GSI" || got[2].name != "gsi-by-org" {
		t.Fatalf("gsi[1] = %+v", got[2])
	}
	if got[3].kind != "LSI" || got[3].name != "lsi-by-created" {
		t.Fatalf("lsi[0] = %+v", got[3])
	}
}

func TestCollectIndexesBaseOnly(t *testing.T) {
	td := &ddbTypes.TableDescription{
		KeySchema: []ddbTypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: ddbTypes.KeyTypeHash},
		},
	}
	got := collectIndexes(td)
	if len(got) != 1 || got[0].kind != "BASE" || got[0].partitionKey != "pk" || got[0].sortKey != "" {
		t.Fatalf("got %+v; want single BASE entry with pk only", got)
	}
}
