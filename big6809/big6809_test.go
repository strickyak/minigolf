package big6809_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runEmbiggenTest(t *testing.T, golfRelPath, wantRelPath string) {
	golfFile := filepath.Join("..", golfRelPath)
	wantFile := filepath.Join("..", wantRelPath)

	wantBytes, err := os.ReadFile(wantFile)
	if err != nil {
		t.Fatalf("Failed to read want file %s: %v", wantFile, err)
	}
	expectedOutput := string(wantBytes)

	tmpDir, err := os.MkdirTemp("", "embiggen_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	asmPath := filepath.Join(tmpDir, "test.asm")
	decbPath := filepath.Join(tmpDir, "test.decb")

	// 1. Compile with minigolf -m=big6809
	mainGo := filepath.Join("..", "main.go")
	cmdCompile := exec.Command("go", "run", mainGo,
		"-m=big6809",
		"-o="+asmPath,
		"-I="+filepath.Join("..", "biggolflib"),
		"-I="+filepath.Join("..", "golflib"),
		"-I="+filepath.Join("..", "tests"),
		"-I="+filepath.Join("..", "demos"),
		golfFile,
	)
	if out, err := cmdCompile.CombinedOutput(); err != nil {
		t.Fatalf("Compilation failed: %v\nOutput: %s", err, string(out))
	}

	// 2. Check for asm6809 assembler
	asmBin := filepath.Join("..", "asm6809")
	if _, err := os.Stat(asmBin); err != nil {
		t.Skipf("asm6809 not found at %s: %v", asmBin, err)
	}

	cmdAsm := exec.Command(asmBin, "--decb", "-o", decbPath, asmPath)
	if out, err := cmdAsm.CombinedOutput(); err != nil {
		t.Fatalf("Assembly failed: %v\nOutput: %s", err, string(out))
	}

	// 3. Check for gep9 VM
	gep9Bin := filepath.Join("..", "..", "hatvan-os", "build", "gep9")
	if _, err := os.Stat(gep9Bin); err != nil {
		t.Skipf("gep9 emulator not found at %s; assembly succeeded", gep9Bin)
	}

	cmdVM := exec.Command(gep9Bin, "--embiggen", "--hypercalls", decbPath)
	out, err := cmdVM.CombinedOutput()
	if err != nil {
		t.Fatalf("gep9 execution failed: %v\nOutput: %s", err, string(out))
	}

	actualOutput := string(out)
	if strings.TrimSpace(actualOutput) != strings.TrimSpace(expectedOutput) {
		t.Fatalf("Output mismatch!\nGot:\n%s\nWant:\n%s", actualOutput, expectedOutput)
	}
}

func TestEMBIGGEN_TestDefines(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_defines.golf"), filepath.Join("tests", "test_defines.want"))
}

func TestEMBIGGEN_TestFarFunc(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_far_func.golf"), filepath.Join("tests", "test_far_func.want"))
}

func TestEMBIGGEN_TestFor3(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_for3.golf"), filepath.Join("tests", "test_for3.want"))
}

func TestEMBIGGEN_TestArithmetic(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_arithmetic.golf"), filepath.Join("tests", "test_arithmetic.want"))
}

func TestEMBIGGEN_TestAPLPrimesNomoto(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_apl_primes_nomoto.golf"), filepath.Join("tests", "test_apl_primes_nomoto.want"))
}

func TestEMBIGGEN_TestAPLNomoto(t *testing.T) {
	runEmbiggenTest(t, filepath.Join("tests", "test_apl_nomoto.golf"), filepath.Join("tests", "test_apl_nomoto.want"))
}


