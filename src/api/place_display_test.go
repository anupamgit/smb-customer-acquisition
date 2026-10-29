package main

import "testing"

func TestPlaceContactFromCandidate(t *testing.T) {
	c := QlooCandidate{
		Properties: []byte(`{"address":"1 Main St","phone":"+15551234567","website":"https://example.com"}`),
	}
	contact := placeContactFromCandidate(c)
	if contact == nil || contact.Address != "1 Main St" || contact.Phone != "+15551234567" {
		t.Fatalf("contact = %+v", contact)
	}
}

func TestQlooBusinessKindIgnoresCity(t *testing.T) {
	c := QlooCandidate{City: "Victor", Properties: []byte(`{"business_category":"Indian restaurant"}`)}
	if got := qlooBusinessKind(c); got != "Indian restaurant" {
		t.Fatalf("got %q", got)
	}
	c2 := QlooCandidate{City: "Sweden"}
	if got := qlooBusinessKind(c2); got != "" {
		t.Fatalf("city must not be used as kind, got %q", got)
	}
}

func TestOpportunityPlaceTypePrefersLLM(t *testing.T) {
	d := AgentDraft{PlaceKind: "Sandwich shop"}
	c := QlooCandidate{City: "Sweden", Properties: []byte(`{"business_category":"Fast food"}`)}
	if got := opportunityPlaceType(d, c); got != "Sandwich shop" {
		t.Fatalf("got %q", got)
	}
}
