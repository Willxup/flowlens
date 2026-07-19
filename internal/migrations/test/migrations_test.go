package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Willxup/flowlens/internal/migrations"
)

func TestListReturnsDeterministicImmutableManifest(t *testing.T) {
	first, err := migrations.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	second, err := migrations.List()
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("List() results differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if len(first) != 2 {
		t.Fatalf("len(List()) = %d", len(first))
	}
	wantNames := []string{"001_initial.sql", "002_stage3_unattributed_backfill.sql"}
	for index, migration := range first {
		if migration.Version != index+1 || migration.Name != wantNames[index] {
			t.Errorf("migration[%d] identity = version:%d name:%q", index, migration.Version, migration.Name)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			t.Errorf("migration[%d] SQL is empty", index)
		}
		sum := sha256.Sum256([]byte(migration.SQL))
		wantChecksum := hex.EncodeToString(sum[:])
		if migration.Checksum != wantChecksum {
			t.Errorf("migration[%d] checksum = %q, want %q", index, migration.Checksum, wantChecksum)
		}
		if matched, _ := regexp.MatchString(`^[0-9a-f]{64}$`, migration.Checksum); !matched {
			t.Errorf("migration[%d] checksum format = %q", index, migration.Checksum)
		}
	}

	first[0].Name = "mutated.sql"
	third, err := migrations.List()
	if err != nil {
		t.Fatalf("third List() error = %v", err)
	}
	if third[0].Name != "001_initial.sql" {
		t.Errorf("List() exposed mutable state: %#v", third[0])
	}
}

func TestLatestVersionIsTwo(t *testing.T) {
	version, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if version != 2 {
		t.Errorf("LatestVersion() = %d", version)
	}
}

func TestInitialMigrationDefinesExactApplicationTables(t *testing.T) {
	migrationList, err := migrations.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{
		"collector_state",
		"flow_dimension",
		"flow_rollup",
		"maintenance_run",
		"quality_event",
		"runtime_session",
		"schema_migration",
		"service_label",
		"traffic_rollup",
	}
	expression := regexp.MustCompile(`(?im)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)`)
	matches := expression.FindAllStringSubmatch(migrationList[0].SQL, -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, strings.ToLower(match[1]))
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("migration tables = %#v, want %#v", got, want)
	}
}
