package main

import (
	"sort"
	"strings"
)

// BuildSignals builds the derived Qloo signals from the scored cohort
// (spec section 6, step 3; Step 3 ADR-004). Geography is the dependable
// path: the locality most cohort customers share. Bought names are the
// deduplicated item names of cohort customers, capped at 20 for a
// bounded resolution step. No customer identifier or PII is included.
func BuildSignals(cohort []ScoredCustomer, businessLat, businessLon *float64) *QlooSignal {
	counts := map[string]int{}
	byKey := map[string]*QlooSignal{}
	for _, c := range cohort {
		if !c.HasLocation {
			continue
		}
		key := strings.Join([]string{c.Locality, c.AdminRegion, c.PostalCode}, "|")
		counts[key]++
		if byKey[key] == nil {
			byKey[key] = &QlooSignal{City: c.Locality, AdminRegion: c.AdminRegion, PostalCode: c.PostalCode}
		}
	}
	var best *QlooSignal
	bestCount := 0
	// Deterministic choice on ties: iterate keys in sorted order.
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if counts[k] > bestCount {
			bestCount = counts[k]
			best = byKey[k]
		}
	}
	signal := &QlooSignal{GeographyOnly: true}
	if best != nil {
		signal.City = best.City
		signal.AdminRegion = best.AdminRegion
		signal.PostalCode = best.PostalCode
	}
	parts := []string{}
	if signal.City != "" {
		parts = append(parts, signal.City)
	}
	if signal.AdminRegion != "" {
		parts = append(parts, signal.AdminRegion)
	}
	if signal.PostalCode != "" && signal.City == "" {
		parts = append(parts, signal.PostalCode)
	}
	signal.LocalityQuery = strings.Join(parts, ", ")
	if businessLat != nil && businessLon != nil {
		lat, lon := *businessLat, *businessLon
		signal.Latitude, signal.Longitude = &lat, &lon
	}

	names := map[string]bool{}
	for _, c := range cohort {
		for _, name := range c.ItemNames {
			names[name] = true
		}
	}
	for name := range names {
		signal.BoughtNames = append(signal.BoughtNames, name)
	}
	sort.Strings(signal.BoughtNames)
	if len(signal.BoughtNames) > 20 {
		signal.BoughtNames = signal.BoughtNames[:20]
	}
	return signal
}

// applyShopLocationFallback sets filter.location.query from the shop address
// when the cohort did not yield a locality string (coordinates may still apply).
func applyShopLocationFallback(signal *QlooSignal, loc *SquareLocation) {
	if signal == nil || strings.TrimSpace(signal.LocalityQuery) != "" || loc == nil || loc.Address == nil {
		return
	}
	a := loc.Address
	city := strings.TrimSpace(a.Locality)
	region := strings.TrimSpace(a.AdminRegion)
	zip := strings.TrimSpace(a.PostalCode)
	if city == "" && zip == "" {
		return
	}
	signal.City = city
	signal.AdminRegion = region
	signal.PostalCode = zip
	var parts []string
	if city != "" {
		parts = append(parts, city)
	}
	if region != "" {
		parts = append(parts, region)
	}
	if zip != "" && city == "" {
		parts = append(parts, zip)
	}
	signal.LocalityQuery = strings.Join(parts, ", ")
}
