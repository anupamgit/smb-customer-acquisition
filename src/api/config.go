package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// config holds server-side settings. Credential values are held only long
// enough to hand them to the three clients; they are never logged and
// never sent to the browser. The booleans record only configured or not.
type config struct {
	port               string
	databaseURL        string
	squareKey          string
	qlooKey            string
	llmKey             string
	squareBaseURL      string
	qlooBaseURL        string
	llmBaseURL         string
	llmProvider        string
	llmModel           string
	squareVersion      string
	liveConnectorEnabled bool // USE_LIVE_CONNECTOR: external business data vs built-in scenarios (default off)
	scenarioName       string
	scenarioDataDir    string
	businessType         string // square path: hair_salon, coffee_shop, etc.
	pipelineAuditEnabled bool
	matchChainUIEnabled  bool
	qlooInsightsTake         int
	qlooLocationRadiusMeters int
	llmCacheEnabled          bool
	llmCacheTTLSeconds       int
}

var knownBusinessTypes = map[string]bool{
	"coffee_shop": true, "hair_salon": true, "mexican_restaurant": true,
}

// configured reports which credentials are present, by name only.
func (c config) missingCredentials() []string {
	var missing []string
	if c.useLiveConnector() {
		if c.squareKey == "" {
			missing = append(missing, "SQUARE_API_KEY")
		}
	} else if !c.allScenarioDataReady() {
		missing = append(missing, "scenario data (run make generate-scenarios)")
	}
	if c.qlooKey == "" {
		missing = append(missing, "QLOO_API_KEY")
	}
	if c.llmProvider == "" {
		missing = append(missing, "LLM_PROVIDER")
	}
	if c.llmKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	if c.llmBaseURL == "" {
		missing = append(missing, "LLM_BASE_URL")
	}
	if c.llmModel == "" {
		missing = append(missing, "LLM_MODEL")
	}
	return missing
}

func (c config) llmConfigured() bool {
	return c.llmProvider != "" && c.llmKey != "" && c.llmBaseURL != "" && c.llmModel != ""
}

func envOr(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// qlooInsightsTakeLimit is the Insights API maximum documented for hackathon use.
const qlooInsightsTakeLimit = 50

// defaultQlooLocationRadiusMeters is ~6.2 mi; Qloo expects meters with POINT(lon lat).
const defaultQlooLocationRadiusMeters = 10000

const defaultLLMCacheTTLSeconds = 300 // 5 minutes per demo scenario

func (c config) effectiveQlooInsightsTake() int {
	take := c.qlooInsightsTake
	if take <= 0 {
		take = qlooInsightsTakeLimit
	}
	if take > qlooInsightsTakeLimit {
		take = qlooInsightsTakeLimit
	}
	return take
}

func (c config) effectiveQlooLocationRadiusMeters() int {
	r := c.qlooLocationRadiusMeters
	if r <= 0 {
		r = defaultQlooLocationRadiusMeters
	}
	if r > 100000 {
		r = 100000
	}
	return r
}

func (c config) effectiveLLMCacheTTL() time.Duration {
	if !c.llmCacheEnabled {
		return 0
	}
	sec := c.llmCacheTTLSeconds
	if sec <= 0 {
		sec = defaultLLMCacheTTLSeconds
	}
	return time.Duration(sec) * time.Second
}

func envIntOr(name string, def, min, max int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// forScenarioRun returns a copy that loads orders from generated scenario JSON
// (used when the UI picks a demo business while the server default is Square).
func (c config) forScenarioRun(name string) config {
	out := c
	out.liveConnectorEnabled = false
	out.scenarioName = name
	return out
}

func (c config) businessDataSource() string {
	if c.liveConnectorEnabled {
		return "live"
	}
	return "scenario"
}

func loadConfig() config {
	return config{
		port:               envOr("PORT", "8080"),
		databaseURL:        os.Getenv("DATABASE_URL"),
		squareKey:          os.Getenv("SQUARE_API_KEY"),
		qlooKey:            os.Getenv("QLOO_API_KEY"),
		llmProvider:        strings.TrimSpace(os.Getenv("LLM_PROVIDER")),
		llmKey:             strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		llmBaseURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		llmModel:           strings.TrimSpace(os.Getenv("LLM_MODEL")),
		squareBaseURL:      envOr("SQUARE_BASE_URL", "https://connect.squareupsandbox.com"),
		qlooBaseURL:        envOr("QLOO_BASE_URL", "https://hackathon.api.qloo.com"),
		squareVersion:      envOr("SQUARE_VERSION", "2026-09-16"),
		liveConnectorEnabled: envBool("USE_LIVE_CONNECTOR"),
		scenarioName:       strings.TrimSpace(envOr("SCENARIO", "coffee-nyc")),
		scenarioDataDir:    strings.TrimSpace(os.Getenv("SCENARIO_DATA_DIR")),
		businessType:         strings.TrimSpace(os.Getenv("BUSINESS_TYPE")),
		pipelineAuditEnabled: envBool("ADVISOR_PIPELINE_AUDIT"),
		matchChainUIEnabled:  envBool("ADVISOR_MATCH_CHAIN"),
		qlooInsightsTake:         envIntOr("QLOO_INSIGHTS_TAKE", qlooInsightsTakeLimit, 1, qlooInsightsTakeLimit),
		qlooLocationRadiusMeters: envIntOr("QLOO_LOCATION_RADIUS_METERS", defaultQlooLocationRadiusMeters, 500, 100000),
		llmCacheEnabled:          envBoolDefault("ADVISOR_LLM_CACHE", true),
		llmCacheTTLSeconds:       envIntOr("ADVISOR_LLM_CACHE_TTL_SECONDS", defaultLLMCacheTTLSeconds, 60, 3600),
	}
}

func envBoolDefault(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}

func envBool(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	return v == "1" || strings.EqualFold(v, "true")
}

// errNotConfigured is returned when a client is called without the
// credential or configuration it needs. The run then fails visibly
// instead of proceeding with invented data.
var errNotConfigured = errors.New("not configured: required server-side credential or setting is missing")

// transientError marks a failure that may be retried under the Step 4
// bounds: a network error, HTTP 429, or HTTP 5xx.
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// statusError is an HTTP failure from an external API. Only 429 and 5xx
// are transient; other client errors are never retried.
type statusError struct {
	status int
	body   string // truncated upstream body; never contains a credential
}

func (e *statusError) Error() string {
	return fmt.Sprintf("external API returned HTTP %d: %s", e.status, e.body)
}

// isTransient reports whether err is retryable under the Step 4 bounds.
func isTransient(err error) bool {
	var te *transientError
	if errors.As(err, &te) {
		return true
	}
	var se *statusError
	if errors.As(err, &se) {
		return se.status == http.StatusTooManyRequests || se.status >= 500
	}
	return false
}
