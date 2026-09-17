package m6809

import (
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestConventionPolicies(t *testing.T) {
	b := New(false, false, false)

	// Function 1: 1 word parameter: f(x int) int
	f1 := &ir.Function{
		Name: "f_single_word",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "x", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	// Function 2: 2 word parameters: f(a int, b int) int
	f2 := &ir.Function{
		Name: "f_two_words",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "a", Typ: ir.TypeWord},
			{ID: 2, Name: "b", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	// Function 3: 3 parameters: f(a int, b int, c int) int
	f3 := &ir.Function{
		Name: "f_three_words",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "a", Typ: ir.TypeWord},
			{ID: 2, Name: "b", Typ: ir.TypeWord},
			{ID: 3, Name: "c", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	// Function 4: 1 byte parameter: f(c byte) void
	f4 := &ir.Function{
		Name: "f_single_byte",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "c", Typ: ir.TypeByte},
		},
		ReturnType: ir.TypeVoid,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	// Function 5: External linkage function
	f5 := &ir.Function{
		Name:    "f_external",
		Linkage: "my_c_func",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "x", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	// Function 6: Address-taken function
	f6 := &ir.Function{
		Name: "f_addrof",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "x", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}
	b.funcAddressTaken["f_addrof"] = true

	// --- 1. Test StackPolicy ---
	stackPol := &StackPolicy{}
	cStack := stackPol.GetConvention(f2, b)
	if cStack.IsFastcall {
		t.Errorf("StackPolicy should not be fastcall")
	}
	if len(cStack.Params) != 2 || cStack.Params[0].Kind != LocStack || cStack.Params[1].Kind != LocStack {
		t.Errorf("StackPolicy should pass all params on stack, got: %v", cStack)
	}
	if cStack.TotalStackArgBytes != 4 {
		t.Errorf("Expected 4 bytes on stack, got %d", cStack.TotalStackArgBytes)
	}

	// --- 2. Test FastcallPolicy ---
	fastPol := &FastcallPolicy{}

	// f1: 1 word in D
	c1 := fastPol.GetConvention(f1, b)
	if !c1.IsFastcall || c1.NumRegParams != 1 || c1.Params[0].Kind != LocReg || c1.Params[0].Reg != "d" {
		t.Errorf("Fastcall for single word should be in D, got: %v", c1)
	}
	if c1.TotalStackArgBytes != 0 {
		t.Errorf("Single word fastcall should use 0 stack bytes, got %d", c1.TotalStackArgBytes)
	}

	// f4: 1 byte in B
	c4 := fastPol.GetConvention(f4, b)
	if !c4.IsFastcall || c4.NumRegParams != 1 || c4.Params[0].Kind != LocReg || c4.Params[0].Reg != "b" {
		t.Errorf("Fastcall for single byte should be in B, got: %v", c4)
	}
	if c4.TotalStackArgBytes != 0 {
		t.Errorf("Single byte fastcall should use 0 stack bytes, got %d", c4.TotalStackArgBytes)
	}

	// f2: 2 words in (D, X)
	c2 := fastPol.GetConvention(f2, b)
	if !c2.IsFastcall || c2.NumRegParams != 2 || c2.Params[0].Reg != "d" || c2.Params[1].Reg != "x" {
		t.Errorf("Fastcall for two words should be in (D, X), got: %v", c2)
	}
	if c2.TotalStackArgBytes != 0 {
		t.Errorf("Two words fastcall should use 0 stack bytes, got %d", c2.TotalStackArgBytes)
	}

	// f3: 3 words in (D, X, stack)
	c3 := fastPol.GetConvention(f3, b)
	if !c3.IsFastcall || c3.NumRegParams != 2 || c3.Params[0].Reg != "d" || c3.Params[1].Reg != "x" || c3.Params[2].Kind != LocStack {
		t.Errorf("Fastcall for three words should be in (D, X, stack), got: %v", c3)
	}
	if c3.TotalStackArgBytes != 2 {
		t.Errorf("Three words fastcall should use 2 stack bytes, got %d", c3.TotalStackArgBytes)
	}

	// f5: External linkage falls back to stack
	c5 := fastPol.GetConvention(f5, b)
	if c5.IsFastcall || c5.NumRegParams != 0 || c5.Params[0].Kind != LocStack {
		t.Errorf("External linkage function should fall back to stack, got: %v", c5)
	}

	// f6: Address-taken function falls back to stack
	c6 := fastPol.GetConvention(f6, b)
	if c6.IsFastcall || c6.NumRegParams != 0 || c6.Params[0].Kind != LocStack {
		t.Errorf("Address-taken function should fall back to stack, got: %v", c6)
	}

	// --- 3. Test GCCPolicy ---
	gccPol := &GCCPolicy{}
	cGcc := gccPol.GetConvention(f2, b)
	if !cGcc.IsFastcall || cGcc.Params[0].Reg != "x" || cGcc.Params[1].Kind != LocStack {
		t.Errorf("GCC convention for 2 words should pass 1st in X, rest on stack, got: %v", cGcc)
	}
}

func TestFastcallAssemblyEmission(t *testing.T) {
	callee := &ir.Function{
		Name: "callee",
		Parameters: []*ir.Parameter{
			{ID: 10, Name: "val", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{ID: 0},
		},
	}

	caller := &ir.Function{
		Name:       "caller",
		ReturnType: ir.TypeVoid,
		Blocks: []*ir.BasicBlock{
			{
				ID: 0,
				Instructions: []ir.Instruction{
					&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 42},
					&ir.Call{
						BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord},
						Func:            callee,
						Args: []ir.Value{
							&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 42},
						},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	prog := &ir.Program{
		Functions: []*ir.Function{caller, callee},
	}

	// 1. Fastcall enabled (default)
	bFast := New(false, false, false)
	asmFast := bFast.Generate(prog)

	// Caller should not push d, and should not pop with leas
	if strings.Contains(asmFast, "pshs d\t; push arg 0") {
		t.Errorf("Fastcall caller should not push arg 0 with pshs d, got:\n%s", asmFast)
	}
	if strings.Contains(asmFast, "leas 2,s\t; pop 2 bytes args") {
		t.Errorf("Fastcall caller should not pop 2 bytes args with leas, got:\n%s", asmFast)
	}
	if !strings.Contains(asmFast, "std 0,s\t; save param 'val'") {
		t.Errorf("Fastcall callee should save param 'val' with std 0,s, got:\n%s", asmFast)
	}

	// 2. Stack policy (Fastcall disabled via SetConventionPolicy or -no-fastcall6809)
	bStack := New(false, false, false)
	bStack.SetConventionPolicy(&StackPolicy{})
	asmStack := bStack.Generate(prog)

	if !strings.Contains(asmStack, "pshs d\t; push arg 0") {
		t.Errorf("Stack policy caller should push arg 0 with pshs d, got:\n%s", asmStack)
	}
	if !strings.Contains(asmStack, "leas 2,s\t; pop 2 bytes args") {
		t.Errorf("Stack policy caller should pop 2 bytes args with leas, got:\n%s", asmStack)
	}
}
