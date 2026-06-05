package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type PhiSimpPass struct{}

func (p *PhiSimpPass) Name() string {
	return "Phi Simplification"
}

func (p *PhiSimpPass) Run(f *ir.Function) bool {
	changed := false

	// Simplify trivial Phis
	for _, b := range f.Blocks {
		var newInstrs []ir.Instruction
		for _, instr := range b.Instructions {
			if phi, ok := instr.(*ir.Phi); ok {
				val := p.simplifyPhi(phi)
				if val != nil {
					// Replace uses of phi with val
					ReplaceUsesOf(f, phi, val)
					changed = true
					// Don't add to newInstrs (delete it)
					continue
				}
			}
			newInstrs = append(newInstrs, instr)
		}
		b.Instructions = newInstrs
	}

	return changed
}

func (p *PhiSimpPass) simplifyPhi(phi *ir.Phi) ir.Value {
	if len(phi.Edges) == 0 {
		return nil
	}

	var commonVal ir.Value
	for _, edge := range phi.Edges {
		val := edge.Value
		if val == phi {
			// Self-referential edge, ignore
			continue
		}
		if commonVal == nil {
			commonVal = val
		} else if commonVal != val {
			// Found different values, cannot simplify
			return nil
		}
	}

	if commonVal == nil {
		// All edges were self-referential
		// This phi is essentially dead or undefined, but returning nil here
		// means we don't simplify it. Let DCE handle it if it has no uses.
		return nil
	}

	return commonVal
}
