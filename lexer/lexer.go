package lexer

import (
	"minigo/token"
)

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int
}

func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	} else {
		return l.input[l.readPosition]
	}
}

func (l *Lexer) nextToken() token.Token {
	var tok token.Token

	l.skipWhitespace()

	startLine := l.line
	startCol := l.column

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.EQ, Literal: "==", Line: startLine, Column: startCol}
		} else {
			tok = newToken(token.ASSIGN, l.ch, startLine, startCol)
		}
	case ':':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.DECLARE, Literal: ":=", Line: startLine, Column: startCol}
		} else {
			tok = newToken(token.COLON, l.ch, startLine, startCol)
		}
	case '+':
		tok = newToken(token.PLUS, l.ch, startLine, startCol)
	case '-':
		tok = newToken(token.MINUS, l.ch, startLine, startCol)
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.NEQ, Literal: "!=", Line: startLine, Column: startCol}
		} else {
			tok = newToken(token.BANG, l.ch, startLine, startCol)
		}
	case '/':
		if l.peekChar() == '/' {
			l.skipSingleLineComment()
			return l.nextToken()
		} else if l.peekChar() == '*' {
			l.skipMultiLineComment()
			return l.nextToken()
		} else {
			tok = newToken(token.SLASH, l.ch, startLine, startCol)
		}
	case '*':
		tok = newToken(token.ASTERISK, l.ch, startLine, startCol)
	case '%':
		tok = newToken(token.MOD, l.ch, startLine, startCol)
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.LTE, Literal: "<=", Line: startLine, Column: startCol}
		} else if l.peekChar() == '<' {
			l.readChar()
			tok = token.Token{Type: token.LSHIFT, Literal: "<<", Line: startLine, Column: startCol}
		} else {
			tok = newToken(token.LT, l.ch, startLine, startCol)
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.GTE, Literal: ">=", Line: startLine, Column: startCol}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = token.Token{Type: token.RSHIFT, Literal: ">>", Line: startLine, Column: startCol}
		} else {
			tok = newToken(token.GT, l.ch, startLine, startCol)
		}
	case '&':
		tok = newToken(token.BIT_AND, l.ch, startLine, startCol)
	case '|':
		tok = newToken(token.BIT_OR, l.ch, startLine, startCol)
	case '^':
		tok = newToken(token.BIT_XOR, l.ch, startLine, startCol)
	case ';':
		tok = newToken(token.SEMICOLON, l.ch, startLine, startCol)
	case ',':
		tok = newToken(token.COMMA, l.ch, startLine, startCol)
	case '(':
		tok = newToken(token.LPAREN, l.ch, startLine, startCol)
	case ')':
		tok = newToken(token.RPAREN, l.ch, startLine, startCol)
	case '{':
		tok = newToken(token.LBRACE, l.ch, startLine, startCol)
	case '}':
		tok = newToken(token.RBRACE, l.ch, startLine, startCol)
	case '[':
		tok = newToken(token.LBRACKET, l.ch, startLine, startCol)
	case ']':
		tok = newToken(token.RBRACKET, l.ch, startLine, startCol)
	case '"':
		tok.Type = token.STRING
		tok.Literal = l.readString()
		tok.Line = startLine
		tok.Column = startCol
	case 0:
		tok.Literal = ""
		tok.Type = token.EOF
		tok.Line = startLine
		tok.Column = startCol
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			tok.Line = startLine
			tok.Column = startCol
			return tok
		} else if isDigit(l.ch) {
			tok.Type = token.INT
			tok.Literal = l.readNumber()
			tok.Line = startLine
			tok.Column = startCol
			return tok
		} else {
			tok = newToken(token.ILLEGAL, l.ch, startLine, startCol)
		}
	}

	l.readChar()
	return tok
}

func newToken(tokenType token.TokenType, ch byte, line, col int) token.Token {
	return token.Token{Type: tokenType, Literal: string(ch), Line: line, Column: col}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() string {
	position := l.position
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readString() string {
	position := l.position + 1
	for {
		l.readChar()
		if l.ch == '"' || l.ch == 0 {
			break
		}
	}
	return l.input[position:l.position]
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		if l.ch == '\n' {
			l.line++
			l.column = 0 // Will become 1 on next readChar (or 0 if EOF)
		}
		l.readChar()
	}
}

func (l *Lexer) skipSingleLineComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

func (l *Lexer) skipMultiLineComment() {
	for {
		l.readChar()
		if l.ch == 0 {
			break
		}
		if l.ch == '\n' {
			l.line++
			l.column = 0
		}
		if l.ch == '*' && l.peekChar() == '/' {
			l.readChar() // consume '*'
			l.readChar() // consume '/'
			break
		}
	}
}

// Lex consumes the entire input string and returns a slice of all tokens.
// This takes advantage of the "big memory" assumption, avoiding streams.
func Lex(input string) []token.Token {
	l := New(input)
	var tokens []token.Token
	for {
		tok := l.nextToken()
		tokens = append(tokens, tok)
		if tok.Type == token.EOF {
			break
		}
	}
	return tokens
}
