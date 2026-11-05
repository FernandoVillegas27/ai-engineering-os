package main

import (
	"context"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/runcontrol"
)

type activeRunContext struct {
	ctx         context.Context
	lease       *runcontrol.LeaseSession
	stopWatcher func()
}

func startActiveRunContext(
	parent context.Context,
	store *runcontrol.Store,
	runID string,
) (*activeRunContext, error) {
	lease, err := runcontrol.StartLeaseSession(parent, store, runID, 2*time.Second)
	if err != nil {
		return nil, err
	}
	ctx, stopWatcher, err := runcontrol.WithCancellationWatcher(
		lease.Context(),
		store,
		runID,
		250*time.Millisecond,
	)
	if err != nil {
		_ = lease.Stop()
		return nil, err
	}
	return &activeRunContext{
		ctx:         ctx,
		lease:       lease,
		stopWatcher: stopWatcher,
	}, nil
}

func (a *activeRunContext) Context() context.Context {
	if a == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *activeRunContext) Stop() error {
	if a == nil {
		return nil
	}
	if a.stopWatcher != nil {
		a.stopWatcher()
	}
	if a.lease != nil {
		return a.lease.Stop()
	}
	return nil
}

func (a *activeRunContext) LeaseError() error {
	if a == nil || a.lease == nil {
		return nil
	}
	return a.lease.Err()
}
