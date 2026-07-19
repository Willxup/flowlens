package backup_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Willxup/flowlens/internal/backup"
)

func TestValidateRejectsCorruptionUnknownFieldsAndPolicyMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, artifact backup.Artifact)
		policy backup.ValidationPolicy
	}{
		{
			name: "corrupt zstd",
			mutate: func(t *testing.T, artifact backup.Artifact) {
				t.Helper()
				if err := os.WriteFile(artifact.DataPath, []byte("not zstd"), 0o600); err != nil {
					t.Fatalf("corrupt data: %v", err)
				}
			},
			policy: backup.ValidationPolicy{ExpectedBucketTimezone: "Asia/Shanghai", MaximumSchemaVersion: latestSchemaVersion(t)},
		},
		{
			name: "unknown manifest field",
			mutate: func(t *testing.T, artifact backup.Artifact) {
				t.Helper()
				contents, err := os.ReadFile(artifact.ManifestPath)
				if err != nil {
					t.Fatalf("read manifest: %v", err)
				}
				var fields map[string]any
				if err := json.Unmarshal(contents, &fields); err != nil {
					t.Fatalf("decode manifest: %v", err)
				}
				fields["extra"] = true
				contents, _ = json.Marshal(fields)
				if err := os.WriteFile(artifact.ManifestPath, contents, 0o600); err != nil {
					t.Fatalf("rewrite manifest: %v", err)
				}
			},
			policy: backup.ValidationPolicy{ExpectedBucketTimezone: "Asia/Shanghai", MaximumSchemaVersion: latestSchemaVersion(t)},
		},
		{
			name:   "timezone mismatch",
			mutate: func(t *testing.T, artifact backup.Artifact) {},
			policy: backup.ValidationPolicy{ExpectedBucketTimezone: "UTC", MaximumSchemaVersion: latestSchemaVersion(t)},
		},
		{
			name:   "schema too new",
			mutate: func(t *testing.T, artifact backup.Artifact) {},
			policy: backup.ValidationPolicy{ExpectedBucketTimezone: "Asia/Shanghai", MaximumSchemaVersion: 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := backupOptions(t)
			artifact, err := backup.Create(context.Background(), options, time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			test.mutate(t, artifact)
			if _, err := backup.Validate(context.Background(), artifact.ManifestPath, test.policy); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestValidateHonorsCanceledContext(t *testing.T) {
	options := backupOptions(t)
	artifact, err := backup.Create(context.Background(), options, time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backup.Validate(ctx, artifact.ManifestPath, backup.ValidationPolicy{
		ExpectedBucketTimezone: options.BucketTimezone, MaximumSchemaVersion: latestSchemaVersion(t),
	}); err == nil {
		t.Fatal("Validate() error = nil with canceled context")
	}
}
