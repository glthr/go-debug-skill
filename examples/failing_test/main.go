package main

import (
	"fmt"
	"math"
	"os"
)

// sensors is the raw 20-reading dataset from the factory floor sensor.
var sensors = []int{
	42, 3, 63, 28, 71, 39, 95, 14,
	55, 33, 77, 48, 7, 60, 25, 91,
	69, 36, 52, 18,
}

// expected holds the ground-truth Stats for each of the 8 windows
// produced by Filter→Window(size=4,step=2)→Aggregate on the sensor data.
//
// Filtered (16 values): [42 63 28 71 39 14 55 33 77 48 60 25 69 36 52 18]
// Windows:
//
//	[0] [0:4]   42 63 28 71
//	[1] [2:6]   28 71 39 14
//	[2] [4:8]   39 14 55 33
//	[3] [6:10]  55 33 77 48
//	[4] [8:12]  77 48 60 25
//	[5] [10:14] 60 25 69 36
//	[6] [12:16] 69 36 52 18  ← last full window (bug truncates this)
//	[7] [14:16] 52 18        ← trailing partial
var expected = []Stats{
	{Count: 4, Sum: 204, Mean: 51.00, Max: 71},
	{Count: 4, Sum: 152, Mean: 38.00, Max: 71},
	{Count: 4, Sum: 141, Mean: 35.25, Max: 55},
	{Count: 4, Sum: 213, Mean: 53.25, Max: 77},
	{Count: 4, Sum: 210, Mean: 52.50, Max: 77},
	{Count: 4, Sum: 190, Mean: 47.50, Max: 69},
	{Count: 4, Sum: 175, Mean: 43.75, Max: 69},
	{Count: 2, Sum: 70, Mean: 35.00, Max: 52},
}

func main() {
	filtered := Filter(sensors, 10, 85)
	fmt.Printf("Filtered: %d readings\n", len(filtered))

	windows := Window(filtered, 4, 2)
	fmt.Printf("Windows:  %d\n", len(windows))

	stats := Aggregate(windows)

	failed := false
	for i, got := range stats {
		if i >= len(expected) {
			fmt.Printf("FAIL extra window[%d]: unexpected\n", i)
			failed = true
			continue
		}
		want := expected[i]
		if got.Count != want.Count {
			fmt.Printf("FAIL window[%d]: count=%d, want %d\n", i, got.Count, want.Count)
			failed = true
		}
		if got.Sum != want.Sum {
			fmt.Printf("FAIL window[%d]: sum=%d, want %d\n", i, got.Sum, want.Sum)
			failed = true
		}
		if math.Abs(got.Mean-want.Mean) > 0.01 {
			fmt.Printf("FAIL window[%d]: mean=%.2f, want %.2f\n", i, got.Mean, want.Mean)
			failed = true
		}
		if got.Max != want.Max {
			fmt.Printf("FAIL window[%d]: max=%d, want %d\n", i, got.Max, want.Max)
			failed = true
		}
	}
	for i := len(stats); i < len(expected); i++ {
		fmt.Printf("FAIL window[%d]: missing\n", i)
		failed = true
	}

	if failed {
		fmt.Println("Result: FAILED")
		os.Exit(1)
	}
	fmt.Println("Result: PASSED")
}
// test
