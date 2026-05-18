package ir

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/token"
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

	globals          map[string]*Global
	funcs            map[string]*Function
	consts           map[string]Value
	constExprs       map[string]ast.Expression
	evaluatingConst  map[string]bool
	evaluatingType   map[string]bool
	varTypes         map[string]Type
	typeDefsAST      map[string]*ast.StructType
	genericTemplates map[string]*GenericTemplate
	currentPackage   string
}

type GenericTemplate struct {
	TypeParams []string
	Tokens     []token.Token
}

func NewBuilder() *Builder {
	return &Builder{
		Program:          &Program{TypeDefs: make(map[string]string)},
		currentDef:       make(map[*BasicBlock]map[string]Value),
		sealedBlocks:     make(map[*BasicBlock]bool),
		incompletePhis:   make(map[*BasicBlock]map[string]*Phi),
		globals:          make(map[string]*Global),
		funcs:            make(map[string]*Function),
		consts:           make(map[string]Value),
		constExprs:       make(map[string]ast.Expression),
		evaluatingConst:  make(map[string]bool),
		evaluatingType:   make(map[string]bool),
		varTypes:         make(map[string]Type),
		typeDefsAST:      make(map[string]*ast.StructType),
		genericTemplates: make(map[string]*GenericTemplate),
	}
}

func (b *Builder) astToIRType(expr ast.Expression) Type {
	if expr == nil {
        panic("TODO: when is expr nil?")
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
        // nando-GOOD
		lenVal := b.EvalConst(e.Length)
		return Type(fmt.Sprintf("[%d]%s", lenVal, b.astToIRType(e.Elt)))
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
    // nando-PROBLEM.  This pass 0, 0.5, 1, 2 might work for the current
    // tests but is is not correct.  It registers const and struct types
    // in pass 0, but that is naive.  Really it should be registering
    // all names in the global space with the AST of their definition.
    // But then be lazy about filling in concrete values for constants
    // and the sizes of things.  And be capable of detecting cirularities.
    //
    // We probably need a loop around work-to-attempt, that keeps attempting,
    // until everything needed to satisfy main() has been resolved and
    // emitted.
    //
    // `package`:  Evaluated immediately.
    //             Only used to set b.currentPackage.
    //             Has to be used first (within eack package).
    //             I think now it is always overrided;
    //             TODO: assert that with code.
    // `import`:  Evaluated immediately.
    //            These must be known to evaluate selectors, which can occur
    //            in any other definitions.
    // `const`:   can depend on other consts and on sizeof types.
    // `type`:    can depend on other consts and types.
    // `var`:     can depend on types.  If we support global initialization,
    //            can depend on func and var and anything.
    // `func`:    can depend on anything.
	b.currentPackage = ""
	for _, stmt := range astProg.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			b.currentPackage = ps.Name.Value
		}
		switch s := stmt.(type) {

		case *ast.TypeStatement:
			if len(s.TypeParameters) > 0 {
                // It's a generic type, so save it to b.genericTemplates
				qname := b.currentPackage + "." + s.Name.Value
				var typeParams []string
				for _, tp := range s.TypeParameters {
					typeParams = append(typeParams, tp.Value)
				}
				b.genericTemplates[qname] = &GenericTemplate{
					TypeParams: typeParams,
					Tokens:     s.Tokens,
				}
			} else if st, ok := s.BaseType.(*ast.StructType); ok {
				qname := b.currentPackage + "." + s.Name.Value
				b.typeDefsAST[qname] = st
                // nando-PROBLEM.  I think TypeDefOrder is not this simple.
				b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, qname)
			} else {
                // nando-PROBLEM
                // Pass 0 is ignoring it.
                // Shouldn't all types be registered in pass 0?
            }

		case *ast.ConstStatement:
			qname := b.currentPackage + "." + s.Name.Value
			b.constExprs[qname] = s.Value
        default:
            // nando-PROBLEM.   Do we need import names? function names?
            // Other global names ignored in pass 0 .
		}
	}

	// Pass 0.5: build struct type strings and evaluate constants
	for _, qname := range b.Program.TypeDefOrder {
		b.getTypeString(qname)
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
            // nando-PROBLEM.  const definitions may involve
            // consts and sizeof types we have not seen yet.
            // Must detect circularities.
			qname := b.currentPackage + "." + s.Name.Value
			val := b.EvalConst(&ast.Identifier{Value: qname})
			b.consts[qname] = &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(val)}
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
	// nando-GOOD.  Do build function bodies after all consts,
    // types, and vars are understood.
    // TODO:  Should we be lazy at this point, and only compile
    // function bodies that are reachable from main()?
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
        // Construct a synthetic return type for a multi-value return.
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
		case TypeInt:
			return rtype
		case TypeUint:
			return rtype
		case TypeConstInteger:
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
		case TypeInt:
			return ltype
		case TypeUint:
			return ltype
		case TypeConstInteger:
			return ltype
		default:
			log.Panicf("NO CASE [right const] in sameTypeOfValues left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
		}
	}
	if ltype == rtype {
		return ltype
	}
	if ltype == TypeConstInteger {
		return rtype
	}
	if rtype == TypeConstInteger {
		return ltype
	}

	log.Panicf("No common type for binop: left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
	panic(0)
}

// nando: When do we use coersion?
func (b *Builder) coerceType(val Value, targetType Type) Value {
	if val.Type() == targetType || val.Type() == TypeUnknown {
		return val
	}
	
	if val.Type() == TypeConstInteger {
		if targetType == TypeByte {
			if cw, ok := val.(*ConstWord); ok {
				return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}, val)
			}
			return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val}, val)
		}
		if targetType == TypeWord || targetType == TypeInt || targetType == TypeUint {
			if cw, ok := val.(*ConstWord); ok {
				return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: targetType}, Val: cw.Val}, val)
			}
			return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: targetType}, Op: "bitcast", Operand: val}, val)
		}
	}
    // nando: TRUNCATION from word to byte?
	if val.Type() == TypeWord && targetType == TypeByte {
		if cw, ok := val.(*ConstWord); ok {
			return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}, val)
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val}, val)
	}
    // nando: PROMOTION from byte to word?
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
			typ := tupleVal.Type()
			if typ.IsAStruct() {
				typStr := string(typ)
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

type ExprResult struct {
	IsLValue bool
	Address  Value
	Value    Value
	Typ      Type
}

func (b *Builder) buildExpr(expr ast.Expression) Value {
	res := b.eval(expr)
	if res.IsLValue {
		return b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: res.Typ}, Ptr: res.Address}, expr)
	}
	return res.Value
}

func (b *Builder) buildAddress(expr ast.Expression) Value {
	res := b.eval(expr)
	if !res.IsLValue {
		panic(fmt.Sprintf("Cannot take the address of expression: %T", expr))
	}
	return res.Address
}

func (b *Builder) eval(expr ast.Expression) ExprResult {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		val := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeConstInteger}, Val: uint64(e.Value)}, e)
		return ExprResult{IsLValue: false, Value: val, Typ: TypeConstInteger}
	case *ast.Identifier:
		qname := b.currentPackage + "." + e.Value
		if g, ok := b.globals[qname]; ok {
			addr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: g.Typ}
		}
		if g, ok := b.globals[e.Value]; ok {
			addr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: g.Typ}
		}
		if c, ok := b.consts[qname]; ok {
			if cw, ok := c.(*ConstWord); ok {
				val := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeConstInteger}, Val: cw.Val}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeConstInteger}
			}
		}
		if c, ok := b.consts[e.Value]; ok {
			if cw, ok := c.(*ConstWord); ok {
				val := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeConstInteger}, Val: cw.Val}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeConstInteger}
			}
		}
		if typ, ok := b.varTypes[e.Value]; ok {
			val := b.readVariable(e.Value, b.currentBlock)
			addr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(typ))}, Local: val}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: typ}
		}
		panic(fmt.Sprintf("Identifier not found: %s", e.Value))
	case *ast.IndexExpression:
		base := b.eval(e.Left)
		idx := b.buildExpr(e.Indices[0])

		var eltTyp string
		if base.Typ.IsAnArray() {
			eltTyp = string(base.Typ.ArrayElementType())
		} else if base.Typ.IsAPointer() {
			panic("Pointer indexing not supported yet")
		} else {
			panic("Indexing non-array type")
		}

		if base.IsLValue {
			addr := b.addInstr(&AddressOfElement{BaseInstruction: BaseInstruction{Typ: Type("*" + eltTyp)}, ArrayPtr: base.Address, Index: idx}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: Type(eltTyp)}
		} else {
			val := b.addInstr(&ExtractElement{BaseInstruction: BaseInstruction{Typ: Type(eltTyp)}, Array: base.Value, Index: idx}, e)
			return ExprResult{IsLValue: false, Value: val, Typ: Type(eltTyp)}
		}

	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if g, ok := b.globals[qname]; ok {
				addr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: Type("*" + string(g.Typ))}, Global: g}, e)
				return ExprResult{IsLValue: true, Address: addr, Typ: g.Typ}
			}
			if c, ok := b.consts[qname]; ok {
				if cw, ok := c.(*ConstWord); ok {
					val := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: cw.Val}, e)
					return ExprResult{IsLValue: false, Value: val, Typ: TypeWord}
				}
			}
		}

		base := b.eval(e.Left)
		isPtr := base.Typ.IsAPointer()
		var structName string
		if isPtr {
			structName = string(base.Typ.PointedType())
		} else {
			structName = string(base.Typ)
		}

		st, ok := b.typeDefsAST[structName]
		if !ok {
			panic("Selector on unknown struct type: " + structName)
		}

		fieldIdx := -1
		var fieldType Type
		for i, f := range st.Fields {
			if f.Name.Value == e.Right.Value {
				fieldIdx = i
				fieldType = b.astToIRType(f.Type)
				break
			}
		}
		if fieldIdx == -1 {
			panic("Field not found: " + e.Right.Value)
		}

		if isPtr {
			var ptrVal Value
			if base.IsLValue {
				ptrVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, e)
			} else {
				ptrVal = base.Value
			}
			addr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: Type("*" + string(fieldType))}, Ptr: ptrVal, FieldIndex: fieldIdx}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: fieldType}
		} else {
			if base.IsLValue {
				addr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: Type("*" + string(fieldType))}, Ptr: base.Address, FieldIndex: fieldIdx}, e)
				return ExprResult{IsLValue: true, Address: addr, Typ: fieldType}
			} else {
				val := b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: fieldType}, Struct: base.Value, FieldIndex: fieldIdx}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: fieldType}
			}
		}

	case *ast.StringLiteral:
		val := &StringLiteral{Value: e.Value}
		return ExprResult{IsLValue: false, Value: val, Typ: Type("*byte")}

	case *ast.InfixExpression:
		if e.Operator == "&&" {
			left := b.buildExpr(e.Left)
			leftBlock := b.currentBlock
			falseVal := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, nil)
			rightBlock := b.newBlock()
			endBlock := b.newBlock()

			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: left, TrueBlock: rightBlock, FalseBlock: endBlock}, expr)
			b.addEdge(b.currentBlock, rightBlock)
			b.addEdge(b.currentBlock, endBlock)
			b.sealBlock(rightBlock)

			b.currentBlock = rightBlock
			right := b.buildExpr(e.Right)
			rightEndBlock := b.currentBlock

			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlock}, expr)
			b.addEdge(b.currentBlock, endBlock)
			b.sealBlock(endBlock)

			b.currentBlock = endBlock
			phi := &Phi{
				BaseInstruction: BaseInstruction{Typ: TypeByte},
				Edges: []PhiEdge{
					{Block: leftBlock, Value: falseVal},
					{Block: rightEndBlock, Value: right},
				},
			}
			val := b.addInstr(phi, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
		}
		if e.Operator == "||" {
			left := b.buildExpr(e.Left)
			leftBlock := b.currentBlock
			trueVal := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, nil)
			rightBlock := b.newBlock()
			endBlock := b.newBlock()

			b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: left, TrueBlock: endBlock, FalseBlock: rightBlock}, expr)
			b.addEdge(b.currentBlock, endBlock)
			b.addEdge(b.currentBlock, rightBlock)
			b.sealBlock(rightBlock)

			b.currentBlock = rightBlock
			right := b.buildExpr(e.Right)
			rightEndBlock := b.currentBlock

			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: endBlock}, expr)
			b.addEdge(b.currentBlock, endBlock)
			b.sealBlock(endBlock)

			b.currentBlock = endBlock
			phi := &Phi{
				BaseInstruction: BaseInstruction{Typ: TypeByte},
				Edges: []PhiEdge{
					{Block: leftBlock, Value: trueVal},
					{Block: rightEndBlock, Value: right},
				},
			}
			val := b.addInstr(phi, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
		}

		left := b.buildExpr(e.Left)
		right := b.buildExpr(e.Right)
		typ := b.commonTypeOfValues(expr, left, e.Operator, right)

		var val Value
		switch e.Operator {
		case "&":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "and", Left: left, Right: right}, expr)
		case "|":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "or", Left: left, Right: right}, expr)
		case "^":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "xor", Left: left, Right: right}, expr)
		case "+":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "add", Left: left, Right: right}, expr)
		case "-":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "sub", Left: left, Right: right}, expr)
		case "*":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "mul", Left: left, Right: right}, expr)
		case "/":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "div", Left: left, Right: right}, expr)
		case "==":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: left, Right: right}, expr)
			typ = TypeByte
		case "!=":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: left, Right: right}, expr)
			typ = TypeByte
		case "<":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lt", Left: left, Right: right}, expr)
			typ = TypeByte
		case "<=":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lte", Left: left, Right: right}, expr)
			typ = TypeByte
		case ">":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gt", Left: left, Right: right}, expr)
			typ = TypeByte
		case ">=":
			val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "gte", Left: left, Right: right}, expr)
			typ = TypeByte
		default:
			log.Panicf("NO CASE operator %q expr (%T)%v", e.Operator, e, e)
		}
		return ExprResult{IsLValue: false, Value: val, Typ: typ}

	case *ast.CallExpression:
		if ptrType, ok := e.Function.(*ast.PointerType); ok {
			targetTyp := b.astToIRType(ptrType)
			val := b.buildExpr(e.Arguments[0])
			res := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: targetTyp}, Op: "word_to_ptr", Operand: val}, expr)
			return ExprResult{IsLValue: false, Value: res, Typ: targetTyp}
		}

		var isGenericFunc bool
		var funcName string
		var rawFuncName string
		var args []Value

		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok {
				if ident.Value == "sizeof" {
					targetTyp := b.astToIRType(idxExpr.Indices[0])
					val := b.addInstr(&Sizeof{BaseInstruction: BaseInstruction{Typ: TypeWord}, TargetTyp: targetTyp}, expr)
					return ExprResult{IsLValue: false, Value: val, Typ: TypeWord}
				}
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
							if argTyp == "" {
								argTyp = "word"
							}
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
				panic(fmt.Sprintf("MISSING GENERIC FUNC: %s", funcName))
			}
			val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
		}

		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
				qname := pkgIdent.Value + "." + sel.Right.Value
				if f, exists := b.funcs[qname]; exists {
					var args []Value
					for _, arg := range e.Arguments {
						args = append(args, b.buildExpr(arg))
					}
					val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
					return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
				}
			}

			base := b.eval(sel.Left)
			isPtr := base.Typ.IsAPointer()
			var baseType string
			if isPtr {
				baseType = string(base.Typ.PointedType())
			} else {
				baseType = string(base.Typ)
			}
			funcName := baseType + "_" + sel.Right.Value
			if f, exists := b.funcs[funcName]; exists {
				var receiverVal Value
				if isPtr {
					if base.IsLValue {
						receiverVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, expr)
					} else {
						receiverVal = base.Value
					}
				} else {
					if base.IsLValue {
						receiverVal = base.Address
					} else {
						panic(fmt.Sprintf("Cannot call pointer method on unaddressable value: %T", sel.Left))
					}
				}
				args := []Value{receiverVal}
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
			}
		}

		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "print" || ident.Value == "println" {
				args := []Value{}
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				val := b.addInstr(&BuiltinCall{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Name: ident.Value, Args: args}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeVoid}
			}
			if ident.Value == "byte" {
				arg := b.buildExpr(e.Arguments[0])
				val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: arg}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
			}
			if ident.Value == "word" {
				arg := b.buildExpr(e.Arguments[0])
				val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: arg}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeWord}
			}

			args := []Value{}
			for _, arg := range e.Arguments {
				args = append(args, b.buildExpr(arg))
			}
			funcName := ident.Value
			if _, ok := b.funcs[b.currentPackage+"."+funcName]; ok {
				funcName = b.currentPackage + "." + funcName
			}
			f := b.funcs[funcName]
			if f == nil {
				qname := b.currentPackage + "." + ident.Value
				if _, ok := b.typeDefsAST[qname]; ok {
					arg := b.buildExpr(e.Arguments[0])
					val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: Type(qname)}, Op: "bitcast", Operand: arg}, expr)
					return ExprResult{IsLValue: false, Value: val, Typ: Type(qname)}
				}
				log.Panicf("Undefined function: %s", funcName)
			}
			val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
		} else {
			log.Panicf("Function is not an identifier: %v", e.Function)
		}

	case *ast.PrefixExpression:
		if e.Operator == "!" {
			right := b.buildExpr(e.Right)
			falseVal := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, nil)
			val := b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: right, Right: falseVal}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
		}
		if e.Operator == "&" {
			res := b.eval(e.Right)
			if !res.IsLValue {
				panic(fmt.Sprintf("Cannot take address of expression: %T", e.Right))
			}
			return ExprResult{IsLValue: false, Value: res.Address, Typ: Type("*" + string(res.Typ))}
		}
		if e.Operator == "*" {
			ptrVal := b.buildExpr(e.Right)
			typStr := string(ptrVal.Type())
			typStr = strings.TrimPrefix(typStr, "*")
			return ExprResult{IsLValue: true, Address: ptrVal, Typ: Type(typStr)}
		}

	case *ast.PointerType:
		ptrVal := b.buildExpr(e.Elt)
		typStr := string(ptrVal.Type())
		typStr = strings.TrimPrefix(typStr, "*")
		return ExprResult{IsLValue: true, Address: ptrVal, Typ: Type(typStr)}
	}
	log.Panicf("NO CASE: Builder.eval: expr (%T)%v", expr, expr)
	return ExprResult{}
}

func (b *Builder) getTypeString(qname string) string {
    // NANDO-recent.
	if res, ok := b.Program.TypeDefs[qname]; ok {
		return res
	}
	if b.evaluatingType[qname] {
		panic("circular dependency in type definition: " + qname)
	}
	st, ok := b.typeDefsAST[qname]
	if !ok {
		panic("unknown type: " + qname)
	}
	b.evaluatingType[qname] = true
	res := "struct{"
	for _, f := range st.Fields {
		res += string(b.astToIRType(f.Type)) + ";"
	}
	res += "}"
	b.Program.TypeDefs[qname] = res
	b.evaluatingType[qname] = false
	return res
}

func (b *Builder) getTypeSize(typ string) int {
    // NANDO-recent.
	if typ == "void" || typ == "byte" {
		return 1
	}
	if typ == "word" || typ == "int" || typ == "uint" {
		return 2
	}
	if Type(typ).IsAPointer() {
		return 2
	}
	if Type(typ).IsAnArray() {
		idx := strings.Index(typ, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ[1:idx])
			eltSize := b.getTypeSize(string(Type(typ).ArrayElementType()))
			return length * eltSize
		}
	}
	if !Type(typ).IsAStruct() {
		if _, ok := b.typeDefsAST[typ]; ok {
			typ = b.getTypeString(typ)
		} else if def, ok := b.Program.TypeDefs[typ]; ok {
			typ = def
		} else {
			return 2
		}
	}
	if Type(typ).IsAStruct() {
		content := typ[7 : len(typ)-1]
		size := 0
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				fTyp := content[start:i]
				size += b.getTypeSize(fTyp)
				start = i + 1
			}
		}
		return size
	}
    panic("why return 2")
	return 2
}

func (b *Builder) EvalConst(expr ast.Expression) int64 {
    // nando-recent
    // Keep this one.   Does ast.Identifier include qualified names?
    // Perhaps use `ok` instead of panic, so that a "keep trying unti
    // everything has been defined" approach works without panics.
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return e.Value
	case *ast.Identifier:
		qname := b.currentPackage + "." + e.Value
		var target ast.Expression
		targetName := qname
		if cExpr, ok := b.constExprs[qname]; ok {
			target = cExpr
		} else if cExpr, ok := b.constExprs[e.Value]; ok {
			target = cExpr
			targetName = e.Value
		} else {
			panic("unknown constant: " + e.Value)
		}

		if b.evaluatingConst[targetName] {
			panic("circular dependency detected for constant: " + targetName)
		}
		b.evaluatingConst[targetName] = true
		val := b.EvalConst(target)
		b.evaluatingConst[targetName] = false

		b.consts[targetName] = &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(val)}
		b.constExprs[targetName] = &ast.IntegerLiteral{Value: val}

		return val
	case *ast.InfixExpression:
		left := b.EvalConst(e.Left)
		right := b.EvalConst(e.Right)
		switch e.Operator {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			return left / right
		}
	case *ast.CallExpression:
		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok && ident.Value == "sizeof" {
				typ := string(b.astToIRType(idxExpr.Indices[0]))
				size := b.getTypeSize(typ)
				return int64(size)
			}
		}
	}
	panic(fmt.Sprintf("not a constant expression: %T", expr))
}



func (b *Builder) assignToExpr(lhs ast.Expression, val Value) {
	if ident, ok := lhs.(*ast.Identifier); ok {
		qname := b.currentPackage + "." + ident.Value
		if g, ok := b.globals[qname]; ok {
			val = b.coerceType(val, g.Typ)
			b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val}, lhs)
			return
		} else if g, ok := b.globals[ident.Value]; ok {
			val = b.coerceType(val, g.Typ)
			b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val}, lhs)
			return
		} else {
			targetType, exists := b.varTypes[ident.Value]
			if !exists {
				targetType = val.Type()
				b.varTypes[ident.Value] = targetType
			}
			val = b.coerceType(val, targetType)
			b.writeVariable(ident.Value, b.currentBlock, val)
			return
		}
	}

	res := b.eval(lhs)
	if !res.IsLValue {
		panic(fmt.Sprintf("Cannot assign to expression: %T", lhs))
	}
	val = b.coerceType(val, res.Typ)
	b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: res.Address, Val: val}, lhs)
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
		if Type(argTyp).IsAPointer() {
			extractTypeParamsIR(prefix.Right, string(Type(argTyp).PointedType()), typeMap, typeParams)
		}
	} else if ptr, ok := paramType.(*ast.PointerType); ok {
		if Type(argTyp).IsAPointer() {
			extractTypeParamsIR(ptr.Elt, string(Type(argTyp).PointedType()), typeMap, typeParams)
		}
	} else if idx, ok := paramType.(*ast.IndexExpression); ok {
        // nando-BAD.  Spliting on _ cannot work.
		parts := strings.Split(argTyp, "_")
		numIdx := len(idx.Indices)
		if len(parts) >= numIdx {
			for i, innerIdx := range idx.Indices {
				extractTypeParamsIR(innerIdx, parts[len(parts)-numIdx+i], typeMap, typeParams)
			}
		}
	}
}
