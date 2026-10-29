package main

import "sort"

// compareQlooMatch orders best Insights matches: higher affinity, then popularity, then name.
func compareQlooMatch(a, b QlooCandidate) bool {
	if a.Affinity != b.Affinity {
		return a.Affinity > b.Affinity
	}
	if a.Popularity != b.Popularity {
		return a.Popularity > b.Popularity
	}
	return a.Name < b.Name
}

func sortQlooCandidatesByMatch(cands []QlooCandidate) {
	sort.Slice(cands, func(i, j int) bool {
		return compareQlooMatch(cands[i], cands[j])
	})
}

func qlooScoresComplete(c QlooCandidate) bool {
	return c.scoreComplete
}

func filterQlooWithCompleteScores(cands []QlooCandidate) []QlooCandidate {
	out := make([]QlooCandidate, 0, len(cands))
	for _, c := range cands {
		if qlooScoresComplete(c) {
			out = append(out, c)
		}
	}
	return out
}
