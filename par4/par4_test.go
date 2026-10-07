package par4_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strickyak/minigolf/par4"
)

func TestPar4BasicExecution(t *testing.T) {
	src := `
; Global variable
.global count: 2

; Function: main
.function main entry
    .local i: word
    .local limit: word

    PUSH_0
    STORE_LOCAL i
    PUSH_I8 5
    STORE_LOCAL limit

.L_loop:
    LOAD_LOCAL i
    LOAD_LOCAL limit
    CMP_GE
    JUMP_IF_TRUE .L_exit

    ; Print string
    PUSH_STR "Loop iteration"
    IO_PRINT

    ; i++
    LOAD_LOCAL i
    PUSH_1
    ADD
    STORE_LOCAL i
    JUMP .L_loop

.L_exit:
    PUSH_STR "Done!"
    IO_PRINT
    RET_VOID
.endfunction
`
	bin, err := par4.Assemble(src)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	vm, err := par4.NewVM(bin)
	if err != nil {
		t.Fatalf("NewVM failed: %v", err)
	}

	var stdout bytes.Buffer
	vm.Stdout = &stdout

	exitCode, err := vm.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("Expected exit code 0, got %d", exitCode)
	}

	expected := "Loop iteration\nLoop iteration\nLoop iteration\nLoop iteration\nLoop iteration\nDone!\n"
	if stdout.String() != expected {
		t.Fatalf("Output mismatch:\nGot:\n%s\nWanted:\n%s", stdout.String(), expected)
	}
}

func TestPar4HatvanFileSyscalls(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_io.txt")

	src := fmt.Sprintf(`
.function main entry
    .local path_str: string
    .local fd: word
    .local write_str: string
    .local read_buf: 64
    .local bytes_read: word

    ; 1. Create file using I$Create ($83)
    ; Stack on entry: [A, B, X, Y, U]
    PUSH_I8 3                       ; A = mode (3 = read/write)
    PUSH_0                          ; B = 0
    PUSH_STR %q
    POP                             ; pop len
    POP                             ; pop cap (leaving string ptr at X)
    PUSH_0                          ; Y = 0
    PUSH_0                          ; U = 0
    HATVAN_TRAP I$Create            ; Syscall $83
    ; Stack on exit: [res_A, res_B, res_Y, err]
    POP                             ; pop err
    POP                             ; pop res_Y
    POP                             ; pop res_B
    STORE_LOCAL fd                  ; save path ID in fd

    ; 2. Write line using I$WritLn ($8C)
    ; Stack on entry: [A, B, X, Y, U]
    LOAD_LOCAL fd                   ; A = path ID
    PUSH_0                          ; B = 0
    PUSH_STR "Hello Hatvan OS from Par4!"
    STORE_LOCAL bytes_read          ; bytes_read = length
    POP                             ; pop cap
                                    ; X = ptr is now on stack
    LOAD_LOCAL bytes_read           ; Y = length
    PUSH_0                          ; U = 0
    HATVAN_TRAP I$WritLn
    POP                             ; err
    POP                             ; res_Y
    POP                             ; res_B
    POP                             ; res_A

    ; 3. Close file using I$Close ($8F)
    LOAD_LOCAL fd                   ; A = path ID
    PUSH_0                          ; B = 0
    PUSH_0                          ; X = 0
    PUSH_0                          ; Y = 0
    PUSH_0                          ; U = 0
    HATVAN_TRAP I$Close
    POP                             ; err
    POP                             ; res_Y
    POP                             ; res_B
    POP                             ; res_A

    ; 4. Reopen file using I$Open ($84)
    PUSH_I8 1                       ; A = mode (1 = read)
    PUSH_0                          ; B = 0
    PUSH_STR %q
    POP                             ; pop len
    POP                             ; pop cap (leaving ptr at X)
    PUSH_0                          ; Y = 0
    PUSH_0                          ; U = 0
    HATVAN_TRAP I$Open
    POP                             ; err
    POP                             ; res_Y
    POP                             ; res_B
    STORE_LOCAL fd                  ; save path ID in fd

    ; 5. Read line using I$ReadLn ($8B)
    LOAD_LOCAL fd                   ; A = path ID
    PUSH_0                          ; B = 0
    ADDR_OF_LOCAL read_buf          ; X = buffer address
    PUSH_I16 64                     ; Y = max bytes
    PUSH_0                          ; U = 0
    HATVAN_TRAP I$ReadLn
    POP                             ; err
    STORE_LOCAL bytes_read          ; res_Y = bytes read
    POP                             ; res_B
    POP                             ; res_A

    ; Print read string slice
    ADDR_OF_LOCAL read_buf
    LOAD_LOCAL bytes_read
    LOAD_LOCAL bytes_read
    SLICE_NEW
    IO_PRINT

    ; 6. Close file again
    LOAD_LOCAL fd
    PUSH_0
    PUSH_0
    PUSH_0
    PUSH_0
    HATVAN_TRAP I$Close
    POP
    POP
    POP
    POP

    RET_VOID
.endfunction
`, testFile, testFile)

	bin, err := par4.Assemble(src)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	vm, err := par4.NewVM(bin)
	if err != nil {
		t.Fatalf("NewVM failed: %v", err)
	}

	var stdout bytes.Buffer
	vm.Stdout = &stdout

	exitCode, err := vm.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("Expected exit code 0, got %d", exitCode)
	}

	// Verify file content on disk: WritLn produced Unix \n
	fileData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file written by VM: %v", err)
	}
	expectedFileContent := "Hello Hatvan OS from Par4!\n"
	if string(fileData) != expectedFileContent {
		t.Fatalf("File data mismatch:\nGot: %q\nWanted: %q", string(fileData), expectedFileContent)
	}

	// Verify VM output
	expectedOutput := "Hello Hatvan OS from Par4!\n\n" // ReadLn read line ending in \n, IO_PRINT added \n
	if stdout.String() != expectedOutput {
		t.Fatalf("VM stdout mismatch:\nGot: %q\nWanted: %q", stdout.String(), expectedOutput)
	}
}

func TestPar4Disassembler(t *testing.T) {
	src := `
.global counter: 2

.function addTwo entry
    .param x: 2
    .param y: 2
    .local res: 2

    LOAD_LOCAL x
    LOAD_LOCAL y
    ADD
    STORE_LOCAL res
    LOAD_LOCAL res
    RET
.endfunction
`
	bin, err := par4.Assemble(src)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	disText, err := par4.Disassemble(bin)
	if err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}

	if !strings.Contains(disText, ".function main entry") && !strings.Contains(disText, ".function addTwo entry") {
		t.Fatalf("Disassembly missing function header:\n%s", disText)
	}
	if !strings.Contains(disText, "ADD") || !strings.Contains(disText, "RET") {
		t.Fatalf("Disassembly missing instructions:\n%s", disText)
	}

	// Re-assemble disassembled text to verify validity
	bin2, err := par4.Assemble(disText)
	if err != nil {
		t.Fatalf("Re-assembling disassembled text failed: %v\nDisassembly:\n%s", err, disText)
	}
	if len(bin2) == 0 {
		t.Fatalf("Re-assembled binary is empty")
	}
}

func TestPar4StringEscapesAndComments(t *testing.T) {
	src := `
.function main entry
    ; String with embedded semicolon and hash, plus trailing comment
    PUSH_STR "Hello; world #1! fix\'d\nNext line" ; this is a real comment
    IO_PRINT
    RET_VOID
.endfunction
`
	bin, err := par4.Assemble(src)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	vm, err := par4.NewVM(bin)
	if err != nil {
		t.Fatalf("NewVM failed: %v", err)
	}

	var stdout bytes.Buffer
	vm.Stdout = &stdout

	exitCode, err := vm.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("Expected exit code 0, got %d", exitCode)
	}

	expected := "Hello; world #1! fix'd\nNext line\n"
	if stdout.String() != expected {
		t.Fatalf("Output mismatch:\nGot: %q\nWanted: %q", stdout.String(), expected)
	}
}
