package semantic

import (
	"fmt"
	"os"
	"strconv"

	"github.com/strickyak/minigolf/ast"
)

type Resolver struct {
	packages    map[string]bool
	globals     map[string]bool // fullyQualifiedName -> true
	currentPkg  string
	localScopes []map[string]bool
	errors      []string
	defines     map[string]string
}

func NewResolver(defines map[string]string) *Resolver {
	if defines == nil {
		defines = make(map[string]string)
	}
	return &Resolver{
		packages:    make(map[string]bool),
		globals:     make(map[string]bool),
		localScopes: make([]map[string]bool, 0),
		defines:     defines,
	}
}

func (r *Resolver) ResolveGenericInst(stmt ast.Statement, defPkg string) ast.Statement {
	r.currentPkg = defPkg
	return r.resolveStatement(stmt)
}

func (r *Resolver) ResolveGenericInstExpr(expr ast.Expression, defPkg string) ast.Expression {
	r.currentPkg = defPkg
	return r.resolveExpression(expr)
}

func (r *Resolver) pushScope() {
	r.localScopes = append(r.localScopes, make(map[string]bool))
}

func (r *Resolver) popScope() {
	r.localScopes = r.localScopes[:len(r.localScopes)-1]
}

func (r *Resolver) defineLocal(name string) {
	if len(r.localScopes) > 0 {
		r.localScopes[len(r.localScopes)-1][name] = true
	}
}

func (r *Resolver) isLocal(name string) bool {
	for i := len(r.localScopes) - 1; i >= 0; i-- {
		if r.localScopes[i][name] {
			return true
		}
	}
	return false
}

func (r *Resolver) Resolve(program *ast.Program) {
	// Pass 1: Collect globals and packages
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			r.currentPkg = s.Name.Value
			r.packages[r.currentPkg] = true
		case *ast.FuncStatement:
			r.globals[r.currentPkg+"."+s.Name.Value] = true
		case *ast.TypeStatement:
			r.globals[r.currentPkg+"."+s.Name.Value] = true
		case *ast.VarStatement:
			r.globals[r.currentPkg+"."+s.Name.Value] = true
		case *ast.ConstStatement:
			r.globals[r.currentPkg+"."+s.Name.Value] = true
		}
	}

	// Implicit builtins
	for _, b := range []string{"word", "byte", "int", "uint", "string", "any", "bool", "panicked"} {
		r.globals["builtin."+b] = true
	}
	r.globals["builtin.true"] = true
	r.globals["builtin.false"] = true
	r.globals["builtin.nil"] = true

	// Pass 2: Rewrite AST
	r.currentPkg = ""
	for i, stmt := range program.Statements {
		if ps, ok := stmt.(*ast.PackageStatement); ok {
			r.currentPkg = ps.Name.Value
		}
		program.Statements[i] = r.resolveStatement(stmt)
	}
}

func (r *Resolver) resolveStatement(stmt ast.Statement) ast.Statement {
	if stmt == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.PackageStatement:
		return s
	case *ast.ImportStatement:
		return s
	case *ast.ConstStatement:
		// TODO: I think we have lost the ability to set string constants.
		// This is because we are not smart enough to realize that
		// this is an integer:
		// const HEAP_SIZE = 10000 + sizeof[*word]()*sizeof[*word]()*sizeof[*word]()*sizeof[*word]()*sizeof[*word]()*512
		// TODO: A ConstExprEval() that can tell us the type and value of constant expressions like that.
		qname := r.currentPkg + "." + s.Name.Value
		if override, ok := r.defines[qname]; ok {
			val, err := strconv.ParseInt(override, 10, 64)
			if err != nil {
				// fatal error
				fmt.Fprintf(os.Stderr, "Error: Invalid override value for %s. Expected integer, got '%s'.\n", qname, override)
				os.Exit(1)
			}
			s.Value = &ast.IntegerLiteral{Value: val, Token: s.Token}
		}
		s.Value = r.resolveExpression(s.Value)
		return s
	case *ast.TypeStatement:
		s.BaseType = r.resolveExpression(s.BaseType)
		return s
	case *ast.VarStatement:
		s.ValueType = r.resolveExpression(s.ValueType)
		s.Value = r.resolveExpression(s.Value)
		return s
	case *ast.FuncStatement:
		r.pushScope()
		// Define parameters
		for _, param := range s.Parameters {
			r.defineLocal(param.Name.Value)
			param.Type = r.resolveExpression(param.Type)
		}
		if s.Receiver != nil {
			r.defineLocal(s.Receiver.Name.Value)
			s.Receiver.Type = r.resolveExpression(s.Receiver.Type)
		}
		for _, rp := range s.ReturnParameters {
			rp.Type = r.resolveExpression(rp.Type)
		}
		if s.Body != nil {
			s.Body = r.resolveStatement(s.Body).(*ast.BlockStatement)
		}
		r.popScope()
		return s
	case *ast.BlockStatement:
		r.pushScope()
		for i, st := range s.Statements {
			s.Statements[i] = r.resolveStatement(st)
		}
		r.popScope()
		return s
	case *ast.AssignStatement:
		for i, lhs := range s.Names {
			s.Names[i] = r.resolveExpression(lhs)
		}
		for i, rhs := range s.Values {
			s.Values[i] = r.resolveExpression(rhs)
		}
		if s.Token.Literal == ":=" {
			for _, lhs := range s.Names {
				if id, ok := lhs.(*ast.Identifier); ok {
					r.defineLocal(id.Value)
				}
			}
		}
		return s
	case *ast.IfStatement:
		s.Condition = r.resolveExpression(s.Condition)
		s.Consequence = r.resolveStatement(s.Consequence).(*ast.BlockStatement)
		if s.Alternative != nil {
			s.Alternative = r.resolveStatement(s.Alternative).(*ast.BlockStatement)
		}
		return s
	case *ast.ForStatement:
		s.Condition = r.resolveExpression(s.Condition)
		s.Body = r.resolveStatement(s.Body).(*ast.BlockStatement)
		return s
	case *ast.For3Statement:
		r.pushScope()
		if s.Init != nil {
			s.Init = r.resolveStatement(s.Init)
		}
		if s.Condition != nil {
			s.Condition = r.resolveExpression(s.Condition)
		}
		if s.Increment != nil {
			s.Increment = r.resolveStatement(s.Increment)
		}
		s.Body = r.resolveStatement(s.Body).(*ast.BlockStatement)
		r.popScope()
		return s
	case *ast.IncDecStatement:
		s.Name = r.resolveExpression(s.Name)
		return s
	case *ast.OpAssignStatement:
		s.Name = r.resolveExpression(s.Name)
		s.Value = r.resolveExpression(s.Value)
		return s
	case *ast.ForRangeStatement:
		r.pushScope()
		if s.Key != nil {
			s.Key = r.resolveExpression(s.Key)
			if id, ok := s.Key.(*ast.Identifier); ok {
				r.defineLocal(id.Value)
			}
		}
		if s.Value != nil {
			s.Value = r.resolveExpression(s.Value)
			if id, ok := s.Value.(*ast.Identifier); ok {
				r.defineLocal(id.Value)
			}
		}
		s.RangeValue = r.resolveExpression(s.RangeValue)
		s.Body = r.resolveStatement(s.Body).(*ast.BlockStatement)
		r.popScope()
		return s
	case *ast.ReturnStatement:
		for i, expr := range s.ReturnValues {
			s.ReturnValues[i] = r.resolveExpression(expr)
		}
		return s
	case *ast.BreakStatement:
		return s
	case *ast.ContinueStatement:
		return s
	case *ast.ExpressionStatement:
		s.Expression = r.resolveExpression(s.Expression)
		return s
	case *ast.DeferStatement:
		if s.Block != nil {
			s.Block = r.resolveStatement(s.Block).(*ast.BlockStatement)
		} else {
			s.Call = r.resolveExpression(s.Call)
		}
		return s
	}
	return stmt
}

func (r *Resolver) resolveExpression(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if !r.isLocal(e.Value) {
			if r.globals[r.currentPkg+"."+e.Value] {
				e.Package = r.currentPkg
				e.ShortName = e.Value
				e.IsResolved = true
			} else if r.globals["prelude."+e.Value] {
				e.Package = "prelude"
				e.ShortName = e.Value
				e.IsResolved = true
			} else if r.globals["builtin."+e.Value] {
				e.Package = "builtin"
				e.ShortName = e.Value
				e.IsResolved = true
			}
		}
		return e
	case *ast.SelectorExpression:
		e.Left = r.resolveExpression(e.Left)
		if leftId, ok := e.Left.(*ast.Identifier); ok {
			if r.packages[leftId.Value] && !r.isLocal(leftId.Value) {
				// Collapse Selector into Identifier!
				return &ast.Identifier{
					BaseExpression: e.BaseExpression,
					Token:          e.Token, // Or keep the right token?
					Value:          e.Right.Value,
					Package:        leftId.Value,
					ShortName:      e.Right.Value,
					IsResolved:     true,
				}
			}
		}
		e.Right = r.resolveExpression(e.Right).(*ast.Identifier)
		return e
	case *ast.IntegerLiteral:
		return e
	case *ast.StringLiteral:
		return e
	case *ast.PrefixExpression:
		e.Right = r.resolveExpression(e.Right)
		return e
	case *ast.InfixExpression:
		e.Left = r.resolveExpression(e.Left)
		e.Right = r.resolveExpression(e.Right)
		return e
	case *ast.CallExpression:
		e.Function = r.resolveExpression(e.Function)
		for i, arg := range e.Arguments {
			e.Arguments[i] = r.resolveExpression(arg)
		}
		return e
	case *ast.FuncType:
		for _, param := range e.Parameters {
			param.Type = r.resolveExpression(param.Type)
		}
		for _, rp := range e.ReturnParameters {
			rp.Type = r.resolveExpression(rp.Type)
		}
		return e
	case *ast.ArrayType:
		if e.Length != nil {
			e.Length = r.resolveExpression(e.Length)
		}
		e.Elt = r.resolveExpression(e.Elt)
		return e
	case *ast.StructType:
		for _, f := range e.Fields {
			f.Type = r.resolveExpression(f.Type)
		}
		return e
	case *ast.RangeExpression:
		e.Value = r.resolveExpression(e.Value)
		return e
	case *ast.IndexExpression:
		e.Left = r.resolveExpression(e.Left)
		for i, idx := range e.Indices {
			e.Indices[i] = r.resolveExpression(idx)
		}
		return e
	case *ast.PointerType:
		e.Elt = r.resolveExpression(e.Elt)
		return e
	case *ast.CompositeLit:
		e.Type = r.resolveExpression(e.Type)
		for i, el := range e.Elements {
			e.Elements[i] = r.resolveExpression(el)
		}
		return e
	case *ast.KeyValueExpr:
		// Key is a field name, not a variable, so don't resolve it!
		// But wait! If Key is not resolved, that's fine.
		e.Value = r.resolveExpression(e.Value)
		return e
	}
	return expr
}
