package ir

import (
	"fmt"
	"log"
	"minigo/ast"
	"minigo/lexer"
	"minigo/parser"
	"minigo/token"
	"strings"
)

type Builder struct {
	Program *Program

	currentFunc  *Function
	currentBlock *BasicBlock
	nextValueID  int
	nextBlockID  int

	currentDef     map[*BasicBlock]map[string]Value
	sealedBlocks   map[*BasicBlock]bool
	incompletePhis map[*BasicBlock]map[string]*Phi

	globals     map[string]*Global
	funcs       map[string]*Function
	consts      map[string]Value
	varTypes    map[string]Type
	typeDefsAST map[string]*ast.StructType
	genericTemplates map[string]*GenericTemplate
	currentPackage string
}

type GenericTemplate struct {
	TypeParams []string
	Tokens     []token.Token
}

func NewBuilder() *Builder {
	return &Builder{
		Program:        &Program{TypeDefs: make(map[string]string)},
		currentDef:     make(map[*BasicBlock]map[string]Value),
		sealedBlocks:   make(map[*BasicBlock]bool),
		incompletePhis: make(map[*BasicBlock]map[string]*Phi),
		globals:        make(map[string]*Global),
		funcs:          make(map[string]*Function),
		consts:         make(map[string]Value),
		varTypes:       make(map[string]Type),
		typeDefsAST:    make(map[string]*ast.StructType),
		genericTemplates: make(map[string]*GenericTemplate),
	}
}

func (b *Builder) astToIRType(expr ast.Expression) Type {
	if expr == nil {
		return TypeWord
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		switch e.Value {
		case "byte":
			return TypeByte
		case "word", "uint":
			return TypeWord
		case "int":
			return TypeInt
		default:
			qname := b.currentPackage + "." + e.Value
			if _, ok := b.typeDefsAST[qname]; ok {
				return Type(qname)
			}
			return Type(e.Value)
		}
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			return Type(pkgIdent.Value + "." + e.Right.Value)
		}
		return TypeWord
	case *ast.IndexExpression:
		var rawGenericName string
		if ident, ok := e.Left.(*ast.Identifier); ok {
			rawGenericName = b.currentPackage + "." + ident.Value
		} else if sel, ok := e.Left.(*ast.SelectorExpression); ok {
			if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
				rawGenericName = pkgIdent.Value + "." + sel.Right.Value
			}
		}
		
		if rawGenericName != "" {
			var instTypStr string
			for _, idx := range e.Indices {
				instTypStr += "_" + string(b.astToIRType(idx))
			}
			instName := fmt.Sprintf("%s%s", rawGenericName, instTypStr)

			if _, ok := b.typeDefsAST[instName]; !ok {
				if tmpl, ok := b.genericTemplates[rawGenericName]; ok {
					b.instantiateGeneric(instName, rawGenericName, e.Indices, tmpl)
				} else {
					panic("Generic template not found: " + rawGenericName)
				}
			}
			return Type(instName)
		}
		return TypeWord
	case *ast.ArrayType:
		lenStr := "0"
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = fmt.Sprintf("%d", il.Value)
		}
		return Type(fmt.Sprintf("[%s]%s", lenStr, b.astToIRType(e.Elt)))
	case *ast.PointerType:
		return Type("*" + string(b.astToIRType(e.Elt)))
	}
	return TypeWord
}

func (b *Builder) substituteGenericTokens(argTyps []string, tmpl *GenericTemplate) []token.Token {
	var argTokensList [][]token.Token
	for _, argTyp := range argTyps {
		argTokens := lexer.Lex(argTyp, "generic_inst")
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.EOF {
			argTokens = argTokens[:len(argTokens)-1]
		}
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.SEMICOLON {
			argTokens = argTokens[:len(argTokens)-1]
		}
		argTokensList = append(argTokensList, argTokens)
	}
	
	var newTokens []token.Token
	for _, tok := range tmpl.Tokens {
		replaced := false
		if tok.Type == token.IDENT {
			for i, tp := range tmpl.TypeParams {
				if tok.Literal == tp {
					if i < len(argTokensList) {
						newTokens = append(newTokens, argTokensList[i]...)
						replaced = true
						break
					}
				}
			}
		}
		if !replaced {
			newTokens = append(newTokens, tok)
		}
	}
	newTokens = append(newTokens, token.Token{Type: token.EOF, Literal: ""})
	return newTokens
}

func (b *Builder) instantiateGeneric(instName, genericName string, argNodes []ast.Expression, tmpl *GenericTemplate) {
	var argTyps []string
	for _, argNode := range argNodes {
		argTyps = append(argTyps, string(b.astToIRType(argNode)))
	}
	newTokens := b.substituteGenericTokens(argTyps, tmpl)
	
	p := parser.New(newTokens)
	baseTypeAST := p.ParseExpressionForGeneric()
	
	if len(p.Errors()) > 0 {
		fmt.Printf("Parser errors during generic instantiation of %s:\n", instName)
		for _, msg := range p.Errors() {
			fmt.Println("\t", msg)
		}
	}
	
	if st, ok := baseTypeAST.(*ast.StructType); ok {
		b.typeDefsAST[instName] = st
		b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, instName)
		
		res := "struct{"
		for _, f := range st.Fields {
			res += string(b.astToIRType(f.Type)) + ";"
		}
		res += "}"
		b.Program.TypeDefs[instName] = res
	} else {
		panic("Generic instantiation did not produce a struct: " + instName)
	}
}

func (b *Builder) instantiateGenericFunc(instName, genericName string, argTyps []string, tmpl *GenericTemplate) {
	newTokens := b.substituteGenericTokens(argTyps, tmpl)
	
	p := parser.New(newTokens)
	stmt := p.ParseStatementForGeneric()
	
	if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
		funcStmt.Name.Value = strings.TrimPrefix(instName, b.currentPackage+".")
		b.registerFunc(funcStmt)
		
		oldFunc := b.currentFunc
		oldNextValID := b.nextValueID
		oldNextBlkID := b.nextBlockID
		oldCurDef := b.currentDef
		oldSealed := b.sealedBlocks
		oldIncPhis := b.incompletePhis
		oldVarTypes := b.varTypes
		oldCurBlk := b.currentBlock

		b.buildFunc(funcStmt)

		b.currentFunc = oldFunc
		b.nextValueID = oldNextValID
		b.nextBlockID = oldNextBlkID
		b.currentDef = oldCurDef
		b.sealedBlocks = oldSealed
		b.incompletePhis = oldIncPhis
		b.varTypes = oldVarTypes
		b.currentBlock = oldCurBlk
	} else {
		panic("Generic instantiation did not produce a function: " + instName)
	}
}

func (b *Builder) Build(astProg *ast.Program) *Program {
	// Pass 0: register struct types
	b.currentPackage = ""
	for _, stmt := range astProg.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			b.currentPackage = ps.Name.Value
		}
		switch s := stmt.(type) {
		case *ast.TypeStatement:
			if len(s.TypeParameters) > 0 {
				qname := b.currentPackage + "." + s.Name.Value
				var typeParams []string
				for _, tp := range s.TypeParameters {
					typeParams = append(typeParams, tp.Value)
				}
				b.genericTemplates[qname] = &GenericTemplate{
					TypeParams: typeParams,
					Tokens:     s.Tokens,
				}
				continue
			}
			if st, ok := s.BaseType.(*ast.StructType); ok {
				qname := b.currentPackage + "." + s.Name.Value
				b.typeDefsAST[qname] = st
				b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, qname)
			}
		}
	}

	// Pass 0.5: build struct type strings
	b.currentPackage = ""
	for _, stmt := range astProg.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			b.currentPackage = ps.Name.Value
		}
		if s, ok := stmt.(*ast.TypeStatement); ok {
			if len(s.TypeParameters) > 0 { continue }
			if st, ok := s.BaseType.(*ast.StructType); ok {
				qname := b.currentPackage + "." + s.Name.Value
				res := "struct{"
				for _, f := range st.Fields {
					res += string(b.astToIRType(f.Type)) + ";"
				}
				res += "}"
				b.Program.TypeDefs[qname] = res
			}
		}
	}

	// First pass: register all globals, constants, and function signatures
	b.currentPackage = ""
	for _, stmt := range astProg.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			b.currentPackage = s.Name.Value
		case *ast.VarStatement:
			typ := b.astToIRType(s.ValueType)
			g := &Global{Name: b.currentPackage + "." + s.Name.Value, Typ: typ}
			b.globals[g.Name] = g
			b.Program.Globals = append(b.Program.Globals, g)
		case *ast.ConstStatement:
			if intLit, ok := s.Value.(*ast.IntegerLiteral); ok {
				b.consts[b.currentPackage + "." + s.Name.Value] = &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(intLit.Value)}
			}
		case *ast.FuncStatement:
			if len(s.TypeParameters) > 0 {
				qname := b.currentPackage + "." + s.Name.Value
				var typeParams []string
				for _, tp := range s.TypeParameters {
					typeParams = append(typeParams, tp.Value)
				}
				b.genericTemplates[qname] = &GenericTemplate{
					TypeParams: typeParams,
					Tokens:     s.Tokens,
				}
				continue
			}
			b.registerFunc(s)
		}
	}
	
	// Pass 2: build functionsbodies
	b.currentPackage = ""
	for _, stmt := range astProg.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			b.currentPackage = ps.Name.Value
		}
		if s, ok := stmt.(*ast.FuncStatement); ok {
			if len(s.TypeParameters) > 0 {
				continue
			}
			b.buildFunc(s)
		}
	}

	return b.Program
}

func (b *Builder) registerFunc(s *ast.FuncStatement) {
	funcName := s.Name.Value
	var receiverTyp Type
	if s.Receiver != nil {
		receiverTyp = b.astToIRType(s.Receiver.Type)
		baseType := string(receiverTyp)
		baseType = strings.TrimPrefix(baseType, "*")
		funcName = baseType + "_" + funcName
	} else {
		if b.currentPackage != "main" || funcName != "main" {
			funcName = b.currentPackage + "." + funcName
		}
	}
	f := &Function{Name: funcName}
	if len(s.ReturnTypes) == 1 {
		f.ReturnType = b.astToIRType(s.ReturnTypes[0])
	} else if len(s.ReturnTypes) > 1 {
		var fields []string
		for _, rt := range s.ReturnTypes {
			fields = append(fields, string(b.astToIRType(rt)))
		}
		f.ReturnType = Type(fmt.Sprintf("struct{%s;}", strings.Join(fields, ";")))
	} else {
		f.ReturnType = TypeVoid
	}
	paramIdx := 0
	if s.Receiver != nil {
		f.Parameters = append(f.Parameters, &Parameter{ID: paramIdx, Name: s.Receiver.Name.Value, Typ: receiverTyp})
		paramIdx++
	}
	for _, p := range s.Parameters {
		typ := b.astToIRType(p.Type)
		f.Parameters = append(f.Parameters, &Parameter{ID: paramIdx, Name: p.Name.Value, Typ: typ})
		paramIdx++
	}
	b.funcs[f.Name] = f
	b.Program.Functions = append(b.Program.Functions, f)
}

func (b *Builder) buildFunc(s *ast.FuncStatement) {
	funcName := s.Name.Value
	if s.Receiver != nil {
		receiverTyp := b.astToIRType(s.Receiver.Type)
		baseType := string(receiverTyp)
		baseType = strings.TrimPrefix(baseType, "*")
		funcName = baseType + "_" + funcName
	} else {
		if b.currentPackage != "main" || funcName != "main" {
			funcName = b.currentPackage + "." + funcName
		}
	}
	b.currentFunc = b.funcs[funcName]
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
		b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}}, s)
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

func (b *Builder) addInstr(instr Instruction, reference any) Instruction {
	instr.SetID(b.nextValueID)
	b.nextValueID++

	switch r := reference.(type) {
	case nil:
		instr.SetComment(fmt.Sprintf("%s {nil}",
			instr.GetComment()))

	case ast.Statement:
		tok := r.GetToken()
		instr.SetComment(fmt.Sprintf("%s S{%s:%d:%d}",
			instr.GetComment(),
			tok.Filename,
			tok.Line,
			tok.Column))

	case ast.Expression:
		tok := r.GetToken()
		instr.SetComment(fmt.Sprintf("%s E{%s:%d:%d}",
			instr.GetComment(),
			tok.Filename,
			tok.Line,
			tok.Column))

	default:
		instr.SetComment(fmt.Sprintf("%s R{%v}",
			instr.GetComment(),
			r))
	}
	instr.SetComment(instr.GetComment())

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

func (b *Builder) commonTypeOfValues(expr ast.Expression, left Value, op string, right Value) Type {
	if left == nil {
		log.Panicf("left is nil, in commonTypeOfValues: %v right=%v", expr, right)
	}
	if right == nil {
		log.Panicf("right is nil, in commonTypeOfValues: %v left=%v", expr, left)
	}

	ltype := left.Type()
	rtype := right.Type()

	switch op {
	case "shl":
		return ltype
	case "shr":
		return ltype
	}

	if _, ok := left.(*ConstWord); ok {
		switch rtype {
		case TypeByte:
			return rtype
		case TypeWord:
			return rtype
		default:
			log.Panicf("NO CASE [left const] in sameTypeOfValues left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
		}
	}
	if _, ok := right.(*ConstWord); ok {
		switch ltype {
		case TypeByte:
			return ltype
		case TypeWord:
			return ltype
		default:
			log.Panicf("NO CASE [right const] in sameTypeOfValues left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
		}
	}
	if ltype == rtype {
		return ltype
	}
	log.Panicf("No common type for binop: left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
	panic(0)
}

func (b *Builder) coerceType(val Value, targetType Type) Value {
	if val.Type() == targetType || val.Type() == TypeUnknown {
		return val
	}
	if val.Type() == TypeWord && targetType == TypeByte {
		if cw, ok := val.(*ConstWord); ok {
			return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}, val)
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val}, val)
	}
	if val.Type() == TypeByte && targetType == TypeWord {
		if cb, ok := val.(*ConstByte); ok {
			return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(cb.Val)}, val)
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: val}, val)
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
		b.buildStatement(stmt)
	}
}

func (b *Builder) buildStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarStatement:
		typ := b.astToIRType(s.ValueType)
		b.varTypes[s.Name.Value] = typ
		var val Value
		if s.Value != nil {
			val = b.buildExpr(s.Value)
			val = b.coerceType(val, typ)
		} else {
			switch typ {
			case TypeByte:
				val = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, s)
			case TypeWord:
				val = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, s)
			default:
				val = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, s)
			}
		}
		b.writeVariable(s.Name.Value, b.currentBlock, val)
	case *ast.AssignStatement:
		if len(s.Names) > 1 && len(s.Values) == 1 {
			tupleVal := b.buildExpr(s.Values[0])
			typStr := string(tupleVal.Type())
			if strings.HasPrefix(typStr, "struct{") {
				content := typStr[7 : len(typStr)-1]
				fields := strings.Split(content, ";")
				for i, nameExpr := range s.Names {
					if i >= len(fields) || fields[i] == "" {
						break
					}
					fieldTyp := Type(strings.TrimSpace(fields[i]))
					b.addInstr(&SourceMarker{
						BaseInstruction: BaseInstruction{Typ: TypeVoid},
						Comment:         fmt.Sprintf("Line %d: Assignment Tuple Unpack LHS: %s", s.Token.Line, nameExpr.TokenLiteral()),
					}, s)
					ext := b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: fieldTyp}, Struct: tupleVal, FieldIndex: i}, s)
					b.assignToExpr(nameExpr, ext)
				}
			}
		} else {
			var vals []Value
			for _, valExpr := range s.Values {
				vals = append(vals, b.buildExpr(valExpr))
			}
			for i, nameExpr := range s.Names {
				b.addInstr(&SourceMarker{
					BaseInstruction: BaseInstruction{Typ: TypeVoid},
					Comment:         fmt.Sprintf("Line %d: Assignment LHS: %s", s.Token.Line, nameExpr.TokenLiteral()),
				}, s)
				b.assignToExpr(nameExpr, vals[i])
			}
		}
	case *ast.IncDecStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: %s", s.Token.Line, s.Token.Literal),
		}, s)
		val := b.buildExpr(s.Name)
		typ := val.Type()
		op := "add"
		if s.Token.Literal == "--" {
			op = "sub"
		}
		var one Value
		if typ == TypeByte {
			one = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, s)
		} else {
			one = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 1}, s)
		}
		newVal := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: op, Left: val, Right: one}, s)
		b.assignToExpr(s.Name, newVal)
	case *ast.ExpressionStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: Expression: %s", s.Token.Line, s.Token.Literal),
		}, s)
		b.buildExpr(s.Expression)
	case *ast.IfStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: If statement", s.Token.Line),
		}, s)
		cond := b.buildExpr(s.Condition)
		trueBlk := b.newBlock()
		endBlk := b.newBlock()

		falseBlk := endBlk
		if s.Alternative != nil {
			falseBlk = b.newBlock()
		}

		b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: trueBlk, FalseBlock: falseBlk}, s)
		b.addEdge(b.currentBlock, trueBlk)
		b.addEdge(b.currentBlock, falseBlk)

		b.sealBlock(trueBlk)
		if s.Alternative != nil {
			b.sealBlock(falseBlk)
		}

		b.currentBlock = trueBlk
		b.buildBlock(s.Consequence)
		if b.currentBlock.Terminator == nil {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlk}, s)
			b.addEdge(b.currentBlock, endBlk)
		}

		if s.Alternative != nil {
			b.currentBlock = falseBlk
			b.buildBlock(s.Alternative)
			if b.currentBlock.Terminator == nil {
				b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlk}, s)
				b.addEdge(b.currentBlock, endBlk)
			}
		}

		b.sealBlock(endBlk)
		b.currentBlock = endBlk

	case *ast.ForStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: For statement loop header", s.Token.Line),
		}, s)
		headerBlk := b.newBlock()
		bodyBlk := b.newBlock()
		endBlk := b.newBlock()

		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.currentBlock = headerBlk
		if s.Condition != nil {
			cond := b.buildExpr(s.Condition)
			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: bodyBlk, FalseBlock: endBlk}, s)
			b.addEdge(headerBlk, bodyBlk)
			b.addEdge(headerBlk, endBlk)
		} else {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: bodyBlk}, s)
			b.addEdge(headerBlk, bodyBlk)
		}

		b.sealBlock(bodyBlk)

		b.currentBlock = bodyBlk
		b.buildBlock(s.Body)
		if b.currentBlock.Terminator == nil {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
			b.addEdge(b.currentBlock, headerBlk)
		}

		b.sealBlock(headerBlk)
		b.sealBlock(endBlk)
		b.currentBlock = endBlk

	case *ast.For3Statement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: For3 statement init", s.Token.Line),
		}, s)
		if s.Init != nil {
			b.buildStatement(s.Init)
		}

		headerBlk := b.newBlock()
		bodyBlk := b.newBlock()
		postBlk := b.newBlock()
		endBlk := b.newBlock()

		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.currentBlock = headerBlk
		if s.Condition != nil {
			cond := b.buildExpr(s.Condition)
			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: bodyBlk, FalseBlock: endBlk}, s)
			b.addEdge(headerBlk, bodyBlk)
			b.addEdge(headerBlk, endBlk)
		} else {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: bodyBlk}, s)
			b.addEdge(headerBlk, bodyBlk)
		}

		b.sealBlock(bodyBlk)

		b.currentBlock = bodyBlk
		b.buildBlock(s.Body)

		if b.currentBlock.Terminator == nil {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: postBlk}, s)
			b.addEdge(b.currentBlock, postBlk)
		}

		b.sealBlock(postBlk)
		b.currentBlock = postBlk
		if s.Increment != nil {
			b.buildStatement(s.Increment)
		}
		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.sealBlock(headerBlk)
		b.sealBlock(endBlk)
		b.currentBlock = endBlk

	case *ast.ForRangeStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: For range statement", s.Token.Line),
		}, s)

		limitVal := b.buildExpr(s.RangeValue)
		typ := limitVal.Type()

		var zero Value
		if typ == TypeByte {
			zero = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, s)
		} else {
			zero = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, s)
		}

		ident, ok := s.Key.(*ast.Identifier)
		if ok && s.IsDecl {
			b.varTypes[ident.Value] = typ
		}
		if ok {
			b.writeVariable(ident.Value, b.currentBlock, zero)
		}

		headerBlk := b.newBlock()
		bodyBlk := b.newBlock()
		postBlk := b.newBlock()
		endBlk := b.newBlock()

		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.currentBlock = headerBlk
		var cond Value
		if ok {
			currentI := b.readVariable(ident.Value, headerBlk)
			cond = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lt", Left: currentI, Right: limitVal}, s)
		} else {
			cond = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, s)
		}
		b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: bodyBlk, FalseBlock: endBlk}, s)
		b.addEdge(headerBlk, bodyBlk)
		b.addEdge(headerBlk, endBlk)

		b.sealBlock(bodyBlk)

		b.currentBlock = bodyBlk
		b.buildBlock(s.Body)

		if b.currentBlock.Terminator == nil {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: postBlk}, s)
			b.addEdge(b.currentBlock, postBlk)
		}

		b.sealBlock(postBlk)

		b.currentBlock = postBlk
		if ok {
			currentI := b.readVariable(ident.Value, postBlk)
			var one Value
			if typ == TypeByte {
				one = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, s)
			} else {
				one = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 1}, s)
			}
			nextI := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "add", Left: currentI, Right: one}, s)
			b.writeVariable(ident.Value, postBlk, nextI)
		}
		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.sealBlock(headerBlk)
		b.sealBlock(endBlk)
		b.currentBlock = endBlk

	case *ast.ReturnStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: Return statement", s.Token.Line),
		}, s)
		var val Value
		if len(s.ReturnValues) == 1 {
			val = b.buildExpr(s.ReturnValues[0])
			if f := b.currentFunc; f != nil && len(f.ReturnType) > 0 {
				val = b.coerceType(val, f.ReturnType)
			}
		} else if len(s.ReturnValues) > 1 {
			structTyp := b.currentFunc.ReturnType
			val = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: structTyp}}, s)
			for i, rv := range s.ReturnValues {
				fieldVal := b.buildExpr(rv)
				val = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: structTyp}, Struct: val, FieldIndex: i, Val: fieldVal}, s)
			}
		}
		b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Val: val}, s)
	}
}

func (b *Builder) buildExpr(expr ast.Expression) Value {
	// log.Printf("Builder.buildExpr: expr (%T)%v", expr, expr)

	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(e.Value)}, e)
	case *ast.Identifier:
		qname := b.currentPackage + "." + e.Value
		if g, ok := b.globals[qname]; ok {
			return b.addInstr(&Load{BaseInstruction: BaseInstruction{Typ: g.Typ}, Global: g}, e)
		}
		if g, ok := b.globals[e.Value]; ok {
			return b.addInstr(&Load{BaseInstruction: BaseInstruction{Typ: g.Typ}, Global: g}, e)
		}
		if c, ok := b.consts[qname]; ok {
			if cw, ok := c.(*ConstWord); ok {
				return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: cw.Val}, e)
			}
		}
		if c, ok := b.consts[e.Value]; ok {
			if cw, ok := c.(*ConstWord); ok {
				return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: cw.Val}, e)
			}
		}
		return b.readVariable(e.Value, b.currentBlock)
	case *ast.IndexExpression:
		arr := b.buildExpr(e.Left)
		idx := b.buildExpr(e.Indices[0])
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
		return b.addInstr(&ExtractElement{BaseInstruction: BaseInstruction{Typ: eltType}, Array: arr, Index: idx}, e)
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if g, ok := b.globals[qname]; ok {
				return b.addInstr(&Load{BaseInstruction: BaseInstruction{Typ: g.Typ}, Global: g}, e)
			}
			if c, ok := b.consts[qname]; ok {
				if cw, ok := c.(*ConstWord); ok {
					return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: cw.Val}, e)
				}
			}
		}

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
				fieldType = b.astToIRType(f.Type)
				break
			}
		}
		if fieldIdx == -1 {
			panic("Field not found: " + fieldName)
		}

		if isPointer {
			return b.addInstr(&ExtractFieldPtr{BaseInstruction: BaseInstruction{Typ: fieldType}, Ptr: strct, FieldIndex: fieldIdx}, e)
		}
		return b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: fieldType}, Struct: strct, FieldIndex: fieldIdx}, e)

	case *ast.StringLiteral:
		return &StringLiteral{Value: e.Value}

	case *ast.InfixExpression:
		// log.Printf("ast.InfixExpression: e.Left=(%T)%v e.Right=(%T)%v", e.Left, e.Left, e.Right, e.Right)
		left := b.buildExpr(e.Left)
		right := b.buildExpr(e.Right)
		typ := b.commonTypeOfValues(expr, left, e.Operator, right)

		switch e.Operator {
		case "&":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "and", Left: left, Right: right}, expr)
		case "|":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "or", Left: left, Right: right}, expr)
		case "^":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "xor", Left: left, Right: right}, expr)

		case "+":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "add", Left: left, Right: right}, expr)
		case "-":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "sub", Left: left, Right: right}, expr)
		case "*":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "mul", Left: left, Right: right}, expr)
		case "/":
			return b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "div", Left: left, Right: right}, expr)

		case "==":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: left, Right: right}, expr)
		case "!=":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: left, Right: right}, expr)
		case "<":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lt", Left: left, Right: right}, expr)
		case "<=":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lte", Left: left, Right: right}, expr)
		case ">":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gt", Left: left, Right: right}, expr)
		case ">=":
			return b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gte", Left: left, Right: right}, expr)
		default:
			log.Panicf("NO CASE operator %q expr (%T)%v", e.Operator, e, e)
		}

	case *ast.CallExpression:
		var isGenericFunc bool
		var funcName string
		var rawFuncName string
		var args []Value
		
		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok {
				rawFuncName = b.currentPackage + "." + ident.Value
			} else if sel, ok := idxExpr.Left.(*ast.SelectorExpression); ok {
				if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
					rawFuncName = pkgIdent.Value + "." + sel.Right.Value
				}
			}
			if rawFuncName != "" {
				var instTypStr string
				var argTyps []string
				for _, idx := range idxExpr.Indices {
					argTyp := string(b.astToIRType(idx))
					argTyps = append(argTyps, argTyp)
					instTypStr += "_" + argTyp
				}
				funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
				if _, ok := b.funcs[funcName]; !ok {
					if tmpl, ok := b.genericTemplates[rawFuncName]; ok {
						b.instantiateGenericFunc(funcName, rawFuncName, argTyps, tmpl)
					}
				}
				isGenericFunc = true
			}
		} else if ident, ok := e.Function.(*ast.Identifier); ok {
			rawFuncName = b.currentPackage + "." + ident.Value
			if _, ok := b.funcs[rawFuncName]; !ok {
				if tmpl, ok := b.genericTemplates[rawFuncName]; ok {
					p := parser.New(tmpl.Tokens)
					stmt := p.ParseStatementForGeneric()
					if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
						typeMap := make(map[string]string)
						for _, arg := range e.Arguments {
							args = append(args, b.buildExpr(arg))
						}
						for i, param := range funcStmt.Parameters {
							if i < len(args) {
								extractTypeParamsIR(param.Type, string(args[i].Type()), typeMap, tmpl.TypeParams)
							}
						}
						
						var argTyps []string
						var instTypStr string
						for _, tp := range tmpl.TypeParams {
							argTyp := typeMap[tp]
							if argTyp == "" { argTyp = "word" }
							argTyps = append(argTyps, argTyp)
							instTypStr += "_" + argTyp
						}
						funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
						if _, ok := b.funcs[funcName]; !ok {
							b.instantiateGenericFunc(funcName, rawFuncName, argTyps, tmpl)
						}
						isGenericFunc = true
					}
				}
			}
		}

		if isGenericFunc {
			if len(args) == 0 && len(e.Arguments) > 0 {
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
			}
			f, ok := b.funcs[funcName]
			if !ok {
				var keys []string
				for k := range b.funcs { keys = append(keys, k) }
				panic(fmt.Sprintf("MISSING GENERIC FUNC: %s, AVAILABLE: %v", funcName, keys))
			}
			return b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
		}

		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
				qname := pkgIdent.Value + "." + sel.Right.Value
				if f, exists := b.funcs[qname]; exists {
					var args []Value
					for _, arg := range e.Arguments {
						args = append(args, b.buildExpr(arg))
					}
					return b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
				}
			}

			leftVal := b.buildExpr(sel.Left)
			structTyp := string(leftVal.Type())
			isPtr := strings.HasPrefix(structTyp, "*")
			baseType := structTyp
			if isPtr {
				baseType = baseType[1:]
			}
			funcName := baseType + "_" + sel.Right.Value
			if f, exists := b.funcs[funcName]; exists {
				var receiverVal Value
				if isPtr {
					receiverVal = leftVal
				} else {
					if ident, ok := sel.Left.(*ast.Identifier); ok {
						qname := b.currentPackage + "." + ident.Value
						if g, ok := b.globals[qname]; ok {
							receiverVal = b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + baseType)}, Global: g}, expr)
						} else if g, ok := b.globals[ident.Value]; ok {
							receiverVal = b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + baseType)}, Global: g}, expr)
						} else {
							receiverVal = b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: Type("*" + baseType)}, Local: leftVal}, expr)
						}
					} else {
						receiverVal = b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: Type("*" + baseType)}, Local: leftVal}, expr)
					}
				}
				args := []Value{receiverVal}
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				return b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
			}
		}

		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "print" || ident.Value == "println" {
				args := []Value{}
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				return b.addInstr(&BuiltinCall{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Name: ident.Value, Args: args}, expr)
			}
			if ident.Value == "byte" {
				arg := b.buildExpr(e.Arguments[0])
				return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: arg}, expr)
			}
			if ident.Value == "word" {
				arg := b.buildExpr(e.Arguments[0])
				return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: arg}, expr)
			}

			args := []Value{}
			for _, arg := range e.Arguments {
				args = append(args, b.buildExpr(arg))
			}
			funcName := ident.Value
			if _, ok := b.funcs[b.currentPackage + "." + funcName]; ok {
				funcName = b.currentPackage + "." + funcName
			}
			f := b.funcs[funcName]
			return b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
		}
	case *ast.PointerType:
		ptrVal := b.buildExpr(e.Elt)
		typ := string(ptrVal.Type())
		typ = strings.TrimPrefix(typ, "*")
		return b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: Type(typ)}, Ptr: ptrVal}, expr)
	case *ast.PrefixExpression:
		if e.Operator == "&" {
			if ident, ok := e.Right.(*ast.Identifier); ok {
				qname := b.currentPackage + "." + ident.Value
				if g, ok := b.globals[qname]; ok {
					return b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g}, expr)
				}
				if g, ok := b.globals[ident.Value]; ok {
					return b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g}, expr)
				}
				panic("Taking address of local variable not supported yet")
			}
		}
	}
	log.Panicf("NO CASE: Builder.buildExpr: expr (%T)%v", expr, expr)
	return nil
}

func (b *Builder) assignToExpr(lhs ast.Expression, val Value) {
	if ident, ok := lhs.(*ast.Identifier); ok {
		qname := b.currentPackage + "." + ident.Value
		if g, ok := b.globals[qname]; ok {
			val = b.coerceType(val, g.Typ)
			b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val}, val)
		} else if g, ok := b.globals[ident.Value]; ok {
			val = b.coerceType(val, g.Typ)
			b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val}, val)
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
		idx := b.buildExpr(idxExpr.Indices[0])
		newArr := b.addInstr(&InsertElement{BaseInstruction: BaseInstruction{Typ: arr.Type()}, Array: arr, Index: idx, Val: val}, arr)
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
			b.addInstr(&InsertFieldPtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: strct, FieldIndex: fieldIdx, Val: val}, lhs)
			return
		}
		newStrct := b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: strct.Type()}, Struct: strct, FieldIndex: fieldIdx, Val: val}, lhs)
		b.assignToExpr(selExpr.Left, newStrct)
	} else if ptrExpr, ok := lhs.(*ast.PointerType); ok {
		ptrVal := b.buildExpr(ptrExpr.Elt)
		b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: ptrVal, Val: val}, lhs)
		return
	}
}

func extractTypeParamsIR(paramType ast.Expression, argTyp string, typeMap map[string]string, typeParams []string) {
	if ident, ok := paramType.(*ast.Identifier); ok {
		for _, tp := range typeParams {
			if tp == ident.Value {
				typeMap[tp] = argTyp
				return
			}
		}
	} else if prefix, ok := paramType.(*ast.PrefixExpression); ok && prefix.Operator == "*" {
		if strings.HasPrefix(argTyp, "*") {
			extractTypeParamsIR(prefix.Right, argTyp[1:], typeMap, typeParams)
		}
	} else if ptr, ok := paramType.(*ast.PointerType); ok {
		if strings.HasPrefix(argTyp, "*") {
			extractTypeParamsIR(ptr.Elt, argTyp[1:], typeMap, typeParams)
		}
	} else if idx, ok := paramType.(*ast.IndexExpression); ok {
		parts := strings.Split(argTyp, "_")
		numIdx := len(idx.Indices)
		if len(parts) >= numIdx {
			for i, innerIdx := range idx.Indices {
				extractTypeParamsIR(innerIdx, parts[len(parts)-numIdx+i], typeMap, typeParams)
			}
		}
	}
}
