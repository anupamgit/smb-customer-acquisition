package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSquareClientParsesFixtures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/locations", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Authorization header")
		}
		_, _ = w.Write([]byte(`{"locations":[{"id":"loc1","name":"Shop","coordinates":{"latitude":21.3,"longitude":-157.8}}]}`))
	})
	mux.HandleFunc("/v2/customers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"customers":[{"id":"c1","address":{"locality":"Honolulu","administrative_district_level_1":"HI","postal_code":"96813","country":"US"}}]}`))
	})
	mux.HandleFunc("/v2/orders/search", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["location_ids"]; !ok {
			t.Errorf("orders search must include location_ids")
		}
		_, _ = w.Write([]byte(`{"orders":[{"id":"o1","customer_id":"c1","closed_at":"2026-10-25T00:00:00Z","state":"COMPLETED","total_money":{"amount":5000,"currency":"USD"},"line_items":[{"name":"Coffee"}]}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &squareClient{baseURL: srv.URL, apiKey: "test-key", version: "2026-09-16", httpClient: srv.Client()}
	data, err := c.FetchSourceData(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Locations) != 1 || len(data.Customers) != 1 || len(data.Orders) != 1 {
		t.Fatalf("data = %+v", data)
	}
	if data.Orders[0].LineItems[0].Name != "Coffee" {
		t.Fatalf("line item = %+v", data.Orders[0].LineItems)
	}
}

func TestSquareClientRetriesTransientOnly(t *testing.T) {
	old := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = old }()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("error"))
			return
		}
		_, _ = w.Write([]byte(`{"locations":[]}`))
	}))
	defer srv.Close()
	c := &squareClient{baseURL: srv.URL, apiKey: "k", version: "v", httpClient: srv.Client()}
	if _, err := c.listLocations(context.Background()); err != nil {
		t.Fatalf("transient failures must be retried: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}

	calls = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"errors":[{"code":"BAD","detail":"bad request"}]}`))
	}))
	defer srv2.Close()
	c2 := &squareClient{baseURL: srv2.URL, apiKey: "k", version: "v", httpClient: srv2.Client()}
	if _, err := c2.listLocations(context.Background()); err == nil {
		t.Fatalf("400 must fail")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on 400)", calls)
	}
}

func TestSquareClientMissingKey(t *testing.T) {
	c := &squareClient{baseURL: "http://example.invalid", apiKey: "", httpClient: http.DefaultClient}
	if _, err := c.FetchSourceData(context.Background()); err == nil {
		t.Fatalf("missing key must fail, not call the network")
	}
}

func TestQlooClientTagsAndInsights(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/tags", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("filter.query")
		if q == "NoSuchTag" {
			_, _ = w.Write([]byte(`{"success":true,"results":{"tags":[]}}`))
			return
		}
		if q == "Haircut" {
			_, _ = w.Write([]byte(`{"success":true,"results":{"tags":[{"id":"urn:tag:offerings:place:kids'_haircuts","name":"Kids' haircuts"},{"id":"urn:tag:offerings:place:kids_haircuts","name":"Kids' haircuts"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"results":{"tags":[{"id":"urn:tag:offerings:place:coffee","name":"Coffee"},{"id":"urn:tag:offerings:place:tea","name":"Tea"}]}}`))
	})
	mux.HandleFunc("/v2/insights", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"success":true,"results":{"entities":[{"entity_id":"q1","name":"Corner Cafe","subtype":"urn:entity:place","popularity":0.7,"properties":{"geocode":{"city":"Honolulu"}},"query":{"affinity":0.85},"explainability":{"contributions":[]}}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &qlooClient{baseURL: srv.URL, apiKey: "k", httpClient: srv.Client(), locationRadiusMeters: 5000}
	urn, ok, err := c.ResolveTag(context.Background(), "coffee")
	if err != nil || !ok || urn != "urn:tag:offerings:place:coffee" {
		t.Fatalf("resolve = %q %v %v", urn, ok, err)
	}
	if _, ok, _ := c.ResolveTag(context.Background(), "NoSuchTag"); ok {
		t.Fatalf("non-matching name must not resolve")
	}
	haircut, ok, err := c.ResolveTag(context.Background(), "Haircut")
	if err != nil || !ok || haircut != "urn:tag:offerings:place:kids_haircuts" {
		t.Fatalf("Haircut resolve = %q %v %v", haircut, ok, err)
	}
	lat, lon := 21.3099, -157.8581
	cands, err := c.Insights(context.Background(), &QlooSignal{LocalityQuery: "Honolulu, HI", City: "Honolulu", Latitude: &lat, Longitude: &lon}, []string{"urn:tag:offerings:place:coffee"})
	if err != nil || len(cands) != 1 || cands[0].City != "Honolulu" {
		t.Fatalf("candidates = %+v err=%v", cands, err)
	}
	for _, want := range []string{"filter.type=urn%3Aentity%3Aplace", "feature.explainability=true", "filter.location.radius=5000", "filter.location=POINT"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("insights query %q missing %s", gotQuery, want)
		}
	}
}

func TestLLMClientParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "test-model" {
			t.Errorf("model = %v", body["model"])
		}
		content, _ := json.Marshal(map[string]any{"opportunities": []map[string]any{{"candidateId": "q1", "action": "Partner", "whyFits": "Near", "nextAction": "Visit", "businessCompatibility": 0.8, "estimatedEffort": "low"}}})
		resp, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(content)}}}})
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	c := &llmClient{baseURL: srv.URL, apiKey: "k", provider: "test", model: "test-model", httpClient: srv.Client()}
	drafts, err := c.Complete(context.Background(), LLMPayload{Locality: "Honolulu", CohortSize: 3})
	if err != nil || len(drafts) != 1 || drafts[0].CandidateID != "q1" {
		t.Fatalf("drafts = %+v err=%v", drafts, err)
	}

	unconfigured := &llmClient{httpClient: srv.Client()}
	if _, err := unconfigured.Complete(context.Background(), LLMPayload{}); err == nil {
		t.Fatalf("unconfigured LLM client must fail, not fabricate output")
	}
}
