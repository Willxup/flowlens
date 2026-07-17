package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

const RedactedValue = "[REDACTED]"

type Secret string

func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("secret must be a string")
	}
	*s = Secret(node.Value)
	return nil
}

func (s Secret) Value() string {
	return string(s)
}

func (Secret) String() string {
	return RedactedValue
}

func (Secret) GoString() string {
	return RedactedValue
}

func (Secret) MarshalYAML() (any, error) {
	return RedactedValue, nil
}
