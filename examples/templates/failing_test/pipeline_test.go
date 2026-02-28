package main

import (
	"math"
	"testing"
)

// filtered16 is the 16-value dataset after applying Filter to the sensor readings.
var filtered16 = []int{42, 63, 28, 71, 39, 14, 55, 33, 77, 48, 60, 25, 69, 36, 52, 18}

// TestWindowLastFull verifies that window[6] (last full window) contains all 4 expected elements.
// With the bug, the clamp fires and drops element 18, producing [69 36 52] instead of [69 36 52 18].
func TestWindowLastFull(t *testing.T) {
	ws := Window(filtered16, 4, 2)
	if len(ws) < 7 {
		t.Fatalf("expected at least 7 windows, got %d", len(ws))
	}
	w := ws[6]
	want := []int{69, 36, 52, 18}
	if len(w) != len(want) {
		t.Errorf("window[6] len=%d, want %d — values: %v", len(w), len(want), w)
		return
	}
	for i, v := range w {
		if v != want[i] {
			t.Errorf("window[6][%d]=%d, want %d", i, v, want[i])
		}
	}
}

// TestWindowTrailing verifies the trailing partial window contains [52 18].
// With the bug, the clamp fires and drops 18, producing [52] instead of [52 18].
func TestWindowTrailing(t *testing.T) {
	ws := Window(filtered16, 4, 2)
	if len(ws) < 8 {
		t.Fatalf("expected 8 windows, got %d", len(ws))
	}
	w := ws[7]
	want := []int{52, 18}
	if len(w) != len(want) {
		t.Errorf("window[7] len=%d, want %d — values: %v", len(w), len(want), w)
		return
	}
	for i, v := range w {
		if v != want[i] {
			t.Errorf("window[7][%d]=%d, want %d", i, v, want[i])
		}
	}
}

// TestAggregateWindow6 verifies aggregate stats for window[6]: count=4, sum=175, mean=43.75, max=69.
// With the bug, window[6] is [69 36 52] so count=3, sum=157, mean=52.33.
func TestAggregateWindow6(t *testing.T) {
	ws := Window(filtered16, 4, 2)
	if len(ws) < 7 {
		t.Fatalf("expected at least 7 windows, got %d", len(ws))
	}
	stats := Aggregate(ws)
	got := stats[6]

	const wantCount = 4
	const wantSum = 175
	const wantMean = 43.75
	const wantMax = 69

	if got.Count != wantCount {
		t.Errorf("window[6] count=%d, want %d", got.Count, wantCount)
	}
	if got.Sum != wantSum {
		t.Errorf("window[6] sum=%d, want %d", got.Sum, wantSum)
	}
	if math.Abs(got.Mean-wantMean) > 0.01 {
		t.Errorf("window[6] mean=%.2f, want %.2f", got.Mean, wantMean)
	}
	if got.Max != wantMax {
		t.Errorf("window[6] max=%d, want %d", got.Max, wantMax)
	}
}
