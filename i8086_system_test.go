package main_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	i8086BuildOnce sync.Once
	i8086Compiler  string
	i8086Assembler string
)

func buildI8086Tools(t *testing.T) (compiler, assembler string) {
	t.Helper()
	i8086BuildOnce.Do(func() {
		os.MkdirAll("_tmp", 0777)
		i8086Compiler = filepath.Join("_tmp", fmt.Sprintf("minigolf.%d", os.Getpid()))
		if out, err := exec.Command("go", "build", "-o", i8086Compiler, "main.go").CombinedOutput(); err != nil {
			t.Fatalf("Failed to build minigolf: %v\n%s", err, out)
		}
		i8086Assembler = filepath.Join("_tmp", fmt.Sprintf("asm8086.%d", os.Getpid()))
		if out, err := exec.Command("go", "build", "-o", i8086Assembler, "./cmd/asm8086").CombinedOutput(); err != nil {
			t.Fatalf("Failed to build asm8086: %v\n%s", err, out)
		}
	})
	return i8086Compiler, i8086Assembler
}

func verifyI8086DECB(t *testing.T, decb []byte, wantDataStart uint16) {
	t.Helper()
	if len(decb) < 10 {
		t.Fatalf("DECB too short: %d bytes", len(decb))
	}
	if !bytes.Equal(decb[:5], []byte{0xFD, 0x00, 0x00, 'x', '0'}) {
		t.Fatalf("Invalid Extended DECB header: %x", decb[:5])
	}
	idx := 5
	curSeg := uint16(0xFFFF)
	sawCode := false
	sawData := false
	firstDataAddr := int(-1)
	sawTrailer := false

	for idx < len(decb) {
		recType := decb[idx]
		switch recType {
		case 0xFE:
			if idx+5 > len(decb) {
				t.Fatalf("Truncated 0xFE record at %d", idx)
			}
			curSeg = binary.BigEndian.Uint16(decb[idx+3 : idx+5])
			idx += 5
		case 0x00:
			if idx+5 > len(decb) {
				t.Fatalf("Truncated 0x00 record at %d", idx)
			}
			length := int(binary.BigEndian.Uint16(decb[idx+1 : idx+3]))
			addr := binary.BigEndian.Uint16(decb[idx+3 : idx+5])
			idx += 5
			if idx+length > len(decb) {
				t.Fatalf("Truncated 0x00 payload at %d (len %d)", idx, length)
			}
			switch curSeg {
			case 0x0001:
				sawCode = true
			case 0x0000:
				sawData = true
				if firstDataAddr < 0 {
					firstDataAddr = int(addr)
				}
			default:
				t.Fatalf("Unexpected segment High16 $%04X in 8086 DECB", curSeg)
			}
			idx += length
		case 0xFF:
			if idx+5 > len(decb) {
				t.Fatalf("Truncated 0xFF record at %d", idx)
			}
			if curSeg != 0x0001 {
				t.Fatalf("Expected High16 $0001 active before 0xFF END/EXEC, got $%04X", curSeg)
			}
			entry := binary.BigEndian.Uint16(decb[idx+3 : idx+5])
			if entry != 0x0200 {
				t.Fatalf("Expected entry $0200, got $%04X", entry)
			}
			sawTrailer = true
			idx += 5
		default:
			t.Fatalf("Unexpected DECB record type $%02X at %d", recType, idx)
		}
	}
	if !sawCode {
		t.Fatalf("DECB missing SEGMENT CODE ($0001) records")
	}
	if !sawTrailer {
		t.Fatalf("DECB missing 0xFF trailer")
	}
	if wantDataStart != 0 && sawData && uint16(firstDataAddr) != wantDataStart {
		t.Fatalf("Expected first DATA record at $%04X, got $%04X", wantDataStart, firstDataAddr)
	}
}

func compileAndAssembleI8086(t *testing.T, sourceFile string, extraArgs []string, expectCompileError bool, wantDataStart uint16) {
	compiler, assembler := buildI8086Tools(t)

	tmpDir := filepath.Join("_tmp", "i8086_"+filepath.Base(sourceFile)+".dir")
	os.MkdirAll(tmpDir, 0777)
	asmFile := filepath.Join(tmpDir, "out.asm")
	decbFile := filepath.Join(tmpDir, "out.decb")
	listFile := filepath.Join(tmpDir, "out.list")

	args := []string{"-m=8086", "-o", asmFile, "-I=tests", "-I=c-tests", "-I=c-demos", "-I=c-demos/floating", "-I=c-demos/pythonsub", "-I=demos", "-I=demos/floating", "-I=golflib"}
	args = append(args, extraArgs...)
	args = append(args, sourceFile)

	cmd := exec.Command(compiler, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if expectCompileError {
			return
		}
		t.Fatalf("minigolf -m=8086 failed on %s: %v\n%s", sourceFile, err, out)
	} else if expectCompileError {
		t.Fatalf("Expected compile error for %s, but succeeded", sourceFile)
	}

	asmCmd := exec.Command(assembler, "-o", decbFile, "-l", listFile, "i8086/cstart.asm", asmFile)
	if out, err := asmCmd.CombinedOutput(); err != nil {
		t.Fatalf("asm8086 failed on %s: %v\n%s", sourceFile, err, out)
	}

	decb, err := os.ReadFile(decbFile)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", decbFile, err)
	}
	verifyI8086DECB(t, decb, wantDataStart)
}

func TestI8086CompileAllGolfFiles(t *testing.T) {
	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, ".bad.golf") {
			continue
		}
		expectCompileError := strings.HasSuffix(file, ".error.golf")
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			compileAndAssembleI8086(t, file, nil, expectCompileError, 0x0200)
		})
	}
}

func TestI8086CompileAllCFiles(t *testing.T) {
	files, err := filepath.Glob("c-tests/*.c")
	if err != nil {
		t.Fatalf("Failed to glob c-tests/*.c: %v", err)
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			compileAndAssembleI8086(t, file, nil, false, 0x0200)
		})
	}
}

func TestI8086GlobalVarOffset(t *testing.T) {
	compileAndAssembleI8086(t, "demos/triangles.golf", []string{"-global_var_offset", "1024"}, false, 0x0400)
	compileAndAssembleI8086(t, "demos/triangles_byte.golf", []string{"-global_var_offset", "1024"}, false, 0x0400)
}

func hasGep0() bool {
	candidates := []string{
		"../hatvan-os/build/gep0",
		"../hatvan-os/gep0",
		"../hatvan-os/cmd/gep0",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

func TestSystemTriangles_8086(t *testing.T) {
	if !hasGep0() {
		t.Skip("gep0 not yet present in ../hatvan-os; skipping 8086 VM execution test")
	}
	testBackend(t, "8086", "demos/triangles.golf", expectedOutput, false, false)
}

func TestSystemTrianglesByte_8086(t *testing.T) {
	if !hasGep0() {
		t.Skip("gep0 not yet present in ../hatvan-os; skipping 8086 VM execution test")
	}
	testBackend(t, "8086", "demos/triangles_byte.golf", expectedOutputByte, false, false)
}

func TestSystemGolfFiles_8086(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 8086 system tests in short mode")
	}
	if !hasGep0() {
		t.Skip("gep0 not yet present in ../hatvan-os; skipping 8086 VM execution test")
	}
	files, err := filepath.Glob("tests/*.golf")
	if err != nil {
		t.Fatalf("Failed to glob tests/*.golf: %v", err)
	}
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
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			testBackend(t, "8086", file, expectedStr, expectCompileError, expectRunError)
		})
	}
}
