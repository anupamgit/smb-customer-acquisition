package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flow "github.com/Azure/go-workflow"
)

type runPipeline struct {
	o      *Orchestrator
	run    *Run
	saveCtx context.Context
	timer  stageTimer
	audit  *pipelineAuditor

	source       *SquareSourceData
	lat, lon     *float64
	signal       *QlooSignal
	tagURNs      []string
	candidates   []QlooCandidate
	ranked              []AgentDraft
	cachedOpportunities []Opportunity
	businessType        string
}

func (p *runPipeline) save() error {
	p.run.UpdatedAt = p.o.now().UTC()
	return p.o.store.UpdateRun(p.saveCtx, p.run)
}

func (p *runPipeline) fail(stage string, err error) error {
	p.run.Status = RunStatusFailed
	p.run.Stage = stage
	p.run.Error = err.Error()
	p.run.UpdatedAt = p.o.now().UTC()
	_ = p.o.store.UpdateRun(p.saveCtx, p.run)
	return err
}

type wfIngest struct{ p *runPipeline }

func (s *wfIngest) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "ingest_normalize"
	if err := p.save(); err != nil {
		return err
	}
	source, err := p.o.connector.FetchSourceData(ctx)
	if err != nil {
		return p.fail("ingest_normalize", fmt.Errorf("connector read failed: %w", err))
	}
	p.source = source
	canonical, unattributed := NormalizeSquare(source)
	p.run.Canonical = canonical
	p.run.UnattributedOrders = unattributed
	if err := p.save(); err != nil {
		return err
	}
	if len(canonical) == 0 {
		return p.fail("ingest_normalize", errors.New("no customer records returned by the connector"))
	}
	p.audit.section("Ingest",
		fmt.Sprintf("Customers in canonical model: %d", len(canonical)),
		fmt.Sprintf("Unattributed orders: %d", unattributed),
	)
	p.timer.logStage("ingest_normalize")
	return nil
}

type wfScore struct{ p *runPipeline }

func (s *wfScore) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "score_cohort"
	cohort := ScoreCohort(p.run.Canonical, p.o.now())
	p.run.Cohort = cohort
	if err := p.save(); err != nil {
		return err
	}
	if len(cohort) == 0 {
		return p.fail("score_cohort", errors.New("no valuable-customer cohort could be scored from the business data"))
	}
	p.audit.section("Valuable customers",
		fmt.Sprintf("Cohort size: %d", len(cohort)),
	)
	p.timer.logStage("score_cohort")
	return nil
}

type wfBuildSignals struct{ p *runPipeline }

func (s *wfBuildSignals) Do(ctx context.Context) error {
	p := s.p
	p.businessType = businessTypeForRun(p.o.connector, p.o.cfg)
	var shopLoc *SquareLocation
	for i := range p.source.Locations {
		loc := &p.source.Locations[i]
		if shopLoc == nil {
			shopLoc = loc
		}
		if loc.Coordinates != nil {
			p.lat, p.lon = &loc.Coordinates.Latitude, &loc.Coordinates.Longitude
			shopLoc = loc
			break
		}
	}
	p.run.Stage = "build_signals"
	if err := p.save(); err != nil {
		return err
	}
	signal := BuildSignals(p.run.Cohort, p.lat, p.lon)
	if shopLoc != nil {
		applyShopLocationFallback(signal, shopLoc)
	}
	p.run.Signal = signal
	p.signal = signal
	if signal.LocalityQuery == "" && p.lat == nil {
		return p.fail("build_signals", errors.New("no usable geographic signal in the valuable-customer cohort"))
	}
	var tagURNs []string
	for _, name := range signal.BoughtNames {
		urn, ok, err := p.o.qloo.ResolveTag(ctx, name)
		if err != nil {
			return p.fail("build_signals", fmt.Errorf("qloo tag resolution failed: %w", err))
		}
		p.run.Resolutions = append(p.run.Resolutions, ResolutionAttempt{InputName: name, Resolved: ok, TagURN: urn})
		if ok {
			tagURNs = append(tagURNs, normalizeInterestTagURN(urn))
		}
	}
	p.tagURNs = tagURNs
	signal.GeographyOnly = len(tagURNs) == 0
	if err := p.save(); err != nil {
		return err
	}
	locLines := []string{
		fmt.Sprintf("Location query: %s", emptyDash(signal.LocalityQuery)),
		fmt.Sprintf("Coordinates sent: %s", coordSummary(p.lat, p.lon)),
		fmt.Sprintf("Geography-only mode: %v", signal.GeographyOnly),
		fmt.Sprintf("Distinct menu lines: %d", len(signal.BoughtNames)),
	}
	p.audit.section("Signals sent to Qloo", locLines...)
	var tagLines []string
	for _, r := range p.run.Resolutions {
		if r.Resolved {
			tagLines = append(tagLines, fmt.Sprintf("`%s` → `%s`", r.InputName, r.TagURN))
		} else {
			tagLines = append(tagLines, fmt.Sprintf("`%s` → (no tag)", r.InputName))
		}
	}
	if len(tagLines) == 0 {
		tagLines = []string{"(none)"}
	}
	p.audit.section("Tag resolution (menu → Qloo tag ID)", tagLines...)
	p.timer.logStage("build_signals")
	return nil
}

type wfDiscover struct{ p *runPipeline }

func (s *wfDiscover) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "discover_candidates"
	if err := p.save(); err != nil {
		return err
	}
	p.businessType = businessTypeForRun(p.o.connector, p.o.cfg)
	exclude := excludeTagsForBusinessType(p.businessType)
	take := p.o.cfg.effectiveQlooInsightsTake()
	radiusM := p.o.cfg.effectiveQlooLocationRadiusMeters()
	insightsRadiusM := partnerInsightsLocationRadiusMeters(radiusM)
	p.audit.section("Insights request (partner discovery)",
		fmt.Sprintf("Interest tag IDs (%d): %s", len(p.tagURNs), stringsJoinBacktick(normalizeInterestTagList(p.tagURNs))),
		fmt.Sprintf("Exclude tag IDs (%d): %s", len(exclude), stringsJoinBacktick(exclude)),
		fmt.Sprintf("take: %d", take),
		fmt.Sprintf("filter.location: POINT(lon lat) with filter.location.radius=%d m (Insights fetch)", insightsRadiusM),
		fmt.Sprintf("Partner shortlist radius: %d m (may widen to Insights fetch disk if in-radius places are all same-category)", radiusM),
	)
	candidates, rawFromQloo, err := discoverCandidates(ctx, p.o.qloo, p.signal, p.tagURNs, p.businessType, take, radiusM)
	if err != nil {
		if len(rawFromQloo) > 0 {
			rawSec, dropSec, shortSec := insightsAuditSections(rawFromQloo, p.businessType, candidates, p.signal, radiusM)
			p.audit.section("Insights response (raw from Qloo)", rawSec...)
			if len(dropSec) > 0 {
				p.audit.section("Insights pipeline (not sent to LLM)", dropSec...)
			}
			if len(shortSec) > 0 {
				p.audit.section("Insights shortlist (sent to LLM)", shortSec...)
			}
		}
		return p.fail("discover_candidates", fmt.Errorf("qloo discovery failed: %w", err))
	}
	p.candidates = candidates
	p.run.QlooEvidence = candidates
	p.run.CrossDomainCategories = crossDomainCategoryLabels(candidates, 5)
	if err := p.save(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return p.fail("discover_candidates", errors.New("qloo returned no usable candidate places for the derived signals"))
	}
	rawSec, dropSec, shortSec := insightsAuditSections(rawFromQloo, p.businessType, candidates, p.signal, radiusM)
	p.audit.section("Insights response (raw from Qloo)", rawSec...)
	if len(dropSec) > 0 {
		p.audit.section("Insights pipeline (not sent to LLM)", dropSec...)
	} else {
		p.audit.section("Insights pipeline (not sent to LLM)", "(all ranked places continued)")
	}
	p.audit.section("Insights shortlist (sent to LLM)", shortSec...)
	p.audit.section("Distances (audit debug)", partnerDistanceAuditLines(p.signal.Latitude, p.signal.Longitude, candidates)...)
	p.timer.logStage("discover_candidates")
	return nil
}

type wfEvaluate struct{ p *runPipeline }

func (s *wfEvaluate) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "evaluate_opportunities"
	if err := p.save(); err != nil {
		return err
	}
	if key := scenarioCacheKey(p.o.connector, p.o.cfg); key != "" {
		if ent, ok := p.o.llmCache.get(key, p.o.now()); ok && ent.usable() && p.o.cfg.effectiveLLMCacheTTL() > 0 {
			p.run.AgentOutput = cloneAgentDrafts(ent.agentOutput)
			p.ranked = cloneAgentDrafts(ent.ranked)
			p.cachedOpportunities = cloneOpportunities(ent.opportunities)
			p.audit.section("LLM input", fmt.Sprintf("(skipped — using cached outcome for scenario `%s`)", key))
			p.audit.section("LLM output (opportunities)", "(cached — OpenRouter not called)")
			p.timer.logStage("evaluate_opportunities")
			return nil
		}
	}
	llmPayload := BuildLLMPayload(p.signal, p.run.Cohort, p.tagURNs, p.candidates)
	drafts, err := p.o.llm.Complete(ctx, llmPayload)
	if err != nil {
		return p.fail("evaluate_opportunities", fmt.Errorf("agent reasoning failed: %w", err))
	}
	drafts, err = validateAgentDrafts(drafts, p.candidates)
	if err != nil {
		return p.fail("evaluate_opportunities", fmt.Errorf("agent reasoning failed: %w", err))
	}
	inLLM, outLLM := llmAuditLines(llmPayload, drafts)
	p.audit.section("LLM input", inLLM...)
	p.audit.section("LLM output (opportunities)", outLLM...)
	p.run.AgentOutput = drafts
	if err := p.save(); err != nil {
		return err
	}
	p.timer.logStage("evaluate_opportunities")
	return nil
}

type wfRank struct{ p *runPipeline }

func (s *wfRank) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "rank_actions"
	if err := p.save(); err != nil {
		return err
	}
	if len(p.ranked) > 0 {
		p.audit.section("Final opportunity rank (combined score)", "(cached with LLM output)")
		p.timer.logStage("rank_actions")
		return nil
	}
	p.ranked = RankOpportunities(p.run.AgentOutput, p.candidates, p.signal)
	if len(p.ranked) == 0 {
		return p.fail("rank_actions", errors.New("no opportunity survived ranking"))
	}
	p.audit.section("Final opportunity rank (combined score)", finalRankAuditLines(p.ranked, p.candidates, p.signal)...)
	p.timer.logStage("rank_actions")
	return nil
}

type wfExplain struct{ p *runPipeline }

func (s *wfExplain) Do(ctx context.Context) error {
	p := s.p
	p.run.Stage = "explain"
	if len(p.cachedOpportunities) > 0 {
		p.run.Opportunities = cloneOpportunities(p.cachedOpportunities)
	} else {
		p.run.Opportunities = BuildOpportunities(p.ranked, p.candidates, p.signal, p.run.Cohort)
	}
	p.run.Status = RunStatusSucceeded
	p.run.Stage = "complete"
	if err := p.save(); err != nil {
		return err
	}
	if key := scenarioCacheKey(p.o.connector, p.o.cfg); key != "" && len(p.cachedOpportunities) == 0 {
		if ttl := p.o.cfg.effectiveLLMCacheTTL(); ttl > 0 {
			p.o.llmCache.putCompleted(key, p.run.AgentOutput, p.ranked, p.run.Opportunities, ttl, p.o.now())
		}
	}
	p.audit.section("Final recommendations",
		fmt.Sprintf("Opportunities returned: %d", len(p.run.Opportunities)),
	)
	p.timer.logStage("explain")
	return nil
}

func (o *Orchestrator) runWorkflow(ctx context.Context, run *Run) error {
	audit := newPipelineAuditor(o.cfg, run.ID)
	p := &runPipeline{o: o, run: run, saveCtx: ctx, timer: newStageTimer(run.ID), audit: audit}
	defer audit.writeRun(run, businessTypeForAudit(o.connector, o.cfg), scenarioNameForConnector(o.connector))
	p.timer.logStage("starting")

	w := new(flow.Workflow)
	w.MaxConcurrency = 1
	w.Add(
		flow.Pipe(
			&wfIngest{p: p},
			&wfScore{p: p},
			&wfBuildSignals{p: p},
			&wfDiscover{p: p},
			&wfEvaluate{p: p},
			&wfRank{p: p},
			&wfExplain{p: p},
		),
	)
	err := w.Do(ctx)
	p.timer.logTotal()
	return err
}

func coordSummary(lat, lon *float64) string {
	if lat == nil || lon == nil {
		return "no"
	}
	return fmt.Sprintf("%.5f, %.5f", *lat, *lon)
}

func stringsJoinBacktick(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	var out []string
	for _, s := range items {
		out = append(out, "`"+s+"`")
	}
	return strings.Join(out, ", ")
}

