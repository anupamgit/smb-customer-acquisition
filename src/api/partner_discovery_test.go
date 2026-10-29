package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"advisor/scenario"
)

func TestLoadPartnerDiscoveryJSON(t *testing.T) {
	path := partnerDiscoveryJSONPath()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected committed partner discovery at %s: %v", path, err)
	}
	doc, err := loadPartnerDiscoveryFile()
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil || len(doc.ByBusiness) == 0 {
		t.Fatal("partner discovery file empty")
	}
	ex := excludeTagsForBusinessType("coffee_shop")
	if len(ex) == 0 {
		t.Fatalf("coffee_shop exclude tags missing: %+v", ex)
	}
}

func TestPartnerInsightsLocationRadiusWidensNarrowEnv(t *testing.T) {
	if got := partnerInsightsLocationRadiusMeters(1600); got != defaultQlooLocationRadiusMeters {
		t.Fatalf("got %d want %d", got, defaultQlooLocationRadiusMeters)
	}
	if got := partnerInsightsLocationRadiusMeters(25000); got != 25000 {
		t.Fatalf("got %d", got)
	}
}

func TestDiscoverCandidatesUsesPartnerFiltersForScenario(t *testing.T) {
	spec, ok := scenario.ProbeSpecByName("coffee-nyc")
	if !ok {
		t.Fatal("probe spec")
	}
	q := &fakeQloo{cands: []QlooCandidate{
		{ID: "g1", Name: "Planet Fitness"},
		{ID: "g2", Name: "Equinox"},
		{ID: "g3", Name: "EVEN Hotel"},
	}}
	lat, lon := spec.Latitude, spec.Longitude
	signal := &QlooSignal{LocalityQuery: spec.LocalityQuery, Latitude: &lat, Longitude: &lon}
	tagURNs := []string{"urn:tag:coffee"}
	cands, _, err := discoverCandidates(context.Background(), q, signal, tagURNs, spec.BusinessType, 50, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("cands = %d", len(cands))
	}
	if len(q.seenFilters) == 0 {
		t.Fatal("expected InsightsWithFilters for coffee_shop")
	}
	f := q.seenFilters[0]
	if len(f.FilterTags) != 0 {
		t.Fatalf("expected no hard-coded partner filter tags, got %+v", f.FilterTags)
	}
	if len(f.ExcludeTags) == 0 {
		t.Fatalf("exclude tags = %+v", f.ExcludeTags)
	}
	if f.Take != 50 {
		t.Fatalf("take = %d want %d", f.Take, 50)
	}
	if f.LocationRadiusMeters != defaultQlooLocationRadiusMeters {
		t.Fatalf("insights location radius = %d want %d", f.LocationRadiusMeters, defaultQlooLocationRadiusMeters)
	}
	if len(f.InterestTags) != 1 || f.InterestTags[0] != "urn:tag:coffee" {
		t.Fatalf("interest tags = %v", f.InterestTags)
	}
}

func TestDiscoverCandidatesWidensRadiusWhenInRadiusAllSameCategory(t *testing.T) {
	lat, lon := 34.1016, -118.3267
	// Near shop: hair-salon names (same category). Farther: cross-category gym.
	nearLat, nearLon := lat+0.001, lon+0.001
	farLat, farLon := lat+0.08, lon+0.08
	q := &fakeQloo{cands: []QlooCandidate{
		{ID: "s1", Name: "Sunset Hair Studio", Affinity: 0.99, Popularity: 0.9, scoreComplete: true, PlaceLatitude: &nearLat, PlaceLongitude: &nearLon},
		{ID: "s2", Name: "Beauty Bar LA", Affinity: 0.98, Popularity: 0.88, scoreComplete: true, PlaceLatitude: &nearLat, PlaceLongitude: &nearLon},
		{ID: "g1", Name: "LA Fitness", Affinity: 0.85, Popularity: 0.7, scoreComplete: true, PlaceLatitude: &farLat, PlaceLongitude: &farLon, Properties: []byte(`{"keywords":[{"name":"gym"}]}`)},
	}}
	signal := &QlooSignal{LocalityQuery: "Los Angeles, CA", Latitude: &lat, Longitude: &lon}
	cands, _, err := discoverCandidates(context.Background(), q, signal, []string{"urn:tag:offerings:place:kids_haircuts"}, "hair_salon", 50, 1600)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].ID != "g1" {
		t.Fatalf("expected cross-category gym after radius widen, got %+v", cands)
	}
}

func TestRankPartnersWidensWhenTooFewInRadius(t *testing.T) {
	lat, lon := 34.1016, -118.3267
	nearLat, nearLon := lat+0.001, lon+0.001
	farLat, farLon := lat+0.08, lon+0.08
	raw := []QlooCandidate{
		{ID: "near", Name: "Neighborhood Grocery", Affinity: 0.76, Popularity: 0.01, scoreComplete: true, PlaceLatitude: &nearLat, PlaceLongitude: &nearLon},
		{ID: "g1", Name: "LA Fitness", Affinity: 0.85, Popularity: 0.7, scoreComplete: true, PlaceLatitude: &farLat, PlaceLongitude: &farLon},
		{ID: "g2", Name: "Equinox", Affinity: 0.84, Popularity: 0.69, scoreComplete: true, PlaceLatitude: &farLat, PlaceLongitude: &farLon},
	}
	signal := &QlooSignal{Latitude: &lat, Longitude: &lon}
	partners, _, _, widened := rankPartnersFromInsights(raw, signal, "hair_salon", 2000)
	if !widened {
		t.Fatal("expected widen when only one in-radius partner")
	}
	if len(partners) < minPartnerCandidates {
		t.Fatalf("partners=%d want >= %d", len(partners), minPartnerCandidates)
	}
}

func TestDiscoverCandidatesFallbackWithoutConfig(t *testing.T) {
	q := &fakeQloo{cands: []QlooCandidate{{ID: "q1", Name: "Place"}}}
	signal := &QlooSignal{LocalityQuery: "Honolulu, HI"}
	cands, _, err := discoverCandidates(context.Background(), q, signal, nil, "", 50, 10000)
	if err != nil || len(cands) != 1 {
		t.Fatalf("cands=%v err=%v", cands, err)
	}
	if len(q.seenFilters) != 1 {
		t.Fatalf("expected one insights call, got %+v", q.seenFilters)
	}
}

func TestPartnerDiscoveryJSONPathFromDataDir(t *testing.T) {
	dir, ok := scenario.FindDataDir("")
	if !ok {
		t.Skip("data dir not found from cwd")
	}
	want := filepath.Join(filepath.Dir(dir), "qloo", "partner_discovery.json")
	if got := partnerDiscoveryJSONPath(); got != want {
		t.Fatalf("path = %q want %q", got, want)
	}
}
