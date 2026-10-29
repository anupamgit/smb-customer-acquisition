package main

import (
	"math"
	"sort"
	"time"
)

// ScoreCohort scores canonical records deterministically and returns the
// valuable-customer cohort: the top 20% (at least 1, at most 20) by
// score. No agent reasoning and no LLM call are used at this stage.
//
// Score = 0.5 * revenue + 0.3 * frequency + 0.2 * recency, where each
// component is min-max normalised across the records supplied (0 when
// all values are equal). Recency uses days since the last order, capped
// at 365 days; a customer with no dated order scores 0 on recency.
// Ties break on CustomerID so the result is stable across runs.
func ScoreCohort(records []CanonicalRecord, now time.Time) []ScoredCustomer {
	if len(records) == 0 {
		return nil
	}
	var maxRevenue int64
	var maxCount int
	for _, r := range records {
		if r.TotalMinor > maxRevenue {
			maxRevenue = r.TotalMinor
		}
		if r.OrderCount > maxCount {
			maxCount = r.OrderCount
		}
	}
	scored := make([]ScoredCustomer, 0, len(records))
	for _, r := range records {
		var revenue, freq, recency float64
		if maxRevenue > 0 {
			revenue = float64(r.TotalMinor) / float64(maxRevenue)
		}
		if maxCount > 0 {
			freq = float64(r.OrderCount) / float64(maxCount)
		}
		if r.LastOrderAt != nil {
			days := now.Sub(*r.LastOrderAt).Hours() / 24
			if days < 0 {
				days = 0
			}
			if days > 365 {
				days = 365
			}
			recency = 1 - days/365
		}
		score := 0.5*revenue + 0.3*freq + 0.2*recency
		scored = append(scored, ScoredCustomer{CanonicalRecord: r, Score: math.Round(score*10000) / 10000})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].CustomerID < scored[j].CustomerID
	})
	n := len(scored) / 5
	if n < 1 {
		n = 1
	}
	if n > 20 {
		n = 20
	}
	// Drop zero-score tail entries beyond the first: a cohort member with
	// no revenue, no orders, and no recency carries no signal.
	cohort := scored[:n]
	for len(cohort) > 1 && cohort[len(cohort)-1].Score == 0 {
		cohort = cohort[:len(cohort)-1]
	}
	return cohort
}
