package semantic

import (
	"github.com/strickyak/minigolf/ast"
)

func (a *Analyzer) foldExpression(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.Identifier:
		if e.IsResolved {
			qname := e.Package + "." + e.ShortName
			if cExpr, ok := a.constExprs[qname]; ok {
				return a.foldExpression(cExpr)
			}
		}
	case *ast.InfixExpression:
		e.Left = a.foldExpression(e.Left)
		e.Right = a.foldExpression(e.Right)

		leftInt, leftOk := e.Left.(*ast.IntegerLiteral)
		rightInt, rightOk := e.Right.(*ast.IntegerLiteral)

		if leftOk && rightOk {
			var result int64
			switch e.Operator {
			case "+":
				result = leftInt.Value + rightInt.Value
			case "-":
				result = leftInt.Value - rightInt.Value
			case "*":
				result = leftInt.Value * rightInt.Value
			case "/":
				if rightInt.Value != 0 {
					result = leftInt.Value / rightInt.Value
				} else {
					return expr
				}
			case "%":
				if rightInt.Value != 0 {
					result = leftInt.Value % rightInt.Value
				} else {
					return expr
				}
			case "==":
				val := int64(0)
				if leftInt.Value == rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case "!=":
				val := int64(0)
				if leftInt.Value != rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case "<":
				val := int64(0)
				if leftInt.Value < rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case "<=":
				val := int64(0)
				if leftInt.Value <= rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case ">":
				val := int64(0)
				if leftInt.Value > rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case ">=":
				val := int64(0)
				if leftInt.Value >= rightInt.Value {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			case "&":
				result = leftInt.Value & rightInt.Value
			case "|":
				result = leftInt.Value | rightInt.Value
			case "^":
				result = leftInt.Value ^ rightInt.Value
			case "<<":
				result = leftInt.Value << rightInt.Value
			case ">>":
				result = leftInt.Value >> rightInt.Value
			default:
				return expr
			}
			return &ast.IntegerLiteral{Token: e.Token, Value: result}
		}

	case *ast.PrefixExpression:
		e.Right = a.foldExpression(e.Right)

		if rightInt, ok := e.Right.(*ast.IntegerLiteral); ok {
			switch e.Operator {
			case "-":
				return &ast.IntegerLiteral{Token: e.Token, Value: -rightInt.Value}
			case "+":
				return &ast.IntegerLiteral{Token: e.Token, Value: rightInt.Value}
			case "^":
				return &ast.IntegerLiteral{Token: e.Token, Value: ^rightInt.Value}
			case "!":
				val := int64(0)
				if rightInt.Value == 0 {
					val = 1
				}
				return &ast.IntegerLiteral{Token: e.Token, Value: val}
			}
		}

	case *ast.CallExpression:
		e.Function = a.foldExpression(e.Function)
		for i, arg := range e.Arguments {
			e.Arguments[i] = a.foldExpression(arg)
		}

	case *ast.IndexExpression:
		e.Left = a.foldExpression(e.Left)
		for i, idx := range e.Indices {
			e.Indices[i] = a.foldExpression(idx)
		}

	case *ast.SelectorExpression:
		e.Left = a.foldExpression(e.Left)
	}

	return expr
}
