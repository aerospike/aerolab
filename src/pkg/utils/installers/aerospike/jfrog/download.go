package jfrog

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Download fetches f into cacheDir and returns the absolute local path.
// The cache layout mirrors the JFrog repo path so two builds with the
// same name never collide.
//
// If a cached copy with a matching SHA1 already exists it is reused
// without a network round-trip. If the SHA1 mismatches the cached file
// is overwritten.
func (c *Config) Download(ctx context.Context, f *File, cacheDir string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("jfrog: nil file")
	}
	if cacheDir == "" {
		return "", fmt.Errorf("jfrog: empty cache dir")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dir := filepath.Join(cacheDir, f.Repo, filepath.FromSlash(f.Path))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("jfrog: mkdir cache: %w", err)
	}
	local := filepath.Join(dir, f.Name)

	if ok, _ := cachedFileMatches(local, f.SHA1, f.Size); ok {
		return local, nil
	}

	tmp := local + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("jfrog: create cache file: %w", err)
	}
	written, err := c.Get(ctx, f.DownloadURL, out)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("jfrog: close cache file: %w", closeErr)
	}
	if f.Size > 0 && written != f.Size {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("jfrog: size mismatch (got %d, expected %d)", written, f.Size)
	}
	if err := os.Rename(tmp, local); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("jfrog: rename cache file: %w", err)
	}
	if ok, err := cachedFileMatches(local, f.SHA1, f.Size); !ok && f.SHA1 != "" {
		_ = os.Remove(local)
		return "", fmt.Errorf("jfrog: sha1 mismatch after download: %w", err)
	}
	return local, nil
}

// cachedFileMatches returns (true, nil) when the file exists and either
// the SHA1 matches (preferred) or the size matches (fallback when JFrog
// did not return a sha1). Any I/O error is returned.
func cachedFileMatches(path, wantSHA1 string, wantSize int64) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if wantSize > 0 && st.Size() != wantSize {
		return false, nil
	}
	if wantSHA1 == "" {
		return wantSize > 0, nil // size-only match when checksum unknown
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == wantSHA1, nil
}
