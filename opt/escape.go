package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// EscapeAnalysisResult holds the results of escape analysis on a function.
type EscapeAnalysisResult struct {
	// EscapingLocals maps the ID of a local variable instruction (e.g. ZeroInit) to true
	// if its address escapes the function or is subjected to non-trivial pointer operations.
	EscapingLocals map[int]bool
	// EscapingAOL maps the ID of an AddressOfLocal instruction to true if that specific
	// address-of instruction escapes.
	EscapingAOL map[int]bool
}

// AnalyzeEscape performs escape analysis on AddressOfLocal instructions in function f.
// An address does NOT escape if all its uses are direct LoadPtr or StorePtr pointer operands
// (including AddressOfField wrappers that only feed LoadPtr or StorePtr).
func AnalyzeEscape(f *ir.Function) EscapeAnalysisResult {
	res := EscapeAnalysisResult{
		EscapingLocals: make(map[int]bool),
		EscapingAOL:    make(map[int]bool),
	}

	aolMap := make(map[int]*ir.AddressOfLocal)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if aol, ok := instr.(*ir.AddressOfLocal); ok {
				aolMap[aol.GetID()] = aol
			}
		}
	}

	if len(aolMap) == 0 {
		return res
	}

	// Trace which pointers only feed LoadPtr / StorePtr (possibly through AddressOfField).
	// A pointer instruction is considered escaping if it is used in any other context.
	isSafePtrUse := func(targetID int, user ir.Instruction) bool {
		switch u := user.(type) {
		case *ir.LoadPtr:
			if ptrInst, ok := u.Ptr.(ir.Instruction); ok && ptrInst.GetID() == targetID {
				return true
			}
		case *ir.StorePtr:
			if ptrInst, ok := u.Ptr.(ir.Instruction); ok && ptrInst.GetID() == targetID {
				// u.Ptr is safe. But if u.Val == targetID, the pointer escapes into memory!
				if valInst, ok := u.Val.(ir.Instruction); ok && valInst.GetID() == targetID {
					return false
				}
				return true
			}
		case *ir.AddressOfField:
			if ptrInst, ok := u.Ptr.(ir.Instruction); ok && ptrInst.GetID() == targetID {
				return true
			}
		}
		return false
	}

	// First pass: find AddressOfField instructions that themselves escape.
	escapingFields := make(map[int]bool)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if aof, ok := instr.(*ir.AddressOfField); ok {
				aofID := aof.GetID()
				for _, b := range f.Blocks {
					for _, other := range b.Instructions {
						if other.GetID() == aofID {
							continue
						}
						for _, op := range OperandsOf(other) {
							if opInst, ok := op.(ir.Instruction); ok && opInst.GetID() == aofID {
								if !isSafePtrUse(aofID, other) {
									escapingFields[aofID] = true
								}
							}
						}
					}
					if b.Terminator != nil {
						for _, op := range OperandsOf(b.Terminator) {
							if opInst, ok := op.(ir.Instruction); ok && opInst.GetID() == aofID {
								escapingFields[aofID] = true
							}
						}
					}
				}
			}
		}
	}

	// Check each AddressOfLocal instruction for escaping uses.
	for aolID, aol := range aolMap {
		escaped := false
		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				if instr.GetID() == aolID {
					continue
				}
				for _, op := range OperandsOf(instr) {
					if opInst, ok := op.(ir.Instruction); ok && opInst.GetID() == aolID {
						if aof, ok := instr.(*ir.AddressOfField); ok {
							if escapingFields[aof.GetID()] {
								escaped = true
								break
							}
						} else if !isSafePtrUse(aolID, instr) {
							escaped = true
							break
						}
					}
				}
				if escaped {
					break
				}
			}
			if escaped {
				break
			}
			if blk.Terminator != nil {
				for _, op := range OperandsOf(blk.Terminator) {
					if opInst, ok := op.(ir.Instruction); ok && opInst.GetID() == aolID {
						escaped = true
						break
					}
				}
			}
			if escaped {
				break
			}
		}

		if escaped {
			res.EscapingAOL[aolID] = true
			if locInst, ok := aol.Local.(ir.Instruction); ok {
				res.EscapingLocals[locInst.GetID()] = true
			}
		}
	}

	return res
}
