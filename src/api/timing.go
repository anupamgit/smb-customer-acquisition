package main

import (
	"log"
	"time"
)

// Set ADVISOR_STAGE_TIMING=1 (or true) in .env to log per-stage durations to stderr.
func analysisStageTimingEnabled() bool {
	return envBool("ADVISOR_STAGE_TIMING")
}

type stageTimer struct {
	runID    string
	runStart time.Time
	start    time.Time
}

func newStageTimer(runID string) stageTimer {
	now := time.Now()
	return stageTimer{runID: runID, runStart: now, start: now}
}

func (t *stageTimer) logStage(stage string) {
	if !analysisStageTimingEnabled() {
		t.start = time.Now()
		return
	}
	log.Printf("[analysis timing] run=%s stage=%s duration_ms=%d", t.runID, stage, time.Since(t.start).Milliseconds())
	t.start = time.Now()
}

func (t *stageTimer) logTotal() {
	if !analysisStageTimingEnabled() {
		return
	}
	log.Printf("[analysis timing] run=%s stage=total duration_ms=%d", t.runID, time.Since(t.runStart).Milliseconds())
}
