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
	pkgName       string
	buf           bytes.Buffer
	typedefBuf    bytes.Buffer
	locals        []map[string]string
	globals       map[string]string
	arrayTypes    map[string]bool
	funcTypes     map[string]string
}

func New() *Transpiler {
	return &Transpiler{
		globals:   make(map[string]string),
		funcTypes: make(map[string]string),
	}
}

func (t *Transpiler) pushScope() {
	t.locals = append(t.locals, make(map[string]string))
}

func (t *Transpiler) popScope() {
	t.locals = t.locals[:len(t.locals)-1]
}

func (t *Transpiler) addLocal(name string, ctype string) {
	if len(t.locals) > 0 {
		t.locals[len(t.locals)-1][name] = ctype
	}
}

func (t *Transpiler) isLocal(name string) bool {
	for i := len(t.locals) - 1; i >= 0; i-- {
		if _, ok := t.locals[i][name]; ok {
			return true
		}
	}
	return false
}

func (t *Transpiler) getVarType(name string) string {
	for i := len(t.locals) - 1; i >= 0; i-- {
		if ctype, ok := t.locals[i][name]; ok {
			return ctype
		}
	}
	if ctype, ok := t.globals[name]; ok {
		return ctype
	}
	return "word"
}

func (t *Transpiler) typeOf(expr ast.Expression) string {
	if expr == nil {
		return "word"
	}
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return "word"
	case *ast.StringLiteral:
		return "word"
	case *ast.Identifier:
		return t.getVarType(e.Value)
	case *ast.CallExpression:
		if ident, ok := e.Function.(*ast.Identifier); ok {
			if ident.Value == "byte" || ident.Value == "word" {
				return ident.Value
			}
			if ctype, ok := t.funcTypes[ident.Value]; ok {
				return ctype
			}
		}
	case *ast.PrefixExpression:
		if e.Operator == "&" {
			res := t.typeOf(e.Right) + "*"
			fmt.Printf("DEBUG typeOf(&%s) -> %s\n", t.emitExprStr(e.Right), res)
			return res
		}
		if e.Operator == "*" {
			typ := t.typeOf(e.Right)
			if strings.HasSuffix(typ, "*") {
				return typ[:len(typ)-1]
			}
		}
		return t.typeOf(e.Right)
	case *ast.InfixExpression:
		return t.typeOf(e.Left)
	case *ast.PointerType:
		return t.mapType(e.Elt) + "*"
	}
	return "word"
}

func (t *Transpiler) Transpile(program *ast.Program) string {
	// Initialize
	t.arrayTypes = make(map[string]bool)

	// First pass: find package name
	t.pkgName = "main" // default
	for _, stmt := range program.Statements {
		if pkg, ok := stmt.(*ast.PackageStatement); ok {
			t.pkgName = pkg.Name.Value
			break
		}
	}

	var forwardBuf bytes.Buffer
	forwardBuf.WriteString("// Forward declarations\n")
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.TypeStatement:
			base := t.mapType(s.BaseType)
			forwardBuf.WriteString(fmt.Sprintf("typedef %s t_%s_%s;\n", base, t.pkgName, s.Name.Value))
		case *ast.FuncStatement:
			retType := "void"
			if s.ReturnType != nil {
				retType = t.mapType(s.ReturnType)
			}
			t.funcTypes[s.Name.Value] = retType
			forwardBuf.WriteString(t.emitFuncSignatureStr(s, true))
		}
	}

	t.buf.WriteString(forwardBuf.String())

	t.buf.WriteString("\n// Global variables and constants\n")
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.VarStatement:
			valType := "word"
			if s.ValueType != nil {
				valType = t.mapType(s.ValueType)
			}
			t.globals[s.Name.Value] = valType
			t.buf.WriteString(fmt.Sprintf("%s v_%s_%s", valType, t.pkgName, s.Name.Value))
			if s.Value != nil {
				t.buf.WriteString(fmt.Sprintf(" = %s", t.emitExprStr(s.Value)))
			} else {
				if strings.HasPrefix(valType, "t_arr_") || strings.HasPrefix(valType, "t_"+t.pkgName+"_") {
					t.buf.WriteString(" = {0}")
				} else {
					t.buf.WriteString(" = 0")
				}
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

	var finalBuf bytes.Buffer
	finalBuf.WriteString("#include <stdio.h>\n")
	finalBuf.WriteString("#include <stdint.h>\n\n")
	finalBuf.WriteString("typedef uint8_t byte;\n")
	finalBuf.WriteString("typedef uintptr_t word;\n\n")

	finalBuf.WriteString(t.typedefBuf.String())
	finalBuf.WriteString("\n")
	finalBuf.WriteString(t.buf.String())

	return finalBuf.String()
}

func (t *Transpiler) mapType(expr ast.Expression) string {
	if expr == nil {
		return "word"
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		name := e.Value
		if name == "byte" || name == "word" {
			return name
		}
		return fmt.Sprintf("t_%s_%s", t.pkgName, name)
	case *ast.ArrayType:
		lenStr := "0"
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = strconv.FormatInt(il.Value, 10)
		}
		eltName := t.mapType(e.Elt)
		typeName := fmt.Sprintf("t_arr_%s_%s", lenStr, eltName)
		
		if !t.arrayTypes[typeName] {
			t.arrayTypes[typeName] = true
			t.typedefBuf.WriteString(fmt.Sprintf("typedef struct { %s data[%s]; } %s;\n", eltName, lenStr, typeName))
		}
		return typeName
	case *ast.StructType:
		var fields []string
		for _, f := range e.Fields {
			fields = append(fields, fmt.Sprintf("%s %s", t.mapType(f.Type), f.Name.Value))
		}
		return fmt.Sprintf("struct { %s; }", strings.Join(fields, "; "))
	case *ast.PointerType:
		return t.mapType(e.Elt) + "*"
	}
	return "word"
}

func (t *Transpiler) emitFuncSignatureStr(s *ast.FuncStatement, isForward bool) string {
	retType := "void"
	if s.ReturnType != nil {
		retType = t.mapType(s.ReturnType)
	}

	var params []string
	
	funcName := s.Name.Value
	if s.Receiver != nil {
		recvType := t.mapType(s.Receiver.Type)
		baseType := recvType
		if strings.HasSuffix(baseType, "*") {
			baseType = baseType[:len(baseType)-1]
		}
		if strings.HasPrefix(baseType, "t_"+t.pkgName+"_") {
			baseType = baseType[len("t_"+t.pkgName+"_"):]
		}
		funcName = baseType + "_" + funcName
		
		if !isForward {
			t.addLocal(s.Receiver.Name.Value, recvType)
		}
		params = append(params, fmt.Sprintf("%s v_%s", recvType, s.Receiver.Name.Value))
	}

	for _, p := range s.Parameters {
		if !isForward {
			t.addLocal(p.Name.Value, t.mapType(p.Type))
		}
		params = append(params, fmt.Sprintf("%s v_%s", t.mapType(p.Type), p.Name.Value))
	}

	res := fmt.Sprintf("%s f_%s_%s(%s)", retType, t.pkgName, funcName, strings.Join(params, ", "))
	if isForward {
		res += ";\n"
	} else {
		res += " "
	}
	return res
}

func (t *Transpiler) emitStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.PackageStatement, *ast.ImportStatement, *ast.ConstStatement, *ast.TypeStatement:
		// Handled in earlier passes or ignored
	case *ast.VarStatement:
		valType := "word"
		if s.ValueType != nil {
			valType = t.mapType(s.ValueType)
		}
		t.addLocal(s.Name.Value, valType)
		t.buf.WriteString(fmt.Sprintf("%s v_%s", valType, s.Name.Value))
		if s.Value != nil {
			t.buf.WriteString(fmt.Sprintf(" = %s", t.emitExprStr(s.Value)))
		} else {
			if strings.HasPrefix(valType, "t_arr_") || strings.HasPrefix(valType, "t_"+t.pkgName+"_") {
				t.buf.WriteString(" = {0}")
			} else {
				t.buf.WriteString(" = 0")
			}
		}
		t.buf.WriteString(";\n")
	case *ast.FuncStatement:
		t.pushScope()
		t.buf.WriteString(t.emitFuncSignatureStr(s, false))
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
		for i, nameExpr := range s.Names {
			val := t.emitExprStr(s.Values[i])
			if s.Token.Literal == ":=" {
				if ident, ok := nameExpr.(*ast.Identifier); ok {
					ctype := t.typeOf(s.Values[i])
					t.addLocal(ident.Value, ctype)
					t.buf.WriteString(fmt.Sprintf("%s v_%s = %s;\n", ctype, ident.Value, val))
				}
			} else {
				if ident, ok := nameExpr.(*ast.Identifier); ok {
					if t.isLocal(ident.Value) {
						t.buf.WriteString(fmt.Sprintf("v_%s = %s;\n", ident.Value, val))
					} else {
						t.buf.WriteString(fmt.Sprintf("v_%s_%s = %s;\n", t.pkgName, ident.Value, val))
					}
				} else {
					// It's an IndexExpression
					t.buf.WriteString(fmt.Sprintf("%s = %s;\n", t.emitExprStr(nameExpr), val))
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
	case *ast.PointerType:
		return fmt.Sprintf("(*%s)", t.emitExprStr(e.Elt))
	case *ast.InfixExpression:
		return fmt.Sprintf("(%s %s %s)", t.emitExprStr(e.Left), e.Operator, t.emitExprStr(e.Right))
	case *ast.IndexExpression:
		return fmt.Sprintf("(%s).data[%s]", t.emitExprStr(e.Left), t.emitExprStr(e.Index))
	case *ast.SelectorExpression:
		if strings.HasSuffix(t.typeOf(e.Left), "*") {
			return fmt.Sprintf("(%s)->%s", t.emitExprStr(e.Left), e.Right.Value)
		}
		return fmt.Sprintf("(%s).%s", t.emitExprStr(e.Left), e.Right.Value)
	case *ast.CallExpression:
		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			receiverType := t.typeOf(sel.Left)
			baseType := receiverType
			isPtr := false
			if strings.HasSuffix(baseType, "*") {
				baseType = baseType[:len(baseType)-1]
				isPtr = true
			}
			if strings.HasPrefix(baseType, "t_"+t.pkgName+"_") {
				baseType = baseType[len("t_"+t.pkgName+"_"):]
			}
			funcName := baseType + "_" + sel.Right.Value
			
			receiverStr := t.emitExprStr(sel.Left)
			if !isPtr {
				receiverStr = "(&" + receiverStr + ")"
			}
			
			args := []string{receiverStr}
			for _, arg := range e.Arguments {
				args = append(args, t.emitExprStr(arg))
			}
			return fmt.Sprintf("f_%s_%s(%s)", t.pkgName, funcName, strings.Join(args, ", "))
		}

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
			formatStrs = append(formatStrs, "%llu")
			argStrs = append(argStrs, fmt.Sprintf("(unsigned long long)(%s)", t.emitExprStr(arg)))
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
