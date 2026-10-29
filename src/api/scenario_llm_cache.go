package main

import (
	"sync"
	"time"
)

// llmTailCache stores LLM-through-UI outcomes per demo scenario (hackathon cost control).
type llmTailCache struct {
	mu      sync.Mutex
	entries map[string]llmTailCacheEntry
}

type llmTailCacheEntry struct {
	until         time.Time
	completed     bool // set only after a full successful run (evaluate → rank → explain)
	agentOutput   []AgentDraft
	ranked        []AgentDraft
	opportunities []Opportunity
}

func (e llmTailCacheEntry) usable() bool {
	return e.completed && len(e.agentOutput) > 0 && len(e.ranked) > 0 && len(e.opportunities) > 0
}

func newLLMTailCache() *llmTailCache {
	return &llmTailCache{entries: map[string]llmTailCacheEntry{}}
}

// sharedLLMTailCache is process-wide so cache hits survive per-request orchestrators.
var sharedLLMTailCache = newLLMTailCache()

func (c *llmTailCache) get(scenarioKey string, now time.Time) (llmTailCacheEntry, bool) {
	if c == nil || scenarioKey == "" {
		return llmTailCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.entries[scenarioKey]
	if !ok || now.After(ent.until) || !ent.usable() {
		if ok {
			delete(c.entries, scenarioKey)
		}
		return llmTailCacheEntry{}, false
	}
	return ent, true
}

// putCompleted stores LLM tail outcomes only after a successful end-to-end run.
func (c *llmTailCache) putCompleted(scenarioKey string, agent []AgentDraft, ranked []AgentDraft, opps []Opportunity, ttl time.Duration, now time.Time) {
	if c == nil || scenarioKey == "" || ttl <= 0 {
		return
	}
	ent := llmTailCacheEntry{
		until:         now.Add(ttl),
		completed:     true,
		agentOutput:   cloneAgentDrafts(agent),
		ranked:        cloneAgentDrafts(ranked),
		opportunities: cloneOpportunities(opps),
	}
	if !ent.usable() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[scenarioKey] = ent
}

func scenarioCacheKey(c Connector, cfg config) string {
	if sn := scenarioNameForConnector(c); sn != "" {
		return sn
	}
	if cfg.useLiveConnector() && cfg.businessType != "" {
		return "live:" + cfg.businessType
	}
	if cfg.scenarioName != "" {
		return cfg.scenarioName
	}
	return ""
}

func cloneAgentDrafts(in []AgentDraft) []AgentDraft {
	if len(in) == 0 {
		return nil
	}
	return append([]AgentDraft(nil), in...)
}

func cloneOpportunities(in []Opportunity) []Opportunity {
	if len(in) == 0 {
		return nil
	}
	out := make([]Opportunity, len(in))
	copy(out, in)
	return out
}
