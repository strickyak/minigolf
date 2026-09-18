package m6809

import (
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/opt"
)

// HelperClobbers returns the bitmask of physical registers clobbered by
// internal M6809 backend runtime helper routines.
func HelperClobbers(name string) RegMask {
	switch name {
	case "__mul16":
		// Multiplies D * stack, leaves result in D, uses X as scratch accumulator.
		// Preserves Y and U.
		return RegD | RegX | RegCC

	case "__div16", "__mod16", "__divmod16":
		// Takes dividend in X, divisor in D.
		// Returns quotient in D (or X) and remainder in D.
		// Preserves U via pshs u / puls u,pc. Preserves Y.
		return RegD | RegX | RegCC

	case "__memcpy":
		// Takes count in D, dest in X, src in Y.
		// Copies bytes using lda ,y+; sta ,x+.
		// Preserves U via pshs u / puls u,pc.
		return RegD | RegX | RegY | RegCC

	case "__memset0":
		// Takes count in D, dest in X.
		// Clears bytes using sta ,x+.
		// Preserves U via pshs u / puls u,pc. Preserves Y.
		return RegD | RegX | RegCC

	default:
		// External C-library or unknown helper (e.g. _printf, _putchar):
		// Conservatively assume all caller-saved registers are clobbered.
		return RegD | RegX | RegCC
	}
}

// InstructionDirectClobbers returns the bitmask of physical registers directly
// modified when lowering instr to M6809 assembly.
func (b *Backend) InstructionDirectClobbers(instr ir.Instruction) RegMask {
	var clobbers RegMask

	// 1. Multi-byte aggregates and copies:
	// emitCopy and aggregate manipulation use X as dest pointer and Y as src pointer.
	switch instr.(type) {
	case *ir.InsertField, *ir.InsertElement:
		return RegD | RegX | RegY | RegCC
	}

	if b.safeTypeSize(instr.Type()) > 2 {
		return RegD | RegX | RegY | RegCC
	}
	for _, op := range opt.OperandsOf(instr) {
		if b.safeTypeSize(op.Type()) > 2 {
			return RegD | RegX | RegY | RegCC
		}
	}

	// 2. Specific instruction classes
	switch inst := instr.(type) {
	case *ir.Call:
		// Calling another function:
		// Evaluates arguments, sets up D/B (arg 0) and X (arg 1) if Fastcall.
		// Under standard ABI, jsr clobbers caller-saved registers (D, X, CC).
		clobbers |= RegD | RegX | RegCC
		// If return type > 2 bytes, return buffer copy uses Y
		if b.safeTypeSize(inst.Typ) > 2 {
			clobbers |= RegY
		}

	case *ir.IndirectCall, *ir.BuiltinCall:
		clobbers |= RegD | RegX | RegCC
		if b.safeTypeSize(inst.Type()) > 2 {
			clobbers |= RegY
		}

	case *ir.BinaryOp:
		clobbers |= RegD | RegCC
		switch inst.Op {
		case "mul", "*":
			// 16-bit multiplication calls __mul16 or inlines 3 muls using X as accumulator
			if b.safeTypeSize(inst.Type()) == 2 {
				clobbers |= RegX
			}
		case "div", "mod", "/", "%":
			// Division calls __div16/__mod16 or inlines div loop using X
			clobbers |= RegX
		case "shl", "shr", "<<", ">>":
			// Shifts may use B/A or index
			clobbers |= RegD | RegCC
		}

	case *ir.LoadPtr:
		// Loads pointer into X (or uses indexed mode), loads value into D or B
		clobbers |= RegD | RegX | RegCC

	case *ir.StorePtr:
		// Loads value into D/B, loads pointer address into X
		clobbers |= RegD | RegX | RegCC

	case *ir.AddressOfElement, *ir.ExtractFieldPtr:
		// Calculates element address into X
		clobbers |= RegX | RegCC

	case *ir.Load:
		// Loading a local variable: usually into D or B
		clobbers |= RegD | RegCC

	case *ir.Store:
		// Storing a value to local slot: uses D or B
		clobbers |= RegD | RegCC

	case *ir.UnaryOp:
		clobbers |= RegD | RegCC

	case *ir.ConstByte:
		clobbers |= RegB | RegCC

	case *ir.ConstWord:
		clobbers |= RegD | RegCC

	default:
		// Any other value-producing instruction conservatively touches D
		if !inst.Type().Equals(ir.TypeVoid) {
			clobbers |= RegD | RegCC
		}
	}

	return clobbers
}

// TerminatorDirectClobbers returns the physical registers clobbered by a block terminator.
func (b *Backend) TerminatorDirectClobbers(term ir.Terminator) RegMask {
	var clobbers RegMask

	for _, op := range opt.OperandsOf(term) {
		if b.safeTypeSize(op.Type()) > 2 {
			return RegD | RegX | RegY | RegCC
		}
	}

	switch t := term.(type) {
	case *ir.Return:
		if t.Val != nil {
			sz := b.safeTypeSize(t.Val.Type())
			if sz > 2 {
				clobbers |= RegD | RegX | RegY | RegCC
			} else if sz == 1 {
				clobbers |= RegB | RegCC
			} else {
				clobbers |= RegD | RegCC
			}
		}
	case *ir.Branch:
		clobbers |= RegD | RegCC
	}

	return clobbers
}

// FunctionDirectClobbers computes the union of physical registers directly
// modified by instructions and terminators in function f.
// (Excludes transitively called functions, which are handled in interprocedural analysis).
func (b *Backend) FunctionDirectClobbers(f *ir.Function) RegMask {
	var mask RegMask

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			mask |= b.InstructionDirectClobbers(instr)
		}
		if blk.Terminator != nil {
			mask |= b.TerminatorDirectClobbers(blk.Terminator)
		}
	}

	return mask
}

// ProgramClobberAnalysis holds direct and transitive clobber sets for all functions in an ir.Program.
type ProgramClobberAnalysis struct {
	DirectClobbers    map[string]RegMask
	TotalClobbers     map[string]RegMask
	CalledFuncs       map[string]map[string]bool
	MakesIndirectCall map[string]bool
}

// AnalyzeProgramClobbers performs whole-program interprocedural register clobber analysis.
// It computes direct register modifications for each function, then propagates transitive
// clobbers bottom-up through the call graph to a fixed point.
func (b *Backend) AnalyzeProgramClobbers(program *ir.Program) *ProgramClobberAnalysis {
	analysis := &ProgramClobberAnalysis{
		DirectClobbers:    make(map[string]RegMask),
		TotalClobbers:     make(map[string]RegMask),
		CalledFuncs:       make(map[string]map[string]bool),
		MakesIndirectCall: make(map[string]bool),
	}

	if program == nil {
		return analysis
	}

	// 1. Compute direct clobbers and collect call graph edges for all functions
	for _, f := range program.Functions {
		analysis.CalledFuncs[f.Name] = make(map[string]bool)
		direct := b.FunctionDirectClobbers(f)
		analysis.DirectClobbers[f.Name] = direct
		analysis.TotalClobbers[f.Name] = direct

		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				switch call := instr.(type) {
				case *ir.Call:
					if call.Func != nil {
						analysis.CalledFuncs[f.Name][call.Func.Name] = true
					}
				case *ir.IndirectCall:
					analysis.MakesIndirectCall[f.Name] = true
				case *ir.BuiltinCall:
					// Builtin calls like print / println clobber caller-saved registers
					analysis.TotalClobbers[f.Name] |= RegD | RegX | RegCC
				}
			}
		}

		// If function makes indirect calls, it may invoke anything; conservatively assume caller-saved registers
		if analysis.MakesIndirectCall[f.Name] {
			analysis.TotalClobbers[f.Name] |= RegD | RegX | RegCC
		}
	}

	// 2. Fixed-point propagation of transitive clobbers along call edges
	changed := true
	for changed {
		changed = false
		for _, f := range program.Functions {
			callerName := f.Name
			for calleeName := range analysis.CalledFuncs[callerName] {
				var calleeClobbers RegMask
				if calleeTotal, ok := analysis.TotalClobbers[calleeName]; ok {
					calleeClobbers = calleeTotal
				} else {
					// External function or runtime helper not in program.Functions
					calleeClobbers = HelperClobbers(calleeName)
				}

				if (analysis.TotalClobbers[callerName] & calleeClobbers) != calleeClobbers {
					analysis.TotalClobbers[callerName] |= calleeClobbers
					changed = true
				}
			}
		}
	}

	return analysis
}

// FunctionTotalClobbers returns the transitive physical register clobber mask for funcName.
func (b *Backend) FunctionTotalClobbers(funcName string) RegMask {
	if b.clobberAnalysis != nil {
		if mask, ok := b.clobberAnalysis.TotalClobbers[funcName]; ok {
			return mask
		}
	}
	return HelperClobbers(funcName)
}

