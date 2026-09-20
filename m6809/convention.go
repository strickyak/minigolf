package m6809

import (
	"fmt"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

// ParamLocationKind specifies how a parameter is passed.
type ParamLocationKind int

const (
	LocStack ParamLocationKind = iota // Parameter is passed on the stack
	LocReg                            // Parameter is passed in a hardware register
)

func (k ParamLocationKind) String() string {
	switch k {
	case LocStack:
		return "stack"
	case LocReg:
		return "reg"
	default:
		return "unknown"
	}
}

// ParamLocation describes where a single parameter is passed.
type ParamLocation struct {
	Kind   ParamLocationKind
	Reg    string // "d", "b", "x"
	Offset int    // byte offset for LocStack (0 for first stack argument)
	Size   int    // byte size (1, 2, or >2)
}

// FunctionConvention describes the calling convention for a specific function.
type FunctionConvention struct {
	FuncName           string
	Params             []ParamLocation
	NumRegParams       int
	TotalStackArgBytes int
	IsFastcall         bool
}

// ConventionPolicy determines the calling convention for functions in a program.
type ConventionPolicy interface {
	Name() string
	GetConvention(f *ir.Function, b *Backend) *FunctionConvention
}

// ProgramConventionInitializer is an optional interface that a ConventionPolicy
// can implement to perform whole-program, bottom-up analysis across all functions.
type ProgramConventionInitializer interface {
	InitConventions(program *ir.Program, b *Backend)
}

// StackPolicy implements the legacy stack-only calling convention where all
// parameters are pushed onto the hardware stack.
type StackPolicy struct{}

func (p *StackPolicy) Name() string { return "stack" }

func (p *StackPolicy) GetConvention(f *ir.Function, b *Backend) *FunctionConvention {
	conv := &FunctionConvention{
		FuncName: f.Name,
		Params:   make([]ParamLocation, len(f.Parameters)),
	}
	offset := 0
	for idx, param := range f.Parameters {
		sz := b.getTypeSizeByType(param.Typ)
		conv.Params[idx] = ParamLocation{
			Kind:   LocStack,
			Offset: offset,
			Size:   sz,
		}
		offset += align(sz)
	}
	conv.TotalStackArgBytes = offset
	return conv
}

// FastcallPolicy implements register parameter passing for whole-program compilation:
// - Parameter 0: In accumulator 'd' (16-bit) or 'b' (8-bit)
// - Parameter 1: In index register 'x' (if 16-bit)
// - Remaining parameters (or size > 2): on the stack.
// Functions with external linkage (except runtime putchar), address-taken functions,
// and bodyless declarations use StackPolicy.
type FastcallPolicy struct{}

func (p *FastcallPolicy) Name() string { return "fastcall" }

func (p *FastcallPolicy) GetConvention(f *ir.Function, b *Backend) *FunctionConvention {
	conv := &FunctionConvention{
		FuncName: f.Name,
		Params:   make([]ParamLocation, len(f.Parameters)),
	}

	// 1. External functions or linkage overrides (except known runtime helpers like putchar)
	if f.Linkage != "" && f.Linkage != "putchar" && f.Linkage != "_putchar" {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	// 2. Functions whose address is taken (might be called via indirect calls)
	if b.funcAddressTaken != nil && b.funcAddressTaken[f.Name] {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	// 3. Declarations without body (external)
	if len(f.Blocks) == 0 && f.EmitName() != "putchar" && f.EmitName() != "_putchar" {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	stackOffset := 0
	for idx, param := range f.Parameters {
		sz := b.getTypeSizeByType(param.Typ)
		if idx == 0 && (sz == 1 || sz == 2) {
			reg := "d"
			if sz == 1 {
				reg = "b"
			}
			conv.Params[idx] = ParamLocation{
				Kind: LocReg,
				Reg:  reg,
				Size: sz,
			}
			conv.NumRegParams++
			conv.IsFastcall = true
		} else if idx == 1 && sz == 2 {
			conv.Params[idx] = ParamLocation{
				Kind: LocReg,
				Reg:  "x",
				Size: sz,
			}
			conv.NumRegParams++
			conv.IsFastcall = true
		} else {
			conv.Params[idx] = ParamLocation{
				Kind:   LocStack,
				Offset: stackOffset,
				Size:   sz,
			}
			stackOffset += align(sz)
		}
	}
	conv.TotalStackArgBytes = stackOffset
	return conv
}

// GCCPolicy implements the GCC 6809 calling convention:
// The first 2-byte argument is passed in X; the first 1-byte argument is passed in B;
// remaining arguments on the stack.
type GCCPolicy struct{}

func (p *GCCPolicy) Name() string { return "gcc" }

func (p *GCCPolicy) GetConvention(f *ir.Function, b *Backend) *FunctionConvention {
	conv := &FunctionConvention{
		FuncName: f.Name,
		Params:   make([]ParamLocation, len(f.Parameters)),
	}

	usedX := false
	usedB := false
	stackOffset := 0

	for idx, param := range f.Parameters {
		sz := b.getTypeSizeByType(param.Typ)
		if sz == 2 && !usedX {
			conv.Params[idx] = ParamLocation{
				Kind: LocReg,
				Reg:  "x",
				Size: sz,
			}
			usedX = true
			conv.NumRegParams++
			conv.IsFastcall = true
		} else if sz == 1 && !usedB {
			conv.Params[idx] = ParamLocation{
				Kind: LocReg,
				Reg:  "b",
				Size: sz,
			}
			usedB = true
			conv.NumRegParams++
			conv.IsFastcall = true
		} else {
			conv.Params[idx] = ParamLocation{
				Kind:   LocStack,
				Offset: stackOffset,
				Size:   sz,
			}
			stackOffset += align(sz)
		}
	}
	conv.TotalStackArgBytes = stackOffset
	return conv
}

// String returns a human-readable summary of the function's calling convention.
func (c *FunctionConvention) String() string {
	var parts []string
	for idx, p := range c.Params {
		if p.Kind == LocReg {
			parts = append(parts, fmt.Sprintf("arg%d in %s", idx, strings.ToUpper(p.Reg)))
		} else {
			parts = append(parts, fmt.Sprintf("arg%d at stack[%d]", idx, p.Offset))
		}
	}
	return fmt.Sprintf("%s(%s) [stackBytes=%d]", c.FuncName, strings.Join(parts, ", "), c.TotalStackArgBytes)
}

// computeLeafLevels performs bottom-up leaf level calculation if not already annotated.
func computeLeafLevels(program *ir.Program) {
	if program == nil {
		return
	}
	makesIndirectCall := make(map[*ir.Function]bool)
	calledFuncs := make(map[*ir.Function]map[*ir.Function]bool)

	for _, f := range program.Functions {
		calledFuncs[f] = make(map[*ir.Function]bool)
		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				if _, ok := instr.(*ir.IndirectCall); ok {
					makesIndirectCall[f] = true
				} else if call, ok := instr.(*ir.Call); ok {
					calledFuncs[f][call.Func] = true
				}
			}
		}
	}

	for level := 1; level <= 10; level++ {
		madeProgress := false
		for _, f := range program.Functions {
			if f.LeafLevel != 0 || makesIndirectCall[f] {
				continue
			}
			allResolved := true
			maxLevel := 0
			for called := range calledFuncs[f] {
				if called.LeafLevel == 0 {
					allResolved = false
					break
				}
				if called.LeafLevel > maxLevel {
					maxLevel = called.LeafLevel
				}
			}
			if allResolved && maxLevel == level-1 {
				f.LeafLevel = level
				madeProgress = true
			}
		}
		if !madeProgress {
			break
		}
	}
}

// AdaptivePolicy assigns optimal calling conventions to functions by analyzing them
// bottom-up from leaf functions upward. Leaf functions get registers best aligned with
// their usage (e.g. pointers in X, arithmetic in D/B), and higher-level functions inherit
// these preferences as interpolation points to minimize register moves across call sites.
type AdaptivePolicy struct{}

func (p *AdaptivePolicy) Name() string { return "adaptive" }

func (p *AdaptivePolicy) InitConventions(program *ir.Program, b *Backend) {
	if program == nil {
		return
	}

	// 1. Ensure LeafLevel annotations exist
	hasAnnotated := false
	for _, f := range program.Functions {
		if f.LeafLevel > 0 {
			hasAnnotated = true
			break
		}
	}
	if !hasAnnotated {
		computeLeafLevels(program)
	}

	// 2. Sort functions bottom-up: LeafLevel == 1, 2, ..., then 0 (recursive / cycles)
	maxLevel := 0
	for _, f := range program.Functions {
		if f.LeafLevel > maxLevel {
			maxLevel = f.LeafLevel
		}
	}
	var sortedFuncs []*ir.Function
	for lvl := 1; lvl <= maxLevel; lvl++ {
		for _, f := range program.Functions {
			if f.LeafLevel == lvl {
				sortedFuncs = append(sortedFuncs, f)
			}
		}
	}
	for _, f := range program.Functions {
		if f.LeafLevel == 0 {
			sortedFuncs = append(sortedFuncs, f)
		}
	}

	// 3. Compute convention for each function in bottom-up order
	for _, f := range sortedFuncs {
		conv := p.computeConvention(f, b)
		b.conventions[f.Name] = conv
		b.conventions[f.EmitName()] = conv
	}
}

func (p *AdaptivePolicy) GetConvention(f *ir.Function, b *Backend) *FunctionConvention {
	if b.conventions != nil {
		if conv, ok := b.conventions[f.Name]; ok {
			return conv
		}
		if conv, ok := b.conventions[f.EmitName()]; ok {
			return conv
		}
	}
	return p.computeConvention(f, b)
}

func (p *AdaptivePolicy) computeConvention(f *ir.Function, b *Backend) *FunctionConvention {
	conv := &FunctionConvention{
		FuncName: f.Name,
		Params:   make([]ParamLocation, len(f.Parameters)),
	}
	if b.conventions != nil {
		b.conventions[f.Name] = conv
		b.conventions[f.EmitName()] = conv
	}

	// 1. External functions or linkage overrides (except known runtime helpers like putchar)
	if f.Linkage != "" && f.Linkage != "putchar" && f.Linkage != "_putchar" {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	// 2. Functions whose address is taken (might be called via indirect calls)
	if b.funcAddressTaken != nil && b.funcAddressTaken[f.Name] {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	// 3. Declarations without body (external)
	if len(f.Blocks) == 0 && f.EmitName() != "putchar" && f.EmitName() != "_putchar" {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	// 4. Runtime putchar helper
	if f.EmitName() == "putchar" || f.Linkage == "putchar" || f.Linkage == "_putchar" {
		if len(f.Parameters) > 0 {
			sz := b.getTypeSizeByType(f.Parameters[0].Typ)
			conv.Params[0] = ParamLocation{Kind: LocReg, Reg: "b", Size: sz}
			conv.NumRegParams = 1
			conv.IsFastcall = true
			return conv
		}
	}

	// 5. Main entrypoint uses stack
	if f.Name == "main" || f.EmitName() == "main" || f.Name == "f_main__main" {
		return (&StackPolicy{}).GetConvention(f, b)
	}

	if len(f.Parameters) == 0 {
		return conv
	}

	// Score each parameter
	type paramScore struct {
		param  *ir.Parameter
		idx    int
		sz     int
		scoreX int
		scoreD int
	}

	scores := make([]paramScore, len(f.Parameters))
	for idx, param := range f.Parameters {
		sz := b.getTypeSizeByType(param.Typ)
		scores[idx] = paramScore{
			param: param,
			idx:   idx,
			sz:    sz,
		}
		if sz > 2 {
			continue
		}

		// A. Type hints
		if param.Typ.IsAPointer() || param.Typ.IsAnArray() || param.Typ.IsASlice() {
			scores[idx].scoreX += 15
		}
		if sz == 1 {
			scores[idx].scoreD += 15 // Prefers accumulator B
		}

		// B. Instruction usage in function body
		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				switch inst := instr.(type) {
				case *ir.LoadPtr:
					if b.resolveVal(inst.Ptr) == param {
						scores[idx].scoreX += 20
					}
				case *ir.StorePtr:
					if b.resolveVal(inst.Ptr) == param {
						scores[idx].scoreX += 20
					}
					if b.resolveVal(inst.Val) == param {
						scores[idx].scoreD += 5
					}
				case *ir.AddressOfElement:
					if b.resolveVal(inst.ArrayPtr) == param {
						scores[idx].scoreX += 15
					}
				case *ir.ExtractFieldPtr:
					if b.resolveVal(inst.Ptr) == param {
						scores[idx].scoreX += 15
					}
				case *ir.BinaryOp:
					if b.resolveVal(inst.Left) == param || b.resolveVal(inst.Right) == param {
						scores[idx].scoreD += 10
					}
				case *ir.Compare:
					if b.resolveVal(inst.Left) == param || b.resolveVal(inst.Right) == param {
						scores[idx].scoreD += 8
					}
				case *ir.UnaryOp:
					if b.resolveVal(inst.Operand) == param {
						scores[idx].scoreD += 8
					}
				case *ir.Call:
					if inst.Func == nil || inst.Func.Name == f.Name {
						continue
					}
					// Bottom-Up Interpolation Point: Callee argument convention
					for argIdx, arg := range inst.Args {
						if b.resolveVal(arg) == param {
							if b.conventions != nil {
								if calleeConv, ok := b.conventions[inst.Func.Name]; ok {
									if argIdx < len(calleeConv.Params) {
										loc := calleeConv.Params[argIdx]
										if loc.Kind == LocReg {
											if loc.Reg == "x" {
												scores[idx].scoreX += 30 // Direct interpolation match!
											} else if loc.Reg == "d" || loc.Reg == "b" {
												scores[idx].scoreD += 30 // Direct interpolation match!
											}
										}
									}
								}
							}
						}
					}
				}
			}
			if blk.Terminator != nil {
				if ret, ok := blk.Terminator.(*ir.Return); ok {
					if b.resolveVal(ret.Val) == param {
						scores[idx].scoreD += 25
					}
				}
			}
		}
	}

	// Assign registers based on scores:
	// We have at most one register for X, and at most one register for D (or B).
	assignedReg := make(map[int]string) // idx -> "x", "d", "b"

	if len(f.Parameters) == 1 {
		p0 := scores[0]
		if p0.sz == 1 {
			assignedReg[0] = "b"
		} else if p0.sz == 2 {
			if p0.scoreX > p0.scoreD {
				assignedReg[0] = "x"
			} else {
				assignedReg[0] = "d"
			}
		}
	} else if len(f.Parameters) == 2 {
		p0 := scores[0]
		p1 := scores[1]
		if p0.sz <= 2 && p1.sz <= 2 {
			if p0.sz == 1 && p1.sz == 1 {
				assignedReg[0] = "b"
			} else if p0.sz == 1 && p1.sz == 2 {
				assignedReg[0] = "b"
				assignedReg[1] = "x"
			} else if p0.sz == 2 && p1.sz == 1 {
				assignedReg[0] = "x"
				assignedReg[1] = "b"
			} else if p0.sz == 2 && p1.sz == 2 {
				diff0 := p0.scoreX - p0.scoreD
				diff1 := p1.scoreX - p1.scoreD
				if diff0 >= diff1 {
					assignedReg[0] = "x"
					assignedReg[1] = "d"
				} else {
					assignedReg[0] = "d"
					assignedReg[1] = "x"
				}
			}
		} else if p0.sz <= 2 {
			if p0.sz == 1 {
				assignedReg[0] = "b"
			} else if p0.scoreX > p0.scoreD {
				assignedReg[0] = "x"
			} else {
				assignedReg[0] = "d"
			}
		} else if p1.sz <= 2 {
			if p1.sz == 1 {
				assignedReg[1] = "b"
			} else if p1.scoreX > p1.scoreD {
				assignedReg[1] = "x"
			} else {
				assignedReg[1] = "d"
			}
		}
	} else {
		// >= 3 parameters: pick best candidate for X, and best candidate for D/B
		bestXIdx := -1
		bestXDiff := -9999
		for idx, p := range scores {
			if p.sz == 2 {
				diff := p.scoreX - p.scoreD
				if diff > bestXDiff {
					bestXDiff = diff
					bestXIdx = idx
				}
			}
		}
		if bestXIdx != -1 && scores[bestXIdx].scoreX > 0 {
			assignedReg[bestXIdx] = "x"
		}

		bestDIdx := -1
		bestDScore := -9999
		for idx, p := range scores {
			if idx == bestXIdx || p.sz > 2 {
				continue
			}
			if p.scoreD > bestDScore {
				bestDScore = p.scoreD
				bestDIdx = idx
			}
		}
		if bestDIdx != -1 {
			if scores[bestDIdx].sz == 1 {
				assignedReg[bestDIdx] = "b"
			} else {
				assignedReg[bestDIdx] = "d"
			}
		}
	}

	stackOffset := 0
	for idx, param := range f.Parameters {
		sz := b.getTypeSizeByType(param.Typ)
		if reg, ok := assignedReg[idx]; ok {
			conv.Params[idx] = ParamLocation{
				Kind: LocReg,
				Reg:  reg,
				Size: sz,
			}
			conv.NumRegParams++
			conv.IsFastcall = true
		} else {
			conv.Params[idx] = ParamLocation{
				Kind:   LocStack,
				Offset: stackOffset,
				Size:   sz,
			}
			stackOffset += align(sz)
		}
	}
	conv.TotalStackArgBytes = stackOffset
	return conv
}
