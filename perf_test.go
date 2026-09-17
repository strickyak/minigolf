package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ctranslator"
)

// TestPerf1AllBackends runs all C benchmarks under perf/perf1/tests/*.c
// through the MiniGolf translator and tests their output against their
// corresponding .want files on all three backends (CBE, amd64, m6809).
func TestPerf1AllBackends(t *testing.T) {
	files, err := filepath.Glob("perf/perf1/tests/*.c")
	if err != nil {
		t.Fatalf("Failed to glob perf/perf1/tests/*.c: %v", err)
	}
	if len(files) == 0 {
		t.Skip("No *.c files found in perf/perf1/tests/")
	}

	backends := []string{"CBE", "amd64", "m6809"}

	if err := os.MkdirAll("_tmp", 0777); err != nil {
		t.Fatalf("Failed to create _tmp directory: %v", err)
	}

	includePaths := []string{"perf/perf1", "golflib"}

	for _, cFile := range files {
		base := filepath.Base(cFile)
		stem := strings.TrimSuffix(base, ".c")
		golfFile := filepath.Join("_tmp", "perf1_"+stem+".golf")

		wantFile := filepath.Join("perf/perf1/tests", stem+".want")
		wb, err := os.ReadFile(wantFile)
		if err != nil {
			t.Errorf("Missing want file for %s: %v", cFile, err)
			continue
		}
		wantStr := string(wb)

		golfSrc, warn := ctranslator.TranslateFile(cFile, ctranslator.Options{
			IncludePaths: includePaths,
		})
		if warn != nil {
			t.Logf("ctranslator warning for %s: %v", cFile, warn)
		}
		if golfSrc == "" {
			t.Errorf("ctranslator returned empty output for %s", cFile)
			continue
		}
		if err := os.WriteFile(golfFile, []byte(golfSrc), 0666); err != nil {
			t.Fatalf("Cannot write translated golf file %s: %v", golfFile, err)
		}

		for _, backend := range backends {
			backend := backend
			golfFile := golfFile
			testName := fmt.Sprintf("%s/%s", stem, backend)
			t.Run(testName, func(t *testing.T) {
				testBackend(t, backend, golfFile, wantStr, false, false)
			})
		}
	}
}
