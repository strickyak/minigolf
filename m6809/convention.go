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
