package main

import "testing"

func TestNormalizeInterestTagURNKidsHaircuts(t *testing.T) {
	got := normalizeInterestTagURN("urn:tag:offerings:place:kids'_haircuts")
	want := "urn:tag:offerings:place:kids_haircuts"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMenuLineTagOverrideHaircut(t *testing.T) {
	urn, ok := menuLineTagOverride("Haircut")
	if !ok || urn != "urn:tag:offerings:place:kids_haircuts" {
		t.Fatalf("override = %q %v", urn, ok)
	}
}

func TestPickMenuItemTagURNPrefersSafeTwin(t *testing.T) {
	rows := []qlooTagRow{
		{ID: "urn:tag:offerings:place:kids'_haircuts", Name: "Kids' haircuts"},
		{ID: "urn:tag:offerings:place:kids_haircuts", Name: "Kids' haircuts"},
	}
	urn, ok := pickMenuItemTagURN(rows)
	if !ok || urn != "urn:tag:offerings:place:kids_haircuts" {
		t.Fatalf("pick = %q %v", urn, ok)
	}
}
