package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/account"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// OnConfigSwitch is called after SwitchAwsConfig finishes resetting clients.
// view package sets this to kind.ResetAll during app init.
var OnConfigSwitch func()

// Store is the legacy god-struct for the API layer. Phase 5 introduces
// Clients alongside it; production builds (NewStore) route every client
// through Clients so construction lives in one place. Per-service field
// pointers (ecs, lambda, ...) are retained because:
//
//   - cluster.go / service.go / task.go etc. read store.ecs directly.
//   - api tests construct Store via struct literal, e.g.
//     &Store{Config: &cfg, lambda: c}, and rely on field names being stable.
//
// PR-B migrates kinds onto Clients accessors and PR-C deletes Store along
// with the legacy initXClient helpers and the field shadows.
type Store struct {
	*aws.Config
	clients        *Clients
	ecs            *ecs.Client
	cloudwatch     *cloudwatch.Client
	cloudwatchlogs *cloudwatchlogs.Client
	autoScaling    *applicationautoscaling.Client
	ssm            *ssm.Client
	account        *account.Client
	lambda         *lambda.Client
	sqs            *sqs.Client
	dynamodb       *dynamodb.Client
}

func NewStore(profile string, region string) (*Store, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		slog.Error("failed to load aws SDK config", "error", err)
		return nil, err
	}
	clients := NewClients(cfg)
	slog.Info("load config", slog.String("AWS_PROFILE", profile), slog.String("AWS_REGION", cfg.Region))
	return &Store{
		Config:  &cfg,
		clients: clients,
		ecs:     clients.ECS(),
	}, nil
}

// init*Client helpers return the resolved client. Each captures store.X into
// a local BEFORE the nil-check so a concurrent SwitchAwsConfig nilling the
// field can't make us return nil. Without this guard, the sequence
//
//	T1 init: if store.lambda == nil { store.lambda = NewFromConfig(...) }
//	T2 SwitchAwsConfig: store.lambda = nil
//	T1 init: return store.lambda    // returns nil → caller panics
//
// crashes the app on profile switch when a Preload goroutine is in flight.
// Callers MUST use the returned local, never re-read store.lambda/etc.
//
// In production builds store.clients is non-nil and construction is delegated
// to it (single source of truth + its own mutex). Tests construct Store via
// struct literal with the per-service field already populated, so the
// store.X cache hit returns the test client and store.clients is never
// dereferenced.
func (store *Store) initCloudwatchClient() *cloudwatch.Client {
	c := store.cloudwatch
	if c == nil {
		c = store.clients.CloudWatch()
		store.cloudwatch = c
	}
	return c
}

func (store *Store) initCloudwatchlogsClient() *cloudwatchlogs.Client {
	c := store.cloudwatchlogs
	if c == nil {
		c = store.clients.CloudWatchLogs()
		store.cloudwatchlogs = c
	}
	return c
}

func (store *Store) initSsmClient() *ssm.Client {
	c := store.ssm
	if c == nil {
		c = store.clients.SSM()
		store.ssm = c
	}
	return c
}

func (store *Store) initAccountClient() *account.Client {
	c := store.account
	if c == nil {
		c = store.clients.Account()
		store.account = c
	}
	return c
}

func (store *Store) initAutoScalingClient() *applicationautoscaling.Client {
	c := store.autoScaling
	if c == nil {
		c = store.clients.AutoScaling()
		store.autoScaling = c
	}
	return c
}

func (store *Store) initLambdaClient() *lambda.Client {
	c := store.lambda
	if c == nil {
		c = store.clients.Lambda()
		store.lambda = c
	}
	return c
}

func (store *Store) initSqsClient() *sqs.Client {
	c := store.sqs
	if c == nil {
		c = store.clients.SQS()
		store.sqs = c
	}
	return c
}

func (store *Store) initDynamoDBClient() *dynamodb.Client {
	c := store.dynamodb
	if c == nil {
		c = store.clients.DynamoDB()
		store.dynamodb = c
	}
	return c
}
