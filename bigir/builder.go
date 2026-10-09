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
	WordSize        int
	stringMap       map[string]*Global
	globalMap       map[*ir.Global]*Global
	globalByName    map[string]*Global
	typeMap         map[string]Type
	convertingTypes map[string]bool
	prog            *Program
	irProg          *ir.Program
}

// NewBuilder creates a new BIGIR builder.
func NewBuilder(wordSize int) *Builder {
	return &Builder{
		WordSize:        wordSize,
		stringMap:       make(map[string]*Global),
		globalMap:       make(map[*ir.Global]*Global),
		globalByName:    make(map[string]*Global),
		typeMap:         make(map[string]Type),
		convertingTypes: make(map[string]bool),
	}
}

func (b *Builder) lookupGlobal(g *ir.Global) *Global {
	if g == nil {
		return nil
	}
	if bg, ok := b.globalMap[g]; ok {
		return bg
	}
	if bg, ok := b.globalByName[g.Name]; ok {
		return bg
	}
	bg := &Global{
		Name:       g.Name,
		Typ:        b.convertType(g.Typ),
		InitString: g.InitString,
		IsFar:      false,
	}
	if b.globalMap != nil {
		b.globalMap[g] = bg
	}
	if b.globalByName != nil {
		b.globalByName[g.Name] = bg
	}
	if g.InitVal != nil {
		bg.InitVal = b.resolveVal(g.InitVal, nil, b.globalMap)
	}
	if b.prog != nil {
		b.prog.Globals = append(b.prog.Globals, bg)
	}
	return bg
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
	b.irProg = irProg
	b.stringMap = make(map[string]*Global)
	b.globalMap = make(map[*ir.Global]*Global)
	b.globalByName = make(map[string]*Global)

	// 1. Translate Globals
	globalMap := b.globalMap
	for _, g := range irProg.Globals {
		bg := &Global{
			Name:       g.Name,
			Typ:        b.convertType(g.Typ),
			InitString: g.InitString,
			IsFar:      false, // Static globals reside in fixed Slot 0 or Slot 6
		}
		prog.Globals = append(prog.Globals, bg)
		b.globalMap[g] = bg
		b.globalByName[g.Name] = bg
		if g.InitString != "" {
			b.stringMap[g.InitString] = bg
		}
	}

	for _, g := range irProg.Globals {
		if g.InitVal != nil {
			bg := b.globalMap[g]
			bg.InitVal = b.resolveVal(g.InitVal, nil, b.globalMap)
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
			strings.HasPrefix(nameLower, "poke") ||
			strings.HasSuffix(nameLower, "putchar") ||
			strings.HasSuffix(nameLower, "getchar") {
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
					if ti.Op == "word_to_ptr" {
						if ef, ok := ti.Operand.(*ir.ExtractField); ok {
							structTyp := ef.Struct.Type()
							if structTyp.IsASlice() || structTyp.Name == "string" || structTyp.Name == "prelude.string" || structTyp.Name == "slice_byte" || structTyp.Name == "prelude.slice_byte" {
								sliceVal := b.resolveVal(ef.Struct, valueMap, globalMap)
								if sm, ok := sliceVal.(*SliceMake); ok {
									if cw, ok := sm.FarRef.(*ConstWord); ok && cw.Val == 0 {
										valueMap[instr] = sm.Offset
										continue
									}
								}
								break
							}
						}
					}
					targetTyp := b.convertType(ti.Type())
					op := b.resolveVal(ti.Operand, valueMap, globalMap)
					if targetTyp.Size == 1 && op.Type().Size > 1 {
						break
					}
					valueMap[instr] = op
					continue
				case *ir.ExtractField:
					base := b.resolveVal(ti.Struct, valueMap, globalMap)
					if sm, ok := base.(*SliceMake); ok {
						switch ti.FieldIndex {
						case 0:
							valueMap[instr] = sm.FarRef
						case 1:
							valueMap[instr] = sm.Offset
						case 2:
							valueMap[instr] = sm.Length
						case 3:
							valueMap[instr] = sm.Capacity
						default:
							valueMap[instr] = sm.FarRef
						}
						continue
					}
					if cs, ok := base.(*ConstStruct); ok && ti.FieldIndex < len(cs.Fields) {
						valueMap[instr] = cs.Fields[ti.FieldIndex]
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

		// Populate edges of Phi instructions
		for _, bb := range fn.Blocks {
			for _, instr := range bb.Instructions {
				if irPhi, ok := instr.(*ir.Phi); ok {
					if bigPhi, ok := valueMap[irPhi].(*Phi); ok {
						for _, edge := range irPhi.Edges {
							predBlock := blockMap[edge.Block]
							val := b.resolveVal(edge.Value, valueMap, globalMap)
							bigPhi.Edges = append(bigPhi.Edges, PhiEdge{
								Block: predBlock,
								Value: val,
							})
						}
					}
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
	if b.typeMap != nil && irt.Name != "" {
		if cached, ok := b.typeMap[irt.Name]; ok {
			return cached
		}
	}
	if b.convertingTypes != nil && irt.Name != "" && b.convertingTypes[irt.Name] {
		// Recursive type reference (e.g. *Frame inside Frame)
		return Type{Kind: KindWord, Name: irt.Name, Size: 2}
	}

	if irt.IsASlice() {
		elemTyp := b.convertType(irt.SliceElementType())
		res := MakeFarSlice(elemTyp)
		if b.typeMap != nil && irt.Name != "" {
			b.typeMap[irt.Name] = res
		}
		return res
	}
	if irt.IsAPointer() {
		elemTyp := b.convertType(irt.PointedType())
		res := MakeNearPtr(elemTyp)
		if b.typeMap != nil && irt.Name != "" {
			b.typeMap[irt.Name] = res
		}
		return res
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
		if irt.Name != "" && b.convertingTypes != nil {
			b.convertingTypes[irt.Name] = true
			defer func() {
				delete(b.convertingTypes, irt.Name)
			}()
		}
		fields := irt.FieldsOfStruct()
		if len(fields) == 0 && b.irProg != nil && b.irProg.TypeDefs != nil {
			if def, ok := b.irProg.TypeDefs[irt.Name]; ok {
				fields = def.FieldsOfStruct()
			}
		}
		if len(fields) > 0 {
			var bFields []Field
			byteOffset := 0
			for _, f := range fields {
				ft := b.convertType(f.Type)
				bFields = append(bFields, Field{
					Name:   f.Name,
					Type:   ft,
					Offset: byteOffset,
				})
				byteOffset += ft.Size
			}
			res := Type{
				Kind:   KindStruct,
				Name:   irt.Name,
				Size:   b.getTypeSize(irt),
				Fields: bFields,
			}
			if b.typeMap != nil && irt.Name != "" {
				b.typeMap[irt.Name] = res
			}
			return res
		}
		res := Type{Kind: KindWord, Name: irt.Name, Size: b.getTypeSize(irt)}
		if b.typeMap != nil && irt.Name != "" {
			b.typeMap[irt.Name] = res
		}
		return res
	}
}

func (b *Builder) getTypeSize(irt ir.Type) int {
	if irt.IsAPointer() {
		return 2
	}
	if irt.IsASlice() || irt.Name == "string" {
		return 8
	}
	if irt.IsAnArray() {
		et := irt.ArrayElementType()
		length := irt.ArrayLength()
		return length * b.getTypeSize(et)
	}
	if irt.IsAStruct() {
		fields := irt.FieldsOfStruct()
		if len(fields) > 0 {
			size := 0
			for _, f := range fields {
				size += b.getTypeSize(f.Type)
			}
			return size
		}
	}
	switch irt.Name {
	case "void", "byte", "bool":
		return 1
	case "word", "int", "const_integer", "uint", "noreturn":
		return 2
	}
	if irt.IsAFuncPtr() || irt.Name == "func" {
		return 2
	}
	if b.irProg != nil && b.irProg.TypeDefs != nil {
		if def, ok := b.irProg.TypeDefs[irt.Name]; ok {
			return b.getTypeSize(def)
		}
	}
	return 2
}

func (b *Builder) getFieldOffsetAndSize(structTyp ir.Type, fieldIndex int) (int, int) {
	fields := structTyp.FieldsOfStruct()
	if len(fields) == 0 && b.irProg != nil && b.irProg.TypeDefs != nil {
		if def, ok := b.irProg.TypeDefs[structTyp.Name]; ok {
			fields = def.FieldsOfStruct()
		}
	}
	if len(fields) == 0 && (structTyp.IsASlice() || structTyp.Name == "string" || strings.HasPrefix(structTyp.Name, "slice") || strings.HasPrefix(structTyp.Name, "prelude.slice")) {
		return fieldIndex * 2, 2
	}
	byteOffset := 0
	fieldSize := 2
	for i, f := range fields {
		sz := b.getTypeSize(f.Type)
		if i < fieldIndex {
			byteOffset += sz
		} else if i == fieldIndex {
			fieldSize = sz
			break
		}
	}
	return byteOffset, fieldSize
}

func (b *Builder) getElementSize(arrayPtrTyp ir.Type) int {
	if arrayPtrTyp.IsAPointer() {
		pt := arrayPtrTyp.PointedType()
		if pt.IsAnArray() {
			et := pt.ArrayElementType()
			return b.getTypeSize(et)
		}
		return b.getTypeSize(pt)
	}
	if arrayPtrTyp.IsAnArray() {
		et := arrayPtrTyp.ArrayElementType()
		return b.getTypeSize(et)
	}
	return 1
}

func (b *Builder) resolvePtrVal(v ir.Value, valueMap map[ir.Value]Value, globalMap map[*ir.Global]*Global) Value {
	if g, ok := v.(*ir.Global); ok {
		bg := b.lookupGlobal(g)
		return &AddressOfGlobal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)},
			Global:          bg,
		}
	}
	return b.resolveVal(v, valueMap, globalMap)
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

	case *ir.Sizeof:
		sz := b.getTypeSize(i.TargetTyp)
		return &ConstWord{
			BaseInstruction: BaseInstruction{Typ: TypeWord},
			Val:             uint64(sz),
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

	case *ir.Phi:
		return &Phi{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Edges:           make([]PhiEdge, 0, len(i.Edges)),
		}

	case *ir.Load:
		bg := b.lookupGlobal(i.Global)
		addr := &AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)}, Global: bg}
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: bg.Typ},
			Addr:            addr,
		}

	case *ir.Store:
		bg := b.lookupGlobal(i.Global)
		addr := &AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)}, Global: bg}
		val := b.resolveVal(i.Val, valueMap, globalMap)
		return &NearStore{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Addr:            addr,
			Val:             val,
		}

	case *ir.LoadPtr:
		ptr := b.resolvePtrVal(i.Ptr, valueMap, globalMap)
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Addr:            ptr,
		}

	case *ir.StorePtr:
		ptr := b.resolvePtrVal(i.Ptr, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		return &NearStore{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Addr:            ptr,
			Val:             val,
		}

	case *ir.AddressOfGlobal:
		bg := b.lookupGlobal(i.Global)
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

	case *ir.AddressOfField:
		ptr := b.resolvePtrVal(i.Ptr, valueMap, globalMap)
		structTyp := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structTyp, i.FieldIndex)
		return &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Op:              "+",
			Left:            ptr,
			Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(byteOffset)},
		}

	case *ir.AddressOfElement:
		arrayPtr := b.resolvePtrVal(i.ArrayPtr, valueMap, globalMap)
		eltSize := b.getElementSize(i.ArrayPtr.Type())
		if i.Index == nil {
			return &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
				Op:              "+",
				Left:            arrayPtr,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0},
			}
		}
		index := b.resolveVal(i.Index, valueMap, globalMap)
		offsetVal := index
		if eltSize > 1 {
			offsetVal = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeWord},
				Op:              "*",
				Left:            index,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(eltSize)},
			}
		}
		return &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Op:              "+",
			Left:            arrayPtr,
			Right:           offsetVal,
		}

	case *ir.ExtractFieldPtr:
		ptr := b.resolvePtrVal(i.Ptr, valueMap, globalMap)
		structTyp := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structTyp, i.FieldIndex)
		addr := ptr
		if byteOffset > 0 {
			addr = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: MakeNearPtr(b.convertType(i.Typ))},
				Op:              "+",
				Left:            ptr,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(byteOffset)},
			}
		}
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Typ)},
			Addr:            addr,
		}

	case *ir.InsertFieldPtr:
		ptr := b.resolvePtrVal(i.Ptr, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		structTyp := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structTyp, i.FieldIndex)
		addr := ptr
		if byteOffset > 0 {
			addr = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: MakeNearPtr(val.Type())},
				Op:              "+",
				Left:            ptr,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(byteOffset)},
			}
		}
		return &NearStore{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Addr:            addr,
			Val:             val,
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

	case *ir.AddressOfFunc:
		return &FuncRef{
			BaseInstruction: BaseInstruction{Typ: TypeWord},
			FuncName:        i.Func.Name,
		}

	case *ir.IndirectCall:
		args := make([]Value, len(i.Args))
		for idx, a := range i.Args {
			args[idx] = b.resolveVal(a, valueMap, globalMap)
		}
		funcPtr := b.resolveVal(i.FuncPtr, valueMap, globalMap)
		return &IndirectCall{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
			FuncPtr:         funcPtr,
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
		if typ.Kind == KindStruct || typ.Size > 2 {
			return &ZeroInit{
				BaseInstruction: BaseInstruction{Typ: typ},
			}
		}
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: typ}, Val: 0}

	case *ir.ExtractField:
		base := b.resolveVal(i.Struct, valueMap, globalMap)
		structTyp := i.Struct.Type()
		if structTyp.IsASlice() || structTyp.Name == "string" {
			return &SliceField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
				Slice:           base,
				FieldIdx:        i.FieldIndex,
			}
		}
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structTyp, i.FieldIndex)
		return &ExtractField{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
			Struct:          base,
			FieldIndex:      i.FieldIndex,
			ByteOffset:      byteOffset,
			FieldSize:       fieldSize,
		}

	case *ir.InsertField:
		base := b.resolveVal(i.Struct, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		structTyp := i.Struct.Type()
		if structTyp.IsASlice() || structTyp.Name == "string" {
			var farRef, offset, length, capacity Value
			if sm, ok := base.(*SliceMake); ok {
				farRef = sm.FarRef
				offset = sm.Offset
				length = sm.Length
				capacity = sm.Capacity
			} else {
				farRef = &SliceField{BaseInstruction: BaseInstruction{Typ: TypeFarRef}, Slice: base, FieldIdx: 0}
				offset = &SliceField{BaseInstruction: BaseInstruction{Typ: TypeWord}, Slice: base, FieldIdx: 1}
				length = &SliceField{BaseInstruction: BaseInstruction{Typ: TypeWord}, Slice: base, FieldIdx: 2}
				capacity = &SliceField{BaseInstruction: BaseInstruction{Typ: TypeWord}, Slice: base, FieldIdx: 3}
			}
			newSm := &SliceMake{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
				FarRef:          farRef,
				Offset:          offset,
				Length:          length,
				Capacity:        capacity,
			}
			switch i.FieldIndex {
			case 0:
				newSm.FarRef = val
			case 1:
				newSm.Offset = val
			case 2:
				newSm.Length = val
			case 3:
				newSm.Capacity = val
			default:
				newSm.FarRef = val
			}
			return newSm
		}
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structTyp, i.FieldIndex)
		return &InsertField{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
			Struct:          base,
			FieldIndex:      i.FieldIndex,
			ByteOffset:      byteOffset,
			FieldSize:       fieldSize,
			Val:             val,
		}

	case *ir.InsertElement:
		base := b.resolveVal(i.Array, valueMap, globalMap)
		val := b.resolveVal(i.Val, valueMap, globalMap)
		eltSize := b.getTypeSize(i.Val.Type())
		if cw, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cw.Val) * eltSize
			return &InsertField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
				Struct:          base,
				FieldIndex:      int(cw.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
				Val:             val,
			}
		}
		return nil

	case *ir.ExtractElement:
		base := b.resolveVal(i.Array, valueMap, globalMap)
		eltSize := b.getTypeSize(i.Type())
		if cw, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cw.Val) * eltSize
			return &ExtractField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
				Struct:          base,
				FieldIndex:      int(cw.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
			}
		}
		if cb, ok := i.Index.(*ir.ConstByte); ok {
			byteOffset := int(cb.Val) * eltSize
			return &ExtractField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
				Struct:          base,
				FieldIndex:      int(cb.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
			}
		}
		indexVal := b.resolveVal(i.Index, valueMap, globalMap)
		offsetVal := indexVal
		if eltSize > 1 {
			offsetVal = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeWord},
				Op:              "*",
				Left:            indexVal,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(eltSize)},
			}
		}
		baseAddr := &AddressOfLocal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(base.Type())},
			Local:           base,
		}
		addr := &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(b.convertType(i.Type()))},
			Op:              "+",
			Left:            baseAddr,
			Right:           offsetVal,
		}
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
			Addr:            addr,
		}

	case *ir.Cast:
		if i.Op == "word_to_ptr" {
			if ef, ok := i.Operand.(*ir.ExtractField); ok {
				structTyp := ef.Struct.Type()
				if structTyp.IsASlice() || structTyp.Name == "string" || structTyp.Name == "prelude.string" || structTyp.Name == "slice_byte" || structTyp.Name == "prelude.slice_byte" {
					sliceVal := b.resolveVal(ef.Struct, valueMap, globalMap)
					if sm, ok := sliceVal.(*SliceMake); ok {
						if cw, ok := sm.FarRef.(*ConstWord); ok && cw.Val == 0 {
							return nil
						}
					}
					return &SliceToPtr{
						BaseInstruction: BaseInstruction{Typ: b.convertType(i.Type())},
						Slice:           sliceVal,
					}
				}
			}
		}
		op := b.resolveVal(i.Operand, valueMap, globalMap)
		targetTyp := b.convertType(i.Type())
		if targetTyp.Size == 1 && op.Type().Size > 1 {
			return &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeByte},
				Op:              "and",
				Left:            op,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0xFF},
			}
		}
		return &BitCast{
			BaseInstruction: BaseInstruction{Typ: targetTyp},
			Operand:         op,
		}

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
		phi := &Phi{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Typ)},
			Edges:           make([]PhiEdge, 0, len(val.Edges)),
		}
		phi.SetID(val.GetID())
		phi.SetComment(val.GetComment())
		valueMap[val] = phi
		return phi
	case *ir.Cast:
		if val.Op == "word_to_ptr" {
			if ef, ok := val.Operand.(*ir.ExtractField); ok {
				structTyp := ef.Struct.Type()
				if structTyp.IsASlice() || structTyp.Name == "string" || structTyp.Name == "prelude.string" || structTyp.Name == "slice_byte" || structTyp.Name == "prelude.slice_byte" {
					sliceVal := b.resolveVal(ef.Struct, valueMap, globalMap)
					if sm, ok := sliceVal.(*SliceMake); ok {
						if cw, ok := sm.FarRef.(*ConstWord); ok && cw.Val == 0 {
							return sm.Offset
						}
					}
					return &SliceToPtr{
						BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
						Slice:           sliceVal,
					}
				}
			}
		}
		op := b.resolveVal(val.Operand, valueMap, globalMap)
		targetTyp := b.convertType(val.Type())
		if targetTyp.Size == 1 && op.Type().Size > 1 {
			return &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeByte},
				Op:              "and",
				Left:            op,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0xFF},
			}
		}
		return &BitCast{
			BaseInstruction: BaseInstruction{Typ: targetTyp},
			Operand:         op,
		}
	case *ir.InsertElement:
		base := b.resolveVal(val.Array, valueMap, globalMap)
		v := b.resolveVal(val.Val, valueMap, globalMap)
		eltSize := b.getTypeSize(val.Val.Type())
		if cw, ok := val.Index.(*ir.ConstWord); ok {
			byteOffset := int(cw.Val) * eltSize
			return &InsertField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
				Struct:          base,
				FieldIndex:      int(cw.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
				Val:             v,
			}
		}
		return base
	case *ir.ExtractElement:
		base := b.resolveVal(val.Array, valueMap, globalMap)
		eltSize := b.getTypeSize(val.Type())
		if cw, ok := val.Index.(*ir.ConstWord); ok {
			byteOffset := int(cw.Val) * eltSize
			return &ExtractField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
				Struct:          base,
				FieldIndex:      int(cw.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
			}
		}
		if cb, ok := val.Index.(*ir.ConstByte); ok {
			byteOffset := int(cb.Val) * eltSize
			return &ExtractField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
				Struct:          base,
				FieldIndex:      int(cb.Val),
				ByteOffset:      byteOffset,
				FieldSize:       eltSize,
			}
		}
		indexVal := b.resolveVal(val.Index, valueMap, globalMap)
		offsetVal := indexVal
		if eltSize > 1 {
			offsetVal = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeWord},
				Op:              "*",
				Left:            indexVal,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(eltSize)},
			}
		}
		baseAddr := &AddressOfLocal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(base.Type())},
			Local:           base,
		}
		addr := &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(b.convertType(val.Type()))},
			Op:              "+",
			Left:            baseAddr,
			Right:           offsetVal,
		}
		return &NearLoad{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
			Addr:            addr,
		}
	case *ir.AddressOfField:
		ptr := b.resolvePtrVal(val.Ptr, valueMap, globalMap)
		structTyp := val.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structTyp, val.FieldIndex)
		if byteOffset == 0 {
			return ptr
		}
		return &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Typ)},
			Op:              "+",
			Left:            ptr,
			Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(byteOffset)},
		}

	case *ir.AddressOfElement:
		arrayPtr := b.resolvePtrVal(val.ArrayPtr, valueMap, globalMap)
		if val.Index == nil {
			return arrayPtr
		}
		index := b.resolveVal(val.Index, valueMap, globalMap)
		eltSize := b.getElementSize(val.ArrayPtr.Type())
		offsetVal := index
		if eltSize > 1 {
			offsetVal = &BinaryOp{
				BaseInstruction: BaseInstruction{Typ: TypeWord},
				Op:              "*",
				Left:            index,
				Right:           &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(eltSize)},
			}
		}
		return &BinaryOp{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Typ)},
			Op:              "+",
			Left:            arrayPtr,
			Right:           offsetVal,
		}
	case *ir.ExtractField:
		if v, ok := valueMap[val]; ok {
			return v
		}
		base := b.resolveVal(val.Struct, valueMap, globalMap)
		if sm, ok := base.(*SliceMake); ok {
			switch val.FieldIndex {
			case 0:
				return sm.FarRef
			case 1:
				return sm.Offset
			case 2:
				return sm.Length
			case 3:
				return sm.Capacity
			default:
				return sm.FarRef
			}
		}
		if cs, ok := base.(*ConstStruct); ok && val.FieldIndex < len(cs.Fields) {
			return cs.Fields[val.FieldIndex]
		}
		structTyp := val.Struct.Type()
		if structTyp.IsASlice() || structTyp.Name == "string" {
			return &SliceField{
				BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
				Slice:           base,
				FieldIdx:        val.FieldIndex,
			}
		}
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structTyp, val.FieldIndex)
		return &ExtractField{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
			Struct:          base,
			FieldIndex:      val.FieldIndex,
			ByteOffset:      byteOffset,
			FieldSize:       fieldSize,
		}
	case *ir.ConstByte:
		return &ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: val.Val}
	case *ir.ConstWord:
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: val.Val}
	case *ir.ConstStruct:
		typ := b.convertType(val.Type())
		fields := make([]Value, len(val.Fields))
		for i, f := range val.Fields {
			fields[i] = b.resolveVal(f, valueMap, globalMap)
		}
		if (typ.Kind == KindFarSlice || typ.Kind == KindFarString || typ.Name == "string" || val.Type().IsASlice()) && len(fields) == 3 {
			fields = append([]Value{&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}}, fields...)
		}
		return &ConstStruct{
			BaseInstruction: BaseInstruction{Typ: typ},
			Fields:          fields,
		}
	case *ir.ConstArray:
		elements := make([]Value, len(val.Elements))
		for i, el := range val.Elements {
			elements[i] = b.resolveVal(el, valueMap, globalMap)
		}
		return &ConstArray{
			BaseInstruction: BaseInstruction{Typ: b.convertType(val.Type())},
			Elements:        elements,
		}
	case *ir.AddressOfGlobal:
		bg := b.lookupGlobal(val.Global)
		return &AddressOfGlobal{
			BaseInstruction: BaseInstruction{Typ: MakeNearPtr(bg.Typ)},
			Global:          bg,
		}
	case *ir.Sizeof:
		sz := b.getTypeSize(val.TargetTyp)
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(sz)}
	case *ir.Global:
		return b.lookupGlobal(val)
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
	case *ir.AddressOfFunc:
		return &FuncRef{
			BaseInstruction: BaseInstruction{Typ: TypeWord},
			FuncName:        val.Func.Name,
		}
	default:
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}
	}
}
