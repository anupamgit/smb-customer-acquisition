package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLM is the interface Agent Reasoning uses for language-model calls.
// The production implementation is llmClient; tests use a fake.
type LLM interface {
	// Complete sends derived, non-PII evidence and candidates and
	// returns structured agent drafts. It must never be called with
	// customer PII or raw customer records.
	Complete(ctx context.Context, payload LLMPayload) ([]AgentDraft, error)
}

// LLMPayload is the whole content sent to the LLM service: cohort
// aggregates and Qloo-returned candidates only. No per-customer data,
// no customer identifiers, names, emails, or addresses appear here.
type LLMPayload struct {
	Locality      string          `json:"locality"`
	CohortSize    int             `json:"cohortSize"`
	CohortBought  []string        `json:"cohortBoughtNames"`
	GeographyOnly bool            `json:"geographyOnly"`
	ResolvedTags  []string        `json:"resolvedTags"`
	Candidates    []QlooCandidate `json:"candidates"`
}

// llmClient calls an LLM Service REST API. Provider and model are NOT
// selected by this workflow (Step 4 deferred them to configuration);
// the deployer must set LLM_BASE_URL, LLM_PROVIDER, and LLM_MODEL.
// The client assumes an OpenAI-compatible chat-completions endpoint
// (POST {baseURL}/chat/completions, messages + json_object response
// format). That assumption is NOT verified against any real service in
// this workflow (no key, no provider selected) and must be confirmed
// before the first live run. If the base URL, provider, model, or key
// is missing, the client reports not-configured and the run fails
// visibly at the agent step; no agent output is fabricated.
type llmClient struct {
	baseURL    string
	apiKey     string
	provider   string
	model      string
	httpClient *http.Client
}

func newLLMClient(cfg config) *llmClient {
	return &llmClient{
		baseURL:    strings.TrimRight(cfg.llmBaseURL, "/"),
		apiKey:     cfg.llmKey,
		provider:   cfg.llmProvider,
		model:      cfg.llmModel,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

const llmSystemPrompt = `You help a local shop owner decide which nearby businesses to partner with. ` +
	`Prefer cross-category complements (e.g. a gym, hotel, or bookstore for a café)—not direct competitors selling the same thing. ` +
	`You receive: their area, how many of their best customers they have, what those customers usually buy, and a list of nearby places from a data partner (each has candidateId and name). ` +
	`Return JSON {"opportunities": [...]} with at most 5 opportunities, best fits first. ` +
	`Each opportunity MUST use a candidateId from that list only; do not name any business not in the list. ` +
	`Fields: candidateId, placeKind (2–4 plain words describing what the place is, e.g. "Neighborhood pizzeria" or "Sandwich shop" — never a city or country name), ` +
	`action (one specific partnership idea), whyFits (1–2 sentences in second person: talk to the owner as "you" / "your customers" / "your shop"), ` +
	`nextAction (one concrete step), businessCompatibility (0–1), estimatedEffort ("low", "medium", or "high"). ` +
	`Voice: plain English for a busy owner. Never use cohort, candidate, affinity, signal, or other analyst jargon. ` +
	`Do not invent statistics, contact details, or deals. Omit weak fits.`

func (c *llmClient) Complete(ctx context.Context, payload LLMPayload) ([]AgentDraft, error) {
	if c.baseURL == "" || c.apiKey == "" || c.model == "" {
		return nil, errNotConfigured
	}
	evidence, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": llmSystemPrompt},
			{"role": "user", "content": string(evidence)},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	var drafts []AgentDraft
	err = doWithRetry(retryDelays, func(attempt int) error {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return &transientError{err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return &transientError{err: err}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := string(raw)
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return &statusError{status: resp.StatusCode, body: msg}
		}
		var envelope struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return fmt.Errorf("llm response could not be parsed: %w", err)
		}
		if len(envelope.Choices) == 0 {
			return fmt.Errorf("llm response contained no choices")
		}
		var parsed struct {
			Opportunities []AgentDraft `json:"opportunities"`
		}
		if err := json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &parsed); err != nil {
			return fmt.Errorf("llm output was not the expected JSON: %w", err)
		}
		drafts = parsed.Opportunities
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm agent call failed: %w", err)
	}
	return drafts, nil
}
