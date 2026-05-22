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
	errors         []string
	globalScope    *Scope
	currentScope   *Scope
	hasMainPackage bool
	hasMainFunc    bool
	currentPackage string
	program        *ast.Program
	genericTemplates map[string]*GenericTemplate
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
		errors:       []string{},
		globalScope:  global,
		currentScope: global,
		genericTemplates: make(map[string]*GenericTemplate),
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

func (a *Analyzer) Analyze(program *ast.Program) {
	a.program = program
	// First pass: define global symbols and collect generic templates
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
				qname = qname + "_" + s.Name.Value
			}
			
			if len(s.TypeParameters) > 0 {
			    var tparams []string
			    for _, tp := range s.TypeParameters { tparams = append(tparams, tp.Value) }
				a.genericTemplates[qname] = &GenericTemplate{TypeParams: tparams, Tokens: s.Tokens}
			}

            var retTypes []ast.Expression
            for _, r := range s.ReturnTypes {
                retTypes = append(retTypes, r)
            }
            ft := &ast.FuncType{Parameters: s.Parameters, ReturnTypes: retTypes}
			a.globalScope.Define(qname, ft)

		case *ast.VarStatement:
			typ := UnknownType
			if s.ValueType != nil {
				typ = s.ValueType
			}
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, typ)
		case *ast.ConstStatement:
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, WordType) 
		case *ast.TypeStatement:
		    qname := a.currentPackage + "." + s.Name.Value
		    if len(s.TypeParameters) > 0 {
		        var tparams []string
			    for _, tp := range s.TypeParameters { tparams = append(tparams, tp.Value) }
				a.genericTemplates[qname] = &GenericTemplate{TypeParams: tparams, Tokens: s.Tokens}
		    }
			a.globalScope.Define(qname, s.BaseType)
		}
	}

	if !a.hasMainPackage {
		a.errors = append(a.errors, "missing 'package main'")
	}
	if a.hasMainPackage && !a.hasMainFunc {
		a.errors = append(a.errors, "missing 'main' function in 'package main'")
	}

	// Second pass: check function bodies
	a.currentPackage = ""
	// Iterate by index so we can append instantiated templates safely!
	for i := 0; i < len(program.Statements); i++ {
	    stmt := program.Statements[i]
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			a.currentPackage = ps.Name.Value
		}
		if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
			a.analyzeFunc(funcStmt)
		}
	}
}

func (a *Analyzer) analyzeFunc(s *ast.FuncStatement) {
    if len(s.TypeParameters) > 0 {
        return // Do not analyze generic templates until instantiated
    }

	a.currentScope = NewScope(a.currentScope)
	defer func() { a.currentScope = a.currentScope.parent }()

	if s.Receiver != nil {
		a.currentScope.Define(s.Receiver.Name.Value, s.Receiver.Type)
	}

	for _, p := range s.Parameters {
		a.currentScope.Define(p.Name.Value, p.Type)
	}

	if s.Body != nil {
		a.analyzeBlock(s.Body)
	}
}

func (a *Analyzer) analyzeBlock(b *ast.BlockStatement) {
	for _, stmt := range b.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := UnknownType
			if s.ValueType != nil {
				typ = s.ValueType
			}
			a.currentScope.Define(s.Name.Value, typ)
		case *ast.AssignStatement:
			if s.Token.Literal == ":=" {
			    for i, nameExpr := range s.Names {
					if name, ok := nameExpr.(*ast.Identifier); ok {
					    typ := UnknownType
					    if i < len(s.Values) {
					        typ = a.analyzeExpression(s.Values[i])
					    }
						a.currentScope.Define(name.Value, typ) 
					}
				}
			} else {
				for _, nameExpr := range s.Names {
				    a.analyzeExpression(nameExpr)
				}
				for _, expr := range s.Values {
				    a.analyzeExpression(expr)
			    }
			}
		case *ast.IfStatement:
			a.analyzeExpression(s.Condition)
			a.analyzeBlock(s.Consequence)
			if s.Alternative != nil {
				a.analyzeBlock(s.Alternative)
			}
		case *ast.ForStatement:
			if s.Condition != nil {
				a.analyzeExpression(s.Condition)
			}
			a.analyzeBlock(s.Body)
		case *ast.For3Statement:
			if s.Init != nil {
				a.analyzeBlock(&ast.BlockStatement{Statements: []ast.Statement{s.Init}})
			}
			if s.Condition != nil {
				a.analyzeExpression(s.Condition)
			}
			if s.Increment != nil {
				a.analyzeBlock(&ast.BlockStatement{Statements: []ast.Statement{s.Increment}})
			}
			a.analyzeBlock(s.Body)
		case *ast.ForRangeStatement:
			if s.IsDecl {
				if ident, ok := s.Key.(*ast.Identifier); ok {
					a.currentScope.Define(ident.Value, WordType)
				}
				if s.Value != nil {
					if ident, ok := s.Value.(*ast.Identifier); ok {
					    valTyp := UnknownType
					    rangeTyp := a.analyzeExpression(s.RangeValue)
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
			a.analyzeExpression(s.RangeValue)
			a.analyzeBlock(s.Body)
		case *ast.ReturnStatement:
			for _, rv := range s.ReturnValues {
				a.analyzeExpression(rv)
			}
		case *ast.ExpressionStatement:
			a.analyzeExpression(s.Expression)
		}
	}
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
    if _, ok := a.globalScope.Resolve(instName); ok { return } // Already instantiated
    tmpl, ok := a.genericTemplates[qname]
    if !ok { return }
    
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
    if _, ok := a.globalScope.Resolve(instName); ok { return } // Already instantiated
    tmpl, ok := a.genericTemplates[qname]
    if !ok { return }
    
    subTokens := a.substituteGenericTokens(argTyps, tmpl)
	p := parser.New(subTokens)
	stmt := p.ParseStatementForGeneric()
	
	if fs, ok := stmt.(*ast.FuncStatement); ok {
	    fs.Name.Value = strings.TrimPrefix(instName, a.currentPackage+".")
	    // If it was a method, clear receiver and make it regular func name
	    fs.Receiver = nil
	    fs.TypeParameters = nil
	    a.program.Statements = append(a.program.Statements, fs)
	    
	    var retTypes []ast.Expression
        for _, r := range fs.ReturnTypes {
            retTypes = append(retTypes, r)
        }
        ft := &ast.FuncType{Parameters: fs.Parameters, ReturnTypes: retTypes}
		a.globalScope.Define(instName, ft)
	}
}

func (a *Analyzer) analyzeExpression(expr ast.Expression) ast.Expression {
    if expr == nil { return UnknownType }
    
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
		} else if sym, ok := a.globalScope.Resolve(qname); ok {
		    typ = sym.Type
		} else if sym, ok := a.globalScope.Resolve("prelude." + e.Value); ok {
		    typ = sym.Type
		} else {
		    a.errors = append(a.errors, fmt.Sprintf("undefined identifier: %s", e.Value))
		}
	case *ast.InfixExpression:
		t1 := a.analyzeExpression(e.Left)
		t2 := a.analyzeExpression(e.Right)
		if e.Operator == "==" || e.Operator == "!=" || e.Operator == "<" || e.Operator == ">" || e.Operator == "<=" || e.Operator == ">=" {
		    typ = WordType
		} else {
		    if t1 != UnknownType { typ = t1 } else { typ = t2 }
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
		leftTyp := a.analyzeExpression(e.Left)
		for _, idx := range e.Indices {
			a.analyzeExpression(idx)
		}
		
		// Could be array indexing or generic instantiation!
		// Check if leftTyp is a generic template
		qname := ""
		if id, ok := e.Left.(*ast.Identifier); ok {
		    if _, ok := a.genericTemplates[a.currentPackage+"."+id.Value]; ok {
		        qname = a.currentPackage+"."+id.Value
		    } else if _, ok := a.genericTemplates["prelude."+id.Value]; ok {
		        qname = "prelude."+id.Value
		    }
		}
		
		if qname != "" {
		    // It's a generic instantiation!
		    instName := qname
		    for _, idx := range e.Indices {
		        instName += "_" + exprToString(idx) // Simplified
		    }
		    
		    a.instantiateGenericType(instName, qname, e.Indices)
		    typ = builtinType(instName)
		} else if arrTyp, ok := leftTyp.(*ast.ArrayType); ok {
		    typ = arrTyp.Elt
		}
		
	case *ast.SelectorExpression:
	    // If it's a package reference
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if sym, ok := a.globalScope.Resolve(qname); ok {
				typ = sym.Type
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
		    } else {
		        // Could be a generic method like slice_byte_Chop!
		        // We need to instantiate it.
		        if strings.HasPrefix(baseTypStr, "prelude.slice_") {
		            eltTypeStr := strings.TrimPrefix(baseTypStr, "prelude.slice_")
		            qname := "prelude.slice_" + e.Right.Value
		            instName := baseTypStr + "_" + e.Right.Value
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
