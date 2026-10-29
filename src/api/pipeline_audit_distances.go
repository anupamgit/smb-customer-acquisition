package main

import (
	"fmt"
	"math"
)

// partnerDistanceAuditLines lists straight-line distance shop → each partner (audit only).
func partnerDistanceAuditLines(shopLat, shopLon *float64, partners []QlooCandidate) []string {
	if shopLat == nil || shopLon == nil {
		return []string{"Shop coordinates unavailable; distances not computed."}
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("From shop at %.5f, %.5f (straight-line miles)", *shopLat, *shopLon))
	for i, c := range partners {
		if c.PlaceLatitude == nil || c.PlaceLongitude == nil {
			lines = append(lines, fmt.Sprintf("#%d %s — distance unknown (no place coordinates in Qloo response)", i+1, c.Name))
			continue
		}
		mi := haversineMiles(*shopLat, *shopLon, *c.PlaceLatitude, *c.PlaceLongitude)
		lines = append(lines, fmt.Sprintf("#%d %s — %.1f mi (%.2f km)", i+1, c.Name, mi, mi*1.60934))
	}
	return lines
}

func haversineMiles(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMi = 3958.8
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMi * c
}
