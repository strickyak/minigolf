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

func TestPar3GoBootstrapAndSelfCompile(t *testing.T) {
	tmpDir := filepath.Join("_tmp", "par3_bootstrap_test")
	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		t.Fatalf("Failed to create tmpDir %s: %v", tmpDir, err)
	}

	// 1. Bootstrap minigolf_par3 with Go
	goCompilerBin := filepath.Join(tmpDir, "minigolf_par3_go")
	cmd := exec.Command("go", "build", "-o", goCompilerBin, "par3-apps/par3comp.par3.and.go", "par3-apps/par3comp_gosupport.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to bootstrap minigolf_par3 with Go: %v\nOutput: %s", err, string(out))
	}

	// 2. Compile all Par3 toolchain apps with the Go-bootstrapped compiler
	tools := []struct {
		name string
		src  string
	}{
		{"minigolf_par3", "par3-apps/par3comp.par3.and.go"},
		{"par3asm", "par3-apps/par3asm.par3"},
		{"par3pack", "par3-apps/par3pack.par3"},
		{"selfgen_preprocess", "par3-apps/selfgen_preprocess.par3"},
		{"build_img", "par3-apps/build_img.par3"},
	}

	par3asmPy := filepath.Join("par3-runtime", "par3asm.py")
	for _, tool := range tools {
		p3aPath := filepath.Join(tmpDir, tool.name+".p3a")
		cmd := exec.Command(goCompilerBin, tool.src, "-o", p3aPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to compile %s with Go-bootstrapped compiler: %v\nOutput: %s", tool.src, err, string(out))
		}

		// Convert CR -> LF for par3asm.py and assemble to .p3p
		rawBytes, err := os.ReadFile(p3aPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", p3aPath, err)
		}
		lfPath := filepath.Join(tmpDir, tool.name+".lf.p3a")
		lfBytes := bytes.ReplaceAll(rawBytes, []byte("\r"), []byte("\n"))
		if err := os.WriteFile(lfPath, lfBytes, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", lfPath, err)
		}

		p3pPath := filepath.Join(tmpDir, tool.name+".p3p")
		asmCmd := exec.Command("python3", par3asmPy, lfPath, "-o", p3pPath)
		if out, err := asmCmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to assemble %s with par3asm.py: %v\nOutput: %s", lfPath, err, string(out))
		}
	}

	// 3. Run minigolf_par3.p3p under par3vm.py to self-compile par3comp.par3.and.go
	par3vmPy := filepath.Join("par3-runtime", "par3vm.py")
	compilerP3p := filepath.Join(tmpDir, "minigolf_par3.p3p")
	selfP3a := filepath.Join(tmpDir, "self_minigolf_par3.p3a")
	vmCmd := exec.Command("python3", par3vmPy, compilerP3p, "--", "par3-apps/par3comp.par3.and.go", "-o", selfP3a)
	if out, err := vmCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to self-compile par3comp.par3.and.go on par3vm.py: %v\nOutput: %s", err, string(out))
	}

	// 4. Verify byte-for-byte identity between Go-bootstrapped .p3a and VM self-compiled .p3a
	stage1Bytes, err := os.ReadFile(filepath.Join(tmpDir, "minigolf_par3.p3a"))
	if err != nil {
		t.Fatalf("Failed to read stage1 .p3a: %v", err)
	}
	stage2Bytes, err := os.ReadFile(selfP3a)
	if err != nil {
		t.Fatalf("Failed to read stage2 .p3a: %v", err)
	}
	if !bytes.Equal(stage1Bytes, stage2Bytes) {
		t.Fatalf("Stage 1 (Go-bootstrapped) and Stage 2 (VM self-compiled) minigolf_par3.p3a differ! (%d vs %d bytes)", len(stage1Bytes), len(stage2Bytes))
	}

	// 5. Verify par3asm.p3p assembles minigolf_par3.p3a identically to par3asm.py
	par3asmP3p := filepath.Join(tmpDir, "par3asm.p3p")
	selfP3p := filepath.Join(tmpDir, "self_minigolf_par3.p3p")
	asmVmCmd := exec.Command("python3", par3vmPy, par3asmP3p, "--", selfP3a, "-o", selfP3p)
	if out, err := asmVmCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to assemble minigolf_par3.p3a with par3asm.p3p on par3vm.py: %v\nOutput: %s", err, string(out))
	}
	stage1P3pBytes, err := os.ReadFile(compilerP3p)
	if err != nil {
		t.Fatalf("Failed to read stage1 .p3p: %v", err)
	}
	stage2P3pBytes, err := os.ReadFile(selfP3p)
	if err != nil {
		t.Fatalf("Failed to read stage2 .p3p: %v", err)
	}
	if !bytes.Equal(stage1P3pBytes, stage2P3pBytes) {
		t.Fatalf("par3asm.py and par3asm.p3p produced different minigolf_par3.p3p binaries! (%d vs %d bytes)", len(stage1P3pBytes), len(stage2P3pBytes))
	}
}

func TestPar3AsMiniGolf(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("par3-apps", "*.par3"))
	if err != nil || len(files) == 0 {
		t.Fatalf("No *.par3 files found in par3-apps")
	}
	files = append(files, filepath.Join("par3-apps", "par3comp.par3.and.go"))

	if err := os.MkdirAll("_tmp", 0777); err != nil {
		t.Fatalf("Failed to create _tmp: %v", err)
	}
	compiler := filepath.Join("_tmp", "minigolf_ast_test")
	buildCmd := exec.Command("go", "build", "-o", compiler, "main.go")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build minigolf: %v\nOutput: %s", err, string(out))
	}

	for _, file := range files {
		testName := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(file), ".par3"), ".par3.and.go")
		t.Run(testName, func(t *testing.T) {
			tmpOut := filepath.Join("_tmp", "mg_"+testName+".ast")
			cmd := exec.Command(compiler, "-m=ast", "-o", tmpOut, file)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Failed to parse .par3 file %s with minigolf -m=ast: %v\nOutput: %s", file, err, out)
			}
		})
	}
}
