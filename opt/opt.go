package opt

import (
	"log"

	"github.com/strickyak/minigolf/ir"
)

type Config struct {
	EnableConstFold   bool
	EnableDBE         bool
	EnableDCE         bool
	EnableCopyProp    bool
	EnableCSE         bool
	EnableStrengthRed bool
	EnablePhiSimp     bool
	EnableStackAlloc  bool
	EnableBranchFold  bool
	EnableDFE         bool
	EnableDebugOpt    bool
}

type Pass interface {
	Name() string
	Run(f *ir.Function) (changed bool)
}

func OptimizeProgram(p *ir.Program, config Config) {
	var passes []Pass

	if config.EnableConstFold {
		passes = append(passes, &ConstFoldPass{})
	}
	if config.EnableDBE {
		passes = append(passes, &DBEPass{})
	}
	if config.EnableDCE {
		passes = append(passes, &DCEPass{})
	}
	if config.EnableCopyProp {
		passes = append(passes, &CopyPropPass{})
	}
	if config.EnableCSE {
		passes = append(passes, &CSEPass{})
	}
	if config.EnableStrengthRed {
		passes = append(passes, &StrengthReductionPass{})
	}
	if config.EnablePhiSimp {
		passes = append(passes, &PhiSimpPass{})
	}
	if config.EnableStackAlloc {
		passes = append(passes, &StackAllocPass{})
	}
	if config.EnableBranchFold {
		passes = append(passes, &BranchFoldPass{})
	}

	for _, f := range p.Functions {
		changed := true
		for iterations := 0; changed && iterations < 10; iterations++ {
			changed = false
			for _, pass := range passes {
				if pass.Run(f) {
					changed = true
				}
			}
		}
	}

	if config.EnableDFE {
		EliminateDeadFunctions(p)
	}

	if config.EnableDebugOpt {
		for _, f := range p.Functions {
			ig := ComputeInterferenceGraph(f)
			log.Printf("Interference Graph for %s:\n%s", f.Name, ig.Format())
		}
	}
}

// ReplaceUsesOf is a helper that replaces all uses of `oldVal` with `newVal` within the function `f`.
func ReplaceUsesOf(f *ir.Function, oldVal ir.Value, newVal ir.Value) {
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			replaceInInstruction(instr, oldVal, newVal)
		}
		if b.Terminator != nil {
			replaceInInstruction(b.Terminator, oldVal, newVal)
		}
	}
}

func replaceInInstruction(instr ir.Instruction, oldVal ir.Value, newVal ir.Value) {
	switch i := instr.(type) {
	case *ir.Store:
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.BinaryOp:
		if i.Left == oldVal {
			i.Left = newVal
		}
		if i.Right == oldVal {
			i.Right = newVal
		}
	case *ir.Compare:
		if i.Left == oldVal {
			i.Left = newVal
		}
		if i.Right == oldVal {
			i.Right = newVal
		}
	case *ir.UnaryOp:
		if i.Operand == oldVal {
			i.Operand = newVal
		}
	case *ir.ExtractElement:
		if i.Array == oldVal {
			i.Array = newVal
		}
		if i.Index == oldVal {
			i.Index = newVal
		}
	case *ir.InsertElement:
		if i.Array == oldVal {
			i.Array = newVal
		}
		if i.Index == oldVal {
			i.Index = newVal
		}
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.ExtractField:
		if i.Struct == oldVal {
			i.Struct = newVal
		}
	case *ir.InsertField:
		if i.Struct == oldVal {
			i.Struct = newVal
		}
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.AddressOfLocal:
		if i.Local == oldVal {
			i.Local = newVal
		}
	case *ir.AddressOfField:
		if i.Ptr == oldVal {
			i.Ptr = newVal
		}
	case *ir.AddressOfElement:
		if i.ArrayPtr == oldVal {
			i.ArrayPtr = newVal
		}
		if i.Index == oldVal {
			i.Index = newVal
		}
	case *ir.ExtractFieldPtr:
		if i.Ptr == oldVal {
			i.Ptr = newVal
		}
	case *ir.InsertFieldPtr:
		if i.Ptr == oldVal {
			i.Ptr = newVal
		}
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.LoadPtr:
		if i.Ptr == oldVal {
			i.Ptr = newVal
		}
	case *ir.StorePtr:
		if i.Ptr == oldVal {
			i.Ptr = newVal
		}
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.Phi:
		for idx := range i.Edges {
			if i.Edges[idx].Value == oldVal {
				i.Edges[idx].Value = newVal
			}
		}
	case *ir.Call:
		for idx := range i.Args {
			if i.Args[idx] == oldVal {
				i.Args[idx] = newVal
			}
		}
	case *ir.IndirectCall:
		if i.FuncPtr == oldVal {
			i.FuncPtr = newVal
		}
		for idx := range i.Args {
			if i.Args[idx] == oldVal {
				i.Args[idx] = newVal
			}
		}
	case *ir.BuiltinCall:
		for idx := range i.Args {
			if i.Args[idx] == oldVal {
				i.Args[idx] = newVal
			}
		}
	case *ir.Cast:
		if i.Operand == oldVal {
			i.Operand = newVal
		}
	case *ir.Branch:
		if i.Condition == oldVal {
			i.Condition = newVal
		}
	case *ir.Return:
		if i.Val == oldVal {
			i.Val = newVal
		}
	case *ir.ConstArray:
		for idx := range i.Elements {
			if i.Elements[idx] == oldVal {
				i.Elements[idx] = newVal
			}
		}
	case *ir.ConstStruct:
		for idx := range i.Fields {
			if i.Fields[idx] == oldVal {
				i.Fields[idx] = newVal
			}
		}
	}
}

// OperandsOf returns all the values that are used as operands by the given instruction.
func OperandsOf(instr ir.Instruction) []ir.Value {
	var ops []ir.Value
	switch i := instr.(type) {
	case *ir.Store:
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.BinaryOp:
		if i.Left != nil {
			ops = append(ops, i.Left)
		}
		if i.Right != nil {
			ops = append(ops, i.Right)
		}
	case *ir.Compare:
		if i.Left != nil {
			ops = append(ops, i.Left)
		}
		if i.Right != nil {
			ops = append(ops, i.Right)
		}
	case *ir.UnaryOp:
		if i.Operand != nil {
			ops = append(ops, i.Operand)
		}
	case *ir.ExtractElement:
		if i.Array != nil {
			ops = append(ops, i.Array)
		}
		if i.Index != nil {
			ops = append(ops, i.Index)
		}
	case *ir.InsertElement:
		if i.Array != nil {
			ops = append(ops, i.Array)
		}
		if i.Index != nil {
			ops = append(ops, i.Index)
		}
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.ExtractField:
		if i.Struct != nil {
			ops = append(ops, i.Struct)
		}
	case *ir.InsertField:
		if i.Struct != nil {
			ops = append(ops, i.Struct)
		}
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.AddressOfLocal:
		if i.Local != nil {
			ops = append(ops, i.Local)
		}
	case *ir.AddressOfField:
		if i.Ptr != nil {
			ops = append(ops, i.Ptr)
		}
	case *ir.AddressOfElement:
		if i.ArrayPtr != nil {
			ops = append(ops, i.ArrayPtr)
		}
		if i.Index != nil {
			ops = append(ops, i.Index)
		}
	case *ir.ExtractFieldPtr:
		if i.Ptr != nil {
			ops = append(ops, i.Ptr)
		}
	case *ir.InsertFieldPtr:
		if i.Ptr != nil {
			ops = append(ops, i.Ptr)
		}
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.LoadPtr:
		if i.Ptr != nil {
			ops = append(ops, i.Ptr)
		}
	case *ir.StorePtr:
		if i.Ptr != nil {
			ops = append(ops, i.Ptr)
		}
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.Phi:
		for _, e := range i.Edges {
			if e.Value != nil {
				ops = append(ops, e.Value)
			}
		}
	case *ir.Call:
		for _, a := range i.Args {
			if a != nil {
				ops = append(ops, a)
			}
		}
	case *ir.IndirectCall:
		if i.FuncPtr != nil {
			ops = append(ops, i.FuncPtr)
		}
		for _, a := range i.Args {
			if a != nil {
				ops = append(ops, a)
			}
		}
	case *ir.BuiltinCall:
		for _, a := range i.Args {
			if a != nil {
				ops = append(ops, a)
			}
		}
	case *ir.Cast:
		if i.Operand != nil {
			ops = append(ops, i.Operand)
		}
	case *ir.Branch:
		if i.Condition != nil {
			ops = append(ops, i.Condition)
		}
	case *ir.Return:
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.ConstArray:
		for _, e := range i.Elements {
			if e != nil {
				ops = append(ops, e)
			}
		}
	case *ir.ConstStruct:
		for _, f := range i.Fields {
			if f != nil {
				ops = append(ops, f)
			}
		}
	}
	return ops
}
