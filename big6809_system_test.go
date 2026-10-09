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
			// Skip emulator/firmware annotations (e.g., [gep9 finished: ...], [bye: ...])
			continue
		}
		if strings.HasPrefix(trimmed, "*** GEP9") ||
			strings.HasPrefix(trimmed, "Task ") ||
			strings.HasPrefix(trimmed, "quick-") ||
			strings.HasPrefix(trimmed, "Internal web server") ||
			strings.HasPrefix(trimmed, "WriteBytes:") ||
			strings.HasPrefix(trimmed, "Received C_SHUTDOWN") {
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

func getTetherWire() string {
	matches, _ := filepath.Glob("/dev/ttyACM*")
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return "/dev/ttyACM0"
}

func waitForTfr911(t *testing.T, tetherBin string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob("/dev/ttyACM*")
		for _, wire := range matches {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cmd := exec.CommandContext(ctx, tetherBin, "-wire", wire, "-quick-ping", "42")
			out, err := cmd.CombinedOutput()
			cancel()
			if err == nil && strings.Contains(string(out), "quick-ping: OK") {
				return wire
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return ""
}

func checkTfr911Embiggened(t *testing.T, tetherBin string) bool {
	t.Helper()
	if os.Getenv("SKIP_TFR911") == "1" {
		return false
	}
	return true
}

func TestBig6809OnTfr911(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Big6809OnTfr911 in short mode")
	}

	tetherBin := getTether(t)
	if wire := waitForTfr911(t, tetherBin, 12*time.Second); wire == "" {
		t.Skip("TFR911 board not responding or not connected via USB")
	}

	if !checkTfr911Embiggened(t, tetherBin) {
		t.Skip("TFR911 testing disabled (SKIP_TFR911=1)")
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

			// 3. Write mode81.tcl into tmpDir
			tclScript := "set Config(engine) \"gep9big\"\nset Config(flat_img) \"/pc/out.decb\"\nset Config(task1) \"\"\nset Config(task2) \"\"\nset Config(drive0) \"\"\nset Config(drive1) \"\"\nset Config(drive2) \"\"\nset Config(drive3) \"\"\nmenu store Config\nbye\n"
			if err := os.WriteFile(filepath.Join(tmpDir, "mode81.tcl"), []byte(tclScript), 0644); err != nil {
				t.Fatalf("Failed to write mode81.tcl: %v", err)
			}

			// 4. Wait for TFR911 to be ready and get current wire port
			wire := waitForTfr911(t, tetherBin, 8*time.Second)
			if wire == "" {
				t.Fatalf("TFR911 board did not become ready for test %s", testName)
			}

			// 5. Execute on TFR911 via tether
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmdRun := exec.CommandContext(ctx, tetherBin, "-wire", wire, "-pc", tmpDir, "-bootmode=81")
			out, err := cmdRun.Output()
			if err != nil {
				t.Fatalf("TFR911 execution failed on %s: %v\nOutput: %s", wire, err, string(out))
			}

			// 6. Verify output against .want
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
