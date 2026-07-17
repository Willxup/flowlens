package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("duration must be a string")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration")
	}
	d.Duration = parsed
	return nil
}

type ByteSize int64

func (b *ByteSize) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("byte size must be a string")
	}

	units := []struct {
		suffix     string
		multiplier int64
	}{
		{suffix: "GiB", multiplier: 1 << 30},
		{suffix: "MiB", multiplier: 1 << 20},
		{suffix: "KiB", multiplier: 1 << 10},
		{suffix: "B", multiplier: 1},
	}

	for _, unit := range units {
		if !strings.HasSuffix(node.Value, unit.suffix) {
			continue
		}
		number := strings.TrimSuffix(node.Value, unit.suffix)
		if number == "" || (len(number) > 1 && number[0] == '0') || !containsOnlyASCIIDigits(number) {
			return fmt.Errorf("invalid byte size")
		}
		value, err := strconv.ParseInt(number, 10, 64)
		if err != nil || value <= 0 || value > math.MaxInt64/unit.multiplier {
			return fmt.Errorf("invalid byte size")
		}
		*b = ByteSize(value * unit.multiplier)
		return nil
	}

	return fmt.Errorf("invalid byte size")
}

type ClockTime struct {
	Hour   int
	Minute int
}

func (c *ClockTime) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("local time must be a string")
	}
	if len(node.Value) != 5 || node.Value[2] != ':' ||
		!isASCIIDigit(node.Value[0]) || !isASCIIDigit(node.Value[1]) ||
		!isASCIIDigit(node.Value[3]) || !isASCIIDigit(node.Value[4]) {
		return fmt.Errorf("local time must use HH:MM")
	}
	hour, hourErr := strconv.Atoi(node.Value[:2])
	minute, minuteErr := strconv.Atoi(node.Value[3:])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return fmt.Errorf("local time must use HH:MM")
	}
	c.Hour = hour
	c.Minute = minute
	return nil
}

func containsOnlyASCIIDigits(value string) bool {
	for index := range len(value) {
		if !isASCIIDigit(value[index]) {
			return false
		}
	}
	return true
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
