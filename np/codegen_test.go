package np_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/np"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/semantic"
)

var npcodeDir = func() string {
	if p, err := filepath.Abs("../np-runtime"); err == nil {
		if _, err := os.Stat(filepath.Join(p, "npasm.py")); err == nil {
			return p
		}
	}
	return "/home/strick/github.com/strickyak/minigolf/np-runtime"
}()

func compileGolfToNP(t *testing.T, src string) string {
	tokens := lexer.Lex(src, "test.golf")
	p := parser.New(tokens)
	prog := p.ParseProgram("main")
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	resolver := semantic.NewResolver(nil)
	resolver.Resolve(prog)

	gen := np.New()
	return gen.Generate(prog)
}

func roundTripAndRun(t *testing.T, asmSource string, expectedOutput string) {
	tmpDir := t.TempDir()
	asmPath := filepath.Join(tmpDir, "test.npasm")
	npcPath := filepath.Join(tmpDir, "test.npc")
	disAsmPath := filepath.Join(tmpDir, "test_re.npasm")
	npcRePath := filepath.Join(tmpDir, "test_re.npc")

	if err := os.WriteFile(asmPath, []byte(asmSource), 0644); err != nil {
		t.Fatalf("failed to write asm: %v", err)
	}

	// 1. Assemble: npasm.py test.npasm -o test.npc
	npasmPy := filepath.Join(npcodeDir, "npasm.py")
	cmd := exec.Command("python3", npasmPy, asmPath, "-o", npcPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("npasm.py failed: %v\nOutput:\n%s\nAssembly Source:\n%s", err, string(out), asmSource)
	}

	// 2. Disassemble: npdis.py test.npc --asm -o test_re.npasm
	npdisPy := filepath.Join(npcodeDir, "npdis.py")
	cmd = exec.Command("python3", npdisPy, npcPath, "--asm", "-o", disAsmPath)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("npdis.py failed: %v\nOutput:\n%s", err, string(out))
	}

	// 3. Re-assemble: npasm.py test_re.npasm -o test_re.npc
	cmd = exec.Command("python3", npasmPy, disAsmPath, "-o", npcRePath)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("re-assembling disAsm failed: %v\nOutput:\n%s", err, string(out))
	}

	// 4. Verify byte-for-byte binary match
	b1, err := os.ReadFile(npcPath)
	if err != nil {
		t.Fatalf("failed to read test.npc: %v", err)
	}
	b2, err := os.ReadFile(npcRePath)
	if err != nil {
		t.Fatalf("failed to read test_re.npc: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("round-trip mismatch: %d bytes vs %d bytes", len(b1), len(b2))
	}

	// 5. Execute in npvm.py
	if expectedOutput != "" {
		npvmPy := filepath.Join(npcodeDir, "npvm.py")
		cmd = exec.Command("python3", npvmPy, npcPath)
		vmOut, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("npvm.py execution failed: %v\nOutput:\n%s", err, string(vmOut))
		}
		if !strings.Contains(string(vmOut), expectedOutput) {
			t.Fatalf("expected VM output to contain %q, got:\n%s", expectedOutput, string(vmOut))
		}
	}
}

func TestBasicArithmeticAndControlFlow(t *testing.T) {
	src := `package main

var g_total word

func square(x word) word {
	return x * x
}

func sum_to(n word) word {
	total := 0
	for i := 1; i <= n; i++ {
		total += i
	}
	return total
}

func main() {
	a := 5
	b := square(a)
	g_total = sum_to(a)

	if b == 25 {
		println("square OK")
	}
	if g_total == 15 {
		println("sum OK")
	}
	sys_exit(0)
}
`
	asm := compileGolfToNP(t, src)
	roundTripAndRun(t, asm, "square OK\nsum OK")
}

func TestStringOperationsAndSlicing(t *testing.T) {
	src := `package main

func main() {
	s := "  NitrOS-9 operating system\r\n"
	r := rstrip(s, "\r\n")
	if startswith(r, "  ") {
		println("prefix OK")
	}
	pos := find(r, "operating", 0)
	if pos >= 0 {
		println("find OK")
		word := r[pos : pos+9]
		println(word)
	}
	ren := replace_ident("LDA DP,X", "DP", "MY_DP")
	println(ren)
	sys_exit(0)
}
`
	asm := compileGolfToNP(t, src)
	roundTripAndRun(t, asm, "prefix OK\nfind OK\noperating\nLDA MY_DP,X")
}

func TestSmapAndDict(t *testing.T) {
	src := `package main

func main() {
	syms := smap.New[word](8)
	syms.Insert("Alpha", 10)
	syms.Insert("Beta", 20)

	val, ok := syms.Lookup("Alpha")
	if ok {
		if val == 10 {
			println("Alpha val OK")
		}
	}

	if syms.Len() == 2 {
		println("syms len OK")
	}
	sys_exit(0)
}
`
	asm := compileGolfToNP(t, src)
	roundTripAndRun(t, asm, "Alpha val OK\nsyms len OK")
}

func TestSwitchStatement(t *testing.T) {
	src := `package main

func classify(x word) {
	switch x {
	case 1:
		println("one")
	case 2, 3:
		println("two or three")
	default:
		println("other")
	}
}

func main() {
	classify(1)
	classify(2)
	classify(3)
	classify(99)
	sys_exit(0)
}
`
	asm := compileGolfToNP(t, src)
	roundTripAndRun(t, asm, "one\ntwo or three\ntwo or three\nother")
}

func TestPanicOnUnsupportedDefer(t *testing.T) {
	src := `package main

func main() {
	defer println("cleanup")
	println("hello")
}
`
	tokens := lexer.Lex(src, "defer_test.golf")
	p := parser.New(tokens)
	prog := p.ParseProgram("main")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected compiler to panic on defer statement, but it succeeded")
		}
		errMsg := r.(string)
		if !strings.Contains(errMsg, "defer") {
			t.Fatalf("expected panic message to mention defer, got: %s", errMsg)
		}
	}()

	gen := np.New()
	gen.Generate(prog)
}
