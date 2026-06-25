// cc_to_golf.go — C to MiniGolf translator
//
// Usage (run from the repo root):
//
//	go run cc_v5/cmd/cc_to_golf/cc_to_golf.go [-k] <file.c>
//
// -k  keep going: emit /* comment */ instead of panic() for unsupported
//     constructs so the output can be inspected as a best-effort translation.
//
// The translator uses the modernc.org/cc/v5 typed AST and walks it directly,
// so most C constructs map cleanly to MiniGolf.  Hard cases:
//   - Ternary (? :)         → inline temp-var split when possible
//   - Pointer arithmetic    → (*T)(word(p)+1) cast idiom or comment
//   - va_list / va_arg      → unsupported, emits panic/comment
//   - __builtin_*           → unsupported
//   - volatile / const      → silently stripped
//   - Comma expression      → split into separate statements where possible

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	cc "modernc.org/cc/v5"
)

var keepGoing = flag.Bool("k", false,
	"keep going after unsupported constructs (emit comments instead of panics)")

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: cc_to_golf [-k] <file.c>")
		os.Exit(1)
	}

	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewConfig: %v\n", err)
		os.Exit(1)
	}
	cfg.Predefined += cc.Builtin

	sources := []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: args[0]},
	}

	ast, err := cc.Translate(cfg, sources)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if ast == nil {
		fmt.Fprintln(os.Stderr, "fatal: nil AST")
		os.Exit(1)
	}

	tr := newTranslator()
	tr.translateProgram(ast)
}

// ─────────────────────────────────────────────────────────────────────────────
// Translator state
// ─────────────────────────────────────────────────────────────────────────────

type staticVar struct {
	golfName string
	golfType string
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

	// static locals: local name → mangled global name for current function
	staticNameMap   map[string]string
}

func newTranslator() *translator {
	return &translator{
		typedefMap:     make(map[string]string),
		structTagMap:   make(map[string]string),
		emittedStructs: make(map[string]bool),
		emittedGlobals: make(map[string]bool),
		staticNameMap:  make(map[string]string),
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
	if *keepGoing {
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
		name := td.Name()
		if gname, ok := t.typedefMap[name]; ok {
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
				if _, exists := t.structTagMap[tag]; !exists && tag != "" {
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

	fmt.Print(t.out.String())
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
		// Also check typedef-pointed struct types.
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
			t.raw(fmt.Sprintf("\nvar %s %s = %s\n", name, golfType, initStr))
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
		params = append(params, "_ ...any")
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
	t.line("%s", t.xExpr(e))
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
			t.line("var %s %s = %s", name, golfType, init)
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
			t.line("while %s {", cond)
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
			// Rewrite: for init; cond { body; post }
			if initStr == "" {
				t.line("for ; %s {", condStr)
			} else {
				t.line("for %s; %s {", initStr, condStr)
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
			t.line("for %s; %s {", initStr, condStr)
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
		t.line("switch %s {", t.xExpr(s.ExpressionList))
		t.depth++
		t.translateBody(s.Statement)
		t.depth--
		t.line("}")
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
		return base + "." + field

	case *cc.IndexExpr:
		return t.xExpr(x.Expr) + "[" + t.xExpr(x.Index) + "]"

	case *cc.CallExpr:
		return t.xCall(x)

	case *cc.PostfixExpr:
		base := t.xExpr(x.Expr)
		if x.Dec {
			return base + "--"
		}
		return base + "++"

	case *cc.PrefixExpr:
		base := t.xExpr(x.Expr)
		if x.Dec {
			return "--" + base
		}
		return "++" + base

	case *cc.UnaryExpr:
		return t.xUnary(x)

	case *cc.CastExpr:
		return t.xCast(x)

	case *cc.BinaryExpression:
		lhs := t.xExpr(x.Lhs)
		rhs := t.xExpr(x.Rhs)
		op := binaryOpStr(x.Op)
		return lhs + " " + op + " " + rhs

	case *cc.AssignmentExpression:
		lhs := t.xExpr(x.Lhs)
		rhs := t.xExpr(x.Rhs)
		op := assignOpStr(x.Op)
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
			return gname
		}
		return name
	case cc.PrimaryExpressionInt:
		return sanitizeIntLit(x.Token.SrcStr())
	case cc.PrimaryExpressionFloat:
		return x.Token.SrcStr()
	case cc.PrimaryExpressionChar:
		return x.Token.SrcStr()
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

	// __builtin_va_* → unsupported
	if strings.HasPrefix(fnStr, "__builtin_va") {
		return t.unsupported("va_arg/va_start: " + fnStr)
	}

	// Collect arguments in source order (first to last).
	var args []string
	for ael := x.Arguments; ael != nil; ael = ael.ArgumentExpressionList {
		args = append(args, t.xExpr(ael.Expression))
	}
	return fnStr + "(" + strings.Join(args, ", ") + ")"
}

func (t *translator) xUnary(x *cc.UnaryExpr) string {
	inner := t.xExpr(x.Expr)
	switch x.Case {
	case cc.UnaryExpressionAddrof:
		return "&" + inner
	case cc.UnaryExpressionDeref:
		return "*" + inner
	case cc.UnaryExpressionPlus:
		return "+" + inner
	case cc.UnaryExpressionMinus:
		return "-" + inner
	case cc.UnaryExpressionNot:
		return "!" + inner
	case cc.UnaryExpressionCpl:
		return "^" + inner
	default:
		return t.unsupported(fmt.Sprintf("unary %v (%s)", x.Case, inner))
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
		if *keepGoing {
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

// castInit wraps a type+value pair into a MiniGolf cast expression.
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
