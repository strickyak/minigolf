package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type Config struct {
	EnableConstFold bool
	EnableDBE       bool
	EnableDCE       bool
	EnablePhiSimp   bool
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
