package main

import (
	"sort"
	"strings"
)

// opportunityScore ranks opportunities: Qloo affinity and popularity (when affinity
// ties on hackathon data), LLM compatibility, geography, and effort.
func opportunityScore(d AgentDraft, cand QlooCandidate, signal *QlooSignal) float64 {
	geo := 0.5
	if cand.City != "" && signal.City != "" && strings.EqualFold(strings.TrimSpace(cand.City), strings.TrimSpace(signal.City)) {
		geo = 1
	}
	var effort float64
	switch d.EstimatedEffort {
	case "low":
		effort = 1
	case "high":
		effort = 0.3
	default:
		effort = 0.6
	}
	return 0.25*cand.Affinity + 0.15*cand.Popularity + 0.25*d.BusinessCompatibility + 0.20*geo + 0.15*effort
}

// RankOpportunities ranks drafts deterministically, with no LLM call.
// Ties break on candidate name. At most maxOpportunities are returned;
// fewer are returned honestly when fewer valid drafts exist.
func RankOpportunities(drafts []AgentDraft, candidates []QlooCandidate, signal *QlooSignal) []AgentDraft {
	byID := map[string]QlooCandidate{}
	for _, c := range candidates {
		byID[c.ID] = c
	}
	sorted := append([]AgentDraft(nil), drafts...)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := opportunityScore(sorted[i], byID[sorted[i].CandidateID], signal), opportunityScore(sorted[j], byID[sorted[j].CandidateID], signal)
		if si != sj {
			return si > sj
		}
		ci, cj := byID[sorted[i].CandidateID], byID[sorted[j].CandidateID]
		if ci.Popularity != cj.Popularity {
			return ci.Popularity > cj.Popularity
		}
		return ci.Name < cj.Name
	})
	if len(sorted) > maxOpportunities {
		sorted = sorted[:maxOpportunities]
	}
	return sorted
}
