package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// OnConfigSwitch is called after SwitchAwsConfig finishes resetting clients.
// view package sets this to kind.ResetAll during app init.
var OnConfigSwitch func()

// Store is the legacy entry point for the API layer. Phase 5 migrated every
// per-service method onto *Clients; Store now embeds *Clients so existing
// callers (app.Store.ListClusters() etc.) keep compiling via Go's method
// promotion. PR-C deletes Store entirely and rewrites callers to hold
// *Clients directly.
type Store struct {
	*aws.Config
	*Clients
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
		Clients: clients,
	}, nil
}
