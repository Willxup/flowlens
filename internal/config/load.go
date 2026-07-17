package config

import (
	"fmt"
	"os"
)

func Load() (Config, error) {
	info, err := os.Lstat(Path)
	if err != nil {
		return Config{}, fmt.Errorf("%s: unavailable: %w", Path, err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, fmt.Errorf("%s: must be a regular file", Path)
	}

	file, err := os.Open(Path)
	if err != nil {
		return Config{}, fmt.Errorf("%s: cannot be opened: %w", Path, err)
	}
	defer file.Close()

	cfg, err := Parse(file)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", Path, err)
	}
	return cfg, nil
}
