package bgclient

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/battleground/server/warzone/infrastructure"

	"github.com/sarvalabs/go-moi/common/tests"
)

var logger hclog.Logger

func init() {
	logger = hclog.New(&hclog.LoggerOptions{
		Name:  "E2E",
		Level: hclog.LevelFromString("ERROR"),
	})
}

type LocalNetwork struct {
	clusterConfig *ClusterConfig
	serverConfigs []*ServerConfig
	cluster       *Cluster
}

func NewLocalNetwork(clusterConfig *ClusterConfig, serverConfigs []*ServerConfig) *LocalNetwork {
	if serverConfigs != nil {
		if len(serverConfigs) != clusterConfig.ValidatorCount {
			log.Fatal("server configs should be provided for all validator nodes")
		}
	}

	return &LocalNetwork{
		clusterConfig: clusterConfig,
		serverConfigs: serverConfigs,
	}
}

func (local *LocalNetwork) ServerStatus(ctx context.Context) error {
	return nil
}

func (local *LocalNetwork) StartNetwork(ctx context.Context) ([]Moipod, error) {
	c, err := NewTestCluster(local.clusterConfig, local.serverConfigs)
	if err != nil {
		return nil, err
	}

	local.cluster = c

	moipods := make([]Moipod, 0, len(local.cluster.Servers))

	for rpc := range local.cluster.Servers {
		moipods = append(moipods, Moipod{URL: rpc})
	}

	return moipods, nil
}

// TODO: Revisit the functionality
func (local *LocalNetwork) NetworkStatus(ctx context.Context) (infrastructure.Status, error) {
	if local.cluster != nil {
		return infrastructure.Active, nil
	}

	return infrastructure.Inactive, nil
}

func (local *LocalNetwork) Accounts(ctx context.Context) ([]tests.AccountWithMnemonic, error) {
	return local.cluster.Accounts(), nil
}

func (local *LocalNetwork) JSONRpcUrls(ctx context.Context) ([]string, error) {
	urls := make([]string, 0, len(local.cluster.Servers))

	for rpc := range local.cluster.Servers {
		urls = append(urls, rpc)
	}

	return urls, nil
}

func (local *LocalNetwork) DestroyNetwork(ctx context.Context, removeFolders bool) error {
	if local.cluster != nil {
		local.cluster.Stop()
		local.cluster = nil
	}

	if removeFolders {
		rootDir := "." // Change this to the directory where you want to start the removal
		pattern := "t*"

		dirs, err := filepath.Glob(filepath.Join(rootDir, pattern))
		if err != nil {
			logger.Error("", err)

			return err
		}

		for _, dir := range dirs {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				logger.Debug("Removing directory: ", dir)

				if err := os.RemoveAll(dir); err != nil {
					logger.Error("", err)
				}
			}
		}
	}

	return nil
}

func (local *LocalNetwork) StartNode(ctx context.Context, rpcAddr string, cleanDB bool) error {
	srv, ok := local.cluster.Servers[rpcAddr]
	if !ok {
		return errors.New("rpc address not found")
	}

	srv.Config.CleanDB = strconv.FormatBool(cleanDB)

	srv.Start(local.cluster.Config.EnableDebugMode)

	return nil
}

func (local *LocalNetwork) StopNode(ctx context.Context, rpcAddr string) error {
	srv, ok := local.cluster.Servers[rpcAddr]
	if !ok {
		return errors.New("rpc address not found")
	}

	srv.Stop()

	return nil
}

func (local *LocalNetwork) UpdateNetwork(ctx context.Context, commitHash string) error {
	return nil
}
