package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// StoreLoadForwardingPass eliminates redundant loads from local variables
// within the same basic block when the stored value is already known.
type StoreLoadForwardingPass struct{}

func (p *StoreLoadForwardingPass) Name() string {
	return "StoreLoadForwardingPass"
}

func getRootLocalAOL(ptr ir.Value) *ir.AddressOfLocal {
	for ptr != nil {
		switch p := ptr.(type) {
		case *ir.AddressOfLocal:
			return p
		case *ir.AddressOfField:
			ptr = p.Ptr
		case *ir.AddressOfElement:
			ptr = p.ArrayPtr
		case *ir.ExtractFieldPtr:
			ptr = p.Ptr
		default:
			return nil
		}
	}
	return nil
}

func (p *StoreLoadForwardingPass) Run(f *ir.Function) bool {
	changed := false

	// Step 1: Escape analysis for AddressOfLocal
	escapeRes := AnalyzeEscape(f)
	escaping := escapeRes.EscapingAOL

	// Step 2: Forward stores to loads within each basic block
	for _, b := range f.Blocks {
		// Map from AddressOfLocal ID to known Value
		knownStore := make(map[int]ir.Value)
		var remaining []ir.Instruction

		for _, instr := range b.Instructions {
			switch inst := instr.(type) {
			case *ir.StorePtr:
				if aol, ok := inst.Ptr.(*ir.AddressOfLocal); ok && !escaping[aol.GetID()] {
					knownStore[aol.GetID()] = inst.Val
				} else {
					if rootAOL := getRootLocalAOL(inst.Ptr); rootAOL != nil {
						delete(knownStore, rootAOL.GetID())
					}
					// An indirect store to an unknown pointer might alias escaping locals
					for id := range knownStore {
						if escaping[id] {
							delete(knownStore, id)
						}
					}
				}
				remaining = append(remaining, instr)

			case *ir.LoadPtr:
				if aol, ok := inst.Ptr.(*ir.AddressOfLocal); ok && !escaping[aol.GetID()] {
					if val, ok := knownStore[aol.GetID()]; ok && val.Type().Equals(inst.Type()) {
						// Forward store to load!
						ReplaceUsesOf(f, inst, val)
						changed = true
						// Do not add to remaining (eliminate redundant load)
						continue
					}
					// If not known, this load establishes the value
					knownStore[aol.GetID()] = inst
				}
				remaining = append(remaining, instr)

			case *ir.Call, *ir.IndirectCall:
				// Calls can only modify escaping variables or globals.
				// Non-escaping locals are guaranteed safe!
				for id := range knownStore {
					if escaping[id] {
						delete(knownStore, id)
					}
				}
				remaining = append(remaining, instr)

			default:
				remaining = append(remaining, instr)
			}
		}

		b.Instructions = remaining
	}

	return changed
}
