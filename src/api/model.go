package main

import (
	"encoding/json"
	"time"
)

// Evidence origin tags. Every stored and displayed evidence item carries
// one of these, so Qloo evidence is never confused with agent inference
// (spec section 11; Step 3 arc42 section 8.2).
const (
	OriginBusiness = "business_data"
	OriginQloo     = "qloo_evidence"
	OriginAgent    = "agent_inference"
)

// CanonicalRecord is one customer in the canonical model (spec section 5),
// aggregated from that customer's transactions. Missing fields stay empty
// and are listed in MissingFields; they are never filled with invented
// values.
type CanonicalRecord struct {
	CustomerID    string     `json:"customerId"`
	Locality      string     `json:"locality,omitempty"`
	AdminRegion   string     `json:"adminRegion,omitempty"`
	PostalCode    string     `json:"postalCode,omitempty"`
	Country       string     `json:"country,omitempty"`
	TotalMinor    int64      `json:"totalMinor"`
	Currency      string     `json:"currency,omitempty"`
	OrderCount    int        `json:"orderCount"`
	FirstOrderAt  *time.Time `json:"firstOrderAt,omitempty"`
	LastOrderAt   *time.Time `json:"lastOrderAt,omitempty"`
	ItemNames     []string   `json:"itemNames,omitempty"`
	MissingFields []string   `json:"missingFields,omitempty"`
	HasLocation   bool       `json:"hasLocation"`
}

// ScoredCustomer is a canonical record with its deterministic score.
type ScoredCustomer struct {
	CanonicalRecord
	Score float64 `json:"score"`
}

// QlooSignal is a derived, non-PII signal built from the scored cohort.
// It contains no customer identifier, name, email, or other PII.
type QlooSignal struct {
	LocalityQuery string   `json:"localityQuery"`
	City          string   `json:"city,omitempty"`
	AdminRegion   string   `json:"adminRegion,omitempty"`
	PostalCode    string   `json:"postalCode,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	BoughtNames   []string `json:"boughtNames,omitempty"`
	GeographyOnly bool     `json:"geographyOnly"`
}

// ResolutionAttempt records one bought-name resolution: what it resolved
// to, or that it was dropped. Unresolved inputs are never represented as
// resolved.
type ResolutionAttempt struct {
	InputName string `json:"inputName"`
	Resolved  bool   `json:"resolved"`
	TagURN    string `json:"tagUrn,omitempty"`
}

// QlooCandidate is one place/business/entity returned by Qloo. Properties
// and Explainability are preserved exactly as returned (raw JSON) so
// Qloo explainability is never rewritten by this service.
type QlooCandidate struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type,omitempty"`
	Affinity       float64         `json:"affinity"`
	Popularity     float64         `json:"popularity"`
	TagLabels      []string        `json:"tagLabels,omitempty"` // Insights entity tags[].name
	PlaceLatitude  *float64        `json:"placeLatitude,omitempty"`
	PlaceLongitude *float64        `json:"placeLongitude,omitempty"`
	scoreComplete  bool            // both Qloo fields present in Insights response
	City           string          `json:"city,omitempty"`
	Properties     json.RawMessage `json:"properties,omitempty"`
	Explainability json.RawMessage `json:"explainability,omitempty"`
}

// AgentDraft is one opportunity as formulated by Agent Reasoning through
// the LLM service. It is agent inference, not Qloo evidence.
// maxOpportunities is how many ranked places we keep on a run (UI shows 3 first, up to 5).
const maxOpportunities = 5

type AgentDraft struct {
	CandidateID           string  `json:"candidateId"`
	PlaceKind             string  `json:"placeKind,omitempty"`
	Action                string  `json:"action"`
	WhyFits               string  `json:"whyFits"`
	NextAction            string  `json:"nextAction"`
	BusinessCompatibility float64 `json:"businessCompatibility"`
	EstimatedEffort       string  `json:"estimatedEffort"`
}

// EvidenceItem is one displayed piece of evidence, tagged with its origin.
type EvidenceItem struct {
	Origin string          `json:"origin"`
	Label  string          `json:"label"`
	Detail string          `json:"detail"`
	Raw    json.RawMessage `json:"raw,omitempty"`
}

// Opportunity is one ranked acquisition opportunity (spec section 8).
type Opportunity struct {
	Rank        int            `json:"rank"`
	CandidateID string         `json:"candidateId,omitempty"`
	PlaceName   string         `json:"placeName,omitempty"`
	PlaceType   string         `json:"placeType,omitempty"`
	Contact     *PlaceContact  `json:"contact,omitempty"`
	Action      string         `json:"action"`
	WhyFits     string         `json:"whyFits"`
	Evidence    []EvidenceItem `json:"evidence,omitempty"`
	NextAction  string         `json:"nextAction"`
	Score       float64        `json:"score,omitempty"`
}

// Feedback records an owner's report on an attempted opportunity
// (spec section 9, optional). It never changes a result already shown.
type Feedback struct {
	OpportunityIndex int        `json:"opportunityIndex"`
	Worked           bool       `json:"worked"`
	Outcome          string     `json:"outcome,omitempty"`
	RemindAt         *time.Time `json:"remindAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// RunStatus values for one analysis run.
const (
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
)

// Run is the stored state for one analysis run. Every stage writes its
// outputs here so the result is traceable after the run (Step 3 ADR-002).
type Run struct {
	ID                 string              `json:"id"`
	Status             string              `json:"status"`
	Stage              string              `json:"stage"`
	Error              string              `json:"error,omitempty"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
	Canonical          []CanonicalRecord   `json:"canonical,omitempty"`
	UnattributedOrders int                 `json:"unattributedOrders"`
	Cohort             []ScoredCustomer    `json:"cohort,omitempty"`
	Signal             *QlooSignal         `json:"signal,omitempty"`
	Resolutions        []ResolutionAttempt `json:"resolutions,omitempty"`
	QlooEvidence            []QlooCandidate     `json:"qlooEvidence,omitempty"`
	CrossDomainCategories   []string            `json:"crossDomainCategories,omitempty"`
	AgentOutput             []AgentDraft        `json:"agentOutput,omitempty"`
	Opportunities      []Opportunity       `json:"opportunities,omitempty"`
	Feedback           []Feedback          `json:"feedback,omitempty"`
}
