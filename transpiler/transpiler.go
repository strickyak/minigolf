package transpiler

import (
	"bytes"
	"fmt"
	"minigo/ast"
	"strconv"
	"strings"
)

// Transpiler walks the AST and emits C99 code
type Transpiler struct {
	pkgName string
	buf     bytes.Buffer
	locals  []map[string]bool
}

func New() *Transpiler {
	return &Transpiler{}
}

func (t *Transpiler) pushScope() {
	t.locals = append(t.locals, make(map[string]bool))
}

func (t *Transpiler) popScope() {
	t.locals = t.locals[:len(t.locals)-1]
}

func (t *Transpiler) addLocal(name string) {
	if len(t.locals) > 0 {
		t.locals[len(t.locals)-1][name] = true
	}
}

func (t *Transpiler) isLocal(name string) bool {
	for i := len(t.locals) - 1; i >= 0; i-- {
		if t.locals[i][name] {
			return true
		}
	}
	return false
}

func (t *Transpiler) Transpile(program *ast.Program) string {
	// First pass: find package name
	t.pkgName = "main" // default
	for _, stmt := range program.Statements {
		if pkg, ok := stmt.(*ast.PackageStatement); ok {
			t.pkgName = pkg.Name.Value
			break
		}
	}

	t.buf.WriteString("#include <stdio.h>\n")
	t.buf.WriteString("#include <stdint.h>\n\n")

	t.buf.WriteString("typedef uint8_t byte;\n")
	t.buf.WriteString("typedef uint16_t word;\n\n")

	// Second pass: Forward declarations for types and functions
	t.buf.WriteString("// Forward declarations\n")
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.TypeStatement:
			base := t.mapType(s.BaseType.Value)
			t.buf.WriteString(fmt.Sprintf("typedef %s t_%s_%s;\n", base, t.pkgName, s.Name.Value))
		case *ast.FuncStatement:
			t.emitFuncSignature(s, true)
		}
	}

	t.buf.WriteString("\n// Global variables and constants\n")
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			valType := "word"
			if s.ValueType != nil {
				valType = t.mapType(s.ValueType.Value)
			}
			t.buf.WriteString(fmt.Sprintf("%s v_%s_%s", valType, t.pkgName, s.Name.Value))
			if s.Value != nil {
				t.buf.WriteString(fmt.Sprintf(" = %s", t.emitExprStr(s.Value)))
			}
			t.buf.WriteString(";\n")
		case *ast.ConstStatement:
			t.buf.WriteString(fmt.Sprintf("#define v_%s_%s %s\n", t.pkgName, s.Name.Value, t.emitExprStr(s.Value)))
		}
	}
	t.buf.WriteString("\n")

	// Third pass: Implementations
	for _, stmt := range program.Statements {
		if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
			t.emitStatement(funcStmt)
		}
	}

	// Finally: C main function
	t.buf.WriteString("\nint main() {\n")
	t.buf.WriteString(fmt.Sprintf("\tf_%s_main();\n", t.pkgName))
	t.buf.WriteString("\treturn 0;\n")
	t.buf.WriteString("}\n")

	return t.buf.String()
}

func (t *Transpiler) mapType(name string) string {
	if name == "byte" || name == "word" {
		return name
	}
	return fmt.Sprintf("t_%s_%s", t.pkgName, name)
}

func (t *Transpiler) emitFuncSignature(s *ast.FuncStatement, isForward bool) {
	retType := "void"
	if s.ReturnType != nil {
		retType = t.mapType(s.ReturnType.Value)
	}

	var params []string
	for _, p := range s.Parameters {
		if !isForward {
			t.addLocal(p.Name.Value)
		}
		params = append(params, fmt.Sprintf("%s v_%s", t.mapType(p.Type.Value), p.Name.Value))
	}

	t.buf.WriteString(fmt.Sprintf("%s f_%s_%s(%s)", retType, t.pkgName, s.Name.Value, strings.Join(params, ", ")))
	if isForward {
		t.buf.WriteString(";\n")
	} else {
		t.buf.WriteString(" ")
	}
}

func (t *Transpiler) emitStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.PackageStatement, *ast.ImportStatement, *ast.ConstStatement, *ast.TypeStatement:
		// Handled in earlier passes or ignored
	case *ast.VarStatement:
		valType := "word"
		if s.ValueType != nil {
			valType = t.mapType(s.ValueType.Value)
		}
		t.addLocal(s.Name.Value)
		t.buf.WriteString(fmt.Sprintf("%s v_%s", valType, s.Name.Value))
		if s.Value != nil {
			t.buf.WriteString(fmt.Sprintf(" = %s", t.emitExprStr(s.Value)))
		}
		t.buf.WriteString(";\n")
	case *ast.FuncStatement:
		t.pushScope()
		t.emitFuncSignature(s, false)
		t.emitStatement(s.Body)
		t.popScope()
		t.buf.WriteString("\n")
	case *ast.BlockStatement:
		t.buf.WriteString("{\n")
		t.pushScope()
		for _, bStmt := range s.Statements {
			t.buf.WriteString("\t")
			t.emitStatement(bStmt)
		}
		t.popScope()
		t.buf.WriteString("}\n")
	case *ast.AssignStatement:
		// Handle parallel assignment by splitting them
		for i, name := range s.Names {
			val := t.emitExprStr(s.Values[i])
			if s.Token.Literal == ":=" {
				t.addLocal(name.Value)
				t.buf.WriteString(fmt.Sprintf("word v_%s = %s;\n", name.Value, val))
			} else {
				if t.isLocal(name.Value) {
					t.buf.WriteString(fmt.Sprintf("v_%s = %s;\n", name.Value, val))
				} else {
					t.buf.WriteString(fmt.Sprintf("v_%s_%s = %s;\n", t.pkgName, name.Value, val))
				}
			}
		}
	case *ast.IfStatement:
		t.buf.WriteString(fmt.Sprintf("if (%s) ", t.emitExprStr(s.Condition)))
		t.emitStatement(s.Consequence)
		if s.Alternative != nil {
			t.buf.WriteString(" else ")
			t.emitStatement(s.Alternative)
		}
	case *ast.ForStatement:
		t.buf.WriteString(fmt.Sprintf("while (%s) ", t.emitExprStr(s.Condition)))
		t.emitStatement(s.Body)
	case *ast.ReturnStatement:
		if s.ReturnValue != nil {
			t.buf.WriteString(fmt.Sprintf("return %s;\n", t.emitExprStr(s.ReturnValue)))
		} else {
			t.buf.WriteString("return;\n")
		}
	case *ast.ExpressionStatement:
		t.buf.WriteString(t.emitExprStr(s.Expression))
		t.buf.WriteString(";\n")
	}
}

func (t *Transpiler) emitExprStr(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if t.isLocal(e.Value) {
			return fmt.Sprintf("v_%s", e.Value)
		}
		return fmt.Sprintf("v_%s_%s", t.pkgName, e.Value)
	case *ast.IntegerLiteral:
		return strconv.FormatInt(e.Value, 10)
	case *ast.StringLiteral:
		return "\"" + e.Value + "\""
	case *ast.PrefixExpression:
		return fmt.Sprintf("(%s%s)", e.Operator, t.emitExprStr(e.Right))
	case *ast.InfixExpression:
		return fmt.Sprintf("(%s %s %s)", t.emitExprStr(e.Left), e.Operator, t.emitExprStr(e.Right))
	case *ast.CallExpression:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "print" || ident.Value == "println" {
				return t.emitPrint(ident.Value == "println", e.Arguments)
			}
			if ident.Value == "byte" || ident.Value == "word" {
				// C-style cast
				return fmt.Sprintf("((%s)(%s))", ident.Value, t.emitExprStr(e.Arguments[0]))
			}
			
			// Normal function call
			args := []string{}
			for _, arg := range e.Arguments {
				args = append(args, t.emitExprStr(arg))
			}
			return fmt.Sprintf("f_%s_%s(%s)", t.pkgName, ident.Value, strings.Join(args, ", "))
		}
		return ""
	}
	return ""
}

func (t *Transpiler) emitPrint(newline bool, args []ast.Expression) string {
	formatStrs := []string{}
	var argStrs []string

	for _, arg := range args {
		if strLit, ok := arg.(*ast.StringLiteral); ok {
			formatStrs = append(formatStrs, strLit.Value)
		} else {
			formatStrs = append(formatStrs, "%u")
			argStrs = append(argStrs, t.emitExprStr(arg))
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\\n"
	}

	if len(argStrs) > 0 {
		return fmt.Sprintf("printf(\"%s\", %s)", format, strings.Join(argStrs, ", "))
	}
	return fmt.Sprintf("printf(\"%s\")", format)
}
