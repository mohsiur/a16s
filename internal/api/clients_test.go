package api

import (
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// TestClientsLazyAndCached pins the contract Store relies on: ECS is built
// eagerly by NewClients; every other accessor is built on first call and
// cached for subsequent calls (same pointer). PR-B's migration off Store
// presumes this caching so kinds can call Clients.Lambda() per request
// without re-paying construction.
func TestClientsLazyAndCached(t *testing.T) {
	c := NewClients(aws.Config{Region: "us-east-1"})

	if c.ECS() == nil {
		t.Fatal("ECS() returned nil after NewClients")
	}
	if got := c.ECS(); got != c.ECS() {
		t.Fatal("ECS() returned different pointers across calls")
	}

	if got := c.Lambda(); got != c.Lambda() {
		t.Fatal("Lambda() not cached")
	}
	if got := c.SQS(); got != c.SQS() {
		t.Fatal("SQS() not cached")
	}
	if got := c.DynamoDB(); got != c.DynamoDB() {
		t.Fatal("DynamoDB() not cached")
	}
	if got := c.CloudWatch(); got != c.CloudWatch() {
		t.Fatal("CloudWatch() not cached")
	}
	if got := c.CloudWatchLogs(); got != c.CloudWatchLogs() {
		t.Fatal("CloudWatchLogs() not cached")
	}
	if got := c.AutoScaling(); got != c.AutoScaling() {
		t.Fatal("AutoScaling() not cached")
	}
	if got := c.SSM(); got != c.SSM() {
		t.Fatal("SSM() not cached")
	}
	if got := c.Account(); got != c.Account() {
		t.Fatal("Account() not cached")
	}
}

// TestClientsSwitchConfigRebuilds pins the post-switch invariant the legacy
// SwitchAwsConfig relied on: ECS is rebuilt against the new config (different
// pointer), every other client is reset and built fresh on next access.
func TestClientsSwitchConfigRebuilds(t *testing.T) {
	c := NewClients(aws.Config{Region: "us-east-1"})

	beforeECS := c.ECS()
	beforeLambda := c.Lambda()

	c.SwitchConfig(aws.Config{Region: "us-west-2"})

	if c.ECS() == beforeECS {
		t.Fatal("ECS() not rebuilt after SwitchConfig")
	}
	if c.Lambda() == beforeLambda {
		t.Fatal("Lambda() not rebuilt after SwitchConfig")
	}
	if got := c.Config().Region; got != "us-west-2" {
		t.Fatalf("Config().Region = %q; want us-west-2", got)
	}
}

// TestClientsConcurrentAccess exercises the mutex: under -race, parallel
// callers to a still-uninitialised accessor must not produce a data race or
// a nil return. This is the property that motivated replacing the
// "local capture before nil-check" pattern in store.go.
func TestClientsConcurrentAccess(t *testing.T) {
	c := NewClients(aws.Config{Region: "us-east-1"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.Lambda() == nil {
				t.Error("Lambda() returned nil under concurrent access")
			}
			if c.SQS() == nil {
				t.Error("SQS() returned nil under concurrent access")
			}
		}()
	}
	wg.Wait()
}
