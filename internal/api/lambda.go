package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

// ListFunctions returns all Lambda functions in the current region. Paginates
// internally; returns whatever it has on the first error after the first
// page (matches ListClusters behaviour in cluster.go).
func (c *Clients) ListFunctions(ctx context.Context) ([]lambdaTypes.FunctionConfiguration, error) {
	slog.Debug("api ListFunctions")

	var out []lambdaTypes.FunctionConfiguration
	var marker *string
	for {
		resp, err := c.Lambda().ListFunctions(ctx, &lambda.ListFunctionsInput{Marker: marker})
		if err != nil {
			slog.Error("ListFunctions failed", "error", err)
			if len(out) == 0 {
				return nil, err
			}
			return out, nil
		}
		out = append(out, resp.Functions...)
		if resp.NextMarker == nil {
			return out, nil
		}
		marker = resp.NextMarker
	}
}

// GetFunction returns the full configuration for a single function (env vars,
// VPC config, layers, DLQ — anything not in the ListFunctions summary).
func (c *Clients) GetFunction(ctx context.Context, name string) (*lambda.GetFunctionOutput, error) {
	slog.Debug("api GetFunction", "name", name)
	return c.Lambda().GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: &name})
}

// InvokeFunction invokes a function with the given payload (raw JSON bytes).
// Always uses RequestResponse so the caller can show the result.
func (c *Clients) InvokeFunction(ctx context.Context, name string, payload []byte) (*lambda.InvokeOutput, error) {
	slog.Debug("api InvokeFunction", "name", name, "payloadBytes", len(payload))
	return c.Lambda().Invoke(ctx, &lambda.InvokeInput{
		FunctionName: &name,
		Payload:      payload,
	})
}

