package main

import (
	"testing"
	"time"
)

func TestLLMTailCacheExpires(t *testing.T) {
	c := newLLMTailCache()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.putCompleted("coffee-nyc",
		[]AgentDraft{{CandidateID: "a"}},
		[]AgentDraft{{CandidateID: "a"}},
		[]Opportunity{{PlaceName: "X"}},
		5*time.Minute, now)
	if _, ok := c.get("coffee-nyc", now.Add(4*time.Minute)); !ok {
		t.Fatal("expected hit before expiry")
	}
	if _, ok := c.get("coffee-nyc", now.Add(6*time.Minute)); ok {
		t.Fatal("expected miss after expiry")
	}
}

func TestLLMTailCacheSkipsIncompleteEntries(t *testing.T) {
	c := newLLMTailCache()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.mu.Lock()
	c.entries["partial"] = llmTailCacheEntry{
		until:         now.Add(time.Hour),
		completed:     false,
		agentOutput:   []AgentDraft{{CandidateID: "a"}},
		ranked:        []AgentDraft{{CandidateID: "a"}},
		opportunities: []Opportunity{{PlaceName: "X"}},
	}
	c.mu.Unlock()
	if _, ok := c.get("partial", now); ok {
		t.Fatal("incomplete entry must not be served")
	}
}

func TestLLMTailCachePutCompletedRequiresFullPayload(t *testing.T) {
	c := newLLMTailCache()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.putCompleted("empty", nil, nil, nil, time.Hour, now)
	if _, ok := c.get("empty", now); ok {
		t.Fatal("empty payload must not be cached")
	}
}

func TestScenarioCacheKeyUsesScenarioName(t *testing.T) {
	cfg := config{scenarioName: "ignored"}
	conn := &scenarioConnector{name: "salon-la"}
	if got := scenarioCacheKey(conn, cfg); got != "salon-la" {
		t.Fatalf("key = %q", got)
	}
}
