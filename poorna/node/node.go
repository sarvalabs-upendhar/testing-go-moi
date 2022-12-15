package node

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/sarvalabs/moichain/ixpool"
	"github.com/sarvalabs/moichain/lattice"
	"github.com/sarvalabs/moichain/utils"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva"
	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/jug"
	"github.com/sarvalabs/moichain/krama"
	kcrypto "github.com/sarvalabs/moichain/mudra"
	"github.com/sarvalabs/moichain/poorna"
	"github.com/sarvalabs/moichain/poorna/api"
	"github.com/sarvalabs/moichain/poorna/flux"
	krpc "github.com/sarvalabs/moichain/poorna/rpc"
	"github.com/sarvalabs/moichain/poorna/syncer"
	"github.com/sarvalabs/moichain/types"
)

const (
	LRUSIZE = 2000
)

type SubHandlers struct {
	syncer *syncer.Syncer
	core   *poorna.SubHandler
	flux   *flux.Randomizer
}

type Node struct {
	ctx              context.Context
	ctxCancel        context.CancelFunc
	logger           hclog.Logger
	cfg              *common.Config
	eventMux         *utils.TypeMux
	network          *poorna.Server
	state            *guna.StateManager
	chain            *lattice.ChainManager
	exec             *jug.ExecutionManager
	kramaEngine      *krama.Engine
	db               *dhruva.PersistenceManager
	ixpool           *ixpool.IxPool
	handlers         *SubHandlers
	cache            *lru.Cache
	rpc              *krpc.Server
	nodeMetrics      *nodeMetrics
	prometheusServer *http.Server
	vault            *kcrypto.KramaVault
}

func NewNode(logLevel string, cfg *common.Config) (n *Node, err error) {
	n = new(Node)

	n.cfg = cfg
	n.eventMux = new(utils.TypeMux)
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())
	n.handlers = new(SubHandlers)

	n.cache, err = lru.New(LRUSIZE)
	if err != nil {
		return nil, err
	}

	n.vault, err = kcrypto.NewVault(cfg.Vault, 1, 1)
	if err != nil {
		return nil, errors.Wrap(types.ErrVaultInit, err.Error())
	}

	if cfg.LogFilePath != "" {
		logFileName := cfg.LogFilePath + string(n.vault.KramaID())

		f, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return nil, err
		}

		n.logger = hclog.New(&hclog.LoggerOptions{
			Name:   "MOI",
			Output: f,
			Level:  hclog.LevelFromString(logLevel),
		})
	} else {
		n.logger = hclog.New(&hclog.LoggerOptions{
			Name:  "MOI",
			Level: hclog.LevelFromString(logLevel),
		})
	}

	// setup p2p network server
	n.network, err = poorna.NewServer(n.ctx, n.logger, n.vault.KramaID(), n.eventMux, cfg.Network, n.vault)
	if err != nil {
		return nil, err
	}

	// setup persistence manager
	db, err := dhruva.NewPersistenceManager(n.ctx, n.logger, cfg.DB)
	if err != nil {
		return nil, err
	}

	n.db = db

	if !cfg.Chain.SkipGenesis {
		if err := n.db.Cleanup(); err != nil {
			return nil, err
		}
	}

	// setup metrics
	n.setupTelemetry()
	// setup state manager
	n.state, err = guna.NewStateManager(n.ctx, db, n.logger, n.cache, n.network, n.nodeMetrics.guna)
	if err != nil {
		return nil, err
	}
	// setup execution engine
	n.exec = jug.NewExecutionManager(n.state, n.logger, cfg.Execution)
	// setup ixpool
	n.ixpool = ixpool.NewIxPool(n.ctx, n.logger, n.eventMux, n.state, cfg.IxPool, n.nodeMetrics.ixpool)

	n.network.Senatus = n.state.SenatusInstance()

	n.handlers.flux = flux.NewRandomizer(n.ctx, n.logger, n.network, n.nodeMetrics.flux)
	// setup lattice manager
	if n.chain, err = lattice.NewChainManager(
		n.ctx,
		n.cfg.Chain,
		db,
		n.state,
		n.logger,
		n.eventMux,
		n.network,
		n.ixpool,
		n.cache,
		n.exec,
		n.state.SenatusInstance(),
		n.nodeMetrics.chain,
	); err != nil {
		return nil, err
	}
	// setup krama engine
	if n.kramaEngine, err = krama.NewKramaEngine(
		n.ctx,
		cfg.Consensus,
		n.logger,
		n.state,
		n.network,
		n.exec,
		n.ixpool,
		n.vault,
		n.chain,
		n.handlers.flux,
		n.nodeMetrics.krama,
	); err != nil {
		return nil, err
	}

	// setup JSON-RPC
	if err = n.SetupRPC(); err != nil {
		return nil, types.ErrRPCFailed
	}

	if err = n.InitSubHandlers(); err != nil {
		return nil, types.ErrHandlersFailed
	}

	if !cfg.Chain.SkipGenesis {
		if err = n.chain.SetupGenesis(cfg.Chain.Genesis); err != nil {
			return nil, err
		}
	}

	return n, nil
}

func (n *Node) InitSubHandlers() (err error) {
	if n.handlers.syncer, err = syncer.NewSyncer(
		n.ctx,
		n.network,
		n.eventMux,
		n.db,
		"full",
		n.chain,
		n.logger,
		n.nodeMetrics.agora,
	); err != nil {
		return err
	}

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

	return
}

func (n *Node) SetupRPC() error {
	n.rpc = krpc.NewRPCServer("/", n.logger, n.cfg.Network.JSONRPCAddr, n.eventMux)

	rpcService := krpc.NewRPCService()
	backend := api.NewBackend(n.ixpool, n.chain, n.state, n.cfg.IxPool)
	publicApis := api.GetPublicAPIs(backend)

	if err := rpcService.RegisterAPIs(publicApis); err != nil {
		return err
	}

	if err := n.rpc.RegisterService("moi", rpcService); err != nil {
		return err
	}

	return nil
}

func (n *Node) Start() {
	if err := n.network.Start(); err != nil {
		log.Panic(err)
	}

	n.startSubHandlers()
	n.ixpool.Start()
	n.kramaEngine.Start()

	if err := n.state.Start(
		n.network.GetKramaID(),
		1,
		n.vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
		n.network.GetAddrs()); err != nil {
		log.Panic(err)
	}

	go n.rpc.Start()

	if err := n.chain.Start(); err != nil {
		log.Panic(err)
	}
}

func (n *Node) startSubHandlers() {
	n.logger.Info("Starting Sub-Handlers")

	go n.handlers.core.Start()

	go n.handlers.syncer.Start()

	go n.handlers.flux.Start()
}

func (n *Node) stopSubHandlers() {
	n.handlers.core.Close()
	n.handlers.syncer.Close()
	n.handlers.flux.Close()
}

func (n *Node) Stop() {
	n.logger.Info("Gracefully shutting down...!!!!")
	n.network.Stop()
	n.ixpool.Close()
	n.chain.Close()
	n.db.Close()
	n.stopSubHandlers()
	n.stopTelemetry()
}

func (n *Node) setupTelemetry() {
	if n.cfg.Metrics.PrometheusAddr != nil {
		n.nodeMetrics = metricProvider("MOI", "test-net", true)

		n.startPrometheusServer(n.cfg.Metrics.PrometheusAddr)
	} else {
		n.nodeMetrics = metricProvider("MOI", "test-net", false)
	}
}

func (n *Node) stopTelemetry() {
	if n.prometheusServer != nil {
		if err := n.prometheusServer.Shutdown(context.Background()); err != nil {
			n.logger.Error("Prometheus server shutdown error", err)
		}
	}
}

func (n *Node) startPrometheusServer(listenAddr *net.TCPAddr) *http.Server {
	srv := &http.Server{ //nolint
		Addr: listenAddr.String(),
		Handler: promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{},
			),
		),
	}
	// TODO: Slowloris attack fix
	go func() {
		n.logger.Info("Prometheus server started", "addr=", listenAddr.String())

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			n.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
		}
	}()

	return srv
}
