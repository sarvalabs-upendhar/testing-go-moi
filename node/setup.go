package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/syncer/agora"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/consensus"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/flux"
	"github.com/sarvalabs/go-moi/ixpool"
	"github.com/sarvalabs/go-moi/jsonrpc"
	"github.com/sarvalabs/go-moi/jsonrpc/api"
	"github.com/sarvalabs/go-moi/lattice"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/syncer/forage"
)

// setupCacheStore creates new lru cache store and setups it to node
func (n *Node) setupCacheStore() (err error) {
	if n.cache, err = lru.New(lruSize); err != nil {
		return err
	}

	return nil
}

// setupGenesis calls SetupGenesis method in chain if SkipGenesis is false in config
func (n *Node) setupGenesis() (err error) {
	if err = n.chain.SetupGenesis(n.cfg.Chain.GenesisFilePath); err != nil {
		return err
	}

	return nil
}

// setupVault creates new vault and setups it to node
func (n *Node) setupVault() (err error) {
	n.vault, err = crypto.NewVault(n.cfg.Vault, validatorType, kramaIDVersion)
	if err != nil {
		return errors.Wrap(common.ErrVaultInit, err.Error())
	}

	return nil
}

// setupConsensusSlots creates a slots instance with the given operator and validator count
func (n *Node) setupConsensusSlots() {
	n.consensusSlots = ktypes.NewSlots(n.cfg.Consensus.OperatorSlotCount, n.cfg.Consensus.ValidatorSlotCount)
}

// setupServer creates new server object and setups it to node
func (n *Node) setupNetwork() error {
	n.network = p2p.NewServer(
		n.logger,
		n.vault.KramaID(),
		n.eventMux,
		n.cfg.Network,
		n.vault,
		n.nodeMetrics.server,
	)

	return n.network.SetupServer()
}

// setupPersistenceManager creates new dhruva(db) object and setups it to node
func (n *Node) setupPersistenceManager() (err error) {
	n.db, err = storage.NewPersistenceManager(n.logger, n.cfg.DB, n.nodeMetrics.storage)
	if err != nil {
		return err
	}

	return nil
}

// setupStateManager creates new StateManager object and setups it to node
func (n *Node) setupStateManager() (err error) {
	n.state, err = state.NewStateManager(n.db, n.logger, n.cache, n.nodeMetrics.guna, n.senatus)
	if err != nil {
		return err
	}

	return nil
}

func (n *Node) setupReputationEngine() (err error) {
	nodeMetaInfo := &senatus.NodeMetaInfo{
		KramaID:   n.vault.KramaID(),
		Addrs:     utils.MultiAddrToString(n.network.GetAddrs()...),
		NTQ:       1,
		PublicKey: n.vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
	}

	n.senatus, err = senatus.NewReputationEngine(
		n.logger,
		n.network,
		n.db,
		nodeMetaInfo,
	)
	if err != nil {
		return err
	}

	return nil
}

// setupExecEngine creates new ExecutionEngine object and setups it to node
func (n *Node) setupExecEngine() {
	n.exec = compute.NewManager(n.state, n.logger, n.cfg.Execution)
}

// setupIxPool creates new InteractionPool object and setups it to node
func (n *Node) setupIxPool() {
	n.ixpool = ixpool.NewIxPool(
		n.logger,
		n.eventMux,
		n.state,
		n.exec,
		n.cfg.IxPool,
		n.nodeMetrics.ixpool,
		crypto.Verify,
	)
}

// setupSenatusToNetwork fetches Senatus from state and setups it to node's network manager(poorna server)
func (n *Node) setupSenatusToNetwork() error {
	n.network.Senatus = n.senatus

	for _, staticPeer := range n.cfg.Network.StaticPeers {
		err := n.network.Senatus.UpdatePeer(&senatus.NodeMetaInfo{
			KramaID: staticPeer.ID,
			Addrs:   utils.MultiAddrToString(staticPeer.Address),
			NTQ:     senatus.DefaultPeerNTQ,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// setupRandomizer creates new Randomizer object and setups it to node
func (n *Node) setupRandomizer() {
	n.handlers.flux = flux.NewRandomizer(n.logger, n.network, n.senatus, n.nodeMetrics.flux)
}

// setupChainManager creates new Chain Manager object and setups it to node
func (n *Node) setupChainManager() (err error) {
	if n.chain, err = lattice.NewChainManager(
		n.cfg.Chain,
		n.db,
		n.state,
		n.logger,
		n.eventMux,
		n.network,
		n.ixpool,
		n.cache,
		n.exec,
		n.senatus,
		n.nodeMetrics.chain,
		crypto.VerifyAggregateSignature,
	); err != nil {
		return err
	}

	return nil
}

// setupKramaEngine creates new Krama Engine object and setups it to node
func (n *Node) setupKramaEngine() (err error) {
	if n.kramaEngine, err = consensus.NewKramaEngine(
		n.cfg.Consensus,
		n.logger,
		n.eventMux,
		n.state,
		n.network,
		n.exec,
		n.ixpool,
		n.vault,
		n.chain,
		n.handlers.flux,
		n.nodeMetrics.krama,
		n.consensusSlots,
	); err != nil {
		return err
	}

	return nil
}

// setupSyncer creates new Syncer object and setups it to node
func (n *Node) setupSyncer() (err error) {
	agoraInstance, err := agora.NewAgora(n.logger, n.db, n.network, n.nodeMetrics.agora)
	if err != nil {
		return errors.Wrap(err, "error initiating agora")
	}

	if n.syncer, err = forage.NewSyncer(
		n.cfg.Syncer,
		n.logger,
		n.network,
		n.eventMux,
		n.db,
		n.chain,
		n.state,
		n.consensusSlots,
		n.lastActiveTimestamp,
		n.nodeMetrics.syncer,
		agoraInstance,
	); err != nil {
		return err
	}

	return nil
}

func (n *Node) setLogger(logLevel string) error {
	if n.cfg.LogFilePath == "" {
		n.logger = hclog.New(&hclog.LoggerOptions{
			Name:  "MOI",
			Level: hclog.LevelFromString(logLevel),
		})

		return nil
	}

	err := utils.EnsureDir(n.cfg.LogFilePath, 0o700)
	if err != nil {
		return fmt.Errorf("failed to ensure log path is in place: %w", err)
	}

	// make sure paths are combined properly regardless of extra slash
	logFileName := filepath.Join(n.cfg.LogFilePath, string(n.vault.KramaID()))

	fileName, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}

	n.logger = hclog.New(&hclog.LoggerOptions{
		Name:   "MOI",
		Output: fileName,
		Level:  hclog.LevelFromString(logLevel),
	})

	return nil
}

// setupRPC sets JSON-RPC
func (n *Node) setupRPC() error {
	n.rpc = jsonrpc.NewRPCServer("/", n.logger, n.cfg.Network, n.eventMux)

	backend := api.NewBackend(
		n.ixpool,
		n.chain,
		n.exec,
		n.state,
		n.syncer,
		n.network,
		n.db,
		n.cfg.IxPool,
	)

	for _, publicAPI := range api.GetPublicAPIs(backend) {
		rpcService := jsonrpc.NewRPCService()

		if err := rpcService.RegisterAPIs(publicAPI.Services); err != nil {
			return err
		}

		if err := n.rpc.RegisterService(publicAPI.Namespace, rpcService); err != nil {
			return err
		}
	}

	return nil
}

func (n *Node) setupTelemetry() {
	if n.cfg.Metrics.PrometheusAddr == nil {
		n.nodeMetrics = metricProvider("MOI", "test-net", false)

		return
	}

	n.nodeMetrics = metricProvider("MOI", "test-net", true)
	n.prometheusServer = n.startPrometheusServer(n.cfg.Metrics.PrometheusAddr)
}

func (n *Node) stopTelemetry() {
	if n.prometheusServer == nil {
		return
	}

	if err := n.prometheusServer.Shutdown(context.Background()); err != nil {
		n.logger.Error("Prometheus server shutdown error", "err", err)
	}
}
