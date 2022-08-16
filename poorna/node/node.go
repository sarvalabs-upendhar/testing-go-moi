package node

import (
	"context"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.com/sarvalabs/moichain/common"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/core/chain"
	"gitlab.com/sarvalabs/moichain/core/ixpool"
	"gitlab.com/sarvalabs/moichain/dhruva"
	"gitlab.com/sarvalabs/moichain/guna"
	"gitlab.com/sarvalabs/moichain/jug"
	"gitlab.com/sarvalabs/moichain/krama"
	kcrypto "gitlab.com/sarvalabs/moichain/mudra"
	"gitlab.com/sarvalabs/moichain/poorna"
	"gitlab.com/sarvalabs/moichain/poorna/api"
	"gitlab.com/sarvalabs/moichain/poorna/flux"
	krpc "gitlab.com/sarvalabs/moichain/poorna/rpc"
	"gitlab.com/sarvalabs/moichain/poorna/senatus"
	"gitlab.com/sarvalabs/moichain/poorna/syncer"
	"log"
	"net"
	"net/http"
	"os"
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
	eventMux         *kutils.TypeMux
	network          *poorna.Server
	state            *guna.StateManager
	chain            *chain.ChainManager
	exec             *jug.Exec
	kramaEngine      *krama.Engine
	db               *dhruva.PersistenceManager
	ixpool           *ixpool.IxPool
	handlers         *SubHandlers
	cache            *lru.Cache
	datastore        blockstore.Blockstore
	rpc              *krpc.Server
	nodeMetrics      *nodeMetrics
	prometheusServer *http.Server
	vault            *kcrypto.KramaVault
	senatus          *senatus.ReputationEngine
}

func NewNode(logLevel string, cfg *common.Config) (n *Node, err error) {
	n = new(Node)

	n.cfg = cfg
	n.eventMux = new(kutils.TypeMux)
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())
	n.handlers = new(SubHandlers)

	n.cache, err = lru.New(LRUSIZE)
	if err != nil {
		return nil, err
	}

	n.vault, err = kcrypto.NewVault(cfg.Vault, 1, 1)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrVaultInit, err.Error())
	}

	if cfg.LogFilePath != "" {
		logFileName := cfg.LogFilePath + string(n.vault.KramaID())

		f, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
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
	//n.validator, err = kbft.NewTestPrivateValidator(cfg.Consensus.DirectoryPath)
	//if err != nil {
	//	return nil, errors.New("unable to parse private key")
	//}
	// setup p2p network server
	n.network, err = poorna.NewServer(n.ctx, n.logger, n.vault.KramaID(), n.eventMux, cfg.Network, n.vault)
	if err != nil {
		return nil, err
	}

	// create block store for Bitswap
	// this will be replaced with a wrapper around dhruva
	n.datastore = blockstore.NewBlockstore(sync.MutexWrap(datastore.NewMapDatastore()))
	// setup persistence manager
	db, err := dhruva.NewPersistenceManager(n.ctx, n.logger, cfg.DB, n.network.BsNetwork, n.datastore)
	if err != nil {
		return nil, err
	}

	n.db = db

	if cfg.Chain.Genesis != "nil" {
		if err := n.db.Cleanup(); err != nil {
			return nil, err
		}
	}
	// setup metrics
	n.setupTelemetry()
	// setup state manager
	n.state = guna.NewStateManager(db, n.logger, n.cache, n.eventMux)
	// setup execution engine
	n.exec = jug.NewExec(n.state)
	// setup ixpool
	n.ixpool = ixpool.NewIxPool(n.ctx, n.logger, n.eventMux, n.state, cfg.IxPool)

	if n.senatus, err = senatus.NewReputationEngine(
		n.ctx,
		n.logger,
		n.network.GetKramaID(),
		5,
		n.state,
		n.db,
	); err != nil {
		return nil, err
	}

	n.network.Senatus = n.senatus
	// setup chain manager
	n.chain = chain.NewChainManager(db, n.state, n.logger, n.eventMux, n.ixpool, n.cache, n.exec, n.senatus)
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
		n.db,
		n.handlers.flux,
	); err != nil {
		return nil, err
	}

	// setup JSON-RPC
	if err = n.SetupRPC(); err != nil {
		return nil, ktypes.ErrRPCFailed
	}

	if err = n.InitSubHandlers(); err != nil {
		return nil, ktypes.ErrHandlersFailed
	}

	if err = n.chain.SetupGenesis(cfg.Chain.Genesis); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *Node) InitSubHandlers() (err error) {
	if n.handlers.syncer, err = syncer.NewSyncer(
		n.ctx,
		n.network,
		n.eventMux,
		n.datastore,
		n.db,
		"full",
		n.chain,
		n.logger,
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

	n.handlers.flux = flux.NewRandomizer(n.ctx, n.logger, n.network)

	return
}

func (n *Node) SetupRPC() error {
	n.rpc = krpc.NewRPCServer("/", n.logger, n.cfg.Network.JSONRPCAddr)

	rpcService := krpc.NewRPCService()
	backend := api.NewBackend(n.ixpool, n.chain, n.state)
	ixpoolAPI := api.NewPublicIXPoolAPI(backend)
	coreAPI := api.NewPublicCoreAPI(backend)

	if err := rpcService.RegisterAPI("core", coreAPI); err != nil {
		return err
	}

	if err := rpcService.RegisterAPI("ixpool", ixpoolAPI); err != nil {
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
	n.senatus.Start()

	go n.rpc.Start()
	go n.chain.Start()
	go n.db.Start()
}
func (n *Node) startSubHandlers() {
	n.logger.Info("Starting Sub-Handlers")

	go n.handlers.core.Start()
	//go n.handlers.consensusHandler.Start()
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
	//TODO: Slowloris attack fix
	go func() {
		n.logger.Info("Prometheus server started", "addr=", listenAddr.String())

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			n.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
		}
	}()

	return srv
}
