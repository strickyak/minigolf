package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type DCEPass struct{}

func (p *DCEPass) Name() string {
	return "Dead Code Elimination"
}

func (p *DCEPass) Run(f *ir.Function) bool {
	changed := false

	// Count uses
	uses := make(map[ir.Value]int)

	// Helper to increment uses
	addUse := func(val ir.Value) {
		uses[val]++
	}

	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			visitOperands(instr, addUse)
		}
		if b.Terminator != nil {
			visitOperands(b.Terminator, addUse)
		}
	}

	// Remove dead instructions
	for _, b := range f.Blocks {
		var newInstrs []ir.Instruction
		for _, instr := range b.Instructions {
			if uses[instr] == 0 && !hasSideEffects(instr) {
				changed = true
			} else {
				newInstrs = append(newInstrs, instr)
			}
		}
		b.Instructions = newInstrs
	}

	return changed
}

func hasSideEffects(instr ir.Instruction) bool {
	switch instr.(type) {
	case *ir.Store, *ir.StorePtr, *ir.Call, *ir.IndirectCall, *ir.BuiltinCall, *ir.Jump, *ir.Branch, *ir.Return, *ir.SourceMarker:
		return true
	}
	return false
}

func visitOperands(instr ir.Instruction, visitor func(ir.Value)) {
	switch i := instr.(type) {
	case *ir.Store:
		visitor(i.Val)
	case *ir.BinaryOp:
		visitor(i.Left)
		visitor(i.Right)
	case *ir.Compare:
		visitor(i.Left)
		visitor(i.Right)
	case *ir.UnaryOp:
		visitor(i.Operand)
	case *ir.ExtractElement:
		visitor(i.Array)
		visitor(i.Index)
	case *ir.InsertElement:
		visitor(i.Array)
		visitor(i.Index)
		visitor(i.Val)
	case *ir.ExtractField:
		visitor(i.Struct)
	case *ir.InsertField:
		visitor(i.Struct)
		visitor(i.Val)
	case *ir.AddressOfLocal:
		visitor(i.Local)
	case *ir.AddressOfField:
		visitor(i.Ptr)
	case *ir.AddressOfElement:
		visitor(i.ArrayPtr)
		visitor(i.Index)
	case *ir.ExtractFieldPtr:
		visitor(i.Ptr)
	case *ir.InsertFieldPtr:
		visitor(i.Ptr)
		visitor(i.Val)
	case *ir.LoadPtr:
		visitor(i.Ptr)
	case *ir.StorePtr:
		visitor(i.Ptr)
		visitor(i.Val)
	case *ir.Phi:
		for _, edge := range i.Edges {
			visitor(edge.Value)
		}
	case *ir.Call:
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.IndirectCall:
		visitor(i.FuncPtr)
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.BuiltinCall:
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.Cast:
		visitor(i.Operand)
	case *ir.Branch:
		visitor(i.Condition)
	case *ir.Return:
		if i.Val != nil {
			visitor(i.Val)
		}
	case *ir.ConstArray:
		for _, el := range i.Elements {
			visitor(el)
		}
	case *ir.ConstStruct:
		for _, el := range i.Fields {
			visitor(el)
		}
	}
}
