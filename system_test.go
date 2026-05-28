package main_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func testBackend(t *testing.T, backend, sourceFile, expectedStr string) {
	tmpDir := filepath.Join("_tmp", backend+"_"+filepath.Base(sourceFile)+".dir")
	os.MkdirAll(tmpDir, 0777)

	ext := ".c"
	if backend == "x86_64" {
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

	cmd := exec.Command(compiler, "-m="+backend, "-o", midFile, "-I=golflib", sourceFile)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile with %q -m=%s: %v\nOutput: %s", compiler, backend, err, out)
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
			t.Fatalf("Failed to compile for backend %s: %v\nStderr: %s", backend, err, stderr.String())
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
			t.Fatalf("Failed to run executable for backend %s: %v", backend, err)
		}
	}

	out := stdout.String()
	actualLines := cleanOutput(out)
	expectedLines := cleanOutput(expectedStr)

	// t.Logf("actual: %q", out)
	// t.Logf("wanted: %q", expectedLines)

	// t.Logf("actual: %dx %s", len(actualLines), actualLines)
	// t.Logf("wanted: %dx %s", len(expectedLines), expectedLines)

	//if len(actualLines) < len(expectedLines) {
	//t.Fatalf("Backend %s output too short. Expected at least %d lines, got %d", backend, len(expectedLines), len(actualLines))
	//}

	// // Truncate actual lines to length of expected lines (since triangles_byte limit is 100 but we only check first 30)
	// actualLines = actualLines[:len(expectedLines)]

	actual := strings.Join(actualLines, ";")
	expected := strings.Join(expectedLines, ";")

	if actual != expected {
		t.Errorf("Backend %s output mismatch.\nGot %d lines:\n%q\n\nWanted %d lines:\n%q",
			backend, len(actualLines), actual, len(expectedLines), expected)
	}
}

func TestSystemTriangles_CBE(t *testing.T) {
	testBackend(t, "CBE", "demos/triangles.golf", expectedOutput)
}

func TestSystemTrianglesByte_CBE(t *testing.T) {
	testBackend(t, "CBE", "demos/triangles_byte.golf", expectedOutputByte)
}

func TestSystemTriangles_x86_64(t *testing.T) {
	testBackend(t, "x86_64", "demos/triangles.golf", expectedOutput)
}

func TestSystemTrianglesByte_x86_64(t *testing.T) {
	testBackend(t, "x86_64", "demos/triangles_byte.golf", expectedOutputByte)
}

func TestSystemAllGolfFiles(t *testing.T) {
	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}

	backends := []string{"CBE", "x86_64", "m6809"}

	for _, file := range files {
		wantFile := strings.TrimSuffix(file, ".golf") + ".want"
		wantBytes, err := os.ReadFile(wantFile)
		if err != nil {
			t.Fatalf("Failed to read want file %s: %v", wantFile, err)
		}
		expectedStr := string(wantBytes)

		for _, backend := range backends {
			if strings.HasSuffix(file, "_nomoto.golf") && backend == "m6809" {
				continue
			}
			testName := fmt.Sprintf("%s_%s", filepath.Base(file), backend)
			t.Run(testName, func(t *testing.T) {
				testBackend(t, backend, file, expectedStr)
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
