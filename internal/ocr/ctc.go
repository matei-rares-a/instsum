package ocr

import (
	"fmt"
	"math"
	"strings"
)

func ctcDecode(values []float32, dictionary []string) (string, float64, error) {
	if len(values) == 0 || len(dictionary) == 0 {
		return "", 0, fmt.Errorf("Paddle recognition output or dictionary is empty")
	}
	classes := len(dictionary) + 1
	if len(values)%classes != 0 {
		return "", 0, fmt.Errorf("recognition output has %d values, expected a multiple of %d classes", len(values), classes)
	}
	timesteps := len(values) / classes
	output := make([]float32, len(values))
	for timestep := 0; timestep < timesteps; timestep++ {
		row := values[timestep*classes : (timestep+1)*classes]
		sum := 0.0
		for _, value := range row {
			sum += float64(value)
		}
		if math.Abs(sum-1) < 0.02 {
			copy(output[timestep*classes:], row)
		} else {
			maximum := float64(row[0])
			for _, value := range row[1:] {
				maximum = math.Max(maximum, float64(value))
			}
			exponentialSum := 0.0
			for index, value := range row {
				exponential := math.Exp(float64(value) - maximum)
				output[timestep*classes+index] = float32(exponential)
				exponentialSum += exponential
			}
			for index := 0; index < classes; index++ {
				output[timestep*classes+index] /= float32(exponentialSum)
			}
		}
	}
	var text strings.Builder
	confidence, count, previous := 0.0, 0, -1
	for timestep := 0; timestep < timesteps; timestep++ {
		row := output[timestep*classes : (timestep+1)*classes]
		best, score := 0, float32(0)
		for class, value := range row {
			if value > score {
				best, score = class, value
			}
		}
		if best != 0 && best != previous {
			text.WriteString(dictionary[best-1])
			confidence += float64(score)
			count++
		}
		previous = best
	}
	if count == 0 {
		return "", 0, nil
	}
	return strings.TrimSpace(text.String()), confidence / float64(count), nil
}
