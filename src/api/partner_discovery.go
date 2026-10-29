package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"advisor/scenario"
)

const minPartnerCandidates = 3

// partnerInsightsLocationRadiusMeters is the disk sent to Qloo Insights. The partner
// shortlist may apply a tighter post-filter; fetching too narrow (e.g. 1.6 km in LA)
// returns only same-category neighbors and discovery fails even after widen logic.
func partnerInsightsLocationRadiusMeters(partnerRadiusMeters int) int {
	if partnerRadiusMeters <= 0 {
		return defaultQlooLocationRadiusMeters
	}
	if partnerRadiusMeters < defaultQlooLocationRadiusMeters {
		return defaultQlooLocationRadiusMeters
	}
	return partnerRadiusMeters
}

type partnerDiscoveryRecord struct {
	ExcludeTags []string `json:"excludeTags"`
}

type partnerDiscoveryFile struct {
	ByBusiness map[string]partnerDiscoveryRecord `json:"byBusinessType"`
}

var (
	partnerDiscoveryOnce sync.Once
	partnerDiscoveryLoad partnerDiscoveryCache
)

type partnerDiscoveryCache struct {
	file *partnerDiscoveryFile
	err  error
}

func partnerDiscoveryJSONPath() string {
	if dir, ok := scenario.FindDataDir(""); ok {
		return filepath.Join(filepath.Dir(dir), "qloo", "partner_discovery.json")
	}
	return filepath.Join("data", "qloo", "partner_discovery.json")
}

func loadPartnerDiscoveryFile() (*partnerDiscoveryFile, error) {
	partnerDiscoveryOnce.Do(func() {
		path := partnerDiscoveryJSONPath()
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				partnerDiscoveryLoad = partnerDiscoveryCache{file: nil, err: nil}
				return
			}
			partnerDiscoveryLoad = partnerDiscoveryCache{err: fmt.Errorf("read partner discovery config %s: %w", path, err)}
			return
		}
		var doc partnerDiscoveryFile
		if err := json.Unmarshal(b, &doc); err != nil {
			partnerDiscoveryLoad = partnerDiscoveryCache{err: fmt.Errorf("parse partner discovery config: %w", err)}
			return
		}
		if doc.ByBusiness == nil {
			doc.ByBusiness = map[string]partnerDiscoveryRecord{}
		}
		partnerDiscoveryLoad = partnerDiscoveryCache{file: &doc}
	})
	if partnerDiscoveryLoad.err != nil {
		return nil, partnerDiscoveryLoad.err
	}
	return partnerDiscoveryLoad.file, nil
}

func excludeTagsForBusinessType(businessType string) []string {
	if businessType == "" {
		return nil
	}
	doc, err := loadPartnerDiscoveryFile()
	if err != nil || doc == nil {
		return nil
	}
	rec, ok := doc.ByBusiness[businessType]
	if !ok {
		return nil
	}
	return append([]string(nil), rec.ExcludeTags...)
}

func businessTypeForRun(c Connector, cfg config) string {
	if sn, ok := c.(interface{ ScenarioName() string }); ok {
		if spec, ok := scenario.ProbeSpecByName(sn.ScenarioName()); ok {
			return spec.BusinessType
		}
	}
	bt := strings.TrimSpace(cfg.businessType)
	if knownBusinessTypes[bt] {
		return bt
	}
	return ""
}

// discoverCandidates ranks nearby places whose tastes align with the cohort signal,
// excluding the owner's business category. Partner categories come from Qloo results,
// not from a fixed gym/restaurant filter list.
// rankPartnersFromInsights ranks Qloo places for partner discovery: complete scores,
// optional radius tighten, then cross-category filtering. When in-radius cross-category
// partners are fewer than minPartnerCandidates (including the "one nearby place" cliff),
// widens to Qloo's full ranking on the Insights fetch disk.
func rankPartnersFromInsights(raw []QlooCandidate, signal *QlooSignal, businessType string, partnerRadiusMeters int) (partners []QlooCandidate, distDrops, crossDrops []partnerFilterDrop, widenedPastRadius bool) {
	scoredAll := filterQlooWithCompleteScores(raw)
	scored := scoredAll
	tightened := false
	if partnerRadiusMeters > 0 && signal.Latitude != nil && signal.Longitude != nil {
		within, drops := filterCandidatesWithinRadiusMeters(signal, scoredAll, partnerRadiusMeters)
		distDrops = drops
		if len(within) > 0 {
			scored = within
			tightened = len(within) < len(scoredAll)
		}
	}
	sortQlooCandidatesByMatch(scored)
	partners, crossDrops = filterCrossCategoryPartnersEx(scored, businessType)
	if tightened && len(partners) < minPartnerCandidates {
		widenedPastRadius = true
		sortQlooCandidatesByMatch(scoredAll)
		partners, crossDrops = filterCrossCategoryPartnersEx(scoredAll, businessType)
	}
	return partners, distDrops, crossDrops, widenedPastRadius
}

// discoverCandidates calls Qloo Insights, then cross-category filters for the pipeline.
// raw is every place entity Qloo returned (up to take); partners is the filtered set capped for downstream steps.
func discoverCandidates(ctx context.Context, q Qloo, signal *QlooSignal, tagURNs []string, businessType string, take, partnerRadiusMeters int) (partners, raw []QlooCandidate, err error) {
	exclude := excludeTagsForBusinessType(businessType)
	tagURNs = normalizeInterestTagList(tagURNs)

	filters := InsightsFilters{
		InterestTags:         tagURNs,
		ExcludeTags:          exclude,
		Take:                 take,
		LocationRadiusMeters: partnerInsightsLocationRadiusMeters(partnerRadiusMeters),
	}
	raw, err = q.InsightsWithFilters(ctx, signal, filters)
	if err != nil {
		return nil, nil, err
	}
	candidates, _, _, _ := rankPartnersFromInsights(raw, signal, businessType, partnerRadiusMeters)

	if len(candidates) == 0 {
		if signal.GeographyOnly {
			return nil, raw, errors.New("no cross-category partner places found from geography alone; product interest tags could not be resolved")
		}
		return nil, raw, errors.New("qloo returned no usable cross-category candidate places for the derived signals")
	}
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}
	return candidates, raw, nil
}
