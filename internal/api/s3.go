package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ListBuckets returns every S3 bucket the active credentials can see. S3
// buckets are global (not regional), so this is a single request — no
// pagination loop like ListFunctions/ListClusters.
func (c *Clients) ListBuckets(ctx context.Context) ([]s3Types.Bucket, error) {
	slog.Debug("api ListBuckets")
	out, err := c.S3().ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		slog.Error("ListBuckets failed", "error", err)
		return nil, err
	}
	return out.Buckets, nil
}

// GetBucketLocation returns the region a bucket lives in. The legacy
// us-east-1 default response is "" — callers normalise that themselves.
func (c *Clients) GetBucketLocation(ctx context.Context, bucket string) (string, error) {
	out, err := c.S3().GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: &bucket})
	if err != nil {
		return "", err
	}
	return string(out.LocationConstraint), nil
}
