package bigir

import (
	"fmt"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/opt"
)

// Builder constructs a BIGIR Program from AST or standard SSA IR.
type Builder struct {
	WordSize  int
	stringMap map[string]*Global
	prog      *Program
}

// NewBuilder creates a new BIGIR builder.
func NewBuilder(wordSize int) *Builder {
	return &Builder{
		WordSize:  wordSize,
		stringMap: make(map[string]*Global),
	}
}

// BuildFromAST converts an AST Program into a BIGIR Program by first lowering
// to SSA IR and then performing EMBIGGEN specialization and 8KB block packing.
func (b *Builder) BuildFromAST(astProg *ast.Program, resolveCallback func(node ast.Node, defPkg string) ast.Node) (*Program, error) {
	irBuilder := ir.NewBuilder(resolveCallback, b.WordSize)
	irProg := irBuilder.Build(astProg)
	opt.MarkMagicFunctions(irProg)

	optConfig := opt.Config{
		EnableConstFold:   true,
		EnableDBE:         true,
		EnableDCE:         true,
		EnableCopyProp:    true,
		EnableStrengthRed: true,
		EnableBranchFold:  true,
		EnableDFE:         true,
		WordSize:          b.WordSize,
	}
	opt.OptimizeProgram(irProg, optConfig)

	return b.BuildFromIR(irProg)
}

// BuildFromIR converts a standard SSA IR Program into a BIGIR Program.
func (b *Builder) BuildFromIR(irProg *ir.Program) (*Program, error) {
	prog := &Program{
		Globals:   make([]*Global, 0),
		Functions: make([]*Function, 0),
		TypeDefs:  make(map[string]Type),
	}
	b.prog = prog
	b.stringMap = make(map[string]*Global)

	// 1. Translate Globals
	globalMap := make(map[*ir.Global]*Global)
	for _, g := range irProg.Globals {
		bg := &Global{
			Name:       g.Name,
			Typ:        b.convertType(g.Typ),
			InitString: g.InitString,
			IsFar:      false, // Static globals reside in fixed Slot 0 or Slot 6
		}
		prog.Globals = append(prog.Globals, bg)
		globalMap[g] = bg
		if g.InitString != "" {
			b.stringMap[g.InitString] = bg
		}
	}

	// 2. Identify Far vs Near Functions
	funcMap := make(map[string]*Function)
	for _, fn := range irProg.Functions {
		isFar := true
		nameLower := strings.ToLower(fn.Name)

		// Core runtime and prelude helpers remain Near (Slot 6)
		if strings.HasPrefix(nameLower, "__far_") ||
			strings.HasPrefix(nameLower, "malloc_core") ||
			strings.HasPrefix(nameLower, "cstart") ||
			strings.HasPrefix(nameLower, "_div") ||
			strings.HasPrefix(nameLower, "_mul") ||
			strings.HasPrefix(nameLower, "peek") ||
			strings.HasPrefix(nameLower, "poke") {
			isFar = false
		}

		bf := &Function{
			Name:       fn.Name,
			IsFar:      isFar,
			ReturnType: b.convertType(fn.ReturnType),
			Parameters: make([]*Parameter, 0),
			Blocks:     make([]*BasicBlock, 0),
		}

		for _, p := range fn.Parameters {
			bp := &Parameter{
				ID:   p.ID,
				Name: p.Name,
				Typ:  b.convertType(p.Typ),
			}
			bf.Parameters = append(bf.Parameters, bp)
		}

		prog.Functions = append(prog.Functions, bf)
		funcMap[fn.Name] = bf
	}

	// 3. Translate Basic Blocks and Instructions for each function
	for _, fn := range irProg.Functions {
		bf := funcMap[fn.Name]
		blockMap := make(map[*ir.BasicBlock]*BasicBlock)
		valueMap := make(map[ir.Value]Value)

		// Create target BasicBlocks
		for _, bb := range fn.Blocks {
			bbb := &BasicBlock{
				ID:           bb.ID,
				Instructions: make([]Instruction, 0),
			}
			bf.Blocks = append(bf.Blocks, bbb)
			blockMap[bb] = bbb
		}

		// Map parameters
		for idx, p := range fn.Parameters {
			bp := bf.Parameters[idx]
			valueMap[p] = bp
		}

		if len(bf.Blocks) > 0 {
			bf.EntryBlock = bf.Blocks[0]
		}

		// Translate instructions within blocks
		for _, bb := range fn.Blocks {
			bbb := blockMap[bb]

			for _, instr := range bb.Instructions {
				switch ti := instr.(type) {
				case *ir.Cast:
					valueMap[instr] = b.resolveVal(ti.Operand, valueMap, globalMap)
					continue
				case *ir.AddressOfElement:
					valueMap[instr] = b.resolveVal(ti.ArrayPtr, valueMap, globalMap)
					continue
				case *ir.ExtractField:
					base := b.resolveVal(ti.Struct, valueMap, globalMap)
					if sm, ok := base.(*SliceMake); ok {
						switch ti.FieldIndex {
						case 0:
							valueMap[instr] = sm.Offset
						case 1:
							valueMap[instr] = sm.Length
						case 2:
							valueMap[instr] = sm.Capacity
						default:
							valueMap[instr] = sm.FarRef
						}
						continue
					}
				}

				binstr := b.convertInstruction(instr, valueMap, globalMap, blockMap, funcMap)
				if binstr != nil {
					binstr.SetID(instr.GetID())
					binstr.SetComment(instr.GetComment())
					bbb.Instructions = append(bbb.Instructions, binstr)
					valueMap[instr] = binstr
				}
			}

			// Translate Terminator
			if bb.Terminator != nil {
				bterm := b.convertTerminator(bb.Terminator, bf.IsFar, valueMap, blockMap)
				if bterm != nil {
					bterm.SetID(bb.Terminator.GetID())
					bterm.SetComment(bb.Terminator.GetComment())
					bbb.Terminator = bterm
				}
			}
		}
	}

	// 4. Perform 8KB Block Packing
	if err := PackProgram(prog); err != nil {
		return nil, err
	}

	return prog, nil
}

func (b *Builder) convertType(irt ir.Type) Type {
	if irt.IsASlice() {
		elemTyp := b.convertType(irt.SliceElementType())
		return MakeFarSlice(elemTyp)
	}
	if irt.IsAPointer() {
		elemTyp := b.convertType(irt.PointedType())
		return MakeNearPtr(elemTyp)
	}

	switch irt.Name {
	case "byte":
		return TypeByte
	case "word":
		return TypeWord
	case "int":
		return TypeInt
	case "bool":
		return TypeBool
	case "void":
		return TypeVoid
	case "noreturn":
		return TypeNoReturn
	case "string":
		return TypeFarString
	default:
		return Type{Kind: KindWord, Name: irt.Name, Size: 2}
	}
}

func (b *Builder) convertInstruction(
	instr ir.Instruction,
	valueMap map[ir.Value]Value,
	globalMap map[*ir.Global]*Global,
	blockMap map[*ir.BasicBlock]*BasicBlock,
	funcMap map[string]*Function,
) Instruction {
	switch i := instr.(type) {
	case *ir.ConstByte:
		return &ConstByte{
			BaseInstruction: BaseInstruction{Typ: TypeByte},
			Val:             i.Val,
		}

	case *ir.ConstWord:
		return &ConstWord{
			BaseInstruction: BaseInstruction{Typ: TypeWord},
			Val:             i.Val,
		}

	case *ir.BinaryOp:
		left := b.resolveVal(i.Left, valueMap, globalMap)
		right := b.resolveVal(i.Right, valueMap, globalMap)
		return &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Op:              i.Op,
			Left:            left,
			Right:           right,
		}

	case *ir.UnaryOp:
		op := b.resolveVal(i.Operand, valueMap, globalMap)
		return &UnaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Op:              i.Op,
			Operand:         op,
		}

	case *ir.Compare:
		left := b.resolveVal(i.Left, valueMap, globalMap)
		right := b.resolveVal(i.Right, valueMap, globalMap)
		return &Compare{
			BaseInstruction: BaseInstruction{Typ: TypeBool},
			Op:              i.Op,
			Left:            left,
			Right:           right,
		}

	case *ir.Load:
		bg := globalMap[i.Global]
		addr := &AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)}, Global: bg}
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: bg.Typ},
			Addr:            addr,
		}

	case *ir.Store:
		bg := globalMap[i.Global]
		addr := &AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)}, Global: bg}
		val := b.resolveVal(i.Val, valueMap, globalMap)
		return &NearStore{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Addr:            addr,
			Val:             val,
		}

	case *ir.LoadPtr:
		ptr := b.resolveVal(i.Ptr, valueMap, globalMap)
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Addr:            ptr,
		}

	case *ir.StorePtr:
		ptr := b.resolveVal(i.Ptr, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		return &NearStore{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Addr:            ptr,
			Val:             val,
		}

	case *ir.AddressOfGlobal:
		bg := globalMap[i.Global]
		return &AddressOfGlobal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)},
			Global:          bg,
		}

	case *ir.AddressOfLocal:
		loc := b.resolveVal(i.Local, valueMap, globalMap)
		return &AddressOfLocal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(loc.Type())},
			Local:           loc,
		}

	case *ir.Call:
		args := make([]Value, len(i.Args))
		for idx, a := range i.Args {
			args[idx] = b.resolveVal(a, valueMap, globalMap)
		}

		calleeName := i.Func.Name
		calleeFn := funcMap[calleeName]

		if calleeFn != nil && calleeFn.IsFar {
			return &FarCall{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
				Callee:          calleeName,
				Args:            args,
			}
		}

		return &NearCall{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Callee:          calleeName,
			Args:            args,
		}

	case *ir.BuiltinCall:
		args := make([]Value, len(i.Args))
		for idx, a := range i.Args {
			args[idx] = b.resolveVal(a, valueMap, globalMap)
		}
		// Map println/print builtins to runtime NearCalls
		return &NearCall{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Callee:          "builtin_" + i.Name,
			Args:            args,
		}

	case *ir.ZeroInit:
		typ := b.convertType(i.Typ)
		if typ.Kind == KindFarSlice || typ.Kind == KindFarString {
			return &SliceMake{
				BaseInstruction: BaseInstruction{Typ: typ},
				FarRef:          &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
				Offset:          &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
				Length:          &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
				Capacity:        &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
			}
		}
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: typ}, Val: 0}

	case *ir.InsertField:
		base := b.resolveVal(i.Struct, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		if sm, ok := base.(*SliceMake); ok {
			newSm := &SliceMake{
				BaseInstruction: BaseInstruction{Typ: sm.Typ},
				FarRef:          sm.FarRef,
				Offset:          sm.Offset,
				Length:          sm.Length,
				Capacity:        sm.Capacity,
			}
			switch i.FieldIndex {
			case 0:
				newSm.Offset = val
			case 1:
				newSm.Length = val
			case 2:
				newSm.Capacity = val
			}
			return newSm
		}
		return nil

	default:
		// Fallback for untyped or unsupported operations
		return nil
	}
}

func (b *Builder) convertTerminator(
	term ir.Terminator,
	isFar bool,
	valueMap map[ir.Value]Value,
	blockMap map[*ir.BasicBlock]*BasicBlock,
) Terminator {
	switch t := term.(type) {
	case *ir.Return:
		var val Value
		if t.Val != nil {
			val = b.resolveVal(t.Val, valueMap, nil)
		}
		if isFar {
			return &FarReturn{
				BaseInstruction: BaseInstruction{Typ: TypeNoReturn},
				Val:             val,
			}
		}
		return &Return{
			BaseInstruction: BaseInstruction{Typ: TypeNoReturn},
			Val:             val,
		}

	case *ir.Jump:
		target := blockMap[t.Target]
		return &Branch{
			BaseInstruction: BaseInstruction{Typ: TypeNoReturn},
			Target:          target,
		}

	case *ir.Branch:
		cond := b.resolveVal(t.Condition, valueMap, nil)
		trueTarget := blockMap[t.TrueBlock]
		falseTarget := blockMap[t.FalseBlock]
		return &CondBranch{
			BaseInstruction: BaseInstruction{Typ: TypeNoReturn},
			Cond:            cond,
			TrueTarget:      trueTarget,
			FalseTarget:     falseTarget,
		}

	default:
		return nil
	}
}

func (b *Builder) resolveVal(v ir.Value, valueMap map[ir.Value]Value, globalMap map[*ir.Global]*Global) Value {
	if v == nil {
		return nil
	}
	if mapped, exists := valueMap[v]; exists {
		return mapped
	}
	switch val := v.(type) {
	case *ir.Phi:
		if len(val.Edges) > 0 {
			return b.resolveVal(val.Edges[0].Value, valueMap, globalMap)
		}
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}
	case *ir.Cast:
		return b.resolveVal(val.Operand, valueMap, globalMap)
	case *ir.AddressOfElement:
		return b.resolveVal(val.ArrayPtr, valueMap, globalMap)
	case *ir.ExtractField:
		base := b.resolveVal(val.Struct, valueMap, globalMap)
		if sm, ok := base.(*SliceMake); ok {
			switch val.FieldIndex {
			case 0:
				return sm.Offset
			case 1:
				return sm.Length
			case 2:
				return sm.Capacity
			default:
				return sm.FarRef
			}
		}
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}
	case *ir.ConstByte:
		return &ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: val.Val}
	case *ir.ConstWord:
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: val.Val}
	case *ir.Global:
		if globalMap != nil {
			if bg, ok := globalMap[val]; ok {
				return bg
			}
		}
		return &Global{Name: val.Name, Typ: b.convertType(val.Typ), InitString: val.InitString}
	case *ir.StringLiteral:
		// Return 8-byte near string literal descriptor:
		// far_ref = 0, offset = &str, length = len, capacity = len
		strGlobal, ok := b.stringMap[val.Value]
		if !ok {
			strGlobal = &Global{
				Name:       fmt.Sprintf("_str_lit_%d", len(b.stringMap)),
				Typ:        TypeByte,
				InitString: val.Value,
				IsFar:      false,
			}
			b.stringMap[val.Value] = strGlobal
			if b.prog != nil {
				b.prog.Globals = append(b.prog.Globals, strGlobal)
			}
		}
		addr := &AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: MakeNearPtr(TypeByte)}, Global: strGlobal}
		return &SliceMake{
			BaseInstruction: BaseInstruction{Typ: TypeFarString},
			FarRef:          &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
			Offset:          addr,
			Length:          &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(len(val.Value))},
			Capacity:        &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(len(val.Value))},
		}
	default:
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}
	}
}
