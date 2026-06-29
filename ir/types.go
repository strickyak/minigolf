package ir

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/token"
)

// TypeManager centralizes all type-related state and operations
// for the IR builder. It is constructed 1-1 with a Builder and
// they share a paired lifetime.
type TypeManager struct {
	builder *Builder

	typeDefsAST       map[string]*ast.StructType
	typeAliases       map[string]ast.Expression
	genericTemplates  map[string]*GenericTemplate
	instantiatedTypes map[string]InstantiatedTypeInfo
	evaluatingType    map[string]bool
}

func newTypeManager(builder *Builder) *TypeManager {
	return &TypeManager{
		builder:           builder,
		typeDefsAST:       make(map[string]*ast.StructType),
		typeAliases:       make(map[string]ast.Expression),
		genericTemplates:  make(map[string]*GenericTemplate),
		instantiatedTypes: make(map[string]InstantiatedTypeInfo),
		evaluatingType:    make(map[string]bool),
	}
}

func (tm *TypeManager) astToIRType(expr ast.Expression) Type {
	b := tm.builder
	if se, ok := expr.(*ast.SelectorExpression); ok {
		_ = se
		//fmt.Printf("DEBUG ASTTOIRTYPE SELECTOR: %#v\n", se)
	}
	if expr == nil {
		log.Panicf("TODO: when is expr nil?")
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		switch e.Value {
		case "bool":
			TypeBool.Builder = b
			return TypeBool
		case "byte":
			TypeByte.Builder = b
			return TypeByte
		case "word", "uint", "noreturn":
			TypeWord.Builder = b
			return TypeWord
		case "int":
			TypeInt.Builder = b
			return TypeInt
		case "string":
			return tm.astToIRType(&ast.IndexExpression{
				Left:    &ast.Identifier{Value: "slice"},
				Indices: []ast.Expression{&ast.Identifier{Value: "byte"}},
			})
		case "const_integer":
			TypeConstInteger.Builder = b
			return TypeConstInteger
		default:
			var qname string
			fullName := e.FullName()
			if strings.Contains(fullName, ".") {
				qname = fullName
			} else {
				qname = b.currentPackage + "." + fullName
			}
			if _, ok := tm.typeDefsAST[qname]; !ok {
				if _, okAlias := tm.typeAliases[qname]; !okAlias {
					if _, ok := tm.typeDefsAST["prelude."+fullName]; ok {
						qname = "prelude." + fullName
					} else if _, okAlias := tm.typeAliases["prelude."+fullName]; okAlias {
						qname = "prelude." + fullName
					} else if _, ok := tm.typeAliases[fullName]; ok {
						qname = fullName
					}
				}
			}

			if aliasExpr, ok := tm.typeAliases[qname]; ok {
				if strings.HasPrefix(qname, "func_ptr_") {
					return Type{Expr: aliasExpr, Name: qname, Bits: TypeBitFuncPtr, Builder: b}
				}
				return tm.astToIRType(aliasExpr)
			}

			if _, ok := tm.typeDefsAST[qname]; ok {
				bits := uint(0)
				if qname == "prelude.any" || qname == "any" {
					bits = TypeBitAny
				}
				return Type{Expr: expr, Name: qname, Bits: bits, Builder: b}
			}
			panic("unresolved:" + qname)
		}
	case *ast.SelectorExpression:
		// We shouldn't hit package lookups here anymore since ResolveNames collapsed them!
		// If we hit it, it's a field lookup type? MiniGolf doesn't support nested structs by selector yet.
		return TypeWord
	case *ast.IndexExpression:
		var rawGenericName string
		if ident, ok := e.Left.(*ast.Identifier); ok {
			rawGenericName = ident.FullName()
		}

		if rawGenericName != "" {
			var instTypStr string
			for _, idx := range e.Indices {
				instTypStr += "_" + tm.astToIRType(idx).Name
			}
			instName := fmt.Sprintf("%s%s", rawGenericName, instTypStr)

			if _, ok := tm.typeDefsAST[instName]; !ok {
				if tmpl, ok := tm.genericTemplates[rawGenericName]; ok {
					tm.instantiateGeneric(instName, rawGenericName, e.Indices, tmpl, e.GetToken())
				} else if ident, ok := e.Left.(*ast.Identifier); ok {
					builtinRawGenericName := "prelude." + ident.Value
					if tmpl, ok := tm.genericTemplates[builtinRawGenericName]; ok {
						rawGenericName = builtinRawGenericName
						instName = fmt.Sprintf("%s%s", rawGenericName, instTypStr)
						if _, ok := tm.typeDefsAST[instName]; !ok {
							tm.instantiateGeneric(instName, rawGenericName, e.Indices, tmpl, e.GetToken())
						}
					} else {
						panic("unresolved:" + rawGenericName)
					}
				} else {
					panic("unresolved:" + rawGenericName)
				}
			} else {
				// Type was already instantiated by semantic phase, but we must populate instantiatedTypes
				var argTyps []Type
				for _, argNode := range e.Indices {
					argTyps = append(argTyps, tm.astToIRType(argNode))
				}
				tm.instantiatedTypes[instName] = InstantiatedTypeInfo{
					RawGenericName: rawGenericName,
					ArgTyps:        argTyps,
				}
			}
			resTyp := Type{Expr: expr, Name: instName, Builder: b}
			// Detect slices and set Bits + ElementType
			baseName := rawGenericName
			if strings.HasSuffix(baseName, ".slice") || baseName == "slice" || baseName == "prelude.slice" {
				resTyp.Bits |= TypeBitSlice
				if info, ok := tm.instantiatedTypes[instName]; ok && len(info.ArgTyps) > 0 {
					elt := info.ArgTyps[0]
					resTyp.ElementType = &elt
				}
			}
			return resTyp
		}
		return TypeWord
	case *ast.ArrayType:
		// nando-GOOD
		lenVal := b.EvalConst(e.Length)
		eltType := tm.astToIRType(e.Elt)
		return Type{Expr: expr, Name: fmt.Sprintf("[%d]%s", lenVal, eltType.Name), Bits: TypeBitArray, ElementType: &eltType, ArrayLen: int(lenVal), Builder: b}
	case *ast.PointerType:
		//zach//fmt.Printf("EVALUATING POINTER_TYPE, CheckNil=%v\n", b.CheckNil)
		eltType := tm.astToIRType(e.Elt)
		return Type{Expr: expr, Name: "*" + eltType.Name, Bits: TypeBitPointer, PointsToType: &eltType, Builder: b}
	case *ast.StructType:
		name := "struct{"
		var fields []NameAndType
		for i, f := range e.Fields {
			fTyp := tm.astToIRType(f.Type)
			name += fTyp.Name + ";"
			fieldName := ""
			if f.Name != nil {
				fieldName = f.Name.ShortName
			}
			fields = append(fields, NameAndType{Name: fieldName, Type: fTyp, FieldIndex: i})
		}
		name += "}"
		return Type{Expr: expr, Name: name, Bits: TypeBitStruct, FieldNamesAndTypes: fields, Builder: b}

	case *ast.CompositeLit:
		return tm.astToIRType(e.Type)
	case *ast.FuncType:
		synName := tm.SyntheticFuncName(e)
		var params []NameAndType
		for i, p := range e.Parameters {
			pName := ""
			if p.Name != nil {
				pName = p.Name.Value
			}
			params = append(params, NameAndType{Name: pName, Type: tm.astToIRType(p.Type), FieldIndex: i})
		}
		var rets []NameAndType
		for i, r := range e.ReturnParameters {
			rName := ""
			if r.Name != nil {
				rName = r.Name.Value
			}
			rets = append(rets, NameAndType{Name: rName, Type: tm.astToIRType(r.Type), FieldIndex: i})
		}
		return Type{Expr: expr, Name: synName, Bits: TypeBitFuncPtr, ParameterNamesAndTypes: params, ReturnNamesAndTypes: rets, Builder: b}
	}
	log.Panicf("astToIRType NO CASE: %#v", expr)
	panic(0)
}

func (tm *TypeManager) SyntheticFuncName(e *ast.FuncType) string {
	b := tm.builder
	name := "func_ptr_"
	for i, param := range e.Parameters {
		if i > 0 {
			name += "_"
		}
		name += tm.astToIRType(param.Type).Name
	}
	name += "__"
	for i, rt := range e.ReturnParameters {
		if i > 0 {
			name += "_"
		}
		name += tm.astToIRType(rt.Type).Name
	}
	// Sanitize by replacing any non-alphanumeric with underscores just in case
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "[", "_")
	name = strings.ReplaceAll(name, "]", "_")
	name = strings.ReplaceAll(name, "*", "ptr_")

	tm.typeAliases[name] = e
	_ = b
	return name
}

func (tm *TypeManager) substituteGenericTokens(argTyps []Type, tmpl *GenericTemplate, instantiateToken *token.Token, instName string) []token.Token {
	var argTokensList [][]token.Token
	for _, argTyp := range argTyps {

		// Use TypeName for type in expansion
		argTokens := lexer.Lex(argTyp.TypeName(), "generic_inst")

		// Trim EOF
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.EOF {
			argTokens = argTokens[:len(argTokens)-1]
		}
		// Trim SEMICOLON
		if len(argTokens) > 0 && argTokens[len(argTokens)-1].Type == token.SEMICOLON {
			argTokens = argTokens[:len(argTokens)-1]
		}

		// Wrap multi-token type arguments in parentheses when the first token
		// is a pointer star, to prevent parser ambiguity (e.g., *byte must not
		// merge with adjacent * tokens). Don't wrap other multi-token types
		// like qualified names (prelude.slice_byte) since parens break var decls.
		if len(argTokens) > 1 && argTokens[0].Type == token.ASTERISK {
			wrapped := []token.Token{{Type: token.LPAREN, Literal: "("}}
			wrapped = append(wrapped, argTokens...)
			wrapped = append(wrapped, token.Token{Type: token.RPAREN, Literal: ")"})
			argTokens = wrapped
		}

		// and add it to the list
		for i := range argTokens {
			if instantiateToken != nil {
				argTokens[i].ExpandedFrom = fmt.Sprintf("expanded %s at %s:%d", instName, instantiateToken.Filename, instantiateToken.Line)
			}
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
			tokCopy := tok
			if instantiateToken != nil {
				tokCopy.ExpandedFrom = fmt.Sprintf("expanded %s at %s:%d", instName, instantiateToken.Filename, instantiateToken.Line)
			}
			newTokens = append(newTokens, tokCopy)
		}
	}
	newTokens = append(newTokens, token.Token{Type: token.EOF, Literal: ""})
	return newTokens
}

func (tm *TypeManager) instantiateGeneric(instName, genericName string, argNodes []ast.Expression, tmpl *GenericTemplate, instantiateToken *token.Token) {
	b := tm.builder
	var argTyps []Type
	for _, argNode := range argNodes {
		argTyps = append(argTyps, tm.astToIRType(argNode))
	}

	tm.instantiatedTypes[instName] = InstantiatedTypeInfo{
		RawGenericName: genericName,
		ArgTyps:        argTyps,
	}

	newTokens := tm.substituteGenericTokens(argTyps, tmpl, instantiateToken, instName)

	p := parser.New(newTokens)
	stmt := p.ParseStatementForGeneric()

	if len(p.Errors()) > 0 {
		fmt.Printf("Parser errors during generic instantiation of %s:\n", instName)
		for _, msg := range p.Errors() {
			fmt.Println("\t", msg)
		}
	}

	parts := strings.SplitN(genericName, ".", 2)
	defPkg := parts[0]
	if b.resolveCallback != nil {
		stmt = b.resolveCallback(stmt, defPkg).(ast.Statement)
	}

	ts, ok := stmt.(*ast.TypeStatement)
	if !ok {
		panic("Generic instantiation did not produce a TypeStatement: " + instName)
	}
	baseTypeAST := ts.BaseType

	if st, ok := baseTypeAST.(*ast.StructType); ok {
		tm.typeDefsAST[instName] = st
		b.Program.TypeDefOrder = append(b.Program.TypeDefOrder, instName)

		var fields []*ast.Field
		for i, f := range st.Fields {
			fields = append(fields, &ast.Field{
				Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
				Type: f.Type,
			})
		}
		structType := tm.astToIRType(&ast.StructType{
			Fields: fields,
		})
		b.Program.TypeDefs[instName] = structType
	} else {
		panic("Generic instantiation did not produce a struct: " + instName)
	}
}

func (tm *TypeManager) getTypeString(qname string) Type {
	b := tm.builder
	// NANDO-recent.
	if res, ok := b.Program.TypeDefs[qname]; ok {
		return res
	}
	if tm.evaluatingType[qname] {
		panic("circular dependency in type definition: " + qname)
	}
	st, ok := tm.typeDefsAST[qname]
	if !ok {
		panic("unknown type: " + qname)
	}
	tm.evaluatingType[qname] = true
	defer func() { tm.evaluatingType[qname] = false }()

	var fields []*ast.Field
	for i, f := range st.Fields {
		fields = append(fields, &ast.Field{
			Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
			Type: f.Type,
		})
	}
	structType := tm.astToIRType(&ast.StructType{
		Fields: fields,
	})
	b.Program.TypeDefs[qname] = structType
	return structType
}

func (tm *TypeManager) getTypeSize(typ Type) int {
	b := tm.builder
	// NANDO-recent.
	if typ.Equals(TypeVoid) || typ.Equals(TypeByte) || typ.Equals(TypeBool) {
		return 1
	}
	if typ.Equals(TypeWord) || typ.Equals(TypeInt) {
		return b.WordSize
	}
	if typ.IsAPointer() {
		return b.WordSize
	}
	if typ.IsAnArray() {
		idx := strings.Index(typ.Name, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ.Name[1:idx])
			eltSize := tm.getTypeSize(typ.ArrayElementType())
			return length * eltSize
		}
	}
	if !typ.IsAStruct() {
		if _, ok := tm.typeDefsAST[typ.Name]; ok {
			typ = tm.getTypeString(typ.Name)
		} else if def, ok := b.Program.TypeDefs[typ.Name]; ok {
			typ = def
		} else {
			return 2
		}
	}
	if typ.IsAStruct() {
		size := 0
		st := typ.Expr.(*ast.StructType)
		for _, f := range st.Fields {
			size += tm.getTypeSize(tm.astToIRType(f.Type))
		}
		return size
	}
	log.Panicf("getTypesize bad %v", typ)
	panic(0)
}

func (tm *TypeManager) extractTypeParamsIR(paramType ast.Expression, argTyp Type, typeMap map[string]Type, typeParams []string) {
	if ident, ok := paramType.(*ast.Identifier); ok {
		for _, tp := range typeParams {
			if tp == ident.Value {
				typeMap[tp] = argTyp
				return
			}
		}
	} else if prefix, ok := paramType.(*ast.PrefixExpression); ok && prefix.Operator == "*" {
		if argTyp.IsAPointer() {
			tm.extractTypeParamsIR(prefix.Right, argTyp.PointedType(), typeMap, typeParams)
		}
	} else if ptr, ok := paramType.(*ast.PointerType); ok {
		if argTyp.IsAPointer() {
			tm.extractTypeParamsIR(ptr.Elt, argTyp.PointedType(), typeMap, typeParams)
		}
	} else if idx, ok := paramType.(*ast.IndexExpression); ok {
		// nando-BAD.  Spliting on _ cannot work.
		parts := strings.Split(argTyp.Name, "_")
		numIdx := len(idx.Indices)
		if len(parts) >= numIdx {
			for i, innerIdx := range idx.Indices {
				tm.extractTypeParamsIR(innerIdx, Type{Expr: &ast.Identifier{Value: parts[len(parts)-numIdx+i]}, Name: parts[len(parts)-numIdx+i], Builder: tm.builder}, typeMap, typeParams)
			}
		}
	}
}

func (tm *TypeManager) getFuncReturnType(returnParams []*ast.Parameter) Type {
	b := tm.builder
	if len(returnParams) == 1 {
		return tm.astToIRType(returnParams[0].Type)
	} else if len(returnParams) > 1 {
		var fields []*ast.Field
		for i, rt := range returnParams {
			fields = append(fields, &ast.Field{
				Name: &ast.Identifier{Value: fmt.Sprintf("f%d", i)},
				Type: rt.Type,
			})
		}
		structTyp := &ast.StructType{Fields: fields}
		name := "struct{"
		for _, rt := range returnParams {
			name += tm.astToIRType(rt.Type).Name + ";"
		}
		name += "}"
		return Type{Expr: structTyp, Name: name, Builder: b}
	} else {
		return TypeVoid
	}
}
