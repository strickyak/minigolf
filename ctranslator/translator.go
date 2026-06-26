// Package ctranslator converts C99 source to MiniGolf source using the
// modernc.org/cc/v5 typed AST. It is the library form of the cc_to_golf tool.
package ctranslator

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	cc "modernc.org/cc/v5"
)

// Options controls the C→MiniGolf translation.
type Options struct {
	// IncludePaths are searched for #include "..." directives in order.
	// The last entry is also used for #include <...> (system includes).
	IncludePaths []string

	// Defines maps C preprocessor macro names (without a dot) to string values.
	// Each entry is injected as a "#define NAME value" line before the source.
	Defines map[string]string

	// KeepGoing, when true, emits "/* UNSUPPORTED: ... */" comments instead of
	// panic() calls for unsupported C constructs.
	KeepGoing bool
}

// TranslateFile translates the C source file at cFile and returns the
// MiniGolf source as a string. Warnings from the C front-end are returned
// together with any non-nil error.
func TranslateFile(cFile string, opts Options) (string, error) {
	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", fmt.Errorf("cc.NewConfig: %w", err)
	}
	cfg.Predefined += cc.Builtin

	// Apply user include paths.
	if len(opts.IncludePaths) > 0 {
		cfg.IncludePaths = append([]string{"@"}, opts.IncludePaths...)
		// Last -I directory serves as the system include path for <...>.
		cfg.SysIncludePaths = []string{opts.IncludePaths[len(opts.IncludePaths)-1]}
	}

	// Build sources list: predefined macros, optional -D defines, then the C file.
	var extraDefs strings.Builder
	for name, val := range opts.Defines {
		if val == "" {
			fmt.Fprintf(&extraDefs, "#define %s\n", name)
		} else {
			fmt.Fprintf(&extraDefs, "#define %s %s\n", name, val)
		}
	}

	sources := []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
	}
	if extraDefs.Len() > 0 {
		sources = append(sources, cc.Source{Name: "<defines>", Value: extraDefs.String()})
	}
	sources = append(sources, cc.Source{Name: cFile})

	ast, err := cc.Translate(cfg, sources)
	if ast == nil {
		// cc.Translate returns nil AST only on fatal errors (missing includes,
		// severe syntax errors, etc.). The error IS the diagnosis.
		if err != nil {
			return "", fmt.Errorf("C frontend: %w", err)
		}
		return "", fmt.Errorf("C frontend: cc.Translate returned nil AST (no error details available)")
	}
	// Non-nil AST with an error means a recoverable warning (e.g. unknown
	// __attribute__). Translation can proceed.
	var warning error
	if err != nil {
		warning = fmt.Errorf("warning: %w", err)
	}

	tr := newTranslator(opts.KeepGoing)
	tr.translateProgram(ast)
	return tr.out.String(), warning
}

// ─────────────────────────────────────────────────────────────────────────────
// Translator state
// ─────────────────────────────────────────────────────────────────────────────

type staticVar struct {
	golfName string
	golfType string
}

// arrStrInit records a C global char array initialised from a string literal.
// We can't emit `var name [N]byte = "..."` in Golf (type mismatch: [N]byte vs
// slice[byte]), so we defer the bytes to a generated func init().
type arrStrInit struct {
	varName string
	nBytes  int    // total bytes to copy (array length, includes NUL)
	strLit  string // the Golf string literal, e.g. `"0123456789ABCDEF"`
}

type translator struct {
	out   strings.Builder
	depth int

	// typedef C-name → MiniGolf type string
	typedefMap map[string]string

	// struct tag → MiniGolf struct type name (e.g. "bin" → "Bin")
	structTagMap map[string]string

	// emitted struct types (to avoid duplicate definitions)
	emittedStructs map[string]bool

	// static local vars extracted to globals
	staticVars []staticVar

	// already-emitted global names
	emittedGlobals map[string]bool

	// temp-var counter (for ternary extraction)
	tempCount int

	// current function name (for static-var naming)
	curFunc string

	// va_list variable name in the current variadic function (e.g. "ap")
	curVaName string

	// static locals: local name → mangled global name for current function
	staticNameMap map[string]string
	// static array globals: global name → element pointer type (e.g. "*byte")
	// used to emit the array-to-pointer cast when the name is referenced.
	staticArrayMap map[string]string

	// keepGoing controls error handling for unsupported C constructs.
	// When true, emit /* UNSUPPORTED: ... */ comments; when false, emit panic().
	keepGoing bool

	// Deferred string-to-array initializations emitted into func init().
	arrStrInits []arrStrInit
}

func newTranslator(keepGoing bool) *translator {
	return &translator{
		typedefMap:     make(map[string]string),
		structTagMap:   make(map[string]string),
		emittedStructs: make(map[string]bool),
		emittedGlobals: make(map[string]bool),
		staticNameMap:  make(map[string]string),
		staticArrayMap: make(map[string]string),
		keepGoing:      keepGoing,
	}
}

// ── Output helpers ────────────────────────────────────────────────────────────

func (t *translator) line(format string, args ...interface{}) {
	fmt.Fprintf(&t.out, "%s%s\n",
		strings.Repeat("    ", t.depth),
		fmt.Sprintf(format, args...))
}

func (t *translator) raw(s string) { t.out.WriteString(s) }

// unsupported emits either a panic or a comment depending on -k.
func (t *translator) unsupported(reason string) string {
	if t.keepGoing {
		return fmt.Sprintf("/* UNSUPPORTED: %s */", reason)
	}
	return fmt.Sprintf(`panic(%q)`, "unsupported: "+reason)
}

func (t *translator) tempVar() string {
	t.tempCount++
	return fmt.Sprintf("_t%d", t.tempCount)
}

// ─────────────────────────────────────────────────────────────────────────────
// Type resolution
// ─────────────────────────────────────────────────────────────────────────────

// cTypeToGolf converts a cc.Type to a MiniGolf type string.
func (t *translator) cTypeToGolf(typ cc.Type) string {
	if typ == nil {
		return "/* nil */"
	}
	// If this type has a typedef name, consult our map first.
	if td := typ.Typedef(); td != nil {
		tdName := td.Name()
		// va_list (and its platform aliases) → slice[any]
		if tdName == "va_list" || tdName == "__gnuc_va_list" || tdName == "__builtin_va_list" {
			return "slice[any]"
		}
		if gname, ok := t.typedefMap[tdName]; ok {
			return gname
		}
	}
	switch typ.Kind() {
	case cc.Void:
		return ""
	case cc.Bool:
		return "byte"
	case cc.Char, cc.SChar, cc.UChar:
		return "byte"
	case cc.Int, cc.Short, cc.Long, cc.LongLong,
		cc.Int8, cc.Int16, cc.Int32, cc.Int64:
		return "int"
	case cc.UInt, cc.UShort, cc.ULong, cc.ULongLong,
		cc.UInt8, cc.UInt16, cc.UInt32, cc.UInt64:
		return "word"
	case cc.Ptr:
		pt, ok := typ.(*cc.PointerType)
		if !ok {
			return "*byte"
		}
		elem := pt.Elem()
		if elem.Kind() == cc.Void {
			return "*byte"
		}
		return "*" + t.cTypeToGolf(elem)
	case cc.Array:
		at, ok := typ.(*cc.ArrayType)
		if !ok {
			return t.unsupported("unknown array")
		}
		n := at.Len()
		elem := t.cTypeToGolf(at.Elem())
		return fmt.Sprintf("[%d]%s", n, elem)
	case cc.Struct:
		st, ok := typ.(*cc.StructType)
		if !ok {
			return t.unsupported("unknown struct")
		}
		tag := tokenStr(st.Tag())
		if gname, ok := t.structTagMap[tag]; ok {
			return gname
		}
		return t.unsupported("unnamed/unregistered struct " + tag)
	case cc.Union:
		return t.unsupported("union type")
	case cc.Function:
		return t.unsupported("function type in field/var")
	default:
		return fmt.Sprintf("/* %s */", typ.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 1: pre-scan for typedefs and struct definitions
// ─────────────────────────────────────────────────────────────────────────────

func (t *translator) prescan(ast *cc.AST) {
	for _, d := range ast.Declarations {
		cd, ok := d.(*cc.CommonDeclaration)
		if !ok {
			continue
		}
		if isBuiltinDecl(cd) {
			continue
		}
		for _, id := range cd.InitDeclarators {
			decl := id.Declarator
			if !decl.IsTypename() {
				continue
			}
			name := decl.Name()
			typ := decl.Type()

			// typedef of pointer-to-struct → define struct, map name → *Name
			if typ.Kind() == cc.Ptr {
				elem := stripPtr(typ)
				if elem.Kind() == cc.Struct {
					st := elem.(*cc.StructType)
					tag := tokenStr(st.Tag())
					if _, exists := t.structTagMap[tag]; !exists && tag != "" {
						t.structTagMap[tag] = name
					}
					t.typedefMap[name] = "*" + name
					continue
				}
			}
			// typedef of struct by value → define struct, map name → Name
			if typ.Kind() == cc.Struct {
				st := typ.(*cc.StructType)
				tag := tokenStr(st.Tag())
				if tag == "" {
					// Anonymous struct: use the typedef name as the key.
					tag = name
				}
				if _, exists := t.structTagMap[tag]; !exists {
					t.structTagMap[tag] = name
				}
				t.typedefMap[name] = name
				continue
			}
			// Primitive typedef
			golf := t.cTypeToGolf(typ)
			t.typedefMap[name] = golf
		}

		// Also scan bare struct declarations like `struct bin q;`
		for _, spec := range cd.DeclarationSpecifiers {
			if ts, ok2 := spec.(*cc.TypeSpecStructOrUnion); ok2 {
				sou := ts.StructOrUnion
				if sou == nil {
					continue
				}
				if st2, ok3 := sou.Type().(*cc.StructType); ok3 {
					tag := tokenStr(st2.Tag())
					if tag != "" {
						if _, exists := t.structTagMap[tag]; !exists {
							// Use capitalised tag as the name.
							t.structTagMap[tag] = capitalise(tag)
						}
					}
				}
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 2: emission
// ─────────────────────────────────────────────────────────────────────────────

func (t *translator) translateProgram(ast *cc.AST) {
	t.prescan(ast)

	t.raw("package main\n")

	// Emit struct type definitions (in declaration order).
	t.emitStructDefs(ast)

	// Emit static globals collected during function translation.
	// (They will be filled in during function translation below.)

	// Translate all top-level items in source order.
	for _, d := range ast.Declarations {
		switch x := d.(type) {
		case *cc.FunctionDefinition:
			if isBuiltin(x.Declarator) {
				continue
			}
			// Emit any accumulated static globals before this function.
			t.flushStaticVars()
			t.translateFuncDef(x)
		case *cc.CommonDeclaration:
			if isBuiltinDecl(x) {
				continue
			}
			t.translateTopLevelDecl(x)
		}
	}
	t.flushStaticVars()
	// Emit a func init() for any global arrays that were initialized from
	// C string literals (which can't be expressed as var [N]byte = "...").
	if len(t.arrStrInits) > 0 {
		t.raw("\nfunc init() {\n")
		for i, asi := range t.arrStrInits {
			srcVar := fmt.Sprintf("_asinit_src%d", i)
			dstVar := fmt.Sprintf("_asinit_dst%d", i)
			nVar := fmt.Sprintf("_asinit_n%d", i)
			t.raw(fmt.Sprintf("    %s := (*byte)(%s)\n", srcVar, asi.strLit))
			t.raw(fmt.Sprintf("    %s := (*byte)(%s)\n", dstVar, asi.varName))
			t.raw(fmt.Sprintf("    for %s := word(0); %s < word(%d); %s++ {\n", nVar, nVar, asi.nBytes, nVar))
			t.raw(fmt.Sprintf("        %s[%s] = %s[%s]\n", dstVar, nVar, srcVar, nVar))
			t.raw("    }\n")
		}
		t.raw("}\n")
	}
	// Note: the caller (TranslateFile) returns t.out.String().
}

func (t *translator) flushStaticVars() {
	for _, sv := range t.staticVars {
		t.raw(fmt.Sprintf("\nvar %s %s\n", sv.golfName, sv.golfType))
	}
	t.staticVars = nil
}

// emitStructDefs emits MiniGolf type definitions for every struct we know about.
func (t *translator) emitStructDefs(ast *cc.AST) {
	// Walk declarations in order so structs appear before functions that use them.
	for _, d := range ast.Declarations {
		cd, ok := d.(*cc.CommonDeclaration)
		if !ok {
			continue
		}
		if isBuiltinDecl(cd) {
			continue
		}
		// Check each specifier for an inline struct definition.
		for _, spec := range cd.DeclarationSpecifiers {
			ts, ok2 := spec.(*cc.TypeSpecStructOrUnion)
			if !ok2 {
				continue
			}
			sou := ts.StructOrUnion
			if sou == nil {
				continue
			}
			st, ok3 := sou.Type().(*cc.StructType)
			if !ok3 || st.IsIncomplete() || st.NumFields() == 0 {
				continue
			}
			tag := tokenStr(st.Tag())
			golfName, exists := t.structTagMap[tag]
			if !exists {
				continue
			}
			if t.emittedStructs[golfName] {
				continue
			}
			t.emitStructType(golfName, st)
		}
		// Also check typedef-pointed struct types (handles anonymous structs too).
		for _, id := range cd.InitDeclarators {
			decl := id.Declarator
			if !decl.IsTypename() {
				continue
			}
			typ := decl.Type()
			elem := stripPtr(typ)
			if elem.Kind() != cc.Struct {
				continue
			}
			st, ok2 := elem.(*cc.StructType)
			if !ok2 || st.IsIncomplete() || st.NumFields() == 0 {
				continue
			}
			tag := tokenStr(st.Tag())
			if tag == "" {
				// Anonymous struct: prescan registered it under the typedef name.
				tag = decl.Name()
			}
			golfName, exists := t.structTagMap[tag]
			if !exists {
				continue
			}
			if t.emittedStructs[golfName] {
				continue
			}
			t.emitStructType(golfName, st)
		}
	}
}

func (t *translator) emitStructType(golfName string, st *cc.StructType) {
	t.emittedStructs[golfName] = true
	t.raw("\n")
	t.raw(fmt.Sprintf("type %s struct {\n", golfName))
	for i := 0; i < st.NumFields(); i++ {
		f := st.FieldByIndex(i)
		if f == nil || f.Name() == "" {
			continue
		}
		golfType := t.cTypeToGolf(f.Type())
		t.raw(fmt.Sprintf("    %s %s\n", f.Name(), golfType))
	}
	t.raw("}\n")
}

// ── Top-level declarations ────────────────────────────────────────────────────

func (t *translator) translateTopLevelDecl(cd *cc.CommonDeclaration) {
	for _, id := range cd.InitDeclarators {
		decl := id.Declarator
		name := decl.Name()
		if name == "" || strings.HasPrefix(name, "__") {
			continue
		}
		if decl.IsTypename() {
			// Emit a comment for non-struct primitive typedefs.
			if golfT, ok := t.typedefMap[name]; ok {
				if !strings.Contains(golfT, "UNSUPPORTED") && !strings.HasPrefix(golfT, "*") {
					t.raw(fmt.Sprintf("\n// typedef %s = %s\n", name, golfT))
				}
			}
			continue
		}
		typ := decl.Type()
		if typ.Kind() == cc.Function {
			// Forward declaration / prototype — emit as comment.
			t.emitProtoComment(decl)
			continue
		}
		if t.emittedGlobals[name] {
			continue
		}
		t.emittedGlobals[name] = true
		golfType := t.cTypeToGolf(typ)
		if id.Initializer != nil {
			initStr := t.xExpr(id.Initializer.Expression)
			// Detect [N]byte = "string" — type mismatch in Golf ([N]byte vs
			// slice[byte]). Emit a bare var and defer copy to func init().
			if strings.HasPrefix(golfType, "[") && strings.HasSuffix(golfType, "byte") &&
				len(initStr) >= 2 && initStr[0] == '"' {
				// Parse array length from golfType "[N]byte".
				close := strings.Index(golfType, "]")
				nBytes := 0
				if close > 1 {
					fmt.Sscanf(golfType[1:close], "%d", &nBytes)
				}
				t.raw(fmt.Sprintf("\nvar %s %s\n", name, golfType))
				t.arrStrInits = append(t.arrStrInits, arrStrInit{
					varName: name,
					nBytes:  nBytes,
					strLit:  initStr,
				})
			} else {
				t.raw(fmt.Sprintf("\nvar %s %s = %s\n", name, golfType, initStr))
			}
		} else {
			t.raw(fmt.Sprintf("\nvar %s %s\n", name, golfType))
		}
	}
}

func (t *translator) emitProtoComment(decl *cc.Declarator) {
	if isBuiltin(decl) {
		return
	}
	name := decl.Name()
	ft, ok := decl.Type().(*cc.FunctionType)
	if !ok {
		return
	}
	t.raw(fmt.Sprintf("\n// func %s\n", t.funcSig(name, ft)))
}

// ── Function definition ───────────────────────────────────────────────────────

func (t *translator) translateFuncDef(f *cc.FunctionDefinition) {
	decl := f.Declarator
	name := decl.Name()
	ft, ok := decl.Type().(*cc.FunctionType)
	if !ok {
		t.raw(fmt.Sprintf("\n// skipping %s: not a function type\n", name))
		return
	}

	prev := t.curFunc
	t.curFunc = name
	prevVaName := t.curVaName
	t.curVaName = t.vaListName(f.Body)
	// Reset static-name substitution map for this function.
	prevStaticMap := t.staticNameMap
	t.staticNameMap = make(map[string]string)

	sig := t.funcSig(name, ft)
	t.raw(fmt.Sprintf("\nfunc %s {\n", sig))
	t.depth++
	t.translateCompound(f.Body)
	t.depth--
	t.raw("}\n")

	t.curFunc = prev
	t.curVaName = prevVaName
	t.staticNameMap = prevStaticMap
}

func (t *translator) funcSig(name string, ft *cc.FunctionType) string {
	var params []string
	for _, p := range ft.Parameters() {
		pname := p.Name()
		ptype := t.cTypeToGolf(p.Type())
		if ptype == "" {
			continue // void
		}
		if pname == "" {
			params = append(params, ptype)
		} else {
			params = append(params, pname+" "+ptype)
		}
	}
	if ft.IsVariadic() {
		vaName := t.curVaName
		if vaName == "" {
			vaName = "_"
		}
		params = append(params, vaName+" ...any")
	}
	ret := ""
	if ft.Result().Kind() != cc.Void {
		if !(name == "main" && ft.Result().Kind() == cc.Int) {
			ret = " " + t.cTypeToGolf(ft.Result())
		}
	}
	return fmt.Sprintf("%s(%s)%s", name, strings.Join(params, ", "), ret)
}

// ─────────────────────────────────────────────────────────────────────────────
// Statement translation
// ─────────────────────────────────────────────────────────────────────────────

func (t *translator) translateCompound(cs *cc.CompoundStatement) {
	if cs == nil {
		return
	}
	for _, item := range cs.List {
		t.translateBlockItem(item)
	}
}

func (t *translator) translateBlockItem(item cc.BlockItem) {
	switch x := item.(type) {
	case *cc.CommonDeclaration:
		t.translateLocalDecl(x)
	case *cc.ExpressionStatement:
		t.translateExprStmt(x)
	case *cc.IterationStatement:
		t.translateIteration(x)
	case *cc.SelectionStatement:
		t.translateSelection(x)
	case *cc.JumpStatement:
		t.translateJump(x)
	case *cc.LabeledStatement:
		t.translateLabeled(x)
	case *cc.CompoundStatement:
		t.translateCompound(x)
	case *cc.AsmStatement:
		t.line("%s", t.unsupported("asm statement"))
	default:
		t.line("%s", t.unsupported(fmt.Sprintf("block item %T", item)))
	}
}

func (t *translator) translateStatement(stmt cc.Statement) {
	switch x := stmt.(type) {
	case *cc.CompoundStatement:
		t.translateCompound(x)
	case *cc.ExpressionStatement:
		t.translateExprStmt(x)
	case *cc.IterationStatement:
		t.translateIteration(x)
	case *cc.SelectionStatement:
		t.translateSelection(x)
	case *cc.JumpStatement:
		t.translateJump(x)
	case *cc.LabeledStatement:
		t.translateLabeled(x)
	case *cc.AsmStatement:
		t.line("%s", t.unsupported("asm statement"))
	default:
		t.line("%s", t.unsupported(fmt.Sprintf("statement %T", stmt)))
	}
}

// translateBody emits the contents of a braced block statement.
// The opening/closing braces are emitted by the caller.
func (t *translator) translateBody(stmt cc.Statement) {
	if cs, ok := stmt.(*cc.CompoundStatement); ok {
		t.translateCompound(cs)
	} else {
		t.depth++
		t.translateStatement(stmt)
		t.depth--
	}
}

// ── Expression statement ──────────────────────────────────────────────────────

func (t *translator) translateExprStmt(s *cc.ExpressionStatement) {
	if s == nil || s.ExpressionList == nil {
		return
	}
	// Comma-expression list at statement level → split into separate lines.
	if el, ok := s.ExpressionList.(*cc.ExpressionList); ok && len(el.List) > 1 {
		for _, e := range el.List {
			t.translateExprStmtOne(e)
		}
		return
	}
	t.translateExprStmtOne(s.ExpressionList)
}

// translateExprStmtOne emits a single expression as a statement, with special
// handling for pointer arithmetic patterns that MiniGolf cannot express directly.
func (t *translator) translateExprStmtOne(e cc.Expression) {
	// Pattern: *p++ = expr  (assign to dereferenced pointer, then advance pointer)
	// Detect AssignmentExpression where LHS is UnaryExpr(Deref, PostfixExpr(++))
	if asgn, ok := e.(*cc.AssignmentExpression); ok && asgn.Op == cc.AssignmentOperationAssign {
		if unary, ok := asgn.Lhs.(*cc.UnaryExpr); ok && unary.Case == cc.UnaryExpressionDeref {
			if postfix, ok := unary.Expr.(*cc.PostfixExpr); ok && !postfix.Dec {
				// This is *(p++) = rhs → split into: *p = rhs ; p = (*T)(word(p)+1)
				pStr := t.xExpr(postfix.Expr)
				rhsStr := t.xExpr(asgn.Rhs)
				golfType := t.cTypeToGolf(postfix.Expr.Type())
				// Narrow the RHS to byte if the destination is *byte and the
				// source expression is wider (int or word). In C, char literals
				// and arithmetic promote to int; storing into a *char needs
				// truncation. In Golf, char literals are const_integer and adapt
				// to context, but explicit int(...) casts in the RHS force int type.
				if golfType == "*byte" {
					rhsGolf := t.cTypeToGolf(asgn.Rhs.Type())
					if rhsGolf == "int" || rhsGolf == "word" {
						rhsStr = "byte(" + rhsStr + ")"
					}
				}
				t.line("*%s = %s", pStr, rhsStr)
				t.line("%s = (%s)(word(%s) + 1)", pStr, golfType, pStr)
				return
			}
		}
	}
	// Pattern: p++  or  p--  where p is a pointer type
	// Emit:  p = (*T)(word(p) ± 1)   instead of bare p++/p--
	if postfix, ok := e.(*cc.PostfixExpr); ok {
		if postfix.Expr.Type().Kind() == cc.Ptr {
			base := t.xExpr(postfix.Expr)
			golfType := t.cTypeToGolf(postfix.Expr.Type())
			if postfix.Dec {
				t.line("%s = (%s)(word(%s) - 1)", base, golfType, base)
			} else {
				t.line("%s = (%s)(word(%s) + 1)", base, golfType, base)
			}
			return
		}
	}
	// Pattern: ++p  or  --p  where p is a pointer type
	if prefix, ok := e.(*cc.PrefixExpr); ok {
		if prefix.Expr.Type().Kind() == cc.Ptr {
			base := t.xExpr(prefix.Expr)
			golfType := t.cTypeToGolf(prefix.Expr.Type())
			if prefix.Dec {
				t.line("%s = (%s)(word(%s) - 1)", base, golfType, base)
			} else {
				t.line("%s = (%s)(word(%s) + 1)", base, golfType, base)
			}
			return
		}
	}
	result := t.xExpr(e)
	if result != "" {
		t.line("%s", result)
	}
}

// ── Local declarations ────────────────────────────────────────────────────────

func (t *translator) translateLocalDecl(cd *cc.CommonDeclaration) {
	isStatic := false
	for _, spec := range cd.DeclarationSpecifiers {
		if sc, ok := spec.(*cc.StorageClassSpecifier); ok {
			if sc.Case == cc.StorageClassSpecifierStatic {
				isStatic = true
			}
		}
	}

	for _, id := range cd.InitDeclarators {
		decl := id.Declarator
		name := decl.Name()
		if name == "" || strings.HasPrefix(name, "__") {
			continue
		}
		if decl.IsTypename() {
			continue
		}
		typ := decl.Type()
		// va_list declarations are no-ops: the variable is already a parameter
		// (named by curVaName) after our varargs transformation.
		if t.isVaListType(typ) {
			continue
		}
		golfType := t.cTypeToGolf(typ)

		if isStatic {
			// Extract static local to a global with a mangled name.
			gname := "_" + t.curFunc + "_" + name
			if !t.emittedGlobals[gname] {
				t.emittedGlobals[gname] = true
				t.staticVars = append(t.staticVars, staticVar{gname, golfType})
			}
			// Register substitution: references to 'name' use 'gname' in this scope.
			t.staticNameMap[name] = gname
			// If it's an array type, record the element pointer so xPrimary can
			// emit the implicit array-to-pointer cast.
			if strings.HasPrefix(golfType, "[") {
				// "[N]elem" → "*elem"
				if idx := strings.Index(golfType, "]"); idx >= 0 {
					elem := golfType[idx+1:]
					t.staticArrayMap[gname] = "*" + elem
				}
			}
			// Inside the body, note the mapping.
			t.line("// static %s → global %s", name, gname)
			if id.Initializer != nil {
				init := t.xExpr(id.Initializer.Expression)
				t.line("%s = %s(%s)", gname, golfType, init)
			}
			continue
		}

		if id.Initializer != nil {
			init := t.xExpr(id.Initializer.Expression)
			// Use := (short declaration) to work in all contexts including
			// switch-case bodies. Append a unique suffix to avoid redeclaration
			// if the same C name appears in multiple case blocks.
			uniq := fmt.Sprintf("%s_%d", name, t.tempCount)
			t.tempCount++
			t.line("%s := %s // %s", uniq, castInit(golfType, init), name)
			// Register the unique name so references in this scope resolve correctly.
			t.staticNameMap[name] = uniq
		} else {
			t.line("var %s %s", name, golfType)
		}
	}
}

// ── Iteration statements ──────────────────────────────────────────────────────

func (t *translator) translateIteration(s *cc.IterationStatement) {
	switch s.Case {
	case cc.IterationStatementWhile:
		cond := t.xExpr(s.ExpressionList)
		// `while (1)` or `while (1 != 0)` → emit as `for {}` (infinite loop).
		condSrc := strings.TrimSpace(cc.NodeSource(s.ExpressionList))
		if condSrc == "1" {
			t.line("for {")
		} else {
			t.line("for %s {", cond)
		}
		t.depth++
		t.translateBody(s.Statement)
		t.depth--
		t.line("}")

	case cc.IterationStatementDo:
		// do { body } while (cond)
		condSrc := strings.TrimSpace(cc.NodeSource(s.ExpressionList))
		if condSrc == "0" {
			// Common macro idiom do { ... } while (0) → just emit body.
			t.translateBody(s.Statement)
		} else {
			// Real do-while → for { body; if !(cond) { break } }
			t.line("for {")
			t.depth++
			t.translateBody(s.Statement)
			cond := t.xExpr(s.ExpressionList)
			t.line("if !(%s) { break }", cond)
			t.depth--
			t.line("}")
		}

	case cc.IterationStatementFor:
		initStr := ""
		if s.ExpressionList != nil {
			initStr = t.xExpr(s.ExpressionList)
		}
		condStr := ""
		if s.ExpressionList2 != nil {
			condStr = t.xExpr(s.ExpressionList2)
		}
		// Check if post-expression is a pointer increment/decrement.
		// If so, we can't inline it in the for-header; rewrite as for { body; post }.
		var postPtrStmts []string
		postStr := ""
		if s.ExpressionList3 != nil {
			if stmts := t.ptrPostExprStmts(s.ExpressionList3); len(stmts) > 0 {
				postPtrStmts = stmts
			} else {
				postStr = t.xExpr(s.ExpressionList3)
			}
		}
		if initStr == "" && condStr == "" && postStr == "" && len(postPtrStmts) == 0 {
			t.line("for {")
		} else if len(postPtrStmts) > 0 {
			// Rewrite: for [init;] cond { body; post }
			if initStr == "" {
				t.line("for %s {", condStr)
			} else {
				t.line("for %s; %s; {", initStr, condStr)
			}
		} else {
			t.line("for %s; %s; %s {", initStr, condStr, postStr)
		}
		t.depth++
		t.translateBody(s.Statement)
		for _, stmt := range postPtrStmts {
			t.line("%s", stmt)
		}
		t.depth--
		t.line("}")

	case cc.IterationStatementForDecl:
		initStr := t.forInitDecl(s.Declaration)
		condStr := ""
		if s.ExpressionList != nil {
			condStr = t.xExpr(s.ExpressionList)
		}
		// Same pointer-post treatment as IterationStatementFor.
		var postPtrStmts []string
		postStr := ""
		if s.ExpressionList2 != nil {
			if stmts := t.ptrPostExprStmts(s.ExpressionList2); len(stmts) > 0 {
				postPtrStmts = stmts
			} else {
				postStr = t.xExpr(s.ExpressionList2)
			}
		}
		if len(postPtrStmts) > 0 {
			t.line("for %s; %s; {", initStr, condStr)
		} else {
			t.line("for %s; %s; %s {", initStr, condStr, postStr)
		}
		t.depth++
		t.translateBody(s.Statement)
		for _, stmt := range postPtrStmts {
			t.line("%s", stmt)
		}
		t.depth--
		t.line("}")
	}
}

// ptrPostExprStmts checks if a for-loop post-expression contains pointer
// increment/decrement (which can't be inlined in MiniGolf's for-header).
// If so, returns the equivalent MiniGolf statement strings; otherwise nil.
func (t *translator) ptrPostExprStmts(e cc.Expression) []string {
	// Unwrap single-element ExpressionList.
	if el, ok := e.(*cc.ExpressionList); ok && len(el.List) == 1 {
		e = el.List[0]
	}
	switch x := e.(type) {
	case *cc.PostfixExpr:
		if x.Expr.Type().Kind() == cc.Ptr {
			base := t.xExpr(x.Expr)
			gt := t.cTypeToGolf(x.Expr.Type())
			op := "+"
			if x.Dec {
				op = "-"
			}
			return []string{fmt.Sprintf("%s = (%s)(word(%s) %s 1)", base, gt, base, op)}
		}
	case *cc.PrefixExpr:
		if x.Expr.Type().Kind() == cc.Ptr {
			base := t.xExpr(x.Expr)
			gt := t.cTypeToGolf(x.Expr.Type())
			op := "+"
			if x.Dec {
				op = "-"
			}
			return []string{fmt.Sprintf("%s = (%s)(word(%s) %s 1)", base, gt, base, op)}
		}
	}
	return nil
}

func (t *translator) forInitDecl(decl cc.Declaration) string {
	switch x := decl.(type) {
	case *cc.CommonDeclaration:
		if len(x.InitDeclarators) == 0 {
			return ""
		}
		id := x.InitDeclarators[0]
		name := id.Declarator.Name()
		golfType := t.cTypeToGolf(id.Declarator.Type())
		if id.Initializer != nil {
			init := t.xExpr(id.Initializer.Expression)
			return fmt.Sprintf("%s := %s", name, castInit(golfType, init))
		}
		return fmt.Sprintf("var %s %s", name, golfType)
	case *cc.AutoDeclaration:
		name := x.Declarator.Name()
		golfType := t.cTypeToGolf(x.Declarator.Type())
		if x.Initializer != nil {
			init := t.xExpr(x.Initializer.Expression)
			return fmt.Sprintf("%s := %s", name, castInit(golfType, init))
		}
		return fmt.Sprintf("var %s %s", name, golfType)
	default:
		return t.unsupported(fmt.Sprintf("for-init decl %T", decl))
	}
}

// ── Selection statements ──────────────────────────────────────────────────────

func (t *translator) translateSelection(s *cc.SelectionStatement) {
	switch s.Case {
	case cc.SelectionStatementIf:
		t.line("if %s {", t.xExpr(s.ExpressionList))
		t.depth++
		t.translateBody(s.Statement)
		t.depth--
		t.line("}")

	case cc.SelectionStatementIfElse:
		t.line("if %s {", t.xExpr(s.ExpressionList))
		t.depth++
		t.translateBody(s.Statement)
		t.depth--
		// Detect else-if chain.
		if inner, ok := s.Statement2.(*cc.SelectionStatement); ok &&
			(inner.Case == cc.SelectionStatementIf || inner.Case == cc.SelectionStatementIfElse) {
			t.raw(strings.Repeat("    ", t.depth) + "} else ")
			t.translateSelection(inner)
		} else {
			t.line("} else {")
			t.depth++
			t.translateBody(s.Statement2)
			t.depth--
			t.line("}")
		}

	case cc.SelectionStatementSwitch:
		// MiniGolf has no switch — emit an if/else-if/else chain.
		// Evaluate switch expression once into a temp.
		swExpr := t.xExpr(s.ExpressionList)
		swVar := fmt.Sprintf("_sw_%d_", t.tempCount)
		t.tempCount++
		t.line("%s := %s", swVar, swExpr)

		type caseGroup struct {
			isDefault bool
			expr      string
			items     []cc.BlockItem
		}
		var groups []caseGroup

		addItem := func(item cc.BlockItem) {
			if len(groups) > 0 {
				groups[len(groups)-1].items = append(groups[len(groups)-1].items, item)
			}
		}

		if cs, ok := s.Statement.(*cc.CompoundStatement); ok {
			for _, item := range cs.List {
				ls, isLabel := item.(*cc.LabeledStatement)
				if !isLabel {
					// Skip top-level break (end-of-case in C; not needed in if-else).
					if js, ok := item.(*cc.JumpStatement); ok && js.Case == cc.JumpStatementBreak {
						continue
					}
					addItem(item)
					continue
				}
				switch ls.Case {
				case cc.LabeledStatementCaseLabel:
					groups = append(groups, caseGroup{expr: t.xExpr(ls.Expression)})
				case cc.LabeledStatementRange:
					groups = append(groups, caseGroup{expr: t.xExpr(ls.Expression)})
				case cc.LabeledStatementDefault:
					groups = append(groups, caseGroup{isDefault: true})
				default:
					addItem(item)
					continue
				}
				// The label's own Statement belongs to the new group.
				if ls.Statement != nil && !isBreakOnly(ls.Statement) {
					groups[len(groups)-1].items = append(groups[len(groups)-1].items, ls.Statement)
				}
			}
		}

		indent := strings.Repeat("    ", t.depth)
		for i, g := range groups {
			var header string
			if g.isDefault {
				if i == 0 {
					header = indent + "{\n"
				} else {
					header = indent + "} else {\n"
				}
			} else {
				cond := fmt.Sprintf("%s == %s", swVar, g.expr)
				if i == 0 {
					header = indent + fmt.Sprintf("if %s {\n", cond)
				} else {
					header = indent + fmt.Sprintf("} else if %s {\n", cond)
				}
			}
			t.raw(header)
			t.depth++
			for _, item := range g.items {
				t.translateBlockItem(item)
			}
			t.depth--
		}
		if len(groups) > 0 {
			t.line("}")
		}
	}
}

// ── Jump statements ───────────────────────────────────────────────────────────

func (t *translator) translateJump(s *cc.JumpStatement) {
	switch s.Case {
	case cc.JumpStatementReturn:
		if s.ExpressionList != nil {
			t.line("return %s", t.xExpr(s.ExpressionList))
		} else {
			t.line("return")
		}
	case cc.JumpStatementBreak:
		t.line("break")
	case cc.JumpStatementContinue:
		t.line("continue")
	case cc.JumpStatementGoto:
		label := strings.TrimSpace(cc.NodeSource(s))
		label = strings.TrimPrefix(label, "goto")
		t.line("%s", t.unsupported("goto "+strings.TrimSpace(label)))
	default:
		t.line("%s", t.unsupported(fmt.Sprintf("jump %v", s.Case)))
	}
}

// ── Labeled statements ────────────────────────────────────────────────────────

func (t *translator) translateLabeled(s *cc.LabeledStatement) {
	switch s.Case {
	case cc.LabeledStatementCaseLabel:
		t.depth--
		t.line("case %s:", t.xExpr(s.Expression))
		t.depth++
		if !isBreakOnly(s.Statement) {
			t.translateStatement(s.Statement)
		}
	case cc.LabeledStatementRange:
		t.depth--
		t.line("case %s ... %s:", t.xExpr(s.Expression), t.xExpr(s.Expression2))
		t.depth++
		if !isBreakOnly(s.Statement) {
			t.translateStatement(s.Statement)
		}
	case cc.LabeledStatementDefault:
		t.depth--
		t.line("default:")
		t.depth++
		if !isBreakOnly(s.Statement) {
			t.translateStatement(s.Statement)
		}
	case cc.LabeledStatementLabel:
		lbl := s.Token.SrcStr()
		t.line("/* label: %s */", lbl)
		t.translateStatement(s.Statement)
	default:
		t.line("%s", t.unsupported(fmt.Sprintf("labeled %v", s.Case)))
	}
}

func isBreakOnly(stmt cc.Statement) bool {
	js, ok := stmt.(*cc.JumpStatement)
	return ok && js.Case == cc.JumpStatementBreak
}

// ─────────────────────────────────────────────────────────────────────────────
// Expression translation
// ─────────────────────────────────────────────────────────────────────────────

// xExpr recursively translates a C expression to MiniGolf syntax.
func (t *translator) xExpr(n cc.Expression) string {
	if n == nil {
		return ""
	}
	switch x := n.(type) {

	case *cc.ExpressionList:
		if len(x.List) == 1 {
			return t.xExpr(x.List[0])
		}
		// Comma-expr in expression context is problematic.
		parts := make([]string, len(x.List))
		for i, e := range x.List {
			parts[i] = t.xExpr(e)
		}
		return t.unsupported("comma-expr(" + strings.Join(parts, ", ") + ")")

	case *cc.PrimaryExpression:
		return t.xPrimary(x)

	case *cc.SelectorExpr:
		base := t.xExpr(x.Expr)
		field := x.Sel.SrcStr()
		// Both . and -> become . in MiniGolf (auto-deref through pointer).
		result := base + "." + field
		// Array-to-pointer decay for struct fields: if the struct field's
		// un-decayed type is an array (e.g. unsigned char[256]), and cc/v5
		// has already decayed x.Type() to a pointer, we must emit (*T)(result)
		// so MiniGolf knows to take the array address rather than copying it.
		if x.Type().Kind() == cc.Ptr {
			// Check the struct type for the field's declared type.
			baseTyp := x.Expr.Type()
			if baseTyp.Kind() == cc.Ptr {
				if pt, ok := baseTyp.(*cc.PointerType); ok {
					baseTyp = pt.Elem()
				}
			}
			if st, ok := baseTyp.(*cc.StructType); ok {
				for i := 0; i < st.NumFields(); i++ {
					f := st.FieldByIndex(i)
					if f == nil {
						continue
					}
					if f.Name() == field && f.Type().Kind() == cc.Array {
						// Field is an array — emit the explicit decay cast.
						golfPtrType := t.cTypeToGolf(x.Type())
						result = "(" + golfPtrType + ")(" + result + ")"
						break
					}
				}
			}
		}
		return result

	case *cc.IndexExpr:
		return t.xExpr(x.Expr) + "[" + t.xExpr(x.Index) + "]"

	case *cc.CallExpr:
		return t.xCall(x)

	case *cc.PostfixExpr:
		base := t.xExpr(x.Expr)
		// Pointer types: only valid at statement level (handled by translateExprStmtOne
		// and ptrPostExprStmts). If reached here in expression context, emit a comment.
		if x.Expr.Type().Kind() == cc.Ptr {
			if x.Dec {
				return t.unsupported("ptr postfix-- in expr: " + base)
			}
			return t.unsupported("ptr postfix++ in expr: " + base)
		}
		// Non-pointer: use prelude helper that returns old value then mutates.
		golfType := t.cTypeToGolf(x.Expr.Type())
		if x.Dec {
			return fmt.Sprintf("post_decrement[%s](&%s)", golfType, base)
		}
		return fmt.Sprintf("post_increment[%s](&%s)", golfType, base)

	case *cc.PrefixExpr:
		base := t.xExpr(x.Expr)
		if x.Expr.Type().Kind() == cc.Ptr {
			// Pointer prefix increment/decrement in expression context.
			// These are rare; if reached, we cannot inline the side-effect cleanly.
			if x.Dec {
				return t.unsupported("ptr prefix-- in expr: " + base)
			}
			return t.unsupported("ptr prefix++ in expr: " + base)
		}
		// Non-pointer: use prelude helper that mutates and returns new value.
		golfType := t.cTypeToGolf(x.Expr.Type())
		if x.Dec {
			return fmt.Sprintf("pre_decrement[%s](&%s)", golfType, base)
		}
		return fmt.Sprintf("pre_increment[%s](&%s)", golfType, base)

	case *cc.UnaryExpr:
		return t.xUnary(x)

	case *cc.CastExpr:
		return t.xCast(x)

	case *cc.BinaryExpression:
		lhs := t.xExpr(x.Lhs)
		rhs := t.xExpr(x.Rhs)
		op := binaryOpStr(x.Op)
		lGolf := t.cTypeToGolf(x.Lhs.Type())
		rGolf := t.cTypeToGolf(x.Rhs.Type())

		// Pointer arithmetic: ptr+n, ptr-n, n+ptr, ptr-ptr.
		lIsPtr := strings.HasPrefix(lGolf, "*")
		rIsPtr := strings.HasPrefix(rGolf, "*")
		if lIsPtr && (op == "+" || op == "-") {
			elemType := lGolf[1:] // strip leading *
			if rIsPtr && op == "-" {
				// ptr - ptr → element-count difference
				return fmt.Sprintf("prelude.pointer_diff[%s](%s, %s)", elemType, lhs, rhs)
			}
			// ptr ± int: for byte* use the direct word-cast idiom (avoids Sizeof).
			// For other element types use the generic prelude helper.
			fn := "pointer_add"
			if op == "-" {
				fn = "pointer_sub"
			}
			rhsInt := rhs
			if rGolf != "int" {
				rhsInt = "int(" + rhs + ")"
			}
			if elemType == "byte" {
				wordOp := "+"
				if op == "-" {
					wordOp = "-"
				}
				return fmt.Sprintf("(*byte)(word(%s) %s word(%s))", lhs, wordOp, rhsInt)
			}
			return fmt.Sprintf("prelude.%s[%s](%s, %s)", fn, elemType, lhs, rhsInt)
		}
		if rIsPtr && op == "+" {
			// int + ptr → commute to ptr + int
			elemType := rGolf[1:]
			lhsInt := lhs
			if lGolf != "int" {
				lhsInt = "int(" + lhs + ")"
			}
			if elemType == "byte" {
				return fmt.Sprintf("(*byte)(word(%s) + word(%s))", rhs, lhsInt)
			}
			return fmt.Sprintf("prelude.pointer_add[%s](%s, %s)", elemType, rhs, lhsInt)
		}

		// Apply C's usual arithmetic conversions.
		resultGolf := t.cTypeToGolf(x.Type())
		if resultGolf == "word" || resultGolf == "int" {
			// Promote any byte operand to the wider result type.
			if golfTypeRank(lGolf) == 0 {
				lhs = castInit(resultGolf, lhs)
			}
			if golfTypeRank(rGolf) == 0 {
				rhs = castInit(resultGolf, rhs)
			}
		}
		// Always resolve any remaining int/word or byte/word mismatches.
		lhs, rhs = t.promoteForBinop(x.Lhs.Type(), lhs, x.Rhs.Type(), rhs)
		return lhs + " " + op + " " + rhs

	case *cc.AssignmentExpression:
		lhs := t.xExpr(x.Lhs)
		rhs := t.xExpr(x.Rhs)
		op := assignOpStr(x.Op)
		lGolf := t.cTypeToGolf(x.Lhs.Type())
		rGolf := t.cTypeToGolf(x.Rhs.Type())
		if op == "=" {
			// C silently narrows when storing a wider int/word result into a byte
			// variable. MiniGolf requires an explicit cast.
			if lGolf == "byte" && (rGolf == "int" || rGolf == "word") {
				rhs = "byte(" + rhs + ")"
			}
			// C implicit array-to-pointer decay: assigning [N]T to *T, or an
			// array field (which cc/v5 may report as already-decayed *T) to *T.
			if strings.HasPrefix(lGolf, "*") {
				switch x.Rhs.Type().Kind() {
				case cc.Array:
					// RHS is array type — cc/v5 has not decayed it yet.
					// Extract element from "[N]elem" and check it matches.
					if strings.HasPrefix(rGolf, "[") {
						if idx := strings.Index(rGolf, "]"); idx >= 0 {
							eltGolf := rGolf[idx+1:]
							if "*"+eltGolf == lGolf {
								rhs = "(" + lGolf + ")(" + rhs + ")"
							}
						}
					}
				}
				// If RHS kind is already Ptr (already-decayed array field),
				// no explicit cast is needed — it's already the right pointer type.
			}
		} else if (op == "+=" || op == "-=") && strings.HasPrefix(lGolf, "*") {
			// Pointer compound assignment: p += n  →  p = prelude.pointer_add[T](p, n)
			elemType := lGolf[1:]
			fn := "pointer_add"
			if op == "-=" {
				fn = "pointer_sub"
			}
			rhsInt := rhs
			if rGolf != "int" {
				rhsInt = "int(" + rhs + ")"
			}
			return fmt.Sprintf("%s = prelude.%s[%s](%s, %s)", lhs, fn, elemType, lhs, rhsInt)
		} else {
			// For other compound assignment ops (+=, -=, …) promote the RHS if narrower.
			_, rhs = t.promoteForBinop(x.Lhs.Type(), lhs, x.Rhs.Type(), rhs)
		}
		return lhs + " " + op + " " + rhs

	case *cc.ConditionalExpression:
		return t.xConditional(x)

	default:
		// Fallback: NodeSource with -> → . post-processing.
		src := cc.NodeSource(n)
		src = strings.TrimRight(src, ";")
		src = strings.ReplaceAll(src, "->", ".")
		return strings.TrimSpace(src)
	}
}

func (t *translator) xPrimary(x *cc.PrimaryExpression) string {
	switch x.Case {
	case cc.PrimaryExpressionIdent:
		name := x.Token.SrcStr()
		// Substitute static-local names with their mangled global names.
		if gname, ok := t.staticNameMap[name]; ok {
			// If the global is an array, decay it to a pointer as C would.
			if ptrType, ok := t.staticArrayMap[gname]; ok {
				return fmt.Sprintf("(%s)(%s)", ptrType, gname)
			}
			return gname
		}
		// Array-to-pointer decay for regular (non-static) variables:
		// if the declared type of this identifier is an array (e.g. char buf[5]),
		// cc/v5 decays the expression type to a pointer but Golf keeps the
		// variable as [N]T. Emit (*EltType)(name) to force the decay.
		if resolved := x.ResolvedTo(); resolved != nil {
			type typer interface{ Type() cc.Type }
			if typerNode, ok := resolved.(typer); ok {
				declaredTyp := typerNode.Type()
				if declaredTyp != nil && declaredTyp.Kind() == cc.Array {
					at, ok := declaredTyp.(*cc.ArrayType)
					if ok {
						eltGolf := t.cTypeToGolf(at.Elem())
						return fmt.Sprintf("(*%s)(%s)", eltGolf, name)
					}
				}
			}
		}
		return name
	case cc.PrimaryExpressionInt:
		return sanitizeIntLit(x.Token.SrcStr())
	case cc.PrimaryExpressionFloat:
		return x.Token.SrcStr()
	case cc.PrimaryExpressionChar:
		return sanitizeCharLit(x.Token.SrcStr())
	case cc.PrimaryExpressionString:
		return x.Token.SrcStr()
	case cc.PrimaryExpressionExpr:
		return "(" + t.xExpr(x.ExpressionList) + ")"
	default:
		return t.unsupported(fmt.Sprintf("primary %v", x.Case))
	}
}

func (t *translator) xCall(x *cc.CallExpr) string {
	fnStr := t.xExpr(x.Func)

	// __builtin_va_start / __builtin_va_end → no-op (va list is a parameter)
	if fnStr == "__builtin_va_start" || fnStr == "__builtin_va_end" {
		return ""
	}
	// __builtin_va_arg_impl is handled by tryVaArg() in the surrounding
	// *((*T)(...)) dereference pattern; if seen bare, emit unsupported.
	if strings.HasPrefix(fnStr, "__builtin_va") {
		return t.unsupported("bare " + fnStr)
	}

	// Collect arguments in source order (first to last), narrowing int→byte
	// when the callee's parameter type is byte (C does this implicitly).
	var paramTypes []cc.Type
	callType := x.Func.Type()
	// In cc/v5, function identifiers may appear as either *FunctionType or
	// *PointerType → *FunctionType (the usual C function-pointer decay).
	if pt, ok := callType.(*cc.PointerType); ok {
		callType = pt.Elem()
	}
	if ft, ok := callType.(*cc.FunctionType); ok {
		for _, p := range ft.Parameters() {
			paramTypes = append(paramTypes, p.Type())
		}
	}
	var args []string
	for i, ael := 0, x.Arguments; ael != nil; i, ael = i+1, ael.ArgumentExpressionList {
		arg := t.xExpr(ael.Expression)
		argExprTyp := ael.Expression.Type() // may be decayed by cc/v5
		argGolf := t.cTypeToGolf(argExprTyp)
		if i < len(paramTypes) {
			paramGolf := t.cTypeToGolf(paramTypes[i])
			if paramGolf == "byte" && (argGolf == "int" || argGolf == "word") {
				arg = "byte(" + arg + ")"
			}
			// Array-to-pointer decay for call arguments: when the un-decayed
			// argument expression is an array (e.g. char buf[5]) passed to a
			// parameter expecting a pointer (e.g. char*), emit (*EltType)(arg).
			// cc/v5 decays it to *T in argExprTyp, so check the arg node's kind
			// via the parameter's expected pointer type vs the Golf type.
			if strings.HasPrefix(paramGolf, "*") && strings.HasPrefix(argGolf, "*") {
				// Both look like pointers — but the arg Golf name may be a
				// local/global [N]T array whose Golf type in the struct is
				// still [N]T. Check the actual argument expression node type
				// before decay.
				if ael.Expression.Type().Kind() == cc.Array {
					eltType := t.cTypeToGolf(ael.Expression.Type().(*cc.ArrayType).Elem())
					if "*"+eltType == paramGolf {
						arg = "(" + paramGolf + ")(" + arg + ")"
					}
				}
			}
		} else {
			// Variadic / excess argument: apply C default argument promotions.
			// In C, integer types narrower than int (char, unsigned char, short,
			// etc.) are promoted to int when passed to variadic functions. This
			// is critical on big-endian targets like M6809: a byte any stores
			// only 1 byte, but peek[word] reads 2 bytes and gets the value in
			// the high byte (VV00 instead of 00VV). On little-endian x86_64 it
			// accidentally works because the low byte is the value byte.
			if argGolf == "byte" {
				arg = "word(" + arg + ")"
			}
			// Array-to-pointer decay for variadic args.
			if argExprTyp.Kind() == cc.Array {
				at := argExprTyp.(*cc.ArrayType)
				eltGolf := t.cTypeToGolf(at.Elem())
				arg = "(*" + eltGolf + ")(" + arg + ")"
			}
		}
		args = append(args, arg)
	}
	return fnStr + "(" + strings.Join(args, ", ") + ")"
}

func (t *translator) xUnary(x *cc.UnaryExpr) string {
	switch x.Case {
	case cc.UnaryExpressionAddrof:
		return "&" + t.xExpr(x.Expr)
	case cc.UnaryExpressionDeref:
		// Detect: *((*T)(__builtin_va_arg_impl(ap)))
		// → peek[T](ap.Pop().BaseAddr)
		if va := t.tryVaArg(x.Expr); va != "" {
			return va
		}
		return "*" + t.xExpr(x.Expr)
	case cc.UnaryExpressionPlus:
		return t.xExpr(x.Expr) // unary + is a no-op
	case cc.UnaryExpressionMinus:
		return "-" + t.xExpr(x.Expr)
	case cc.UnaryExpressionNot:
		return "!" + t.xExpr(x.Expr)
	case cc.UnaryExpressionCpl:
		return "^" + t.xExpr(x.Expr)
	default:
		return t.unsupported(fmt.Sprintf("unary %v (%s)", x.Case, t.xExpr(x.Expr)))
	}
}

func (t *translator) xCast(x *cc.CastExpr) string {
	inner := t.xExpr(x.Expr)
	golfType := t.typeNameToGolf(x.TypeName)
	if golfType == "" {
		// Cast to void → just the inner expression.
		return inner
	}
	if strings.Contains(golfType, "UNSUPPORTED") || strings.HasPrefix(golfType, "/*") {
		if t.keepGoing {
			return fmt.Sprintf("/* cast(%s) */(%s)", golfType, inner)
		}
		return inner
	}
	// Pointer types need outer parens: (*T)(val) not *T(val).
	if strings.HasPrefix(golfType, "*") {
		return "(" + golfType + ")(" + inner + ")"
	}
	return golfType + "(" + inner + ")"
}

// xConditional handles ternary  a ? b : c.
// MiniGolf has no ternary operator.
func (t *translator) xConditional(x *cc.ConditionalExpression) string {
	cond := t.xExpr(x.Condition)
	thn := t.xExpr(x.Then)
	els := t.xExpr(x.Else)
	// Promote branch arguments to the C result type when branches are narrower.
	// e.g. 'byte_val ? byte_val : 0' has C result type int/word.
	resultGolf := t.cTypeToGolf(x.Type())
	if resultGolf == "word" || resultGolf == "int" {
		thnGolf := t.cTypeToGolf(x.Then.Type())
		elsGolf := t.cTypeToGolf(x.Else.Type())
		if golfTypeRank(thnGolf) < 1 {
			thn = castInit(resultGolf, thn)
		}
		if golfTypeRank(elsGolf) < 1 {
			els = castInit(resultGolf, els)
		}
	}
	return fmt.Sprintf("cond(%s, %s, %s)", cond, thn, els)
}

func (t *translator) typeNameToGolf(tn *cc.TypeName) string {
	if tn == nil {
		return ""
	}
	return t.cTypeToGolf(tn.Type())
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func sanitizeIntLit(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimRight(s, "ul")
	return s
}

// sanitizeCharLit converts a C character literal to a valid MiniGolf literal.
// Simple printable ASCII characters are kept as 'c'.  Non-printable or
// non-simple characters (escape sequences like '\0', '\n', '\x41', octal) are
// converted to their decimal integer value, which MiniGolf always accepts.
func sanitizeCharLit(src string) string {
	// src looks like 'a', '\n', '\0', '\x41', '\101', etc.
	if len(src) < 3 || src[0] != '\'' || src[len(src)-1] != '\'' {
		return src // malformed — pass through
	}
	inner := src[1 : len(src)-1]

	var val rune
	if len(inner) == 1 {
		val = rune(inner[0])
	} else if len(inner) >= 2 && inner[0] == '\\' {
		switch inner[1] {
		case 'n':
			val = '\n'
		case 't':
			val = '\t'
		case 'r':
			val = '\r'
		case 'a':
			val = '\a'
		case 'b':
			val = '\b'
		case 'f':
			val = '\f'
		case 'v':
			val = '\v'
		case '\\':
			val = '\\'
		case '\'':
			val = '\''
		case '"':
			val = '"'
		case 'x', 'X':
			n, err := strconv.ParseInt(inner[2:], 16, 32)
			if err != nil {
				return src
			}
			val = rune(n)
		default:
			// C octal escape: \0, \012, etc.
			if inner[1] >= '0' && inner[1] <= '7' {
				n, err := strconv.ParseInt(inner[1:], 8, 32)
				if err != nil {
					return src
				}
				val = rune(n)
			} else {
				return src // unknown escape — pass through
			}
		}
	} else {
		return src // multi-byte char — pass through
	}

	// Simple printable ASCII (space to ~) with no quoting needed.
	if val >= 32 && val <= 126 && val != '\'' && val != '\\' {
		return fmt.Sprintf("'%c'", val)
	}
	// Single-quote and backslash need escaping but are still valid literals.
	if val == '\'' {
		return "'\\'"
	}
	if val == '\\' {
		return "'\\\\'"
	}
	// Everything else (NUL, newline, tab, …): emit as plain decimal.
	return fmt.Sprintf("%d", val)
}

func binaryOpStr(op cc.BinaryOperation) string {
	switch op {
	case cc.BinaryOperationAdd:
		return "+"
	case cc.BinaryOperationSub:
		return "-"
	case cc.BinaryOperationMul:
		return "*"
	case cc.BinaryOperationDiv:
		return "/"
	case cc.BinaryOperationMod:
		return "%"
	case cc.BinaryOperationOr:
		return "|"
	case cc.BinaryOperationAnd:
		return "&"
	case cc.BinaryOperationXor:
		return "^"
	case cc.BinaryOperationLsh:
		return "<<"
	case cc.BinaryOperationRsh:
		return ">>"
	case cc.BinaryOperationEq:
		return "=="
	case cc.BinaryOperationNeq:
		return "!="
	case cc.BinaryOperationLt:
		return "<"
	case cc.BinaryOperationGt:
		return ">"
	case cc.BinaryOperationLeq:
		return "<="
	case cc.BinaryOperationGeq:
		return ">="
	case cc.BinaryOperationLOr:
		return "||"
	case cc.BinaryOperationLAnd:
		return "&&"
	default:
		return fmt.Sprintf("/* binop %d */", int(op))
	}
}

func assignOpStr(op cc.AssignmentOperation) string {
	switch op {
	case cc.AssignmentOperationAssign:
		return "="
	case cc.AssignmentOperationMul:
		return "*="
	case cc.AssignmentOperationDiv:
		return "/="
	case cc.AssignmentOperationMod:
		return "%="
	case cc.AssignmentOperationAdd:
		return "+="
	case cc.AssignmentOperationSub:
		return "-="
	case cc.AssignmentOperationLsh:
		return "<<="
	case cc.AssignmentOperationRsh:
		return ">>="
	case cc.AssignmentOperationAnd:
		return "&="
	case cc.AssignmentOperationXor:
		return "^="
	case cc.AssignmentOperationOr:
		return "|="
	default:
		return fmt.Sprintf("/* assignop%d */=", int(op))
	}
}

// tokenStr safely calls SrcStr() on a cc.Token return value.
// SrcStr has a pointer receiver, so we must store the token in a local first.
func tokenStr(tok cc.Token) string {
	t := tok
	return t.SrcStr()
}

func structTag(typ cc.Type) string {
	inner := stripPtr(typ)
	if inner.Kind() == cc.Struct {
		if st, ok := inner.(*cc.StructType); ok {
			return tokenStr(st.Tag())
		}
	}
	return ""
}

func stripPtr(typ cc.Type) cc.Type {
	if typ.Kind() == cc.Ptr {
		if pt, ok := typ.(*cc.PointerType); ok {
			return pt.Elem()
		}
	}
	return typ
}

// capitalise uppercases the first letter of s.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func isBuiltin(d *cc.Declarator) bool {
	pos := d.Position()
	return strings.HasPrefix(pos.Filename, "<") || strings.HasPrefix(d.Name(), "__")
}

func isBuiltinDecl(cd *cc.CommonDeclaration) bool {
	if len(cd.InitDeclarators) == 0 {
		return false
	}
	for _, id := range cd.InitDeclarators {
		if !isBuiltin(id.Declarator) {
			return false
		}
	}
	return true
}

// ── va_list / varargs helpers ─────────────────────────────────────────────────

// ── Integer promotion helpers ─────────────────────────────────────────────────

// golfTypeRank assigns a numeric width rank to a MiniGolf type string so that
// promoteForBinop can decide which side needs a widening cast.
//
//	 0 = byte (8-bit)
//	 1 = int or word (16-bit signed / unsigned)
//	 2 = const_integer or unknown (compatible with any numeric type)
//	-1 = pointer / struct / other non-numeric — left alone
func golfTypeRank(gt string) int {
	switch gt {
	case "byte":
		return 0
	case "word", "int":
		return 1
	case "const_integer", "":
		return 2 // untyped literal — compatible with any numeric
	}
	// Pointer types, structs, etc.
	if strings.HasPrefix(gt, "*") || strings.HasPrefix(gt, "[") || strings.HasPrefix(gt, "/*") {
		return -1
	}
	return 2 // treat unknown as untyped
}

// promoteForBinop applies C's usual-arithmetic-conversion rule: when the two
// operands have different ranks, wrap the narrower one in a cast to the wider
// type.  It returns the (possibly-wrapped) lhs and rhs strings.
func (t *translator) promoteForBinop(lTyp cc.Type, lhs string, rTyp cc.Type, rhs string) (string, string) {
	lg := t.cTypeToGolf(lTyp)
	rg := t.cTypeToGolf(rTyp)
	lr := golfTypeRank(lg)
	rr := golfTypeRank(rg)
	// Skip non-numeric types (pointers, structs, etc.).
	if lr < 0 || rr < 0 {
		return lhs, rhs
	}
	// byte vs word/int: promote the byte side.
	if lr < rr && rr == 1 {
		return castInit(rg, lhs), rhs
	}
	if rr < lr && lr == 1 {
		return lhs, castInit(lg, rhs)
	}
	// int vs word (same rank but different signedness): cast int→word.
	// In C, unsigned dominates in mixed signed/unsigned arithmetic.
	if lr == 1 && rr == 1 && lg != rg {
		if lg == "int" {
			return castInit("word", lhs), rhs
		}
		return lhs, castInit("word", rhs)
	}
	return lhs, rhs
}

// isVaListType reports whether typ is a C va_list type (or its platform aliases).
func (t *translator) isVaListType(typ cc.Type) bool {
	if typ == nil {
		return false
	}
	td := typ.Typedef()
	if td == nil {
		return false
	}
	n := td.Name()
	return n == "va_list" || n == "__gnuc_va_list" || n == "__builtin_va_list"
}

// vaListName scans the top-level declarations of a function body looking for
// a local variable of va_list type and returns its name.
// This is used to rename the "..." parameter of a variadic function.
func (t *translator) vaListName(body *cc.CompoundStatement) string {
	if body == nil {
		return ""
	}
	for _, item := range body.List {
		cd, ok := item.(*cc.CommonDeclaration)
		if !ok {
			continue
		}
		for _, id := range cd.InitDeclarators {
			if t.isVaListType(id.Declarator.Type()) {
				return id.Declarator.Name()
			}
		}
	}
	return ""
}

// tryVaArg detects the pattern produced by cc_v5 for __builtin_va_arg(ap, T):
//
//	*((*T)(__builtin_va_arg_impl(ap)))
//
// The outer UnaryExpr(Deref) calls this with its inner expression.
// If the pattern matches, it returns the equivalent MiniGolf expression:
//
//	peek[T](ap.Pop().BaseAddr)
//
// where T is the element type extracted from the (*T) cast, and ap is the
// va_list argument name.  Returns "" if the pattern does not match.
func (t *translator) tryVaArg(expr cc.Expression) string {
	castExpr, ok := expr.(*cc.CastExpr)
	if !ok {
		return ""
	}
	golfPtrType := t.typeNameToGolf(castExpr.TypeName)
	if !strings.HasPrefix(golfPtrType, "*") {
		return ""
	}
	elemType := golfPtrType[1:] // strip leading *

	callExpr, ok := castExpr.Expr.(*cc.CallExpr)
	if !ok {
		return ""
	}
	fnStr := t.xExpr(callExpr.Func)
	if fnStr != "__builtin_va_arg_impl" {
		return ""
	}
	if callExpr.Arguments == nil {
		return ""
	}
	apName := t.xExpr(callExpr.Arguments.Expression)
	// Wrap element type in parens if it is not a plain identifier
	// (e.g. peek[*byte] needs no extra wrapping, but the generic syntax
	// already handles it).
	return fmt.Sprintf("peek[%s](%s.Pop().BaseAddr)", elemType, apName)
}

// If golfType is a plain identifier (e.g. "int", "word", "Bin",
// "prelude.Bin"), it emits T(expr).
// If golfType is compound (e.g. "*byte", "[4]byte", "/* ... */"),
// it emits (T)(expr) to avoid parse ambiguity.
func castInit(golfType, init string) string {
	if isSingleIdent(golfType) {
		return golfType + "(" + init + ")"
	}
	return "(" + golfType + ")(" + init + ")"
}

// isSingleIdent reports whether s is a plain MiniGolf identifier,
// i.e. consists only of letters, digits, underscores, and dots
// (for package-qualified names like "prelude.Bin"), with no leading digit.
func isSingleIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			// always ok
		case r >= '0' && r <= '9':
			if i == 0 {
				return false // can't start with digit
			}
		default:
			return false
		}
	}
	return true
}
