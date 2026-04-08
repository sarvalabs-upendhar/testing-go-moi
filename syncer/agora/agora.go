package agora

import (
	"context"

	"github.com/sarvalabs/go-moi/common"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/go-moi/network/p2p"
	"github.com/sarvalabs/go-moi/syncer"
	"github.com/sarvalabs/go-moi/syncer/agora/db"
	"github.com/sarvalabs/go-moi/syncer/agora/decision"
	"github.com/sarvalabs/go-moi/syncer/agora/network"
	"github.com/sarvalabs/go-moi/syncer/agora/notifications"
	"github.com/sarvalabs/go-moi/syncer/agora/session"
	"github.com/sarvalabs/go-moi/syncer/cid"
)

const (
	DefaultRequestWorkerCount  = 15
	DefaultResponseWorkerCount = 15
	DefaultLedgerWorkerCount   = 2
)

type Agora struct {
	engine   *decision.Engine
	ledger   *decision.Ledger
	db       *db.DataStore
	network  *network.AgoraNetwork
	im       *session.InterestManager
	sm       *session.Manager
	notifier notifications.PubSubNotifier
}

func NewAgora(
	logger hclog.Logger,
	store db.PersistenceManager,
	server *p2p.Server,
	metrics *Metrics,
	compressor common.Compressor,
) (*Agora, error) {
	interestManager := session.NewInterestManager()

	dataStore := db.NewDataStore(logger, store)

	ledger, err := decision.NewLedger(logger, DefaultLedgerWorkerCount, dataStore)
	if err != nil {
		return nil, err
	}

	notifier := notifications.NewNotifier()

	agoraNetwork := network.NewAgoraNetwork(logger, server, metrics.Network)

	engine, err := decision.NewEngine(
		logger.Named("Agora"),
		DefaultRequestWorkerCount,
		DefaultResponseWorkerCount,
		dataStore,
		ledger,
		agoraNetwork,
		metrics.Engine,
		decision.MaxQueueSize,
		compressor,
	)
	if err != nil {
		return nil, err
	}

	sessionManager := session.NewSessionManager(logger, interestManager, notifier, engine)

	ag := Agora{
		engine:   engine,
		ledger:   ledger,
		db:       dataStore,
		network:  agoraNetwork,
		im:       interestManager,
		sm:       sessionManager,
		notifier: notifier,
	}

	return &ag, nil
}

func (ag *Agora) NewSession(
	ctx context.Context,
	contextPeers []identifiers.KramaID,
	id identifiers.Identifier,
	stateHash cid.CID,
) (syncer.Session, error) {
	return ag.sm.NewSession(ctx, id, stateHash, ag.network, contextPeers)
}

func (ag *Agora) Start() {
	go ag.db.Start()
	go ag.engine.Start()
	go ag.ledger.Start()
	go ag.network.Start(ag.sm)
}

func (ag *Agora) Close() {
	ag.engine.Close()
	ag.ledger.Close()
	ag.db.Close()
	ag.network.Close()
}
