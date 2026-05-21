package view

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func newTestDDBTable() *ddbTypes.TableDescription {
	return &ddbTypes.TableDescription{
		TableName:      aws.String("Users"),
		TableStatus:    ddbTypes.TableStatusActive,
		ItemCount:      aws.Int64(42),
		TableSizeBytes: aws.Int64(2048),
		BillingModeSummary: &ddbTypes.BillingModeSummary{
			BillingMode: ddbTypes.BillingModePayPerRequest,
		},
		StreamSpecification: &ddbTypes.StreamSpecification{
			StreamEnabled: aws.Bool(true),
		},
		KeySchema: []ddbTypes.KeySchemaElement{
			{AttributeName: aws.String("userId"), KeyType: ddbTypes.KeyTypeHash},
			{AttributeName: aws.String("createdAt"), KeyType: ddbTypes.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []ddbTypes.GlobalSecondaryIndexDescription{
			{
				IndexName: aws.String("by-email"),
				KeySchema: []ddbTypes.KeySchemaElement{
					{AttributeName: aws.String("email"), KeyType: ddbTypes.KeyTypeHash},
				},
			},
		},
	}
}

func TestDDBViewHeaderPageItems(t *testing.T) {
	app, _ := newApp(Option{})
	app.kind = DynamoDBKind
	td := newTestDDBTable()
	v := newDDBView([]*ddbTypes.TableDescription{td}, app)
	items := v.headerPageItems(0)
	got := map[string]string{}
	for _, it := range items {
		got[it.name] = it.value
	}
	if got["Name"] != "Users" {
		t.Errorf("Name = %q; want Users", got["Name"])
	}
	if got["Status"] != "ACTIVE" {
		t.Errorf("Status = %q; want ACTIVE", got["Status"])
	}
	if got["Items"] != "42" {
		t.Errorf("Items = %q; want 42", got["Items"])
	}
	if got["Streams"] != "yes" {
		t.Errorf("Streams = %q; want yes", got["Streams"])
	}
	if got["PartitionKey"] != "userId" {
		t.Errorf("PartitionKey = %q", got["PartitionKey"])
	}
	if got["SortKey"] != "createdAt" {
		t.Errorf("SortKey = %q", got["SortKey"])
	}
	if got["GSIs"] != "1" {
		t.Errorf("GSIs = %q; want 1", got["GSIs"])
	}
}

func TestDDBViewTableParamsBuilder(t *testing.T) {
	app, _ := newApp(Option{})
	app.kind = DynamoDBKind
	v := newDDBView([]*ddbTypes.TableDescription{newTestDDBTable()}, app)
	_, headers, rowsBuilder := v.tableParamsBuilder()
	if headers[0] != "TableName" {
		t.Fatalf("headers = %v", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 1 || rows[0][0] != "Users" {
		t.Fatalf("rows = %v", rows)
	}
	if len(v.originalRowReferences) != 1 || v.originalRowReferences[0].ddbTable == nil {
		t.Errorf("ddbTable reference not wired: %+v", v.originalRowReferences)
	}
}

func TestDDBIndexViewBuildsBaseAndGSI(t *testing.T) {
	app, _ := newApp(Option{})
	app.kind = DynamoDBIndexKind
	td := newTestDDBTable()
	indexes := collectIndexes(td)
	v := newDDBIndexView(aws.ToString(td.TableName), indexes, app)
	_, headers, rowsBuilder := v.tableParamsBuilder()
	if headers[0] != "Index" {
		t.Fatalf("headers = %v", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 2 {
		t.Fatalf("rows = %d; want 2 (base + 1 GSI)", len(rows))
	}
	if rows[0][0] != "(base table)" || rows[0][1] != "BASE" {
		t.Errorf("base row = %v", rows[0])
	}
	if rows[1][0] != "by-email" || rows[1][1] != "GSI" {
		t.Errorf("gsi row = %v", rows[1])
	}
	if len(v.originalRowReferences) != 2 || v.originalRowReferences[0].ddbIndex == nil {
		t.Errorf("ddbIndex reference not wired")
	}
}

func TestDDBScanViewTableParamsBuilder(t *testing.T) {
	app, _ := newApp(Option{})
	app.kind = DynamoDBScanKind
	items := []map[string]ddbTypes.AttributeValue{
		{
			"userId": &ddbTypes.AttributeValueMemberS{Value: "u-1"},
			"email":  &ddbTypes.AttributeValueMemberS{Value: "a@b.com"},
		},
		{
			"userId": &ddbTypes.AttributeValueMemberS{Value: "u-2"},
		},
	}
	// pk="userId", no sort key → expect userId first, then remaining attrs
	// alphabetically (just "email" here).
	v := newDDBScanView("Users", "", "userId", "", items, app)
	_, headers, rowsBuilder := v.tableParamsBuilder()
	if len(headers) != 2 || headers[0] != "userId" || headers[1] != "email" {
		t.Fatalf("headers = %v; want pk-first [userId email]", headers)
	}
	rows := rowsBuilder()
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0][0] != "u-1" || rows[0][1] != "a@b.com" {
		t.Errorf("row 0 = %v", rows[0])
	}
	if rows[1][0] != "u-2" || rows[1][1] != "" {
		t.Errorf("row 1 = %v (missing attr should be empty)", rows[1])
	}
}
