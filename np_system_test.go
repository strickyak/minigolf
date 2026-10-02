package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func getMinigolfCompiler(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll("_tmp", 0777); err != nil {
		t.Fatalf("Failed to create _tmp dir: %v", err)
	}
	compiler := filepath.Join("_tmp", fmt.Sprintf("minigolf.%d", os.Getpid()))
	if _, err := os.Stat(compiler); err != nil {
		cmd := exec.Command("go", "build", "-o", compiler, "main.go")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to build minigolf compiler: %v\nOutput: %s", err, string(out))
		}
	}
	return compiler
}

func getNPImportDir() string {
	for _, cand := range []string{"np-lib", "minigolf/np-lib", "../np-lib"} {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return "np-lib"
}

func getNPRuntimeDir() string {
	for _, cand := range []string{"np-runtime", "minigolf/np-runtime", "../np-runtime"} {
		if fi, err := os.Stat(filepath.Join(cand, "npasm.py")); err == nil && !fi.IsDir() {
			return cand
		}
	}
	return "np-runtime"
}

func getNPTestsDir() string {
	for _, cand := range []string{"np-tests", "minigolf/np-tests", "../np-tests"} {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return "np-tests"
}

func testNPBackend(t *testing.T, sourceFile, expectedStr string, expectCompileError, expectRunError bool) {
	stem := strings.TrimSuffix(filepath.Base(sourceFile), ".golf")
	variantDir := "np_" + stem + ".dir"
	tmpDir := filepath.Join("_tmp", variantDir)
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		t.Fatalf("Failed to create tmpDir %s: %v", tmpDir, err)
	}

	midFile := filepath.Join(tmpDir, "out.npasm")
	npcFile := filepath.Join(tmpDir, "out.npc")

	compiler := getMinigolfCompiler(t)
	importDir := getNPImportDir()
	runtimeDir := getNPRuntimeDir()

	// 1. Compile with minigolf -m=np -I=<importDir>
	args := []string{"-m=np", "-o", midFile, "-I=" + importDir, sourceFile}
	cmd := exec.Command(compiler, args...)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return // Success: compilation failed as expected
		}
		t.Fatalf("Failed to compile with %q -m=np: %v\nOutput: %s", compiler, err, out)
	} else if expectCompileError {
		t.Fatalf("Expected compile error for %q -m=np but compilation succeeded. Output: %s", compiler, out)
	}

	// 2. Assemble with npasm.py
	npasmPy := filepath.Join(runtimeDir, "npasm.py")
	cmd = exec.Command("python3", npasmPy, midFile, "-o", npcFile)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return
		}
		t.Fatalf("Failed to assemble %s with npasm.py: %v\nOutput: %s", midFile, err, out)
	}

	// 3. Execute with npvm.py
	npvmPy := filepath.Join(runtimeDir, "npvm.py")
	cmd = exec.Command("python3", npvmPy, npcFile)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	t.Logf("Running: %v", cmd)
	if err := cmd.Run(); err != nil {
		if expectRunError {
			return // Success: execution failed as expected
		}
		t.Fatalf("Failed to run %s with npvm.py: %v\nStderr: %s\nStdout: %s", npcFile, err, stderr.String(), stdout.String())
	} else if expectRunError {
		t.Fatalf("Expected run error for %s but execution succeeded", sourceFile)
	}

	// 4. Compare output using cleanOutput from system_test.go
	out := stdout.String()
	actualLines := cleanOutput(out)
	expectedLines := cleanOutput(expectedStr)

	actual := strings.Join(actualLines, ";")
	expected := strings.Join(expectedLines, ";")

	if actual != expected {
		t.Errorf("NP backend output mismatch for %s.\nGot %d lines:\n%s\n\nWanted %d lines:\n%s",
			sourceFile, len(actualLines), strings.Join(actualLines, "\n"), len(expectedLines), strings.Join(expectedLines, "\n"))
	}
}

func TestNPSystem(t *testing.T) {
	testsDir := getNPTestsDir()
	files, err := filepath.Glob(filepath.Join(testsDir, "*.golf"))
	if err != nil {
		t.Fatalf("Failed to glob %s/*.golf: %v", testsDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("No *.golf files found in %s", testsDir)
	}

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

		testName := strings.TrimSuffix(filepath.Base(file), ".golf")
		targetFile := file
		exp := expectedStr
		t.Run(testName, func(t *testing.T) {
			testNPBackend(t, targetFile, exp, expectCompileError, expectRunError)
		})
	}
}
