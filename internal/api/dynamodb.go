package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ListTables returns every DynamoDB table name in the current region. Paginates
// internally; on first error after the first page, returns what it has.
func (store *Store) ListTables(ctx context.Context) ([]string, error) {
	store.initDynamoDBClient()
	slog.Debug("api ListTables")

	var out []string
	var lastEvaluated *string
	for {
		resp, err := store.dynamodb.ListTables(ctx, &dynamodb.ListTablesInput{
			ExclusiveStartTableName: lastEvaluated,
		})
		if err != nil {
			slog.Error("ListTables failed", "error", err)
			if len(out) == 0 {
				return nil, err
			}
			return out, nil
		}
		out = append(out, resp.TableNames...)
		if resp.LastEvaluatedTableName == nil {
			return out, nil
		}
		lastEvaluated = resp.LastEvaluatedTableName
	}
}

// DescribeTable returns the full table metadata (key schema, GSIs, streams).
func (store *Store) DescribeTable(ctx context.Context, name string) (*ddbTypes.TableDescription, error) {
	store.initDynamoDBClient()
	slog.Debug("api DescribeTable", "name", name)
	resp, err := store.dynamodb.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: &name})
	if err != nil {
		return nil, err
	}
	return resp.Table, nil
}

// ScanFirstPage runs a Scan with the given limit and returns the first page
// only. Pagination beyond page 1 is a follow-up.
func (store *Store) ScanFirstPage(ctx context.Context, table string, limit int32) ([]map[string]ddbTypes.AttributeValue, error) {
	return store.ScanIndexFirstPage(ctx, table, "", limit)
}

// ScanIndexFirstPage scans either the base table (indexName == "") or a GSI/LSI
// (indexName non-empty) and returns the first page of items.
func (store *Store) ScanIndexFirstPage(ctx context.Context, table, indexName string, limit int32) ([]map[string]ddbTypes.AttributeValue, error) {
	store.initDynamoDBClient()
	slog.Debug("api ScanIndexFirstPage", "table", table, "index", indexName, "limit", limit)
	in := &dynamodb.ScanInput{
		TableName: &table,
		Limit:     &limit,
	}
	if indexName != "" {
		in.IndexName = &indexName
	}
	resp, err := store.dynamodb.Scan(ctx, in)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// QueryEquality runs a Query against the base table or an index using a single
// equality condition on `keyAttr`. The value is treated as a string. Returns
// the first page only.
func (store *Store) QueryEquality(ctx context.Context, table, indexName, keyAttr, keyValue string, limit int32) ([]map[string]ddbTypes.AttributeValue, error) {
	store.initDynamoDBClient()
	slog.Debug("api QueryEquality", "table", table, "index", indexName, "attr", keyAttr, "limit", limit)
	in := &dynamodb.QueryInput{
		TableName:              &table,
		Limit:                  &limit,
		KeyConditionExpression: aws.String("#k = :v"),
		ExpressionAttributeNames: map[string]string{
			"#k": keyAttr,
		},
		ExpressionAttributeValues: map[string]ddbTypes.AttributeValue{
			":v": &ddbTypes.AttributeValueMemberS{Value: keyValue},
		},
	}
	if indexName != "" {
		in.IndexName = &indexName
	}
	resp, err := store.dynamodb.Query(ctx, in)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
