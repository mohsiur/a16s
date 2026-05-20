package api

import (
	"context"
	"log/slog"

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
	store.initDynamoDBClient()
	slog.Debug("api ScanFirstPage", "table", table, "limit", limit)
	resp, err := store.dynamodb.Scan(ctx, &dynamodb.ScanInput{
		TableName: &table,
		Limit:     &limit,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
