package main

import "testing"

func TestHaversineMilesNYCApprox(t *testing.T) {
	// Midtown to Brooklyn ~5–6 mi straight line
	mi := haversineMiles(40.7589, -73.9851, 40.6782, -73.9442)
	if mi < 4 || mi > 8 {
		t.Fatalf("distance = %.2f mi, expected roughly 5–6", mi)
	}
}
