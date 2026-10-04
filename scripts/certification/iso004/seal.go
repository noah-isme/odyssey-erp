package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Seal subcommand (plan Step 6). After the run, the operator adds
// worker-journal.log (journalctl output for the run window) to --out and
// runs `iso004 seal --out DIR`. Seal regenerates SHA256SUMS only when every
// file the previous manifest listed is still present with the same hash, so
// the bundle can only grow; a changed or missing file is refused and nothing
// is written.

const workerJournalFileName = "worker-journal.log"

// sealTempPattern names the temporary manifest; it is renamed over
// SHA256SUMS so a crash never leaves a truncated manifest.
const sealTempPattern = ".SHA256SUMS.tmp-*"

// SealResult describes a successful seal.
type SealResult struct {
	Added     []string // paths that were not in the previous manifest
	Unchanged int      // previously listed files, all verified
	Written   bool     // false when there was nothing to add
}

// ManifestMismatch lists why a bundle does not match its SHA256SUMS.
type ManifestMismatch struct {
	Changed []string // listed with a different hash now
	Missing []string // listed but no longer present
	Added   []string // present but not listed
}

func (m ManifestMismatch) empty() bool {
	return len(m.Changed) == 0 && len(m.Missing) == 0 && len(m.Added) == 0
}

func (m ManifestMismatch) String() string {
	var parts []string
	if len(m.Changed) > 0 {
		parts = append(parts, "changed: "+strings.Join(m.Changed, ", "))
	}
	if len(m.Missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(m.Missing, ", "))
	}
	if len(m.Added) > 0 {
		parts = append(parts, "not listed: "+strings.Join(m.Added, ", "))
	}
	return strings.Join(parts, "; ")
}

// readManifest parses dir/SHA256SUMS. A missing, malformed or empty manifest
// is an error, as is a path listed twice.
func readManifest(dir string) ([]SumEntry, error) {
	data, err := os.ReadFile(filepath.Join(dir, sha256SumsFileName))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sha256SumsFileName, err)
	}
	entries, err := parseSums(data)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s lists no files", sha256SumsFileName)
	}
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.Path] {
			return nil, fmt.Errorf("%s lists %s twice", sha256SumsFileName, e.Path)
		}
		seen[e.Path] = true
	}
	return entries, nil
}

// compareManifest compares the listed entries with the current files.
func compareManifest(listed, current []SumEntry) ManifestMismatch {
	now := make(map[string]string, len(current))
	for _, e := range current {
		now[e.Path] = e.SHA256
	}
	var m ManifestMismatch
	was := make(map[string]bool, len(listed))
	for _, e := range listed {
		was[e.Path] = true
		sum, ok := now[e.Path]
		switch {
		case !ok:
			m.Missing = append(m.Missing, e.Path)
		case !strings.EqualFold(sum, e.SHA256):
			m.Changed = append(m.Changed, e.Path)
		}
	}
	for _, e := range current {
		if !was[e.Path] {
			m.Added = append(m.Added, e.Path)
		}
	}
	sort.Strings(m.Changed)
	sort.Strings(m.Missing)
	sort.Strings(m.Added)
	return m
}

// verifyBundle checks that dir/SHA256SUMS exists and describes the bundle
// exactly: every listed file unchanged and no unlisted file present.
func verifyBundle(dir string) ([]SumEntry, error) {
	listed, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	current, err := computeSums(dir)
	if err != nil {
		return nil, err
	}
	if m := compareManifest(listed, current); !m.empty() {
		return nil, fmt.Errorf("%s does not verify (%s)", sha256SumsFileName, m)
	}
	return listed, nil
}

// sealBundle re-seals dir. It refuses (returns an error, writes nothing) if
// any previously listed file changed or disappeared; only additions are
// accepted.
func sealBundle(dir string) (SealResult, error) {
	listed, err := readManifest(dir)
	if err != nil {
		return SealResult{}, err
	}
	current, err := computeSums(dir)
	if err != nil {
		return SealResult{}, err
	}
	m := compareManifest(listed, current)
	if len(m.Changed) > 0 || len(m.Missing) > 0 {
		return SealResult{}, fmt.Errorf("refusing to seal: previously hashed files differ (%s); nothing was written", ManifestMismatch{Changed: m.Changed, Missing: m.Missing})
	}
	res := SealResult{Added: m.Added, Unchanged: len(listed)}
	if len(m.Added) == 0 {
		return res, nil
	}
	if err := writeFileAtomic(dir, sha256SumsFileName, sealTempPattern, formatSums(current)); err != nil {
		return SealResult{}, err
	}
	res.Written = true
	return res, nil
}

// writeFileAtomic writes dir/name through a temporary file in dir that is
// synced and renamed over the target.
func writeFileAtomic(dir, name, tempPattern string, data []byte) (err error) {
	tmp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("open temporary file for %s: %w", name, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary %s: %w", name, err)
	}
	if err = tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary %s: %w", name, err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary %s: %w", name, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", name, err)
	}
	if err = os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}

func cmdSeal(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("iso004 seal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dir string
	fs.StringVar(&dir, "out", "", "evidence bundle directory written by `iso004 run` (required)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitUsage
		}
		fmt.Fprintf(stderr, "iso004 seal: %v\n", err)
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "iso004 seal: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return exitUsage
	}
	if strings.TrimSpace(dir) == "" {
		fmt.Fprintln(stderr, "iso004 seal: --out is required")
		return exitUsage
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "iso004 seal: --out %s is not a directory\n", dir)
		return exitUsage
	}

	res, err := sealBundle(dir)
	if err != nil {
		fmt.Fprintf(stderr, "iso004 seal: %v\n", err)
		return exitFail
	}
	if !res.Written {
		fmt.Fprintf(stdout, "iso004 seal: %d files verified; nothing to add, %s unchanged\n", res.Unchanged, sha256SumsFileName)
	} else {
		fmt.Fprintf(stdout, "iso004 seal: %d files verified unchanged; added %s; %s rewritten\n", res.Unchanged, strings.Join(res.Added, ", "), sha256SumsFileName)
	}
	if _, err := os.Stat(filepath.Join(dir, workerJournalFileName)); err != nil {
		fmt.Fprintf(stderr, "iso004 seal: note: %s is not in the bundle; the procedure expects the worker journal for the run window\n", workerJournalFileName)
	}
	return exitOK
}
