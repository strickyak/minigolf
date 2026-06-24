// cc_demo.go — demo for modernc.org/cc/v5
//
// Usage:
//
//	go run cc_demo.go  [file.c ...]
//	go run cc_demo.go  /path/to/foo.c
//
// It preprocesses, parses, and type-checks each file, then prints a
// structured report covering:
//
//   - Macros defined after preprocessing
//   - Function definitions (name, signature, inline/static flags)
//   - Global variable declarations (name, type, storage class)
//   - typedef declarations
//   - struct/union tags defined at file scope
//
// Build from inside the cc_v5 directory:
//
//	cd cc_v5
//	go run cmd/cc_demo.go  /usr/include/stdio.h
//
// The program belongs to package main but lives inside the cc_v5 module so it
// can import modernc.org/cc/v5 directly.

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	cc "modernc.org/cc/v5"
)

// filterBuiltins skips declarations from the <predefined> source (builtins).
var filterBuiltins = flag.Bool("no-builtins", true,
	"hide __builtin_* and system-predefined declarations")

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: cc_demo [-no-builtins=false] <file.c> [file.c ...]")
		os.Exit(1)
	}

	// ------------------------------------------------------------------ config
	// NewConfig probes the host C compiler (cc / gcc) to obtain predefined
	// macros and system include paths — exactly what a real compiler sees.
	cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fatalf("NewConfig: %v", err)
	}
	// Add the cc/v5 builtin definitions so GCC built-in identifiers resolve.
	cfg.Predefined += cc.Builtin

	for _, path := range args {
		fmt.Printf("══════════════════════════════════════════\n")
		fmt.Printf("  File: %s\n", path)
		fmt.Printf("══════════════════════════════════════════\n")
		analyzeFile(cfg, path)
		fmt.Println()
	}
}

// analyzeFile runs the full pipeline on one source file and prints a report.
func analyzeFile(cfg *cc.Config, path string) {
	// Sources: the file itself, prepended by the predefined macro block.
	// Using a nil Value tells the library to open the file by Name.
	sources := []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: path},
	}

	// Translate = Preprocess + Parse + Type-check.
	// Use Parse instead if you only want the AST without type information.
	ast, err := cc.Translate(cfg, sources)
	if err != nil {
		fmt.Printf("  [ERROR] %v\n", err)
		return
	}

	printStats(ast)
	printMacros(ast)
	printDeclarations(ast)
}

// ── statistics ────────────────────────────────────────────────────────────────

func printStats(ast *cc.AST) {
	nFuncs, nVars, nTypedefs, nTags := 0, 0, 0, 0
	for _, d := range ast.Declarations {
		switch x := d.(type) {
		case *cc.FunctionDefinition:
			if x.Declarator.IsFuncDef() {
				nFuncs++
			}
		case *cc.CommonDeclaration:
			for _, id := range x.InitDeclarators {
				decl := id.Declarator
				switch {
				case decl.IsTypename():
					nTypedefs++
				case decl.Type().Kind() == cc.Function:
					// forward declaration of a function
				default:
					nVars++
				}
			}
		}
	}
	// Count struct/union/enum tags in file scope.
	for _, nodes := range ast.Scope.Nodes {
		for _, n := range nodes {
			switch n.(type) {
			case *cc.StructOrUnionSpecifier, *cc.EnumSpecifier:
				nTags++
			}
		}
	}
	fmt.Printf("  Stats: %d function def(s), %d global var(s), %d typedef(s), %d tag(s)\n",
		nFuncs, nVars, nTypedefs, nTags)
	fmt.Printf("  Macros defined: %d\n", len(ast.Macros))
}

// ── macros ────────────────────────────────────────────────────────────────────

func printMacros(ast *cc.AST) {
	// Collect only user-visible macros (skip internal __ prefixed ones for brevity).
	var names []string
	for name := range ast.Macros {
		if !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	fmt.Printf("\n  ── Macros (non-__ prefixed) ──\n")
	for _, name := range names {
		m := ast.Macros[name]
		// Body is available via NodeSource; just show the name here.
		_ = m
		fmt.Printf("    #define %s\n", name)
	}
}

// ── declarations ──────────────────────────────────────────────────────────────

// isBuiltinPos returns true if the position comes from <predefined> / system
// include paths and the symbol name starts with __ — i.e. internal noise.
func isBuiltinDecl(d *cc.Declarator) bool {
	if !*filterBuiltins {
		return false
	}
	pos := d.Position()
	name := d.Name()
	if strings.HasPrefix(pos.Filename, "<predefined") {
		return true
	}
	if strings.HasPrefix(name, "__") {
		return true
	}
	return false
}

func printDeclarations(ast *cc.AST) {
	fmt.Printf("\n  ── Top-level declarations ──\n")
	for _, d := range ast.Declarations {
		switch x := d.(type) {
		case *cc.FunctionDefinition:
			if !isBuiltinDecl(x.Declarator) {
				printFuncDef(x)
			}
		case *cc.CommonDeclaration:
			printCommonDecl(x)
		case *cc.StaticAssertDeclaration:
			fmt.Printf("    [static_assert] %s\n", x.Position())
		}
	}
}

func printFuncDef(f *cc.FunctionDefinition) {
	d := f.Declarator
	pos := d.Position()
	name := d.Name()

	flags := []string{}
	if d.IsInline() {
		flags = append(flags, "inline")
	}
	if d.IsStatic() {
		flags = append(flags, "static")
	}
	if d.IsExtern() {
		flags = append(flags, "extern")
	}
	flagStr := ""
	if len(flags) > 0 {
		flagStr = " [" + strings.Join(flags, ",") + "]"
	}

	// Build a human-readable signature.
	sig := funcSignature(d)

	fmt.Printf("    func%s  %s  %s  (at %s:%d)\n",
		flagStr, name, sig, pos.Filename, pos.Line)
}

// funcSignature returns "( paramTypes... ) -> returnType" for a function declarator.
func funcSignature(d *cc.Declarator) string {
	t := d.Type()
	if t == nil || t.Kind() != cc.Function {
		return d.Type().String()
	}
	ft, ok := t.(*cc.FunctionType)
	if !ok {
		return t.String()
	}
	params := ft.Parameters()
	parts := make([]string, 0, len(params))
	for _, p := range params {
		pname := p.Name()
		ptype := p.Type().String()
		if pname != "" {
			parts = append(parts, pname+" "+ptype)
		} else {
			parts = append(parts, ptype)
		}
	}
	paramStr := strings.Join(parts, ", ")
	if ft.IsVariadic() {
		if paramStr != "" {
			paramStr += ", ..."
		} else {
			paramStr = "..."
		}
	}
	retStr := ft.Result().String()
	return fmt.Sprintf("(%s) -> %s", paramStr, retStr)
}

func printCommonDecl(cd *cc.CommonDeclaration) {
	for _, id := range cd.InitDeclarators {
		d := id.Declarator
		if isBuiltinDecl(d) {
			continue
		}
		pos := d.Position()
		name := d.Name()
		if name == "" || strings.HasPrefix(name, "__predefined") {
			continue // skip synthetic injected declarations
		}
		t := d.Type()
		kind := t.Kind()

		switch {
		case d.IsTypename():
			fmt.Printf("    typedef  %s  =  %s  (at %s:%d)\n",
				name, t.String(), pos.Filename, pos.Line)

		case kind == cc.Function:
			// Forward declaration / prototype.
			sig := funcSignature(d)
			fmt.Printf("    proto    %s  %s  (at %s:%d)\n",
				name, sig, pos.Filename, pos.Line)

		case kind == cc.Struct || kind == cc.Union:
			fmt.Printf("    var      %s  %s{...}  (at %s:%d)\n",
				name, kindStr(kind), pos.Filename, pos.Line)

		default:
			storClass := storageClass(d)
			fmt.Printf("    var%s   %s  %s  (at %s:%d)\n",
				storClass, name, t.String(), pos.Filename, pos.Line)
		}
	}
}

func kindStr(k cc.Kind) string {
	switch k {
	case cc.Struct:
		return "struct"
	case cc.Union:
		return "union"
	default:
		return k.String()
	}
}

func storageClass(d *cc.Declarator) string {
	switch {
	case d.IsStatic():
		return " [static]"
	case d.IsExtern():
		return " [extern]"
	case d.IsRegister():
		return " [register]"
	default:
		return ""
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "cc_demo: "+format+"\n", args...)
	os.Exit(1)
}
