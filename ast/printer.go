package ast

import (
	"fmt"
	"reflect"
	"strings"
)

// Print returns a human-readable ASCII string representation of an AST node or tree.
// It uses braces {} around nested structures and brackets [] around lists.
func Print(node any) string {
	defer func() {
		r := recover()
		if r != nil {
			panic(fmt.Errorf("PANIC in printing %#v: %v", node, r))
		}
	}()

	if node == nil {
		return "<nil>"
	}

	if reflect.ValueOf(node).IsNil() {
		return "<.nil.>"
	}

	switch n := node.(type) {
	case *Program:
		var stmts []string
		for _, s := range n.Statements {
			stmts = append(stmts, Print(s))
		}
		return fmt.Sprintf("Program{Statements: [%s]}\n", strings.Join(stmts, ", "))

	case *PackageStatement:
		return fmt.Sprintf("PackageStatement{Name: %s}\n", Print(n.Name))

	case *ImportStatement:
		return fmt.Sprintf("ImportStatement{Path: %s}\n", Print(n.Path))

	case *ConstStatement:
		return fmt.Sprintf("ConstStatement{Name: %s, Value: %s}\n", Print(n.Name), Print(n.Value))

	case *TypeStatement:
		var params []string
		for _, p := range n.TypeParameters {
			params = append(params, Print(p))
		}
		return fmt.Sprintf("TypeStatement{Name: %s, TypeParameters: [%s], BaseType: %s}\n",
			Print(n.Name), strings.Join(params, ", "), Print(n.BaseType))

	case *VarStatement:
		return fmt.Sprintf("VarStatement{Name: %s, ValueType: %s, Value: %s}\n",
			Print(n.Name), Print(n.ValueType), Print(n.Value))

	case *FuncStatement:
		var typeParams []string
		for _, p := range n.TypeParameters {
			typeParams = append(typeParams, Print(p))
		}
		var params []string
		for _, p := range n.Parameters {
			params = append(params, Print(p))
		}
		var retTypes []string
		for _, r := range n.ReturnTypes {
			retTypes = append(retTypes, Print(r))
		}
		return fmt.Sprintf("FuncStatement{Name: %s, TypeParameters: [%s], Receiver: %s, Parameters: [%s], ReturnTypes: [%s], Body: %s}\n",
			Print(n.Name), strings.Join(typeParams, ", "), Print(n.Receiver), strings.Join(params, ", "), strings.Join(retTypes, ", "), Print(n.Body))

	case *Parameter:
		if n == nil {
			return "Paramter(nil)"
		}
		return fmt.Sprintf("Parameter{Name: %s, Type: %s}", Print(n.Name), Print(n.Type))

	case *BlockStatement:
		var stmts []string
		for _, s := range n.Statements {
			stmts = append(stmts, Print(s))
		}
		return fmt.Sprintf("BlockStatement{Statements: [%s]}", strings.Join(stmts, ";\n"))

	case *AssignStatement:
		var names []string
		for _, name := range n.Names {
			names = append(names, Print(name))
		}
		var values []string
		for _, val := range n.Values {
			values = append(values, Print(val))
		}
		return fmt.Sprintf("AssignStatement{Token: %s, Names: [%s], Values: [%s]}",
			n.Token.Literal, strings.Join(names, ", "), strings.Join(values, ", "))

	case *IfStatement:
		return fmt.Sprintf("IfStatement{\n ? Condition: %s,\n ? Consequence: %s,\n ? Alternative: %s}",
			Print(n.Condition), Print(n.Consequence), Print(n.Alternative))

	case *ForStatement:
		return fmt.Sprintf("ForStatement{Condition: %s, Body: %s}",
			Print(n.Condition), Print(n.Body))

	case *For3Statement:
		return fmt.Sprintf("For3Statement{\n - Init: %s,\n - Condition: %s,\n - Increment: %s,\n - Body: %s}",
			Print(n.Init), Print(n.Condition), Print(n.Increment), Print(n.Body))

	case *IncDecStatement:
		return fmt.Sprintf("IncDecStatement{Token: %s, Name: %s}", n.Token.Literal, Print(n.Name))

	case *ForRangeStatement:
		return fmt.Sprintf("ForRangeStatement{Key: %s, IsDecl: %v, RangeValue: %s, Body: %s}",
			Print(n.Key), n.IsDecl, Print(n.RangeValue), Print(n.Body))

	case *DeferStatement:
		return fmt.Sprintf("DeferStatement{Call: %s}", Print(n.Call))

	case *ReturnStatement:
		var retVals []string
		for _, r := range n.ReturnValues {
			retVals = append(retVals, Print(r))
		}
		return fmt.Sprintf("ReturnStatement{ReturnValues: [%s]}", strings.Join(retVals, ", "))

	case *ExpressionStatement:
		return fmt.Sprintf("ExpressionStatement{Expression: %s}", Print(n.Expression))

	case *Identifier:
		return fmt.Sprintf("Identifier{Value: %s}", n.Value)

	case *IntegerLiteral:
		return fmt.Sprintf("IntegerLiteral{Value: %d}", n.Value)

	case *StringLiteral:
		return fmt.Sprintf("StringLiteral{Value: %q}", n.Value)

	case *PrefixExpression:
		return fmt.Sprintf("PrefixExpression{Operator: %s, Right: %s}", n.Operator, Print(n.Right))

	case *InfixExpression:
		return fmt.Sprintf("InfixExpression{Left: %s, Operator: %s, Right: %s}", Print(n.Left), n.Operator, Print(n.Right))

	case *CallExpression:
		var args []string
		for _, a := range n.Arguments {
			args = append(args, Print(a))
		}
		return fmt.Sprintf("CallExpression{Function: %s, Arguments: [%s]}", Print(n.Function), strings.Join(args, ", "))

	case *ArrayType:
		return fmt.Sprintf("ArrayType{Length: %s, Elt: %s}", Print(n.Length), Print(n.Elt))

	case *StructType:
		var fields []string
		for _, f := range n.Fields {
			fields = append(fields, Print(f))
		}
		return fmt.Sprintf("StructType{Fields: [%s]}", strings.Join(fields, ", "))

	case *Field:
		return fmt.Sprintf("Field{Name: %s, Type: %s}", Print(n.Name), Print(n.Type))

	case *SelectorExpression:
		return fmt.Sprintf("SelectorExpression{Left: %s, Right: %s}", Print(n.Left), Print(n.Right))

	case *RangeExpression:
		return fmt.Sprintf("RangeExpression{Value: %s}", Print(n.Value))

	case *IndexExpression:
		var indices []string
		for _, idx := range n.Indices {
			indices = append(indices, Print(idx))
		}
		return fmt.Sprintf("IndexExpression{Left: %s, Indices: [%s]}", Print(n.Left), strings.Join(indices, ", "))

	case *PointerType:
		return fmt.Sprintf("PointerType{Elt: %s}", Print(n.Elt))
	}

	return fmt.Sprintf("UnknownNode{%T}", node)
}
