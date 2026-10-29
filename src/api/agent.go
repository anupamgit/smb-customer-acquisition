package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ReasonOverEvidence is Agent Reasoning (Step 3): it sends stored
// evidence to the LLM service and validates the drafts that come back.
// A draft is kept only when it names a Qloo-returned candidate, has a
// non-empty action, and has a compatibility score in range. Drafts that
// fail validation are dropped, never repaired with invented content.
// If no draft survives, the run fails visibly: agent-only suggestions
// with no Qloo evidence behind them are never shown (Step 3 section 8.2).
// BuildLLMPayload aggregates non-PII evidence for the agent step (also used in pipeline audit).
func BuildLLMPayload(signal *QlooSignal, cohort []ScoredCustomer, tagURNs []string, candidates []QlooCandidate) LLMPayload {
	bought := map[string]bool{}
	for _, c := range cohort {
		for _, name := range c.ItemNames {
			bought[name] = true
		}
	}
	var boughtNames []string
	for name := range bought {
		boughtNames = append(boughtNames, name)
	}
	sort.Strings(boughtNames)
	if len(boughtNames) > 20 {
		boughtNames = boughtNames[:20]
	}
	return LLMPayload{
		Locality:      signal.LocalityQuery,
		CohortSize:    len(cohort),
		CohortBought:  boughtNames,
		GeographyOnly: signal.GeographyOnly,
		ResolvedTags:  tagURNs,
		Candidates:    candidates,
	}
}

func ReasonOverEvidence(ctx context.Context, llm LLM, signal *QlooSignal, cohort []ScoredCustomer, tagURNs []string, candidates []QlooCandidate) ([]AgentDraft, error) {
	payload := BuildLLMPayload(signal, cohort, tagURNs, candidates)
	drafts, err := llm.Complete(ctx, payload)
	if err != nil {
		return nil, err
	}
	return validateAgentDrafts(drafts, candidates)
}

func validateAgentDrafts(drafts []AgentDraft, candidates []QlooCandidate) ([]AgentDraft, error) {
	known := map[string]bool{}
	for _, cand := range candidates {
		known[cand.ID] = true
	}
	var kept []AgentDraft
	for _, d := range drafts {
		if !known[d.CandidateID] {
			continue
		}
		if strings.TrimSpace(d.Action) == "" {
			continue
		}
		if d.BusinessCompatibility < 0 || d.BusinessCompatibility > 1 {
			continue
		}
		switch d.EstimatedEffort {
		case "low", "medium", "high":
		default:
			d.EstimatedEffort = "medium"
		}
		kept = append(kept, d)
		if len(kept) >= maxOpportunities {
			break
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("agent reasoning produced no usable opportunity tied to Qloo evidence")
	}
	return kept, nil
}
