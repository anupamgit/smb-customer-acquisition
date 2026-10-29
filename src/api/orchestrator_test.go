package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeConnector struct{ data *SquareSourceData }

func (f *fakeConnector) FetchSourceData(context.Context) (*SquareSourceData, error) {
	return f.data, nil
}

type fakeQloo struct {
	resolve       map[string]string // bought name -> tag URN
	cands         []QlooCandidate
	seenSig       *QlooSignal
	seenTag       []string
	seenFilters   []InsightsFilters
	insightsCalls int
}

func (f *fakeQloo) ResolveTag(_ context.Context, name string) (string, bool, error) {
	if urn, ok := f.resolve[name]; ok {
		return urn, true, nil
	}
	return "", false, nil
}

func (f *fakeQloo) SearchTags(_ context.Context, _ string) ([]qlooTagRow, error) {
	return nil, nil
}

func (f *fakeQloo) Insights(_ context.Context, signal *QlooSignal, tagURNs []string) ([]QlooCandidate, error) {
	f.seenSig = signal
	f.seenTag = tagURNs
	return markFakeQlooScores(f.cands), nil
}

func (f *fakeQloo) InsightsWithFilters(_ context.Context, signal *QlooSignal, filters InsightsFilters) ([]QlooCandidate, error) {
	f.insightsCalls++
	f.seenSig = signal
	f.seenTag = filters.InterestTags
	f.seenFilters = append(f.seenFilters, filters)
	return markFakeQlooScores(f.cands), nil
}

func markFakeQlooScores(cands []QlooCandidate) []QlooCandidate {
	out := append([]QlooCandidate(nil), cands...)
	for i := range out {
		out[i].scoreComplete = true
	}
	return out
}

type fakeLLM struct {
	drafts  []AgentDraft
	seen    *LLMPayload
	seenRaw string
	err     error
	calls   int
}

func (f *fakeLLM) Complete(_ context.Context, payload LLMPayload) ([]AgentDraft, error) {
	f.calls++
	f.seen = &payload
	b, _ := json.Marshal(payload)
	f.seenRaw = string(b)
	return f.drafts, f.err
}

func fixtureData() *SquareSourceData {
	mk := func(id, locality string, n int, total int64, closed, item string) (SquareCustomer, []SquareOrder) {
		c := SquareCustomer{ID: id, Email: id + "@example.com"}
		if locality != "" {
			c.Address = &SquareAddress{Locality: locality, AdminRegion: "HI", PostalCode: "96813"}
		}
		var orders []SquareOrder
		for i := 0; i < n; i++ {
			orders = append(orders, SquareOrder{ID: id + "-o", CustomerID: id, ClosedAt: closed, TotalMoney: &SquareMoney{Amount: total / int64(n), Currency: "USD"}, LineItems: []SquareLineItem{{Name: item}}})
		}
		return c, orders
	}
	var customers []SquareCustomer
	var orders []SquareOrder
	rows := []struct {
		id, loc, closed, item string
		n                     int
		total                 int64
	}{
		{"cust_a1", "Honolulu", "2026-10-27T00:00:00Z", "Coffee", 3, 90000},
		{"cust_a2", "Honolulu", "2026-10-20T00:00:00Z", "Sandwich", 2, 60000},
		{"cust_a3", "Honolulu", "2026-10-10T00:00:00Z", "Coffee", 1, 30000},
		{"cust_a4", "Kapolei", "2026-09-01T00:00:00Z", "Tea", 1, 20000},
		{"cust_a5", "", "2026-08-01T00:00:00Z", "Tea", 1, 10000},
		{"cust_a6", "Honolulu", "2026-10-27T00:00:00Z", "Coffee", 0, 0},
	}
	for _, r := range rows {
		c, o := mk(r.id, r.loc, r.n, r.total, r.closed, r.item)
		customers = append(customers, c)
		orders = append(orders, o...)
	}
	lat, lon := 21.3, -157.8
	return &SquareSourceData{
		Customers: customers,
		Orders:    orders,
		Locations: []SquareLocation{{ID: "loc1", Coordinates: &struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}{Latitude: lat, Longitude: lon}}},
	}
}

func TestOrchestratorScenarioLLMCacheSkipsSecondCall(t *testing.T) {
	cands := []QlooCandidate{
		{ID: "q1", Name: "Gym", Affinity: 0.85, Popularity: 0.7, City: "Honolulu", Explainability: []byte(`{}`)},
	}
	q := &fakeQloo{resolve: map[string]string{"Coffee": "urn:tag:coffee"}, cands: cands}
	llm := &fakeLLM{drafts: []AgentDraft{
		{CandidateID: "q1", Action: "Partner", WhyFits: "Near", NextAction: "Visit", BusinessCompatibility: 0.9, EstimatedEffort: "low"},
	}}
	cfg := config{llmCacheEnabled: true, llmCacheTTLSeconds: 300}
	conn := &scenarioConnector{name: "coffee-nyc", data: fixtureData()}
	o := newOrchestrator(newMemoryStore(), conn, q, llm, cfg)
	run1, err := o.Run(context.Background(), nil)
	if err != nil || run1.Status != RunStatusSucceeded {
		t.Fatalf("first run: %v %+v", err, run1)
	}
	if llm.calls != 1 {
		t.Fatalf("first run llm calls = %d want 1", llm.calls)
	}
	run2, err := o.Run(context.Background(), nil)
	if err != nil || run2.Status != RunStatusSucceeded {
		t.Fatalf("second run: %v %+v", err, run2)
	}
	if llm.calls != 1 {
		t.Fatalf("second run should use cache, llm calls = %d want 1", llm.calls)
	}
	if q.insightsCalls != 2 {
		t.Fatalf("Qloo must run each time; insights calls = %d want 2", q.insightsCalls)
	}
}

func TestOrchestratorScenarioLLMCacheStillWritesAudit(t *testing.T) {
	cands := []QlooCandidate{
		{ID: "q1", Name: "Gym", Affinity: 0.85, Popularity: 0.7, City: "Honolulu", Explainability: []byte(`{}`)},
	}
	q := &fakeQloo{resolve: map[string]string{"Coffee": "urn:tag:coffee"}, cands: cands}
	llm := &fakeLLM{drafts: []AgentDraft{
		{CandidateID: "q1", Action: "Partner", WhyFits: "Near", NextAction: "Visit", BusinessCompatibility: 0.9, EstimatedEffort: "low"},
	}}
	cfg := config{llmCacheEnabled: true, llmCacheTTLSeconds: 300, pipelineAuditEnabled: true}
	conn := &scenarioConnector{name: "coffee-nyc", data: fixtureData()}
	o := newOrchestrator(newMemoryStore(), conn, q, llm, cfg)
	run1, err := o.Run(context.Background(), nil)
	if err != nil || run1.Status != RunStatusSucceeded {
		t.Fatalf("first run: %v %+v", err, run1)
	}
	run2, err := o.Run(context.Background(), nil)
	if err != nil || run2.Status != RunStatusSucceeded {
		t.Fatalf("second run: %v %+v", err, run2)
	}
	auditPath, err := pipelineAuditDir()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(auditPath, run2.ID+".md"))
	if err != nil {
		t.Fatalf("expected audit for cached run: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "Insights response (raw from Qloo)") {
		t.Fatalf("audit missing Qloo sections: %s", text)
	}
	if !strings.Contains(text, "cached — OpenRouter not called") {
		t.Fatalf("audit missing cached LLM marker: %s", text)
	}
}

func TestOrchestratorHappyPath(t *testing.T) {
	cands := []QlooCandidate{
		{ID: "q1", Name: "Corner Cafe", Affinity: 0.85, Popularity: 0.7, City: "Honolulu", Explainability: []byte(`{"x":1}`)},
		{ID: "q2", Name: "Far Diner", Affinity: 0.6, Popularity: 0.5, City: "Kapolei"},
	}
	q := &fakeQloo{resolve: map[string]string{"Coffee": "urn:tag:coffee"}, cands: cands}
	llm := &fakeLLM{drafts: []AgentDraft{
		{CandidateID: "q1", Action: "Offer a joint loyalty card", WhyFits: "Same area", NextAction: "Call the owner", BusinessCompatibility: 0.9, EstimatedEffort: "low"},
		{CandidateID: "unknown", Action: "Invented", WhyFits: "x", NextAction: "y", BusinessCompatibility: 1, EstimatedEffort: "low"},
		{CandidateID: "q2", Action: "Cross-promote", WhyFits: "Nearby", NextAction: "Email", BusinessCompatibility: 0.5, EstimatedEffort: "high"},
	}}
	o := newOrchestrator(newMemoryStore(), &fakeConnector{data: fixtureData()}, q, llm, config{})
	run, err := o.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if run.Status != RunStatusSucceeded || len(run.Opportunities) != 2 {
		t.Fatalf("run = status %s opportunities %d, want succeeded/2", run.Status, len(run.Opportunities))
	}
	if run.Opportunities[0].CandidateID != "q1" {
		t.Fatalf("first opportunity = %s, want q1", run.Opportunities[0].CandidateID)
	}
	if run.Signal.GeographyOnly {
		t.Fatalf("signal must not be geography-only when Coffee resolved")
	}
	// No PII may reach Qloo or the LLM: the captured payloads must not
	// contain customer IDs or email addresses from the fixture.
	if strings.Contains(llm.seenRaw, "cust_") || strings.Contains(llm.seenRaw, "@example.com") {
		t.Fatalf("LLM payload contains customer PII: %s", llm.seenRaw)
	}
	sigRaw, _ := json.Marshal(q.seenSig)
	if strings.Contains(string(sigRaw), "cust_") {
		t.Fatalf("Qloo signal contains customer IDs: %s", sigRaw)
	}
	if len(q.seenTag) != 1 || q.seenTag[0] != "urn:tag:coffee" {
		t.Fatalf("resolved tags = %v", q.seenTag)
	}
	// Feedback: valid index records, invalid index is rejected.
	if err := RecordFeedback(context.Background(), o.store, run.ID, Feedback{OpportunityIndex: 0, Worked: true}, time.Now()); err != nil {
		t.Fatalf("feedback failed: %v", err)
	}
	if err := RecordFeedback(context.Background(), o.store, run.ID, Feedback{OpportunityIndex: 9}, time.Now()); err == nil {
		t.Fatalf("out-of-range feedback must be rejected")
	}
}

func TestOrchestratorGeographyFallback(t *testing.T) {
	q := &fakeQloo{resolve: map[string]string{}, cands: []QlooCandidate{{ID: "q1", Name: "Corner Cafe", Affinity: 0.8, City: "Honolulu"}}}
	llm := &fakeLLM{drafts: []AgentDraft{{CandidateID: "q1", Action: "Partner", WhyFits: "Near", NextAction: "Visit", BusinessCompatibility: 0.8, EstimatedEffort: "medium"}}}
	o := newOrchestrator(newMemoryStore(), &fakeConnector{data: fixtureData()}, q, llm, config{})
	run, err := o.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("geography fallback run failed: %v", err)
	}
	if !run.Signal.GeographyOnly || len(run.Resolutions) == 0 {
		t.Fatalf("fallback not recorded: signal=%+v resolutions=%v", run.Signal, run.Resolutions)
	}
	for _, r := range run.Resolutions {
		if r.Resolved {
			t.Fatalf("no name should resolve in this test: %+v", r)
		}
	}
}

func TestOrchestratorEmptyQlooFailsVisibly(t *testing.T) {
	q := &fakeQloo{resolve: map[string]string{}, cands: nil}
	llm := &fakeLLM{drafts: []AgentDraft{{CandidateID: "q1", Action: "x", BusinessCompatibility: 0.5, EstimatedEffort: "low"}}}
	o := newOrchestrator(newMemoryStore(), &fakeConnector{data: fixtureData()}, q, llm, config{})
	run, err := o.Run(context.Background(), nil)
	if err == nil || run.Status != RunStatusFailed || run.Stage != "discover_candidates" {
		t.Fatalf("empty Qloo result must fail visibly at discover_candidates: run=%+v err=%v", run, err)
	}
	if len(run.Opportunities) != 0 {
		t.Fatalf("failed run must not carry opportunities")
	}
}

func TestOrchestratorLLMFailureFailsVisibly(t *testing.T) {
	q := &fakeQloo{resolve: map[string]string{}, cands: []QlooCandidate{{ID: "q1", Name: "Corner Cafe", Affinity: 0.8}}}
	llm := &fakeLLM{err: errors.New("llm unavailable")}
	o := newOrchestrator(newMemoryStore(), &fakeConnector{data: fixtureData()}, q, llm, config{})
	run, err := o.Run(context.Background(), nil)
	if err == nil || run.Status != RunStatusFailed || run.Stage != "evaluate_opportunities" {
		t.Fatalf("LLM failure must fail visibly at evaluate_opportunities: run=%+v err=%v", run, err)
	}
}

func TestMarkRunFailedWhenStillRunning(t *testing.T) {
	store := newMemoryStore()
	o := newOrchestrator(store, &fakeConnector{data: fixtureData()}, &fakeQloo{}, &fakeLLM{}, config{})
	now := time.Now().UTC()
	run := &Run{ID: "run_test_stuck", Status: RunStatusRunning, Stage: "discover_candidates", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	o.markRunFailedIfStillRunning(run, context.DeadlineExceeded)
	got, err := store.GetRun(context.Background(), run.ID)
	if err != nil || got.Status != RunStatusFailed || got.Error == "" {
		t.Fatalf("stored run = %+v err=%v", got, err)
	}
}

func TestOrchestratorNoCustomersFailsVisibly(t *testing.T) {
	q := &fakeQloo{cands: []QlooCandidate{{ID: "q1", Name: "x"}}}
	llm := &fakeLLM{}
	o := newOrchestrator(newMemoryStore(), &fakeConnector{data: &SquareSourceData{}}, q, llm, config{})
	run, err := o.Run(context.Background(), nil)
	if err == nil || run.Status != RunStatusFailed {
		t.Fatalf("empty connector data must fail visibly: run=%+v err=%v", run, err)
	}
}
