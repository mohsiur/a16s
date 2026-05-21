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

type Store struct {
	*aws.Config
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
	ecsClient := ecs.NewFromConfig(cfg)
	slog.Info("load config", slog.String("AWS_PROFILE", profile), slog.String("AWS_REGION", cfg.Region))
	return &Store{
		Config: &cfg,
		ecs:    ecsClient,
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
func (store *Store) initCloudwatchClient() *cloudwatch.Client {
	c := store.cloudwatch
	if c == nil {
		c = cloudwatch.NewFromConfig(*store.Config)
		store.cloudwatch = c
	}
	return c
}

func (store *Store) initCloudwatchlogsClient() *cloudwatchlogs.Client {
	c := store.cloudwatchlogs
	if c == nil {
		c = cloudwatchlogs.NewFromConfig(*store.Config)
		store.cloudwatchlogs = c
	}
	return c
}

func (store *Store) initSsmClient() *ssm.Client {
	c := store.ssm
	if c == nil {
		c = ssm.NewFromConfig(*store.Config)
		store.ssm = c
	}
	return c
}

func (store *Store) initAccountClient() *account.Client {
	c := store.account
	if c == nil {
		c = account.NewFromConfig(*store.Config)
		store.account = c
	}
	return c
}

func (store *Store) initAutoScalingClient() *applicationautoscaling.Client {
	c := store.autoScaling
	if c == nil {
		c = applicationautoscaling.NewFromConfig(*store.Config)
		store.autoScaling = c
	}
	return c
}

func (store *Store) initLambdaClient() *lambda.Client {
	c := store.lambda
	if c == nil {
		c = lambda.NewFromConfig(*store.Config)
		store.lambda = c
	}
	return c
}

func (store *Store) initSqsClient() *sqs.Client {
	c := store.sqs
	if c == nil {
		c = sqs.NewFromConfig(*store.Config)
		store.sqs = c
	}
	return c
}

func (store *Store) initDynamoDBClient() *dynamodb.Client {
	c := store.dynamodb
	if c == nil {
		c = dynamodb.NewFromConfig(*store.Config)
		store.dynamodb = c
	}
	return c
}
