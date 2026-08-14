// Package main (cmd/datasetbuild) — source.lock generation and verification.
//
// source.lock pins the exact input dump used to generate dictionary.json.gz,
// so builds are reproducible and tampered/mismatched inputs are caught early.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// LockFile is the parsed representation of source.lock.
type LockFile struct {
	URL     string
	Fetched string // RFC3339 UTC timestamp
	SHA256  string
	Path    string // local filename, informational only
}

// GenerateLock hashes the file at dumpPath and writes a source.lock file
// at lockPath recording the URL it came from, the fetch timestamp, and the
// SHA-256 hash.
func GenerateLock(dumpPath, sourceURL, lockPath string) error {
	hash, err := sha256File(dumpPath)
	if err != nil {
		return fmt.Errorf("hashing %s: %w", dumpPath, err)
	}

	lock := LockFile{
		URL:     sourceURL,
		Fetched: time.Now().UTC().Format(time.RFC3339),
		SHA256:  hash,
		Path:    dumpPath,
	}

	f, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", lockPath, err)
	}
	defer f.Close()

	fmt.Fprintf(f, "url=%s\n", lock.URL)
	fmt.Fprintf(f, "fetched=%s\n", lock.Fetched)
	fmt.Fprintf(f, "sha256=%s\n", lock.SHA256)
	fmt.Fprintf(f, "path=%s\n", lock.Path)

	fmt.Printf("wrote %s\n  url=%s\n  fetched=%s\n  sha256=%s\n",
		lockPath, lock.URL, lock.Fetched, lock.SHA256)
	return nil
}

// VerifyLock reads lockPath and confirms that dumpPath's current SHA-256
// matches the hash recorded there. Returns a descriptive error on mismatch
// so datasetbuild can abort instead of silently generating from the wrong
// input.
func VerifyLock(dumpPath, lockPath string) error {
	lock, err := parseLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", lockPath, err)
	}

	actual, err := sha256File(dumpPath)
	if err != nil {
		return fmt.Errorf("hashing %s: %w", dumpPath, err)
	}

	if actual != lock.SHA256 {
		return fmt.Errorf(
			"checksum mismatch for %s\n  expected (source.lock): %s\n  actual:                 %s\n"+
				"the input dump does not match the version pinned in %s — "+
				"re-download the dump from %s, or regenerate source.lock with -generate-lock if this is an intentional update",
			dumpPath, lock.SHA256, actual, lockPath, lock.URL,
		)
	}

	fmt.Printf("source.lock verified: %s matches pinned sha256\n", dumpPath)
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func parseLockFile(lockPath string) (LockFile, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return LockFile{}, err
	}

	var lock LockFile
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "url":
			lock.URL = val
		case "fetched":
			lock.Fetched = val
		case "sha256":
			lock.SHA256 = val
		case "path":
			lock.Path = val
		}
	}

	if lock.SHA256 == "" {
		return LockFile{}, fmt.Errorf("%s has no sha256 entry", lockPath)
	}
	return lock, nil
}
