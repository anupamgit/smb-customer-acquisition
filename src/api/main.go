// Command advisor is the Application Service for Customer Acquisition
// Advisor (CAA).
//
// It serves the built React frontend and the Application Service API:
// health, configuration state (names only, never values), analysis
// runs, and optional feedback. All product logic lives in the
// component files listed in doc.go. Configuration is server-side
// environment variables only; credential values are never logged and
// never sent to the browser.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"advisor/scenario"
)

type server struct {
	cfg           config
	store         Store
	postgresInUse bool
	orchestrator  *Orchestrator
}

type healthResponse struct {
	Status       string `json:"status"`
	Service      string `json:"service"`
	DBConfigured bool   `json:"dbConfigured"`
	Store        string `json:"store"`
}

type configResponse struct {
	BusinessDataSource string                  `json:"businessDataSource"`
	Scenario           string                  `json:"scenario,omitempty"`
	BusinessName       string                  `json:"businessName,omitempty"`
	BusinessLocality   string                  `json:"businessLocality,omitempty"`
	SampleProducts     []string                `json:"sampleProducts,omitempty"`
	DemoBusinesses     []scenario.DemoBusiness `json:"demoBusinesses,omitempty"`
	SquareConfigured   bool                    `json:"squareConfigured"`
	ScenarioReady      bool                    `json:"scenarioReady"`
	QlooConfigured     bool                    `json:"qlooConfigured"`
	LLMConfigured      bool                    `json:"llmConfigured"`
	LLMProvider        string                  `json:"llmProvider"`
	LLMModel           string                  `json:"llmModel"`
	ShowMatchChain     bool                    `json:"showMatchChain"`
	Missing            []string                `json:"missing"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	storeName := "memory"
	if s.postgresInUse {
		storeName = "postgres"
	}
	writeJSON(w, http.StatusOK, healthResponse{
		Status:       "ok",
		Service:      "customer-acquisition-advisor",
		DBConfigured: s.postgresInUse,
		Store:        storeName,
	})
}

func (s *server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	missing := s.cfg.missingCredentials()
	if missing == nil {
		missing = []string{}
	}
	scenarioName := strings.TrimSpace(r.URL.Query().Get("scenario"))
	if scenarioName == "" {
		scenarioName = s.cfg.scenarioName
	}
	if scenarioName != "" && !scenario.ValidName(scenarioName) {
		scenarioName = s.cfg.scenarioName
	}
	bizName, bizLocality := s.cfg.demoBusinessDisplayFor(scenarioName)
	products := s.cfg.demoSampleProductsFor(scenarioName)
	if products == nil {
		products = []string{}
	}
	demos := scenario.DemoBusinesses()
	writeJSON(w, http.StatusOK, configResponse{
		BusinessDataSource: s.cfg.businessDataSource(),
		Scenario:           scenarioName,
		BusinessName:       bizName,
		BusinessLocality:   bizLocality,
		SampleProducts:     products,
		DemoBusinesses:     demos,
		SquareConfigured:   s.cfg.useLiveConnector() && s.cfg.squareKey != "",
		ScenarioReady:      s.cfg.scenarioDataReady(),
		QlooConfigured:     s.cfg.qlooKey != "",
		LLMConfigured:      s.cfg.llmConfigured(),
		LLMProvider:        s.cfg.llmProvider,
		LLMModel:           s.cfg.llmModel,
		ShowMatchChain:     s.cfg.matchChainUIEnabled,
		Missing:            missing,
	})
}

type startRunRequest struct {
	Scenario string `json:"scenario"`
}

// handleStartRun starts one analysis run. Missing credentials fail visibly
// with HTTP 503 naming what is missing. The run executes in the background
// so the UI can poll its progress. Multiple concurrent runs are allowed
// (each client polls its own run id).
func (s *server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if missing := s.cfg.missingCredentials(); len(missing) > 0 {
		writeAPIError(w, http.StatusServiceUnavailable, "required server-side configuration is missing: "+joinNames(missing))
		return
	}

	var req startRunRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	runCfg := s.cfg
	scenarioName := strings.TrimSpace(req.Scenario)
	if scenarioName != "" {
		if !scenario.ValidName(scenarioName) {
			writeAPIError(w, http.StatusBadRequest, "unknown scenario (choose one of the built-in demo businesses)")
			return
		}
		runCfg = s.cfg.forScenarioRun(scenarioName)
	} else if !runCfg.useLiveConnector() {
		scenarioName = runCfg.scenarioName
		if !scenario.ValidName(scenarioName) {
			writeAPIError(w, http.StatusBadRequest, "unknown scenario (choose one of the built-in demo businesses)")
			return
		}
		runCfg.scenarioName = scenarioName
	}

	connector, err := newBusinessConnector(runCfg)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	orchestrator := newOrchestrator(s.store, connector, newQlooClient(s.cfg), newLLMClient(s.cfg), s.cfg)

	runIDReady := make(chan string, 1)
	go func() {
		run, err := orchestrator.Run(context.Background(), runIDReady)
		if err != nil {
			log.Printf("analysis run failed at stage %q: %v", runStage(run), err)
		}
	}()

	select {
	case id := <-runIDReady:
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": RunStatusRunning})
	case <-time.After(10 * time.Second):
		writeAPIError(w, http.StatusInternalServerError, "analysis run could not be started")
	}
}

func runStage(run *Run) string {
	if run == nil {
		return ""
	}
	return run.Stage
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

func (s *server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	var run *Run
	var err error
	if id == "" || id == "latest" {
		run, err = s.store.LatestRun(r.Context())
	} else {
		run, err = s.store.GetRun(r.Context(), id)
	}
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

type feedbackRequest struct {
	OpportunityIndex int     `json:"opportunityIndex"`
	Worked           bool    `json:"worked"`
	Outcome          string  `json:"outcome"`
	RemindAt         *string `json:"remindAt"`
}

func (s *server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "feedback body could not be parsed")
		return
	}
	fb := Feedback{OpportunityIndex: req.OpportunityIndex, Worked: req.Worked, Outcome: req.Outcome}
	if req.RemindAt != nil && *req.RemindAt != "" {
		t, err := time.Parse(time.RFC3339, *req.RemindAt)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "remindAt must be an RFC 3339 timestamp")
			return
		}
		fb.RemindAt = &t
	}
	if err := RecordFeedback(r.Context(), s.store, r.PathValue("id"), fb, time.Now()); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func main() {
	loadDotEnvFiles()
	cfg := loadConfig()
	ctx := context.Background()
	store, postgresInUse, err := openRunStore(ctx, cfg.databaseURL)
	if err != nil {
		log.Fatalf("run store could not be opened: %v", err)
	}
	if err := store.ResetStaleRuns(ctx); err != nil {
		log.Printf("stale runs could not be reset: %v", err)
	}
	srv := &server{
		cfg:           cfg,
		store:         store,
		postgresInUse: postgresInUse,
	}
	connector, err := newBusinessConnector(cfg)
	if err != nil {
		log.Fatalf("business data connector: %v", err)
	}
	srv.orchestrator = newOrchestrator(store, connector, newQlooClient(cfg), newLLMClient(cfg), cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/api/config", srv.handleConfig)
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			srv.handleStartRun(w, r)
			return
		}
		if r.Method == http.MethodGet {
			srv.handleGetRun(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/runs/{id}", srv.handleGetRun)
	mux.HandleFunc("/api/runs/{id}/feedback", srv.handleFeedback)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/", serveFrontend)

	addr := ":" + cfg.port
	log.Printf("listening on %s (database configured: %v)", addr, postgresInUse)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// serveFrontend serves the built React app from ./static, falling back
// to the single-page entry point for client-side routes. When the
// frontend has not been built, it reports that plainly instead of
// serving a misleading empty page.
func serveFrontend(w http.ResponseWriter, r *http.Request) {
	staticDir := "static"
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.Error(w, "frontend not built; run `npm run build` in src/web", http.StatusServiceUnavailable)
		return
	}
	path := filepath.Join(staticDir, filepath.Clean("/"+r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, indexPath)
}
