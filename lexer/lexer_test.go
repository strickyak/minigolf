package lexer

import (
	"minigo/token"
	"testing"
)

func TestLexerBasic(t *testing.T) {
	input := `=+(){},;`

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.ASSIGN, "="},
		{token.PLUS, "+"},
		{token.LPAREN, "("},
		{token.RPAREN, ")"},
		{token.LBRACE, "{"},
		{token.RBRACE, "}"},
		{token.COMMA, ","},
		{token.SEMICOLON, ";"},
		{token.EOF, ""},
	}

	tokens := Lex(input)

	if len(tokens) != len(tests) {
		t.Fatalf("lengths differ. expected=%d, got=%d", len(tests), len(tokens))
	}

	for i, tt := range tests {
		tok := tokens[i]

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestLexerMiniGoCode(t *testing.T) {
	input := `package main

import "fmt"

const myConst = 10

type myWord word

var globalVar byte = 5

func main() {
	var x word = 5
	y := byte(10)
	
	if x != 5 {
		return
	} else {
		// single line comment
		y = y + 1 /* multiline
		comment */
	}

	for x < 10 {
		x = x + 1
	}
	
	print("hello", x)
	println(y)
}
`

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.PACKAGE, "package"},
		{token.IDENT, "main"},

		{token.IMPORT, "import"},
		{token.STRING, "fmt"},

		{token.CONST, "const"},
		{token.IDENT, "myConst"},
		{token.ASSIGN, "="},
		{token.INT, "10"},

		{token.TYPE, "type"},
		{token.IDENT, "myWord"},
		{token.IDENT, "word"},

		{token.VAR, "var"},
		{token.IDENT, "globalVar"},
		{token.IDENT, "byte"},
		{token.ASSIGN, "="},
		{token.INT, "5"},

		{token.FUNC, "func"},
		{token.IDENT, "main"},
		{token.LPAREN, "("},
		{token.RPAREN, ")"},
		{token.LBRACE, "{"},

		{token.VAR, "var"},
		{token.IDENT, "x"},
		{token.IDENT, "word"},
		{token.ASSIGN, "="},
		{token.INT, "5"},

		{token.IDENT, "y"},
		{token.DECLARE, ":="},
		{token.IDENT, "byte"},
		{token.LPAREN, "("},
		{token.INT, "10"},
		{token.RPAREN, ")"},

		{token.IF, "if"},
		{token.IDENT, "x"},
		{token.NEQ, "!="},
		{token.INT, "5"},
		{token.LBRACE, "{"},

		{token.RETURN, "return"},

		{token.RBRACE, "}"},
		{token.ELSE, "else"},
		{token.LBRACE, "{"},

		{token.IDENT, "y"},
		{token.ASSIGN, "="},
		{token.IDENT, "y"},
		{token.PLUS, "+"},
		{token.INT, "1"},

		{token.RBRACE, "}"},

		{token.FOR, "for"},
		{token.IDENT, "x"},
		{token.LT, "<"},
		{token.INT, "10"},
		{token.LBRACE, "{"},

		{token.IDENT, "x"},
		{token.ASSIGN, "="},
		{token.IDENT, "x"},
		{token.PLUS, "+"},
		{token.INT, "1"},

		{token.RBRACE, "}"},

		{token.IDENT, "print"},
		{token.LPAREN, "("},
		{token.STRING, "hello"},
		{token.COMMA, ","},
		{token.IDENT, "x"},
		{token.RPAREN, ")"},

		{token.IDENT, "println"},
		{token.LPAREN, "("},
		{token.IDENT, "y"},
		{token.RPAREN, ")"},

		{token.RBRACE, "}"},
		{token.EOF, ""},
	}

	tokens := Lex(input)

	if len(tokens) != len(tests) {
		t.Fatalf("lengths differ. expected=%d, got=%d", len(tests), len(tokens))
	}

	for i, tt := range tests {
		tok := tokens[i]

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q (literal: %q)",
				i, tt.expectedType, tok.Type, tok.Literal)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestLexerLineAndColumn(t *testing.T) {
	input := "var x = 1\ny = 2"
	
	tests := []struct {
		expectedType token.TokenType
		expectedLine int
		expectedCol  int
	}{
		{token.VAR, 1, 1},
		{token.IDENT, 1, 5},
		{token.ASSIGN, 1, 7},
		{token.INT, 1, 9},
		{token.IDENT, 2, 1},
		{token.ASSIGN, 2, 3},
		{token.INT, 2, 5},
		{token.EOF, 2, 6},
	}
	
	tokens := Lex(input)

	if len(tokens) != len(tests) {
		t.Fatalf("lengths differ. expected=%d, got=%d", len(tests), len(tokens))
	}

	for i, tt := range tests {
		tok := tokens[i]

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Line != tt.expectedLine {
			t.Fatalf("tests[%d] - line wrong for %q. expected=%d, got=%d",
				i, tok.Literal, tt.expectedLine, tok.Line)
		}
		
		if tok.Column != tt.expectedCol {
			t.Fatalf("tests[%d] - column wrong for %q. expected=%d, got=%d",
				i, tok.Literal, tt.expectedCol, tok.Column)
		}
	}
}
