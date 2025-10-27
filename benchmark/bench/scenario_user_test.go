package bench

import (
	"testing"
)

func TestGenerateRandomItinerary(t *testing.T) {
	// Uncomment the line below to make the test deterministic
	// rand.Seed(42)

	for i := 0; i < 10; i++ {
		itinerary := generateRandomItinerary()

		// Test 1: Check the number of stations is between 2 and 5
		if len(itinerary.Stations) < 2 || len(itinerary.Stations) > 5 {
			t.Errorf("Expected number of stations between 2 and 5, got %d", len(itinerary.Stations))
		}

		// Test 2: Check no consecutive stations are the same
		for i := 1; i < len(itinerary.Stations); i++ {
			if itinerary.Stations[i] == itinerary.Stations[i-1] {
				t.Errorf("Consecutive stations are the same at positions %d and %d: %s", i-1, i, itinerary.Stations[i])
			}
		}
	}
}
