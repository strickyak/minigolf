package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type StackAllocPass struct{}

func (p *StackAllocPass) Name() string { return "StackAlloc" }

func (p *StackAllocPass) Run(f *ir.Function) bool {
	oldAlias := f.SlotAlias
	f.SlotAlias = make(map[int]int)

	// 1. Identify which block each variable is defined in.
	defBlock := make(map[ir.Value]*ir.BasicBlock)
	for _, b := range f.Blocks {
		for _, inst := range b.Instructions {
			defBlock[inst] = b
		}
	}

	// 2. Determine cross-block usage.
	crossBlock := make(map[ir.Value]bool)
	// Track if AddressOfLocal is used (makes it escape).
	escapes := make(map[ir.Value]bool)

	for _, b := range f.Blocks {
		for _, inst := range b.Instructions {
			for _, op := range getOperands(inst) {
				if db, ok := defBlock[op]; ok && db != b {
					crossBlock[op] = true
				}
			}
			if aol, ok := inst.(*ir.AddressOfLocal); ok {
				escapes[aol.Local] = true
				crossBlock[aol.Local] = true
			}
			if phi, ok := inst.(*ir.Phi); ok {
				for _, e := range phi.Edges {
					if e.Value != nil {
						crossBlock[e.Value] = true
					}
				}
			}
		}
		if b.Terminator != nil {
			for _, op := range getOperands(b.Terminator) {
				if db, ok := defBlock[op]; ok && db != b {
					crossBlock[op] = true
				}
			}
		}
	}

	// Parameters are implicitly cross-block (defined before any block).
	for _, param := range f.Parameters {
		crossBlock[param] = true
	}

	// 3. For each block, compute live ranges of local variables.
	// Also group them by ir.Type.String()
	globalAliases := make(map[string][]int)

	for _, b := range f.Blocks {
		// Collect local vars in this block
		var locals []ir.Value
		// live ranges: maps local var -> [startIdx, endIdx]
		startIdx := make(map[ir.Value]int)
		endIdx := make(map[ir.Value]int)

		for i, inst := range b.Instructions {
			if !crossBlock[inst] && !escapes[inst] {
				locals = append(locals, inst)
				startIdx[inst] = i
				endIdx[inst] = i // default end index is its definition
			}
			// Update endIdx for operands
			for _, op := range getOperands(inst) {
				if db, ok := defBlock[op]; ok && db == b && !crossBlock[op] {
					if i > endIdx[op] {
						endIdx[op] = i
					}
				}
			}
		}

		// Also check Terminator
		if b.Terminator != nil {
			i := len(b.Instructions)
			for _, op := range getOperands(b.Terminator) {
				if db, ok := defBlock[op]; ok && db == b && !crossBlock[op] {
					if i > endIdx[op] {
						endIdx[op] = i
					}
				}
			}
		}

		// Group locals by type
		byType := make(map[string][]ir.Value)
		for _, loc := range locals {
			tStr := loc.Type().String()
			byType[tStr] = append(byType[tStr], loc)
		}

		// Allocate aliases
		for tStr, vars := range byType {
			// We can track available aliases (colors) and when they become free in THIS block.
			type colorSlot struct {
				id     int
				freeAt int
			}
			var activeColors []*colorSlot

			// Add global colors (from other blocks) as free at index -1
			for _, gId := range globalAliases[tStr] {
				activeColors = append(activeColors, &colorSlot{id: gId, freeAt: -1})
			}

			for _, v := range vars {
				assigned := false
				vid := v.(interface{ GetID() int }).GetID()
				for _, c := range activeColors {
					if startIdx[v] > c.freeAt {
						// We can share this slot!
						f.SlotAlias[vid] = c.id
						c.freeAt = endIdx[v]
						assigned = true
						break
					}
				}
				if !assigned {
					// Create new color
					c := &colorSlot{id: vid, freeAt: endIdx[v]}
					activeColors = append(activeColors, c)
					globalAliases[tStr] = append(globalAliases[tStr], vid)
				}
			}
		}
	}

	if len(oldAlias) != len(f.SlotAlias) {
		return true
	}
	for k, v := range f.SlotAlias {
		if oldAlias[k] != v {
			return true
		}
	}
	return false
}

func getOperands(instr ir.Instruction) []ir.Value {
	var ops []ir.Value
	switch i := instr.(type) {
	case *ir.Store:
		ops = append(ops, i.Val)
	case *ir.BinaryOp:
		ops = append(ops, i.Left, i.Right)
	case *ir.Compare:
		ops = append(ops, i.Left, i.Right)
	case *ir.UnaryOp:
		ops = append(ops, i.Operand)
	case *ir.ExtractElement:
		ops = append(ops, i.Array, i.Index)
	case *ir.InsertElement:
		ops = append(ops, i.Array, i.Index, i.Val)
	case *ir.ExtractField:
		ops = append(ops, i.Struct)
	case *ir.InsertField:
		ops = append(ops, i.Struct, i.Val)
	case *ir.AddressOfLocal:
		ops = append(ops, i.Local)
	case *ir.AddressOfField:
		ops = append(ops, i.Ptr)
	case *ir.AddressOfElement:
		ops = append(ops, i.ArrayPtr, i.Index)
	case *ir.ExtractFieldPtr:
		ops = append(ops, i.Ptr)
	case *ir.InsertFieldPtr:
		ops = append(ops, i.Ptr, i.Val)
	case *ir.LoadPtr:
		ops = append(ops, i.Ptr)
	case *ir.StorePtr:
		ops = append(ops, i.Ptr, i.Val)
	case *ir.Phi:
		for _, e := range i.Edges {
			ops = append(ops, e.Value)
		}
	case *ir.Call:
		ops = append(ops, i.Args...)
	case *ir.IndirectCall:
		ops = append(ops, i.FuncPtr)
		ops = append(ops, i.Args...)
	case *ir.BuiltinCall:
		ops = append(ops, i.Args...)
	case *ir.Cast:
		ops = append(ops, i.Operand)
	case *ir.Branch:
		if i.Condition != nil {
			ops = append(ops, i.Condition)
		}
	case *ir.Return:
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	}
	return ops
}
