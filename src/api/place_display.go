package main

import (
	"encoding/json"
	"strings"
)

// PlaceContact is Qloo-sourced contact info for the UI contact card.
type PlaceContact struct {
	Address string `json:"address,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Website string `json:"website,omitempty"`
}

func (c PlaceContact) any() bool {
	return c.Address != "" || c.Phone != "" || c.Website != ""
}

type qlooPlaceProperties struct {
	Address          string   `json:"address"`
	Phone            string   `json:"phone"`
	Website          string   `json:"website"`
	BusinessCategory string   `json:"business_category"`
	Category         string   `json:"category"`
	Types            []string `json:"types"`
	Tags             []string `json:"tags"`
}

func parseQlooPlaceProperties(raw json.RawMessage) qlooPlaceProperties {
	if len(raw) == 0 {
		return qlooPlaceProperties{}
	}
	var p qlooPlaceProperties
	_ = json.Unmarshal(raw, &p)
	return p
}

func placeContactFromCandidate(c QlooCandidate) *PlaceContact {
	p := parseQlooPlaceProperties(c.Properties)
	out := PlaceContact{
		Address: strings.TrimSpace(p.Address),
		Phone:   strings.TrimSpace(p.Phone),
		Website: strings.TrimSpace(p.Website),
	}
	if !out.any() {
		return nil
	}
	return &out
}

// qlooBusinessKind is a short label from Qloo properties only (never geocode city).
func qlooBusinessKind(c QlooCandidate) string {
	p := parseQlooPlaceProperties(c.Properties)
	for _, s := range []string{p.BusinessCategory, p.Category} {
		if t := shortPlaceType(s); t != "" {
			return t
		}
	}
	return ""
}

func opportunityPlaceType(d AgentDraft, c QlooCandidate) string {
	if t := shortPlaceType(d.PlaceKind); t != "" {
		return t
	}
	return qlooBusinessKind(c)
}

func shortPlaceType(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "urn:") {
		return ""
	}
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	if len(s) > 48 {
		s = s[:48]
	}
	return strings.TrimSpace(s)
}
