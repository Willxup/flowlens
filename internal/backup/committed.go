package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HasCommittedForDay reports whether a fully validated backup is committed for
// the configured natural day containing at.
func HasCommittedForDay(
	ctx context.Context,
	directory string,
	at time.Time,
	timezone string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil || at.Unix() <= 0 || directory == "" || !filepath.IsAbs(directory) ||
		filepath.Clean(directory) != directory {
		return false, errors.New("invalid FlowLens backup lookup")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, errors.New("cannot read FlowLens backup directory")
	}
	target := at.In(location)
	targetYear, targetMonth, targetDay := target.Date()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".manifest.json") {
			continue
		}
		manifestPath := filepath.Join(directory, entry.Name())
		manifest, err := readManifest(manifestPath)
		if err != nil || manifest.BucketTimezone != timezone {
			continue
		}
		created := time.Unix(manifest.CreatedAt, 0).In(location)
		year, month, day := created.Date()
		if year != targetYear || month != targetMonth || day != targetDay {
			continue
		}
		dataPath := strings.TrimSuffix(manifestPath, ".manifest.json") + ".db.zst"
		info, err := os.Lstat(dataPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if _, err := Validate(ctx, manifestPath, ValidationPolicy{
			ExpectedBucketTimezone: timezone,
			MaximumSchemaVersion:   manifest.SchemaVersion,
		}); err == nil {
			return true, nil
		} else if ctx.Err() != nil {
			return false, ctx.Err()
		}
	}
	return false, nil
}
