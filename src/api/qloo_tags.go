package main

import "strings"

// qlooBrokenInterestTagURN maps tag IDs that crash hackathon Insights to a safe twin.
var qlooBrokenInterestTagURN = map[string]string{
	"urn:tag:offerings:place:kids'_haircuts": "urn:tag:offerings:place:kids_haircuts",
}

// menuLineTagURNOverrides pin demo menu lines to Insights-safe URNs. Qloo semantic
// search often returns kids'_haircuts (500 on Insights) or shave for "Haircut".
var menuLineTagURNOverrides = map[string]string{
	"Haircut":                "urn:tag:offerings:place:kids_haircuts",
	"Trim":                   "urn:tag:offerings:place:kids_haircuts",
	"Hair Color":             "urn:tag:offerings:place:hair_coloring",
	"Blowout":                "urn:tag:offerings:place:blowouts",
	"Conditioning Treatment": "urn:tag:offerings:place:hair_coloring",
}

func menuLineTagOverride(name string) (string, bool) {
	urn, ok := menuLineTagURNOverrides[strings.TrimSpace(name)]
	if !ok {
		return "", false
	}
	return normalizeInterestTagURN(urn), true
}

// normalizeInterestTagURN returns a tag ID safe to send on signal.interests.tags.
func normalizeInterestTagURN(id string) string {
	id = strings.TrimSpace(id)
	if alt, ok := qlooBrokenInterestTagURN[id]; ok {
		return alt
	}
	if strings.Contains(id, "'") {
		id = strings.ReplaceAll(id, "'", "")
	}
	return id
}

// pickMenuItemTagURN chooses a tag from semantic search results, avoiding known-broken URNs.
func pickMenuItemTagURN(rows []qlooTagRow) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		n := normalizeInterestTagURN(r.ID)
		if seen[n] {
			continue
		}
		seen[n] = true
		if _, broken := qlooBrokenInterestTagURN[r.ID]; broken || strings.Contains(r.ID, "'") {
			continue
		}
		return n, true
	}
	for _, r := range rows {
		if r.ID != "" {
			return normalizeInterestTagURN(r.ID), true
		}
	}
	return "", false
}

func normalizeInterestTagList(tagURNs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range tagURNs {
		n := normalizeInterestTagURN(u)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
