package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

//go:embed *.sql
var migrationFiles embed.FS

var migrationNamePattern = regexp.MustCompile(`^([0-9]{3})_[a-z0-9]+(?:_[a-z0-9]+)*\.sql$`)

// Migration is one immutable embedded database migration.
type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// List returns the complete ordered migration manifest.
func List() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, ".")
	if err != nil {
		return nil, errors.New("cannot read embedded migration manifest")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, errors.New("embedded migration manifest contains a directory")
		}
		if !migrationNamePattern.MatchString(entry.Name()) {
			return nil, errors.New("embedded migration has an invalid name")
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	migrations := make([]Migration, 0, len(names))
	for index, name := range names {
		matches := migrationNamePattern.FindStringSubmatch(name)
		version, err := strconv.Atoi(matches[1])
		if err != nil || version != index+1 {
			return nil, errors.New("embedded migration versions are not contiguous")
		}
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, errors.New("cannot read embedded migration")
		}
		sum := sha256.Sum256(contents)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     name,
			SQL:      string(contents),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return migrations, nil
}

// LatestVersion returns the highest embedded schema version.
func LatestVersion() (int, error) {
	migrations, err := List()
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, nil
	}
	return migrations[len(migrations)-1].Version, nil
}
