package main

import "testing"

func TestCrossDomainCategoryLabelsPreferEntityTags(t *testing.T) {
	cands := []QlooCandidate{
		{ID: "1", Name: "Place A", TagLabels: []string{"Bookstore"}},
		{ID: "2", Name: "Place B", TagLabels: []string{"Gym"}},
	}
	labels := crossDomainCategoryLabels(cands, 5)
	if len(labels) != 2 || labels[0] != "bookstore" || labels[1] != "gym" {
		t.Fatalf("labels = %v", labels)
	}
}
