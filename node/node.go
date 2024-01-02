package node

import (
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sarvalabs/go-legacy-kramaid"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/compute"
	"github.com/sarvalabs/go-moi/consensus"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/ixpool"
	"github.com/sarvalabs/go-moi/jsonrpc"
	"github.com/sarvalabs/go-moi/lattice"
	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/senatus"
	"github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-moi/storage"
	"github.com/sarvalabs/go-moi/syncer/forage"
)

const (
	lruSize           = 2000
	validatorType     = 1
	kramaIDVersion    = 1
	syncMode          = "full"
	readHeaderTimeout = 5 * time.Second
)

type Node struct {
	logger              hclog.Logger
	cfg                 *config.Config
	eventMux            *utils.TypeMux
	network             *p2p.Server
	state               *state.StateManager
	chain               *lattice.ChainManager
	senatus             *senatus.ReputationEngine
	exec                *compute.Manager
	kramaEngine         *consensus.Engine
	db                  *storage.PersistenceManager
	ixpool              *ixpool.IxPool
	syncer              *forage.Syncer
	handlers            *SubHandlers
	cache               *lru.Cache
	rpc                 *jsonrpc.Server
	nodeMetrics         *nodeMetrics
	prometheusServer    *http.Server
	vault               *crypto.KramaVault
	consensusSlots      *ktypes.Slots
	lastActiveTimestamp uint64
}

func NewNode(logLevel string, cfg *config.Config) (n *Node, err error) {
	n = &Node{
		cfg:      cfg,
		eventMux: new(utils.TypeMux),
		handlers: new(SubHandlers),
	}

	if err = n.setupCacheStore(); err != nil {
		return nil, err
	}

	if err = n.setupVault(); err != nil {
		return nil, errors.Wrap(common.ErrVaultInit, err.Error())
	}

	// We should set up logger only after setting up the vault, as we need kramaID
	if err = n.setLogger(logLevel); err != nil {
		return nil, err
	}

	n.setupTelemetry()

	if err = n.setupPersistenceManager(); err != nil {
		return nil, err
	}

	if err = n.setupNetwork(); err != nil {
		return nil, err
	}

	n.setupConsensusSlots()

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

	n.setupRandomizer()

	if err = n.setupChainManager(); err != nil {
		return nil, err
	}

	if err = n.setupKramaEngine(); err != nil {
		return nil, err
	}

	if err = n.setupSyncer(); err != nil {
		return nil, errors.New("unable to create and setup syncer")
	}

	// setup JSON-RPC
	if err = n.setupRPC(); err != nil {
		return nil, common.ErrRPCFailed
	}

	n.setupSubHandler()

	if err = n.setupGenesis(); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *Node) GetKramaID() kramaid.KramaID {
	return n.network.GetKramaID()
}

func (n *Node) loadLatestActiveTimeStamp() {
	n.lastActiveTimestamp = n.db.GetLastActiveTimeStamp()
}

// Start starts network Stream handler, network's Discovery routine, Handlers, IxPool
// Krama engine, State manager, JSON-RPC server Chain manager
// returns any error invoked
func (n *Node) Start() (err error) {
	if err = n.startHandlers(); err != nil {
		return errors.Wrap(err, "failed to start sub handlers")
	}

	if err = n.network.StartServer(); err != nil {
		return errors.Wrap(err, "failed to start the p2p server")
	}

	go n.syncer.Start(forage.DefaultMinConnectedPeers)

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
	n.logger.Error("JSON RPC server stopped", "err", err)
}

func (n *Node) Stop() {
	n.logger.Info("Gracefully shutting down...!!!!")
	n.network.Stop()
	n.ixpool.Close()
	n.syncer.Close()
	n.chain.Close()
	n.senatus.Close()
	n.stopHandlers()
	n.stopTelemetry()
	n.db.Close()
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
		n.logger.Info("Prometheus server started", "addr", listenAddr.String())

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			n.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
		}
	}()

	return srv
}
