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

type GlobalItemKind int

const (
	ItemUnknown GlobalItemKind = iota
	ItemConst
	ItemType
	ItemAlias
	ItemGenericType
	ItemVar
	ItemFunc
	ItemGenericFunc
)

type GlobalItem struct {
	Kind     GlobalItemKind
	QName    string
	ASTNode  ast.Node
	Resolved bool
	Blocker  string
}

type Builder struct {
	Program *Program

	currentFunc          *Function
	currentBlock         *BasicBlock
	currentDestructBlock *BasicBlock
	destructables        []string
	nextValueID          int
	nextBlockID          int

	currentDef     map[*BasicBlock]map[string]Value
	sealedBlocks   map[*BasicBlock]bool
	incompletePhis map[*BasicBlock]map[string]*Phi

	globals           map[string]*Global
	funcs             map[string]*Function
	consts            map[string]Value
	constExprs        map[string]ast.Expression
	evaluatingConst   map[string]bool
	evaluatingType    map[string]bool
	varTypes          map[string]Type
	typeDefsAST       map[string]*ast.StructType
	typeAliases       map[string]ast.Expression
	genericTemplates  map[string]*GenericTemplate
	instantiatedTypes map[string]InstantiatedTypeInfo
	stringConstants   map[string]*Global
	globalItems       map[string]*GlobalItem
	worklist          []*GlobalItem
	currentPackage    string

	breakStack        []*BasicBlock
	continueStack     []*BasicBlock
	resolveCallback   func(node ast.Node, defPkg string) ast.Node
	varInitStatements []*GlobalItem
	WordSize          int
}

func (b *Builder) SetCurrentPackage(pkg string) {
	b.currentPackage = pkg
}

type InstantiatedTypeInfo struct {
	RawGenericName string
	ArgTyps        []Type
}

type GenericTemplate struct {
	TypeParams []string
	Tokens     []token.Token
}

func NewBuilder(resolveCallback func(node ast.Node, defPkg string) ast.Node, wordSize int) *Builder {
	return &Builder{
		Program:           &Program{TypeDefs: make(map[string]Type)},
		currentDef:        make(map[*BasicBlock]map[string]Value),
		sealedBlocks:      make(map[*BasicBlock]bool),
		incompletePhis:    make(map[*BasicBlock]map[string]*Phi),
		globals:           make(map[string]*Global),
		funcs:             make(map[string]*Function),
		consts:            make(map[string]Value),
		constExprs:        make(map[string]ast.Expression),
		evaluatingConst:   make(map[string]bool),
		evaluatingType:    make(map[string]bool),
		varTypes:          make(map[string]Type),
		typeDefsAST:       make(map[string]*ast.StructType),
		typeAliases:       make(map[string]ast.Expression),
		genericTemplates:  make(map[string]*GenericTemplate),
		instantiatedTypes: make(map[string]InstantiatedTypeInfo),
		stringConstants:   make(map[string]*Global),
		globalItems:       make(map[string]*GlobalItem),
		worklist:          make([]*GlobalItem, 0),
		resolveCallback:   resolveCallback,
		WordSize:          wordSize,
	}
}

func (b *Builder) astToIRType(expr ast.Expression) Type {
	if se, ok := expr.(*ast.SelectorExpression); ok {
		_ = se
		//fmt.Printf("DEBUG ASTTOIRTYPE SELECTOR: %#v\n", se)
	}
	if expr == nil {
		log.Panicf("TODO: when is expr nil?")
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
		case "const_integer":
			return TypeConstInteger
		default:
			var qname string
			fullName := e.FullName()
			if strings.Contains(fullName, ".") {
				qname = fullName
			} else {
				qname = b.currentPackage + "." + fullName
			}
			if _, ok := b.typeDefsAST[qname]; !ok {
				if _, okAlias := b.typeAliases[qname]; !okAlias {
					if _, ok := b.typeDefsAST["prelude."+fullName]; ok {
						qname = "prelude." + fullName
					} else if _, okAlias := b.typeAliases["prelude."+fullName]; okAlias {
						qname = "prelude." + fullName
					} else if _, ok := b.typeAliases[fullName]; ok {
						qname = fullName
					}
				}
			}

			if aliasExpr, ok := b.typeAliases[qname]; ok {
				if strings.HasPrefix(qname, "func_ptr_") {
					return Type{Expr: aliasExpr, Name: qname}
				}
				return b.astToIRType(aliasExpr)
			}

			if _, ok := b.typeDefsAST[qname]; ok {
				return Type{Expr: expr, Name: qname}
			}
			panic("unresolved:" + qname)
		}
	case *ast.SelectorExpression:
		// We shouldn't hit package lookups here anymore since ResolveNames collapsed them!
		// If we hit it, it's a field lookup type? MiniGolf doesn't support nested structs by selector yet.
		return TypeWord
	case *ast.IndexExpression:
		var rawGenericName string
		if ident, ok := e.Left.(*ast.Identifier); ok {
			rawGenericName = ident.FullName()
		}

		if rawGenericName != "" {
			var instTypStr string
			for _, idx := range e.Indices {
				instTypStr += "_" + b.astToIRType(idx).Name
			}
			instName := fmt.Sprintf("%s%s", rawGenericName, instTypStr)

			if _, ok := b.typeDefsAST[instName]; !ok {
				if tmpl, ok := b.genericTemplates[rawGenericName]; ok {
					b.instantiateGeneric(instName, rawGenericName, e.Indices, tmpl, e.GetToken())
				} else if ident, ok := e.Left.(*ast.Identifier); ok {
					builtinRawGenericName := "prelude." + ident.Value
					if tmpl, ok := b.genericTemplates[builtinRawGenericName]; ok {
						rawGenericName = builtinRawGenericName
						instName = fmt.Sprintf("%s%s", rawGenericName, instTypStr)
						if _, ok := b.typeDefsAST[instName]; !ok {
							b.instantiateGeneric(instName, rawGenericName, e.Indices, tmpl, e.GetToken())
						}
					} else {
						panic("unresolved:" + rawGenericName)
					}
				} else {
					panic("unresolved:" + rawGenericName)
				}
			} else {
				// Type was already instantiated by semantic phase, but we must populate instantiatedTypes
				var argTyps []Type
				for _, argNode := range e.Indices {
					argTyps = append(argTyps, b.astToIRType(argNode))
				}
				b.instantiatedTypes[instName] = InstantiatedTypeInfo{
					RawGenericName: rawGenericName,
					ArgTyps:        argTyps,
				}
			}
			return Type{Expr: expr, Name: instName}
		}
		return TypeWord
	case *ast.ArrayType:
		// nando-GOOD
		lenVal := b.EvalConst(e.Length)
		return Type{Expr: expr, Name: fmt.Sprintf("[%d]%s", lenVal, b.astToIRType(e.Elt).Name)}
	case *ast.PointerType:
		return Type{Expr: expr, Name: "*" + b.astToIRType(e.Elt).Name}
	case *ast.StructType:
		name := "struct{"
		for _, f := range e.Fields {
			name += b.astToIRType(f.Type).Name + ";"
		}
		name += "}"
		return Type{Expr: expr, Name: name}

	case *ast.CompositeLit:
		return b.astToIRType(e.Type)
	case *ast.FuncType:
		return Type{Expr: expr, Name: b.SyntheticFuncName(e)}
	}
	log.Panicf("astToIRType NO CASE: %#v", expr)
	panic(0)
}

func (b *Builder) SyntheticFuncName(e *ast.FuncType) string {
	name := "func_ptr_"
	for i, param := range e.Parameters {
		if i > 0 {
			name += "_"
		}
		name += b.astToIRType(param.Type).Name
	}
	name += "__"
	for i, rt := range e.ReturnTypes {
		if i > 0 {
			name += "_"
		}
		name += b.astToIRType(rt).Name
	}
	// Sanitize by replacing any non-alphanumeric with underscores just in case
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "[", "_")
	name = strings.ReplaceAll(name, "]", "_")
	name = strings.ReplaceAll(name, "*", "ptr_")

	b.typeAliases[name] = e
	return name
}

func (b *Builder) packageAsAny(val Value, expr ast.Node) Value {
	tmpName := fmt.Sprintf(".anytmp_%d", b.nextValueID)
	b.varTypes[tmpName] = val.Type()
	b.writeVariable(tmpName, b.currentBlock, val)
	tmpLocal := b.readVariable(tmpName, b.currentBlock)
	addr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: val.Type().PointerTo()}, Local: tmpLocal}, expr)

	typName := val.Type().Name
	var typeChar string
	if typName == "byte" || typName == "word" {
		typeChar = typName
	} else if strings.HasPrefix(typName, "prelude.slice_") || strings.HasPrefix(typName, "slice_") {
		typeChar = "slice[" + strings.TrimPrefix(strings.TrimPrefix(typName, "prelude.slice_"), "slice_") + "]"
	} else {
		typeChar = typName
	}

	gStr := b.addStringConstant(typeChar)
	typeStrAddr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Global: gStr}, expr)

	anyTyp := Type{Name: "prelude.any"}
	if _, ok := b.Program.TypeDefs["prelude.any"]; !ok {
		anyTyp = Type{Name: "any"}
	}

	structVal := b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: anyTyp}}, expr)

	addrWord := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "bitcast", Operand: addr}, expr)
	typeStrWord := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "bitcast", Operand: typeStrAddr}, expr)

	structVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: anyTyp}, Struct: structVal, FieldIndex: 0, Val: addrWord}, expr)
	structVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: anyTyp}, Struct: structVal, FieldIndex: 1, Val: typeStrWord}, expr)

	return structVal
}

func (b *Builder) getAddress(val Value, expr ast.Node) Value {
	if addrOp, ok := val.(*LoadPtr); ok {
		return addrOp.Ptr
	}
	tmpName := fmt.Sprintf(".addrtmp_%d", b.nextValueID)
	b.nextValueID++
	b.varTypes[tmpName] = val.Type()
	b.writeVariable(tmpName, b.currentBlock, val)
	tmpLocal := b.readVariable(tmpName, b.currentBlock)
	return b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: val.Type().PointerTo()}, Local: tmpLocal}, expr)
}

func (b *Builder) coerceCallArgs(f *Function, args []Value, expr ast.Node) {
	for i, argVal := range args {
		if i < len(f.Parameters) {
			paramTyp := f.Parameters[i].Typ
			if paramTyp.Name == "prelude.any" || paramTyp.Name == "any" {
				args[i] = b.packageAsAny(argVal, expr)
			} else {
				args[i] = b.coerceType(argVal, paramTyp)
			}
		}
	}
}

func (b *Builder) substituteGenericTokens(argTyps []Type, tmpl *GenericTemplate, instantiateToken *token.Token, instName string) []token.Token {
	var argTokensList [][]token.Token
	for _, argTyp := range argTyps {

		// Use TypeName for type in expansion
		argTokens := lexer.Lex(argTyp.TypeName(), "generic_inst")

		// Trim EOF
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.EOF {
			argTokens = argTokens[:len(argTokens)-1]
		}
		// Trim SEMICOLON
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.SEMICOLON {
			argTokens = argTokens[:len(argTokens)-1]
		}

		// and add it to the list
		for i := range argTokens {
			if instantiateToken != nil {
				argTokens[i].ExpandedFrom = fmt.Sprintf("expanded %s at %s:%d", instName, instantiateToken.Filename, instantiateToken.Line)
			}
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
			tokCopy := tok
			if instantiateToken != nil {
				tokCopy.ExpandedFrom = fmt.Sprintf("expanded %s at %s:%d", instName, instantiateToken.Filename, instantiateToken.Line)
			}
			newTokens = append(newTokens, tokCopy)
		}
	}
	newTokens = append(newTokens, token.Token{Type: token.EOF, Literal: ""})
	return newTokens
}

func (b *Builder) instantiateGeneric(instName, genericName string, argNodes []ast.Expression, tmpl *GenericTemplate, instantiateToken *token.Token) {
	var argTyps []Type
	for _, argNode := range argNodes {
		argTyps = append(argTyps, b.astToIRType(argNode))
	}

	b.instantiatedTypes[instName] = InstantiatedTypeInfo{
		RawGenericName: genericName,
		ArgTyps:        argTyps,
	}

	newTokens := b.substituteGenericTokens(argTyps, tmpl, instantiateToken, instName)

	p := parser.New(newTokens)
	stmt := p.ParseStatementForGeneric()

	if len(p.Errors()) > 0 {
		fmt.Printf("Parser errors during generic instantiation of %s:\n", instName)
		for _, msg := range p.Errors() {
			fmt.Println("\t", msg)
		}
	}

	parts := strings.SplitN(genericName, ".", 2)
	defPkg := parts[0]
	if b.resolveCallback != nil {
		stmt = b.resolveCallback(stmt, defPkg).(ast.Statement)
	}

	ts, ok := stmt.(*ast.TypeStatement)
	if !ok {
		panic("Generic instantiation did not produce a TypeStatement: " + instName)
	}
	baseTypeAST := ts.BaseType

	if st, ok := baseTypeAST.(*ast.StructType); ok {
		b.typeDefsAST[instName] = st
		b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, instName)

		var fields []*ast.Field
		for i, f := range st.Fields {
			fields = append(fields, &ast.Field{
				Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
				Type: f.Type,
			})
		}
		structType := b.astToIRType(&ast.StructType{
			Fields: fields,
		})
		b.Program.TypeDefs[instName] = structType
	} else {
		panic("Generic instantiation did not produce a struct: " + instName)
	}
}

func (b *Builder) instantiateGenericFunc(instName, genericName string, argTyps []Type, tmpl *GenericTemplate, instantiateToken *token.Token) {
	//fmt.Printf("DEBUG instantiateGenericFunc ENTER: instName=%s genericName=%s argTyps=%v\n", instName, genericName, argTyps)
	newTokens := b.substituteGenericTokens(argTyps, tmpl, instantiateToken, instName)

	p := parser.New(newTokens)
	stmt := p.ParseStatementForGeneric()

	genParts := strings.SplitN(genericName, ".", 2)
	defPkg := "main"
	if len(genParts) == 2 {
		defPkg = genParts[0]
	}
	if b.resolveCallback != nil {
		stmt = b.resolveCallback(stmt, defPkg).(ast.Statement)
	}
	if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
		oldPkg := b.currentPackage
		parts := strings.SplitN(instName, ".", 2)
		if len(parts) == 2 {
			b.currentPackage = parts[0]
			funcStmt.Name.Value = parts[1]
		} else {
			funcStmt.Name.Value = instName
		}
		b.registerFunc(funcStmt)

		if funcStmt.Body != nil {
			oldFunc := b.currentFunc
			oldNextValID := b.nextValueID
			oldNextBlkID := b.nextBlockID
			oldCurDef := b.currentDef
			oldSealed := b.sealedBlocks
			oldIncPhis := b.incompletePhis
			oldVarTypes := b.varTypes
			oldCurBlk := b.currentBlock
			oldCurDestructBlk := b.currentDestructBlock
			oldDestructables := b.destructables

			b.buildFunc(funcStmt)

			b.currentFunc = oldFunc
			b.nextValueID = oldNextValID
			b.nextBlockID = oldNextBlkID
			b.currentDef = oldCurDef
			b.sealedBlocks = oldSealed
			b.incompletePhis = oldIncPhis
			b.varTypes = oldVarTypes
			b.currentBlock = oldCurBlk
			b.currentDestructBlock = oldCurDestructBlk
			b.destructables = oldDestructables
		}
		b.currentPackage = oldPkg
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
			qname := b.currentPackage + "." + s.Name.Value
			if qname == "main.Command" {
				//fmt.Printf("DEBUG Build TypeStatement main.Command: BaseType is %T\n", s.BaseType)
			}
			item := &GlobalItem{QName: qname, ASTNode: s}
			if len(s.TypeParameters) > 0 {
				item.Kind = ItemGenericType
			} else if _, ok := s.BaseType.(*ast.StructType); ok {
				item.Kind = ItemType
			} else {
				item.Kind = ItemAlias
			}
			b.globalItems[qname] = item
			b.worklist = append(b.worklist, item)
		case *ast.ConstStatement:
			qname := b.currentPackage + "." + s.Name.Value
			item := &GlobalItem{Kind: ItemConst, QName: qname, ASTNode: s}
			b.globalItems[qname] = item
			b.worklist = append(b.worklist, item)
		case *ast.VarStatement:
			qname := b.currentPackage + "." + s.Name.Value
			item := &GlobalItem{Kind: ItemVar, QName: qname, ASTNode: s}
			b.globalItems[qname] = item
			b.worklist = append(b.worklist, item)
		case *ast.FuncStatement:
			var qname string
			if s.Receiver != nil {
				var rawBase string
				var findRawBase func(expr ast.Expression)
				findRawBase = func(expr ast.Expression) {
					switch e := expr.(type) {
					case *ast.PointerType:
						findRawBase(e.Elt)
					case *ast.IndexExpression:
						if ident, ok := e.Left.(*ast.Identifier); ok {
							rawBase = ident.Value
						}
					case *ast.Identifier:
						rawBase = e.Value
					}
				}
				findRawBase(s.Receiver.Type)
				qname = b.currentPackage + "." + rawBase + "_" + s.Name.Value
			} else {
				qname = b.currentPackage + "." + s.Name.Value
			}
			item := &GlobalItem{QName: qname, ASTNode: s}
			if len(s.TypeParameters) > 0 {
				item.Kind = ItemGenericFunc
			} else {
				item.Kind = ItemFunc
			}
			b.globalItems[qname] = item
			b.worklist = append(b.worklist, item)
		}
	}

	for {
		madeProgress := false
		allResolved := true

		for i := 0; i < len(b.worklist); i++ {
			item := b.worklist[i]
			if item.Resolved {
				continue
			}
			allResolved = false

			parts := strings.SplitN(item.QName, ".", 2)
			if len(parts) == 2 {
				b.currentPackage = parts[0]
			} else {
				b.currentPackage = ""
			}

			err := b.tryResolve(item)
			if err == nil {
				item.Resolved = true
				madeProgress = true
			} else {
				item.Blocker = err.Error()
			}
		}

		if allResolved {
			break
		}
		if !madeProgress {
			fmt.Println("Error: Circular dependency or unresolved items detected:")
			for _, item := range b.worklist {
				if !item.Resolved {
					fmt.Printf("  %s (%v) depends on: %s\n", item.QName, item.Kind, item.Blocker)
				}
			}
			panic("Unresolved globals in compilation")
		}
	}

	b.currentPackage = ""
	for _, item := range b.worklist {
		if item.Kind == ItemFunc {
			s := item.ASTNode.(*ast.FuncStatement)
			parts := strings.SplitN(item.QName, ".", 2)
			if len(parts) == 2 {
				b.currentPackage = parts[0]
			}
			if s.Body != nil {
				b.buildFunc(s)
			}
		}
	}

	if len(b.varInitStatements) > 0 {
		b.buildSyntheticInit()

		mainFunc := b.funcs["main"]
		if mainFunc != nil && len(mainFunc.Blocks) > 0 {
			initFunc := b.funcs["init_main"]
			callInstr := &Call{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Func: initFunc}

			// We need a unique ID for callInstr
			maxID := 0
			for _, instr := range mainFunc.Blocks[0].Instructions {
				if instr.GetID() > maxID {
					maxID = instr.GetID()
				}
			}
			callInstr.SetID(maxID + 1000)

			mainFunc.Blocks[0].Instructions = append([]Instruction{callInstr}, mainFunc.Blocks[0].Instructions...)
		}
	}

	return b.Program
}

func (b *Builder) registerFunc(s *ast.FuncStatement) {
	funcName := s.Name.Value
	var receiverTyp Type
	if s.Receiver != nil {
		receiverTyp = b.astToIRType(s.Receiver.Type)
		baseType := receiverTyp
		if baseType.IsAPointer() {
			baseType = baseType.PointedType()
		}
		funcName = MangleName(baseType.String()) + "_" + funcName
	} else {
		if b.currentPackage != "main" || funcName != "main" {
			funcName = b.currentPackage + "." + funcName
		}
	}
	f := &Function{Name: funcName}
	f.ReturnType = b.getFuncReturnType(s.ReturnTypes)
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
	f.IsVariadic = s.IsVariadic
	b.funcs[f.Name] = f
	b.Program.Functions = append(b.Program.Functions, f)
}

func (b *Builder) buildFunc(s *ast.FuncStatement) {
	defer func() {
		if r := recover(); r != nil {
			tok := s.GetToken()
			file, line := "unknown", 0
			if tok != nil {
				file, line = tok.Filename, tok.Line
				if tok.ExpandedFrom != "" {
					file += " [" + tok.ExpandedFrom + "]"
				}
			}
			panic(fmt.Sprintf("%v\n\tin buildFunc: %s (at %s:%d)", r, s.Name.Value, file, line))
		}
	}()

	funcName := s.Name.Value
	if s.Receiver != nil {
		receiverTyp := b.astToIRType(s.Receiver.Type)
		baseType := receiverTyp

		// If baseType is a pointer, use its pointed type.
		if baseType.IsAPointer() {
			baseType = baseType.PointedType()
		}

		funcName = MangleName(baseType.String()) + "_" + funcName
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

	b.currentDestructBlock = b.newBlock()
	b.destructables = nil
	if !b.currentFunc.ReturnType.Equals(TypeUnknown) && !b.currentFunc.ReturnType.Equals(TypeVoid) {
		b.varTypes["__retval"] = b.currentFunc.ReturnType
	}

	// Map parameters
	for _, p := range b.currentFunc.Parameters {

		b.writeVariable(p.Name, b.currentBlock, p)
		if b.isDestructable(p.Typ) {
			b.destructables = append(b.destructables, p.Name)
		}
	}

	b.sealBlock(entry)

	b.buildBlock(s.Body)

	if b.currentBlock.Terminator == nil {
		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: b.currentDestructBlock}, s)
		b.addEdge(b.currentBlock, b.currentDestructBlock)
	}

	b.currentBlock = b.currentDestructBlock
	if len(b.destructables) > 0 {
		// Destruct in reverse order of declaration
		for i := len(b.destructables) - 1; i >= 0; i-- {
			vname := b.destructables[i]
			val := b.readVariable(vname, b.currentBlock)
			ptr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: val.Type().PointerTo()}, Local: val}, s)
			b.emitDestruction(val.Type(), ptr, s)
		}
	}

	if !b.currentFunc.ReturnType.Equals(TypeUnknown) && !b.currentFunc.ReturnType.Equals(TypeVoid) {
		retVal := b.readVariable("__retval", b.currentBlock)
		b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Val: retVal}, s)
	} else {
		b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}}, s)
	}
	b.sealBlock(b.currentDestructBlock)

	b.currentDestructBlock = nil
	b.destructables = nil
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
	//fmt.Printf("DEBUG writeVariable name=%s\n", variable)
	if b.currentDef[block] == nil {
		b.currentDef[block] = make(map[string]Value)
	}
	b.currentDef[block][variable] = value
	if !value.Type().Equals(TypeUnknown) {
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
		switch rtype.Name {
		case "byte":
			return rtype
		case "word":
			return rtype
		case "int":
			return rtype
		case "uint":
			return rtype
		case "const_integer":
			return rtype
		default:
			log.Panicf("NO CASE [left const] in sameTypeOfValues left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
		}
	}
	if _, ok := right.(*ConstWord); ok {
		switch ltype.Name {
		case "byte":
			return ltype
		case "word":
			return ltype
		case "int":
			return ltype
		case "uint":
			return ltype
		case "const_integer":
			return ltype
		default:
			log.Panicf("NO CASE [right const] in sameTypeOfValues left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
		}
	}
	if ltype.Equals(rtype) {
		return ltype
	}
	if ltype.Equals(TypeConstInteger) {
		return rtype
	}
	if rtype.Equals(TypeConstInteger) {
		return ltype
	}

	log.Panicf("No common type for binop: left=(%T)%v:%v op=%q right=(%T)%v:%v", left, left, ltype, op, right, right, rtype)
	panic(0)
}

// nando: When do we use coersion?
func (b *Builder) coerceType(val Value, targetType Type) Value {
	if val.Type().Equals(targetType) || val.Type().Equals(TypeUnknown) {
		return val
	}

	if val.Type().Equals(TypeConstInteger) {
		if targetType.Equals(TypeByte) {
			if cw, ok := val.(*ConstWord); ok {
				return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}, val)
			}
			return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val}, val)
		}
		if targetType.Equals(TypeWord) || targetType.Equals(TypeInt) || targetType.Equals(TypeUint) {
			if cw, ok := val.(*ConstWord); ok {
				return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: targetType}, Val: cw.Val}, val)
			}
			return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: targetType}, Op: "bitcast", Operand: val}, val)
		}
	}
	// nando: TRUNCATION from word to byte?
	if val.Type().Equals(TypeWord) && targetType.Equals(TypeByte) {
		if cw, ok := val.(*ConstWord); ok {
			return b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}, val)
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: val}, val)
	}
	// nando: PROMOTION from byte to word?
	if val.Type().Equals(TypeByte) && targetType.Equals(TypeWord) {
		if cb, ok := val.(*ConstByte); ok {
			return b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(cb.Val)}, val)
		}
		return b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "zero_ext", Operand: val}, val)
	}
	return val
}

func (b *Builder) readVariable(name string, block *BasicBlock) Value {
	//fmt.Printf("DEBUG readVariable name=%s\n", name)
	if defs, ok := b.currentDef[block]; ok {
		if val, ok := defs[name]; ok {
			return val
		}
	}
	return b.readVariableRecursive(name, block)
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
	defer func() {
		if r := recover(); r != nil {
			tok := stmt.GetToken()
			file, line := "unknown", 0
			if tok != nil {
				file, line = tok.Filename, tok.Line
				if tok.ExpandedFrom != "" {
					file += " [" + tok.ExpandedFrom + "]"
				}
			}
			panic(fmt.Sprintf("%v\n\tin buildStatement: %T (at %s:%d)", r, stmt, file, line))
		}
	}()

	switch s := stmt.(type) {
	case *ast.BlockStatement:
		b.buildBlock(s)
	case *ast.VarStatement:
		var typ Type
		var val Value

		if s.ValueType != nil {
			typ = b.astToIRType(s.ValueType)
			if s.Value != nil {
				val = b.buildExpr(s.Value)
				val = b.coerceType(val, typ)
			}
		} else if s.Value != nil {
			val = b.buildExpr(s.Value)
			typ = val.Type()
		} else {
			panic("variable declaration without type or value")
		}

		b.varTypes[s.Name.Value] = typ

		if val == nil {
			switch typ.Name {
			case "byte":
				val = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, s)
			case "word":
				val = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, s)
			default:
				val = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, s)
			}
		}
		b.writeVariable(s.Name.Value, b.currentBlock, val)
		if b.isDestructable(typ) {
			b.destructables = append(b.destructables, s.Name.Value)
		}
	case *ast.AssignStatement:
		if len(s.Names) > 1 && len(s.Values) == 1 {
			tupleVal := b.buildExpr(s.Values[0])
			//fmt.Printf("DEBUG ASSIGN tupleVal type: %#v IsStruct: %v\n", tupleVal.Type(), tupleVal.Type().IsAStruct())
			typ := tupleVal.Type()
			if typ.IsAStruct() {
				// typStr := string(typ)
				// content := typStr[7 : len(typStr)-1]
				// fields := strings.Split(content, ";")
				fields := typ.Expr.(*ast.StructType).Fields

				for i, f := range fields {
					if f.Name.Value == "" {
						// TODO: when does this happen?
						break
					}
					fieldTyp := f.Type
					b.addInstr(&SourceMarker{
						BaseInstruction: BaseInstruction{Typ: TypeVoid},
						Comment:         fmt.Sprintf("Line %d: Assignment Tuple Unpack LHS: %v", s.Token.Line, f),
					}, s)
					ext := b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: b.astToIRType(fieldTyp)}, Struct: tupleVal, FieldIndex: i}, s)
					b.assignToExpr(s.Names[i], ext)
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

		var oldVal Value
		var ptr Value
		var typ Type
		isIdent := false
		if _, ok := s.Name.(*ast.Identifier); ok {
			isIdent = true
			oldVal = b.buildExpr(s.Name)
			typ = oldVal.Type()
		} else {
			ptr, typ = b.buildAddressOf(s.Name)
			oldVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: typ}, Ptr: ptr}, s)
		}

		op := "add"
		if s.Token.Literal == "--" {
			op = "sub"
		}
		var one Value
		if typ.Equals(TypeByte) {
			one = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, s)
		} else {
			one = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 1}, s)
		}
		newVal := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: op, Left: oldVal, Right: one}, s)

		if isIdent {
			b.assignToExpr(s.Name, newVal)
		} else {
			b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: ptr, Val: newVal}, s)
		}
	case *ast.OpAssignStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: %s", s.Token.Line, s.Token.Literal),
		}, s)

		var oldVal Value
		var ptr Value
		var typ Type
		isIdent := false
		if _, ok := s.Name.(*ast.Identifier); ok {
			isIdent = true
			oldVal = b.buildExpr(s.Name)
			typ = oldVal.Type()
		} else {
			ptr, typ = b.buildAddressOf(s.Name)
			oldVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: typ}, Ptr: ptr}, s)
		}

		rightVal := b.buildExpr(s.Value)
		rightVal = b.coerceType(rightVal, typ)

		op := s.Operator
		switch op {
		case "+":
			op = "add"
		case "-":
			op = "sub"
		case "*":
			op = "mul"
		case "/":
			op = "div"
		case "%":
			op = "mod"
		case "&":
			op = "and"
		case "|":
			op = "or"
		case "^":
			op = "xor"
		case "<<":
			op = "shl"
		case ">>":
			op = "shr"
		case "&^":
			op = "andnot"
		}

		if typ.Name == "word" && (op == "mul" || op == "div" || op == "mod") {
			var funcName string
			if op == "mul" {
				funcName = "prelude.mul_word"
			}
			if op == "div" {
				funcName = "prelude.div_word"
			}
			if op == "mod" {
				funcName = "prelude.mod_word"
			}

			if f := b.funcs[funcName]; f != nil {
				args := []Value{oldVal, rightVal}
				b.coerceCallArgs(f, args, s)
				newVal := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f, Args: args}, s)
				if isIdent {
					b.assignToExpr(s.Name, newVal)
				} else {
					b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: ptr, Val: newVal}, s)
				}
				break
			}
		}

		newVal := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: op, Left: oldVal, Right: rightVal}, s)
		if isIdent {
			b.assignToExpr(s.Name, newVal)
		} else {
			b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: ptr, Val: newVal}, s)
		}
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
		b.breakStack = append(b.breakStack, endBlk)
		b.continueStack = append(b.continueStack, headerBlk)
		b.buildBlock(s.Body)
		b.breakStack = b.breakStack[:len(b.breakStack)-1]
		b.continueStack = b.continueStack[:len(b.continueStack)-1]
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
		b.breakStack = append(b.breakStack, endBlk)
		b.continueStack = append(b.continueStack, postBlk)
		b.buildBlock(s.Body)
		b.breakStack = b.breakStack[:len(b.breakStack)-1]
		b.continueStack = b.continueStack[:len(b.continueStack)-1]

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
		isSlice := strings.HasPrefix(typ.Name, "prelude.slice_") || strings.HasPrefix(typ.Name, "slice_")
		isArray := strings.HasPrefix(typ.Name, "[")

		if isSlice {
			limitVal = b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: TypeWord}, Struct: limitVal, FieldIndex: 2}, s) // Len
		} else if isArray {
			var arrayLen int
			fmt.Sscanf(typ.Name, "[%d]", &arrayLen)
			limitVal = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(arrayLen)}, s)
		}

		var zero Value
		idxTyp := TypeWord
		if !isSlice && typ.Equals(TypeByte) {
			idxTyp = TypeByte
			zero = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, s)
		} else {
			zero = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, s)
		}

		b.nextValueID++
		hiddenIdxName := fmt.Sprintf(".range_idx_%d", b.nextValueID)
		b.varTypes[hiddenIdxName] = idxTyp
		b.writeVariable(hiddenIdxName, b.currentBlock, zero)

		ident, ok := s.Key.(*ast.Identifier)
		hasUserIdent := ok && ident.Value != "_"
		if hasUserIdent {
			if s.IsDecl {
				b.varTypes[ident.Value] = idxTyp
			}
			b.writeVariable(ident.Value, b.currentBlock, zero)
		}

		headerBlk := b.newBlock()
		bodyBlk := b.newBlock()
		postBlk := b.newBlock()
		endBlk := b.newBlock()

		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: headerBlk}, s)
		b.addEdge(b.currentBlock, headerBlk)

		b.currentBlock = headerBlk
		currentI := b.readVariable(hiddenIdxName, headerBlk)
		cond := b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "lt", Left: currentI, Right: limitVal}, s)

		b.addInstr(&Branch{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Condition: cond, TrueBlock: bodyBlk, FalseBlock: endBlk}, s)
		b.addEdge(headerBlk, bodyBlk)
		b.addEdge(headerBlk, endBlk)

		b.sealBlock(bodyBlk)

		b.currentBlock = bodyBlk

		if (isSlice || isArray) && s.Value != nil {
			valIdent, valOk := s.Value.(*ast.Identifier)
			if valOk && valIdent.Value != "_" {
				idxExpr := &ast.IndexExpression{
					Token:   s.Token,
					Left:    s.RangeValue,
					Indices: []ast.Expression{&ast.Identifier{Token: s.Token, Value: hiddenIdxName}},
				}
				valRes := b.buildExpr(idxExpr)
				if s.IsDecl {
					b.varTypes[valIdent.Value] = valRes.Type()
				}
				b.writeVariable(valIdent.Value, b.currentBlock, valRes)
			}
		}
		b.breakStack = append(b.breakStack, endBlk)
		b.continueStack = append(b.continueStack, postBlk)
		b.buildBlock(s.Body)
		b.breakStack = b.breakStack[:len(b.breakStack)-1]
		b.continueStack = b.continueStack[:len(b.continueStack)-1]

		if b.currentBlock.Terminator == nil {
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: postBlk}, s)
			b.addEdge(b.currentBlock, postBlk)
		}

		b.sealBlock(postBlk)

		b.currentBlock = postBlk
		currentI = b.readVariable(hiddenIdxName, postBlk)
		var one Value
		if idxTyp.Equals(TypeByte) {
			one = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 1}, s)
		} else {
			one = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 1}, s)
		}
		nextI := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: idxTyp}, Op: "add", Left: currentI, Right: one}, s)
		b.writeVariable(hiddenIdxName, postBlk, nextI)
		if hasUserIdent {
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
			if f := b.currentFunc; f != nil && !f.ReturnType.Equals(TypeUnknown) {
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
		if b.currentDestructBlock != nil {
			if val != nil {
				b.writeVariable("__retval", b.currentBlock, val)
			}
			b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: b.currentDestructBlock}, s)
			b.addEdge(b.currentBlock, b.currentDestructBlock)
		} else {
			b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Val: val}, s)
		}
	case *ast.BreakStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: break", s.Token.Line),
		}, s)
		if len(b.breakStack) == 0 {
			log.Panicf("break statement outside of a loop at line %d", s.Token.Line)
		}
		targetBlk := b.breakStack[len(b.breakStack)-1]
		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: targetBlk}, s)
		b.addEdge(b.currentBlock, targetBlk)
		b.currentBlock = b.newBlock() // Unreachable block
	case *ast.ContinueStatement:
		b.addInstr(&SourceMarker{
			BaseInstruction: BaseInstruction{Typ: TypeVoid},
			Comment:         fmt.Sprintf("Line %d: continue", s.Token.Line),
		}, s)
		if len(b.continueStack) == 0 {
			log.Panicf("continue statement outside of a loop at line %d", s.Token.Line)
		}
		targetBlk := b.continueStack[len(b.continueStack)-1]
		b.addInstr(&Jump{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Target: targetBlk}, s)
		b.addEdge(b.currentBlock, targetBlk)
		b.currentBlock = b.newBlock() // Unreachable block
	}
}

type ExprResult struct {
	IsLValue bool
	Address  Value
	Value    Value
	Typ      Type
}

func (b *Builder) buildExpr(expr ast.Expression) Value {
	defer func() {
		if r := recover(); r != nil {
			tok := expr.GetToken()
			file, line := "unknown", 0
			if tok != nil {
				file, line = tok.Filename, tok.Line
				if tok.ExpandedFrom != "" {
					file += " [" + tok.ExpandedFrom + "]"
				}
			}
			panic(fmt.Sprintf("%v\n\tin buildExpr: %T (at %s:%d)", r, expr, file, line))
		}
	}()

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
		if e.Value == "nil" {
			return ExprResult{IsLValue: false, Value: &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, Typ: TypeWord}
		}

		fullName := e.FullName()
		if f, ok := b.funcs[fullName]; ok {
			val := b.addInstr(&AddressOfFunc{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f}, e)
			return ExprResult{IsLValue: false, Value: val, Typ: TypeWord}
		}
		if g, ok := b.globals[fullName]; ok {
			addr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: g.Typ.PointerTo()}, Global: g}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: g.Typ}
		}
		if c, ok := b.consts[fullName]; ok {
			if cw, ok := c.(*ConstWord); ok {
				val := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeConstInteger}, Val: cw.Val}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeConstInteger}
			}
		}
		if typ, ok := b.varTypes[e.Value]; ok {
			val := b.readVariable(e.Value, b.currentBlock)
			addr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: typ.PointerTo()}, Local: val}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: typ}
		}
		panic(fmt.Sprintf("Identifier not found: %s (fullName=%s, currentPackage=%s)", e.Value, fullName, b.currentPackage))
	case *ast.IndexExpression:
		base := b.eval(e.Left)
		idx := b.buildExpr(e.Indices[0])

		if strings.HasPrefix(base.Typ.Name, "prelude.slice_") || strings.HasPrefix(base.Typ.Name, "slice_") {
			isPtr := base.Typ.IsAPointer()
			var baseType string
			if isPtr {
				baseType = base.Typ.PointedType().Name
			} else {
				baseType = base.Typ.Name
			}
			methodName := "Get"
			if e.IsSlice {
				methodName = "Chop"
			}
			funcName := MangleName(baseType) + "_" + methodName

			if _, exists := b.funcs[funcName]; !exists {
				if instInfo, ok := b.instantiatedTypes[baseType]; ok {
					rawGenericFuncName := instInfo.RawGenericName + "_" + methodName
					if tmpl, ok := b.genericTemplates[rawGenericFuncName]; ok {
						parts := strings.SplitN(baseType, ".", 2)
						b.instantiateGenericFunc(parts[0]+"."+methodName, rawGenericFuncName, instInfo.ArgTyps, tmpl, e.GetToken())
					}
				}
			}

			if f, exists := b.funcs[funcName]; exists {
				var receiverVal Value
				if isPtr {
					if base.IsLValue {
						receiverVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, e)
					} else {
						receiverVal = base.Value
					}
				} else {
					if base.IsLValue {
						receiverVal = base.Address
					} else {
						log.Printf("warning: Cannot call method on non-lvalue struct %s (IndexExpression in slice). Synthesizing temporary lvalue.", base.Typ.Name)
						receiverVal = b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: base.Typ.PointerTo()}, Local: base.Value}, e)
					}
				}
				args := []Value{receiverVal, idx}
				if e.IsSlice && len(e.Indices) == 2 {
					idx2 := b.buildExpr(e.Indices[1])
					args = append(args, idx2)
				}
				b.coerceCallArgs(f, args, e)
				val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
			}
			panic("Slice " + methodName + " method not found: " + funcName)
		}

		var eltTyp Type
		if base.Typ.IsAnArray() {
			eltTyp = base.Typ.ArrayElementType()
		} else if base.Typ.IsAPointer() {
			panic("Pointer indexing not supported yet")
		} else {
			panic("Indexing non-array type")
		}

		if base.IsLValue {
			addr := b.addInstr(&AddressOfElement{BaseInstruction: BaseInstruction{Typ: eltTyp.PointerTo()}, ArrayPtr: base.Address, Index: idx}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: eltTyp}
		} else {
			val := b.addInstr(&ExtractElement{BaseInstruction: BaseInstruction{Typ: eltTyp}, Array: base.Value, Index: idx}, e)
			return ExprResult{IsLValue: false, Value: val, Typ: eltTyp}
		}

	case *ast.SelectorExpression:
		base := b.eval(e.Left)
		isPtr := base.Typ.IsAPointer()
		var structName string
		if isPtr {
			structName = base.Typ.PointedType().Name
		} else {
			structName = base.Typ.Name
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
			var fieldNames []string
			for _, f := range st.Fields {
				fieldNames = append(fieldNames, f.Name.Value)
			}
			panic(fmt.Sprintf("Field not found: %s in struct %s (available: %v)", e.Right.Value, structName, fieldNames))
		}

		if isPtr {
			var ptrVal Value
			if base.IsLValue {
				ptrVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, e)
			} else {
				ptrVal = base.Value
			}
			addr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: fieldType.PointerTo()}, Ptr: ptrVal, FieldIndex: fieldIdx}, e)
			return ExprResult{IsLValue: true, Address: addr, Typ: fieldType}
		} else {
			if base.IsLValue {
				addr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: fieldType.PointerTo()}, Ptr: base.Address, FieldIndex: fieldIdx}, e)
				return ExprResult{IsLValue: true, Address: addr, Typ: fieldType}
			} else {
				val := b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: fieldType}, Struct: base.Value, FieldIndex: fieldIdx}, e)
				return ExprResult{IsLValue: false, Value: val, Typ: fieldType}
			}
		}

	case *ast.StringLiteral:
		g := b.addStringConstant(e.Value)

		typ := Type{Name: "prelude.slice_byte"}
		if _, ok := b.Program.TypeDefs["prelude.slice_byte"]; !ok {
			typ = Type{Name: "slice_byte"}
		}

		var structVal Value = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, e)

		globalAddr := b.addInstr(&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Global: g}, e)
		globalWord := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "bitcast", Operand: globalAddr}, e)

		structVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: structVal, FieldIndex: 0, Val: globalWord}, e)

		length := int64(len(e.Value))
		lenVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(length)}, e)

		structVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: structVal, FieldIndex: 1, Val: lenVal}, e)
		structVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: structVal, FieldIndex: 2, Val: lenVal}, e)

		return ExprResult{IsLValue: false, Value: structVal, Typ: typ}

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
		case "&^":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "andnot", Left: left, Right: right}, expr)
		case "<<":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "shl", Left: left, Right: right}, expr)
		case ">>":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "shr", Left: left, Right: right}, expr)
		case "+":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "add", Left: left, Right: right}, expr)
		case "-":
			val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "sub", Left: left, Right: right}, expr)
		case "*":
			if typ.Name == "word" && b.funcs["prelude.mul_word"] != nil {
				f := b.funcs["prelude.mul_word"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				val = b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f, Args: args}, expr)
			} else {
				val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "mul", Left: left, Right: right}, expr)
			}
		case "/":
			if typ.Name == "word" && b.funcs["prelude.div_word"] != nil {
				f := b.funcs["prelude.div_word"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				val = b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f, Args: args}, expr)
			} else {
				val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "div", Left: left, Right: right}, expr)
			}
		case "%":
			if typ.Name == "word" && b.funcs["prelude.mod_word"] != nil {
				f := b.funcs["prelude.mod_word"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				val = b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f, Args: args}, expr)
			} else {
				val = b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "mod", Left: left, Right: right}, expr)
			}
		case "==":
			if typ.Name == "prelude.slice_byte" || typ.Name == "slice_byte" {
				f := b.funcs["prelude.streq"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				val = b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeByte}, Func: f, Args: args}, expr)
			} else if typ.IsAStruct() || typ.IsAnArray() {
				f := b.funcs["prelude.memeq"]
				leftAddr := b.getAddress(left, expr)
				rightAddr := b.getAddress(right, expr)
				sizeof := b.addInstr(&Sizeof{BaseInstruction: BaseInstruction{Typ: TypeWord}, TargetTyp: typ}, expr)
				leftBytePtr := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Op: "bitcast", Operand: leftAddr}, expr)
				rightBytePtr := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Op: "bitcast", Operand: rightAddr}, expr)
				args := []Value{leftBytePtr, rightBytePtr, sizeof}
				b.coerceCallArgs(f, args, expr)
				val = b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeByte}, Func: f, Args: args}, expr)
			} else {
				val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: left, Right: right}, expr)
			}
			typ = TypeByte
		case "!=":
			if typ.Name == "prelude.slice_byte" || typ.Name == "slice_byte" {
				f := b.funcs["prelude.streq"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				callVal := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeByte}, Func: f, Args: args}, expr)
				zero := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, expr)
				val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: callVal, Right: zero}, expr)
			} else if typ.IsAStruct() || typ.IsAnArray() {
				f := b.funcs["prelude.memeq"]
				leftAddr := b.getAddress(left, expr)
				rightAddr := b.getAddress(right, expr)
				sizeof := b.addInstr(&Sizeof{BaseInstruction: BaseInstruction{Typ: TypeWord}, TargetTyp: typ}, expr)
				leftBytePtr := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Op: "bitcast", Operand: leftAddr}, expr)
				rightBytePtr := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Op: "bitcast", Operand: rightAddr}, expr)
				args := []Value{leftBytePtr, rightBytePtr, sizeof}
				b.coerceCallArgs(f, args, expr)
				callVal := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeByte}, Func: f, Args: args}, expr)
				zero := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, expr)
				val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: callVal, Right: zero}, expr)
			} else {
				val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: left, Right: right}, expr)
			}
			typ = TypeByte
		case "<", "<=", ">", ">=":
			if typ.Name == "prelude.slice_byte" || typ.Name == "slice_byte" {
				f := b.funcs["prelude.strcmp"]
				args := []Value{left, right}
				b.coerceCallArgs(f, args, expr)
				callVal := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: f, Args: args}, expr) // Note: strcmp returns int (word in minigolf usually)
				zero := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, expr)
				one := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 1}, expr)
				negOne := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "sub", Left: zero, Right: one}, expr)

				if e.Operator == "<" {
					val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: callVal, Right: negOne}, expr)
				} else if e.Operator == "<=" {
					val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: callVal, Right: one}, expr)
				} else if e.Operator == ">" {
					val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: callVal, Right: one}, expr)
				} else if e.Operator == ">=" {
					val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "neq", Left: callVal, Right: negOne}, expr)
				}
			} else if typ.IsAStruct() || typ.IsAnArray() {
				panic(fmt.Sprintf("Ordering operator %s not supported for struct or array", e.Operator))
			} else {
				opMap := map[string]string{"<": "lt", "<=": "lte", ">": "gt", ">=": "gte"}
				val = b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: opMap[e.Operator], Left: left, Right: right}, expr)
			}
			typ = TypeByte
		default:
			log.Panicf("NO CASE operator %q expr (%T)%v", e.Operator, e, e)
		}
		return ExprResult{IsLValue: false, Value: val, Typ: typ}

	case *ast.CompositeLit:
		typ := b.astToIRType(e.Type)
		var val Value

		isSliceSugar := false
		if strings.HasPrefix(typ.Name, "prelude.slice_") {
			hasKeys := false
			for _, el := range e.Elements {
				if _, ok := el.(*ast.KeyValueExpr); ok {
					hasKeys = true
					break
				}
			}
			if !hasKeys && len(e.Elements) > 0 {
				isSliceSugar = true
			}
		}

		if isSliceSugar {
			eltTypeName := strings.TrimPrefix(typ.Name, "prelude.slice_")
			eltTyp := Type{Name: eltTypeName}
			eltSize := b.getTypeSize(eltTyp)

			sliceVal := b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, e)

			zallocFunc, ok := b.funcs["prelude.zalloc"]
			if !ok {
				panic("prelude.zalloc not found")
			}

			nBytes := len(e.Elements) * eltSize
			nBytesVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(nBytes)}, e)
			basePtrVal := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Func: zallocFunc, Args: []Value{nBytesVal}}, e)
			baseWordVal := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeWord}, Op: "ptr_to_word", Operand: basePtrVal}, e)

			sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: sliceVal, FieldIndex: 0, Val: baseWordVal}, e)
			capVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(len(e.Elements))}, e)
			sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: sliceVal, FieldIndex: 1, Val: capVal}, e)
			sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: sliceVal, FieldIndex: 2, Val: capVal}, e)

			arrTyp := Type{Name: "[" + strconv.Itoa(len(e.Elements)) + "]" + eltTypeName}
			arrPtrTyp := arrTyp.PointerTo()
			arrPtrVal := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: arrPtrTyp}, Op: "word_to_ptr", Operand: baseWordVal}, e)

			for i, el := range e.Elements {
				elVal := b.buildExpr(el)
				elVal = b.coerceType(elVal, eltTyp)

				idxVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(i)}, e)

				elAddr := b.addInstr(&AddressOfElement{BaseInstruction: BaseInstruction{Typ: eltTyp.PointerTo()}, ArrayPtr: arrPtrVal, Index: idxVal}, e)
				b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeUnknown}, Ptr: elAddr, Val: elVal}, e)
			}

			return ExprResult{IsLValue: false, Value: sliceVal, Typ: typ}
		}

		val = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, e)

		st, ok := b.typeDefsAST[typ.Name]
		if !ok {
			panic("Composite literal on unknown struct type: " + typ.Name)
		}

		fieldIdxMap := make(map[string]int)
		for i, f := range st.Fields {
			fieldIdxMap[f.Name.Value] = i
		}

		for i, el := range e.Elements {
			var fieldIdx int
			var valExpr ast.Expression

			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if ident, isIdent := kv.Key.(*ast.Identifier); isIdent {
					fieldIdx = fieldIdxMap[ident.Value]
				} else {
					panic("Key must be identifier")
				}
				valExpr = kv.Value
			} else {
				fieldIdx = i
				valExpr = el
			}

			fieldVal := b.buildExpr(valExpr)
			fTyp := b.astToIRType(st.Fields[fieldIdx].Type)
			fieldVal = b.coerceType(fieldVal, fTyp)
			val = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: typ}, Struct: val, FieldIndex: fieldIdx, Val: fieldVal}, e)
		}
		return ExprResult{IsLValue: false, Value: val, Typ: typ}

	case *ast.ArrayType:
		typ := b.astToIRType(e)
		var val Value = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: typ}}, e)
		if comp, ok := e.Elt.(*ast.CompositeLit); ok {
			var arrayLen int
			var eltTypStr string
			fmt.Sscanf(typ.Name, "[%d]%s", &arrayLen, &eltTypStr)
			eltTyp := b.astToIRType(&ast.Identifier{Value: eltTypStr})

			for i, el := range comp.Elements {
				if i >= arrayLen {
					panic("too many elements in array literal")
				}
				eltVal := b.buildExpr(el)
				eltVal = b.coerceType(eltVal, eltTyp)
				idxVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(i)}, e)
				val = b.addInstr(&InsertElement{BaseInstruction: BaseInstruction{Typ: typ}, Array: val, Index: idxVal, Val: eltVal}, e)
			}
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
				rawFuncName = ident.FullName()
			}
			if rawFuncName != "" {
				var instTypStr string
				var argTyps []Type
				for _, idx := range idxExpr.Indices {
					argTyp := b.astToIRType(idx)
					argTyps = append(argTyps, argTyp)
					instTypStr += "_" + argTyp.Name
				}
				funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
				if _, ok := b.funcs[funcName]; !ok {
					if tmpl, ok := b.genericTemplates[rawFuncName]; ok {
						b.instantiateGenericFunc(funcName, rawFuncName, argTyps, tmpl, e.GetToken())
					}
				}
				isGenericFunc = true
			}
		} else if ident, ok := e.Function.(*ast.Identifier); ok {
			rawFuncName = ident.FullName()
			if _, ok := b.funcs[rawFuncName]; !ok {
				if tmpl, ok := b.genericTemplates[rawFuncName]; ok {
					p := parser.New(tmpl.Tokens)
					stmt := p.ParseStatementForGeneric()
					if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
						typeMap := make(map[string]Type)
						for _, arg := range e.Arguments {
							args = append(args, b.buildExpr(arg))
						}
						for i, param := range funcStmt.Parameters {
							if i < len(args) {
								extractTypeParamsIR(param.Type, args[i].Type(), typeMap, tmpl.TypeParams)
							}
						}

						var argTyps []Type
						var instTypStr string
						for _, tp := range tmpl.TypeParams {
							argTyp := typeMap[tp]
							if argTyp.Name == "" {
								argTyp = TypeWord
							}
							argTyps = append(argTyps, argTyp)
							instTypStr += "_" + argTyp.Name
						}
						funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
						if _, ok := b.funcs[funcName]; !ok {
							b.instantiateGenericFunc(funcName, rawFuncName, argTyps, tmpl, e.GetToken())
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

			base := b.eval(sel.Left)
			isPtr := base.Typ.IsAPointer()
			var baseType string
			if isPtr {
				baseType = base.Typ.PointedType().Name
			} else {
				baseType = base.Typ.Name
			}
			funcName := MangleName(baseType) + "_" + sel.Right.Value

			if _, exists := b.funcs[funcName]; !exists {
				if instInfo, ok := b.instantiatedTypes[baseType]; ok {
					//fmt.Printf("DEBUG buildExpr call method: funcName=%s baseType=%s instInfo.ArgTyps=%v\n", funcName, baseType, instInfo.ArgTyps)
					rawGenericFuncName := instInfo.RawGenericName + "_" + sel.Right.Value
					if tmpl, ok := b.genericTemplates[rawGenericFuncName]; ok {
						b.instantiateGenericFunc(b.currentPackage+"."+sel.Right.Value, rawGenericFuncName, instInfo.ArgTyps, tmpl, e.GetToken())
					}
				}
			}

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
				if f.IsVariadic {
					args = b.packVariadicArgs(f, args, expr)
				}
				b.coerceCallArgs(f, args, expr)
				val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
			}
		}

		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "print" || ident.Value == "println" || ident.Value == "exit" {
				args := []Value{}
				for _, arg := range e.Arguments {
					if strLit, ok := arg.(*ast.StringLiteral); ok {
						args = append(args, &StringLiteral{Value: strLit.Value})
					} else {
						args = append(args, b.buildExpr(arg))
					}
				}
				val := b.addInstr(&BuiltinCall{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Name: ident.Value, Args: args}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeVoid}
			}
			if ident.Value == "byte" {
				arg := b.buildExpr(e.Arguments[0])
				val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "trunc", Operand: arg}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
			}
			if ident.Value == "len" || ident.Value == "cap" {
				arg := b.buildExpr(e.Arguments[0])
				fieldIdx := 2 // Len
				if ident.Value == "cap" {
					fieldIdx = 1 // Cap
				}
				val := b.addInstr(&ExtractField{BaseInstruction: BaseInstruction{Typ: TypeWord}, Struct: arg, FieldIndex: fieldIdx}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: TypeWord}
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
			funcName := ident.FullName()
			if _, ok := b.funcs[funcName]; ok {
				// Found by exact full name
			} else if _, ok := b.funcs[b.currentPackage+"."+funcName]; ok {
				funcName = b.currentPackage + "." + funcName
			} else if _, ok := b.funcs["prelude."+funcName]; ok {
				funcName = "prelude." + funcName
			}
			f := b.funcs[funcName]
			if f == nil {
				qname := b.currentPackage + "." + ident.Value
				if _, ok := b.typeDefsAST[qname]; ok {
					arg := b.buildExpr(e.Arguments[0])
					val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: Type{Name: qname}}, Op: "bitcast", Operand: arg}, expr)
					return ExprResult{IsLValue: false, Value: val, Typ: Type{Name: qname}}
				}
				if _, ok := b.typeDefsAST["prelude."+ident.Value]; ok {
					arg := b.buildExpr(e.Arguments[0])
					val := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: Type{Name: "prelude." + ident.Value}}, Op: "bitcast", Operand: arg}, expr)
					return ExprResult{IsLValue: false, Value: val, Typ: Type{Name: "prelude." + ident.Value}}
				}
				// It's not a typedef, treat as an indirect call from a variable holding a function!
				funcVal := b.buildExpr(e.Function)
				var args []Value
				for _, arg := range e.Arguments {
					args = append(args, b.buildExpr(arg))
				}
				retTyp := TypeWord
				if ft, ok := funcVal.Type().Expr.(*ast.FuncType); ok {
					retTyp = b.getFuncReturnType(ft.ReturnTypes)
				}

				val := b.addInstr(&IndirectCall{BaseInstruction: BaseInstruction{Typ: retTyp}, FuncPtr: funcVal, Args: args}, expr)
				return ExprResult{IsLValue: false, Value: val, Typ: retTyp}
			}
			if f.IsVariadic {
				args = b.packVariadicArgs(f, args, expr)
			}
			b.coerceCallArgs(f, args, expr)
			val := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: f.ReturnType}, Func: f, Args: args}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: f.ReturnType}
		} else {
			// Treat as an indirect call!
			funcVal := b.buildExpr(e.Function)
			var args []Value
			for _, arg := range e.Arguments {
				args = append(args, b.buildExpr(arg))
			}

			retTyp := TypeWord
			if ft, ok := funcVal.Type().Expr.(*ast.FuncType); ok {
				retTyp = b.getFuncReturnType(ft.ReturnTypes)
			}
			val := b.addInstr(&IndirectCall{BaseInstruction: BaseInstruction{Typ: retTyp}, FuncPtr: funcVal, Args: args}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: retTyp}
		}

	case *ast.PrefixExpression:
		if e.Operator == "!" {
			right := b.buildExpr(e.Right)
			falseVal := b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, nil)
			val := b.addInstr(&Compare{BaseInstruction: BaseInstruction{Typ: TypeByte}, Op: "eq", Left: right, Right: falseVal}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: TypeByte}
		}
		if e.Operator == "-" {
			right := b.buildExpr(e.Right)
			typ := right.Type()
			var zero Value
			if typ.Equals(TypeByte) {
				zero = b.addInstr(&ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: 0}, expr)
			} else {
				zero = b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: 0}, expr)
			}
			val := b.addInstr(&BinaryOp{BaseInstruction: BaseInstruction{Typ: typ}, Op: "sub", Left: zero, Right: right}, expr)
			return ExprResult{IsLValue: false, Value: val, Typ: typ}
		}
		if e.Operator == "&" {
			ptr, typ := b.buildAddressOf(e.Right)
			return ExprResult{IsLValue: false, Value: ptr, Typ: typ.PointerTo()}
		}
		if e.Operator == "*" {
			ptrVal := b.buildExpr(e.Right)
			return ExprResult{IsLValue: true, Address: ptrVal, Typ: ptrVal.Type().PointedType()}
		}

	case *ast.PointerType:
		ptrVal := b.buildExpr(e.Elt)
		return ExprResult{IsLValue: true, Address: ptrVal, Typ: ptrVal.Type().PointedType()}

	default:
		log.Panicf("NO CASE: Builder.eval: expr (%T)%v", expr, expr)
	}

	panic(fmt.Sprintf("Not Reached in eval: %T %v", expr, expr))
}

func (b *Builder) packVariadicArgs(f *Function, rawArgs []Value, tokenNode ast.Node) []Value {
	if f == nil || !f.IsVariadic || len(f.Parameters) == 0 {
		return rawArgs
	}

	varIdx := len(f.Parameters) - 1
	varTyp := f.Parameters[varIdx].Typ

	// Assume slice_ prefix
	eltTypName := varTyp.Name
	if strings.HasPrefix(eltTypName, "prelude.slice_") {
		eltTypName = strings.TrimPrefix(eltTypName, "prelude.slice_")
	} else if strings.HasPrefix(eltTypName, "slice_") {
		eltTypName = strings.TrimPrefix(eltTypName, "slice_")
	} else {
		panic("Variadic parameter type must be a slice, got: " + varTyp.Name)
	}

	eltTyp := Type{Name: eltTypName}
	numVarArgs := len(rawArgs) - varIdx
	if numVarArgs < 0 {
		numVarArgs = 0
	}

	arrTyp := Type{Name: fmt.Sprintf("[%d]%s", numVarArgs, eltTyp.Name)}
	var arrVal Value = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: arrTyp}}, tokenNode)

	for i := 0; i < numVarArgs; i++ {
		idxVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(i)}, tokenNode)
		var eltVal Value
		if eltTyp.Name == "prelude.any" || eltTyp.Name == "any" {
			eltVal = b.packageAsAny(rawArgs[varIdx+i], tokenNode)
		} else {
			eltVal = b.coerceType(rawArgs[varIdx+i], eltTyp)
		}
		arrVal = b.addInstr(&InsertElement{BaseInstruction: BaseInstruction{Typ: arrTyp}, Array: arrVal, Index: idxVal, Val: eltVal}, tokenNode)
	}

	arrPtr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: arrTyp.PointerTo()}, Local: arrVal}, tokenNode)
	basePtr := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: eltTyp.PointerTo()}, Op: "bitcast", Operand: arrPtr}, tokenNode)

	var sliceVal Value = b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: varTyp}}, tokenNode)
	sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: varTyp}, Struct: sliceVal, FieldIndex: 0, Val: basePtr}, tokenNode)
	lenVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(numVarArgs)}, tokenNode)
	sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: varTyp}, Struct: sliceVal, FieldIndex: 1, Val: lenVal}, tokenNode)
	sliceVal = b.addInstr(&InsertField{BaseInstruction: BaseInstruction{Typ: varTyp}, Struct: sliceVal, FieldIndex: 2, Val: lenVal}, tokenNode)

	newArgs := append([]Value(nil), rawArgs[:varIdx]...)
	newArgs = append(newArgs, sliceVal)
	return newArgs
}

func (b *Builder) buildAddressOf(expr ast.Expression) (Value, Type) {
	if idxExpr, ok := expr.(*ast.IndexExpression); ok {
		base := b.eval(idxExpr.Left)
		if strings.HasPrefix(base.Typ.Name, "prelude.slice_") || strings.HasPrefix(base.Typ.Name, "slice_") {
			idx := b.buildExpr(idxExpr.Indices[0])
			isPtr := base.Typ.IsAPointer()
			var baseType string
			if isPtr {
				baseType = base.Typ.PointedType().Name
			} else {
				baseType = base.Typ.Name
			}
			funcName := MangleName(baseType) + "_Address"

			if _, exists := b.funcs[funcName]; !exists {
				if instInfo, ok := b.instantiatedTypes[baseType]; ok {
					rawGenericFuncName := instInfo.RawGenericName + "_Address"
					if tmpl, ok := b.genericTemplates[rawGenericFuncName]; ok {
						parts := strings.SplitN(baseType, ".", 2)
						b.instantiateGenericFunc(parts[0]+".Address", rawGenericFuncName, instInfo.ArgTyps, tmpl, expr.GetToken())
					}
				}
			}

			args := []Value{}
			if isPtr {
				if base.IsLValue {
					receiverVal := b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, expr)
					args = append(args, receiverVal)
				} else {
					args = append(args, base.Value)
				}
			} else {
				if base.IsLValue {
					args = append(args, base.Address)
				} else {
					log.Printf("warning: Cannot call method on non-lvalue struct %s (in buildAddressOf). Synthesizing temporary lvalue.", base.Typ.Name)
					tmpAddr := b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: base.Typ.PointerTo()}, Local: base.Value}, expr)
					args = append(args, tmpAddr)
				}
			}
			args = append(args, idx)

			funcObj := b.funcs[funcName]
			call := b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeWord}, Func: funcObj, Args: args}, expr)

			// Determine element type
			eltType := TypeByte
			instInfo, ok := b.instantiatedTypes[baseType]
			log.Printf("DEBUG buildAddressOf baseType=%q ok=%v len=%d", baseType, ok, len(instInfo.ArgTyps))
			if ok && len(instInfo.ArgTyps) > 0 {
				eltType = instInfo.ArgTyps[0]
			}

			ptrCast := b.addInstr(&Cast{BaseInstruction: BaseInstruction{Typ: eltType.PointerTo()}, Op: "bitcast", Operand: call}, expr)
			return ptrCast, eltType
		}
	}

	res := b.eval(expr)
	if !res.IsLValue {
		panic(fmt.Sprintf("Cannot take address of non-lvalue expression: %T", expr))
	}
	return res.Address, res.Typ
}

func (b *Builder) getTypeString(qname string) Type {
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
	defer func() { b.evaluatingType[qname] = false }()

	var fields []*ast.Field
	for i, f := range st.Fields {
		fields = append(fields, &ast.Field{
			Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
			Type: f.Type,
		})
	}
	structType := b.astToIRType(&ast.StructType{
		Fields: fields,
	})
	b.Program.TypeDefs[qname] = structType
	return structType
}

func (b *Builder) getTypeSize(typ Type) int {
	// NANDO-recent.
	if typ.Equals(TypeVoid) || typ.Equals(TypeByte) {
		return 1
	}
	if typ.Equals(TypeWord) || typ.Equals(TypeInt) || typ.Equals(TypeUint) {
		return b.WordSize
	}
	if typ.IsAPointer() {
		return b.WordSize
	}
	if typ.IsAnArray() {
		idx := strings.Index(typ.Name, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ.Name[1:idx])
			eltSize := b.getTypeSize(typ.ArrayElementType())
			return length * eltSize
		}
	}
	if !typ.IsAStruct() {
		if _, ok := b.typeDefsAST[typ.Name]; ok {
			typ = b.getTypeString(typ.Name)
		} else if def, ok := b.Program.TypeDefs[typ.Name]; ok {
			typ = def
		} else {
			return 2
		}
	}
	if typ.IsAStruct() {
		size := 0
		st := typ.Expr.(*ast.StructType)
		for _, f := range st.Fields {
			size += b.getTypeSize(b.astToIRType(f.Type))
		}
		return size
	}
	panic("why return 2")
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
			panic("unresolved:" + e.Value)
		}

		if b.evaluatingConst[targetName] {
			panic("unresolved:" + targetName)
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
		case "&":
			return left & right
		case "|":
			return left | right
		case "^":
			return left ^ right
		case "<<":
			return left << right
		case ">>":
			return left >> right
		case "&^":
			return left &^ right
		}
	case *ast.CallExpression:
		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok && ident.Value == "sizeof" {
				typ := b.astToIRType(idxExpr.Indices[0])
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
				if b.isDestructable(targetType) {
					b.destructables = append(b.destructables, ident.Value)
				}
			}
			val = b.coerceType(val, targetType)
			b.writeVariable(ident.Value, b.currentBlock, val)
			return
		}
	}

	if idxExpr, ok := lhs.(*ast.IndexExpression); ok {
		base := b.eval(idxExpr.Left)
		if strings.HasPrefix(base.Typ.Name, "prelude.slice_") || strings.HasPrefix(base.Typ.Name, "slice_") {
			idx := b.buildExpr(idxExpr.Indices[0])
			isPtr := base.Typ.IsAPointer()
			var baseType string
			if isPtr {
				baseType = base.Typ.PointedType().Name
			} else {
				baseType = base.Typ.Name
			}
			funcName := MangleName(baseType) + "_Put"

			if _, exists := b.funcs[funcName]; !exists {
				if instInfo, ok := b.instantiatedTypes[baseType]; ok {
					rawGenericFuncName := instInfo.RawGenericName + "_Put"
					if tmpl, ok := b.genericTemplates[rawGenericFuncName]; ok {
						parts := strings.SplitN(baseType, ".", 2)
						b.instantiateGenericFunc(parts[0]+".Put", rawGenericFuncName, instInfo.ArgTyps, tmpl, lhs.GetToken())
					}
				}
			}

			if f, exists := b.funcs[funcName]; exists {
				var receiverVal Value
				if isPtr {
					if base.IsLValue {
						receiverVal = b.addInstr(&LoadPtr{BaseInstruction: BaseInstruction{Typ: base.Typ}, Ptr: base.Address}, lhs)
					} else {
						receiverVal = base.Value
					}
				} else {
					if base.IsLValue {
						receiverVal = base.Address
					} else {
						log.Printf("warning: Cannot call method on non-lvalue struct %s (in assignToExpr). Synthesizing temporary lvalue.", base.Typ.Name)
						receiverVal = b.addInstr(&AddressOfLocal{BaseInstruction: BaseInstruction{Typ: base.Typ.PointerTo()}, Local: base.Value}, lhs)
					}
				}
				val = b.coerceType(val, f.Parameters[2].Typ)
				args := []Value{receiverVal, idx, val}
				b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Func: f, Args: args}, lhs)
				return
			}
			panic("Slice Put method not found: " + funcName)
		}
	}

	res := b.eval(lhs)
	if !res.IsLValue {
		panic(fmt.Sprintf("Cannot assign to expression: %T", lhs))
	}
	val = b.coerceType(val, res.Typ)
	b.addInstr(&StorePtr{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Ptr: res.Address, Val: val}, lhs)
}

func extractTypeParamsIR(paramType ast.Expression, argTyp Type, typeMap map[string]Type, typeParams []string) {
	if ident, ok := paramType.(*ast.Identifier); ok {
		for _, tp := range typeParams {
			if tp == ident.Value {
				typeMap[tp] = argTyp
				return
			}
		}
	} else if prefix, ok := paramType.(*ast.PrefixExpression); ok && prefix.Operator == "*" {
		if argTyp.IsAPointer() {
			extractTypeParamsIR(prefix.Right, argTyp.PointedType(), typeMap, typeParams)
		}
	} else if ptr, ok := paramType.(*ast.PointerType); ok {
		if argTyp.IsAPointer() {
			extractTypeParamsIR(ptr.Elt, argTyp.PointedType(), typeMap, typeParams)
		}
	} else if idx, ok := paramType.(*ast.IndexExpression); ok {
		// nando-BAD.  Spliting on _ cannot work.
		parts := strings.Split(argTyp.Name, "_")
		numIdx := len(idx.Indices)
		if len(parts) >= numIdx {
			for i, innerIdx := range idx.Indices {
				extractTypeParamsIR(innerIdx, Type{Name: parts[len(parts)-numIdx+i]}, typeMap, typeParams)
			}
		}
	}
}

func (b *Builder) tryResolve(item *GlobalItem) (err error) {
	log.Printf("# tryResolve (%T)%v", item, item)

	defer func() {
		if r := recover(); r != nil {
			if errStr, ok := r.(string); ok && len(errStr) > 11 && errStr[:11] == "unresolved:" {
				err = fmt.Errorf("%s", errStr[11:])
			} else {
				panic(r) // re-panic if it's a real bug
			}
		}
	}()

	parts := strings.SplitN(item.QName, ".", 2)
	if len(parts) == 2 {
		b.currentPackage = parts[0]
	} else {
		b.currentPackage = ""
	}

	switch item.Kind {
	case ItemGenericType:
		s := item.ASTNode.(*ast.TypeStatement)
		var typeParams []string
		for _, tp := range s.TypeParameters {
			typeParams = append(typeParams, tp.Value)
		}
		b.genericTemplates[item.QName] = &GenericTemplate{
			TypeParams: typeParams,
			Tokens:     s.Tokens,
		}
	case ItemAlias:
		s := item.ASTNode.(*ast.TypeStatement)
		b.typeAliases[item.QName] = s.BaseType
		// try to resolve the base type to make sure it's valid
		b.astToIRType(s.BaseType)
	case ItemUnknown:
		// Fallback for types that are not struct, not alias, etc.
		// Historically ignored in pass 0.
	case ItemType:
		s := item.ASTNode.(*ast.TypeStatement)
		if st, ok := s.BaseType.(*ast.StructType); ok {
			b.typeDefsAST[item.QName] = st
		}

		// Ensure TypeDefOrder only contains resolved ones in order
		found := false
		for _, n := range b.Program.TypeDefOrder {
			if n == item.QName {
				found = true
				break
			}
		}
		if !found {
			b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, item.QName)
		}
		if _, ok := s.BaseType.(*ast.StructType); ok {
			b.getTypeString(item.QName) // resolves sizes
		} else if _, ok := s.BaseType.(*ast.FuncType); ok {
			b.Program.TypeDefs[item.QName] = TypeWord
		}
	case ItemConst:
		s := item.ASTNode.(*ast.ConstStatement)
		b.constExprs[item.QName] = s.Value
		val := b.EvalConst(&ast.Identifier{Value: item.QName})
		b.consts[item.QName] = &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(val)}
	case ItemVar:
		s := item.ASTNode.(*ast.VarStatement)
		var typ Type
		if s.ValueType != nil {
			typ = b.astToIRType(s.ValueType)
		} else if s.Value != nil {
			if lit, ok := s.Value.(*ast.CompositeLit); ok {
				typ = b.astToIRType(lit.Type)
			} else if arrLit, ok := s.Value.(*ast.ArrayType); ok {
				typ = b.astToIRType(arrLit)
			} else if _, ok := s.Value.(*ast.IntegerLiteral); ok {
				typ = TypeWord
			} else if _, ok := s.Value.(*ast.StringLiteral); ok {
				if _, ok := b.Program.TypeDefs["prelude.slice_byte"]; ok {
					typ = Type{Name: "prelude.slice_byte"}
				} else {
					typ = Type{Name: "slice_byte"}
				}
			} else {
				panic("Cannot infer type for global variable: " + item.QName)
			}
		} else {
			panic("Global variable without type must have a value: " + item.QName)
		}
		g := &Global{Name: item.QName, Typ: typ}
		b.globals[g.Name] = g
		b.Program.Globals = append(b.Program.Globals, g)
		if s.Value != nil {
			b.varInitStatements = append(b.varInitStatements, item)
		}
	case ItemGenericFunc:
		s := item.ASTNode.(*ast.FuncStatement)
		var typeParams []string
		for _, tp := range s.TypeParameters {
			typeParams = append(typeParams, tp.Value)
		}
		b.genericTemplates[item.QName] = &GenericTemplate{
			TypeParams: typeParams,
			Tokens:     s.Tokens,
		}
	case ItemFunc:
		s := item.ASTNode.(*ast.FuncStatement)
		b.registerFunc(s)
	}

	return nil
}

func (b *Builder) addStringConstant(val string) *Global {
	if existing, ok := b.stringConstants[val]; ok {
		return existing
	}

	name := fmt.Sprintf("str_const_%d", len(b.Program.Globals))
	valWithNull := val + "\x00"
	g := &Global{
		Name:       name,
		Typ:        Type{Name: fmt.Sprintf("[%d]byte", len(valWithNull))},
		InitString: valWithNull,
		IsInit:     true,
	}
	b.Program.Globals = append(b.Program.Globals, g)
	b.stringConstants[val] = g
	return g
}

func (t Type) ArrayLength() int {
	idx := strings.Index(t.Name, "]")
	if idx != -1 && strings.HasPrefix(t.Name, "[") {
		length, _ := strconv.Atoi(t.Name[1:idx])
		return length
	}
	return 0
}

func (b *Builder) getFuncReturnType(returnTypes []ast.Expression) Type {
	if len(returnTypes) == 1 {
		return b.astToIRType(returnTypes[0])
	} else if len(returnTypes) > 1 {
		var fields []*ast.Field
		for i, rt := range returnTypes {
			fields = append(fields, &ast.Field{
				Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
				Type: rt,
			})
		}
		structTyp := &ast.StructType{Fields: fields}
		name := "struct{"
		for _, rt := range returnTypes {
			name += b.astToIRType(rt).Name + ";"
		}
		name += "}"
		return Type{Expr: structTyp, Name: name}
	} else {
		return TypeVoid
	}
}

func (b *Builder) isConstantExpr(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return true
	case *ast.StringLiteral:
		return true
	case *ast.PrefixExpression:
		if e.Operator == "-" || e.Operator == "+" {
			return b.isConstantExpr(e.Right)
		}
		return false
	case *ast.CompositeLit:
		for _, el := range e.Elements {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if !b.isConstantExpr(kv.Value) {
					return false
				}
			} else {
				if !b.isConstantExpr(el) {
					return false
				}
			}
		}
		return true
	case *ast.Identifier:
		fullName := e.FullName()
		if _, ok := b.consts[fullName]; ok {
			return true
		}
		return false
	}
	return false
}

func (b *Builder) zeroConstant(typ Type) Value {
	if typ.Equals(TypeByte) {
		return &ConstByte{BaseInstruction: BaseInstruction{Typ: typ}, Val: 0}
	}
	if typ.Equals(TypeWord) || typ.Equals(TypeInt) || typ.Equals(TypeUint) {
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: typ}, Val: 0}
	}
	if typ.IsAStruct() {
		st, ok := b.typeDefsAST[typ.Name]
		if !ok {
			panic("unknown struct type for zeroing: " + typ.Name)
		}
		var fields []Value
		for _, f := range st.Fields {
			fTyp := b.astToIRType(f.Type)
			fields = append(fields, b.zeroConstant(fTyp))
		}
		return &ConstStruct{BaseInstruction: BaseInstruction{Typ: typ}, Fields: fields}
	}
	if typ.IsAnArray() {
		var fields []Value
		eltTyp := typ.ArrayElementType()
		length := typ.ArrayLength()
		for i := 0; i < length; i++ {
			fields = append(fields, b.zeroConstant(eltTyp))
		}
		return &ConstStruct{BaseInstruction: BaseInstruction{Typ: typ}, Fields: fields}
	}
	return &ConstWord{BaseInstruction: BaseInstruction{Typ: typ}, Val: 0}
}

func (b *Builder) evalConstantExpr(expr ast.Expression, targetTyp Type) Value {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		if targetTyp.Equals(TypeByte) {
			return &ConstByte{BaseInstruction: BaseInstruction{Typ: targetTyp}, Val: uint8(e.Value)}
		}
		return &ConstWord{BaseInstruction: BaseInstruction{Typ: targetTyp}, Val: uint64(e.Value)}
	case *ast.StringLiteral:
		g := b.addStringConstant(e.Value)
		length := int64(len(e.Value))
		lenVal := &ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(length)}
		return &ConstStruct{
			BaseInstruction: BaseInstruction{Typ: targetTyp},
			Fields: []Value{
				&AddressOfGlobal{BaseInstruction: BaseInstruction{Typ: TypeByte.PointerTo()}, Global: g},
				lenVal,
				lenVal,
			},
		}
	case *ast.PrefixExpression:
		if e.Operator == "-" {
			val := b.evalConstantExpr(e.Right, targetTyp)
			if cw, ok := val.(*ConstWord); ok {
				return &ConstWord{BaseInstruction: BaseInstruction{Typ: targetTyp}, Val: uint64(-int64(cw.Val))}
			}
			if cb, ok := val.(*ConstByte); ok {
				return &ConstByte{BaseInstruction: BaseInstruction{Typ: targetTyp}, Val: uint8(-int8(cb.Val))}
			}
		} else if e.Operator == "+" {
			return b.evalConstantExpr(e.Right, targetTyp)
		}
	case *ast.Identifier:
		fullName := e.FullName()
		if c, ok := b.consts[fullName]; ok {
			if cw, ok := c.(*ConstWord); ok {
				if targetTyp.Equals(TypeByte) {
					return &ConstByte{BaseInstruction: BaseInstruction{Typ: TypeByte}, Val: uint8(cw.Val)}
				}
				return &ConstWord{BaseInstruction: BaseInstruction{Typ: targetTyp}, Val: cw.Val}
			}
		}
	case *ast.ArrayType:
		if comp, ok := e.Elt.(*ast.CompositeLit); ok {
			return b.evalConstantExpr(comp, targetTyp)
		}
	case *ast.CompositeLit:
		if strings.HasPrefix(targetTyp.Name, "[") {
			// Array literal
			var arrayLen int
			var eltTypStr string
			fmt.Sscanf(targetTyp.Name, "[%d]%s", &arrayLen, &eltTypStr)
			eltTyp := b.astToIRType(&ast.Identifier{Value: eltTypStr})

			elements := make([]Value, arrayLen)
			for i, el := range e.Elements {
				if i >= arrayLen {
					panic("too many elements in array literal")
				}
				elements[i] = b.evalConstantExpr(el, eltTyp)
			}
			for i := range elements {
				if elements[i] == nil {
					elements[i] = b.zeroConstant(eltTyp)
				}
			}
			return &ConstArray{BaseInstruction: BaseInstruction{Typ: targetTyp}, Elements: elements}
		}

		st, ok := b.typeDefsAST[targetTyp.Name]
		if !ok {
			panic("constant struct of unknown type " + targetTyp.Name)
		}
		fieldIdxMap := make(map[string]int)
		for i, f := range st.Fields {
			fieldIdxMap[f.Name.Value] = i
		}

		fields := make([]Value, len(st.Fields))
		for i, el := range e.Elements {
			fieldIdx := i
			var valExpr ast.Expression
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				ident := kv.Key.(*ast.Identifier)
				fieldIdx = fieldIdxMap[ident.Value]
				valExpr = kv.Value
			} else {
				valExpr = el
			}
			fTyp := b.astToIRType(st.Fields[fieldIdx].Type)
			fields[fieldIdx] = b.evalConstantExpr(valExpr, fTyp)
		}
		for i, f := range fields {
			if f == nil {
				fTyp := b.astToIRType(st.Fields[i].Type)
				fields[i] = b.zeroConstant(fTyp)
			}
		}
		return &ConstStruct{BaseInstruction: BaseInstruction{Typ: targetTyp}, Fields: fields}
	}
	panic(fmt.Sprintf("Not a constant or unsupported constant expr: %T", expr))
}

func (b *Builder) buildSyntheticInit() {
	f := &Function{Name: "init_main", ReturnType: TypeVoid}
	b.funcs[f.Name] = f
	b.Program.Functions = append(b.Program.Functions, f)

	oldFunc := b.currentFunc
	b.currentFunc = f
	b.nextValueID = 1
	b.nextBlockID = 1
	b.currentDef = make(map[*BasicBlock]map[string]Value)
	b.sealedBlocks = make(map[*BasicBlock]bool)
	b.incompletePhis = make(map[*BasicBlock]map[string]*Phi)
	b.varTypes = make(map[string]Type)

	entry := b.newBlock()
	b.currentBlock = entry
	b.sealBlock(entry)

	for _, item := range b.varInitStatements {
		s := item.ASTNode.(*ast.VarStatement)
		g := b.globals[item.QName]

		var val Value
		if b.isConstantExpr(s.Value) {
			constVal := b.evalConstantExpr(s.Value, g.Typ)

			constName := fmt.Sprintf(".const_struct_%d", len(b.Program.Globals))
			constGlobal := &Global{
				Name:    constName,
				Typ:     g.Typ,
				InitVal: constVal,
				IsInit:  true,
			}
			b.globals[constName] = constGlobal
			b.Program.Globals = append(b.Program.Globals, constGlobal)

			val = b.addInstr(&Load{BaseInstruction: BaseInstruction{Typ: g.Typ}, Global: constGlobal}, s)
		} else {
			val = b.buildExpr(s.Value)
			val = b.coerceType(val, g.Typ)
		}

		b.addInstr(&Store{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Global: g, Val: val}, s)
	}

	b.addInstr(&Return{BaseInstruction: BaseInstruction{Typ: TypeVoid}}, nil)
	b.currentFunc = oldFunc
}

func (b *Builder) isDestructable(t Type) bool {
	if t.IsAPointer() {
		return false
	}
	methodName := MangleName(t.Name) + "_destructor"
	if _, ok := b.funcs[methodName]; ok {
		return true
	}
	if t.IsAStruct() && t.Expr != nil {
		if st, ok := t.Expr.(*ast.StructType); ok {
			for _, field := range st.Fields {
				fieldTyp := b.astToIRType(field.Type)
				if b.isDestructable(fieldTyp) {
					return true
				}
			}
		}
	}
	if st, ok := b.typeDefsAST[t.Name]; ok {
		for _, field := range st.Fields {
			fieldTyp := b.astToIRType(field.Type)
			if b.isDestructable(fieldTyp) {
				return true
			}
		}
	}
	if t.IsAnArray() && t.Expr != nil {
		if arr, ok := t.Expr.(*ast.ArrayType); ok {
			elemTyp := b.astToIRType(arr.Elt)
			if b.isDestructable(elemTyp) {
				return true
			}
		}
	}
	return false
}

func (b *Builder) emitDestruction(typ Type, ptrVal Value, tokenNode ast.Node) {
	if typ.IsAPointer() {
		return
	}
	// Direct destructor
	methodName := MangleName(typ.Name) + "_destructor"
	if f, ok := b.funcs[methodName]; ok {
		b.addInstr(&Call{BaseInstruction: BaseInstruction{Typ: TypeVoid}, Func: f, Args: []Value{ptrVal}}, tokenNode)
	}
	// Struct fields
	if typ.IsAStruct() && typ.Expr != nil {
		if st, ok := typ.Expr.(*ast.StructType); ok {
			for i, field := range st.Fields {
				fieldTyp := b.astToIRType(field.Type)
				if b.isDestructable(fieldTyp) {
					fieldPtr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: fieldTyp.PointerTo()}, Ptr: ptrVal, FieldIndex: i}, tokenNode)
					b.emitDestruction(fieldTyp, fieldPtr, tokenNode)
				}
			}
		}
	} else if st, ok := b.typeDefsAST[typ.Name]; ok {
		for i, field := range st.Fields {
			fieldTyp := b.astToIRType(field.Type)
			if b.isDestructable(fieldTyp) {
				fieldPtr := b.addInstr(&AddressOfField{BaseInstruction: BaseInstruction{Typ: fieldTyp.PointerTo()}, Ptr: ptrVal, FieldIndex: i}, tokenNode)
				b.emitDestruction(fieldTyp, fieldPtr, tokenNode)
			}
		}
	}
	// Array elements
	if typ.IsAnArray() && typ.Expr != nil {
		if arr, ok := typ.Expr.(*ast.ArrayType); ok {
			elemTyp := b.astToIRType(arr.Elt)
			if b.isDestructable(elemTyp) {
				lenVal := b.EvalConst(arr.Length)
				for i := int64(0); i < lenVal; i++ {
					idxVal := b.addInstr(&ConstWord{BaseInstruction: BaseInstruction{Typ: TypeWord}, Val: uint64(i)}, tokenNode)
					elemPtr := b.addInstr(&AddressOfElement{BaseInstruction: BaseInstruction{Typ: elemTyp.PointerTo()}, ArrayPtr: ptrVal, Index: idxVal}, tokenNode)
					b.emitDestruction(elemTyp, elemPtr, tokenNode)
				}
			}
		}
	}
}
