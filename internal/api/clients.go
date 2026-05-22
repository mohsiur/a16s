package api

import (
	"context"
	"log/slog"
	"sync"

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

// Clients is the canonical per-service AWS client factory. Each accessor
// (Lambda, SQS, ...) returns a lazily-built client constructed against the
// current aws.Config. SwitchConfig swaps the config and resets every lazy
// client so the next accessor call rebuilds against the new credentials.
//
// Concurrency: Lambda()/SQS()/... are safe for concurrent callers via mu.
// SwitchConfig also takes mu so a concurrent accessor either sees the old
// config (and returns the old client) or waits and gets a fresh one — never
// nil.
type Clients struct {
	mu  sync.Mutex
	cfg aws.Config

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

// NewClients eagerly builds the ECS client (it's hot on every cluster page)
// and leaves every other service for lazy construction.
func NewClients(cfg aws.Config) *Clients {
	return &Clients{
		cfg: cfg,
		ecs: ecs.NewFromConfig(cfg),
	}
}

// NewAWSClients loads the default AWS SDK config for the given profile/region
// and returns a *Clients that lazy-builds per-service clients on demand.
func NewAWSClients(profile string, region string) (*Clients, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		slog.Error("failed to load aws SDK config", "error", err)
		return nil, err
	}
	slog.Info("load config", slog.String("AWS_PROFILE", profile), slog.String("AWS_REGION", cfg.Region))
	return NewClients(cfg), nil
}

// Config returns the active AWS config. Callers should not mutate it.
func (c *Clients) Config() aws.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// SwitchConfig swaps in a new AWS config and resets every lazy client so
// subsequent accessor calls rebuild against the new config. ECS is rebuilt
// eagerly to match NewClients.
func (c *Clients) SwitchConfig(cfg aws.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	c.ecs = ecs.NewFromConfig(cfg)
	c.cloudwatch = nil
	c.cloudwatchlogs = nil
	c.autoScaling = nil
	c.ssm = nil
	c.account = nil
	c.lambda = nil
	c.sqs = nil
	c.dynamodb = nil
}

func (c *Clients) ECS() *ecs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ecs == nil {
		c.ecs = ecs.NewFromConfig(c.cfg)
	}
	return c.ecs
}

func (c *Clients) CloudWatch() *cloudwatch.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cloudwatch == nil {
		c.cloudwatch = cloudwatch.NewFromConfig(c.cfg)
	}
	return c.cloudwatch
}

func (c *Clients) CloudWatchLogs() *cloudwatchlogs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cloudwatchlogs == nil {
		c.cloudwatchlogs = cloudwatchlogs.NewFromConfig(c.cfg)
	}
	return c.cloudwatchlogs
}

func (c *Clients) AutoScaling() *applicationautoscaling.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.autoScaling == nil {
		c.autoScaling = applicationautoscaling.NewFromConfig(c.cfg)
	}
	return c.autoScaling
}

func (c *Clients) SSM() *ssm.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ssm == nil {
		c.ssm = ssm.NewFromConfig(c.cfg)
	}
	return c.ssm
}

func (c *Clients) Account() *account.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.account == nil {
		c.account = account.NewFromConfig(c.cfg)
	}
	return c.account
}

func (c *Clients) Lambda() *lambda.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lambda == nil {
		c.lambda = lambda.NewFromConfig(c.cfg)
	}
	return c.lambda
}

func (c *Clients) SQS() *sqs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sqs == nil {
		c.sqs = sqs.NewFromConfig(c.cfg)
	}
	return c.sqs
}

func (c *Clients) DynamoDB() *dynamodb.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dynamodb == nil {
		c.dynamodb = dynamodb.NewFromConfig(c.cfg)
	}
	return c.dynamodb
}

// ClientsWith*ForTest helpers seed a Clients with a pre-built service
// client so middleware-mocked tests can target a specific AWS API. Other
// accessors will lazy-build against the supplied cfg as usual.
func ClientsWithLambdaForTest(cfg aws.Config, c *lambda.Client) *Clients {
	return &Clients{cfg: cfg, lambda: c}
}

func ClientsWithSqsForTest(cfg aws.Config, c *sqs.Client) *Clients {
	return &Clients{cfg: cfg, sqs: c}
}

func ClientsWithDynamoDBForTest(cfg aws.Config, c *dynamodb.Client) *Clients {
	return &Clients{cfg: cfg, dynamodb: c}
}
