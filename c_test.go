package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllCFiles globs c-tests/*.c, translates each with cc_to_golf into a
// temporary .golf file, then runs it through all three backends (CBE,
// x86_64, m6809) comparing stdout against the matching .want file.
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

	// Build (or reuse) the cc_to_golf binary once per test run.
	ccToGolf := filepath.Join("_tmp", fmt.Sprintf("cc_to_golf.%d", os.Getpid()))
	if _, err := os.Stat(ccToGolf); err != nil {
		cmd := exec.Command("go", "build", "-o", ccToGolf, "./cc_v5/cmd/cc_to_golf/")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to build cc_to_golf: %v\n%s", err, out)
		}
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

		// Run cc_to_golf to produce the .golf file.
		translateCmd := exec.Command(ccToGolf, cFile)
		golfBytes, translateErr := translateCmd.Output()
		if translateErr != nil {
			if expectCompileError {
				// Translation failure counts as a compile error — all backends pass.
				continue
			}
			t.Errorf("cc_to_golf failed for %s: %v\nStderr: %s", cFile, translateErr,
				func() string {
					if ee, ok := translateErr.(*exec.ExitError); ok {
						return string(ee.Stderr)
					}
					return ""
				}())
			continue
		}
		if err := os.WriteFile(golfFile, golfBytes, 0666); err != nil {
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
