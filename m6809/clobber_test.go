package m6809

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestHelperClobbers(t *testing.T) {
	// __mul16 must clobber D and X, but NOT Y or U
	mul := HelperClobbers("__mul16")
	if !mul.Contains(RegD) || !mul.Contains(RegX) {
		t.Errorf("__mul16 should clobber D and X, got %s", mul.String())
	}
	if mul.Overlaps(RegY) || mul.Overlaps(RegU) {
		t.Errorf("__mul16 should not clobber Y or U, got %s", mul.String())
	}

	// __divmod16, __div16, __mod16 must clobber D and X, but NOT Y or U
	div := HelperClobbers("__div16")
	if !div.Contains(RegD) || !div.Contains(RegX) {
		t.Errorf("__div16 should clobber D and X, got %s", div.String())
	}
	if div.Overlaps(RegY) || div.Overlaps(RegU) {
		t.Errorf("__div16 should not clobber Y or U, got %s", div.String())
	}

	// __memcpy must clobber D, X, and Y, but NOT U (preserves U)
	cpy := HelperClobbers("__memcpy")
	if !cpy.Contains(RegD) || !cpy.Contains(RegX) || !cpy.Contains(RegY) {
		t.Errorf("__memcpy should clobber D, X, and Y, got %s", cpy.String())
	}
	if cpy.Overlaps(RegU) {
		t.Errorf("__memcpy should not clobber U, got %s", cpy.String())
	}

	// __memset0 must clobber D and X, but NOT Y or U
	set := HelperClobbers("__memset0")
	if !set.Contains(RegD) || !set.Contains(RegX) {
		t.Errorf("__memset0 should clobber D and X, got %s", set.String())
	}
	if set.Overlaps(RegY) || set.Overlaps(RegU) {
		t.Errorf("__memset0 should not clobber Y or U, got %s", set.String())
	}
}

func TestInstructionDirectClobbers(t *testing.T) {
	b := New(false, false, false)

	// 1. ConstWord: only touches D and CC
	cw := &ir.ConstWord{Val: 42}
	c1 := b.InstructionDirectClobbers(cw)
	if !c1.Contains(RegD) {
		t.Errorf("ConstWord should clobber D, got %s", c1.String())
	}
	if c1.Overlaps(RegX) || c1.Overlaps(RegY) || c1.Overlaps(RegU) {
		t.Errorf("ConstWord should not clobber X, Y, or U, got %s", c1.String())
	}

	// 2. 16-bit Multiply: touches D, X, CC
	mul := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{Typ: ir.TypeWord},
		Op:              "mul",
		Left:            &ir.ConstWord{Val: 2},
		Right:           &ir.ConstWord{Val: 3},
	}
	c2 := b.InstructionDirectClobbers(mul)
	if !c2.Contains(RegD) || !c2.Contains(RegX) {
		t.Errorf("16-bit Multiply should clobber D and X, got %s", c2.String())
	}
	if c2.Overlaps(RegY) || c2.Overlaps(RegU) {
		t.Errorf("16-bit Multiply should not clobber Y or U, got %s", c2.String())
	}

	// 3. Multi-byte aggregate (4-byte struct): touches D, X, and Y
	structType := ir.Type{
		Name: "Point",
		Bits: ir.TypeBitStruct,
		FieldNamesAndTypes: []ir.NameAndType{
			{Name: "x", Type: ir.TypeWord},
			{Name: "y", Type: ir.TypeWord},
		},
	}
	insField := &ir.InsertField{
		BaseInstruction: ir.BaseInstruction{Typ: structType},
		Struct:          &ir.ConstWord{Val: 0},
		FieldIndex:      0,
		Val:             &ir.ConstWord{Val: 10},
	}
	c3 := b.InstructionDirectClobbers(insField)
	if !c3.Contains(RegY) || !c3.Contains(RegX) || !c3.Contains(RegD) {
		t.Errorf("InsertField on 4-byte struct must clobber D, X, and Y, got %s", c3.String())
	}
}

func TestFunctionDirectClobbers(t *testing.T) {
	b := New(false, false, false)

	// Function 1: Pure scalar arithmetic (inc: x + 1)
	// Only uses D; never touches X, Y, or U!
	f1 := &ir.Function{
		Name: "inc",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.BinaryOp{
						BaseInstruction: ir.BaseInstruction{Typ: ir.TypeWord},
						Op:              "add",
						Left:            &ir.ConstWord{Val: 10},
						Right:           &ir.ConstWord{Val: 1},
					},
				},
				Terminator: &ir.Return{
					Val: &ir.ConstWord{Val: 11},
				},
			},
		},
	}

	clobbers1 := b.FunctionDirectClobbers(f1)
	if !clobbers1.Contains(RegD) {
		t.Errorf("inc should clobber D, got %s", clobbers1.String())
	}
	if clobbers1.Overlaps(RegX) {
		t.Errorf("inc should NOT clobber X, got %s", clobbers1.String())
	}
	if clobbers1.Overlaps(RegY) {
		t.Errorf("inc should NOT clobber Y, got %s", clobbers1.String())
	}
	if clobbers1.Overlaps(RegU) {
		t.Errorf("inc should NOT clobber U, got %s", clobbers1.String())
	}

	// Verify functionClobbersY compatibility
	if b.functionClobbersY(f1) {
		t.Errorf("inc should not clobber Y")
	}

	// Function 2: Struct copy function
	structType := ir.Type{
		Name: "Point",
		Bits: ir.TypeBitStruct,
		FieldNamesAndTypes: []ir.NameAndType{
			{Name: "x", Type: ir.TypeWord},
			{Name: "y", Type: ir.TypeWord},
		},
	}
	f2 := &ir.Function{
		Name: "copy_point",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.InsertField{
						BaseInstruction: ir.BaseInstruction{Typ: structType},
						Struct:          &ir.ConstWord{Val: 0},
						FieldIndex:      0,
						Val:             &ir.ConstWord{Val: 10},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	clobbers2 := b.FunctionDirectClobbers(f2)
	if !clobbers2.Contains(RegY) {
		t.Errorf("copy_point must clobber Y, got %s", clobbers2.String())
	}
	if !b.functionClobbersY(f2) {
		t.Errorf("functionClobbersY(f2) must return true")
	}
}

func TestProgramClobberPropagation(t *testing.T) {
	b := New(false, false, false)

	// 1. leaf_inc: scalar leaf function (only touches D and CC)
	leafInc := &ir.Function{
		Name: "leaf_inc",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.BinaryOp{
						BaseInstruction: ir.BaseInstruction{Typ: ir.TypeWord},
						Op:              "add",
						Left:            &ir.ConstWord{Val: 1},
						Right:           &ir.ConstWord{Val: 2},
					},
				},
				Terminator: &ir.Return{Val: &ir.ConstWord{Val: 3}},
			},
		},
	}

	// 2. caller_inc: calls leaf_inc
	callerInc := &ir.Function{
		Name: "caller_inc",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.Call{
						BaseInstruction: ir.BaseInstruction{Typ: ir.TypeWord},
						Func:            leafInc,
						Args:            []ir.Value{&ir.ConstWord{Val: 1}},
					},
				},
				Terminator: &ir.Return{Val: &ir.ConstWord{Val: 4}},
			},
		},
	}

	// 3. leaf_copy: struct copy leaf (touches D, X, Y, CC)
	structType := ir.Type{
		Name: "Point",
		Bits: ir.TypeBitStruct,
		FieldNamesAndTypes: []ir.NameAndType{
			{Name: "x", Type: ir.TypeWord},
			{Name: "y", Type: ir.TypeWord},
		},
	}
	leafCopy := &ir.Function{
		Name: "leaf_copy",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.InsertField{
						BaseInstruction: ir.BaseInstruction{Typ: structType},
						Struct:          &ir.ConstWord{Val: 0},
						FieldIndex:      0,
						Val:             &ir.ConstWord{Val: 10},
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	// 4. caller_copy: calls leaf_copy
	callerCopy := &ir.Function{
		Name: "caller_copy",
		Blocks: []*ir.BasicBlock{
			{
				Instructions: []ir.Instruction{
					&ir.Call{
						BaseInstruction: ir.BaseInstruction{Typ: ir.TypeVoid},
						Func:            leafCopy,
						Args:            nil,
					},
				},
				Terminator: &ir.Return{},
			},
		},
	}

	prog := &ir.Program{
		Functions: []*ir.Function{leafInc, callerInc, leafCopy, callerCopy},
	}

	analysis := b.AnalyzeProgramClobbers(prog)

	// Verify leafInc: only D and CC
	leafTotal := analysis.TotalClobbers["leaf_inc"]
	if !leafTotal.Contains(RegD) || leafTotal.Overlaps(RegY) || leafTotal.Overlaps(RegU) {
		t.Errorf("leaf_inc total clobbers should be D|CC, got %s", leafTotal.String())
	}

	// Verify leafCopy: must contain Y
	leafCopyTotal := analysis.TotalClobbers["leaf_copy"]
	if !leafCopyTotal.Contains(RegY) {
		t.Errorf("leaf_copy total clobbers must contain Y, got %s", leafCopyTotal.String())
	}

	// Verify callerCopy: transitively inherits Y from leafCopy!
	callerCopyTotal := analysis.TotalClobbers["caller_copy"]
	if !callerCopyTotal.Contains(RegY) {
		t.Errorf("caller_copy must transitively inherit Y clobber from leaf_copy, got %s", callerCopyTotal.String())
	}
}
