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

func getPar3ImportDir() string {
	for _, cand := range []string{"par3-lib", "minigolf/par3-lib", "../par3-lib", "np-lib", "minigolf/np-lib", "../np-lib"} {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return "par3-lib"
}

func getPar3RuntimeDir() string {
	for _, cand := range []string{"par3-runtime", "minigolf/par3-runtime", "../par3-runtime", "np-runtime", "minigolf/np-runtime", "../np-runtime"} {
		if fi, err := os.Stat(filepath.Join(cand, "npasm.py")); err == nil && !fi.IsDir() {
			return cand
		}
	}
	return "par3-runtime"
}

func getPar3TestsDir() string {
	for _, cand := range []string{"par3-tests", "minigolf/par3-tests", "../par3-tests", "np-tests", "minigolf/np-tests", "../np-tests"} {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return "par3-tests"
}

func cleanPar3Output(out string) []string {
	lines := strings.Split(out, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func testPar3Backend(t *testing.T, sourceFile, expectedStr string, expectCompileError, expectRunError bool) {
	stem := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(sourceFile), ".par3"), ".golf")
	variantDir := "par3_" + stem + ".dir"
	tmpDir := filepath.Join("_tmp", variantDir)
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		t.Fatalf("Failed to create tmpDir %s: %v", tmpDir, err)
	}

	midFile := filepath.Join(tmpDir, "out.p3a")
	p3pFile := filepath.Join(tmpDir, "out.p3p")

	compiler := getMinigolfCompiler(t)
	importDir := getPar3ImportDir()
	runtimeDir := getPar3RuntimeDir()

	// 1. Compile with minigolf -m=par3 -I=<importDir>
	args := []string{"-m=par3", "-o", midFile, "-I=" + importDir, sourceFile}
	cmd := exec.Command(compiler, args...)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return // Success: compilation failed as expected
		}
		t.Fatalf("Failed to compile with %q -m=par3: %v\nOutput: %s", compiler, err, out)
	} else if expectCompileError {
		t.Fatalf("Expected compile error for %q -m=par3 but compilation succeeded. Output: %s", compiler, out)
	}

	// 2. Assemble with npasm.py
	npasmPy := filepath.Join(runtimeDir, "npasm.py")
	cmd = exec.Command("python3", npasmPy, midFile, "-o", p3pFile)
	t.Logf("Running: %v", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return
		}
		t.Fatalf("Failed to assemble %s with npasm.py: %v\nOutput: %s", midFile, err, out)
	}

	// 3. Execute with npvm.py
	npvmPy := filepath.Join(runtimeDir, "npvm.py")
	cmd = exec.Command("python3", npvmPy, p3pFile)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	t.Logf("Running: %v", cmd)
	if err := cmd.Run(); err != nil {
		if expectRunError {
			return // Success: execution failed as expected
		}
		t.Fatalf("Failed to run %s with npvm.py: %v\nStderr: %s\nStdout: %s", p3pFile, err, stderr.String(), stdout.String())
	} else if expectRunError {
		t.Fatalf("Expected run error for %s but execution succeeded", sourceFile)
	}

	// 4. Compare output
	out := stdout.String()
	actualLines := cleanPar3Output(out)
	expectedLines := cleanPar3Output(expectedStr)

	actual := strings.Join(actualLines, ";")
	expected := strings.Join(expectedLines, ";")

	if actual != expected {
		t.Errorf("Par3 backend output mismatch for %s.\nGot %d lines:\n%s\n\nWanted %d lines:\n%s",
			sourceFile, len(actualLines), strings.Join(actualLines, "\n"), len(expectedLines), strings.Join(expectedLines, "\n"))
	}
}

func TestPar3System(t *testing.T) {
	testsDir := getPar3TestsDir()
	files, err := filepath.Glob(filepath.Join(testsDir, "*.par3"))
	if err != nil {
		t.Fatalf("Failed to glob %s/*.par3: %v", testsDir, err)
	}
	if len(files) == 0 {
		// Fallback to .golf
		files, _ = filepath.Glob(filepath.Join(testsDir, "*.golf"))
	}
	if len(files) == 0 {
		t.Fatalf("No *.par3 or *.golf files found in %s", testsDir)
	}

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.par3") || strings.HasSuffix(file, ".bad.golf") {
			continue // Skip known broken/wip files
		}

		expectCompileError := strings.HasSuffix(file, ".error.par3") || strings.HasSuffix(file, ".error.golf")
		expectRunError := strings.HasSuffix(file, ".panic.par3") || strings.HasSuffix(file, ".panic.golf")

		var expectedStr string
		if !expectCompileError && !expectRunError {
			wantFile := strings.TrimSuffix(strings.TrimSuffix(file, ".par3"), ".golf") + ".want"
			wantBytes, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("Failed to read want file %s: %v", wantFile, err)
			}
			expectedStr = string(wantBytes)
		}

		testName := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(file), ".par3"), ".golf")
		targetFile := file
		exp := expectedStr
		t.Run(testName, func(t *testing.T) {
			testPar3Backend(t, targetFile, exp, expectCompileError, expectRunError)
		})
	}
}

// TestPar3AsMiniGolf verifies that .par3 input files are accepted by minigolf as valid MiniGolf source code.
func TestPar3AsMiniGolf(t *testing.T) {
	testsDir := getPar3TestsDir()
	files, err := filepath.Glob(filepath.Join(testsDir, "*.par3"))
	if err != nil || len(files) == 0 {
		t.Fatalf("No *.par3 files found in %s", testsDir)
	}

	compiler := getMinigolfCompiler(t)
	importDir := getPar3ImportDir()

	for _, file := range files {
		if strings.HasSuffix(file, ".bad.par3") || strings.HasSuffix(file, ".error.par3") || strings.HasSuffix(file, ".panic.par3") {
			continue
		}
		testName := strings.TrimSuffix(filepath.Base(file), ".par3")
		t.Run(testName, func(t *testing.T) {
			tmpOut := filepath.Join("_tmp", "mg_"+testName+".ast")
			cmd := exec.Command(compiler, "-m=ast", "-o", tmpOut, "-I="+importDir, file)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Failed to parse .par3 file %s with minigolf -m=ast: %v\nOutput: %s", file, err, out)
			}
		})
	}
}

func TestPar3NamedReturnRejected(t *testing.T) {
	compiler := getMinigolfCompiler(t)
	importDir := getPar3ImportDir()

	tmpDir := filepath.Join("_tmp", "named_ret_reject")
	_ = os.MkdirAll(tmpDir, 0777)

	// 1. Test named return parameter rejection
	badFile := filepath.Join(tmpDir, "bad_named.par3")
	badCode := "package main\n\nfunc compute(x word) (n word) {\n    n = x + 1\n    return n\n}\n\nfunc main() {}\n"
	if err := os.WriteFile(badFile, []byte(badCode), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(compiler, "-m=par3", "-o", filepath.Join(tmpDir, "bad.p3a"), "-I="+importDir, badFile)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Expected compilation to fail for named return parameter, but succeeded. Output:\n%s", out)
	}
	if !strings.Contains(string(out), "named return parameters are not supported in Par3") {
		t.Fatalf("Expected error message 'named return parameters are not supported in Par3', got:\n%s", out)
	}

	// 2. Test bare return in non-void function rejection
	badBareFile := filepath.Join(tmpDir, "bad_bare.par3")
	badBareCode := "package main\n\nfunc getVal() word {\n    return\n}\n\nfunc main() {}\n"
	if err := os.WriteFile(badBareFile, []byte(badBareCode), 0644); err != nil {
		t.Fatal(err)
	}

	cmd2 := exec.Command(compiler, "-m=par3", "-o", filepath.Join(tmpDir, "bad_bare.p3a"), "-I="+importDir, badBareFile)
	out2, err2 := cmd2.CombinedOutput()
	if err2 == nil {
		t.Fatalf("Expected compilation to fail for bare return, but succeeded. Output:\n%s", out2)
	}
	if !strings.Contains(string(out2), "bare return is not supported in Par3") {
		t.Fatalf("Expected error message 'bare return is not supported in Par3', got:\n%s", out2)
	}
}
