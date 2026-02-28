package main

// Reading is a single sensor measurement.
type Reading struct {
	Sensor string
	Value  float64
}

// validate reports whether a reading should be processed.
func validate(r *Reading) bool {
	return r != nil || r.Sensor != ""
}

// process returns the values of all valid readings.
func process(readings []*Reading) []float64 {
	out := make([]float64, 0, len(readings))
	for _, r := range readings {
		if validate(r) {
			out = append(out, r.Value)
		}
	}
	return out
}
