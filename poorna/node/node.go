package node

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/guna/senatus"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/jug"
	"github.com/sarvalabs/moichain/krama"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/mudra"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/poorna"
	"github.com/sarvalabs/moichain/poorna/api"
	"github.com/sarvalabs/moichain/poorna/flux"
	krpc "github.com/sarvalabs/moichain/poorna/rpc"
	"github.com/sarvalabs/moichain/poorna/syncer"
	"github.com/sarvalabs/moichain/types"
	"github.com/sarvalabs/moichain/utils"
)

const (
	lruSize           = 2000
	validatorType     = 1
	kramaIDVersion    = 1
	syncMode          = "full"
	readHeaderTimeout = 5 * time.Second
)

type SubHandlers struct {
	syncer *syncer.Syncer
	core   *poorna.SubHandler
	flux   *flux.Randomizer
}

type Node struct {
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	logger              hclog.Logger
	cfg                 *common.Config
	eventMux            *utils.TypeMux
	network             *poorna.Server
	state               *guna.StateManager
	chain               *lattice.ChainManager
	senatus             *senatus.ReputationEngine
	exec                *jug.ExecutionManager
	kramaEngine         *krama.Engine
	db                  *dhruva.PersistenceManager
	ixpool              *ixpool.IxPool
	handlers            *SubHandlers
	cache               *lru.Cache
	rpc                 *krpc.Server
	nodeMetrics         *nodeMetrics
	prometheusServer    *http.Server
	vault               *mudra.KramaVault
	consensusSlots      *ktypes.Slots
	lastActiveTimestamp uint64
}

func NewNode(logLevel string, cfg *common.Config) (n *Node, err error) {
	n = &Node{
		cfg:      cfg,
		eventMux: new(utils.TypeMux),
		handlers: new(SubHandlers),
	}
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())

	if err = n.setupCacheStore(); err != nil {
		return nil, err
	}

	if err = n.setupVault(); err != nil {
		return nil, errors.Wrap(types.ErrVaultInit, err.Error())
	}

	if err = n.setLogger(logLevel); err != nil {
		return nil, err
	}

	if err = n.setupPersistenceManager(); err != nil {
		return nil, err
	}

	if err = n.setupNetwork(); err != nil {
		return nil, err
	}

	n.setupConsensusSlots()
	n.setupTelemetry()
	n.loadLatestActiveTimeStamp()

	if err = n.setupReputationEngine(); err != nil {
		return nil, err
	}

	if err = n.setupStateManager(); err != nil {
		return nil, err
	}

	n.setupExecEngine()

	n.setupIxPool()

	if err = n.setupSenatusToNetwork(); err != nil {
		return nil, err
	}

	if err = n.network.StartServer(); err != nil {
		return nil, errors.Wrap(err, "failed to start the p2p server")
	}

	n.setupRandomizer()

	if err = n.setupChainManager(); err != nil {
		return nil, err
	}

	if err = n.setupKramaEngine(); err != nil {
		return nil, err
	}

	// setup JSON-RPC
	if err = n.setupRPC(); err != nil {
		return nil, types.ErrRPCFailed
	}

	if err = n.setupSyncer(); err != nil {
		return nil, errors.New("unable to create and setup syncer")
	}

	n.setupSubHandler()

	if err = n.setupGenesis(); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *Node) GetKramaID() id.KramaID {
	return n.network.GetKramaID()
}

func (n *Node) loadLatestActiveTimeStamp() {
	n.lastActiveTimestamp = n.db.GetLastActiveTimeStamp()
}

// Start starts network Stream handler, network's Discovery routine, Handlers, IxPool
// Krama engine, State manager, JSON-RPC server Chain manager
// returns any error invoked
func (n *Node) Start() (err error) {
	n.startHandlers()

	n.ixpool.Start()

	n.kramaEngine.Start()

	if err = n.senatus.Start(); err != nil {
		return errors.Wrap(err, "failed to start senatus")
	}

	// starting JSON-RPC server
	go n.startJSONRPCServer()

	if err := n.chain.Start(); err != nil {
		return errors.Wrap(err, "failed to start chain manager")
	}

	return nil
}

// startJsonRPCServer start JSON-RPC server and stops node when JSON-RPC server stops
func (n *Node) startJSONRPCServer() {
	err := n.rpc.Start()
	n.logger.Error("JSON RPC server stopped", "error", err)
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
	if err = n.chain.SetupGenesis(n.cfg.Chain.GenesisFilePath); err != nil {
		return err
	}

	return nil
}

// setupVault creates new vault and setups it to node
func (n *Node) setupVault() (err error) {
	n.vault, err = mudra.NewVault(n.cfg.Vault, validatorType, kramaIDVersion)
	if err != nil {
		return errors.Wrap(types.ErrVaultInit, err.Error())
	}

	return nil
}

// setupConsensusSlots creates a slots instance with the given operator and validator count
func (n *Node) setupConsensusSlots() {
	n.consensusSlots = ktypes.NewSlots(n.cfg.Consensus.OperatorSlotCount, n.cfg.Consensus.ValidatorSlotCount)
}

// setupServer creates new server object and setups it to node
func (n *Node) setupNetwork() error {
	n.network = poorna.NewServer(n.ctx, n.logger, n.vault.KramaID(), n.eventMux, n.cfg.Network, n.vault)

	return n.network.SetupServer()
}

// setupPersistenceManager creates new dhruva(db) object and setups it to node
func (n *Node) setupPersistenceManager() (err error) {
	n.db, err = dhruva.NewPersistenceManager(n.ctx, n.logger, n.cfg.DB)
	if err != nil {
		return err
	}

	return nil
}

// setupStateManager creates new StateManager object and setups it to node
func (n *Node) setupStateManager() (err error) {
	n.state, err = guna.NewStateManager(n.ctx, n.db, n.logger, n.cache, n.nodeMetrics.guna, n.senatus)
	if err != nil {
		return err
	}

	return nil
}

func (n *Node) setupReputationEngine() (err error) {
	nodeMetaInfo := &gtypes.NodeMetaInfo{
		Addrs:     utils.MultiAddrToString(n.network.GetAddrs()...),
		NTQ:       1,
		PublicKey: n.vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
	}

	n.senatus, err = senatus.NewReputationEngine(
		n.ctx,
		n.logger,
		n.network,
		n.db,
		n.vault.KramaID(),
		nodeMetaInfo,
	)
	if err != nil {
		return err
	}

	return nil
}

// setupExecEngine creates new ExecutionEngine object and setups it to node
func (n *Node) setupExecEngine() {
	n.exec = jug.NewExecutionManager(n.state, n.logger, n.cfg.Execution)
}

// setupIxPool creates new InteractionPool object and setups it to node
func (n *Node) setupIxPool() {
	n.ixpool = ixpool.NewIxPool(n.ctx, n.logger, n.eventMux, n.state, n.cfg.IxPool, n.nodeMetrics.ixpool, mudra.Verify)
}

// setupSenatusToNetwork fetches Senatus from state and setups it to node's network manager(poorna server)
func (n *Node) setupSenatusToNetwork() error {
	n.network.Senatus = n.senatus

	for _, staticPeer := range n.cfg.Network.StaticPeers {
		err := n.network.Senatus.AddNewPeer(staticPeer.ID, &gtypes.NodeMetaInfo{
			Addrs: utils.MultiAddrToString(staticPeer.Address),
			NTQ:   senatus.DefaultPeerNTQ,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// setupRandomizer creates new Randomizer object and setups it to node
func (n *Node) setupRandomizer() {
	n.handlers.flux = flux.NewRandomizer(n.ctx, n.logger, n.network, n.nodeMetrics.flux)
}

// setupChainManager creates new Chain Manager object and setups it to node
func (n *Node) setupChainManager() (err error) {
	if n.chain, err = lattice.NewChainManager(
		n.ctx,
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
		mudra.VerifyAggregateSignature,
	); err != nil {
		return err
	}

	return nil
}

// setupKramaEngine creates new Krama Engine object and setups it to node
func (n *Node) setupKramaEngine() (err error) {
	if n.kramaEngine, err = krama.NewKramaEngine(
		n.ctx,
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
	if n.handlers.syncer, err = syncer.NewSyncer(
		n.ctx,
		n.cfg.Syncer,
		n.logger,
		n.network,
		n.eventMux,
		n.db,
		n.chain,
		n.state,
		n.nodeMetrics.agora,
		n.consensusSlots,
		n.lastActiveTimestamp,
		n.nodeMetrics.syncer,
	); err != nil {
		return err
	}

	return nil
}

// setupSubHandler creates new poorna SubHandler object and setups it to node's handler's core
func (n *Node) setupSubHandler() {
	n.handlers.core = poorna.NewSubHandler(
		n.ctx,
		n.network.GetKramaID(),
		n.logger,
		n.network,
		n.network.Peers,
		n.eventMux,
		n.ixpool,
		n.chain,
	)
}

func (n *Node) setLogger(logLevel string) error {
	if n.cfg.LogFilePath == "" {
		n.logger = hclog.New(&hclog.LoggerOptions{
			Name:  "MOI",
			Level: hclog.LevelFromString(logLevel),
		})

		return nil
	}

	logFileName := n.cfg.LogFilePath + string(n.vault.KramaID())

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
	n.rpc = krpc.NewRPCServer("/", n.logger, n.cfg.Network, n.eventMux)

	backend := api.NewBackend(n.ixpool, n.chain, n.exec, n.state, n.network, n.db, n.cfg.IxPool)

	publicApis := api.GetPublicAPIs(backend)

	for _, api := range publicApis {
		rpcService := krpc.NewRPCService()

		if err := rpcService.RegisterAPIs(api.Services); err != nil {
			return err
		}

		if err := n.rpc.RegisterService(api.Namespace, rpcService); err != nil {
			return err
		}
	}

	return nil
}

// startHandlers starts syncer, core and flux(randomizer)
func (n *Node) startHandlers() {
	n.logger.Info("Starting Sub-Handlers")

	go n.handlers.core.Start()

	go n.handlers.syncer.Start()

	go n.handlers.flux.Start()
}

// stopHandlers stops syncer, core and flux(randomizer)
func (n *Node) stopHandlers() {
	n.handlers.core.Close()
	n.handlers.syncer.Close()
	n.handlers.flux.Close()
}

func (n *Node) Stop() {
	defer n.ctxCancel()
	n.logger.Info("Gracefully shutting down...!!!!")
	n.network.Stop()
	n.ixpool.Close()
	n.chain.Close()
	n.stopHandlers()
	n.stopTelemetry()
	n.db.Close()
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
		n.logger.Error("Prometheus server shutdown error", err)
	}
}

func (n *Node) startPrometheusServer(listenAddr *net.TCPAddr) *http.Server {
	srv := &http.Server{
		Addr: listenAddr.String(),
		Handler: promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{},
			),
		),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		n.logger.Info("Prometheus server started", "addr=", listenAddr.String())

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			n.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
		}
	}()

	return srv
}
