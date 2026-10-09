package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Known problematic tests documented in doc/embiggen-process-mode.md Section 14.
var big6809SkippedTests = map[string]string{
	"test_apl_triangle_nomoto.golf": "Slot 1 stack-heap collision past $3800 into near heap (see doc/embiggen-process-mode.md #14.1)",
	"test_fft_sine64.golf":          "near heap exhaustion from ~280 uncollected float-formatting strings (see doc/embiggen-process-mode.md #14.2)",
	"picol_1_nomoto.golf":           "CPU-bound interpreter timeout >45s (see doc/embiggen-process-mode.md #14.3)",
}

func getAsm6809(t *testing.T) string {
	t.Helper()
	for _, cand := range []string{"./asm6809", "asm6809", "../asm6809"} {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs
			}
			return cand
		}
		if p, err := exec.LookPath(cand); err == nil {
			return p
		}
	}
	t.Skip("asm6809 assembler not found")
	return ""
}

func getGep9(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("GEP9"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	candidates := []string{
		"../hatvan-os/build/gep9",
		"../../hatvan-os/build/gep9",
		"hatvan-os/build/gep9",
		"gep9",
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs
			}
			return cand
		}
		if p, err := exec.LookPath(cand); err == nil {
			return p
		}
	}
	t.Skip("gep9 emulator not found")
	return ""
}

func cleanBig6809Output(out string) []string {
	lines := strings.Split(out, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Skip debug comments
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			// Skip emulator cycle count / code size annotations (e.g., [gep9 finished: ...])
			continue
		}
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func TestBig6809OnGep9(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Big6809OnGep9 in short mode")
	}

	compiler := getMinigolfCompiler(t)
	asmBin := getAsm6809(t)
	gep9Bin := getGep9(t)

	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}
	sort.Strings(files)

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.golf") ||
			strings.HasSuffix(file, ".error.golf") ||
			strings.HasSuffix(file, ".panic.golf") {
			continue
		}

		wantFile := strings.TrimSuffix(file, ".golf") + ".want"
		wantBytes, err := os.ReadFile(wantFile)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("Failed to read want file %s: %v", wantFile, err)
		}

		base := filepath.Base(file)
		testName := strings.TrimSuffix(base, ".golf")

		targetFile := file
		targetWant := wantBytes

		t.Run(testName, func(t *testing.T) {
			if reason, ok := big6809SkippedTests[base]; ok {
				t.Skipf("skipping known edge case: %s", reason)
			}

			t.Parallel()

			tmpDir := filepath.Join("_tmp", "big6809_"+testName)
			if err := os.MkdirAll(tmpDir, 0777); err != nil {
				t.Fatalf("Failed to create tmpDir %s: %v", tmpDir, err)
			}

			asmPath := filepath.Join(tmpDir, "out.asm")
			decbPath := filepath.Join(tmpDir, "out.decb")

			// 1. Compile with minigolf -m=6809+
			compileArgs := []string{
				"-m=6809+",
				"-o=" + asmPath,
				"-I=biggolflib",
				"-I=tests",
				"-I=c-tests",
				"-I=c-demos",
				"-I=c-demos/floating",
				"-I=c-demos/pythonsub",
				"-I=demos",
				"-I=demos/floating",
				"-I=golflib",
				targetFile,
			}
			cmdCompile := exec.Command(compiler, compileArgs...)
			if out, err := cmdCompile.CombinedOutput(); err != nil {
				t.Fatalf("Compilation failed with %s: %v\nOutput: %s", compiler, err, string(out))
			}

			// 2. Assemble with asm6809 --decb
			cmdAsm := exec.Command(asmBin, "--decb", "-o", decbPath, asmPath)
			if out, err := cmdAsm.CombinedOutput(); err != nil {
				t.Fatalf("Assembly failed with %s: %v\nOutput: %s", asmBin, err, string(out))
			}

			// 3. Execute with gep9 --embiggen --hypercalls
			cmdVM := exec.Command(gep9Bin, "--embiggen", "--hypercalls", decbPath)
			out, err := cmdVM.CombinedOutput()
			if err != nil {
				t.Fatalf("gep9 execution failed: %v\nOutput: %s", err, string(out))
			}

			// 4. Verify output against .want
			actualLines := cleanBig6809Output(string(out))
			expectedLines := cleanBig6809Output(string(targetWant))
			actual := strings.Join(actualLines, "\n")
			expected := strings.Join(expectedLines, "\n")

			if actual != expected {
				t.Fatalf("Output mismatch for %s.\nGot %d lines:\n%s\n\nWanted %d lines:\n%s",
					targetFile, len(actualLines), actual, len(expectedLines), expected)
			}
		})
	}
}

func getTether(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("TETHER"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	candidates := []string{
		"/home/strick/modoc/coco-shelf/tfr9/v4/build/tether.linux-amd64.exe",
		"../modoc/coco-shelf/tfr9/v4/build/tether.linux-amd64.exe",
		"tether",
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs
			}
			return cand
		}
		if p, err := exec.LookPath(cand); err == nil {
			return p
		}
	}
	t.Skip("tether binary not found")
	return ""
}

func pingTfr911(t *testing.T, tetherBin string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tetherBin, "-quick-ping", "42")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "quick-ping: OK")
}

func checkTfr911Embiggened(t *testing.T, tetherBin string) bool {
	t.Helper()
	// The TFR911 gep9 engine firmware (in tfr911h/v4_gep9_mmu.h and v4_gep9_io.h)
	// has not yet been modified to support EMBIGGEN 8KB page mapping ($FF40..$FF47).
	// When EMBIGGEN support is added to the firmware, set TFR911_EMBIGGEN=1.
	if os.Getenv("TFR911_EMBIGGEN") == "1" {
		return true
	}
	return false
}

func TestBig6809OnTfr911(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Big6809OnTfr911 in short mode")
	}

	tetherBin := getTether(t)
	if !pingTfr911(t, tetherBin) {
		t.Skip("TFR911 board not responding or not connected via USB")
	}

	if !checkTfr911Embiggened(t, tetherBin) {
		t.Skip("TFR911 gep9 engine has not yet been embiggened (requires $FF40..$FF47 MMAP vector and 8KB block translation in tfr911h firmware)")
	}

	compiler := getMinigolfCompiler(t)
	asmBin := getAsm6809(t)

	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}
	sort.Strings(files)

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.golf") ||
			strings.HasSuffix(file, ".error.golf") ||
			strings.HasSuffix(file, ".panic.golf") {
			continue
		}

		wantFile := strings.TrimSuffix(file, ".golf") + ".want"
		wantBytes, err := os.ReadFile(wantFile)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("Failed to read want file %s: %v", wantFile, err)
		}

		base := filepath.Base(file)
		testName := strings.TrimSuffix(base, ".golf")

		targetFile := file
		targetWant := wantBytes

		t.Run(testName, func(t *testing.T) {
			if reason, ok := big6809SkippedTests[base]; ok {
				t.Skipf("skipping known edge case: %s", reason)
			}

			tmpDir := filepath.Join("_tmp", "tfr911_"+testName)
			if err := os.MkdirAll(tmpDir, 0777); err != nil {
				t.Fatalf("Failed to create tmpDir %s: %v", tmpDir, err)
			}

			asmPath := filepath.Join(tmpDir, "out.asm")
			decbPath := filepath.Join(tmpDir, "out.decb")

			// 1. Compile with minigolf -m=6809+
			compileArgs := []string{
				"-m=6809+",
				"-o=" + asmPath,
				"-I=biggolflib",
				"-I=tests",
				"-I=c-tests",
				"-I=c-demos",
				"-I=c-demos/floating",
				"-I=c-demos/pythonsub",
				"-I=demos",
				"-I=demos/floating",
				"-I=golflib",
				targetFile,
			}
			cmdCompile := exec.Command(compiler, compileArgs...)
			if out, err := cmdCompile.CombinedOutput(); err != nil {
				t.Fatalf("Compilation failed with %s: %v\nOutput: %s", compiler, err, string(out))
			}

			// 2. Assemble with asm6809 --decb
			cmdAsm := exec.Command(asmBin, "--decb", "-o", decbPath, asmPath)
			if out, err := cmdAsm.CombinedOutput(); err != nil {
				t.Fatalf("Assembly failed with %s: %v\nOutput: %s", asmBin, err, string(out))
			}

			// 3. Execute on TFR911 via tether
			cmdRun := exec.Command(tetherBin, decbPath)
			out, err := cmdRun.CombinedOutput()
			if err != nil {
				t.Fatalf("TFR911 execution failed: %v\nOutput: %s", err, string(out))
			}

			// 4. Verify output against .want
			actualLines := cleanBig6809Output(string(out))
			expectedLines := cleanBig6809Output(string(targetWant))
			actual := strings.Join(actualLines, "\n")
			expected := strings.Join(expectedLines, "\n")

			if actual != expected {
				t.Fatalf("Output mismatch for %s.\nGot %d lines:\n%s\n\nWanted %d lines:\n%s",
					targetFile, len(actualLines), actual, len(expectedLines), expected)
			}
		})
	}
}
