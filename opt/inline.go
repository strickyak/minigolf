package opt

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

var _ = fmt.Sprintf

type InlineOptions struct {
	EnableTiny       bool
	EnableSingleCall bool
}

func DefaultInlineOptions() InlineOptions {
	noInline := os.Getenv("NO_INLINE") != ""
	noTiny := os.Getenv("NO_INLINE_TINY") != "" || noInline
	noSingle := os.Getenv("NO_INLINE_SINGLE_CALL") != "" || noInline
	return InlineOptions{
		EnableTiny:       !noTiny,
		EnableSingleCall: !noSingle,
	}
}

// InlinePass runs function inlining across the entire whole-program IR.
func InlinePass(p *ir.Program, opts InlineOptions) bool {
	if !opts.EnableTiny && !opts.EnableSingleCall {
		return false
	}

	overallChanged := false

	// Find the current maximum instruction ID and block ID
	maxID := 0
	for _, f := range p.Functions {
		for _, b := range f.Blocks {
			for _, instr := range b.Instructions {
				if instr.GetID() > maxID {
					maxID = instr.GetID()
				}
			}
		}
	}
	nextID := func() int {
		maxID++
		return maxID
	}

	for round := 0; round < 10; round++ {
		roundChanged := false

		// Analyze call counts across all functions in the program
		callCounts := make(map[*ir.Function]int)
		addrTaken := make(map[*ir.Function]bool)
		for _, f := range p.Functions {
			for _, b := range f.Blocks {
				for _, instr := range b.Instructions {
					switch i := instr.(type) {
					case *ir.Call:
						if i.Func != nil {
							callCounts[i.Func]++
						}
					case *ir.AddressOfFunc:
						if i.Func != nil {
							addrTaken[i.Func] = true
						}
					}
				}
			}
		}

		for _, caller := range p.Functions {
			for _, b := range caller.Blocks {
				for j := 0; j < len(b.Instructions); j++ {
					call, isCall := b.Instructions[j].(*ir.Call)
					if !isCall || call.Func == nil {
						continue
					}
					callee := call.Func
					if callee == caller {
						continue // Never self-inline
					}
					if !canInline(callee) {
						continue
					}

					isTiny := opts.EnableTiny && isTinyFunction(callee)
					isSingle := opts.EnableSingleCall && callCounts[callee] == 1 && !addrTaken[callee]

					if isTiny || isSingle {
						if isStraightLine(callee) {
							inlineStraightLine(caller, b, j, call, callee, nextID)
							roundChanged = true
							overallChanged = true
							break // Restart scan of this block
						}
					}
				}
			}
		}

		if !roundChanged {
			break
		}
	}

	return overallChanged
}

// canInline checks whether a callee is eligible for inlining under any strategy.
func canInline(f *ir.Function) bool {
	if f == nil || len(f.Blocks) == 0 {
		return false
	}
	// Never inline entry points or runtime root functions
	if f.Name == "main.main" || f.Name == "_main" || f.Name == "prelude.init_0" {
		return false
	}
	// Never inline functions with defers, destructors, setjmp, longjmp, or local variable addresses
	if hasDefersOrDestructors(f) {
		return false
	}
	return true
}

// hasDefersOrDestructors checks if the function has deferred actions, destructors, or local address operations.
func hasDefersOrDestructors(f *ir.Function) bool {
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			switch i := instr.(type) {
			case *ir.SetJmp, *ir.LongJmp:
				return true
			case *ir.AddressOfLocal:
				return true
			case *ir.BuiltinCall:
				if i.Name == "_unlink_jmp_" || i.Name == "_link_jmp_" || strings.Contains(i.Name, "destruct") {
					return true
				}
			}
		}
	}
	return false
}

// isStraightLine checks if the function has single-path execution from entry to return.
func isStraightLine(f *ir.Function) bool {
	if len(f.Blocks) == 0 {
		return false
	}
	if len(f.Blocks) == 1 {
		_, isRet := f.Blocks[0].Terminator.(*ir.Return)
		return isRet
	}
	if len(f.Blocks) == 2 {
		j, isJump := f.Blocks[0].Terminator.(*ir.Jump)
		if !isJump || j.Target != f.Blocks[1] {
			return false
		}
		_, isRet := f.Blocks[1].Terminator.(*ir.Return)
		if !isRet {
			return false
		}
		// f.Blocks[1] must not have real instructions other than markers, terminators, and single-edge phis
		for _, instr := range f.Blocks[1].Instructions {
			if _, isMarker := instr.(*ir.SourceMarker); isMarker {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				if len(phi.Edges) == 1 && phi.Edges[0].Block == f.Blocks[0] {
					continue
				}
			}
			return false
		}
		return true
	}
	return false
}

// isTinyFunction checks if a function is small enough to inline unconditionally everywhere.
func isTinyFunction(f *ir.Function) bool {
	if !isStraightLine(f) {
		return false
	}
	instrCount := 0
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			switch instr.(type) {
			case *ir.SourceMarker, ir.Terminator, *ir.Phi:
				continue
			case *ir.Call, *ir.IndirectCall, *ir.BuiltinCall:
				return false // Tiny functions should not have nested calls
			default:
				instrCount++
			}
		}
	}
	return instrCount <= 8
}

// inlineStraightLine inlines a straight-line callee into caller block b at callIdx.
func inlineStraightLine(caller *ir.Function, b *ir.BasicBlock, callIdx int, call *ir.Call, callee *ir.Function, nextID func() int) {
	valMap := make(map[ir.Value]ir.Value)
	for i, param := range callee.Parameters {
		if i < len(call.Args) {
			valMap[param] = call.Args[i]
		}
	}

	var clonedInstrs []ir.Instruction
	var retVal ir.Value

	for _, cBlk := range callee.Blocks {
		for _, instr := range cBlk.Instructions {
			if _, isMarker := instr.(*ir.SourceMarker); isMarker {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				if len(phi.Edges) == 1 {
					valMap[phi] = mapVal(phi.Edges[0].Value, valMap)
					continue
				}
			}
			cloned := cloneInstruction(instr, nextID(), valMap)
			valMap[instr] = cloned
			clonedInstrs = append(clonedInstrs, cloned)
		}
		if ret, ok := cBlk.Terminator.(*ir.Return); ok {
			if ret.Val != nil {
				retVal = mapVal(ret.Val, valMap)
			}
		}
	}

	if retVal != nil {
		ReplaceUsesOf(caller, call, retVal)
	}

	newInstructions := make([]ir.Instruction, 0, len(b.Instructions)-1+len(clonedInstrs))
	newInstructions = append(newInstructions, b.Instructions[:callIdx]...)
	newInstructions = append(newInstructions, clonedInstrs...)
	newInstructions = append(newInstructions, b.Instructions[callIdx+1:]...)
	b.Instructions = newInstructions
}

func mapVal(v ir.Value, valMap map[ir.Value]ir.Value) ir.Value {
	if v == nil {
		return nil
	}
	if repl, ok := valMap[v]; ok {
		return repl
	}
	return v
}

func cloneInstruction(instr ir.Instruction, newID int, valMap map[ir.Value]ir.Value) ir.Instruction {
	base := ir.BaseInstruction{
		ID:      newID,
		Typ:     instr.Type(),
		Comment: instr.GetComment(),
		Name:    instr.GetName(),
	}

	switch i := instr.(type) {
	case *ir.ConstByte:
		return &ir.ConstByte{BaseInstruction: base, Val: i.Val}
	case *ir.ConstWord:
		return &ir.ConstWord{BaseInstruction: base, Val: i.Val}
	case *ir.Sizeof:
		return &ir.Sizeof{BaseInstruction: base, TargetTyp: i.TargetTyp}
	case *ir.Load:
		return &ir.Load{BaseInstruction: base, Global: i.Global}
	case *ir.Store:
		return &ir.Store{BaseInstruction: base, Global: i.Global, Val: mapVal(i.Val, valMap)}
	case *ir.BinaryOp:
		return &ir.BinaryOp{BaseInstruction: base, Op: i.Op, Left: mapVal(i.Left, valMap), Right: mapVal(i.Right, valMap)}
	case *ir.Compare:
		return &ir.Compare{BaseInstruction: base, Op: i.Op, Left: mapVal(i.Left, valMap), Right: mapVal(i.Right, valMap)}
	case *ir.UnaryOp:
		return &ir.UnaryOp{BaseInstruction: base, Op: i.Op, Operand: mapVal(i.Operand, valMap)}
	case *ir.ExtractElement:
		return &ir.ExtractElement{BaseInstruction: base, Array: mapVal(i.Array, valMap), Index: mapVal(i.Index, valMap)}
	case *ir.InsertElement:
		return &ir.InsertElement{BaseInstruction: base, Array: mapVal(i.Array, valMap), Index: mapVal(i.Index, valMap), Val: mapVal(i.Val, valMap)}
	case *ir.ExtractField:
		return &ir.ExtractField{BaseInstruction: base, Struct: mapVal(i.Struct, valMap), FieldIndex: i.FieldIndex}
	case *ir.InsertField:
		return &ir.InsertField{BaseInstruction: base, Struct: mapVal(i.Struct, valMap), FieldIndex: i.FieldIndex, Val: mapVal(i.Val, valMap)}
	case *ir.AddressOfGlobal:
		return &ir.AddressOfGlobal{BaseInstruction: base, Global: i.Global}
	case *ir.AddressOfFunc:
		return &ir.AddressOfFunc{BaseInstruction: base, Func: i.Func}
	case *ir.AddressOfField:
		return &ir.AddressOfField{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex}
	case *ir.AddressOfElement:
		return &ir.AddressOfElement{BaseInstruction: base, ArrayPtr: mapVal(i.ArrayPtr, valMap), Index: mapVal(i.Index, valMap)}
	case *ir.ExtractFieldPtr:
		return &ir.ExtractFieldPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex}
	case *ir.InsertFieldPtr:
		return &ir.InsertFieldPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex, Val: mapVal(i.Val, valMap)}
	case *ir.LoadPtr:
		return &ir.LoadPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap)}
	case *ir.StorePtr:
		return &ir.StorePtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), Val: mapVal(i.Val, valMap)}
	case *ir.ZeroInit:
		return &ir.ZeroInit{BaseInstruction: base}
	case *ir.Cast:
		return &ir.Cast{BaseInstruction: base, Op: i.Op, Operand: mapVal(i.Operand, valMap)}
	case *ir.Call:
		newArgs := make([]ir.Value, len(i.Args))
		for idx, a := range i.Args {
			newArgs[idx] = mapVal(a, valMap)
		}
		return &ir.Call{BaseInstruction: base, Func: i.Func, Args: newArgs}
	case *ir.BuiltinCall:
		newArgs := make([]ir.Value, len(i.Args))
		for idx, a := range i.Args {
			newArgs[idx] = mapVal(a, valMap)
		}
		return &ir.BuiltinCall{BaseInstruction: base, Name: i.Name, Args: newArgs}
	default:
		log.Panicf("cloneInstruction: unhandled instruction type %T", instr)
		return nil
	}
}
