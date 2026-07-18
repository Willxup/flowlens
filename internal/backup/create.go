package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Create writes one online snapshot, compressed data file, and manifest commit marker.
func Create(ctx context.Context, options Options, createdAt time.Time) (Artifact, error) {
	if err := validateOptions(options, createdAt); err != nil {
		return Artifact{}, err
	}
	base := "flowlens-" + createdAt.UTC().Format("20060102T150405Z")
	artifact := Artifact{
		DataPath:     filepath.Join(options.Directory, base+".db.zst"),
		ManifestPath: filepath.Join(options.Directory, base+".manifest.json"),
	}
	for _, path := range []string{artifact.DataPath, artifact.ManifestPath} {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return Artifact{}, errors.New("FlowLens backup artifact already exists")
		}
	}
	temporaryDatabase := filepath.Join(options.Directory, "."+base+".snapshot.tmp")
	temporaryData := filepath.Join(options.Directory, "."+base+".db.zst.tmp")
	temporaryManifest := filepath.Join(options.Directory, "."+base+".manifest.tmp")
	committed := false
	defer func() {
		_ = os.Remove(temporaryDatabase)
		_ = os.Remove(temporaryDatabase + "-wal")
		_ = os.Remove(temporaryDatabase + "-shm")
		_ = os.Remove(temporaryData)
		_ = os.Remove(temporaryManifest)
		if !committed {
			_ = os.Remove(artifact.DataPath)
			_ = os.Remove(artifact.ManifestPath)
		}
	}()

	if err := options.Store.OnlineBackup(ctx, temporaryDatabase); err != nil {
		return Artifact{}, errors.New("cannot create FlowLens backup snapshot")
	}
	manifest, err := inspectSnapshot(temporaryDatabase, options, createdAt)
	if err != nil {
		return Artifact{}, err
	}
	if err := compressSnapshot(ctx, temporaryDatabase, temporaryData); err != nil {
		return Artifact{}, err
	}
	if err := verifyCompressedSnapshot(ctx, temporaryData, manifest); err != nil {
		return Artifact{}, err
	}
	if err := syncFile(temporaryData); err != nil {
		return Artifact{}, err
	}
	if err := os.Rename(temporaryData, artifact.DataPath); err != nil {
		return Artifact{}, errors.New("cannot commit FlowLens backup data")
	}
	if err := os.Chmod(artifact.DataPath, 0o600); err != nil {
		return Artifact{}, errors.New("cannot secure FlowLens backup data")
	}
	if err := syncDirectory(options.Directory); err != nil {
		return Artifact{}, err
	}
	if err := writeManifest(temporaryManifest, manifest); err != nil {
		return Artifact{}, err
	}
	if err := os.Rename(temporaryManifest, artifact.ManifestPath); err != nil {
		return Artifact{}, errors.New("cannot commit FlowLens backup manifest")
	}
	if err := os.Chmod(artifact.ManifestPath, 0o600); err != nil {
		return Artifact{}, errors.New("cannot secure FlowLens backup manifest")
	}
	if err := syncDirectory(options.Directory); err != nil {
		return Artifact{}, err
	}
	committed = true
	return artifact, nil
}

func validateOptions(options Options, createdAt time.Time) error {
	if options.Store == nil || createdAt.Unix() <= 0 || options.DailyKeep <= 0 || options.MonthlyKeep <= 0 ||
		options.ApplicationVersion == "" || strings.TrimSpace(options.ApplicationVersion) != options.ApplicationVersion ||
		len(options.ApplicationVersion) > 64 || options.BucketTimezone == "" {
		return errors.New("invalid FlowLens backup options")
	}
	if _, err := time.LoadLocation(options.BucketTimezone); err != nil {
		return errors.New("invalid FlowLens backup options")
	}
	if options.Directory == "" || !filepath.IsAbs(options.Directory) || filepath.Clean(options.Directory) != options.Directory {
		return errors.New("invalid FlowLens backup directory")
	}
	info, err := os.Stat(options.Directory)
	if err != nil || !info.IsDir() {
		return errors.New("FlowLens backup directory is unavailable")
	}
	return nil
}

func inspectSnapshot(path string, options Options, createdAt time.Time) (Manifest, error) {
	schemaVersion, err := inspectDatabase(path)
	if err != nil {
		return Manifest{}, err
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 {
		return Manifest{}, errors.New("cannot inspect FlowLens backup snapshot")
	}
	digest, err := fileSHA256(path)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{
		FormatVersion: 1, ApplicationVersion: options.ApplicationVersion,
		SchemaVersion: schemaVersion, CreatedAt: createdAt.Unix(), OriginalSize: info.Size(),
		DatabaseSHA256: digest, BucketTimezone: options.BucketTimezone,
	}, nil
}

func compressSnapshot(ctx context.Context, sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return errors.New("cannot open FlowLens backup snapshot")
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("cannot create FlowLens compressed backup")
	}
	encoder, err := zstd.NewWriter(destination)
	if err != nil {
		_ = destination.Close()
		return errors.New("cannot initialize FlowLens backup compression")
	}
	_, copyErr := io.Copy(encoder, contextReader{ctx: ctx, reader: source})
	closeEncoderErr := encoder.Close()
	closeFileErr := destination.Close()
	if copyErr != nil || closeEncoderErr != nil || closeFileErr != nil {
		return errors.New("cannot compress FlowLens backup")
	}
	return nil
}

func verifyCompressedSnapshot(ctx context.Context, path string, manifest Manifest) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("cannot open FlowLens compressed backup")
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return errors.New("cannot read FlowLens compressed backup")
	}
	defer decoder.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(contextReader{ctx: ctx, reader: decoder}, manifest.OriginalSize+1))
	if err != nil || written != manifest.OriginalSize || hex.EncodeToString(hash.Sum(nil)) != manifest.DatabaseSHA256 {
		return errors.New("FlowLens compressed backup verification failed")
	}
	return nil
}

func writeManifest(path string, manifest Manifest) error {
	contents, err := json.Marshal(manifest)
	if err != nil {
		return errors.New("cannot encode FlowLens backup manifest")
	}
	contents = append(contents, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("cannot create FlowLens backup manifest")
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return errors.New("cannot write FlowLens backup manifest")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return errors.New("cannot sync FlowLens backup manifest")
	}
	if err := file.Close(); err != nil {
		return errors.New("cannot close FlowLens backup manifest")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open FlowLens backup snapshot")
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", errors.New("cannot hash FlowLens backup snapshot")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return errors.New("cannot open FlowLens backup for sync")
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return errors.New("cannot sync FlowLens backup")
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.New("cannot open FlowLens backup directory")
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return errors.New("cannot sync FlowLens backup directory")
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
