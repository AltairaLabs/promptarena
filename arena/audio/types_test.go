package audio

import "testing"

func TestIsValidRate(t *testing.T) {
	for _, r := range ValidRates {
		if !IsValidRate(r) {
			t.Fatalf("expected %d to be valid", r)
		}
	}
	for _, r := range []int{0, 8000, 22050, 32000, 44100, 100000} {
		if IsValidRate(r) {
			t.Fatalf("expected %d to be invalid", r)
		}
	}
}
