package jug

import (
	"log"
	"sync"

	"github.com/sarvalabs/moichain/guna"
	"github.com/sarvalabs/moichain/types"
)

type Exec struct {
	executorInstances sync.Map
	state             *guna.StateManager
}

func NewExec(state *guna.StateManager) *Exec {
	e := &Exec{
		state: state,
	}

	return e
}

func (e *Exec) ExecuteInteractions(
	clusterID types.ClusterID,
	ixs []*types.Interaction,
	contextDelta types.ContextDelta,
) (types.Receipts, error) {
	executor := NewExecutor(ixs, 1000, contextDelta, e.state)
	if err := executor.Execute(); err != nil {
		if err := executor.Revert(); err != nil {
			log.Fatal(err) // This should not happen
		}

		return nil, err
	}

	e.executorInstances.Store(clusterID, executor)

	return executor.Receipts(), nil
}

func (e *Exec) Revert(clusterID types.ClusterID) error {
	rawExecutor, ok := e.executorInstances.Load(clusterID)
	if !ok {
		return nil
	}

	executor, ok := rawExecutor.(*Executor)
	if !ok {
		return types.ErrInterfaceConversion
	}

	return executor.Revert()
}

func (e *Exec) CleanupExecutorInstances(id types.ClusterID) {
	e.executorInstances.Delete(id)
}
