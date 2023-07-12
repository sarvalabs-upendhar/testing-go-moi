package agora

import (
	"context"

	id "github.com/sarvalabs/moichain/common/kramaid"
	"github.com/sarvalabs/moichain/syncer/agora/db"
	"github.com/sarvalabs/moichain/syncer/agora/decision"
	"github.com/sarvalabs/moichain/syncer/agora/network"
	"github.com/sarvalabs/moichain/syncer/agora/notifications"
	"github.com/sarvalabs/moichain/syncer/agora/session"
	"github.com/sarvalabs/moichain/syncer/cid"

	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/network/p2p"
)

const (
	DefaultRequestWorkerCount  = 15
	DefaultResponseWorkerCount = 15
	DefaultLedgerWorkerCount   = 2
)

type Agora struct {
	ctx      context.Context
	engine   *decision.Engine
	ledger   *decision.Ledger
	db       *db.DataStore
	network  *network.AgoraNetwork
	im       *session.InterestManager
	sm       *session.Manager
	notifier notifications.PubSubNotifier
}

func NewAgora(
	ctx context.Context,
	logger hclog.Logger,
	store db.PersistenceManager,
	server *p2p.Server,
	metrics *Metrics,
) (*Agora, error) {
	interestManager := session.NewInterestManager()

	dataStore := db.NewDataStore(ctx, logger, store)

	ledger, err := decision.NewLedger(ctx, logger, DefaultLedgerWorkerCount, dataStore)
	if err != nil {
		return nil, err
	}

	notifier := notifications.NewNotifier()

	agoraNetwork := network.NewAgoraNetwork(ctx, logger, server, metrics.Network)

	engine := decision.NewEngine(
		ctx,
		logger.Named("Agora"),
		DefaultRequestWorkerCount,
		DefaultResponseWorkerCount,
		dataStore,
		ledger,
		agoraNetwork,
		metrics.Engine,
		decision.MaxQueueSize,
	)

	sessionManager := session.NewSessionManager(logger, interestManager, notifier, engine)

	ag := &Agora{
		ctx:      ctx,
		engine:   engine,
		ledger:   ledger,
		db:       dataStore,
		network:  agoraNetwork,
		im:       interestManager,
		sm:       sessionManager,
		notifier: notifier,
	}

	return ag, nil
}

func (ag *Agora) NewSession(
	ctx context.Context,
	contextPeers []id.KramaID,
	address common.Address,
	stateHash cid.CID,
) (*session.Session, error) {
	return ag.sm.NewSession(ctx, address, stateHash, ag.network, contextPeers)
}

func (ag *Agora) Start() {
	go ag.db.Start()
	go ag.engine.Start()
	go ag.ledger.Start()
	go ag.network.Start(ag.sm)
}
