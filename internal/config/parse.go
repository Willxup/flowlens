package config

import (
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

func Parse(reader io.Reader) (Config, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("configuration is invalid YAML or contains unknown fields")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("configuration must contain exactly one YAML document")
	}

	if err := Validate(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
