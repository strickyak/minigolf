package ast_test

import (
	"testing"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/semantic"
)

func TestPopularityPropagation(t *testing.T) {
	src := `
package main

func leaf1() {
}

func leaf2() {
}

func middle() {
	leaf1()
	for i := 0; i < 5; i++ {
		leaf2()
	}
}

func main() {
	middle()
	middle()
}
`
	tokens := lexer.Lex(src, "test.golf")
	p := parser.New(tokens)
	program := p.ParseProgram("main")
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	resolver := semantic.NewResolver(nil)
	resolver.Resolve(program)
	analyzer := semantic.New(resolver)
	analyzer.Analyze(program)
	if len(analyzer.Errors()) > 0 {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	program.MarkTrunkFunctions(analyzer.ResolveFunc)

	// Expected popularities:
	// main: 1 (always)
	// middle: main calls it twice at depth 0. Pop = 2
	// leaf1: middle calls it once at depth 0. Pop = caller_pop * (1<<0) = 2 * 1 = 2
	// leaf2: middle calls it once at depth 1. Pop = caller_pop * (1<<2) = 2 * 4 = 8

	expected := map[string]int{
		"main":   1,
		"middle": 2,
		"leaf1":  2,
		"leaf2":  8,
	}

	for _, stmt := range program.Statements {
		if fs, ok := stmt.(*ast.FuncStatement); ok {
			name := fs.Name.Value
			exp, ok := expected[name]
			if !ok {
				continue
			}
			if fs.Popularity != exp {
				t.Errorf("Expected popularity of %s to be %d, got %d", name, exp, fs.Popularity)
			}
		}
	}
}

func TestPopularityWithRecursion(t *testing.T) {
	src := `
package main

func rec(n word) {
	if n > 0 {
		rec(n - 1)
	}
}

func main() {
	rec(5)
}
`
	tokens := lexer.Lex(src, "test.golf")
	p := parser.New(tokens)
	program := p.ParseProgram("main")
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	resolver := semantic.NewResolver(nil)
	resolver.Resolve(program)
	analyzer := semantic.New(resolver)
	analyzer.Analyze(program)
	if len(analyzer.Errors()) > 0 {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	program.MarkTrunkFunctions(analyzer.ResolveFunc)

	expected := map[string]int{
		"main": 1,
		"rec":  1,
	}

	for _, stmt := range program.Statements {
		if fs, ok := stmt.(*ast.FuncStatement); ok {
			name := fs.Name.Value
			exp, ok := expected[name]
			if !ok {
				continue
			}
			if fs.Popularity != exp {
				t.Errorf("Expected popularity of %s to be %d, got %d", name, exp, fs.Popularity)
			}
		}
	}
}

func TestPopularityWithMutualRecursion(t *testing.T) {
	src := `
package main

func ping(n word) {
	if n > 0 {
		pong(n - 1)
	}
}

func pong(n word) {
	if n > 0 {
		ping(n - 1)
	}
}

func main() {
	ping(5)
}
`
	tokens := lexer.Lex(src, "test.golf")
	p := parser.New(tokens)
	program := p.ParseProgram("main")
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	resolver := semantic.NewResolver(nil)
	resolver.Resolve(program)
	analyzer := semantic.New(resolver)
	analyzer.Analyze(program)
	if len(analyzer.Errors()) > 0 {
		t.Fatalf("semantic errors: %v", analyzer.Errors())
	}

	program.MarkTrunkFunctions(analyzer.ResolveFunc)

	expected := map[string]int{
		"main": 1,
		"ping": 1,
		"pong": 1,
	}

	for _, stmt := range program.Statements {
		if fs, ok := stmt.(*ast.FuncStatement); ok {
			name := fs.Name.Value
			exp, ok := expected[name]
			if !ok {
				continue
			}
			if fs.Popularity != exp {
				t.Errorf("Expected popularity of %s to be %d, got %d", name, exp, fs.Popularity)
			}
		}
	}
}
