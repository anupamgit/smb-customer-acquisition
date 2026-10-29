package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// BuildOpportunities is the Evidence and Explanation Builder (Step 3):
// it assembles each ranked opportunity with its action, rationale,
// supporting evidence, and next action. Every evidence item is tagged
// with its origin, Qloo explainability is preserved unchanged in the
// Raw field, and agent wording is tagged as agent inference. Nothing
// here invents evidence: business evidence comes only from the stored
// cohort and signal, Qloo evidence only from Qloo-returned candidates.
func BuildOpportunities(ranked []AgentDraft, candidates []QlooCandidate, signal *QlooSignal, cohort []ScoredCustomer) []Opportunity {
	byID := map[string]QlooCandidate{}
	for _, c := range candidates {
		byID[c.ID] = c
	}
	// Cohort aggregate for the business-evidence line: counts and bought
	// names only, never per-customer data.
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
	businessDetail := fmt.Sprintf("%d valuable customers in %s", len(cohort), signal.LocalityQuery)
	if len(boughtNames) > 0 {
		businessDetail += "; they bought: " + strings.Join(boughtNames, ", ")
	}
	if signal.GeographyOnly {
		businessDetail += ". Qloo signals were geography-only: no bought name resolved to a Qloo tag."
	}

	out := make([]Opportunity, 0, len(ranked))
	for i, d := range ranked {
		cand := byID[d.CandidateID]
		evidence := []EvidenceItem{
			{
				Origin: OriginBusiness,
				Label:  "Your business data",
				Detail: businessDetail,
			},
			{
				Origin: OriginQloo,
				Label:  "Qloo evidence",
				Detail: fmt.Sprintf("%s (affinity %.2f, popularity %.2f)", cand.Name, cand.Affinity, cand.Popularity),
				Raw:    cand.Explainability,
			},
			{
				Origin: OriginAgent,
				Label:  "Agent reasoning",
				Detail: d.WhyFits,
			},
		}
		out = append(out, Opportunity{
			Rank:        i + 1,
			CandidateID: d.CandidateID,
			PlaceName:   cand.Name,
			PlaceType:   opportunityPlaceType(d, cand),
			Contact:     placeContactFromCandidate(cand),
			Action:      d.Action,
			WhyFits:     d.WhyFits,
			Evidence:    evidence,
			NextAction:  d.NextAction,
			Score:       math.Round(opportunityScore(d, cand, signal)*10000) / 10000,
		})
	}
	return out
}
