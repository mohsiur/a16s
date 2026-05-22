package api

import (
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
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

// Clients is the canonical per-service AWS client factory. Each accessor
// (Lambda, SQS, ...) returns a lazily-built client constructed against the
// current aws.Config. SwitchConfig swaps the config and resets every lazy
// client so the next accessor call rebuilds against the new credentials.
//
// Phase 5 introduces this alongside Store. Store keeps its public API and
// delegates client construction here; PR-B migrates kinds onto Clients
// directly and PR-C deletes Store. Today both types coexist.
//
// Concurrency: Lambda()/SQS()/... are safe for concurrent callers via mu.
// SwitchConfig also takes mu so a concurrent accessor either sees the old
// config (and returns the old client) or waits and gets a fresh one — never
// nil. This eliminates the read-then-nil race that the legacy initXClient
// helpers in store.go had to work around with local-variable capture.
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

// NewClients eagerly builds the ECS client (matching NewStore which has
// always pre-built it) and leaves every other service for lazy construction.
func NewClients(cfg aws.Config) *Clients {
	return &Clients{
		cfg: cfg,
		ecs: ecs.NewFromConfig(cfg),
	}
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
