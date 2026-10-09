package bigir

// EliminateTrivialPhis removes redundant Phi instructions where all incoming
// operands (excluding self-references) resolve to the same value.
func EliminateTrivialPhis(prog *Program) {
	for _, fn := range prog.Functions {
		eliminateFuncTrivialPhis(fn)
	}
}

func eliminateFuncTrivialPhis(fn *Function) {
	rep := make(map[int]Value)

	var resolve func(v Value) Value
	resolve = func(v Value) Value {
		if instr, ok := v.(Instruction); ok {
			if next, ok := rep[instr.GetID()]; ok && next != nil && next != v {
				return resolve(next)
			}
		}
		return v
	}

	replaceVal := func(v *Value) {
		if *v != nil {
			resolved := resolve(*v)
			if resolved != *v {
				*v = resolved
			}
		}
	}

	for {
		changed := false

		for _, bb := range fn.Blocks {
			var kept []Instruction
			for _, instr := range bb.Instructions {
				phi, ok := instr.(*Phi)
				if !ok {
					kept = append(kept, instr)
					continue
				}

				var same Value
				isTrivial := true
				for _, edge := range phi.Edges {
					val := resolve(edge.Value)
					if val == phi {
						continue // self-loop
					}
					if same == nil {
						same = val
					} else if same != val {
						isTrivial = false
						break
					}
				}

				if isTrivial && same != nil {
					rep[phi.GetID()] = same
					changed = true
				} else {
					kept = append(kept, instr)
				}
			}
			bb.Instructions = kept
		}

		if !changed {
			break
		}

		// Update all operands across the function with resolved values
		for _, bb := range fn.Blocks {
			for _, instr := range bb.Instructions {
				switch inst := instr.(type) {
				case *UnaryOp:
					replaceVal(&inst.Operand)
				case *BinaryOp:
					replaceVal(&inst.Left)
					replaceVal(&inst.Right)
				case *Compare:
					replaceVal(&inst.Left)
					replaceVal(&inst.Right)
				case *NearStore:
					replaceVal(&inst.Addr)
					replaceVal(&inst.Val)
				case *NearLoad:
					replaceVal(&inst.Addr)
				case *FarStore:
					replaceVal(&inst.FarRef)
					replaceVal(&inst.Offset)
					replaceVal(&inst.Val)
				case *FarLoad:
					replaceVal(&inst.FarRef)
					replaceVal(&inst.Offset)
				case *SliceMake:
					replaceVal(&inst.FarRef)
					replaceVal(&inst.Offset)
					replaceVal(&inst.Length)
					replaceVal(&inst.Capacity)
				case *SliceGet:
					replaceVal(&inst.Slice)
					replaceVal(&inst.Index)
				case *SlicePut:
					replaceVal(&inst.Slice)
					replaceVal(&inst.Index)
					replaceVal(&inst.Val)
				case *SliceChop:
					replaceVal(&inst.Slice)
					replaceVal(&inst.Start)
					replaceVal(&inst.Limit)
				case *SliceField:
					replaceVal(&inst.Slice)
				case *SliceToPtr:
					replaceVal(&inst.Slice)
				case *BitCast:
					replaceVal(&inst.Operand)
				case *ExtractField:
					replaceVal(&inst.Struct)
				case *InsertField:
					replaceVal(&inst.Struct)
					replaceVal(&inst.Val)
				case *NearCall:
					for idx := range inst.Args {
						replaceVal(&inst.Args[idx])
					}
				case *FarCall:
					for idx := range inst.Args {
						replaceVal(&inst.Args[idx])
					}
				case *IndirectCall:
					replaceVal(&inst.FuncPtr)
					for idx := range inst.Args {
						replaceVal(&inst.Args[idx])
					}
				case *Phi:
					for idx := range inst.Edges {
						replaceVal(&inst.Edges[idx].Value)
					}
				case *ConstStruct:
					for idx := range inst.Fields {
						replaceVal(&inst.Fields[idx])
					}
				case *ConstArray:
					for idx := range inst.Elements {
						replaceVal(&inst.Elements[idx])
					}
				}
			}

			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *Return:
					replaceVal(&term.Val)
				case *FarReturn:
					replaceVal(&term.Val)
				case *CondBranch:
					replaceVal(&term.Cond)
				}
			}
		}
	}
}
