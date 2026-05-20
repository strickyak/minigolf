package lexer

import (
	"strconv"

	"github.com/strickyak/minigolf/token"
)

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int
	filename     string
}

func New(input string, filename string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0, filename: filename}
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
			tok = token.Token{Type: token.EQ, Literal: "==", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.ASSIGN, l.ch, startLine, startCol)
		}
	case ':':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.DECLARE, Literal: ":=", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.COLON, l.ch, startLine, startCol)
		}
	case '+':
		if l.peekChar() == '+' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.INC, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1, Filename: l.filename}
		} else {
			tok = l.newToken(token.PLUS, l.ch, l.line, l.column)
		}
	case '-':
		if l.peekChar() == '-' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.DEC, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1, Filename: l.filename}
		} else {
			tok = l.newToken(token.MINUS, l.ch, l.line, l.column)
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.NEQ, Literal: "!=", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.BANG, l.ch, startLine, startCol)
		}
	case '/':
		if l.peekChar() == '/' {
			l.skipSingleLineComment()
			return l.nextToken()
		} else if l.peekChar() == '*' {
			l.skipMultiLineComment()
			return l.nextToken()
		} else {
			tok = l.newToken(token.SLASH, l.ch, startLine, startCol)
		}
	case '*':
		tok = l.newToken(token.ASTERISK, l.ch, startLine, startCol)
	case '%':
		tok = l.newToken(token.MOD, l.ch, startLine, startCol)
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.LTE, Literal: "<=", Line: startLine, Column: startCol, Filename: l.filename}
		} else if l.peekChar() == '<' {
			l.readChar()
			tok = token.Token{Type: token.LSHIFT, Literal: "<<", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.LT, l.ch, startLine, startCol)
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.GTE, Literal: ">=", Line: startLine, Column: startCol, Filename: l.filename}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = token.Token{Type: token.RSHIFT, Literal: ">>", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.GT, l.ch, startLine, startCol)
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			tok = token.Token{Type: token.AND, Literal: "&&", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.BIT_AND, l.ch, startLine, startCol)
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok = token.Token{Type: token.OR, Literal: "||", Line: startLine, Column: startCol, Filename: l.filename}
		} else {
			tok = l.newToken(token.BIT_OR, l.ch, startLine, startCol)
		}
	case '^':
		tok = l.newToken(token.BIT_XOR, l.ch, startLine, startCol)
	case ';':
		tok = l.newToken(token.SEMICOLON, l.ch, startLine, startCol)
	case ',':
		tok = l.newToken(token.COMMA, l.ch, startLine, startCol)
	case '.':
		tok = l.newToken(token.DOT, l.ch, startLine, startCol)
	case '(':
		tok = l.newToken(token.LPAREN, l.ch, startLine, startCol)
	case ')':
		tok = l.newToken(token.RPAREN, l.ch, startLine, startCol)
	case '{':
		tok = l.newToken(token.LBRACE, l.ch, startLine, startCol)
	case '}':
		tok = l.newToken(token.RBRACE, l.ch, startLine, startCol)
	case '[':
		tok = l.newToken(token.LBRACKET, l.ch, startLine, startCol)
	case ']':
		tok = l.newToken(token.RBRACKET, l.ch, startLine, startCol)
	case '"':
		tok.Type = token.STRING
		tok.Literal = l.readString()
		tok.Line = startLine
		tok.Column = startCol
	case '\'':
		l.readChar() // consume opening '
		var charVal byte
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				charVal = '\n'
			case 't':
				charVal = '\t'
			case 'r':
				charVal = '\r'
			case '\\':
				charVal = '\\'
			case '\'':
				charVal = '\''
			default:
				charVal = l.ch
			}
		} else {
			charVal = l.ch
		}
		tok.Type = token.INT
		tok.Literal = strconv.Itoa(int(charVal))
		tok.Line = startLine
		tok.Column = startCol
		l.readChar() // consume the character
		if l.ch != '\'' {
			tok = l.newToken(token.ILLEGAL, l.ch, startLine, startCol)
		}
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
			tok = l.newToken(token.ILLEGAL, l.ch, startLine, startCol)
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) newToken(tokenType token.TokenType, ch byte, line, col int) token.Token {
	return token.Token{Type: tokenType, Literal: string(ch), Line: line, Column: col, Filename: l.filename}
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

func Lex(input, filename string) []token.Token {
	l := New(input, filename)
	var tokens []token.Token
	var prev token.Token

	for {
		tok := l.nextToken()

		// Automatic Semicolon Insertion (ASI)
		if len(tokens) > 0 {
			if tok.Line > prev.Line || tok.Type == token.EOF {
				if prev.Type == token.IDENT || prev.Type == token.INT || prev.Type == token.STRING ||
					prev.Type == token.RETURN || prev.Type == token.RPAREN || prev.Type == token.RBRACE || prev.Type == token.RBRACKET {
					tokens = append(tokens, token.Token{Type: token.SEMICOLON, Literal: ";", Line: prev.Line, Column: prev.Column + len(prev.Literal), Filename: l.filename})
				}
			}
		}

		tokens = append(tokens, tok)
		prev = tok
		if tok.Type == token.EOF {
			break
		}
	}
	return tokens
}
