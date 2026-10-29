package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func mustTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestNormalizeSquare(t *testing.T) {
	data := &SquareSourceData{
		Customers: []SquareCustomer{
			{ID: "c1", Email: "a@example.com", Address: &SquareAddress{Locality: "Honolulu", AdminRegion: "HI", PostalCode: "96813"}},
			{ID: "c2"},
		},
		Orders: []SquareOrder{
			{ID: "o1", CustomerID: "c1", ClosedAt: "2026-10-20T00:00:00Z", TotalMoney: &SquareMoney{Amount: 5000, Currency: "USD"}, LineItems: []SquareLineItem{{Name: "Coffee"}}},
			{ID: "o2", CustomerID: "c1", ClosedAt: "2026-10-25T00:00:00Z", TotalMoney: &SquareMoney{Amount: 3000, Currency: "USD"}, LineItems: []SquareLineItem{{Name: "Coffee"}, {Name: "Sandwich"}}},
			{ID: "o3", CustomerID: "", ClosedAt: "2026-10-25T00:00:00Z", TotalMoney: &SquareMoney{Amount: 9999}},
			{ID: "o4", CustomerID: "c9", ClosedAt: "2026-10-24T00:00:00Z", TotalMoney: &SquareMoney{Amount: 1000}, LineItems: []SquareLineItem{{Name: "Tea"}}},
		},
	}
	records, unattributed := NormalizeSquare(data)
	if unattributed != 1 {
		t.Fatalf("unattributed = %d, want 1", unattributed)
	}
	byID := map[string]CanonicalRecord{}
	for _, r := range records {
		byID[r.CustomerID] = r
		if r.CustomerID == "c1" && (r.MissingFields != nil) {
			t.Errorf("c1 should have no missing location field, got %v", r.MissingFields)
		}
	}
	c1 := byID["c1"]
	if c1.TotalMinor != 8000 || c1.OrderCount != 2 || len(c1.ItemNames) != 2 {
		t.Errorf("c1 = %+v, want total 8000, 2 orders, 2 items", c1)
	}
	if c1.LastOrderAt == nil || !c1.LastOrderAt.Equal(*mustTime("2026-10-25T00:00:00Z")) {
		t.Errorf("c1 last order = %v", c1.LastOrderAt)
	}
	if byID["c2"].HasLocation {
		t.Errorf("c2 must not have a location")
	}
	// Order-only customer is kept, with location recorded missing.
	if _, ok := byID["c9"]; !ok {
		t.Errorf("order-only customer c9 was lost")
	}
}

func TestScoreCohort(t *testing.T) {
	now := *mustTime("2026-10-28T00:00:00Z")
	records := []CanonicalRecord{
		{CustomerID: "top", TotalMinor: 90000, OrderCount: 3, LastOrderAt: mustTime("2026-10-27T00:00:00Z")},
		{CustomerID: "mid", TotalMinor: 50000, OrderCount: 2, LastOrderAt: mustTime("2026-10-20T00:00:00Z")},
		{CustomerID: "low", TotalMinor: 10000, OrderCount: 1, LastOrderAt: mustTime("2026-01-01T00:00:00Z")},
		{CustomerID: "none", TotalMinor: 0, OrderCount: 0},
	}
	cohort := ScoreCohort(records, now)
	if len(cohort) != 1 || cohort[0].CustomerID != "top" {
		t.Fatalf("cohort = %v, want only top", cohort)
	}
	// Ten records -> top 2.
	var ten []CanonicalRecord
	for i := 0; i < 10; i++ {
		ten = append(ten, CanonicalRecord{CustomerID: string(rune('a' + i)), TotalMinor: int64((10 - i) * 1000), OrderCount: 1, LastOrderAt: mustTime("2026-10-27T00:00:00Z")})
	}
	if got := len(ScoreCohort(ten, now)); got != 2 {
		t.Fatalf("cohort size = %d, want 2", got)
	}
	if len(ScoreCohort(nil, now)) != 0 {
		t.Fatalf("empty input must give empty cohort")
	}
}

func TestBuildSignals(t *testing.T) {
	cohort := []ScoredCustomer{
		{CanonicalRecord: CanonicalRecord{CustomerID: "a", Locality: "Honolulu", AdminRegion: "HI", HasLocation: true, ItemNames: []string{"Coffee"}}},
		{CanonicalRecord: CanonicalRecord{CustomerID: "b", Locality: "Honolulu", AdminRegion: "HI", HasLocation: true, ItemNames: []string{"Sandwich"}}},
		{CanonicalRecord: CanonicalRecord{CustomerID: "c", Locality: "Kapolei", AdminRegion: "HI", HasLocation: true, ItemNames: []string{"Tea"}}},
	}
	sig := BuildSignals(cohort, nil, nil)
	if sig.City != "Honolulu" || sig.LocalityQuery != "Honolulu, HI" {
		t.Fatalf("signal = %+v", sig)
	}
	if len(sig.BoughtNames) != 3 || sig.BoughtNames[0] != "Coffee" {
		t.Fatalf("bought names = %v, want sorted", sig.BoughtNames)
	}
	lat, lon := 21.3, -157.8
	sig2 := BuildSignals(cohort, &lat, &lon)
	if sig2.Latitude == nil || *sig2.Latitude != 21.3 {
		t.Fatalf("coordinates not carried: %+v", sig2)
	}
}

func TestRankOpportunities(t *testing.T) {
	signal := &QlooSignal{City: "Honolulu"}
	cands := []QlooCandidate{
		{ID: "p1", Name: "Alpha", Affinity: 0.9, Popularity: 0.3, City: "Honolulu"},
		{ID: "p2", Name: "Beta", Affinity: 0.5, Popularity: 0.9, City: "Honolulu"},
	}
	drafts := []AgentDraft{
		{CandidateID: "p2", Action: "b", BusinessCompatibility: 0.5, EstimatedEffort: "high"},
		{CandidateID: "p1", Action: "a", BusinessCompatibility: 0.5, EstimatedEffort: "high"},
	}
	ranked := RankOpportunities(drafts, cands, signal)
	if len(ranked) != 2 || ranked[0].CandidateID != "p1" {
		t.Fatalf("ranked = %+v, want p1 first (affinity dominates)", ranked)
	}
}

func TestRankOpportunitiesPopularityBreaksAffinityTie(t *testing.T) {
	signal := &QlooSignal{City: "Honolulu"}
	const tie = 0.764996
	cands := []QlooCandidate{
		{ID: "low", Name: "Alpha", Affinity: tie, Popularity: 0.2, City: "Honolulu"},
		{ID: "high", Name: "Beta", Affinity: tie, Popularity: 0.9, City: "Honolulu"},
	}
	drafts := []AgentDraft{
		{CandidateID: "low", Action: "a", BusinessCompatibility: 0.5, EstimatedEffort: "medium"},
		{CandidateID: "high", Action: "b", BusinessCompatibility: 0.5, EstimatedEffort: "medium"},
	}
	ranked := RankOpportunities(drafts, cands, signal)
	if ranked[0].CandidateID != "high" {
		t.Fatalf("ranked = %+v, want higher popularity first when affinity ties", ranked)
	}
}

func TestBuildOpportunitiesProvenance(t *testing.T) {
	signal := &QlooSignal{LocalityQuery: "Honolulu, HI", City: "Honolulu", GeographyOnly: true}
	cands := []QlooCandidate{{ID: "p1", Name: "Alpha", Affinity: 0.8, Popularity: 0.7, Explainability: []byte(`{"contributions":[]}`)}}
	drafts := []AgentDraft{{CandidateID: "p1", Action: "Partner", WhyFits: "Near your best customers", NextAction: "Visit", BusinessCompatibility: 0.8, EstimatedEffort: "low"}}
	opps := BuildOpportunities(drafts, cands, signal, []ScoredCustomer{{CanonicalRecord: CanonicalRecord{CustomerID: "x", ItemNames: []string{"Coffee"}}}})
	if len(opps) != 1 || opps[0].Rank != 1 || opps[0].PlaceName != "Alpha" {
		t.Fatalf("opportunities = %+v", opps)
	}
	origins := map[string]bool{}
	for _, e := range opps[0].Evidence {
		origins[e.Origin] = true
	}
	if !origins[OriginBusiness] || !origins[OriginQloo] || !origins[OriginAgent] {
		t.Fatalf("evidence origins = %v, want all three distinct", origins)
	}
	if string(opps[0].Evidence[1].Raw) != `{"contributions":[]}` {
		t.Fatalf("qloo explainability was not preserved unchanged")
	}
}

func TestDoWithRetry(t *testing.T) {
	old := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = old }()

	attempts := 0
	err := doWithRetry(retryDelays, func(attempt int) error {
		attempts++
		if attempts < 3 {
			return &statusError{status: 500, body: "boom"}
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d, want success after 3", err, attempts)
	}

	attempts = 0
	err = doWithRetry(retryDelays, func(attempt int) error {
		attempts++
		return &statusError{status: 400, body: "bad"}
	})
	if err == nil || attempts != 1 {
		t.Fatalf("non-transient error must not retry: err=%v attempts=%d", err, attempts)
	}

	attempts = 0
	err = doWithRetry(retryDelays, func(attempt int) error {
		attempts++
		return errors.New("plain")
	})
	if err == nil || attempts != 1 {
		t.Fatalf("plain error must not retry: err=%v attempts=%d", err, attempts)
	}
}

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore()
	run := &Run{ID: "r1", Status: RunStatusRunning, Stage: "x", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.Status = RunStatusSucceeded
	if err := s.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, "r1")
	if err != nil || got.Status != RunStatusSucceeded {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	if err := s.SaveFeedback(ctx, "r1", Feedback{OpportunityIndex: 0, Worked: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestRun(ctx)
	if err != nil || len(latest.Feedback) != 1 {
		t.Fatalf("latest feedback = %+v err=%v", latest, err)
	}
	if _, err := s.GetRun(ctx, "missing"); !errors.Is(err, errRunNotFound) {
		t.Fatalf("missing run err = %v, want errRunNotFound", err)
	}
}
