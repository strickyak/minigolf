package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// EliminateDeadFunctions removes functions from the program that are never reachable
// starting from the main entry points.
func EliminateDeadFunctions(p *ir.Program) bool {
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

	// 1. Initial roots
	for _, fn := range p.Functions {
		name := fn.Name
		if name == "main" || name == "prelude.init_0" || name == "main.init_0" || name == "init_main" {
			markReachable(name)
		}
	}

	// 2. Trace reachability
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

	// 3. Prune functions
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
