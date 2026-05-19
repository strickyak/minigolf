package transpiler

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/token"
)

// Transpiler walks the AST and emits C99 code
type Transpiler struct {
	buf               bytes.Buffer
	typedefBuf        bytes.Buffer
	funcDeclsBuf      bytes.Buffer
	genericImplBuf    bytes.Buffer
	locals            []map[string]string
	globals           map[string]string
	arrayTypes        map[string]bool
	funcTypes         map[string]string
	funcRetTypes      map[string][]string
	currentFunc       *ast.FuncStatement
	currentPackage    string
	typeDefs          map[string]*ast.TypeStatement
	typeAliases       map[string]ast.Expression
	genericTemplates  map[string]*GenericTemplate
	instantiatedTypes map[string]InstantiatedTypeInfoC
	irBuilder         *ir.Builder
}

type InstantiatedTypeInfoC struct {
	RawGenericName string
	ArgTyps        []string
}

type GenericTemplate struct {
	TypeParams []string
	Tokens     []token.Token
}

func New() *Transpiler {
	return &Transpiler{
		globals:           make(map[string]string),
		funcTypes:         make(map[string]string),
		funcRetTypes:      make(map[string][]string),
		genericTemplates:  make(map[string]*GenericTemplate),
		typeDefs:          make(map[string]*ast.TypeStatement),
		typeAliases:       make(map[string]ast.Expression),
		instantiatedTypes: make(map[string]InstantiatedTypeInfoC),
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
	if ctype, ok := t.globals[t.currentPackage+"."+name]; ok {
		return ctype
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
			if ctype, ok := t.funcTypes[t.currentPackage+"."+ident.Value]; ok {
				return ctype
			}
			qname := t.currentPackage + "." + ident.Value
			if _, ok := t.typeDefs[qname]; ok {
				return t.mapType(ident)
			}
		}
		if ptrType, ok := e.Function.(*ast.PointerType); ok {
			return t.mapType(ptrType)
		}
		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok {
				rawFuncName := t.currentPackage + "." + ident.Value
				var instTypStr string
				var argTyps []string
				for _, idx := range idxExpr.Indices {
					argTyp := t.mapType(idx)
					argTyps = append(argTyps, argTyp)
					instTypStr += "_" + argTyp
				}
				funcName := fmt.Sprintf("%s%s", rawFuncName, instTypStr)
				funcName = strings.ReplaceAll(funcName, ".", "_")

				if !t.arrayTypes[funcName] {
					t.arrayTypes[funcName] = true
					if tmpl, ok := t.genericTemplates[rawFuncName]; ok {
						t.instantiateGenericFuncC(funcName, rawFuncName, argTyps, tmpl)
					}
				}
				if ctype, ok := t.funcTypes[funcName]; ok {
					return ctype
				}
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
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := t.getVarType(pkgIdent.Value)

			// Extract struct name from mangled t_pkg_StructName
			structName := strings.TrimPrefix(qname, "t_")
			structName = strings.Replace(structName, "_", ".", 1)

			if st, ok := t.typeDefs[structName]; ok {
				if structType, ok := st.BaseType.(*ast.StructType); ok {
					for _, f := range structType.Fields {
						if f.Name.Value == e.Right.Value {
							if ident, ok := f.Type.(*ast.Identifier); ok {
								if ident.Value == "byte" || ident.Value == "word" {
									return ident.Value
								}
								if ident.Value == "int" {
									return "intptr_t"
								}
							}
							return t.mapType(f.Type)
						}
					}
				}
			}

			qname2 := t.currentPackage + "." + e.Right.Value
			if ctype, ok := t.globals[qname2]; ok {
				return ctype
			}
			if ctype, ok := t.funcTypes[qname2]; ok {
				return ctype
			}
			return "word"
		}
		return "word"
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
	t.irBuilder = ir.NewBuilder()
	t.irBuilder.Build(program)

	// First pass: find package name
	t.currentPackage = "main" // default

	t.typedefBuf.WriteString("// Forward type declarations\n")
	t.funcDeclsBuf.WriteString("// Forward function declarations\n")
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			t.currentPackage = s.Name.Value
		case *ast.TypeStatement:
			t.typeDefs[t.currentPackage+"."+s.Name.Value] = s
			if len(s.TypeParameters) > 0 {
				qname := t.currentPackage + "." + s.Name.Value
				var typeParams []string
				for _, tp := range s.TypeParameters {
					typeParams = append(typeParams, tp.Value)
				}
				t.genericTemplates[qname] = &GenericTemplate{
					TypeParams: typeParams,
					Tokens:     s.Tokens,
				}
				continue
			}
			if s.IsAlias {
				t.typeAliases[t.currentPackage+"."+s.Name.Value] = s.BaseType
				continue
			}
			base := t.mapType(s.BaseType)
			name := fmt.Sprintf("t_%s_%s", t.currentPackage, s.Name.Value)
			if strings.HasPrefix(base, "struct {") {
				t.typedefBuf.WriteString(fmt.Sprintf("typedef struct %s %s;\n", name, name))
				t.typedefBuf.WriteString(fmt.Sprintf("struct %s %s;\n", name, base[6:]))
			} else {
				t.typedefBuf.WriteString(fmt.Sprintf("typedef %s %s;\n", base, name))
			}
		case *ast.FuncStatement:
			if len(s.TypeParameters) > 0 {
				qname := t.currentPackage + "." + s.Name.Value
				if s.Receiver != nil {
					var rawBase string
					var findRawBase func(expr ast.Expression)
					findRawBase = func(expr ast.Expression) {
						switch e := expr.(type) {
						case *ast.PointerType:
							findRawBase(e.Elt)
						case *ast.IndexExpression:
							if ident, ok := e.Left.(*ast.Identifier); ok {
								rawBase = ident.Value
							}
						case *ast.Identifier:
							rawBase = e.Value
						}
					}
					findRawBase(s.Receiver.Type)
					qname = t.currentPackage + "." + rawBase + "_" + s.Name.Value
				}
				var typeParams []string
				for _, tp := range s.TypeParameters {
					typeParams = append(typeParams, tp.Value)
				}
				t.genericTemplates[qname] = &GenericTemplate{
					TypeParams: typeParams,
					Tokens:     s.Tokens,
				}
				continue
			}
			retType := "void"
			if len(s.ReturnTypes) == 1 {
				retType = t.mapType(s.ReturnTypes[0])
			} else if len(s.ReturnTypes) > 1 {
				var fields []string
				var retTypes []string
				for i, rt := range s.ReturnTypes {
					mapped := t.mapType(rt)
					fields = append(fields, fmt.Sprintf("%s f%d", mapped, i))
					retTypes = append(retTypes, mapped)
				}
				funcName := s.Name.Value
				if s.Receiver != nil {
					recvType := t.mapType(s.Receiver.Type)
					baseType := recvType
					baseType = strings.TrimSuffix(baseType, "*")
					if strings.HasPrefix(baseType, "t_"+t.currentPackage+"_") {
						baseType = baseType[len("t_"+t.currentPackage+"_"):]
					} else {
						// It might be from another package?
						// In transpiler, recvType has format t_pkg_Type
						if strings.HasPrefix(baseType, "t_") {
							parts := strings.SplitN(baseType[2:], "_", 2)
							if len(parts) == 2 {
								baseType = parts[0] + "_" + parts[1]
							}
						}
					}
					funcName = baseType + "_" + funcName
				}
				structName := fmt.Sprintf("f_%s_%s_returns", t.currentPackage, funcName)
				retType = fmt.Sprintf("struct %s", structName)
				t.typedefBuf.WriteString(fmt.Sprintf("%s { %s; };\n", retType, strings.Join(fields, "; ")))
				t.funcRetTypes[t.currentPackage+"."+s.Name.Value] = retTypes
			}
			t.funcTypes[t.currentPackage+"."+s.Name.Value] = retType
			t.funcDeclsBuf.WriteString(t.emitFuncSignatureStr(s, true))
		}
	}

	t.buf.WriteString("\n// Global variables and constants\n")
	t.currentPackage = "main"
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			t.currentPackage = s.Name.Value
		case *ast.VarStatement:
			valType := "word"
			if s.ValueType != nil {
				valType = t.mapType(s.ValueType)
			}
			t.globals[t.currentPackage+"."+s.Name.Value] = valType
			t.buf.WriteString(fmt.Sprintf("%s v_%s_%s", valType, t.currentPackage, s.Name.Value))
			if s.Value != nil {
				t.buf.WriteString(fmt.Sprintf(" = %s", t.emitExprStr(s.Value)))
			} else {
				if strings.HasPrefix(valType, "t_") && !strings.HasSuffix(valType, "*") {
					t.buf.WriteString(" = {0}")
				} else {
					t.buf.WriteString(" = 0")
				}
			}
			t.buf.WriteString(";\n")
		case *ast.ConstStatement:
			t.buf.WriteString(fmt.Sprintf("#define v_%s_%s %s\n", t.currentPackage, s.Name.Value, t.emitExprStr(s.Value)))
		}
	}
	t.buf.WriteString("\n")

	// Third pass: Implementations
	t.currentPackage = "main"
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			t.currentPackage = s.Name.Value
		case *ast.FuncStatement:
			if len(s.TypeParameters) > 0 {
				continue
			}
			t.emitStatement(s)
		}
	}

	// Finally: C main function
	t.buf.WriteString("\nint main() {\n")
	t.buf.WriteString("\tf_main_main();\n")
	t.buf.WriteString("\treturn 0;\n")
	t.buf.WriteString("}\n")

	var finalBuf bytes.Buffer
	finalBuf.WriteString("#include <stdio.h>\n")
	finalBuf.WriteString("#include <stdint.h>\n\n")
	finalBuf.WriteString("typedef uint8_t byte;\n")
	finalBuf.WriteString("typedef uintptr_t word;\n\n")

	finalBuf.WriteString(t.typedefBuf.String())
	finalBuf.WriteString("\n")
	finalBuf.WriteString(t.funcDeclsBuf.String())
	finalBuf.WriteString("\n")
	finalBuf.WriteString(t.genericImplBuf.String())
	finalBuf.WriteString(t.buf.String())

	return finalBuf.String()
}

func extractTypeParamsC(paramType ast.Expression, argTyp string, typeMap map[string]string, typeParams []string) {
	if ident, ok := paramType.(*ast.Identifier); ok {
		for _, tp := range typeParams {
			if tp == ident.Value {
				typeMap[tp] = argTyp
				return
			}
		}
	} else if prefix, ok := paramType.(*ast.PrefixExpression); ok && prefix.Operator == "*" {
		if strings.HasSuffix(argTyp, "*") {
			argTyp = strings.TrimSpace(argTyp[:len(argTyp)-1])
			extractTypeParamsC(prefix.Right, argTyp, typeMap, typeParams)
		} else if (ir.Type{Name: argTyp}).IsAPointer() {
			extractTypeParamsC(prefix.Right, (ir.Type{Name: argTyp}).PointedType().Name, typeMap, typeParams)
		} else {
			extractTypeParamsC(prefix.Right, argTyp, typeMap, typeParams)
		}
	} else if ptr, ok := paramType.(*ast.PointerType); ok {
		if strings.HasSuffix(argTyp, "*") {
			argTyp = strings.TrimSpace(argTyp[:len(argTyp)-1])
			extractTypeParamsC(ptr.Elt, argTyp, typeMap, typeParams)
		} else if (ir.Type{Name: argTyp}).IsAPointer() {
			extractTypeParamsC(ptr.Elt, (ir.Type{Name: argTyp}).PointedType().Name, typeMap, typeParams)
		} else {
			extractTypeParamsC(ptr.Elt, argTyp, typeMap, typeParams)
		}
	} else if idx, ok := paramType.(*ast.IndexExpression); ok {
		parts := strings.Split(argTyp, "_")
		numIdx := len(idx.Indices)
		if len(parts) >= numIdx {
			for i, innerIdx := range idx.Indices {
				extractTypeParamsC(innerIdx, parts[len(parts)-numIdx+i], typeMap, typeParams)
			}
		}
	}
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
		if name == "uint" {
			return "word"
		}
		if name == "int" {
			return "intptr_t"
		}
		if name == "intptr_t" {
			return "intptr_t"
		}
		if strings.HasPrefix(name, "t_") {
			return name
		}
		qname := t.currentPackage + "." + name

		if _, ok := t.typeDefs[qname]; !ok {
			if _, ok := t.typeDefs["prelude."+name]; ok {
				qname = "prelude." + name
			} else if _, ok := t.typeAliases["prelude."+name]; ok {
				qname = "prelude." + name
			}
		}

		if aliasExpr, ok := t.typeAliases[qname]; ok {
			return t.mapType(aliasExpr)
		}

		if _, ok := t.typeDefs[qname]; ok {
			parts := strings.SplitN(qname, ".", 2)
			return fmt.Sprintf("t_%s_%s", parts[0], parts[1])
		}
		return fmt.Sprintf("t_%s_%s", t.currentPackage, name)
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			if t.isLocal(pkgIdent.Value) || t.getVarType(pkgIdent.Value) != "word" {
				return "word" // Struct field access mapped to word by default in transpiler
			}
			return fmt.Sprintf("t_%s_%s", pkgIdent.Value, e.Right.Value)
		}
		return "word"
	case *ast.IndexExpression:
		var genericName string
		var rawGenericName string
		if ident, ok := e.Left.(*ast.Identifier); ok {
			genericName = t.currentPackage + "_" + ident.Value
			rawGenericName = t.currentPackage + "." + ident.Value
		} else if sel, ok := e.Left.(*ast.SelectorExpression); ok {
			if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
				genericName = pkgIdent.Value + "_" + sel.Right.Value
				rawGenericName = pkgIdent.Value + "." + sel.Right.Value
			}
		}

		if genericName != "" {
			var argTyps []string
			for _, argNode := range e.Indices {
				argTyps = append(argTyps, t.mapType(argNode))
			}

			instTypStr := strings.Join(argTyps, "_")
			instName := fmt.Sprintf("t_%s_%s", genericName, instTypStr)
			instName = strings.ReplaceAll(instName, "*", "ptr_")
			instName = strings.ReplaceAll(instName, "[", "arr_")
			instName = strings.ReplaceAll(instName, "]", "_")

			t.instantiatedTypes[strings.ReplaceAll(instName, ".", "_")] = InstantiatedTypeInfoC{
				RawGenericName: rawGenericName,
				ArgTyps:        argTyps,
			}

			if !t.arrayTypes[instName] {
				t.arrayTypes[instName] = true
				if tmpl, ok := t.genericTemplates[rawGenericName]; ok {
					t.instantiateGenericC(instName, rawGenericName, e.Indices, tmpl)
				} else if ident, ok := e.Left.(*ast.Identifier); ok {
					preludeRawGenericName := "prelude." + ident.Value
					if tmpl, ok := t.genericTemplates[preludeRawGenericName]; ok {
						rawGenericName = preludeRawGenericName
						genericName = "prelude_" + ident.Value
						instName = fmt.Sprintf("t_%s_%s", genericName, instTypStr)
						instName = strings.ReplaceAll(instName, "*", "ptr_")
						instName = strings.ReplaceAll(instName, "[", "arr_")
						instName = strings.ReplaceAll(instName, "]", "_")
						if !t.arrayTypes[instName] {
							t.arrayTypes[instName] = true
							t.instantiatedTypes[strings.ReplaceAll(instName, ".", "_")] = InstantiatedTypeInfoC{
								RawGenericName: rawGenericName,
								ArgTyps:        argTyps,
							}
							t.instantiateGenericC(instName, rawGenericName, e.Indices, tmpl)
						}
					}
				}
			}
			return instName
		}
		return "word"
	case *ast.ArrayType:
		lenVal := t.irBuilder.EvalConst(e.Length)
		lenStr := strconv.FormatInt(lenVal, 10)
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

func (t *Transpiler) instantiateGenericC(instName, genericName string, argNodes []ast.Expression, tmpl *GenericTemplate) {
	var argTokensList [][]token.Token
	for _, argNode := range argNodes {
		argTyp := astToGoString(argNode)
		argTokens := lexer.Lex(argTyp, "generic_inst")
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.EOF {
			argTokens = argTokens[:len(argTokens)-1]
		}
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.SEMICOLON {
			argTokens = argTokens[:len(argTokens)-1]
		}
		argTokensList = append(argTokensList, argTokens)
	}

	var newTokens []token.Token
	for _, tok := range tmpl.Tokens {
		replaced := false
		if tok.Type == token.IDENT {
			for i, tp := range tmpl.TypeParams {
				if tok.Literal == tp {
					if i < len(argTokensList) {
						newTokens = append(newTokens, argTokensList[i]...)
						replaced = true
						break
					}
				}
			}
		}
		if !replaced {
			newTokens = append(newTokens, tok)
		}
	}
	newTokens = append(newTokens, token.Token{Type: token.EOF, Literal: ""})

	p := parser.New(newTokens)
	baseTypeAST := p.ParseExpressionForGeneric()

	base := t.mapType(baseTypeAST)
	if strings.HasPrefix(base, "struct {") {
		t.typedefBuf.WriteString(fmt.Sprintf("typedef struct %s %s;\n", instName, instName))
		t.typedefBuf.WriteString(fmt.Sprintf("struct %s %s;\n", instName, base[6:]))
	} else {
		t.typedefBuf.WriteString(fmt.Sprintf("typedef %s %s;\n", base, instName))
	}
}

func (t *Transpiler) instantiateGenericFuncC(instName, genericName string, argTyps []string, tmpl *GenericTemplate) {
	var argTokensList [][]token.Token
	for _, argTyp := range argTyps {
		argTokens := lexer.Lex(argTyp, "generic_inst")
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.EOF {
			argTokens = argTokens[:len(argTokens)-1]
		}
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.SEMICOLON {
			argTokens = argTokens[:len(argTokens)-1]
		}
		argTokensList = append(argTokensList, argTokens)
	}

	var newTokens []token.Token
	for _, tok := range tmpl.Tokens {
		replaced := false
		if tok.Type == token.IDENT {
			for i, tp := range tmpl.TypeParams {
				if tok.Literal == tp {
					if i < len(argTokensList) {
						newTokens = append(newTokens, argTokensList[i]...)
						replaced = true
						break
					}
				}
			}
		}
		if !replaced {
			newTokens = append(newTokens, tok)
		}
	}
	newTokens = append(newTokens, token.Token{Type: token.EOF, Literal: ""})

	p := parser.New(newTokens)
	stmt := p.ParseStatementForGeneric()
	if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
		oldPkg := t.currentPackage
		var parts []string
		if strings.Contains(instName, ".") {
			parts = strings.SplitN(instName, ".", 2)
		} else {
			parts = strings.SplitN(instName, "_", 2)
		}
		if len(parts) == 2 {
			t.currentPackage = parts[0]
			if funcStmt.Receiver == nil {
				funcStmt.Name.Value = parts[1]
			}
		} else {
			if funcStmt.Receiver == nil {
				funcStmt.Name.Value = instName
			}
		}

		oldFunc := t.currentFunc
		t.currentFunc = funcStmt

		retType := "void"
		if len(funcStmt.ReturnTypes) == 1 {
			retType = t.mapType(funcStmt.ReturnTypes[0])
		} else if len(funcStmt.ReturnTypes) > 1 {
			retType = fmt.Sprintf("t_tuple_%d", len(funcStmt.ReturnTypes))
		}
		t.funcTypes[instName] = retType

		funcSigDecl := t.emitFuncSignatureStr(funcStmt, true)
		t.funcDeclsBuf.WriteString(funcSigDecl)

		savedBytes := append([]byte(nil), t.buf.Bytes()...)
		t.buf.Reset()

		t.emitStatement(funcStmt)
		t.buf.WriteString("\n")

		t.genericImplBuf.WriteString(t.buf.String())

		t.buf.Reset()
		t.buf.Write(savedBytes)
		t.currentFunc = oldFunc
		t.currentPackage = oldPkg
	}
}

func astToGoString(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value
	case *ast.SelectorExpression:
		return astToGoString(e.Left) + "." + e.Right.Value
	case *ast.PointerType:
		return "*" + astToGoString(e.Elt)
	case *ast.ArrayType:
		lenStr := ""
		if il, ok := e.Length.(*ast.IntegerLiteral); ok {
			lenStr = strconv.FormatInt(il.Value, 10)
		}
		return "[" + lenStr + "]" + astToGoString(e.Elt)
	case *ast.IndexExpression:
		var indices []string
		for _, idx := range e.Indices {
			indices = append(indices, astToGoString(idx))
		}
		return astToGoString(e.Left) + "[" + strings.Join(indices, ",") + "]"
	}
	return "word"
}

func (t *Transpiler) emitFuncSignatureStr(s *ast.FuncStatement, isForward bool) string {
	retType := "void"
	if rt, ok := t.funcTypes[t.currentPackage+"."+s.Name.Value]; ok {
		retType = rt
	} else if len(s.ReturnTypes) == 1 {
		retType = t.mapType(s.ReturnTypes[0])
	}

	var params []string

	funcName := s.Name.Value
	if s.Receiver != nil {
		recvType := t.mapType(s.Receiver.Type)
		baseType := recvType
		baseType = strings.TrimSuffix(baseType, "*")
		if strings.HasPrefix(baseType, "t_"+t.currentPackage+"_") {
			baseType = baseType[len("t_"+t.currentPackage+"_"):]
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

	res := fmt.Sprintf("%s f_%s_%s(%s)", retType, t.currentPackage, funcName, strings.Join(params, ", "))
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
			if strings.HasPrefix(valType, "t_") && !strings.HasSuffix(valType, "*") {
				t.buf.WriteString(" = {0}")
			} else {
				t.buf.WriteString(" = 0")
			}
		}
		t.buf.WriteString(";\n")
	case *ast.FuncStatement:
		if s.Body == nil {
			t.buf.WriteString(t.emitFuncSignatureStr(s, true))
			break
		}
		prevFunc := t.currentFunc
		t.currentFunc = s
		t.pushScope()
		t.buf.WriteString(t.emitFuncSignatureStr(s, false))
		t.emitStatement(s.Body)
		t.popScope()
		t.buf.WriteString("\n")
		t.currentFunc = prevFunc
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
		if len(s.Names) > 1 && len(s.Values) > 1 {
			for i := range s.Values {
				ctype := t.typeOf(s.Values[i])
				t.buf.WriteString(fmt.Sprintf("%s tmp_val_%p_%d = %s;\n", ctype, s, i, t.emitExprStr(s.Values[i])))
			}
			for i, nameExpr := range s.Names {
				if s.Token.Literal == ":=" {
					if ident, ok := nameExpr.(*ast.Identifier); ok {
						ctype := t.typeOf(s.Values[i])
						t.addLocal(ident.Value, ctype)
						t.buf.WriteString(fmt.Sprintf("%s v_%s = tmp_val_%p_%d;\n", ctype, ident.Value, s, i))
					}
				} else {
					if ident, ok := nameExpr.(*ast.Identifier); ok {
						if t.isLocal(ident.Value) {
							t.buf.WriteString(fmt.Sprintf("v_%s = tmp_val_%p_%d;\n", ident.Value, s, i))
						} else {
							t.buf.WriteString(fmt.Sprintf("v_%s_%s = tmp_val_%p_%d;\n", t.currentPackage, ident.Value, s, i))
						}
					} else {
						t.buf.WriteString(fmt.Sprintf("%s = tmp_val_%p_%d;\n", t.emitExprStr(nameExpr), s, i))
					}
				}
			}
		} else if len(s.Names) > 1 && len(s.Values) == 1 {
			tmpName := fmt.Sprintf("tmp_tuple_%p", s)
			ctype := t.typeOf(s.Values[0])
			t.buf.WriteString(fmt.Sprintf("%s %s = %s;\n", ctype, tmpName, t.emitExprStr(s.Values[0])))
			var fieldTypes []string
			if callExpr, ok := s.Values[0].(*ast.CallExpression); ok {
				if ident, ok := callExpr.Function.(*ast.Identifier); ok {
					fieldTypes = t.funcRetTypes[ident.Value]
				}
			}
			for i, nameExpr := range s.Names {
				fType := "word"
				if i < len(fieldTypes) {
					fType = fieldTypes[i]
				}
				if s.Token.Literal == ":=" {
					if ident, ok := nameExpr.(*ast.Identifier); ok {
						t.addLocal(ident.Value, fType)
						t.buf.WriteString(fmt.Sprintf("%s v_%s = %s.f%d;\n", fType, ident.Value, tmpName, i))
					}
				} else {
					if ident, ok := nameExpr.(*ast.Identifier); ok {
						if t.isLocal(ident.Value) {
							t.buf.WriteString(fmt.Sprintf("v_%s = %s.f%d;\n", ident.Value, tmpName, i))
						} else {
							t.buf.WriteString(fmt.Sprintf("v_%s_%s = %s.f%d;\n", t.currentPackage, ident.Value, tmpName, i))
						}
					} else {
						t.buf.WriteString(fmt.Sprintf("%s = %s.f%d;\n", t.emitExprStr(nameExpr), tmpName, i))
					}
				}
			}
		} else {
			// Single assignment
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
							t.buf.WriteString(fmt.Sprintf("v_%s_%s = %s;\n", t.currentPackage, ident.Value, val))
						}
					} else {
						t.buf.WriteString(fmt.Sprintf("%s = %s;\n", t.emitExprStr(nameExpr), val))
					}
				}
			}
		}
	case *ast.IncDecStatement:
		val := t.emitExprStr(s.Name)
		op := "++"
		if s.Token.Literal == "--" {
			op = "--"
		}

		if ident, ok := s.Name.(*ast.Identifier); ok {
			if t.isLocal(ident.Value) {
				t.buf.WriteString(fmt.Sprintf("v_%s%s;\n", ident.Value, op))
			} else {
				t.buf.WriteString(fmt.Sprintf("v_%s_%s%s;\n", t.currentPackage, ident.Value, op))
			}
		} else {
			t.buf.WriteString(fmt.Sprintf("%s%s;\n", val, op))
		}
	case *ast.IfStatement:
		t.buf.WriteString(fmt.Sprintf("if (%s) ", t.emitExprStr(s.Condition)))
		t.emitStatement(s.Consequence)
		if s.Alternative != nil {
			t.buf.WriteString(" else ")
			t.emitStatement(s.Alternative)
		}
	case *ast.ForStatement:
		condStr := "1"
		if s.Condition != nil {
			condStr = t.emitExprStr(s.Condition)
		}
		t.buf.WriteString(fmt.Sprintf("while (%s) ", condStr))
		t.emitStatement(s.Body)
	case *ast.For3Statement:
		t.buf.WriteString("{\n")
		t.pushScope()
		if s.Init != nil {
			t.emitStatement(s.Init)
		}
		condStr := "1"
		if s.Condition != nil {
			condStr = t.emitExprStr(s.Condition)
		}
		t.buf.WriteString(fmt.Sprintf("while (%s) {\n", condStr))
		for _, bStmt := range s.Body.Statements {
			t.buf.WriteString("\t")
			t.emitStatement(bStmt)
		}
		if s.Increment != nil {
			t.buf.WriteString("\t")
			t.emitStatement(s.Increment)
		}
		t.buf.WriteString("}\n")
		t.popScope()
		t.buf.WriteString("}\n")
	case *ast.ForRangeStatement:
		t.buf.WriteString("{\n")
		t.pushScope()
		limitVal := t.emitExprStr(s.RangeValue)
		ctype := t.typeOf(s.RangeValue)

		ident, ok := s.Key.(*ast.Identifier)
		var loopVar string
		if ok {
			if s.IsDecl {
				t.addLocal(ident.Value, ctype)
				t.buf.WriteString(fmt.Sprintf("%s v_%s = 0;\n", ctype, ident.Value))
				loopVar = fmt.Sprintf("v_%s", ident.Value)
			} else {
				if t.isLocal(ident.Value) {
					t.buf.WriteString(fmt.Sprintf("v_%s = 0;\n", ident.Value))
					loopVar = fmt.Sprintf("v_%s", ident.Value)
				} else {
					t.buf.WriteString(fmt.Sprintf("v_%s_%s = 0;\n", t.currentPackage, ident.Value))
					loopVar = fmt.Sprintf("v_%s_%s", t.currentPackage, ident.Value)
				}
			}
			t.buf.WriteString(fmt.Sprintf("%s limit_val = %s;\n", ctype, limitVal))
			t.buf.WriteString(fmt.Sprintf("while (%s < limit_val) {\n", loopVar))
			for _, bStmt := range s.Body.Statements {
				t.buf.WriteString("\t")
				t.emitStatement(bStmt)
			}
			t.buf.WriteString(fmt.Sprintf("\t%s++;\n", loopVar))
			t.buf.WriteString("}\n")
		} else {
			t.buf.WriteString(fmt.Sprintf("while(0) {\n"))
		}
		t.popScope()
		t.buf.WriteString("}\n")
	case *ast.ReturnStatement:
		if len(s.ReturnValues) == 1 {
			t.buf.WriteString(fmt.Sprintf("return %s;\n", t.emitExprStr(s.ReturnValues[0])))
		} else if len(s.ReturnValues) > 1 {
			structTyp := t.funcTypes[t.currentPackage+"."+t.currentFunc.Name.Value]
			var vals []string
			for _, rv := range s.ReturnValues {
				vals = append(vals, t.emitExprStr(rv))
			}
			t.buf.WriteString(fmt.Sprintf("return (%s){ %s };\n", structTyp, strings.Join(vals, ", ")))
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
		return fmt.Sprintf("v_%s_%s", t.currentPackage, e.Value)
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
		return fmt.Sprintf("(%s).data[%s]", t.emitExprStr(e.Left), t.emitExprStr(e.Indices[0]))
	case *ast.SelectorExpression:
		if pkgIdent, ok := e.Left.(*ast.Identifier); ok {
			qname := pkgIdent.Value + "." + e.Right.Value
			if _, ok := t.globals[qname]; ok {
				return fmt.Sprintf("v_%s_%s", pkgIdent.Value, e.Right.Value)
			}
		}
		if strings.HasSuffix(t.typeOf(e.Left), "*") {
			return fmt.Sprintf("(%s)->%s", t.emitExprStr(e.Left), e.Right.Value)
		}
		return fmt.Sprintf("(%s).%s", t.emitExprStr(e.Left), e.Right.Value)
	case *ast.CallExpression:
		if ptrType, ok := e.Function.(*ast.PointerType); ok {
			targetTyp := t.mapType(ptrType)
			argStr := t.emitExprStr(e.Arguments[0])
			return fmt.Sprintf("((%s)(%s))", targetTyp, argStr)
		}

		var isGenericFunc bool
		var funcName string
		var rawFuncName string
		var args []string

		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			if ident, ok := idxExpr.Left.(*ast.Identifier); ok {
				if ident.Value == "sizeof" {
					targetTyp := t.mapType(idxExpr.Indices[0])
					return fmt.Sprintf("sizeof(%s)", targetTyp)
				}
				rawFuncName = t.currentPackage + "." + ident.Value
			} else if sel, ok := idxExpr.Left.(*ast.SelectorExpression); ok {
				if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
					rawFuncName = pkgIdent.Value + "." + sel.Right.Value
				}
			}
			if rawFuncName != "" {
				var instTypStr string
				var argTyps []string
				for _, idx := range idxExpr.Indices {
					argTyp := t.mapType(idx)
					argTyps = append(argTyps, argTyp)
					instTypStr += "_" + argTyp
				}
				funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
				funcName = strings.ReplaceAll(funcName, ".", "_")
				if !t.arrayTypes[funcName] {
					t.arrayTypes[funcName] = true
					if tmpl, ok := t.genericTemplates[rawFuncName]; ok {
						t.instantiateGenericFuncC(funcName, rawFuncName, argTyps, tmpl)
					} else if ident, ok := idxExpr.Left.(*ast.Identifier); ok {
						preludeRawFuncName := "prelude." + ident.Value
						if tmpl, ok := t.genericTemplates[preludeRawFuncName]; ok {
							rawFuncName = preludeRawFuncName
							funcName = fmt.Sprintf("prelude_%s%s", ident.Value, instTypStr)
							funcName = strings.ReplaceAll(funcName, ".", "_")
							t.arrayTypes[funcName] = true
							t.instantiateGenericFuncC(funcName, rawFuncName, argTyps, tmpl)
						}
					}
				}
				isGenericFunc = true
			}
		} else if ident, ok := e.Function.(*ast.Identifier); ok {
			rawFuncName = t.currentPackage + "." + ident.Value
			if _, ok := t.funcTypes[rawFuncName]; !ok {
				if tmpl, ok := t.genericTemplates[rawFuncName]; ok {
					p := parser.New(tmpl.Tokens)
					stmt := p.ParseStatementForGeneric()
					if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
						typeMap := make(map[string]string)
						for _, arg := range e.Arguments {
							args = append(args, t.emitExprStr(arg))
						}
						for i, param := range funcStmt.Parameters {
							if i < len(args) {
								extractTypeParamsC(param.Type, t.typeOf(e.Arguments[i]), typeMap, tmpl.TypeParams)
							}
						}

						var argTyps []string
						var instTypStr string
						for _, tp := range tmpl.TypeParams {
							argTyp := typeMap[tp]
							if argTyp == "" {
								argTyp = "word"
							}
							argTyps = append(argTyps, argTyp)
							instTypStr += "_" + argTyp
						}
						funcName = fmt.Sprintf("%s%s", rawFuncName, instTypStr)
						funcName = strings.ReplaceAll(funcName, ".", "_")
						if !t.arrayTypes[funcName] {
							t.arrayTypes[funcName] = true
							t.instantiateGenericFuncC(funcName, rawFuncName, argTyps, tmpl)
						}
						isGenericFunc = true
					}
				} else if tmpl, ok := t.genericTemplates["prelude."+ident.Value]; ok {
					rawFuncName = "prelude." + ident.Value
					p := parser.New(tmpl.Tokens)
					stmt := p.ParseStatementForGeneric()
					if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
						typeMap := make(map[string]string)
						for _, arg := range e.Arguments {
							args = append(args, t.emitExprStr(arg))
						}
						for i, param := range funcStmt.Parameters {
							if i < len(args) {
								extractTypeParamsC(param.Type, t.typeOf(e.Arguments[i]), typeMap, tmpl.TypeParams)
							}
						}

						var argTyps []string
						var instTypStr string
						for _, tp := range tmpl.TypeParams {
							argTyp := typeMap[tp]
							if argTyp == "" {
								argTyp = "word"
							}
							argTyps = append(argTyps, argTyp)
							instTypStr += "_" + argTyp
						}
						funcName = fmt.Sprintf("prelude_%s%s", ident.Value, instTypStr)
						funcName = strings.ReplaceAll(funcName, ".", "_")
						if !t.arrayTypes[funcName] {
							t.arrayTypes[funcName] = true
							t.instantiateGenericFuncC(funcName, rawFuncName, argTyps, tmpl)
						}
						isGenericFunc = true
					}
				} else if _, ok := t.funcTypes["prelude."+ident.Value]; ok {
					funcName = "prelude_" + ident.Value
				}
			}
		}

		if isGenericFunc {
			if len(args) == 0 && len(e.Arguments) > 0 {
				for _, arg := range e.Arguments {
					args = append(args, t.emitExprStr(arg))
				}
			}
			return fmt.Sprintf("f_%s(%s)", funcName, strings.Join(args, ", "))
		}

		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			if pkgIdent, ok := sel.Left.(*ast.Identifier); ok {
				funcQName := pkgIdent.Value + "." + sel.Right.Value
				if _, ok := t.funcTypes[funcQName]; ok {
					args := []string{}
					for _, arg := range e.Arguments {
						args = append(args, t.emitExprStr(arg))
					}
					return fmt.Sprintf("f_%s_%s(%s)", pkgIdent.Value, sel.Right.Value, strings.Join(args, ", "))
				}
			}

			receiverType := t.typeOf(sel.Left)
			baseType := receiverType
			isPtr := false
			if strings.HasSuffix(baseType, "*") {
				baseType = baseType[:len(baseType)-1]
				isPtr = true
			}

			pkgPart := t.currentPackage
			if strings.HasPrefix(baseType, "t_") {
				parts := strings.SplitN(baseType[2:], "_", 2)
				if len(parts) == 2 {
					pkgPart = parts[0]
					baseType = parts[1]
				}
			}

			funcName := baseType + "_" + sel.Right.Value

			funcQNameCheck := pkgPart + "." + funcName
			if _, exists := t.funcTypes[funcQNameCheck]; !exists {
				if instInfo, ok := t.instantiatedTypes["t_"+pkgPart+"_"+baseType]; ok {
					rawGenericFuncName := instInfo.RawGenericName + "_" + sel.Right.Value
					if tmpl, ok := t.genericTemplates[rawGenericFuncName]; ok {
						t.instantiateGenericFuncC(funcQNameCheck, rawGenericFuncName, instInfo.ArgTyps, tmpl)
					}
				}
			}

			receiverStr := t.emitExprStr(sel.Left)
			if !isPtr {
				receiverStr = "(&" + receiverStr + ")"
			}

			args := []string{receiverStr}
			for _, arg := range e.Arguments {
				args = append(args, t.emitExprStr(arg))
			}
			return fmt.Sprintf("f_%s_%s(%s)", pkgPart, funcName, strings.Join(args, ", "))
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
			funcName := ident.Value
			return fmt.Sprintf("f_%s_%s(%s)", t.currentPackage, funcName, strings.Join(args, ", "))
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
