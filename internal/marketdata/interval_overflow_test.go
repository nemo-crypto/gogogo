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

func TestIntervalDurationPreservesLargestRepresentableUnits(t *testing.T) {
	cases := []struct {
		interval string
		value    int64
	}{
		{"153722867m", 153722867 * 60 * 1000000000},
		{"2562047h", 2562047 * 3600 * 1000000000},
		{"106751d", 106751 * 86400 * 1000000000},
		{"15250w", 15250 * 7 * 86400 * 1000000000},
		{"1m", 60 * 1000000000},
	}
	for _, tc := range cases {
		t.Run(tc.interval, func(t *testing.T) {
			got, err := IntervalDuration(tc.interval)
			if err != nil || int64(got) != tc.value {
				t.Fatalf("IntervalDuration(%q) = %v, %v; want %v ns", tc.interval, got, err, tc.value)
			}
		})
	}
	for _, interval := range []string{"0m", "-1h", "1M", "9223372036854775808h"} {
		if _, err := IntervalDuration(interval); err == nil {
			t.Fatalf("expected invalid interval %q to fail", interval)
		}
	}
}
