package transpiler

import (
	"strings"
	"testing"

	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/semantic"
)

func TestTranspiler(t *testing.T) {
	input := `package main
	
	import "fmt"
	
	const limit = 10
	
	type index word
	
	var count byte = 0
	
	func sum(a word, b word) word {
		return a + b
	}
	
	func main() {
		var x word = 5
		y := byte(10)
		
		for x < limit {
			x = x + 1
			count = count + 1
		}
		
		print("hello world", x)
		println("sum is", sum(x, word(y)))
	}`

	tokens := lexer.Lex(input, "<test>")
	p := parser.New(tokens)
	program := p.ParseProgram("")

	if len(p.Errors()) > 0 {
		t.Fatalf("Parser errors: %v", p.Errors())
	}

	res := semantic.NewResolver()
	res.Resolve(program)

	tr := New()
	output := tr.Transpile(program, nil)

	if len(output) == 0 {
		t.Fatalf("Transpiler returned empty string")
	}

	expectedSubstrings := []string{
		"typedef uint8_t byte;",
		"typedef uintptr_t word;",
		"typedef word t_main_index;",
		"word f_main_sum(word v_a, word v_b);",
		"void f_main_main();",
		"#define v_main_limit 10",
		"byte v_main_count = 0;",
		"while ((v_x < v_main_limit))",
		"printf(\"hello world %llu\", (unsigned long long)(v_x));",
		"printf(\"sum is %llu\\n\", (unsigned long long)(f_main_sum(v_x, ((word)(v_y)))));",
		"int main() {",
		"\tf_main_main();",
		"\treturn 0;",
		"}",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(output, sub) {
			t.Errorf("expected output to contain: \n%s\nOutput was:\n%s", sub, output)
		}
	}
}
