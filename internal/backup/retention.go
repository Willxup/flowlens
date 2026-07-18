package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type retainedArtifact struct {
	manifest Manifest
	artifact Artifact
	local    time.Time
}

// Prune keeps the newest daily and first-of-month artifacts, counting overlap once.
func Prune(directory string, dailyKeep, monthlyKeep int) error {
	if directory == "" || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory ||
		dailyKeep <= 0 || monthlyKeep <= 0 {
		return errors.New("invalid FlowLens backup retention")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return errors.New("cannot read FlowLens backup directory")
	}
	var artifacts []retainedArtifact
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".manifest.json") {
			continue
		}
		manifestPath := filepath.Join(directory, entry.Name())
		manifest, err := readManifest(manifestPath)
		if err != nil {
			continue
		}
		dataPath := strings.TrimSuffix(manifestPath, ".manifest.json") + ".db.zst"
		info, err := os.Stat(dataPath)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		validation, err := Validate(context.Background(), manifestPath, ValidationPolicy{
			ExpectedBucketTimezone: manifest.BucketTimezone,
			MaximumSchemaVersion:   manifest.SchemaVersion,
		})
		if err != nil {
			continue
		}
		location, err := time.LoadLocation(validation.Manifest.BucketTimezone)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, retainedArtifact{
			manifest: validation.Manifest,
			artifact: Artifact{DataPath: dataPath, ManifestPath: manifestPath},
			local:    time.Unix(validation.Manifest.CreatedAt, 0).In(location),
		})
	}
	sort.Slice(artifacts, func(left, right int) bool {
		return artifacts[left].manifest.CreatedAt > artifacts[right].manifest.CreatedAt
	})
	keep := make(map[string]struct{})
	seenDays := make(map[string]struct{})
	dailyArtifacts := make([]retainedArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		dayKey := artifact.manifest.BucketTimezone + ":" + artifact.local.Format("2006-01-02")
		if _, exists := seenDays[dayKey]; exists {
			continue
		}
		seenDays[dayKey] = struct{}{}
		dailyArtifacts = append(dailyArtifacts, artifact)
	}
	for index := 0; index < len(dailyArtifacts) && index < dailyKeep; index++ {
		keep[dailyArtifacts[index].artifact.ManifestPath] = struct{}{}
	}
	monthly := 0
	seenMonths := make(map[string]struct{})
	for _, artifact := range dailyArtifacts {
		if artifact.local.Day() != 1 {
			continue
		}
		monthKey := artifact.manifest.BucketTimezone + ":" + artifact.local.Format("2006-01")
		if _, exists := seenMonths[monthKey]; exists {
			continue
		}
		seenMonths[monthKey] = struct{}{}
		keep[artifact.artifact.ManifestPath] = struct{}{}
		monthly++
		if monthly == monthlyKeep {
			break
		}
	}
	for _, artifact := range artifacts {
		if _, exists := keep[artifact.artifact.ManifestPath]; exists {
			continue
		}
		if err := os.Remove(artifact.artifact.ManifestPath); err != nil {
			return errors.New("cannot delete expired FlowLens backup manifest")
		}
		if err := syncDirectory(directory); err != nil {
			return err
		}
		if err := os.Remove(artifact.artifact.DataPath); err != nil {
			return errors.New("cannot delete expired FlowLens backup data")
		}
	}
	return syncDirectory(directory)
}
