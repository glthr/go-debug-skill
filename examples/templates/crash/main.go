package main

import "fmt"

// stream simulates a sensor feed; nil entries represent dropped packets.
var stream = []*Reading{
	{Sensor: "temp-1", Value: 23.4},
	{Sensor: "temp-2", Value: 25.1},
	nil,
	{Sensor: "temp-3", Value: 22.8},
	{Sensor: "temp-4", Value: 24.0},
}

func main() {
	values := process(stream)
	fmt.Printf("processed %d readings: %v\n", len(values), values)
}
