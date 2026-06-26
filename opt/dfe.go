package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// EliminateDeadFunctions removes functions from the program that are never reachable
// starting from the main entry points.
//
// When protectMagic is true (the "normal" DCE round), every function whose
// IsMagic flag is set is treated as an unconditional root so that it survives
// until after all templates have been expanded and all operator-to-helper
// calls have been materialised in the IR.
//
// When protectMagic is false (the final "magic" DCE round), magic functions
// are subject to the same reachability analysis as ordinary functions.
func EliminateDeadFunctions(p *ir.Program, protectMagic bool) bool {
	reachable := make(map[string]bool)
	queue := make([]*ir.Function, 0)

	funcMap := make(map[string]*ir.Function)
	for _, fn := range p.Functions {
		funcMap[fn.Name] = fn
	}

	markReachable := func(name string) {
		if !reachable[name] {
			reachable[name] = true
			if fn, ok := funcMap[name]; ok {
				queue = append(queue, fn)
			}
		}
	}

	// 1. Initial roots: always-live entry points.
	for _, fn := range p.Functions {
		name := fn.Name
		if name == "main.main" || name == "prelude.init_0" || name == "main.init_0" || name == "init__main" {
			markReachable(name)
		}
		// In the normal round, magic functions are unconditional roots so that
		// they are not removed before the IR builder has had a chance to emit
		// implicit calls to them (e.g. strcmp for string '<').
		if protectMagic && fn.IsMagic {
			markReachable(name)
		}
	}

	// 2. Trace reachability through explicit Call and AddressOfFunc instructions.
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]

		for _, b := range f.Blocks {
			for _, instr := range b.Instructions {
				if call, ok := instr.(*ir.Call); ok {
					markReachable(call.Func.Name)
				} else if addrFunc, ok := instr.(*ir.AddressOfFunc); ok {
					markReachable(addrFunc.Func.Name)
				}
			}
		}
	}

	// 3. Prune unreachable functions.
	changed := false
	var newFunctions []*ir.Function
	for _, f := range p.Functions {
		if reachable[f.Name] {
			newFunctions = append(newFunctions, f)
		} else {
			changed = true
		}
	}

	if changed {
		p.Functions = newFunctions
	}

	return changed
}
