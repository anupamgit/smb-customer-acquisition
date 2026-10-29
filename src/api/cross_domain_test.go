package main

import (
	"strings"
	"testing"
)

func TestSameCategoryDoesNotMatchHairInsideWheelchair(t *testing.T) {
	c := QlooCandidate{
		Name:      "Century Apartments",
		TagLabels: []string{"Wheelchair accessible parking lot", "Student housing center"},
	}
	if isSameCategoryAsOwner(c, "hair_salon") {
		t.Fatal("wheelchair tag must not count as hair salon")
	}
	c2 := QlooCandidate{Name: "Sunset Hair Studio", TagLabels: []string{"Hair salon"}}
	if !isSameCategoryAsOwner(c2, "hair_salon") {
		t.Fatal("expected real salon to match")
	}
}

func TestFilterCrossCategoryPartnersMexican(t *testing.T) {
	cands := []QlooCandidate{
		{ID: "1", Name: "Torchy's Tacos", Affinity: 0.95},
		{ID: "2", Name: "Gregory Gym", Affinity: 0.82, Properties: []byte(`{"keywords":[{"name":"campus"}]}`)},
		{ID: "3", Name: "Planet Fitness", Affinity: 0.80, Properties: []byte(`{"keywords":[{"name":"gym"}]}`)},
	}
	out := filterCrossCategoryPartners(cands, "mexican_restaurant")
	if len(out) < 2 {
		t.Fatalf("got %v", out)
	}
	for _, c := range out {
		if strings.Contains(strings.ToLower(c.Name), "taco") {
			t.Fatalf("same-category place kept: %s", c.Name)
		}
	}
}
