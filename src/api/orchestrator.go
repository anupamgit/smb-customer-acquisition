package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// Orchestrator is the Analysis Orchestrator (Step 3 ADR-002): it invokes
// the eight processing steps of spec section 6 in order, owns run state,
// applies the bounded retries (inside the clients) and the geography
// fallback, and makes failures visible instead of silent. Every stage
// reads its inputs from, and writes its outputs to, the Run Store.
//
// Step order is declared with Azure/go-workflow (see orchestrator_workflow.go).
type Orchestrator struct {
	store     Store
	connector Connector
	qloo      Qloo
	llm       LLM
	cfg       config
	llmCache  *llmTailCache
	now       func() time.Time
}

func newOrchestrator(store Store, connector Connector, qloo Qloo, llm LLM, cfg config) *Orchestrator {
	return &Orchestrator{
		store: store, connector: connector, qloo: qloo, llm: llm, cfg: cfg,
		llmCache: sharedLLMTailCache, now: time.Now,
	}
}

func newRunID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "run_" + hex.EncodeToString(b[:])
}

// runDeadline bounds one analysis. Live Qloo Insights plus LLM calls often
// exceed 90s when retries apply; 3 minutes keeps the UI from polling a
// run that the server already abandoned while status stayed "running".
const runDeadline = 3 * time.Minute

// Run executes one full analysis run. On any stage failure the run is
// stored as failed with the stage and reason, and the error is
// returned; no partial result is ever presented as complete.
//
// If runIDReady is non-nil, run.ID is sent on it immediately after the
// run is persisted so HTTP handlers can return the correct id before the
// workflow finishes (supports concurrent runs).
func (o *Orchestrator) Run(parentCtx context.Context, runIDReady chan<- string) (*Run, error) {
	ctx, cancel := context.WithTimeout(parentCtx, runDeadline)
	defer cancel()
	now := o.now().UTC()
	run := &Run{
		ID:        newRunID(),
		Status:    RunStatusRunning,
		Stage:     "starting",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := o.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	if runIDReady != nil {
		select {
		case runIDReady <- run.ID:
		default:
		}
	}
	if err := o.runWorkflow(ctx, run); err != nil {
		o.markRunFailedIfStillRunning(run, err)
		return run, err
	}
	return run, nil
}

func (o *Orchestrator) markRunFailedIfStillRunning(run *Run, err error) {
	if run == nil || run.Status != RunStatusRunning {
		return
	}
	run.Status = RunStatusFailed
	if errors.Is(err, context.DeadlineExceeded) {
		run.Error = "Analysis took too long and was stopped. Try again."
	} else if err != nil {
		run.Error = err.Error()
	} else {
		run.Error = "Analysis stopped before it could finish."
	}
	run.UpdatedAt = o.now().UTC()
	// Parent ctx may already be canceled after a deadline; persist with a fresh ctx.
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = o.store.UpdateRun(saveCtx, run)
}
