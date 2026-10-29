package main

import "testing"

func TestParseQlooInsightsResponse_HackathonShape(t *testing.T) {
	const sample = `{"success":true,"results":{"entities":[
		{"name":"A","entity_id":"1","subtype":"urn:entity:place","popularity":0.2,"tags":[{"name":"Bookstore"}],"query":{"affinity":0.76}},
		{"name":"B","entity_id":"2","subtype":"urn:entity:place","popularity":0.9,"query":{"affinity":0.76}},
		{"name":"C","entity_id":"3","subtype":"urn:entity:place","query":{"affinity":0.5}}
	]}}`
	rows, ok, err := parseQlooInsightsResponse([]byte(sample))
	if err != nil || !ok || len(rows) != 3 {
		t.Fatalf("rows=%d ok=%v err=%v", len(rows), ok, err)
	}
	if !rows[0].ScoreComplete || rows[0].Popularity != 0.2 || rows[0].Affinity != 0.76 || len(rows[0].TagLabels) != 1 || rows[0].TagLabels[0] != "Bookstore" {
		t.Fatalf("A = %+v", rows[0])
	}
	if rows[2].ScoreComplete {
		t.Fatalf("C missing popularity must be incomplete, got %+v", rows[2])
	}
	cands := []QlooCandidate{
		{Name: "A", Affinity: 0.76, Popularity: 0.2, scoreComplete: true},
		{Name: "B", Affinity: 0.76, Popularity: 0.9, scoreComplete: true},
	}
	sortQlooCandidatesByMatch(cands)
	if cands[0].Name != "B" {
		t.Fatalf("sort = %+v, want B first (same affinity, higher popularity)", cands)
	}
}
