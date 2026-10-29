package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RecordFeedback is the Feedback Recorder (Step 3, optional; spec
// section 9). It records whether an attempted opportunity worked and,
// if known, its business outcome, plus an optional later reminder. It
// validates the opportunity index against the stored result and never
// changes a result already shown. No attribution or lead management.
func RecordFeedback(ctx context.Context, store Store, runID string, fb Feedback, now time.Time) error {
	run, err := store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusSucceeded {
		return errors.New("feedback can only be recorded for a completed run")
	}
	if fb.OpportunityIndex < 0 || fb.OpportunityIndex >= len(run.Opportunities) {
		return fmt.Errorf("opportunity index %d is out of range for this run", fb.OpportunityIndex)
	}
	fb.CreatedAt = now.UTC()
	return store.SaveFeedback(ctx, runID, fb)
}
