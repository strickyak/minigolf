package parser

import (
	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/lexer"
	"strconv"
	"strings"
	"testing"
)

func checkParserErrors(t *testing.T, p *Parser) {
	errors := p.Errors()
	if len(errors) == 0 {
		return
	}

	t.Errorf("parser has %d errors", len(errors))
	for _, msg := range errors {
		t.Errorf("parser error: %q", msg)
	}
	t.FailNow()
}

func TestPackageStatement(t *testing.T) {
	input := `package main`
	tokens := lexer.Lex(input, "<test>")
	p := New(tokens)
	program := p.ParseProgram("")
	checkParserErrors(t, p)

	if program == nil {
		t.Fatalf("ParseProgram() returned nil")
	}
	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statements. got=%d", len(program.Statements))
	}

	stmt, ok := program.Statements[0].(*ast.PackageStatement)
	if !ok {
		t.Fatalf("stmt is not ast.PackageStatement. got=%T", program.Statements[0])
	}

	if stmt.Name.Value != "main" {
		t.Errorf("stmt.Name.Value not '%s'. got=%s", "main", stmt.Name.Value)
	}
}

func TestVarStatements(t *testing.T) {
	input := `
	var x word = 5;
	var y byte;
	var z = 10
	`
	tokens := lexer.Lex(input, "<test>")
	p := New(tokens)
	program := p.ParseProgram("")
	checkParserErrors(t, p)

	if len(program.Statements) != 3 {
		t.Fatalf("program.Statements does not contain 3 statements. got=%d", len(program.Statements))
	}

	tests := []struct {
		expectedIdentifier string
		expectedValueType  string
	}{
		{"x", "word"},
		{"y", "byte"},
		{"z", ""},
	}

	for i, tt := range tests {
		stmt := program.Statements[i]
		varStmt, ok := stmt.(*ast.VarStatement)
		if !ok {
			t.Errorf("stmt not *ast.VarStatement. got=%T", stmt)
			continue
		}

		if varStmt.Name.Value != tt.expectedIdentifier {
			t.Errorf("varStmt.Name.Value not '%s'. got=%s", tt.expectedIdentifier, varStmt.Name.Value)
		}

		if tt.expectedValueType != "" {
			if varStmt.ValueType == nil {
				t.Errorf("varStmt.ValueType is nil, expected %s", tt.expectedValueType)
			} else if varStmt.ValueType.TokenLiteral() != tt.expectedValueType {
				t.Errorf("varStmt.ValueType.TokenLiteral() not '%s'. got=%s", tt.expectedValueType, varStmt.ValueType.TokenLiteral())
			}
		} else {
			if varStmt.ValueType != nil {
				t.Errorf("varStmt.ValueType is not nil, got %s", varStmt.ValueType.TokenLiteral())
			}
		}
	}
}

func TestAssignStatement(t *testing.T) {
	input := `
	func test() {
		x = 5
		x, y = 1, 2
		z := 10
	}
	`
	tokens := lexer.Lex(input, "<test>")
	p := New(tokens)
	program := p.ParseProgram("")
	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statements. got=%d", len(program.Statements))
	}

	funcStmt, ok := program.Statements[0].(*ast.FuncStatement)
	if !ok {
		t.Fatalf("stmt is not ast.FuncStatement")
	}

	if len(funcStmt.Body.Statements) != 3 {
		t.Fatalf("body should have 3 statements, got %d", len(funcStmt.Body.Statements))
	}

	// 1. x = 5
	assign1, ok := funcStmt.Body.Statements[0].(*ast.AssignStatement)
	if !ok {
		t.Fatalf("stmt 0 is not AssignStatement")
	}
	if len(assign1.Names) != 1 || assign1.Names[0].(*ast.Identifier).Value != "x" {
		t.Errorf("assign1 name is not x")
	}
	if len(assign1.Values) != 1 {
		t.Errorf("assign1 values len is not 1")
	}

	// 2. x, y = 1, 2
	assign2, ok := funcStmt.Body.Statements[1].(*ast.AssignStatement)
	if !ok {
		t.Fatalf("stmt 1 is not AssignStatement")
	}
	if len(assign2.Names) != 2 || assign2.Names[0].(*ast.Identifier).Value != "x" || assign2.Names[1].(*ast.Identifier).Value != "y" {
		t.Errorf("assign2 names incorrect")
	}
	if len(assign2.Values) != 2 {
		t.Errorf("assign2 values len is not 2")
	}

	// 3. z := 10
	assign3, ok := funcStmt.Body.Statements[2].(*ast.AssignStatement)
	if !ok {
		t.Fatalf("stmt 2 is not AssignStatement")
	}
	if len(assign3.Names) != 1 || assign3.Names[0].(*ast.Identifier).Value != "z" {
		t.Errorf("assign3 name is not z")
	}
	if assign3.Token.Literal != ":=" {
		t.Errorf("assign3 token literal is not :=, got %s", assign3.Token.Literal)
	}
}

// ASTString recursively builds a string representation of an expression node
func ASTString(node ast.Node) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *ast.Identifier:
		return n.Value
	case *ast.IntegerLiteral:
		return strconv.FormatInt(n.Value, 10)
	case *ast.StringLiteral:
		return "\"" + n.Value + "\""
	case *ast.PrefixExpression:
		return "(" + n.Operator + ASTString(n.Right) + ")"
	case *ast.InfixExpression:
		return "(" + ASTString(n.Left) + " " + n.Operator + " " + ASTString(n.Right) + ")"
	case *ast.CallExpression:
		args := []string{}
		for _, a := range n.Arguments {
			args = append(args, ASTString(a))
		}
		return ASTString(n.Function) + "(" + strings.Join(args, ", ") + ")"
	case *ast.ExpressionStatement:
		return ASTString(n.Expression)
	}
	return ""
}

func TestOperatorPrecedenceParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"-a * b",
			"((-a) * b)",
		},
		{
			"!-a",
			"(!(-a))",
		},
		{
			"a + b + c",
			"((a + b) + c)",
		},
		{
			"a + b - c",
			"((a + b) - c)",
		},
		{
			"a * b * c",
			"((a * b) * c)",
		},
		{
			"a * b / c",
			"((a * b) / c)",
		},
		{
			"a + b / c",
			"(a + (b / c))",
		},
		{
			"a + b * c + d / e - f",
			"(((a + (b * c)) + (d / e)) - f)",
		},
		{
			"1 + (2 + 3) + 4",
			"((1 + (2 + 3)) + 4)",
		},
		{
			"a + funcName(b * c) + d",
			"((a + funcName((b * c))) + d)",
		},
		{
			"byte(x) + word(10)",
			"(byte(x) + word(10))",
		},
	}

	for _, tt := range tests {
		input := "func test() { " + tt.input + " }"
		tokens := lexer.Lex(input, "<test>")
		p := New(tokens)
		program := p.ParseProgram("")
		checkParserErrors(t, p)

		funcStmt := program.Statements[0].(*ast.FuncStatement)
		stmt := funcStmt.Body.Statements[0].(*ast.ExpressionStatement)

		actual := ASTString(stmt)
		if actual != tt.expected {
			t.Errorf("expected=%q, got=%q", tt.expected, actual)
		}
	}
}
