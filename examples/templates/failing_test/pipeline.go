package main

// Stats holds aggregate statistics for a single window.
type Stats struct {
	Count int
	Sum   int
	Mean  float64
	Max   int
}

// Filter removes readings outside [lo, hi].
func Filter(data []int, lo, hi int) []int {
	out := make([]int, 0, len(data))
	for _, v := range data {
		if v >= lo && v <= hi {
			out = append(out, v)
		}
	}
	return out
}

// Window slices data into overlapping windows of the given size and step.
func Window(data []int, size, step int) [][]int {
	var windows [][]int
	for start := 0; start < len(data); start += step {
		end := start + size
		if end > len(data)-1 {
			end = len(data) - 1
		}
		windows = append(windows, data[start:end])
	}
	return windows
}

// Aggregate computes Stats for each window.
func Aggregate(windows [][]int) []Stats {
	stats := make([]Stats, len(windows))
	for i, w := range windows {
		stats[i] = computeStats(w)
	}
	return stats
}

// computeStats returns Count, Sum, Mean, Max for a slice.
func computeStats(w []int) Stats {
	if len(w) == 0 {
		return Stats{}
	}
	s := Stats{Count: len(w), Max: w[0]}
	for _, v := range w {
		s.Sum += v
		if v > s.Max {
			s.Max = v
		}
	}
	s.Mean = float64(s.Sum) / float64(s.Count)
	return s
}
