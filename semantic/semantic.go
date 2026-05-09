package semantic

import (
	"fmt"
	"github.com/strickyak/minigolf/ast"
	"strings"
)

type Symbol struct {
	Name string
	Type string // "byte", "word", or "func"
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

func (s *Scope) Define(name, typ string) {
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

type Analyzer struct {
	errors         []string
	globalScope    *Scope
	currentScope   *Scope
	hasMainPackage bool
	hasMainFunc    bool
	currentPackage string
}

func New() *Analyzer {
	global := NewScope(nil)
	// Built-ins
	global.Define("print", "func")
	global.Define("println", "func")
	global.Define("sizeof", "func")
	global.Define("byte", "type")
	global.Define("word", "type")
	global.Define("int", "type")
	global.Define("uint", "type")
	global.Define("any", "type")

	return &Analyzer{
		errors:       []string{},
		globalScope:  global,
		currentScope: global,
	}
}

func (a *Analyzer) exprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		qname := a.currentPackage + "." + e.Value
		if _, ok := a.globalScope.symbols[qname]; ok {
			return qname
		}
		return e.Value
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			return pkgIdent.Value + "." + e.Right.Value
		}
		return a.exprToString(e.Left) + "." + e.Right.Value
	case *ast.ArrayType:
		lenStr := "0"
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = fmt.Sprintf("%d", il.Value)
		}
		return fmt.Sprintf("[%s]%s", lenStr, a.exprToString(e.Elt))
	case *ast.StructType:
		res := "struct{"
		for _, f := range e.Fields {
			res += a.exprToString(f.Type) + ";"
		}
		res += "}"
		return res
	case *ast.PointerType:
		return "*" + a.exprToString(e.Elt)
	default:
		return expr.TokenLiteral()
	}
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func (a *Analyzer) Analyze(program *ast.Program) {
	// First pass: define global symbols
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

			funcName := s.Name.Value
			if s.Receiver != nil {
				recvTyp := a.exprToString(s.Receiver.Type)
				baseType := recvTyp
				baseType = strings.TrimPrefix(baseType, "*")
				funcName = baseType + "_" + funcName
			} else {
				if a.currentPackage != "main" || funcName != "main" {
					funcName = a.currentPackage + "." + funcName
				}
			}
			a.globalScope.Define(funcName, "func")
		case *ast.VarStatement:
			typ := "word" // default
			if s.ValueType != nil {
				typ = a.exprToString(s.ValueType)
			}
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, typ)
		case *ast.ConstStatement:
			a.globalScope.Define(a.currentPackage+"."+s.Name.Value, "word") // simplification
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
	for _, stmt := range program.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			a.currentPackage = ps.Name.Value
		}
		if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
			a.analyzeFunc(funcStmt)
		}
	}
}

func (a *Analyzer) analyzeFunc(s *ast.FuncStatement) {
	a.currentScope = NewScope(a.currentScope)
	defer func() { a.currentScope = a.currentScope.parent }()

	if s.Receiver != nil {
		a.currentScope.Define(s.Receiver.Name.Value, a.exprToString(s.Receiver.Type))
	}

	for _, p := range s.Parameters {
		a.currentScope.Define(p.Name.Value, a.exprToString(p.Type))
	}

	a.analyzeBlock(s.Body)
}

func (a *Analyzer) analyzeBlock(b *ast.BlockStatement) {
	for _, stmt := range b.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := "word"
			if s.ValueType != nil {
				typ = a.exprToString(s.ValueType)
			}
			a.currentScope.Define(s.Name.Value, typ)
		case *ast.AssignStatement:
			if s.Token.Literal == ":=" {
				for _, nameExpr := range s.Names {
					if name, ok := nameExpr.(*ast.Identifier); ok {
						a.currentScope.Define(name.Value, "word") // default
					}
				}
			} else {
				for _, nameExpr := range s.Names {
					if name, ok := nameExpr.(*ast.Identifier); ok {
						if _, ok := a.currentScope.Resolve(name.Value); !ok {
							a.errors = append(a.errors, fmt.Sprintf("undefined variable: %s", name.Value))
						}
					} else if idx, ok := nameExpr.(*ast.IndexExpression); ok {
						a.analyzeExpression(idx) // arrays assignment a[i] = v
					}
				}
			}
			// analyze right side
			for _, expr := range s.Values {
				a.analyzeExpression(expr)
			}
		case *ast.IfStatement:
			a.analyzeExpression(s.Condition)
			a.analyzeBlock(s.Consequence)
			if s.Alternative != nil {
				a.analyzeBlock(s.Alternative)
			}
		case *ast.ForStatement:
			a.analyzeExpression(s.Condition)
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

func (a *Analyzer) analyzeExpression(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.Identifier:
		qname := a.currentPackage + "." + e.Value
		if _, ok := a.currentScope.Resolve(qname); ok {
			return
		}
		if _, ok := a.currentScope.Resolve(e.Value); !ok {
			a.errors = append(a.errors, fmt.Sprintf("undefined identifier: %s", e.Value))
		}
	case *ast.InfixExpression:
		a.analyzeExpression(e.Left)
		a.analyzeExpression(e.Right)
	case *ast.PrefixExpression:
		a.analyzeExpression(e.Right)
	case *ast.CallExpression:
		a.analyzeExpression(e.Function)
		for _, arg := range e.Arguments {
			a.analyzeExpression(arg)
		}
	case *ast.IndexExpression:
		a.analyzeExpression(e.Left)
		for _, idx := range e.Indices {
			a.analyzeExpression(idx)
		}
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if _, ok := a.currentScope.Resolve(qname); ok {
				return
			}
		}
		a.analyzeExpression(e.Left)
	}
}
