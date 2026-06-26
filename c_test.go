package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ctranslator"
)

// TestAllCFiles globs c-tests/*.c, translates each with the in-process
// ctranslator into a temporary .golf file, then runs it through all three
// backends (CBE, x86_64, m6809) comparing stdout against the matching .want
// file.
//
// Naming conventions (mirroring system_test.go):
//   - foo.c          – normal test, must match foo.want
//   - foo.bad.c      – skipped (work-in-progress)
//   - foo.error.c    – translation or compile error expected
//   - foo.panic.c    – run-time panic expected
//   - foo_nomoto.c   – skip the m6809 backend
//   - foo_nocbe.c    – skip the CBE backend
//   - foo_motoonly.c – only run on the m6809 backend (uses hardware I/O, etc.)
func TestAllCFiles(t *testing.T) {
	files, err := filepath.Glob("c-tests/*.c")
	if err != nil {
		t.Fatalf("Failed to glob c-tests/*.c: %v", err)
	}
	if len(files) == 0 {
		t.Skip("No *.c files found in c-tests/")
	}

	backends := []string{"CBE", "x86_64", "m6809"}

	for _, cFile := range files {
		base := filepath.Base(cFile) // e.g. "hello1.c"

		if strings.HasSuffix(base, ".bad.c") {
			continue // skip known broken / WIP files
		}

		expectCompileError := strings.HasSuffix(base, ".error.c")
		expectRunError := strings.HasSuffix(base, ".panic.c")

		// Translate C → MiniGolf once, reuse across backends.
		stem := strings.TrimSuffix(base, ".c") // e.g. "hello1"
		golfFile := filepath.Join("_tmp", "c_test_"+stem+".golf")

		// Read the expected output (unless we expect an error).
		var wantStr string
		if !expectCompileError && !expectRunError {
			wantFile := filepath.Join("c-tests", stem+".want")
			wb, err := os.ReadFile(wantFile)
			if err != nil {
				t.Errorf("Missing want file for %s: %v", cFile, err)
				continue
			}
			wantStr = string(wb)
		}

		// Load extra include paths from a sidecar <stem>.ipath file, if present.
		// Each non-empty, non-comment line is an include path (relative to the
		// repo root). Paths are prepended so that golflib remains last (system).
		includePaths := []string{"golflib"}
		iPaths, err2 := os.ReadFile(filepath.Join("c-tests", stem+".ipath"))
		if err2 == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(iPaths)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					includePaths = append([]string{line}, includePaths...) // prepend; golflib stays last
				}
			}
		}

		// Translate in-process using ctranslator.
		golfSrc, warn := ctranslator.TranslateFile(cFile, ctranslator.Options{
			IncludePaths: includePaths,
		})
		if warn != nil {
			t.Logf("cc warning for %s: %v", cFile, warn)
		}
		if golfSrc == "" {
			if expectCompileError {
				// Empty output on error path counts as a compile-error — all backends pass.
				continue
			}
			t.Errorf("ctranslator returned empty output for %s", cFile)
			continue
		}
		if err := os.WriteFile(golfFile, []byte(golfSrc), 0666); err != nil {
			t.Fatalf("Cannot write translated golf file %s: %v", golfFile, err)
		}

		for _, backend := range backends {
			if strings.HasSuffix(base, "_nomoto.c") && backend == "m6809" {
				continue
			}
			if strings.HasSuffix(base, "_nocbe.c") && backend == "CBE" {
				continue
			}
			if strings.HasSuffix(base, "_motoonly.c") && backend != "m6809" {
				continue
			}
			backend := backend // capture for t.Run closure
			golfFile := golfFile
			testName := fmt.Sprintf("%s/%s", stem, backend)
			t.Run(testName, func(t *testing.T) {
				testBackend(t, backend, golfFile, wantStr, expectCompileError, expectRunError)
			})
		}
	}
}
