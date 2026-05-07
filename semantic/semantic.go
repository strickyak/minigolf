package semantic

import (
	"fmt"
	"minigo/ast"
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
}

func New() *Analyzer {
	global := NewScope(nil)
	// Built-ins
	global.Define("print", "func")
	global.Define("println", "func")
	global.Define("byte", "type")
	global.Define("word", "type")

	return &Analyzer{
		errors:       []string{},
		globalScope:  global,
		currentScope: global,
	}
}

func exprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value
	case *ast.ArrayType:
		lenStr := "0"
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = fmt.Sprintf("%d", il.Value)
		}
		return fmt.Sprintf("[%s]%s", lenStr, exprToString(e.Elt))
	case *ast.StructType:
		res := "struct{"
		for _, f := range e.Fields {
			res += exprToString(f.Type) + ";"
		}
		res += "}"
		return res
	case *ast.PointerType:
		return "*" + exprToString(e.Elt)
	default:
		return expr.TokenLiteral()
	}
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func (a *Analyzer) Analyze(program *ast.Program) {
	// First pass: define global symbols
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			if s.Name.Value == "main" {
				a.hasMainPackage = true
			}
		case *ast.FuncStatement:
			if s.Name.Value == "main" && s.Receiver == nil {
				a.hasMainFunc = true
			}

			funcName := s.Name.Value
			if s.Receiver != nil {
				recvTyp := exprToString(s.Receiver.Type)
				baseType := recvTyp
				baseType = strings.TrimPrefix(baseType, "*")
				funcName = baseType + "_" + funcName
			}
			a.globalScope.Define(funcName, "func")
		case *ast.VarStatement:
			typ := "word" // default
			if s.ValueType != nil {
				typ = exprToString(s.ValueType)
			}
			a.globalScope.Define(s.Name.Value, typ)
		case *ast.ConstStatement:
			a.globalScope.Define(s.Name.Value, "word") // simplification
		}
	}

	if !a.hasMainPackage {
		a.errors = append(a.errors, "missing 'package main'")
	}
	if a.hasMainPackage && !a.hasMainFunc {
		a.errors = append(a.errors, "missing 'main' function in 'package main'")
	}

	// Second pass: check function bodies
	for _, stmt := range program.Statements {
		if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
			a.analyzeFunc(funcStmt)
		}
	}
}

func (a *Analyzer) analyzeFunc(s *ast.FuncStatement) {
	a.currentScope = NewScope(a.currentScope)
	defer func() { a.currentScope = a.currentScope.parent }()

	if s.Receiver != nil {
		a.currentScope.Define(s.Receiver.Name.Value, exprToString(s.Receiver.Type))
	}

	for _, p := range s.Parameters {
		a.currentScope.Define(p.Name.Value, exprToString(p.Type))
	}

	a.analyzeBlock(s.Body)
}

func (a *Analyzer) analyzeBlock(b *ast.BlockStatement) {
	for _, stmt := range b.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := "word"
			if s.ValueType != nil {
				typ = exprToString(s.ValueType)
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
		a.analyzeExpression(e.Index)
	case *ast.SelectorExpression:
		a.analyzeExpression(e.Left)
	}
}
