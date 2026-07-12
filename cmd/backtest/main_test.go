package main

import "testing"

func TestParseInts(t *testing.T) {
	t.Parallel()

	values, err := parseInts("6, 12,12,24")
	if err != nil {
		t.Fatalf("parse ints: %v", err)
	}

	want := []int{6, 12, 24}
	if len(values) != len(want) {
		t.Fatalf("len = %d, want %d", len(values), len(want))
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("values[%d] = %d, want %d", i, values[i], want[i])
		}
	}
}

func TestBuildConfigsSkipsInvalidWindows(t *testing.T) {
	t.Parallel()

	configs := buildConfigs([]int{6, 24}, []int{12, 24}, 0.001)
	if len(configs) != 2 {
		t.Fatalf("len = %d, want 2", len(configs))
	}
	if configs[0].FastWindow != 6 || configs[0].SlowWindow != 12 {
		t.Fatalf("first config = %+v, want 6/12", configs[0])
	}
	if configs[1].FastWindow != 6 || configs[1].SlowWindow != 24 {
		t.Fatalf("second config = %+v, want 6/24", configs[1])
	}
}
