package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

// Validate fully validates one committed artifact without touching the live database.
func Validate(
	ctx context.Context,
	manifestPath string,
	policy ValidationPolicy,
) (Validation, error) {
	if err := ctx.Err(); err != nil {
		return Validation{}, err
	}
	if policy.ExpectedBucketTimezone == "" || policy.MaximumSchemaVersion <= 0 {
		return Validation{}, errors.New("invalid FlowLens restore validation policy")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return Validation{}, err
	}
	if manifest.BucketTimezone != policy.ExpectedBucketTimezone || manifest.SchemaVersion > policy.MaximumSchemaVersion {
		return Validation{}, errors.New("FlowLens backup is incompatible")
	}
	dataPath := strings.TrimSuffix(manifestPath, ".manifest.json") + ".db.zst"
	temporary, err := os.CreateTemp(filepath.Dir(manifestPath), ".flowlens-restore-*.db")
	if err != nil {
		return Validation{}, errors.New("cannot create FlowLens restore validation file")
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
		_ = os.Remove(temporaryPath + "-wal")
		_ = os.Remove(temporaryPath + "-shm")
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Validation{}, errors.New("cannot secure FlowLens restore validation file")
	}
	written, digest, err := decompressAndHash(ctx, dataPath, temporary, manifest.OriginalSize)
	closeErr := temporary.Close()
	if err != nil || closeErr != nil || written != manifest.OriginalSize || digest != manifest.DatabaseSHA256 {
		return Validation{}, errors.New("FlowLens backup data validation failed")
	}
	schemaVersion, err := inspectDatabase(temporaryPath)
	if err != nil || schemaVersion != manifest.SchemaVersion || schemaVersion > policy.MaximumSchemaVersion {
		return Validation{}, errors.New("FlowLens backup database validation failed")
	}
	return Validation{Manifest: manifest}, nil
}

func readManifest(path string) (Manifest, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || !strings.HasSuffix(path, ".manifest.json") {
		return Manifest{}, errors.New("invalid FlowLens backup manifest path")
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, errors.New("cannot open FlowLens backup manifest")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<10))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, errors.New("cannot decode FlowLens backup manifest")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("FlowLens backup manifest has trailing data")
	}
	if !validManifest(manifest) {
		return Manifest{}, errors.New("invalid FlowLens backup manifest")
	}
	return manifest, nil
}

func validManifest(manifest Manifest) bool {
	if manifest.FormatVersion != 1 || manifest.ApplicationVersion == "" ||
		strings.TrimSpace(manifest.ApplicationVersion) != manifest.ApplicationVersion ||
		len(manifest.ApplicationVersion) > 64 || manifest.SchemaVersion <= 0 ||
		manifest.CreatedAt <= 0 || manifest.OriginalSize <= 0 ||
		len(manifest.DatabaseSHA256) != 64 || manifest.BucketTimezone == "" {
		return false
	}
	if _, err := hex.DecodeString(manifest.DatabaseSHA256); err != nil {
		return false
	}
	_, err := time.LoadLocation(manifest.BucketTimezone)
	return err == nil
}

func decompressAndHash(
	ctx context.Context,
	dataPath string,
	destination *os.File,
	maximum int64,
) (int64, string, error) {
	data, err := os.Open(dataPath)
	if err != nil {
		return 0, "", errors.New("cannot open FlowLens backup data")
	}
	defer data.Close()
	decoder, err := zstd.NewReader(data)
	if err != nil {
		return 0, "", errors.New("cannot decode FlowLens backup data")
	}
	defer decoder.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(contextReader{ctx: ctx, reader: decoder}, maximum+1))
	if err != nil {
		return 0, "", errors.New("cannot decompress FlowLens backup data")
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

func inspectDatabase(path string) (int, error) {
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	query.Set("mode", "ro")
	uri.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return 0, errors.New("cannot open FlowLens backup database")
	}
	defer database.Close()
	rows, err := database.Query("PRAGMA quick_check")
	if err != nil {
		return 0, errors.New("cannot check FlowLens backup database")
	}
	count := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil || result != "ok" {
			_ = rows.Close()
			return 0, errors.New("FlowLens backup quick check failed")
		}
		count++
	}
	if err := rows.Err(); err != nil || count != 1 {
		_ = rows.Close()
		return 0, errors.New("FlowLens backup quick check failed")
	}
	if err := rows.Close(); err != nil {
		return 0, errors.New("cannot close FlowLens backup check")
	}
	var schemaVersion int
	if err := database.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migration").Scan(&schemaVersion); err != nil || schemaVersion <= 0 {
		return 0, errors.New("cannot inspect FlowLens backup schema")
	}
	return schemaVersion, nil
}
