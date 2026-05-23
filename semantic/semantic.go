package semantic

import (
	"fmt"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/token"
)

type Symbol struct {
	Name string
	Type ast.Expression
}

type Scope struct {
	parent  *Scope
	symbols map[string]Symbol
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent:  parent,
		symbols: make(map[string]Symbol),
	}
}

func (s *Scope) Define(name string, typ ast.Expression) {
	s.symbols[name] = Symbol{Name: name, Type: typ}
}

func (s *Scope) Resolve(name string) (Symbol, bool) {
	if sym, ok := s.symbols[name]; ok {
		return sym, true
	}
	if s.parent != nil {
		return s.parent.Resolve(name)
	}
	return Symbol{}, false
}

type GenericTemplate struct {
	TypeParams []string
	Tokens     []token.Token
}

type Analyzer struct {
	errors           []string
	globalScope      *Scope
	currentScope     *Scope
	hasMainPackage   bool
	hasMainFunc      bool
	currentPackage   string
	program          *ast.Program
	genericTemplates map[string]*GenericTemplate
	funcMap          map[string]*ast.FuncStatement
	reachableFuncs   map[string]bool
	queue            []string
}

func builtinType(name string) ast.Expression {
	return &ast.Identifier{Token: token.Token{Type: token.IDENT, Literal: name}, Value: name}
}

var UnknownType = builtinType("UnknownType")
var WordType = builtinType("word")
var ByteType = builtinType("byte")
var AnyType = builtinType("any")
var FuncTypeBuiltin = builtinType("func")

func New() *Analyzer {
	global := NewScope(nil)
	// Built-ins
	global.Define("print", FuncTypeBuiltin)
	global.Define("println", FuncTypeBuiltin)
	global.Define("exit", FuncTypeBuiltin)
	global.Define("sizeof", FuncTypeBuiltin)
	global.Define("len", FuncTypeBuiltin)
	global.Define("cap", FuncTypeBuiltin)

	global.Define("byte", builtinType("type"))
	global.Define("word", builtinType("type"))
	global.Define("int", builtinType("type"))
	global.Define("uint", builtinType("type"))
	global.Define("any", builtinType("type"))
	global.Define("string", &ast.ArrayType{Elt: ByteType}) // string is alias for slice[byte]

	return &Analyzer{
		errors:           []string{},
		globalScope:      global,
		currentScope:     global,
		genericTemplates: make(map[string]*GenericTemplate),
		funcMap:          make(map[string]*ast.FuncStatement),
		reachableFuncs:   make(map[string]bool),
		queue:            []string{},
	}
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func exprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value
	case *ast.ArrayType:
		return "slice_" + exprToString(e.Elt)
	case *ast.PointerType:
		return "*" + exprToString(e.Elt)
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			return pkgIdent.Value + "." + e.Right.Value
		}
		return exprToString(e.Left) + "." + e.Right.Value
	}
	return expr.TokenLiteral()
}

func typesEqual(t1, t2 ast.Expression) bool {
	if t1 == UnknownType || t2 == UnknownType {
		return true // Prevent cascade errors
	}
	return exprToString(t1) == exprToString(t2)
}

func isFuncType(typ ast.Expression) bool {
	if typ == FuncTypeBuiltin {
		return true
	}
	_, ok := typ.(*ast.FuncType)
	return ok
}

func (a *Analyzer) markReachable(qname string) {
	if !a.reachableFuncs[qname] {
		a.reachableFuncs[qname] = true
		a.queue = append(a.queue, qname)
	}
}

func (a *Analyzer) Analyze(program *ast.Program) {
	a.program = program

	// Pass 1a: Collect generic templates
	a.currentPackage = ""
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			a.currentPackage = s.Name.Value
		case *ast.TypeStatement:
			if len(s.TypeParameters) > 0 {
				qname := a.currentPackage + "." + s.Name.Value
				var tparams []string
				for _, tp := range s.TypeParameters {
					tparams = append(tparams, tp.Value)
				}
				a.genericTemplates[qname] = &GenericTemplate{TypeParams: tparams, Tokens: s.Tokens}
			}
		case *ast.FuncStatement:
			if len(s.TypeParameters) > 0 {
				qname := a.currentPackage + "." + s.Name.Value
				if s.Receiver != nil {
					// Simplified for pass 1a
					if pt, ok := s.Receiver.Type.(*ast.PointerType); ok {
						if idx, ok := pt.Elt.(*ast.IndexExpression); ok {
							if id, ok := idx.Left.(*ast.Identifier); ok {
								qname = a.currentPackage + "." + id.Value + "_" + s.Name.Value
							}
						}
					}
				}
				var tparams []string
				for _, tp := range s.TypeParameters {
					tparams = append(tparams, tp.Value)
				}
				a.genericTemplates[qname] = &GenericTemplate{TypeParams: tparams, Tokens: s.Tokens}
			}
		}
	}

	// Pass 1: define global symbols, map functions, and scan global vars
	a.currentPackage = ""
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			a.currentPackage = s.Name.Value
			if s.Name.Value == "main" {
				a.hasMainPackage = true
			}
		case *ast.FuncStatement:
			if a.currentPackage == "main" && s.Name.Value == "main" && s.Receiver == nil {
				a.hasMainFunc = true
			}

			qname := a.currentPackage + "." + s.Name.Value
			if s.Receiver != nil {
				qname = exprToString(s.Receiver.Type)
				qname = strings.TrimPrefix(qname, "*")
				if !strings.Contains(qname, ".") {
					if _, ok := a.globalScope.Resolve("prelude." + qname); ok {
						qname = "prelude." + qname
					} else {
						qname = a.currentPackage + "." + qname
					}
				}
				qname = qname + "_" + s.Name.Value
			}

			if len(s.TypeParameters) == 0 {
				var retTypes []ast.Expression
				for _, r := range s.ReturnTypes {
					retTypes = append(retTypes, r)
				}
				ft := &ast.FuncType{Parameters: s.Parameters, ReturnTypes: retTypes}
				a.globalScope.Define(qname, ft)
			}
			a.funcMap[qname] = s

		case *ast.VarStatement:
			typ := UnknownType
			if s.ValueType != nil {
				typ = a.analyzeExpression(s.ValueType)
			}
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, typ)
			// Any global var assignments? If there's an initializer, it might call functions!
			// In minigolf, ast.VarStatement does not have Values. Wait, yes it does?
			// Actually VarStatement has no initializers in ast.go right now. Wait, let's look at AST later.

		case *ast.ConstStatement:
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, WordType)
		case *ast.TypeStatement:
			qname := a.currentPackage + "." + s.Name.Value
			if len(s.TypeParameters) > 0 {
				// Don't analyze base type for generic templates until instantiated
				a.globalScope.Define(qname, s.BaseType)
			} else {
				a.globalScope.Define(qname, builtinType(qname))
			}
		}
	}

	if !a.hasMainPackage {
		a.errors = append(a.errors, "missing 'package main'")
	}
	if a.hasMainPackage && !a.hasMainFunc {
		a.errors = append(a.errors, "missing 'main' function in 'package main'")
	}

	// Pass 2: Reachability-driven Type Checking (DCE)
	a.markReachable("main.main")
	a.markReachable("prelude.init_0")
	// Also mark all builtins as reachable so we don't accidentally drop them if someone manually defined them.
	// Actually, just init_0 and main.main is enough for roots.

	for len(a.queue) > 0 {
		qname := a.queue[0]
		a.queue = a.queue[1:]

		parts := strings.Split(qname, ".")
		if len(parts) >= 2 {
			a.currentPackage = parts[0]
		}

		if fs, ok := a.funcMap[qname]; ok {
			a.analyzeFunc(fs)
		}
	}

	// Pass 3: Filter AST
	var reachableStatements []ast.Statement
	a.currentPackage = ""
	for _, stmt := range program.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			a.currentPackage = ps.Name.Value
		}
		if fs, ok := stmt.(*ast.FuncStatement); ok {
			qname := a.currentPackage + "." + fs.Name.Value
			if fs.Receiver != nil {
				qname = exprToString(fs.Receiver.Type)
				qname = strings.TrimPrefix(qname, "*")
				if !strings.Contains(qname, ".") {
					if _, ok := a.globalScope.Resolve("prelude." + qname); ok {
						qname = "prelude." + qname
					} else {
						qname = a.currentPackage + "." + qname
					}
				}
				qname = qname + "_" + fs.Name.Value
			}

			if len(fs.TypeParameters) > 0 {
				reachableStatements = append(reachableStatements, stmt)
				continue
			}

			if !a.reachableFuncs[qname] {

				continue // DEAD CODE ELIMINATED!
			}
		}
		reachableStatements = append(reachableStatements, stmt)
	}
	program.Statements = reachableStatements
}

func (a *Analyzer) analyzeFunc(s *ast.FuncStatement) {
	if len(s.TypeParameters) > 0 {
		return // Do not analyze generic templates until instantiated
	}

	a.currentScope = NewScope(a.currentScope)
	defer func() { a.currentScope = a.currentScope.parent }()

	if s.Receiver != nil {
		a.currentScope.Define(s.Receiver.Name.Value, a.analyzeExpression(s.Receiver.Type))
	}

	for _, p := range s.Parameters {
		a.currentScope.Define(p.Name.Value, a.analyzeExpression(p.Type))
	}

	if s.Body != nil {
		a.analyzeBlock(s.Body)
	}
}

func (a *Analyzer) analyzeBlock(b *ast.BlockStatement) {
	var newStatements []ast.Statement
	for _, stmt := range b.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			if s.Value != nil {
				s.Value = foldExpression(s.Value)
			}
			typ := UnknownType
			if s.ValueType != nil {
				typ = a.analyzeExpression(s.ValueType)
			} else if s.Value != nil {
				typ = a.analyzeExpression(s.Value)
			}
			a.currentScope.Define(s.Name.Value, typ)
		case *ast.AssignStatement:
			if s.Token.Literal == ":=" {
				for i, nameExpr := range s.Names {
					typ := UnknownType
					if i < len(s.Values) {
						s.Values[i] = foldExpression(s.Values[i])
						typ = a.analyzeExpression(s.Values[i])
					}
					if name, ok := nameExpr.(*ast.Identifier); ok {
						a.currentScope.Define(name.Value, typ)
					} else {
						a.analyzeExpression(nameExpr)
					}
				}
			} else {
				for i, nameExpr := range s.Names {
					s.Names[i] = foldExpression(nameExpr)
					a.analyzeExpression(s.Names[i])
				}
				for i, expr := range s.Values {
					s.Values[i] = foldExpression(expr)
					a.analyzeExpression(s.Values[i])
				}
			}
		case *ast.IfStatement:
			s.Condition = foldExpression(s.Condition)
			a.analyzeExpression(s.Condition)
			
			if intLit, ok := s.Condition.(*ast.IntegerLiteral); ok {
				if intLit.Value != 0 {
					a.analyzeBlock(s.Consequence)
					newStatements = append(newStatements, s.Consequence)
				} else if s.Alternative != nil {
					a.analyzeBlock(s.Alternative)
					newStatements = append(newStatements, s.Alternative)
				}
				continue // DEAD BRANCH ELIMINATED
			}
			
			a.analyzeBlock(s.Consequence)
			if s.Alternative != nil {
				a.analyzeBlock(s.Alternative)
			}
		case *ast.ForStatement:
			if s.Condition != nil {
				s.Condition = foldExpression(s.Condition)
				a.analyzeExpression(s.Condition)
			}
			a.analyzeBlock(s.Body)
		case *ast.For3Statement:
			if s.Init != nil {
				a.analyzeBlock(&ast.BlockStatement{Statements: []ast.Statement{s.Init}})
			}
			if s.Condition != nil {
				s.Condition = foldExpression(s.Condition)
				a.analyzeExpression(s.Condition)
			}
			if s.Increment != nil {
				a.analyzeBlock(&ast.BlockStatement{Statements: []ast.Statement{s.Increment}})
			}
			a.analyzeBlock(s.Body)
		case *ast.ForRangeStatement:
			s.RangeValue = foldExpression(s.RangeValue)
			rangeTyp := a.analyzeExpression(s.RangeValue)
			if s.IsDecl {
				if ident, ok := s.Key.(*ast.Identifier); ok {
					a.currentScope.Define(ident.Value, WordType)
				}
				if s.Value != nil {
					if ident, ok := s.Value.(*ast.Identifier); ok {
						valTyp := UnknownType
						if arrayTyp, ok := rangeTyp.(*ast.ArrayType); ok {
							valTyp = arrayTyp.Elt
						}
						a.currentScope.Define(ident.Value, valTyp)
					}
				}
			} else {
				a.analyzeExpression(s.Key)
				if s.Value != nil {
					a.analyzeExpression(s.Value)
				}
			}

			if s.Value != nil {
				baseTypStr := exprToString(rangeTyp)
				if strings.HasPrefix(baseTypStr, "prelude.slice_") || strings.HasPrefix(baseTypStr, "slice_") {
					var eltTypeStr string
					if strings.HasPrefix(baseTypStr, "prelude.slice_") {
						eltTypeStr = strings.TrimPrefix(baseTypStr, "prelude.slice_")
					} else {
						eltTypeStr = strings.TrimPrefix(baseTypStr, "slice_")
					}
					methodsToInstantiate := []string{"Address", "Put", "Get", "Chop"}
					for _, m := range methodsToInstantiate {
						instName := baseTypStr + "_" + m
						if !strings.HasPrefix(instName, "prelude.") {
							instName = "prelude." + instName
						}
						a.instantiateGenericFunc(instName, "prelude.slice_"+m, []ast.Expression{builtinType(eltTypeStr)})
					}
				}
			}

			a.analyzeBlock(s.Body)
		case *ast.ReturnStatement:
			for i, rv := range s.ReturnValues {
				s.ReturnValues[i] = foldExpression(rv)
				a.analyzeExpression(s.ReturnValues[i])
			}
		case *ast.ExpressionStatement:
			s.Expression = foldExpression(s.Expression)
			a.analyzeExpression(s.Expression)
		case *ast.IncDecStatement:
			s.Name = foldExpression(s.Name)
			a.analyzeExpression(s.Name)
		}
		newStatements = append(newStatements, stmt)
	}
	b.Statements = newStatements
}

func (a *Analyzer) substituteGenericTokens(argTyps []ast.Expression, tmpl *GenericTemplate) []token.Token {
	var res []token.Token
	for _, tok := range tmpl.Tokens {
		newTok := tok
		if tok.Type == token.IDENT {
			for i, tp := range tmpl.TypeParams {
				if tok.Literal == tp && i < len(argTyps) {
					newTok.Literal = exprToString(argTyps[i])
				}
			}
		}
		res = append(res, newTok)
	}
	res = append(res, token.Token{Type: token.EOF, Literal: ""})
	return res
}

func (a *Analyzer) instantiateGenericType(instName string, qname string, argTyps []ast.Expression) {
	if _, ok := a.globalScope.Resolve(instName); ok {
		return
	} // Already instantiated
	tmpl, ok := a.genericTemplates[qname]
	if !ok {
		return
	}

	subTokens := a.substituteGenericTokens(argTyps, tmpl)
	p := parser.New(subTokens)
	stmt := p.ParseStatementForGeneric()

	if ts, ok := stmt.(*ast.TypeStatement); ok {
		ts.Name.Value = strings.TrimPrefix(instName, a.currentPackage+".")
		ts.TypeParameters = nil
		a.globalScope.Define(instName, ts.BaseType)
		a.program.Statements = append(a.program.Statements, ts)
	}
}

func (a *Analyzer) instantiateGenericFunc(instName string, qname string, argTyps []ast.Expression) {
	if _, ok := a.globalScope.Resolve(instName); ok {
		return
	}
	tmpl, ok := a.genericTemplates[qname]
	if !ok {
		return
	}

	subTokens := a.substituteGenericTokens(argTyps, tmpl)
	p := parser.New(subTokens)
	stmt := p.ParseStatementForGeneric()

	if fs, ok := stmt.(*ast.FuncStatement); ok {
		// Keep original name and receiver
		fs.TypeParameters = nil
		a.program.Statements = append(a.program.Statements, fs)
		a.funcMap[instName] = fs

		var retTypes []ast.Expression
		for _, r := range fs.ReturnTypes {
			retTypes = append(retTypes, r)
		}
		ft := &ast.FuncType{Parameters: fs.Parameters, ReturnTypes: retTypes}
		a.globalScope.Define(instName, ft)

		// Queue the instantiated function for reachability analysis!
		a.markReachable(instName)
	}
}

func (a *Analyzer) analyzeExpression(expr ast.Expression) ast.Expression {
	if expr == nil {
		return UnknownType
	}

	var typ ast.Expression = UnknownType

	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		typ = WordType
	case *ast.StringLiteral:
		typ = &ast.ArrayType{Elt: ByteType}
	case *ast.Identifier:
		qname := a.currentPackage + "." + e.Value

		if sym, ok := a.currentScope.Resolve(e.Value); ok {
			typ = sym.Type
			if isFuncType(typ) {
				a.markReachable(sym.Name)
			}
		} else if sym, ok := a.globalScope.Resolve(qname); ok {
			typ = sym.Type
			if isFuncType(typ) {
				a.markReachable(sym.Name)
			}
		} else if sym, ok := a.globalScope.Resolve("prelude." + e.Value); ok {
			typ = sym.Type
			if isFuncType(typ) {
				a.markReachable(sym.Name)
			}
		} else if _, ok := a.genericTemplates[qname]; ok {
			typ = builtinType("func") // It's a generic template func or type
		} else if _, ok := a.genericTemplates["prelude."+e.Value]; ok {
			typ = builtinType("func") // It's a prelude generic template
		} else {
			a.errors = append(a.errors, fmt.Sprintf("undefined identifier: %s", e.Value))
		}
	case *ast.InfixExpression:
		t1 := a.analyzeExpression(e.Left)
		t2 := a.analyzeExpression(e.Right)
		if e.Operator == "==" || e.Operator == "!=" || e.Operator == "<" || e.Operator == ">" || e.Operator == "<=" || e.Operator == ">=" {
			typ = WordType

			// ir.Builder uses prelude helpers for comparing slices and structs
			t1Str := exprToString(t1)
			if t1Str == "slice_byte" || t1Str == "prelude.slice_byte" || t1Str == "string" {
				if e.Operator == "==" || e.Operator == "!=" {
					a.markReachable("prelude.streq")
				} else {
					a.markReachable("prelude.strcmp")
				}
			} else {
				isStruct := false
				if _, ok := t1.(*ast.StructType); ok {
					isStruct = true
				} else if _, ok := t1.(*ast.ArrayType); ok {
					isStruct = true
				} else if _, ok := a.globalScope.Resolve(t1Str); ok {
					// We just assume it might be a struct if it's resolved, to be safe.
					// We only check if it is explicitly WordType or ByteType to avoid memeq for simple types.
					if t1 != WordType && t1 != ByteType && t1 != UnknownType {
						isStruct = true
					}
				}
				if isStruct && (e.Operator == "==" || e.Operator == "!=") {
					a.markReachable("prelude.memeq")
				}
			}
		} else {
			if t1 != UnknownType {
				typ = t1
			} else {
				typ = t2
			}
		}
	case *ast.PrefixExpression:
		typ = a.analyzeExpression(e.Right)
		if e.Operator == "&" {
			typ = &ast.PointerType{Elt: typ}
		} else if e.Operator == "*" {
			if pt, ok := typ.(*ast.PointerType); ok {
				typ = pt.Elt
			}
		}
	case *ast.CallExpression:
		funcTyp := a.analyzeExpression(e.Function)
		var argTyps []ast.Expression
		for _, arg := range e.Arguments {
			argTyps = append(argTyps, a.analyzeExpression(arg))
		}

		// If calling a generic struct like slice[byte]
		if _, ok := e.Function.(*ast.IndexExpression); ok {
			// It's not a function call, it's a cast like slice[byte](x)
			typ = funcTyp
		} else if ft, ok := funcTyp.(*ast.FuncType); ok {
			if len(argTyps) != len(ft.Parameters) && exprToString(funcTyp) != "func" {
				a.errors = append(a.errors, fmt.Sprintf("argument count mismatch: expected %d, got %d", len(ft.Parameters), len(argTyps)))
			}
			if len(ft.ReturnTypes) > 0 {
				typ = ft.ReturnTypes[0]
			} else {
				typ = WordType // void essentially
			}
		} else {
			// Some other call expression, like int() or string()
			typ = funcTyp
		}
	case *ast.IndexExpression:
		qname := ""
		if id, ok := e.Left.(*ast.Identifier); ok {
			if _, ok := a.genericTemplates[a.currentPackage+"."+id.Value]; ok {
				qname = a.currentPackage + "." + id.Value
			} else if _, ok := a.genericTemplates["prelude."+id.Value]; ok {
				qname = "prelude." + id.Value
			}
		}

		var leftTyp ast.Expression = UnknownType
		if qname == "" {
			leftTyp = a.analyzeExpression(e.Left)
		}

		for _, idx := range e.Indices {
			a.analyzeExpression(idx)
		}

		if qname != "" {
			// It's a generic instantiation!
			instName := qname
			for _, idx := range e.Indices {
				instName += "_" + exprToString(idx) // Simplified
			}

			a.instantiateGenericType(instName, qname, e.Indices)
			typ = builtinType(instName)
		} else {
			if arrTyp, ok := leftTyp.(*ast.ArrayType); ok {
				typ = arrTyp.Elt
			}

			// ir.Builder will compile index expressions to a call to Address (or Put, Get, Chop).
			// We must ensure they are instantiated and marked reachable so their
			// dependencies (like prelude.mul_word) aren't dropped.
			baseTypStr := exprToString(leftTyp)
			if strings.HasPrefix(baseTypStr, "prelude.slice_") || strings.HasPrefix(baseTypStr, "slice_") {
				var eltTypeStr string
				if strings.HasPrefix(baseTypStr, "prelude.slice_") {
					eltTypeStr = strings.TrimPrefix(baseTypStr, "prelude.slice_")
				} else {
					eltTypeStr = strings.TrimPrefix(baseTypStr, "slice_")
				}

				if typ == UnknownType || typ == nil {
					typ = builtinType(eltTypeStr)
				}

				methodsToInstantiate := []string{"Address", "Put", "Get", "Chop"}
				for _, m := range methodsToInstantiate {
					instName := baseTypStr + "_" + m
					if !strings.HasPrefix(instName, "prelude.") {
						instName = "prelude." + instName
					}
					a.instantiateGenericFunc(instName, "prelude.slice_"+m, []ast.Expression{builtinType(eltTypeStr)})
				}
			}
		}

	case *ast.SelectorExpression:
		// If it's a package reference
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if sym, ok := a.globalScope.Resolve(qname); ok {
				typ = sym.Type
				if isFuncType(typ) {
					a.markReachable(sym.Name)
				}
				expr.SetResolvedType(typ)
				return typ
			}
		}

		leftTyp := a.analyzeExpression(e.Left)

		if typ == UnknownType {
			// It's a method call or field access!
			baseTypStr := exprToString(leftTyp)
			baseTypStr = strings.TrimPrefix(baseTypStr, "*")

			// Check for method
			methodName := baseTypStr + "_" + e.Right.Value
			if sym, ok := a.globalScope.Resolve(methodName); ok {
				typ = sym.Type
				a.markReachable(methodName)
			} else if sym, ok := a.globalScope.Resolve("prelude." + methodName); ok {
				typ = sym.Type
				a.markReachable(sym.Name)
			} else {
				// Could be a generic method like slice_byte_Chop!
				// We need to instantiate it.
				if strings.HasPrefix(baseTypStr, "prelude.slice_") || strings.HasPrefix(baseTypStr, "slice_") {
					var eltTypeStr string
					if strings.HasPrefix(baseTypStr, "prelude.slice_") {
						eltTypeStr = strings.TrimPrefix(baseTypStr, "prelude.slice_")
					} else {
						eltTypeStr = strings.TrimPrefix(baseTypStr, "slice_")
					}
					qname := "prelude.slice_" + e.Right.Value
					instName := baseTypStr + "_" + e.Right.Value
					if !strings.HasPrefix(instName, "prelude.") {
						instName = "prelude." + instName
					}
					a.instantiateGenericFunc(instName, qname, []ast.Expression{builtinType(eltTypeStr)})

					if sym, ok := a.globalScope.Resolve(instName); ok {
						typ = sym.Type
					}
				}
			}
		}
	}

	expr.SetResolvedType(typ)
	return typ
}
