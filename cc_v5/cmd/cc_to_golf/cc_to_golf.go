// cc_to_golf.go — simple C-to-MiniGolf translator
//
// Translates simple C programs to MiniGolf syntax.  Designed for programs
// like count.c.txt; not a general-purpose translator.
//
// Usage (run from inside the cc_v5 directory):
//
//	go run cmd/cc_to_golf.go cmd/count.c.txt
//
// It sends the MiniGolf output to stdout.

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	cc "modernc.org/cc/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cc_to_golf <file.c>")
		os.Exit(1)
	}

	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewConfig: %v\n", err)
		os.Exit(1)
	}
	// Provide built-in definitions; the file may rely on them even without
	// a full #include.
	cfg.Predefined += cc.Builtin

	// Parse as a free-standing file (no system headers unless the file
	// itself #includes them).
	sources := []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: os.Args[1]},
	}

	ast, err := cc.Translate(cfg, sources)
	if err != nil {
		// Non-fatal: some constructs may cause warnings; proceed anyway.
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if ast == nil {
		fmt.Fprintln(os.Stderr, "fatal: nil AST")
		os.Exit(1)
	}

	tr := &translator{}
	tr.translateProgram(ast)
}

// ─────────────────────────────────────────────────────────────────────────────
// Translator
// ─────────────────────────────────────────────────────────────────────────────

type translator struct {
	out    strings.Builder
	depth  int // indentation level
}

// writeln writes one indented line to the output buffer.
func (t *translator) writeln(format string, args ...interface{}) {
	fmt.Fprintf(&t.out, "%s%s\n", strings.Repeat("    ", t.depth),
		fmt.Sprintf(format, args...))
}

// write writes text without a trailing newline.
func (t *translator) write(s string) { t.out.WriteString(s) }

// ── Top-level ────────────────────────────────────────────────────────────────

func (t *translator) translateProgram(ast *cc.AST) {
	t.write("package main\n")

	for _, d := range ast.Declarations {
		fd, ok := d.(*cc.FunctionDefinition)
		if !ok {
			continue
		}
		// Skip built-in / system declarations.
		if isBuiltin(fd.Declarator) {
			continue
		}
		t.write("\n")
		t.translateFuncDef(fd)
	}

	fmt.Print(t.out.String())
}

// ── Function definition ───────────────────────────────────────────────────────

func (t *translator) translateFuncDef(f *cc.FunctionDefinition) {
	d := f.Declarator
	name := d.Name()

	ft, ok := d.Type().(*cc.FunctionType)
	if !ok {
		t.writeln("// (skipping %s: not a function type)", name)
		return
	}

	// Parameters
	var params []string
	for _, p := range ft.Parameters() {
		pname := p.Name()
		ptype := cTypeToGolf(p.Type())
		if pname == "" {
			params = append(params, ptype)
		} else {
			params = append(params, pname+" "+ptype)
		}
	}

	// Return type (int main → no return in MiniGolf; void → nothing)
	ret := ""
	if ft.Result().Kind() != cc.Void &&
		!(name == "main" && ft.Result().Kind() == cc.Int) {
		ret = " " + cTypeToGolf(ft.Result())
	}

	t.writeln("func %s(%s)%s {", name, strings.Join(params, ", "), ret)
	t.depth++
	t.translateCompound(f.Body)
	t.depth--
	t.writeln("}")
}

// ── Statements ───────────────────────────────────────────────────────────────

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
	case *cc.ExpressionStatement:
		t.translateExprStmt(x)
	case *cc.IterationStatement:
		t.translateIteration(x)
	case *cc.SelectionStatement:
		t.translateSelection(x)
	case *cc.JumpStatement:
		t.translateJump(x)
	case *cc.CompoundStatement:
		t.writeln("{")
		t.depth++
		t.translateCompound(x)
		t.depth--
		t.writeln("}")
	case *cc.CommonDeclaration:
		// local variable declarations (e.g. `int x = 5;`)
		t.translateLocalDecl(x)
	case *cc.LabeledStatement:
		t.translateLabeled(x)
	default:
		t.writeln("/* TODO block item: %T */", item)
	}
}

func (t *translator) translateStatement(stmt cc.Statement) {
	switch x := stmt.(type) {
	case *cc.CompoundStatement:
		// Already inside a function; just translate the body inline
		// (curly braces will be emitted by the parent).
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
	default:
		t.writeln("/* TODO statement: %T */", stmt)
	}
}

// translateBodyStmt handles the body of a control structure.  If the body is
// a CompoundStatement the braces are already emitted by the caller; otherwise
// we add the indented single statement.
func (t *translator) translateBodyStmt(stmt cc.Statement) {
	if cs, ok := stmt.(*cc.CompoundStatement); ok {
		t.translateCompound(cs)
	} else {
		t.depth++
		t.translateStatement(stmt)
		t.depth--
	}
}

// ExpressionStatement: `expr;`
func (t *translator) translateExprStmt(s *cc.ExpressionStatement) {
	if s == nil || s.ExpressionList == nil {
		return
	}
	t.writeln("%s", t.expr(s.ExpressionList))
}

// IterationStatement: while, do-while, for
func (t *translator) translateIteration(s *cc.IterationStatement) {
	switch s.Case {

	case cc.IterationStatementWhile:
		// while (cond) stmt
		cond := t.expr(s.ExpressionList)
		t.writeln("for %s {", cond)
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		t.writeln("}")

	case cc.IterationStatementDo:
		// do stmt while (cond)  → approximate as: for { stmt; if !cond { break } }
		t.writeln("for {")
		t.depth++
		t.translateBodyStmt(s.Statement)
		cond := t.expr(s.ExpressionList)
		t.writeln("if !(%s) { break }", cond)
		t.depth--
		t.writeln("}")

	case cc.IterationStatementFor:
		// for (init; cond; post)
		init := ""
		if s.ExpressionList != nil {
			init = t.expr(s.ExpressionList)
		}
		cond := ""
		if s.ExpressionList2 != nil {
			cond = t.expr(s.ExpressionList2)
		}
		post := ""
		if s.ExpressionList3 != nil {
			post = t.expr(s.ExpressionList3)
		}
		t.writeln("for %s; %s; %s {", init, cond, post)
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		t.writeln("}")

	case cc.IterationStatementForDecl:
		// for (T var = init; cond; post)  — declaration in init
		initDecl := t.forInitDecl(s.Declaration)
		cond := ""
		if s.ExpressionList != nil {
			cond = t.expr(s.ExpressionList)
		}
		post := ""
		if s.ExpressionList2 != nil {
			post = t.expr(s.ExpressionList2)
		}
		t.writeln("for %s; %s; %s {", initDecl, cond, post)
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		t.writeln("}")
	}
}

// forInitDecl converts the declaration part of `for (T v = init; ...; ...)`.
// Returns "v := T(init)" or "v := T" for zero init.
func (t *translator) forInitDecl(decl cc.Declaration) string {
	switch x := decl.(type) {
	case *cc.CommonDeclaration:
		if len(x.InitDeclarators) == 0 {
			return "/* empty decl */"
		}
		id := x.InitDeclarators[0]
		name := id.Declarator.Name()
		golfType := cTypeToGolf(id.Declarator.Type())
		if id.Initializer != nil {
			initSrc := t.expr(id.Initializer.Expression)
			return fmt.Sprintf("%s := %s(%s)", name, golfType, initSrc)
		}
		return fmt.Sprintf("var %s %s", name, golfType)
	case *cc.AutoDeclaration:
		name := x.Declarator.Name()
		golfType := cTypeToGolf(x.Declarator.Type())
		if x.Initializer != nil {
			initSrc := t.expr(x.Initializer.Expression)
			return fmt.Sprintf("%s := %s(%s)", name, golfType, initSrc)
		}
		return fmt.Sprintf("var %s %s", name, golfType)
	default:
		return fmt.Sprintf("/* TODO decl %T */", decl)
	}
}

// SelectionStatement: if / switch
func (t *translator) translateSelection(s *cc.SelectionStatement) {
	switch s.Case {
	case cc.SelectionStatementIf:
		cond := t.expr(s.ExpressionList)
		t.writeln("if %s {", cond)
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		t.writeln("}")
	case cc.SelectionStatementIfElse:
		cond := t.expr(s.ExpressionList)
		t.writeln("if %s {", cond)
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		// Check if else branch is itself an if (else-if chain)
		if inner, ok := s.Statement2.(*cc.SelectionStatement); ok &&
			inner.Case == cc.SelectionStatementIf {
			t.writeln("} else {")
			t.depth++
			t.translateSelection(inner)
			t.depth--
			t.writeln("}")
		} else {
			t.writeln("} else {")
			t.depth++
			t.translateBodyStmt(s.Statement2)
			t.depth--
			t.writeln("}")
		}
	case cc.SelectionStatementSwitch:
		t.writeln("switch %s {", t.expr(s.ExpressionList))
		t.depth++
		t.translateBodyStmt(s.Statement)
		t.depth--
		t.writeln("}")
	}
}

// JumpStatement: return / break / continue
func (t *translator) translateJump(s *cc.JumpStatement) {
	switch s.Case {
	case cc.JumpStatementReturn:
		if s.ExpressionList != nil {
			t.writeln("return %s", t.expr(s.ExpressionList))
		} else {
			t.writeln("return")
		}
	case cc.JumpStatementBreak:
		t.writeln("break")
	case cc.JumpStatementContinue:
		t.writeln("continue")
	default:
		t.writeln("/* TODO jump: %v */", s.Case)
	}
}

// LabeledStatement: case / default / label
func (t *translator) translateLabeled(s *cc.LabeledStatement) {
	switch s.Case {
	case cc.LabeledStatementCaseLabel:
		t.depth--
		t.writeln("case %s:", t.expr(s.Expression))
		t.depth++
		t.translateStatement(s.Statement)
	case cc.LabeledStatementDefault:
		t.depth--
		t.writeln("default:")
		t.depth++
		t.translateStatement(s.Statement)
	case cc.LabeledStatementLabel:
		name := cc.NodeSource(s)
		t.writeln("/* label: %s */", strings.Split(name, ":")[0])
		t.translateStatement(s.Statement)
	}
}

// translateLocalDecl: local variable declaration like `int x = 5;`
func (t *translator) translateLocalDecl(cd *cc.CommonDeclaration) {
	for _, id := range cd.InitDeclarators {
		d := id.Declarator
		name := d.Name()
		if name == "" {
			continue
		}
		// __func__ is a magic compiler-injected variable; skip it.
		if strings.HasPrefix(name, "__") {
			continue
		}
		golfType := cTypeToGolf(d.Type())
		if id.Initializer != nil {
			init := t.expr(id.Initializer.Expression)
			t.writeln("var %s %s = %s(%s)", name, golfType, golfType, init)
		} else {
			t.writeln("var %s %s", name, golfType)
		}
	}
}

// ── Expressions ──────────────────────────────────────────────────────────────
//
// For simple scalar expressions, C and MiniGolf syntax are nearly identical.
// We use cc.NodeSource(n) to retrieve the original C source text and apply
// only the minimal transformations needed.

func (t *translator) expr(n cc.Expression) string {
	if n == nil {
		return ""
	}
	// NodeSource reconstructs the source text from the token stream — almost
	// always valid MiniGolf for simple expressions.
	src := cc.NodeSource(n)
	// Trim trailing semicolons (ExpressionStatement sometimes includes them).
	src = strings.TrimRight(src, ";")
	return strings.TrimSpace(src)
}

// ── Type mapping ─────────────────────────────────────────────────────────────

// cTypeToGolf converts a C type to the corresponding MiniGolf type name.
func cTypeToGolf(t cc.Type) string {
	if t == nil {
		panic("/* nil type */")
	}
	switch t.Kind() {
	case cc.Void:
		return "" // MiniGolf functions with no return type just omit it
	case cc.Int, cc.Short, cc.Long, cc.LongLong,
		cc.SChar, cc.Int8, cc.Int16, cc.Int32, cc.Int64:
		return "int"
	case cc.UInt, cc.UShort, cc.ULong, cc.ULongLong,
		cc.UChar, cc.UInt8, cc.UInt16, cc.UInt32, cc.UInt64:
		return "word" // unsigned → word in MiniGolf
	case cc.Char:
		return "byte"
	case cc.Bool:
		return "bool"
	case cc.Float, cc.Double, cc.LongDouble:
		panic("/* float: unsupported */")
	case cc.Ptr:
		if pt, ok := t.(*cc.PointerType); ok {
			elem := cTypeToGolf(pt.Elem())
			if elem == "" {
				return "*byte" // void* → *byte
			}
			return "*" + elem
		}
		return "*byte"
	case cc.Struct:
		td := t.Typedef()
		if td != nil && td.Name() != "" {
			return td.Name()
		}
		return "/* struct */"
	case cc.Array:
		return "/* array */"
	default:
		// Fall back to what cc prints; caller can fix manually.
		panic("/* " + t.String() + " */")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// isBuiltin returns true for declarations coming from the predefined / system
// sources so they are excluded from translation.
func isBuiltin(d *cc.Declarator) bool {
	pos := d.Position()
	return strings.HasPrefix(pos.Filename, "<") ||
		strings.HasPrefix(d.Name(), "__")
}
