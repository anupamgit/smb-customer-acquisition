package main

import (
	"encoding/json"
	"strings"
	"unicode"
)

var ownerNameKeywords = map[string][]string{
	"coffee_shop":        {"coffee", "cafe", "café", "espresso", "latte", "roaster", "starbucks"},
	"hair_salon":         {"salon", "hair", "barber", "beauty", "nail", "barbershop"},
	"mexican_restaurant": {"mexican", "taco", "taqueria", "burrito", "cantina", "tex-mex", "tex mex", "torchy"},
}

func partnerCategoryBucket(c QlooCandidate) string {
	labels := candidateCategoryLabels(c)
	if len(labels) > 0 {
		return strings.ToLower(labels[0])
	}
	return strings.ToLower(strings.TrimSpace(c.Type))
}

// candidateCategoryLabels prefers Qloo entity tag names, then properties.keywords.
func candidateCategoryLabels(c QlooCandidate) []string {
	if len(c.TagLabels) > 0 {
		return append([]string(nil), c.TagLabels...)
	}
	return candidatePropertyKeywords(c)
}

func candidatePropertyKeywords(c QlooCandidate) []string {
	if len(c.Properties) == 0 {
		return nil
	}
	var props struct {
		Keywords []struct {
			Name string `json:"name"`
		} `json:"keywords"`
	}
	if err := json.Unmarshal(c.Properties, &props); err != nil {
		return nil
	}
	var out []string
	for _, k := range props.Keywords {
		if k.Name != "" {
			out = append(out, k.Name)
		}
	}
	return out
}

func isSameCategoryAsOwner(c QlooCandidate, businessType string) bool {
	keywords := ownerNameKeywords[businessType]
	if len(keywords) == 0 {
		return false
	}
	hay := strings.ToLower(c.Name + " " + c.Type + " " + strings.Join(candidateCategoryLabels(c), " "))
	for _, kw := range keywords {
		if ownerKeywordMatches(hay, kw) {
			return true
		}
	}
	return false
}

// ownerKeywordMatches avoids substring false positives (e.g. "hair" inside "wheelchair").
func ownerKeywordMatches(hay, kw string) bool {
	kw = strings.ToLower(strings.TrimSpace(kw))
	if kw == "" {
		return false
	}
	if strings.Contains(kw, " ") {
		return strings.Contains(hay, kw)
	}
	for _, tok := range tokenizeOwnerMatchText(hay) {
		if ownerTokenMatchesKeyword(tok, kw) {
			return true
		}
	}
	return false
}

func ownerTokenMatchesKeyword(tok, kw string) bool {
	if tok == kw {
		return true
	}
	// Simple plural: "tacos" matches keyword "taco".
	if strings.HasPrefix(tok, kw) && len(tok) == len(kw)+1 && strings.HasSuffix(tok, "s") {
		return true
	}
	return false
}

func tokenizeOwnerMatchText(hay string) []string {
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
	}
	for _, r := range hay {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\'' {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// crossDomainCategoryLabels returns short labels for the top diverse partner buckets (for UI).
func crossDomainCategoryLabels(cands []QlooCandidate, limit int) []string {
	var labels []string
	seen := map[string]bool{}
	for _, c := range cands {
		b := partnerCategoryBucket(c)
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		labels = append(labels, humanizeBucket(b))
		if len(labels) >= limit {
			break
		}
	}
	return labels
}

func humanizeBucket(b string) string {
	return strings.ReplaceAll(strings.TrimSpace(b), "_", " ")
}
