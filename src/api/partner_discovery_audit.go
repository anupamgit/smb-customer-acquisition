package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

const maxPartnersToLLM = 20

type partnerFilterDrop struct {
	Candidate QlooCandidate
	Reason    string
}

func filterCrossCategoryPartnersEx(cands []QlooCandidate, businessType string) (kept []QlooCandidate, dropped []partnerFilterDrop) {
	if businessType == "" {
		return cands, nil
	}
	sorted := append([]QlooCandidate(nil), cands...)
	sort.Slice(sorted, func(i, j int) bool {
		return compareQlooMatch(sorted[i], sorted[j])
	})
	seenBucket := map[string]bool{}
	for _, c := range sorted {
		if isSameCategoryAsOwner(c, businessType) {
			dropped = append(dropped, partnerFilterDrop{Candidate: c, Reason: "same category as your business (keyword / tag match)"})
			continue
		}
		bucket := partnerCategoryBucket(c)
		if bucket == "" {
			bucket = c.Name
		}
		if seenBucket[bucket] && len(kept) >= minPartnerCandidates {
			dropped = append(dropped, partnerFilterDrop{Candidate: c, Reason: "duplicate partner category (diversity cap)"})
			continue
		}
		seenBucket[bucket] = true
		kept = append(kept, c)
	}
	return kept, dropped
}

func filterCrossCategoryPartners(cands []QlooCandidate, businessType string) []QlooCandidate {
	kept, _ := filterCrossCategoryPartnersEx(cands, businessType)
	return kept
}

func qlooRankByID(cands []QlooCandidate) map[string]int {
	sorted := append([]QlooCandidate(nil), cands...)
	sortQlooCandidatesByMatch(sorted)
	rank := map[string]int{}
	for i, c := range sorted {
		if qlooScoresComplete(c) {
			rank[c.ID] = i + 1
		}
	}
	return rank
}

func formatQlooScores(c QlooCandidate) string {
	return fmt.Sprintf("affinity %.6f, popularity %.6f", c.Affinity, c.Popularity)
}

func insightsAuditSections(raw []QlooCandidate, businessType string, partners []QlooCandidate, signal *QlooSignal, partnerRadiusMeters int) (rawSec, dropSec, shortlistSec []string) {
	ranks := qlooRankByID(raw)
	sortedRaw := append([]QlooCandidate(nil), raw...)
	sortQlooCandidatesByMatch(sortedRaw)

	rawSec = append(rawSec, fmt.Sprintf("Count from Qloo: %d (sorted by affinity, then popularity)", len(raw)))
	for _, c := range sortedRaw {
		if !qlooScoresComplete(c) {
			rawSec = append(rawSec, fmt.Sprintf("%s — skipped for ranking: missing affinity or popularity", c.Name))
			continue
		}
		rawSec = append(rawSec, fmt.Sprintf("qlooRank #%d — %s — %s", ranks[c.ID], c.Name, formatQlooScores(c)))
	}

	afterCross, distDrops, crossDrops, widened := rankPartnersFromInsights(raw, signal, businessType, partnerRadiusMeters)
	if widened {
		dropSec = append(dropSec, fmt.Sprintf("Partner radius tightened first, then widened to full Qloo ranking because in-radius cross-category partners were fewer than %d", minPartnerCandidates))
	}

	for _, d := range distDrops {
		r := ranks[d.Candidate.ID]
		line := fmt.Sprintf("%s — not sent to LLM: %s", d.Candidate.Name, d.Reason)
		if r > 0 {
			line = fmt.Sprintf("qlooRank #%d — %s — %s — %s", r, d.Candidate.Name, formatQlooScores(d.Candidate), d.Reason)
		}
		dropSec = append(dropSec, line)
	}

	for _, c := range raw {
		if qlooScoresComplete(c) {
			continue
		}
		dropSec = append(dropSec, fmt.Sprintf("%s — not sent to LLM: missing affinity or popularity from Qloo", c.Name))
	}
	for _, d := range crossDrops {
		r := ranks[d.Candidate.ID]
		line := fmt.Sprintf("%s — not sent to LLM: %s", d.Candidate.Name, d.Reason)
		if r > 0 {
			line = fmt.Sprintf("qlooRank #%d — %s — %s — %s", r, d.Candidate.Name, formatQlooScores(d.Candidate), d.Reason)
		}
		dropSec = append(dropSec, line)
	}

	final := afterCross
	if len(final) > maxPartnersToLLM {
		for _, c := range final[maxPartnersToLLM:] {
			r := ranks[c.ID]
			dropSec = append(dropSec, fmt.Sprintf("qlooRank #%d — %s — not sent to LLM: beyond top %d shortlist cap", r, c.Name, maxPartnersToLLM))
		}
		final = final[:maxPartnersToLLM]
	}

	shortlistSec = append(shortlistSec, fmt.Sprintf("Sent to LLM: %d places", len(partners)))
	partnerRank := qlooRankByID(partners)
	for i, c := range partners {
		shortlistSec = append(shortlistSec, fmt.Sprintf("shortlist #%d, qlooRank #%d — %s — %s", i+1, partnerRank[c.ID], c.Name, formatQlooScores(c)))
	}
	return rawSec, dropSec, shortlistSec
}

func llmAuditLines(payload LLMPayload, drafts []AgentDraft) (inputSec, outputSec []string) {
	inputSec = append(inputSec, auditJSONLines(payload)...)
	out := map[string]any{"opportunities": drafts}
	outputSec = append(outputSec, auditJSONLines(out)...)
	return inputSec, outputSec
}

func finalRankAuditLines(ranked []AgentDraft, candidates []QlooCandidate, signal *QlooSignal) []string {
	byID := map[string]QlooCandidate{}
	for _, c := range candidates {
		byID[c.ID] = c
	}
	var lines []string
	lines = append(lines, "Combined score = 0.25×affinity + 0.15×popularity + 0.25×LLM compatibility + 0.20×geography + 0.15×effort (ties: popularity, then name)")
	for i, d := range ranked {
		c := byID[d.CandidateID]
		score := opportunityScore(d, c, signal)
		lines = append(lines, fmt.Sprintf("rank #%d score %.4f — %s — %s — LLM compat %.2f, effort %s",
			i+1, score, c.Name, formatQlooScores(c), d.BusinessCompatibility, d.EstimatedEffort))
	}
	return lines
}

func auditJSONLines(v any) []string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []string{fmt.Sprintf("(could not serialize: %v)", err)}
	}
	const maxLine = 2000
	if len(b) <= maxLine {
		return []string{"```json", string(b), "```"}
	}
	return []string{
		fmt.Sprintf("```json (%d bytes, truncated)", len(b)),
		string(b[:maxLine]) + "…",
		"```",
	}
}
