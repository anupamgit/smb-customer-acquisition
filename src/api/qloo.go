package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Qloo is the interface the orchestrator uses for Qloo calls. The
// production implementation is qlooClient; tests use a fake.
type Qloo interface {
	// ResolveTag looks one bought name up in the Qloo Tags API. It
	// returns ("", false, nil) when the name does not resolve: the caller
	// drops the input and continues on geography alone.
	ResolveTag(ctx context.Context, name string) (string, bool, error)
	SearchTags(ctx context.Context, query string) ([]qlooTagRow, error)
	// Insights returns candidate places for the derived signals, with
	// explainability requested. It sends derived signals only.
	Insights(ctx context.Context, signal *QlooSignal, tagURNs []string) ([]QlooCandidate, error)
	// InsightsWithFilters is used for cross-category partner discovery (include/exclude tags).
	InsightsWithFilters(ctx context.Context, signal *QlooSignal, f InsightsFilters) ([]QlooCandidate, error)
}

// qlooClient calls the Qloo REST API (Insights and Tags) as documented
// on docs.qloo.com and read in the Oct 27 feasibility research:
// base https://api.qloo.com, GET /v2/tags with filter.query, and
// GET/POST /v2/insights with filter.type, signal/filter location
// queries, signal.interests.tags, and feature.explainability. The
// hackathon API host may differ; QLOO_BASE_URL overrides the default.
// No live Qloo call was possible in this workflow (no key), so the exact
// request/response shapes are NOT live-verified and must be confirmed
// on the first live run. The client parses defensively and never
// invents a candidate: unparseable results are dropped and counted,
// and an empty result is reported as an empty result.
// Audiences are deliberately not used (deprecated in the Qloo docs).
// No demographics inputs are sent: no age or gender data exists.
type qlooClient struct {
	baseURL              string
	apiKey               string
	httpClient           *http.Client
	locationRadiusMeters int // 0 = legacy lat,lon without radius
}

// Qloo hackathon docs: no published numeric rate limit; Insights take ≤ 50.
// Use bounded exponential backoff on 429/5xx and respect context deadlines.
var qlooRetryDelays = []time.Duration{
	time.Second, 2 * time.Second, 4 * time.Second,
}

func newQlooClient(cfg config) *qlooClient {
	return &qlooClient{
		baseURL:              strings.TrimRight(cfg.qlooBaseURL, "/"),
		apiKey:               cfg.qlooKey,
		httpClient:           &http.Client{Timeout: 45 * time.Second},
		locationRadiusMeters: cfg.effectiveQlooLocationRadiusMeters(),
	}
}

func (c *qlooClient) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	if c.apiKey == "" {
		return nil, errNotConfigured
	}
	var respBody []byte
	err := doWithRetry(qlooRetryDelays, func(attempt int) error {
		u := c.baseURL + path
		if len(query) > 0 {
			u += "?" + query.Encode()
		}
		var reqBody io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reqBody = strings.NewReader(string(b))
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
		if err != nil {
			return err
		}
		// Header name as documented for Qloo API keys; not live-verified.
		req.Header.Set("X-Api-Key", c.apiKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return &transientError{err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return &transientError{err: err}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := string(b)
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return &statusError{status: resp.StatusCode, body: msg}
		}
		respBody = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

const menuItemTagTypes = "urn:tag:offerings:place"

// ResolveTag maps a sold item name to a Qloo tag URN via GET /v2/tags with
// offerings place tags and semantic search; the first result wins.
func (c *qlooClient) ResolveTag(ctx context.Context, name string) (string, bool, error) {
	if urn, ok := menuLineTagOverride(name); ok {
		return urn, true, nil
	}
	tags, err := c.searchMenuItemTags(ctx, name)
	if err != nil {
		return "", false, err
	}
	urn, ok := pickMenuItemTagURN(tags)
	return urn, ok, nil
}

// SearchTags queries GET /v2/tags with filter.query (partial match).
func (c *qlooClient) SearchTags(ctx context.Context, query string) ([]qlooTagRow, error) {
	q := url.Values{}
	q.Set("filter.query", query)
	q.Set("take", "50")
	b, err := c.do(ctx, http.MethodGet, "/v2/tags", q, nil)
	if err != nil {
		return nil, fmt.Errorf("qloo tags lookup failed: %w", err)
	}
	return parseQlooTagsResponse(b)
}

func (c *qlooClient) searchMenuItemTags(ctx context.Context, query string) ([]qlooTagRow, error) {
	q := url.Values{}
	q.Set("filter.query", query)
	q.Set("take", "10")
	q.Set("filter.tag.types", menuItemTagTypes)
	q.Set("feature.semantic_search", "true")
	b, err := c.do(ctx, http.MethodGet, "/v2/tags", q, nil)
	if err != nil {
		return nil, fmt.Errorf("qloo tags lookup failed: %w", err)
	}
	return parseQlooTagsResponse(b)
}

type qlooTagRow struct {
	ID   string
	Name string
}

func parseQlooTagsResponse(b []byte) ([]qlooTagRow, error) {
	var doc struct {
		Success bool `json:"success"`
		Results struct {
			Tags []struct {
				TagID string `json:"tag_id"`
				ID    string `json:"id"`
				Name  string `json:"name"`
			} `json:"tags"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("qloo tags response could not be parsed: %w", err)
	}
	var rows []qlooTagRow
	for _, t := range doc.Results.Tags {
		id := t.TagID
		if id == "" {
			id = t.ID
		}
		if id != "" && t.Name != "" {
			rows = append(rows, qlooTagRow{ID: id, Name: t.Name})
		}
	}
	return rows, nil
}

type qlooEntityRow struct {
	ID             string
	Name           string
	Type           string
	Affinity       float64
	Popularity     float64
	ScoreComplete  bool
	TagLabels      []string
	PlaceLat       *float64
	PlaceLon       *float64
	Properties     json.RawMessage
	Explainability json.RawMessage
}

func parseQlooInsightsResponse(b []byte) ([]qlooEntityRow, bool, error) {
	var doc struct {
		Success *bool `json:"success"`
		Results struct {
			Entities []struct {
				EntityID       string          `json:"entity_id"`
				Name           string          `json:"name"`
				Subtype        string          `json:"subtype"`
				Type           string          `json:"type"`
				Popularity     *float64        `json:"popularity"`
				Properties     json.RawMessage `json:"properties"`
				Explainability json.RawMessage `json:"explainability"`
				Tags []struct {
					Name string `json:"name"`
				} `json:"tags"`
				Location *struct {
					Lat float64 `json:"lat"`
					Lon float64 `json:"lon"`
				} `json:"location"`
				Query *struct {
					Affinity *float64 `json:"affinity"`
				} `json:"query"`
			} `json:"entities"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, false, fmt.Errorf("qloo insights response could not be parsed: %w", err)
	}
	var rows []qlooEntityRow
	for _, e := range doc.Results.Entities {
		if e.EntityID == "" || e.Name == "" {
			continue
		}
		typ := e.Subtype
		if typ == "" {
			typ = e.Type
		}
		var aff, pop float64
		complete := e.Query != nil && e.Query.Affinity != nil && e.Popularity != nil
		if complete {
			aff = *e.Query.Affinity
			pop = *e.Popularity
		}
		var tagLabels []string
		for _, t := range e.Tags {
			if n := strings.TrimSpace(t.Name); n != "" {
				tagLabels = append(tagLabels, n)
			}
		}
		var placeLat, placeLon *float64
		if e.Location != nil && (e.Location.Lat != 0 || e.Location.Lon != 0) {
			lat, lon := e.Location.Lat, e.Location.Lon
			placeLat, placeLon = &lat, &lon
		}
		rows = append(rows, qlooEntityRow{
			ID: e.EntityID, Name: e.Name, Type: typ,
			Affinity: aff, Popularity: pop, ScoreComplete: complete, TagLabels: tagLabels,
			PlaceLat: placeLat, PlaceLon: placeLon,
			Properties: e.Properties, Explainability: e.Explainability,
		})
	}
	ok := doc.Success == nil || *doc.Success
	return rows, ok, nil
}

// InsightsFilters optional include/exclude tags for partner discovery probes and future orchestrator use.
type InsightsFilters struct {
	InterestTags    []string
	FilterTags      []string
	ExcludeTags     []string
	ExcludeEntities []string
	Take            int
	// LocationRadiusMeters overrides the client default for this Insights call (meters, POINT lon lat).
	LocationRadiusMeters int
}

func (c *qlooClient) Insights(ctx context.Context, signal *QlooSignal, tagURNs []string) ([]QlooCandidate, error) {
	return c.InsightsWithFilters(ctx, signal, InsightsFilters{InterestTags: tagURNs})
}

func (c *qlooClient) InsightsWithFilters(ctx context.Context, signal *QlooSignal, f InsightsFilters) ([]QlooCandidate, error) {
	q := url.Values{}
	q.Set("filter.type", "urn:entity:place")
	if signal.LocalityQuery != "" {
		q.Set("filter.location.query", signal.LocalityQuery)
	}
	if signal.Latitude != nil && signal.Longitude != nil {
		lat, lon := *signal.Latitude, *signal.Longitude
		locationRadius := c.locationRadiusMeters
		if f.LocationRadiusMeters > 0 {
			locationRadius = f.LocationRadiusMeters
		}
		if locationRadius > 0 {
			q.Set("filter.location", fmt.Sprintf("POINT(%v %v)", lon, lat))
			q.Set("filter.location.radius", fmt.Sprintf("%d", locationRadius))
		} else {
			q.Set("filter.location", fmt.Sprintf("%v,%v", lat, lon))
		}
	}
	interestTags := normalizeInterestTagList(f.InterestTags)
	if len(interestTags) > 0 {
		q.Set("signal.interests.tags", strings.Join(interestTags, ","))
	}
	if len(f.FilterTags) > 0 {
		q.Set("filter.tags", strings.Join(f.FilterTags, ","))
	}
	if len(f.ExcludeTags) > 0 {
		q.Set("filter.exclude.tags", strings.Join(f.ExcludeTags, ","))
	}
	if len(f.ExcludeEntities) > 0 {
		q.Set("filter.exclude.entities", strings.Join(f.ExcludeEntities, ","))
	}
	q.Set("feature.explainability", "true")
	take := f.Take
	if take <= 0 {
		take = 20
	}
	if take > 50 {
		take = 50
	}
	q.Set("take", fmt.Sprintf("%d", take))

	b, err := c.do(ctx, http.MethodGet, "/v2/insights", q, nil)
	if err != nil {
		return nil, fmt.Errorf("qloo insights call failed: %w", err)
	}
	entities, ok, err := parseQlooInsightsResponse(b)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("qloo insights response reported success=false")
	}
	var candidates []QlooCandidate
	for _, r := range entities {
		city := ""
		if len(r.Properties) > 0 {
			var props struct {
				Geocode *struct {
					City string `json:"city"`
				} `json:"geocode"`
				Address string `json:"address"`
			}
			_ = json.Unmarshal(r.Properties, &props)
			if props.Geocode != nil {
				city = props.Geocode.City
			}
		}
		candidates = append(candidates, QlooCandidate{
			ID: r.ID, Name: r.Name, Type: r.Type,
			Affinity: r.Affinity, Popularity: r.Popularity, scoreComplete: r.ScoreComplete,
			TagLabels: append([]string(nil), r.TagLabels...),
			PlaceLatitude: r.PlaceLat, PlaceLongitude: r.PlaceLon,
			City: city, Properties: r.Properties, Explainability: r.Explainability,
		})
	}
	return candidates, nil
}
