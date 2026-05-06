package ir

import (
	"fmt"
	"minigo/ast"
    "strings"
)

type Builder struct {
	Program *Program
	
	currentFunc  *Function
	currentBlock *BasicBlock
	nextValueID  int
	nextBlockID  int
	
	currentDef map[*BasicBlock]map[string]Value
	sealedBlocks map[*BasicBlock]bool
	incompletePhis map[*BasicBlock]map[string]*Phi
	
	globals map[string]*Global
	funcs   map[string]*Function
	consts  map[string]Value
	varTypes map[string]Type
	typeDefsAST map[string]*ast.StructType
}

func NewBuilder() *Builder {
	return &Builder{
		Program: &Program{TypeDefs: make(map[string]string)},
		currentDef: make(map[*BasicBlock]map[string]Value),
		sealedBlocks: make(map[*BasicBlock]bool),
		incompletePhis: make(map[*BasicBlock]map[string]*Phi),
		globals: make(map[string]*Global),
		funcs: make(map[string]*Function),
		consts: make(map[string]Value),
		varTypes: make(map[string]Type),
		typeDefsAST: make(map[string]*ast.StructType),
	}
}

func astToIRType(expr ast.Expression) Type {
	if expr == nil {
		return TypeWord
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		switch e.Value {
		case "byte":
			return TypeByte
		case "word":
			return TypeWord
		default:
			return Type(e.Value)
		}
	case *ast.ArrayType:
		lenStr := "0"
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = fmt.Sprintf("%d", il.Value)
		}
		return Type(fmt.Sprintf("[%s]%s", lenStr, astToIRType(e.Elt)))
	case *ast.PointerType:
		return Type("*" + string(astToIRType(e.Elt)))
	}
	return TypeWord
}

func (b *Builder) Build(astProg *ast.Program) *Program {
	// Pass 0: register struct types
	for _, stmt := range astProg.Statements {
		if s, ok := stmt.(*ast.TypeStatement); ok {
			if st, ok := s.BaseType.(*ast.StructType); ok {
				b.typeDefsAST[s.Name.Value] = st
				res := "struct{"
				for _, f := range st.Fields {
					res += string(astToIRType(f.Type)) + ";"
				}
				res += "}"
				b.Program.TypeDefs[s.Name.Value] = res
				b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, s.Name.Value)
			}
		}
	}

	// First pass: register all globals, constants, and function signatures
	for _, stmt := range astProg.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := astToIRType(s.ValueType)
			g := &Global{Name: s.Name.Value, Typ: typ}
			b.globals[g.Name] = g
			b.Program.Globals = append(b.Program.Globals, g)
		case *ast.ConstStatement:
			if intLit, ok := s.Value.(*ast.IntegerLiteral); ok {
				b.consts[s.Name.Value] = &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(intLit.Value)}
			}
		case *ast.FuncStatement:
			f := &Function{Name: s.Name.Value}
			if s.ReturnType != nil {
				f.ReturnType = astToIRType(s.ReturnType)
			} else {
				f.ReturnType = TypeVoid
			}
			for i, p := range s.Parameters {
				typ := astToIRType(p.Type)
				f.Parameters = append(f.Parameters, &Parameter{ID: i, Name: p.Name.Value, Typ: typ})
			}
			b.funcs[f.Name] = f
			b.Program.Functions = append(b.Program.Functions, f)
		}
	}
	
	// Second pass: build bodies
	for _, stmt := range astProg.Statements {
		if s, ok := stmt.(*ast.FuncStatement); ok {
			b.buildFunc(s)
		}
	}
	
	return b.Program
}

func (b *Builder) buildFunc(s *ast.FuncStatement) {
	b.currentFunc = b.funcs[s.Name.Value]
	b.nextValueID = 1
	b.nextBlockID = 1
	b.currentDef = make(map[*BasicBlock]map[string]Value)
	b.sealedBlocks = make(map[*BasicBlock]bool)
	b.incompletePhis = make(map[*BasicBlock]map[string]*Phi)
	b.varTypes = make(map[string]Type)
	
	entry := b.newBlock()
	b.currentBlock = entry
	
	// Map parameters
	for _, p := range b.currentFunc.Parameters {
		b.writeVariable(p.Name, b.currentBlock, p)
	}
	
	b.sealBlock(entry)
	
	b.buildBlock(s.Body)
	
	if b.currentBlock.Terminator == nil {
		b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}})
	}
}

func (b *Builder) newBlock() *BasicBlock {
	blk := &BasicBlock{ID: b.nextBlockID}
	b.nextBlockID++
	b.currentFunc.Blocks = append(b.currentFunc.Blocks, blk)
	return blk
}

func (b *Builder) addEdge(from, to *BasicBlock) {
	from.Successors = append(from.Successors, to)
	to.Predecessors = append(to.Predecessors, from)
}

func (b *Builder) addInstr(instr Instruction) Instruction {
	instr.SetID(b.nextValueID)
	b.nextValueID++
	b.currentBlock.Instructions = append(b.currentBlock.Instructions, instr)
	
	if term, ok := instr.(Terminator); ok {
		b.currentBlock.Terminator = term
	}
	return instr
}

// Braun et al. SSA Construction Methods
func (b *Builder) writeVariable(variable string, block *BasicBlock, value Value) {
	if b.currentDef[block] == nil {
		b.currentDef[block] = make(map[string]Value)
	}
	b.currentDef[block][variable] = value
	if value.Type() != TypeUnknown {
		if _, exists := b.varTypes[variable]; !exists {
			b.varTypes[variable] = value.Type()
		}
	}
}

func (b *Builder) coerceType(val Value, targetType Type) Value {
	if val.Type() == targetType || val.Type() == TypeUnknown {
		return val
	}
	if val.Type() == TypeWord && targetType == TypeByte {
		if cw, ok := val.(*ConstWord); ok {
			return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)})
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val})
	}
	if val.Type() == TypeByte && targetType == TypeWord {
		if cb, ok := val.(*ConstByte); ok {
			return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(cb.Val)})
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: val})
	}
	return val
}

func (b *Builder) readVariable(variable string, block *BasicBlock) Value {
	if defs, ok := b.currentDef[block]; ok {
		if val, ok := defs[variable]; ok {
			return val
		}
	}
	return b.readVariableRecursive(variable, block)
}

func (b *Builder) readVariableRecursive(variable string, block *BasicBlock) Value {
	var val Value
	if !b.sealedBlocks[block] {
		// Incomplete CFG
		phi := &Phi{BaseInstruction: BaseInstruction{Typ: b.varTypes[variable]}}
		phi.SetID(b.nextValueID)
		b.nextValueID++
		// Prepended so Phis appear at top
		block.Instructions = append([]Instruction{phi}, block.Instructions...) 
		
		if b.incompletePhis[block] == nil {
			b.incompletePhis[block] = make(map[string]*Phi)
		}
		b.incompletePhis[block][variable] = phi
		val = phi
	} else if len(block.Predecessors) == 1 {
		val = b.readVariable(variable, block.Predecessors[0])
	} else {
		phi := &Phi{BaseInstruction: BaseInstruction{Typ: b.varTypes[variable]}}
		phi.SetID(b.nextValueID)
		b.nextValueID++
		block.Instructions = append([]Instruction{phi}, block.Instructions...)
		b.writeVariable(variable, block, phi)
		val = b.addPhiOperands(variable, phi, block)
	}
	b.writeVariable(variable, block, val)
	return val
}

func (b *Builder) addPhiOperands(variable string, phi *Phi, block *BasicBlock) Value {
	for _, pred := range block.Predecessors {
		val := b.readVariable(variable, pred)
		phi.Edges = append(phi.Edges, PhiEdge{Block: pred, Value: val})
		phi.Typ = val.Type()
	}
	return phi
}

func (b *Builder) sealBlock(block *BasicBlock) {
	for variable, phi := range b.incompletePhis[block] {
		b.addPhiOperands(variable, phi, block)
	}
	b.sealedBlocks[block] = true
}

func (b *Builder) buildBlock(blockAst *ast.BlockStatement) {
	for _, stmt := range blockAst.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := astToIRType(s.ValueType)
			b.varTypes[s.Name.Value] = typ
			var val Value
			if s.Value != nil {
				val = b.buildExpr(s.Value)
				val = b.coerceType(val, typ)
			} else {
				switch typ {
				case TypeByte:
					val = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0})
				case TypeWord:
					val = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0})
				default:
					val = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}})
				}
			}
			b.writeVariable(s.Name.Value, b.currentBlock, val)
		case *ast.AssignStatement:
			for i, nameExpr := range s.Names {
				val := b.buildExpr(s.Values[i])
				b.assignToExpr(nameExpr, val)
			}
		case *ast.ExpressionStatement:
			b.buildExpr(s.Expression)
		case *ast.IfStatement:
			cond := b.buildExpr(s.Condition)
			trueBlk := b.newBlock()
			endBlk := b.newBlock()
			
			falseBlk := endBlk
			if s.Alternative != nil {
				falseBlk = b.newBlock()
			}
			
			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: trueBlk, FalseBlock: falseBlk})
			b.addEdge(b.currentBlock, trueBlk)
			b.addEdge(b.currentBlock, falseBlk)
			
			b.sealBlock(trueBlk)
			if s.Alternative != nil {
				b.sealBlock(falseBlk)
			}
			
			b.currentBlock = trueBlk
			b.buildBlock(s.Consequence)
			if b.currentBlock.Terminator == nil {
				b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlk})
				b.addEdge(b.currentBlock, endBlk)
			}
			
			if s.Alternative != nil {
				b.currentBlock = falseBlk
				b.buildBlock(s.Alternative)
				if b.currentBlock.Terminator == nil {
					b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlk})
					b.addEdge(b.currentBlock, endBlk)
				}
			}
			
			b.sealBlock(endBlk)
			b.currentBlock = endBlk
			
		case *ast.ForStatement:
			headerBlk := b.newBlock()
			bodyBlk := b.newBlock()
			endBlk := b.newBlock()
			
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk})
			b.addEdge(b.currentBlock, headerBlk)
			
			b.currentBlock = headerBlk
			cond := b.buildExpr(s.Condition)
			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: bodyBlk, FalseBlock: endBlk})
			b.addEdge(headerBlk, bodyBlk)
			b.addEdge(headerBlk, endBlk)
			
			b.sealBlock(bodyBlk)
			
			b.currentBlock = bodyBlk
			b.buildBlock(s.Body)
			if b.currentBlock.Terminator == nil {
				b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk})
				b.addEdge(b.currentBlock, headerBlk)
			}
			
			b.sealBlock(headerBlk) 
			b.sealBlock(endBlk)
			b.currentBlock = endBlk
			
		case *ast.ReturnStatement:
			var val Value
			if s.ReturnValue != nil {
				val = b.buildExpr(s.ReturnValue)
			}
			b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Val: val})
		}
	}
}

func (b *Builder) buildExpr(expr ast.Expression) Value {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(e.Value)})
	case *ast.Identifier:
		if g, ok := b.globals[e.Value]; ok {
			return b.addInstr(&Load{BaseInstruction: BaseInstruction{Typ: g.Typ}, Global: g})
		}
		if c, ok := b.consts[e.Value]; ok {
			if cw, ok := c.(*ConstWord); ok {
				return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: cw.Val})
			}
		}
		return b.readVariable(e.Value, b.currentBlock)
	case *ast.IndexExpression:
		arr := b.buildExpr(e.Left)
		idx := b.buildExpr(e.Index)
		// type of elt:
		var eltType Type = TypeUnknown
		if arr != nil && string(arr.Type()) != "" && string(arr.Type())[0] == '[' {
			// e.g. "[3]byte" -> "byte"
			s := string(arr.Type())
			for i, c := range s {
				if c == ']' {
					eltType = Type(s[i+1:])
					break
				}
			}
		}
		return b.addInstr(&ExtractElement{BaseInstruction: BaseInstruction{Typ: eltType}, Array: arr, Index: idx})
	case *ast.SelectorExpression:
		strct := b.buildExpr(e.Left)
		fieldName := e.Right.Value
		
		structName := string(strct.Type())
		isPointer := false
		if strings.HasPrefix(structName, "*") {
			isPointer = true
			structName = structName[1:]
		}
		
		st, ok := b.typeDefsAST[structName]
		if !ok {
			panic("Selector on unknown struct type: " + structName)
		}
		
		fieldIdx := -1
		var fieldType Type
		for i, f := range st.Fields {
			if f.Name.Value == fieldName {
				fieldIdx = i
				fieldType = astToIRType(f.Type)
				break
			}
		}
		if fieldIdx == -1 {
			panic("Field not found: " + fieldName)
		}
		
		if isPointer {
			return b.addInstr(&ExtractFieldPtr{BaseInstruction: BaseInstruction{Typ: fieldType}, Ptr: strct, FieldIndex: fieldIdx})
		}
		return b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: fieldType}, Struct: strct, FieldIndex: fieldIdx})
	case *ast.StringLiteral:
		return &StringLiteral{Value: e.Value}
	case *ast.InfixExpression:
		left := b.buildExpr(e.Left)
		right := b.buildExpr(e.Right)
		
		switch e.Operator {
		case "+": return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: left.Type()}, Op: "add", Left: left, Right: right})
		case "-": return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: left.Type()}, Op: "sub", Left: left, Right: right})
		case "*": return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: left.Type()}, Op: "mul", Left: left, Right: right})
		case "/": return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: left.Type()}, Op: "div", Left: left, Right: right})
		case "==": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: left, Right: right})
		case "!=": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: left, Right: right})
		case "<": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lt", Left: left, Right: right})
		case "<=": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lte", Left: left, Right: right})
		case ">": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gt", Left: left, Right: right})
		case ">=": return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gte", Left: left, Right: right})
		}
	case *ast.CallExpression:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "print" || ident.Value == "println" {
				args := []Value{}
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				return b.addInstr(&BuiltinCall{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Name: ident.Value, Args: args})
			}
			if ident.Value == "byte" {
				arg := b.buildExpr(e.Arguments[0])
				return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: arg})
			}
			if ident.Value == "word" {
				arg := b.buildExpr(e.Arguments[0])
				return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: arg})
			}
			
			args := []Value{}
			for _, arg := range e.Arguments {
				args = append(args, b.buildExpr(arg))
			}
			f := b.funcs[ident.Value]
			return b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args})
		}
	case *ast.PointerType:
		ptrVal := b.buildExpr(e.Elt)
		typ := string(ptrVal.Type())
		if strings.HasPrefix(typ, "*") {
			typ = typ[1:]
		}
		return b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: Type(typ)}, Ptr: ptrVal})
	case *ast.PrefixExpression:
		if e.Operator == "&" {
			if ident, ok := e.Right.(*ast.Identifier); ok {
				if g, ok := b.globals[ident.Value]; ok {
					return b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g})
				}
				panic("Taking address of local variable not supported yet")
			}
		}
	}
	return nil
}

func (b *Builder) assignToExpr(lhs ast.Expression, val Value) {
	if ident, ok := lhs.(*ast.Identifier); ok {
		if g, ok := b.globals[ident.Value]; ok {
			val = b.coerceType(val, g.Typ)
			b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val})
		} else {
			targetType, exists := b.varTypes[ident.Value]
			if !exists {
				targetType = val.Type()
				b.varTypes[ident.Value] = targetType
			}
			val = b.coerceType(val, targetType)
			b.writeVariable(ident.Value, b.currentBlock, val)
		}
	} else if idxExpr, ok := lhs.(*ast.IndexExpression); ok {
		arr := b.buildExpr(idxExpr.Left)
		idx := b.buildExpr(idxExpr.Index)
		newArr := b.addInstr(&InsertElement{BaseInstruction: BaseInstruction{Typ: arr.Type()}, Array: arr, Index: idx, Val: val})
		b.assignToExpr(idxExpr.Left, newArr)
	} else if selExpr, ok := lhs.(*ast.SelectorExpression); ok {
		strct := b.buildExpr(selExpr.Left)
		fieldName := selExpr.Right.Value
		structName := string(strct.Type())
		isPointer := false
		if strings.HasPrefix(structName, "*") {
			isPointer = true
			structName = structName[1:]
		}
		st := b.typeDefsAST[structName]
		fieldIdx := -1
		for i, f := range st.Fields {
			if f.Name.Value == fieldName {
				fieldIdx = i
				break
			}
		}
		if isPointer {
			b.addInstr(&InsertFieldPtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: strct, FieldIndex: fieldIdx, Val: val})
			return
		}
		newStrct := b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: strct.Type()}, Struct: strct, FieldIndex: fieldIdx, Val: val})
		b.assignToExpr(selExpr.Left, newStrct)
	} else if ptrExpr, ok := lhs.(*ast.PointerType); ok {
		ptrVal := b.buildExpr(ptrExpr.Elt)
		b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: ptrVal, Val: val})
		return
	}
}
