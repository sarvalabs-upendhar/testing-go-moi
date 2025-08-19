package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarvalabs/go-moi/jsonrpc/api"

	"github.com/sarvalabs/go-moi/consensus/transport"

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
	"github.com/sarvalabs/go-moi/jsonrpc/backend"
	"github.com/sarvalabs/go-moi/lattice"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/syncer/forage"
)

func (n *Node) newStateManager(cacheStateObjects bool) (*state.StateManager, error) {
	return state.NewStateManager(n.db, n.logger, n.cache, n.nodeMetrics.state, n.cfg.State, cacheStateObjects)
}

// setupCacheStore creates new lru cache store and setups it to node
func (n *Node) setupCacheStore() (err error) {
	if n.cache, err = lru.New(lruSize); err != nil {
		return err
	}

	return nil
}

// setupGenesis calls SetupGenesis method in chain if SkipGenesis is false in config
func (n *Node) setupGenesis() (err error) {
	if err = n.kramaEngine.SetupGenesis(); err != nil {
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

func (n *Node) setupReputationEngine() (err error) {
	nodeMetaInfo := &senatus.NodeMetaInfo{
		KramaID:    n.vault.KramaID(),
		Addrs:      utils.MultiAddrToString(n.network.GetAddrs()...),
		NTQ:        1,
		Registered: true,
	}

	sm, err := n.newStateManager(false)
	if err != nil {
		return err
	}

	n.senatus, err = senatus.NewReputationEngine(
		n.logger,
		n.db,
		nodeMetaInfo,
		n.eventMux,
		sm,
	)
	if err != nil {
		return err
	}

	return nil
}

// setupExecEngine creates new ExecutionEngine object and setups it to node
func (n *Node) setupExecEngine() {
	n.exec = compute.NewManager(n.logger, n.cfg.Execution, n.nodeMetrics.compute)
}

// setupIxPool creates new InteractionPool object and setups it to node
func (n *Node) setupIxPool() error {
	sm, err := n.newStateManager(true)
	if err != nil {
		return err
	}

	n.ixpool, err = ixpool.NewIxPool(
		n.logger,
		n.eventMux,
		n.network,
		sm,
		n.cfg.IxPool,
		n.nodeMetrics.ixpool,
		crypto.Verify,
		n.cfg.Consensus.GenesisTimestamp,
	)
	if err != nil {
		return err
	}

	return nil
}

// storeStaticPeersInSenatus stores static peers in senatus
func (n *Node) storeStaticPeersInSenatus() error {
	n.network.Senatus = n.senatus

	for _, staticPeer := range n.cfg.Network.StaticPeers {
		if err := n.network.Senatus.UpdatePeer(&senatus.NodeMetaInfo{
			KramaID:    staticPeer.ID,
			Addrs:      utils.MultiAddrToString(staticPeer.Address),
			NTQ:        senatus.DefaultPeerNTQ,
			Registered: true,
		}); err != nil {
			return err
		}
	}

	return nil
}

// storeTrustedPeersInSenatus stores trusted peers in senatus
func (n *Node) storeTrustedPeersInSenatus() error {
	for _, trustedPeer := range n.cfg.Consensus.TrustedPeers {
		if err := n.network.Senatus.UpdatePeer(&senatus.NodeMetaInfo{
			KramaID:    trustedPeer.ID,
			Addrs:      utils.MultiAddrToString(trustedPeer.Address),
			NTQ:        senatus.DefaultPeerNTQ,
			Registered: true,
		}); err != nil {
			return err
		}
	}

	return nil
}

// setupRandomizer creates new Randomizer object and setups it to node
func (n *Node) setupRandomizer() {
	n.handlers.flux = flux.NewRandomizer(n.logger, n.network, n.senatus, n.nodeMetrics.flux)
}

// setupChainManager creates a new Chain Manager object
func (n *Node) setupChainManager() (err error) {
	if n.chain, err = lattice.NewChainManager(
		n.db,
		n.logger,
		n.eventMux,
		n.network,
		n.ixpool,
		n.cache,
		n.senatus,
		n.nodeMetrics.chain,
	); err != nil {
		return err
	}

	return nil
}

func (n *Node) setupChainManagerToSenatus() {
	n.senatus.Chain = n.chain
}

// setupCompressor instantiates a new zstd compressor
func (n *Node) setupCompressor() (err error) {
	n.compressor, err = common.NewZstdCompressor(n.logger)
	if err != nil {
		return err
	}

	return nil
}

// setupKramaEngine instantiates transport and krama engine
func (n *Node) setupKramaEngine(sm *state.StateManager) (err error) {
	kramaTransport := transport.NewKramaTransport(
		n.network.GetKramaID(),
		n.logger,
		n.nodeMetrics.transport,
		n.network,
		n.network.ConnManager,
		n.cfg.Consensus.MinGossipPeers, n.cfg.Consensus.MaxGossipPeers,
	)

	if n.kramaEngine, err = consensus.NewKramaEngine(
		n.db,
		n.cfg.Consensus,
		n.logger,
		n.eventMux,
		n.network.GetKramaID(),
		sm,
		n.exec,
		n.ixpool,
		n.vault,
		n.chain,
		n.handlers.flux,
		kramaTransport,
		n.nodeMetrics.krama,
		n.consensusSlots,
		crypto.VerifyAggregateSignature,
		n.compressor,
	); err != nil {
		return err
	}

	return nil
}

// setupSyncer creates a syncer object which includes agora and forage services
func (n *Node) setupSyncer(sm *state.StateManager) (err error) {
	agoraInstance, err := agora.NewAgora(n.logger, n.db, n.network, n.nodeMetrics.agora, n.compressor)
	if err != nil {
		return errors.Wrap(err, "error initiating agora")
	}

	if n.syncer, err = forage.NewSyncer(
		n.cfg.Syncer,
		n.logger,
		n.network,
		n.eventMux,
		n.db,
		n.kramaEngine,
		n.chain,
		sm,
		n.ixpool,
		n.consensusSlots,
		n.lastActiveTimestamp,
		n.nodeMetrics.syncer,
		agoraInstance,
		n.compressor,
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
	sm, err := n.newStateManager(false)
	if err != nil {
		return err
	}

	newBackend := backend.NewBackend(n.ixpool, n.chain, n.exec, sm, n.syncer, n.network, n.db)

	filterMan := jsonrpc.NewFilterManager(n.logger, n.eventMux, n.cfg.JSONRPC, newBackend)

	n.rpc = jsonrpc.NewRPCServer("/", n.logger, n.cfg, filterMan)

	for _, publicAPI := range api.GetPublicAPIs(newBackend, filterMan) {
		if err := n.rpc.RegisterService(publicAPI.Namespace, publicAPI.Services); err != nil {
			n.logger.Error("register service error:", err)

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
