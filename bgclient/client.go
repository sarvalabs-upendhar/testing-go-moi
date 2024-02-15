package bgclient

import (
	"context"
	"net/http"

	"github.com/sarvalabs/battleground/server/warzone/infrastructure"

	"github.com/sarvalabs/go-moi/common/tests"
)

type Client interface {
	ServerStatus(ctx context.Context) error
	StartNetwork(ctx context.Context) ([]Moipod, error)
	NetworkStatus(ctx context.Context) (infrastructure.Status, error)
	Accounts(ctx context.Context) ([]tests.AccountWithMnemonic, error)
	JSONRpcUrls(ctx context.Context) ([]string, error)
	DestroyNetwork(ctx context.Context, removeFolders bool) error
	StartNode(ctx context.Context, rpcAddr string, cleanDB bool) error
	StopNode(ctx context.Context, rpcAddr string) error
	UpdateNetwork(ctx context.Context, commitHash string) error
}

func NewClient(cfg *Config) Client {
	switch cfg.Network {
	case LOCAL:
		return NewLocalNetwork(cfg.ClusterConfig, cfg.ServerConfigs)
	case CLOUD:
		return NewCloudNetwork(cfg, &http.Client{
			Timeout: cfg.DialTimeout,
		})
	default:
		panic("unexpected network type used!")
	}
}
