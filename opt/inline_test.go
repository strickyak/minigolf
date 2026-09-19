package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestInlineMaxTinyThreshold(t *testing.T) {
	// Callee: add_three(a, b) -> a + b + 1
	paramA := &ir.Parameter{ID: 1, Name: "a", Typ: ir.TypeWord}
	paramB := &ir.Parameter{ID: 2, Name: "b", Typ: ir.TypeWord}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord}, Val: 1}
	op1 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeWord}, Op: "add", Left: paramA, Right: paramB}
	op2 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeWord}, Op: "add", Left: op1, Right: c1}
	ret := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 6, Typ: ir.TypeWord}, Val: op2}

	calleeBlock := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{c1, op1, op2},
		Terminator:   ret,
	}
	callee := &ir.Function{
		Name:       "add_three",
		Parameters: []*ir.Parameter{paramA, paramB},
		Blocks:     []*ir.BasicBlock{calleeBlock},
	}

	buildProgram := func() (*ir.Program, *ir.Function, *ir.Function) {
		// Caller 1: call add_three
		call1 := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord},
			Func:            callee,
			Args: []ir.Value{
				&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord}, Val: 10},
				&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Val: 20},
			},
		}
		caller1Block := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{call1},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeWord}, Val: call1},
		}
		caller1 := &ir.Function{
			Name:   "caller1",
			Blocks: []*ir.BasicBlock{caller1Block},
		}

		// Caller 2: call add_three (so call count is 2, not a single call site)
		call2 := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord},
			Func:            callee,
			Args: []ir.Value{
				&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 21, Typ: ir.TypeWord}, Val: 30},
				&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 22, Typ: ir.TypeWord}, Val: 40},
			},
		}
		caller2Block := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{call2},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 23, Typ: ir.TypeWord}, Val: call2},
		}
		caller2 := &ir.Function{
			Name:   "caller2",
			Blocks: []*ir.BasicBlock{caller2Block},
		}

		prog := &ir.Program{
			Functions: []*ir.Function{caller1, caller2, callee},
		}
		return prog, caller1, caller2
	}

	// Case 1: MaxTinyInstructions = 2 (instruction count of callee is 3: c1, op1, op2)
	// Should NOT inline because 3 > 2.
	prog1, caller1_1, _ := buildProgram()
	changed1 := InlinePass(prog1, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    false,
		MaxTinyInstructions: 2,
		MaxInlineRounds:     10,
	})
	if changed1 {
		t.Errorf("Expected no inlining when MaxTinyInstructions=2 < 3, but changed was true")
	}
	if _, isCall := caller1_1.Blocks[0].Instructions[0].(*ir.Call); !isCall {
		t.Errorf("Expected call to remain in caller1 when MaxTinyInstructions=2")
	}

	// Case 2: MaxTinyInstructions = 5 (callee instr count is 3 <= 5)
	// SHOULD inline.
	prog2, caller1_2, _ := buildProgram()
	changed2 := InlinePass(prog2, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    false,
		MaxTinyInstructions: 5,
		MaxInlineRounds:     10,
	})
	if !changed2 {
		t.Errorf("Expected inlining when MaxTinyInstructions=5 >= 3, but changed was false")
	}
	for _, instr := range caller1_2.Blocks[0].Instructions {
		if _, isCall := instr.(*ir.Call); isCall {
			t.Errorf("Expected call to be eliminated via inlining in caller1")
		}
	}
}

func TestInlineSingleCallsite(t *testing.T) {
	// Callee: 4 instructions
	paramA := &ir.Parameter{ID: 1, Name: "a", Typ: ir.TypeWord}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 1}
	op1 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord}, Op: "add", Left: paramA, Right: c1}
	op2 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeWord}, Op: "add", Left: op1, Right: c1}
	op3 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeWord}, Op: "add", Left: op2, Right: c1}
	ret := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 6, Typ: ir.TypeWord}, Val: op3}

	calleeBlock := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{c1, op1, op2, op3},
		Terminator:   ret,
	}
	callee := &ir.Function{
		Name:       "single_callee",
		Parameters: []*ir.Parameter{paramA},
		Blocks:     []*ir.BasicBlock{calleeBlock},
	}

	buildProgram := func() (*ir.Program, *ir.Function) {
		call := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord},
			Func:            callee,
			Args: []ir.Value{
				&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord}, Val: 100},
			},
		}
		callerBlock := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{call},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Val: call},
		}
		caller := &ir.Function{
			Name:   "caller",
			Blocks: []*ir.BasicBlock{callerBlock},
		}
		prog := &ir.Program{
			Functions: []*ir.Function{caller, callee},
		}
		return prog, caller
	}

	// When EnableSingleCall is disabled and EnableTiny is disabled -> no inlining
	prog1, caller1 := buildProgram()
	changed1 := InlinePass(prog1, InlineOptions{
		EnableTiny:          false,
		EnableSingleCall:    false,
		MaxTinyInstructions: 8,
		MaxInlineRounds:     10,
	})
	if changed1 {
		t.Errorf("Expected no inlining when EnableSingleCall and EnableTiny are false")
	}
	if _, isCall := caller1.Blocks[0].Instructions[0].(*ir.Call); !isCall {
		t.Errorf("Expected Call instruction to remain")
	}

	// When EnableSingleCall is enabled -> callee has 1 call site, inlined!
	prog2, caller2 := buildProgram()
	changed2 := InlinePass(prog2, InlineOptions{
		EnableTiny:          false,
		EnableSingleCall:    true,
		MaxTinyInstructions: 1, // Callee has 4 instructions, so tiny inlining would fail
		MaxInlineRounds:     10,
	})
	if !changed2 {
		t.Errorf("Expected inlining when EnableSingleCall=true for single-site function")
	}
	for _, instr := range caller2.Blocks[0].Instructions {
		if _, isCall := instr.(*ir.Call); isCall {
			t.Errorf("Expected Call instruction to be inlined")
		}
	}
}

func TestInlineDiamondCFG(t *testing.T) {
	// Callee: min(a, b) -> if a < b return a else return b
	paramA := &ir.Parameter{ID: 1, Name: "a", Typ: ir.TypeWord}
	paramB := &ir.Parameter{ID: 2, Name: "b", Typ: ir.TypeWord}

	cBlk0 := &ir.BasicBlock{ID: 100}
	cBlk1 := &ir.BasicBlock{ID: 101}
	cBlk2 := &ir.BasicBlock{ID: 102}

	cmp := &ir.Compare{
		BaseInstruction: ir.BaseInstruction{ID: 103, Typ: ir.TypeByte},
		Op:              "lt",
		Left:            paramA,
		Right:           paramB,
	}
	br := &ir.Branch{
		BaseInstruction: ir.BaseInstruction{ID: 104, Typ: ir.TypeVoid},
		Condition:       cmp,
		TrueBlock:       cBlk1,
		FalseBlock:      cBlk2,
	}
	cBlk0.Instructions = []ir.Instruction{cmp}
	cBlk0.Terminator = br
	cBlk0.Successors = []*ir.BasicBlock{cBlk1, cBlk2}

	ret1 := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 105, Typ: ir.TypeWord}, Val: paramA}
	cBlk1.Instructions = []ir.Instruction{}
	cBlk1.Terminator = ret1
	cBlk1.Predecessors = []*ir.BasicBlock{cBlk0}

	ret2 := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 106, Typ: ir.TypeWord}, Val: paramB}
	cBlk2.Instructions = []ir.Instruction{}
	cBlk2.Terminator = ret2
	cBlk2.Predecessors = []*ir.BasicBlock{cBlk0}

	callee := &ir.Function{
		Name:       "min_func",
		Parameters: []*ir.Parameter{paramA, paramB},
		Blocks:     []*ir.BasicBlock{cBlk0, cBlk1, cBlk2},
	}

	// Caller has 2 calls to min_func (so not single call site)
	callerParamX := &ir.Parameter{ID: 200, Name: "x", Typ: ir.TypeWord}
	callerParamY := &ir.Parameter{ID: 201, Name: "y", Typ: ir.TypeWord}

	call1 := &ir.Call{
		BaseInstruction: ir.BaseInstruction{ID: 202, Typ: ir.TypeWord},
		Func:            callee,
		Args:            []ir.Value{callerParamX, callerParamY},
	}
	callerBlk1 := &ir.BasicBlock{
		ID:           203,
		Instructions: []ir.Instruction{call1},
		Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 204, Typ: ir.TypeWord}, Val: call1},
	}
	caller1 := &ir.Function{
		Name:       "caller1",
		Parameters: []*ir.Parameter{callerParamX, callerParamY},
		Blocks:     []*ir.BasicBlock{callerBlk1},
	}

	call2 := &ir.Call{
		BaseInstruction: ir.BaseInstruction{ID: 205, Typ: ir.TypeWord},
		Func:            callee,
		Args:            []ir.Value{callerParamX, callerParamY},
	}
	callerBlk2 := &ir.BasicBlock{
		ID:           206,
		Instructions: []ir.Instruction{call2},
		Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 207, Typ: ir.TypeWord}, Val: call2},
	}
	caller2 := &ir.Function{
		Name:       "caller2",
		Parameters: []*ir.Parameter{callerParamX, callerParamY},
		Blocks:     []*ir.BasicBlock{callerBlk2},
	}

	prog := &ir.Program{
		Functions: []*ir.Function{caller1, caller2, callee},
	}

	changed := InlinePass(prog, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    false,
		MaxTinyInstructions: 8,
		MaxInlineRounds:     10,
	})

	if !changed {
		t.Fatalf("Expected diamond CFG inlining to succeed")
	}

	// Verify call1 is removed from caller1
	for _, b := range caller1.Blocks {
		for _, instr := range b.Instructions {
			if _, isCall := instr.(*ir.Call); isCall {
				t.Errorf("Expected call to be inlined, found remaining call in caller1 block %d", b.ID)
			}
		}
	}

	// Verify that a Phi node was created at the join point
	foundPhi := false
	for _, b := range caller1.Blocks {
		for _, instr := range b.Instructions {
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				foundPhi = true
				if len(phi.Edges) != 2 {
					t.Errorf("Expected join Phi to have 2 edges, got %d", len(phi.Edges))
				}
			}
		}
	}
	if !foundPhi {
		t.Errorf("Expected Phi instruction at join block after diamond inlining")
	}
}

func TestInlineDiamondFoldConstant(t *testing.T) {
	// Callee: min(a, b) -> if a < b return a else return b
	paramA := &ir.Parameter{ID: 1, Name: "a", Typ: ir.TypeWord}
	paramB := &ir.Parameter{ID: 2, Name: "b", Typ: ir.TypeWord}

	cBlk0 := &ir.BasicBlock{ID: 100}
	cBlk1 := &ir.BasicBlock{ID: 101}
	cBlk2 := &ir.BasicBlock{ID: 102}

	cmp := &ir.Compare{
		BaseInstruction: ir.BaseInstruction{ID: 103, Typ: ir.TypeByte},
		Op:              "lt",
		Left:            paramA,
		Right:           paramB,
	}
	br := &ir.Branch{
		BaseInstruction: ir.BaseInstruction{ID: 104, Typ: ir.TypeVoid},
		Condition:       cmp,
		TrueBlock:       cBlk1,
		FalseBlock:      cBlk2,
	}
	cBlk0.Instructions = []ir.Instruction{cmp}
	cBlk0.Terminator = br
	cBlk0.Successors = []*ir.BasicBlock{cBlk1, cBlk2}

	ret1 := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 105, Typ: ir.TypeWord}, Val: paramA}
	cBlk1.Instructions = []ir.Instruction{}
	cBlk1.Terminator = ret1
	cBlk1.Predecessors = []*ir.BasicBlock{cBlk0}

	ret2 := &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 106, Typ: ir.TypeWord}, Val: paramB}
	cBlk2.Instructions = []ir.Instruction{}
	cBlk2.Terminator = ret2
	cBlk2.Predecessors = []*ir.BasicBlock{cBlk0}

	callee := &ir.Function{
		Name:       "min_const",
		Parameters: []*ir.Parameter{paramA, paramB},
		Blocks:     []*ir.BasicBlock{cBlk0, cBlk1, cBlk2},
	}

	// Caller calls min_const(10, 20) with constants
	c10 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 200, Typ: ir.TypeWord}, Val: 10}
	c20 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 201, Typ: ir.TypeWord}, Val: 20}
	call := &ir.Call{
		BaseInstruction: ir.BaseInstruction{ID: 202, Typ: ir.TypeWord},
		Func:            callee,
		Args:            []ir.Value{c10, c20},
	}
	callerBlk := &ir.BasicBlock{
		ID:           203,
		Instructions: []ir.Instruction{c10, c20, call},
		Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 204, Typ: ir.TypeWord}, Val: call},
	}
	caller := &ir.Function{
		Name:   "caller_const",
		Blocks: []*ir.BasicBlock{callerBlk},
	}

	prog := &ir.Program{
		Functions: []*ir.Function{caller, callee},
	}

	changed := InlinePass(prog, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    true,
		MaxTinyInstructions: 8,
		MaxInlineRounds:     10,
		WordSize:            8,
	})

	if !changed {
		t.Fatalf("Expected inlining to succeed")
	}

	// Find return block
	var retVal ir.Value
	for _, b := range caller.Blocks {
		if ret, ok := b.Terminator.(*ir.Return); ok {
			retVal = ret.Val
			break
		}
	}
	if retVal == nil {
		t.Fatalf("Expected caller to return directly")
	}
	cw, isConst := retVal.(*ir.ConstWord)
	if !isConst || cw.Val != 10 {
		t.Errorf("Expected caller return value to fold to ConstWord 10, got %v", retVal)
	}
}

func TestInlinePopularityWeighting(t *testing.T) {
	// Callee has 12 instructions (above base budget of 8)
	paramA := &ir.Parameter{ID: 1, Name: "a", Typ: ir.TypeWord}
	var instrs []ir.Instruction
	curr := ir.Value(paramA)
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 1}
	instrs = append(instrs, c1)
	for i := 0; i < 11; i++ {
		add := &ir.BinaryOp{
			BaseInstruction: ir.BaseInstruction{ID: 10 + i, Typ: ir.TypeWord},
			Op:              "add",
			Left:            curr,
			Right:           c1,
		}
		instrs = append(instrs, add)
		curr = add
	}
	calleeBlk := &ir.BasicBlock{
		ID:           0,
		Instructions: instrs,
		Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 50, Typ: ir.TypeWord}, Val: curr},
	}
	callee := &ir.Function{
		Name:       "medium_func",
		Parameters: []*ir.Parameter{paramA},
		Blocks:     []*ir.BasicBlock{calleeBlk},
	}

	buildProg := func(callerPop int) (*ir.Program, *ir.Function) {
		call1 := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 60, Typ: ir.TypeWord},
			Func:            callee,
			Args:            []ir.Value{&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 61, Typ: ir.TypeWord}, Val: 42}},
		}
		call2 := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 62, Typ: ir.TypeWord},
			Func:            callee,
			Args:            []ir.Value{&ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 63, Typ: ir.TypeWord}, Val: 99}},
		}
		callerBlk := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{call1, call2},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 64, Typ: ir.TypeWord}, Val: call2},
		}
		caller := &ir.Function{
			Name:       "caller_pop",
			Popularity: callerPop,
			Blocks:     []*ir.BasicBlock{callerBlk},
		}
		prog := &ir.Program{
			Functions: []*ir.Function{caller, callee},
		}
		return prog, caller
	}

	// Case 1: Caller has Popularity = 1 (cold code).
	// Base budget is 8. Callee has 12 instructions.
	// Should NOT inline.
	prog1, caller1 := buildProg(1)
	changed1 := InlinePass(prog1, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    false,
		MaxTinyInstructions: 8,
		MaxInlineRounds:     10,
	})
	if changed1 {
		t.Errorf("Expected cold caller (Popularity=1) NOT to inline 12-instruction function with budget 8")
	}
	if _, isCall := caller1.Blocks[0].Instructions[0].(*ir.Call); !isCall {
		t.Errorf("Expected call to remain in caller1")
	}

	// Case 2: Caller has Popularity = 20 (hot loop path).
	// Effective budget scales to 8 * 2 = 16. Callee has 12 instructions <= 16.
	// SHOULD inline!
	prog2, caller2 := buildProg(20)
	changed2 := InlinePass(prog2, InlineOptions{
		EnableTiny:          true,
		EnableSingleCall:    false,
		MaxTinyInstructions: 8,
		MaxInlineRounds:     10,
	})
	if !changed2 {
		t.Errorf("Expected hot caller (Popularity=20) TO inline 12-instruction function under scaled budget 16")
	}
	for _, instr := range caller2.Blocks[0].Instructions {
		if _, isCall := instr.(*ir.Call); isCall {
			t.Errorf("Expected calls to be inlined in hot caller2")
		}
	}
}

func TestInlineNoInitFunctions(t *testing.T) {
	for _, initName := range []string{"init__main", "prelude.init_0", "main.init_0", "foo.init_1"} {
		initBlk := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeVoid}},
		}
		initFn := &ir.Function{
			Name:       initName,
			Blocks:     []*ir.BasicBlock{initBlk},
			ReturnType: ir.TypeVoid,
		}

		call := &ir.Call{
			BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeVoid},
			Func:            initFn,
		}
		callerBlk := &ir.BasicBlock{
			ID:           0,
			Instructions: []ir.Instruction{call},
			Terminator:   &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeVoid}},
		}
		caller := &ir.Function{
			Name:       "main.main",
			Blocks:     []*ir.BasicBlock{callerBlk},
			ReturnType: ir.TypeVoid,
		}

		prog := &ir.Program{
			Functions: []*ir.Function{caller, initFn},
		}

		changed := InlinePass(prog, InlineOptions{
			EnableTiny:          true,
			EnableSingleCall:    true,
			MaxTinyInstructions: 8,
			MaxInlineRounds:     10,
		})
		if changed {
			t.Errorf("Expected init function %q NOT to be inlined", initName)
		}
		if len(caller.Blocks[0].Instructions) != 1 {
			t.Errorf("Expected 1 instruction in caller for %q, got %d", initName, len(caller.Blocks[0].Instructions))
		}
	}
}
