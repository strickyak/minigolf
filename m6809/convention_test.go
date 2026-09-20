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

func TestAdaptivePolicy(t *testing.T) {
	ptrType := ir.Type{Name: "*int", Bits: ir.TypeBitPointer}

	// Leaf 1: clear_buf(buf *int) -> pointer only, should get X
	clearBuf := &ir.Function{
		Name: "clear_buf",
		Parameters: []*ir.Parameter{
			{ID: 1, Name: "buf", Typ: ptrType},
		},
		ReturnType: ir.TypeVoid,
		Blocks: []*ir.BasicBlock{
			{
				ID: 0,
				Instructions: []ir.Instruction{
					&ir.StorePtr{
						BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeVoid},
						Ptr:             &ir.Parameter{ID: 1, Name: "buf", Typ: ptrType},
						Val:             &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord}, Val: 0},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	// Leaf 2: add_one(n int) int -> scalar arithmetic, should get D
	addOne := &ir.Function{
		Name: "add_one",
		Parameters: []*ir.Parameter{
			{ID: 10, Name: "n", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeWord,
		Blocks: []*ir.BasicBlock{
			{
				ID: 0,
				Instructions: []ir.Instruction{
					&ir.BinaryOp{
						BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord},
						Op:              "add",
						Left:            &ir.Parameter{ID: 10, Name: "n", Typ: ir.TypeWord},
						Right:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Val: 1},
					},
				},
				Terminator: &ir.Return{
					Val: &ir.Parameter{ID: 10, Name: "n", Typ: ir.TypeWord},
				},
			},
		},
	}

	// Leaf 3: sort_array(arr *int, n int) -> arr is pointer (wants X), n is scalar (wants D)
	sortArray := &ir.Function{
		Name: "sort_array",
		Parameters: []*ir.Parameter{
			{ID: 20, Name: "arr", Typ: ptrType},
			{ID: 21, Name: "n", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeVoid,
		Blocks: []*ir.BasicBlock{
			{
				ID: 0,
				Instructions: []ir.Instruction{
					&ir.LoadPtr{
						BaseInstruction: ir.BaseInstruction{ID: 22, Typ: ir.TypeWord},
						Ptr:             &ir.Parameter{ID: 20, Name: "arr", Typ: ptrType},
					},
					&ir.Compare{
						BaseInstruction: ir.BaseInstruction{ID: 23, Typ: ir.TypeBool},
						Op:              "<",
						Left:            &ir.Parameter{ID: 21, Name: "n", Typ: ir.TypeWord},
						Right:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 24, Typ: ir.TypeWord}, Val: 10},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	// Caller (Level 2): wrapper(arr *int, n int) -> calls sort_array(arr, n)
	// Interpolation point: wrapper should inherit arr in X and n in D!
	wrapper := &ir.Function{
		Name: "wrapper",
		Parameters: []*ir.Parameter{
			{ID: 30, Name: "arr", Typ: ptrType},
			{ID: 31, Name: "n", Typ: ir.TypeWord},
		},
		ReturnType: ir.TypeVoid,
		Blocks: []*ir.BasicBlock{
			{
				ID: 0,
				Instructions: []ir.Instruction{
					&ir.Call{
						BaseInstruction: ir.BaseInstruction{ID: 32, Typ: ir.TypeVoid},
						Func:            sortArray,
						Args: []ir.Value{
							&ir.Parameter{ID: 30, Name: "arr", Typ: ptrType},
							&ir.Parameter{ID: 31, Name: "n", Typ: ir.TypeWord},
						},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	prog := &ir.Program{
		Functions: []*ir.Function{clearBuf, addOne, sortArray, wrapper},
	}

	b := New(false, false, false)
	b.SetConventionPolicy(&AdaptivePolicy{})
	b.initConventions(prog)

	// 1. clearBuf: 1 param (pointer) -> X
	cClear := b.getFunctionConvention(clearBuf)
	if !cClear.IsFastcall || len(cClear.Params) != 1 || cClear.Params[0].Reg != "x" {
		t.Errorf("clearBuf should have param 0 in X, got: %v", cClear)
	}

	// 2. addOne: 1 param (scalar) -> D
	cAdd := b.getFunctionConvention(addOne)
	if !cAdd.IsFastcall || len(cAdd.Params) != 1 || cAdd.Params[0].Reg != "d" {
		t.Errorf("addOne should have param 0 in D, got: %v", cAdd)
	}

	// 3. sortArray: arr in X, n in D
	cSort := b.getFunctionConvention(sortArray)
	if !cSort.IsFastcall || len(cSort.Params) != 2 || cSort.Params[0].Reg != "x" || cSort.Params[1].Reg != "d" {
		t.Errorf("sortArray should have (X, D), got: %v", cSort)
	}

	// 4. wrapper: calls sortArray(arr, n) -> inherits arr in X, n in D
	cWrap := b.getFunctionConvention(wrapper)
	if !cWrap.IsFastcall || len(cWrap.Params) != 2 || cWrap.Params[0].Reg != "x" || cWrap.Params[1].Reg != "d" {
		t.Errorf("wrapper should inherit (X, D) via interpolation, got: %v", cWrap)
	}
}
