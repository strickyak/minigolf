package main_test

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const expectedOutput = `Triangle number 1 is 1
Triangle number 2 is 3
Triangle number 3 is 6
Triangle number 4 is 10
Triangle number 5 is 15
Triangle number 6 is 21
Triangle number 7 is 28
Triangle number 8 is 36
Triangle number 9 is 45
Triangle number 10 is 55`

const expectedOutputByte = `Triangle number 1 is 1
Triangle number 2 is 3
Triangle number 3 is 6
Triangle number 4 is 10
Triangle number 5 is 15
Triangle number 6 is 21
Triangle number 7 is 28
Triangle number 8 is 36
Triangle number 9 is 45
Triangle number 10 is 55
Triangle number 11 is 66
Triangle number 12 is 78
Triangle number 13 is 91
Triangle number 14 is 105
Triangle number 15 is 120
Triangle number 16 is 136
Triangle number 17 is 153
Triangle number 18 is 171
Triangle number 19 is 190
Triangle number 20 is 210
Triangle number 21 is 231
Triangle number 22 is 253
Triangle number 23 is 20
Triangle number 24 is 44
Triangle number 25 is 69
Triangle number 26 is 95
Triangle number 27 is 122
Triangle number 28 is 150
Triangle number 29 is 179
Triangle number 30 is 209`

func cleanOutput(out string) []string {
	lines := strings.Split(out, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Allow debug comments starting with '#' that do not affect output comparison.
			continue
		}
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

var (
	reCodeSize = regexp.MustCompile(`\[m6809 codesize:\s*(\d+)\]`)
	reCycles   = regexp.MustCompile(`\[hatvan-vm finished:\s*(\d+)\s*total cycles executed\]`)

	telemetryMu   sync.Mutex
	telemetryOnce sync.Once
	telemetryPath string
)

func getTelemetryPath() string {
	telemetryOnce.Do(func() {
		if os.Getenv("TELEMETRY") == "0" || os.Getenv("TELEMETRY") == "false" {
			return
		}
		if path := os.Getenv("TELEMETRY_FILE"); path != "" {
			telemetryPath = path
			return
		}
		telemetryDir := "telemetry"
		if fi, err := os.Stat(telemetryDir); err == nil && fi.IsDir() {
			label := os.Getenv("TELEMETRY_LABEL")
			if label == "" {
				label = os.Getenv("TELEMETRY")
			}
			if label == "1" || label == "true" {
				label = ""
			}
			if label == "" {
				label = "phase-one-complete"
			}
			now := time.Now().Format("2006-01-02-150405")
			telemetryPath = filepath.Join(telemetryDir, fmt.Sprintf("perf-%s-%s", now, label))
		}
	})
	return telemetryPath
}

func recordTelemetry(testName, variant string, codeSize int, runCycles uint64) {
	path := getTelemetryPath()
	if path == "" {
		return
	}
	telemetryMu.Lock()
	defer telemetryMu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return
	}
	defer f.Close()

	if codeSize > 0 {
		fmt.Fprintf(f, "%s.%s.codesize %d\n", testName, variant, codeSize)
	}
	if runCycles > 0 {
		fmt.Fprintf(f, "%s.%s.runcycles %d\n", testName, variant, runCycles)
	}
}

func testBackend(t *testing.T, backend, sourceFile, expectedStr string, expectCompileError, expectRunError bool) {
	testBackendVariant(t, backend, "default", nil, sourceFile, expectedStr, expectCompileError, expectRunError)
}

func testBackendVariant(t *testing.T, backend, variant string, extraArgs []string, sourceFile, expectedStr string, expectCompileError, expectRunError bool) {
	variantDir := backend + "_" + variant + "_" + filepath.Base(sourceFile) + ".dir"
	tmpDir := filepath.Join("_tmp", variantDir)
	os.MkdirAll(tmpDir, 0777)

	ext := ".c"
	if backend == "amd64" || backend == "x86_64" {
		ext = ".s"
	}
	if backend == "m6809" {
		ext = ".asm"
	}
	t.Logf("TempDir is %q", tmpDir)
	midFile := filepath.Join(tmpDir, "out"+ext)
	exeFile := filepath.Join(tmpDir, "out.exe")

	// Compile demo file using minigolf
	compiler := filepath.Join("_tmp", fmt.Sprintf("minigolf.%d", os.Getpid()))
	_, err := os.Stat(compiler)
	if err != nil {
		exec.Command("go", "build", "-o", compiler, "main.go").Run()
	}

	args := []string{"-m=" + backend, "-o", midFile, "-I=tests", "-I=c-tests", "-I=demos", "-I=demos/floating", "-I=golflib"}
	args = append(args, extraArgs...)
	args = append(args, sourceFile)

	cmd := exec.Command(compiler, args...)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return // Success: compilation failed as expected
		}
		t.Fatalf("Failed to compile with %q -m=%s: %v\nOutput: %s", compiler, backend, err, out)
	} else if expectCompileError {
		t.Fatalf("Expected compile error for %q -m=%s but compilation succeeded. Output: %s", compiler, backend, out)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	switch backend {
	case "m6809":
		cmd = exec.Command("sh", "run9.sh", midFile)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		t.Logf("Running: %v", cmd)
		if err := cmd.Run(); err != nil {
			if expectRunError {
				return // Success: execution failed as expected
			}
			t.Fatalf("Failed to compile for backend %s: %v\nStderr: %s", backend, err, stderr.String())
		} else if expectRunError {
			t.Fatalf("Expected run error for backend %s but execution succeeded", backend)
		}

	default:
		// Compile generated code with gcc
		cmd = exec.Command("gcc", "-g", "-o", exeFile, midFile)
		t.Logf("Running: %v", cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to compile C code with gcc for backend %s: %v\nOutput: %s", backend, err, out)
		}

		// Run the executable
		cmd = exec.Command(exeFile)
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			if expectRunError {
				return // Success: execution failed as expected
			}
			t.Fatalf("Failed to run executable for backend %s: %v", backend, err)
		} else if expectRunError {
			t.Fatalf("Expected run error for backend %s but execution succeeded", backend)
		}
	}

	out := stdout.String()
	actualLines := cleanOutput(out)
	expectedLines := cleanOutput(expectedStr)

	actual := strings.Join(actualLines, ";")
	expected := strings.Join(expectedLines, ";")

	if actual != expected {
		t.Errorf("Backend %s output mismatch.\nGot %d lines:\n%q\n\nWanted %d lines:\n%q",
			backend, len(actualLines), actual, len(expectedLines), expected)
	} else if backend == "m6809" && !expectCompileError && !expectRunError {
		codeSize := 0
		var runCycles uint64

		stderrStr := stderr.String()
		if m := reCodeSize.FindStringSubmatch(stderrStr); len(m) > 1 {
			codeSize, _ = strconv.Atoi(m[1])
		}
		if m := reCycles.FindStringSubmatch(stderrStr); len(m) > 1 {
			runCycles, _ = strconv.ParseUint(m[1], 10, 64)
		}

		testName := filepath.Base(sourceFile)
		testName = strings.TrimPrefix(testName, "c_test_")
		testName = strings.TrimSuffix(testName, ".golf")
		testName = strings.TrimSuffix(testName, ".c")

		recordTelemetry(testName, variant, codeSize, runCycles)
	}
}

func TestSystemTriangles_CBE(t *testing.T) {
	testBackend(t, "CBE", "demos/triangles.golf", expectedOutput, false, false)
}

func TestSystemTrianglesByte_CBE(t *testing.T) {
	testBackend(t, "CBE", "demos/triangles_byte.golf", expectedOutputByte, false, false)
}

func TestSystemTriangles_amd64(t *testing.T) {
	testBackend(t, "amd64", "demos/triangles.golf", expectedOutput, false, false)
}

func TestSystemTrianglesByte_amd64(t *testing.T) {
	testBackend(t, "amd64", "demos/triangles_byte.golf", expectedOutputByte, false, false)
}

func TestSystemTriangles_m6809(t *testing.T) {
	testBackend(t, "m6809", "demos/triangles.golf", expectedOutput, false, false)
}

func TestSystemTrianglesByte_m6809(t *testing.T) {
	testBackend(t, "m6809", "demos/triangles_byte.golf", expectedOutputByte, false, false)
}

var m6809Variants = []struct {
	name string
	args []string
}{
	{"default", nil},
	{"globals-at-y", []string{"-globals-at-y"}},
	{"frame-pointer", []string{"-frame-pointer"}},
	{"pic", []string{"-pic"}},
	{"globals-at-y-frame-pointer", []string{"-globals-at-y", "-frame-pointer"}},
	{"globals-at-y-pic", []string{"-globals-at-y", "-pic"}},
	{"frame-pointer-pic", []string{"-frame-pointer", "-pic"}},
	{"globals-at-y-frame-pointer-pic", []string{"-globals-at-y", "-frame-pointer", "-pic"}},
}

func TestSystemTelemetryVariants_m6809(t *testing.T) {
	for _, v := range m6809Variants {
		v := v
		t.Run(v.name, func(t *testing.T) {
			testBackendVariant(t, "m6809", v.name, v.args, "demos/triangles.golf", expectedOutput, false, false)
		})
	}
}

func TestSystemAllVariants_m6809(t *testing.T) {
	runFlag := ""
	if f := flag.Lookup("test.run"); f != nil {
		runFlag = f.Value.String()
	}
	if os.Getenv("ALL_VARIANTS") == "" && os.Getenv("TELEMETRY_VARIANTS") == "" && !strings.Contains(runFlag, "AllVariants") {
		t.Skip("skipping all 7 variants test; set ALL_VARIANTS=1 or TELEMETRY_VARIANTS=1 to run")
	}

	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}

	otherVariants := m6809Variants[1:]

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.golf") || strings.HasSuffix(file, "_nomoto.golf") {
			continue
		}

		expectCompileError := strings.HasSuffix(file, ".error.golf")
		expectRunError := strings.HasSuffix(file, ".panic.golf")

		var expectedStr string
		if !expectCompileError && !expectRunError {
			wantFile := strings.TrimSuffix(file, ".golf") + ".want"
			wantBytes, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("Failed to read want file %s: %v", wantFile, err)
			}
			expectedStr = string(wantBytes)
		}

		for _, v := range otherVariants {
			v := v
			testName := fmt.Sprintf("%s_%s", filepath.Base(file), v.name)
			t.Run(testName, func(t *testing.T) {
				testBackendVariant(t, "m6809", v.name, v.args, file, expectedStr, expectCompileError, expectRunError)
			})
		}
	}
}

func TestSystemAllGolfFiles(t *testing.T) {
	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}

	backends := []string{"CBE", "amd64", "m6809"}

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.golf") {
			continue // Skip known broken/wip files
		}

		expectCompileError := strings.HasSuffix(file, ".error.golf")
		expectRunError := strings.HasSuffix(file, ".panic.golf")

		var expectedStr string
		if !expectCompileError && !expectRunError {
			wantFile := strings.TrimSuffix(file, ".golf") + ".want"
			wantBytes, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("Failed to read want file %s: %v", wantFile, err)
			}
			expectedStr = string(wantBytes)
		}

		for _, backend := range backends {
			if strings.HasSuffix(file, "_nomoto.golf") && backend == "m6809" {
				continue
			}
			testName := fmt.Sprintf("%s_%s", filepath.Base(file), backend)
			t.Run(testName, func(t *testing.T) {
				testBackend(t, backend, file, expectedStr, expectCompileError, expectRunError)
			})
		}
	}
}

func Value[T any](val T, err error) T {
	if err != nil {
		log.Panicf("Failure within Value(%T, err): %v", val, err)
	}
	return val
}
