package token

type TokenType string

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

const (
	ILLEGAL = "ILLEGAL"
	EOF     = "EOF"

	// Identifiers + literals
	IDENT  = "IDENT"  // add, foobar, x, y, ...
	INT    = "INT"    // 1343456
	STRING = "STRING" // "hello world"

	// Operators
	ASSIGN   = "="
	DECLARE  = ":="
	PLUS     = "+"
	MINUS    = "-"
	BANG     = "!"
	ASTERISK = "*"
	SLASH    = "/"
	MOD      = "%"
	BIT_AND  = "&"
	BIT_OR   = "|"
	BIT_XOR  = "^"
	LSHIFT   = "<<"
	RSHIFT   = ">>"

	LT  = "<"
	GT  = ">"
	EQ  = "=="
	NEQ = "!="
	LTE = "<="
	GTE = ">="

	// Delimiters
	COMMA     = ","
	SEMICOLON = ";"
	COLON     = ":"

	LPAREN = "("
	RPAREN = ")"
	LBRACE = "{"
	RBRACE = "}"

	// Keywords
	PACKAGE = "PACKAGE"
	IMPORT  = "IMPORT"
	CONST   = "CONST"
	TYPE    = "TYPE"
	VAR     = "VAR"
	FUNC    = "FUNC"
	IF      = "IF"
	ELSE    = "ELSE"
	FOR     = "FOR"
	RETURN  = "RETURN"
)

var keywords = map[string]TokenType{
	"package": PACKAGE,
	"import":  IMPORT,
	"const":   CONST,
	"type":    TYPE,
	"var":     VAR,
	"func":    FUNC,
	"if":      IF,
	"else":    ELSE,
	"for":     FOR,
	"return":  RETURN,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
