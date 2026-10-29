package main

import "fmt"

// filterCandidatesWithinRadiusMeters drops partners farther than radius from the shop (straight-line).
func filterCandidatesWithinRadiusMeters(signal *QlooSignal, cands []QlooCandidate, radiusMeters int) (kept []QlooCandidate, dropped []partnerFilterDrop) {
	if radiusMeters <= 0 || signal.Latitude == nil || signal.Longitude == nil {
		return cands, nil
	}
	maxMi := float64(radiusMeters) / 1609.34
	for _, c := range cands {
		if c.PlaceLatitude == nil || c.PlaceLongitude == nil {
			kept = append(kept, c)
			continue
		}
		mi := haversineMiles(*signal.Latitude, *signal.Longitude, *c.PlaceLatitude, *c.PlaceLongitude)
		if mi <= maxMi*1.02 {
			kept = append(kept, c)
			continue
		}
		dropped = append(dropped, partnerFilterDrop{
			Candidate: c,
			Reason:    fmt.Sprintf("beyond partner radius (~%.1f mi straight-line)", maxMi),
		})
	}
	return kept, dropped
}
