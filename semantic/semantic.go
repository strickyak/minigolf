package semantic

import (
	"fmt"
	"minigo/ast"
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
	errors []string
	globalScope *Scope
	currentScope *Scope
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
		errors: []string{},
		globalScope: global,
		currentScope: global,
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
			if s.Name.Value == "main" {
				a.hasMainFunc = true
			}
			a.globalScope.Define(s.Name.Value, "func")
		case *ast.VarStatement:
			typ := "word" // default
			if s.ValueType != nil {
				typ = s.ValueType.Value
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

	for _, p := range s.Parameters {
		a.currentScope.Define(p.Name.Value, p.Type.Value)
	}

	a.analyzeBlock(s.Body)
}

func (a *Analyzer) analyzeBlock(b *ast.BlockStatement) {
	for _, stmt := range b.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			typ := "word"
			if s.ValueType != nil {
				typ = s.ValueType.Value
			}
			a.currentScope.Define(s.Name.Value, typ)
		case *ast.AssignStatement:
			if s.Token.Literal == ":=" {
				for _, name := range s.Names {
					a.currentScope.Define(name.Value, "word") // default
				}
			} else {
				for _, name := range s.Names {
					if _, ok := a.currentScope.Resolve(name.Value); !ok {
						a.errors = append(a.errors, fmt.Sprintf("undefined variable: %s", name.Value))
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
			if s.ReturnValue != nil {
				a.analyzeExpression(s.ReturnValue)
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
	}
}
