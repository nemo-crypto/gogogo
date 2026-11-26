package marketdata

import "testing"

func TestIntervalDurationRejectsOverflow(t *testing.T) {
	for _, interval := range []string{"153722868m", "2562048h", "106752d", "15251w", "4294967296m"} {
		t.Run(interval, func(t *testing.T) {
			if got, err := IntervalDuration(interval); err == nil {
				t.Fatalf("IntervalDuration(%q) = %v without error; oversized intervals must not wrap", interval, got)
			}
		})
	}
}
