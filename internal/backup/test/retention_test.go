package backup_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
)

func TestPruneKeepsNewestDailyAndMonthlyArtifactsWithoutDoubleCounting(t *testing.T) {
	options := backupOptions(t)
	dates := []time.Time{
		time.Date(2026, time.January, 1, 4, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 2, 4, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 1, 4, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 2, 4, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 1, 4, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 2, 4, 0, 0, 0, time.UTC),
	}
	for _, date := range dates {
		if _, err := backup.Create(context.Background(), options, date); err != nil {
			t.Fatalf("Create(%s) error = %v", date, err)
		}
	}
	if err := backup.Prune(options.Directory, 2, 2); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	entries, err := os.ReadDir(options.Directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var manifests []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".manifest.json") {
			manifests = append(manifests, entry.Name())
		}
	}
	sort.Strings(manifests)
	want := []string{
		"flowlens-20260201T040000Z.manifest.json",
		"flowlens-20260301T040000Z.manifest.json",
		"flowlens-20260302T040000Z.manifest.json",
	}
	if strings.Join(manifests, ",") != strings.Join(want, ",") {
		t.Fatalf("remaining manifests = %#v, want %#v", manifests, want)
	}
}

func TestPruneIgnoresIncompleteFilesWithoutManifest(t *testing.T) {
	options := backupOptions(t)
	incomplete := options.Directory + "/flowlens-20260101T040000Z.db.zst"
	if err := os.WriteFile(incomplete, []byte("incomplete"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := backup.Prune(options.Directory, 1, 1); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if _, err := os.Stat(incomplete); err != nil {
		t.Fatalf("incomplete file was removed: %v", err)
	}
}

func TestPruneDoesNotLetCorruptNewestArtifactEvictValidBackup(t *testing.T) {
	options := backupOptions(t)
	older, err := backup.Create(context.Background(), options, time.Date(2026, time.January, 2, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("older Create() error = %v", err)
	}
	newer, err := backup.Create(context.Background(), options, time.Date(2026, time.January, 3, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("newer Create() error = %v", err)
	}
	if err := os.WriteFile(newer.DataPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt newer data: %v", err)
	}

	if err := backup.Prune(options.Directory, 1, 1); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if _, err := os.Stat(older.ManifestPath); err != nil {
		t.Fatalf("valid older manifest was removed: %v", err)
	}
}

func TestPruneCountsDistinctNaturalDays(t *testing.T) {
	options := backupOptions(t)
	jan2, err := backup.Create(context.Background(), options, time.Date(2026, time.January, 2, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Jan 2 Create() error = %v", err)
	}
	jan3Older, err := backup.Create(context.Background(), options, time.Date(2026, time.January, 3, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("older Jan 3 Create() error = %v", err)
	}
	jan3Newer, err := backup.Create(context.Background(), options, time.Date(2026, time.January, 3, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("newer Jan 3 Create() error = %v", err)
	}

	if err := backup.Prune(options.Directory, 2, 1); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	for _, path := range []string{jan2.ManifestPath, jan3Newer.ManifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("retained manifest %q error = %v", path, err)
		}
	}
	if _, err := os.Stat(jan3Older.ManifestPath); !os.IsNotExist(err) {
		t.Fatalf("older duplicate manifest still exists: %v", err)
	}
}
