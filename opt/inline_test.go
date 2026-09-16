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
		EnableTiny:           true,
		EnableSingleCall:     false,
		MaxTinyInstructions: 2,
		MaxInlineRounds:      10,
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
		EnableTiny:           true,
		EnableSingleCall:     false,
		MaxTinyInstructions: 5,
		MaxInlineRounds:      10,
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
		EnableTiny:           false,
		EnableSingleCall:     false,
		MaxTinyInstructions: 8,
		MaxInlineRounds:      10,
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
		EnableTiny:           false,
		EnableSingleCall:     true,
		MaxTinyInstructions: 1, // Callee has 4 instructions, so tiny inlining would fail
		MaxInlineRounds:      10,
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
