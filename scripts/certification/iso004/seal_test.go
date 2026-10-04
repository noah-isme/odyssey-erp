package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSealedBundle writes a small bundle for runID under a fresh parent
// directory and seals it with SHA256SUMS, like `iso004 run` does.
func makeSealedBundle(t *testing.T, runID string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "iso-004-out")
	files := map[string]string{
		runFileName:                         `{"tool":"iso004","run_id":"` + runID + `"}` + "\n",
		summaryJSONFileName:                 `{"evidence_id":"ISO-004","result":"PASS"}` + "\n",
		evidenceRecordName:                  "CERTIFICATION_EVIDENCE evidence_id=ISO-004 {}\n",
		"scenarios/S01-forged/result.json":  `{"result":"PASS"}` + "\n",
		"scenarios/S01-forged/enqueue.json": `{"submissions":[]}` + "\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	require.NoError(t, writeSHA256SUMS(dir))
	return dir
}

func readSums(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, sha256SumsFileName))
	require.NoError(t, err)
	return b
}

func runSeal(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := dispatch(context.Background(), append([]string{"seal"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// assertNoTempFiles proves a refused or completed seal left no temporary
// manifest behind (it would be hashed by the next seal).
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".SHA256SUMS.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestSealAddsWorkerJournal(t *testing.T) {
	dir := makeSealedBundle(t, "4242")
	before, err := parseSums(readSums(t, dir))
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, workerJournalFileName), []byte("Oct 04 worker[1]: processed iso004:4242:S01:1\n"), 0o644))
	code, stdout, stderr := runSeal(t, "--out", dir)
	require.Equal(t, exitOK, code, stderr)
	assert.Contains(t, stdout, "added "+workerJournalFileName)
	assert.NotContains(t, stderr, "note:")

	assertSumsVerify(t, dir)
	after, err := parseSums(readSums(t, dir))
	require.NoError(t, err)
	assert.Len(t, after, len(before)+1)
	for _, e := range before {
		assert.Contains(t, after, e, "previous entries are kept verbatim")
	}
	assert.Contains(t, string(readSums(t, dir)), "  "+workerJournalFileName+"\n")
	assertNoTempFiles(t, dir)

	// Sealing again with nothing new is a no-op that still succeeds.
	sums := readSums(t, dir)
	code, stdout, stderr = runSeal(t, "--out", dir)
	require.Equal(t, exitOK, code, stderr)
	assert.Contains(t, stdout, "nothing to add")
	assert.Equal(t, sums, readSums(t, dir))
}

func TestSealRefusesChangedOrMissingFiles(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   string
	}{
		{"modified file", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, summaryJSONFileName), []byte(`{"result":"FAIL"}`), 0o644))
		}, "changed: " + summaryJSONFileName},
		{"modified nested file", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "scenarios", "S01-forged", "result.json"), []byte(`{"result":"FAIL"}`), 0o644))
		}, "changed: scenarios/S01-forged/result.json"},
		{"removed file", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, evidenceRecordName)))
		}, "missing: " + evidenceRecordName},
		{"modified file plus added journal", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, workerJournalFileName), []byte("journal\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, runFileName), []byte(`{"run_id":"other"}`), 0o644))
		}, "changed: " + runFileName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := makeSealedBundle(t, "4242")
			sums := readSums(t, dir)
			tc.mutate(t, dir)
			code, stdout, stderr := runSeal(t, "--out", dir)
			assert.Equal(t, exitFail, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "refusing to seal")
			assert.Contains(t, stderr, tc.want)
			assert.Contains(t, stderr, "nothing was written")
			assert.Equal(t, sums, readSums(t, dir), "SHA256SUMS is untouched")
			assertNoTempFiles(t, dir)
		})
	}
}

func TestSealRefusesWithoutUsableManifest(t *testing.T) {
	t.Run("missing SHA256SUMS", func(t *testing.T) {
		dir := makeSealedBundle(t, "4242")
		require.NoError(t, os.Remove(filepath.Join(dir, sha256SumsFileName)))
		code, _, stderr := runSeal(t, "--out", dir)
		assert.Equal(t, exitFail, code)
		assert.Contains(t, stderr, "read "+sha256SumsFileName)
		_, err := os.Stat(filepath.Join(dir, sha256SumsFileName))
		assert.ErrorIs(t, err, os.ErrNotExist, "seal never creates a first manifest")
	})
	t.Run("malformed SHA256SUMS", func(t *testing.T) {
		dir := makeSealedBundle(t, "4242")
		require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), []byte("not a manifest\n"), 0o644))
		code, _, stderr := runSeal(t, "--out", dir)
		assert.Equal(t, exitFail, code)
		assert.Contains(t, stderr, "malformed")
	})
	t.Run("empty SHA256SUMS", func(t *testing.T) {
		dir := makeSealedBundle(t, "4242")
		require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), nil, 0o644))
		code, _, stderr := runSeal(t, "--out", dir)
		assert.Equal(t, exitFail, code)
		assert.Contains(t, stderr, "lists no files")
	})
	t.Run("duplicate entry", func(t *testing.T) {
		dir := makeSealedBundle(t, "4242")
		sums := readSums(t, dir)
		first := strings.SplitAfter(string(sums), "\n")[0]
		require.NoError(t, os.WriteFile(filepath.Join(dir, sha256SumsFileName), append(sums, first...), 0o644))
		code, _, stderr := runSeal(t, "--out", dir)
		assert.Equal(t, exitFail, code)
		assert.Contains(t, stderr, "twice")
	})
}

func TestSealUsage(t *testing.T) {
	code, _, stderr := runSeal(t)
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, stderr, "--out is required")

	code, _, stderr = runSeal(t, "--out", filepath.Join(t.TempDir(), "absent"))
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, stderr, "is not a directory")

	code, _, _ = runSeal(t, "--out", t.TempDir(), "extra")
	assert.Equal(t, exitUsage, code)
}

func TestSealNotesMissingJournal(t *testing.T) {
	dir := makeSealedBundle(t, "4242")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "operator-notes.txt"), []byte("notes\n"), 0o644))
	code, _, stderr := runSeal(t, "--out", dir)
	require.Equal(t, exitOK, code, stderr)
	assert.Contains(t, stderr, "note: "+workerJournalFileName+" is not in the bundle")
	assertSumsVerify(t, dir)
}
