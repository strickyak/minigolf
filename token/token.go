package token

type TokenType string

type Token struct {
	Type         TokenType
	Literal      string
	Line         int
	Column       int
	Filename     string
	ExpandedFrom string
}

const (
	ILLEGAL = "ILLEGAL"
	EOF     = "EOF"

	// Identifiers + literals
	IDENT  = "IDENT"  // add, foobar, x, y, ...
	INT    = "INT"    // 1343456
	STRING = "STRING" // "hello world"

	// Operators
	ASSIGN    = "="
	DECLARE   = ":="
	PLUS      = "+"
	INC       = "++"
	MINUS     = "-"
	DEC       = "--"
	BANG      = "!"
	ASTERISK  = "*"
	SLASH     = "/"
	MOD       = "%"
	BIT_AND   = "&"
	AND       = "&&"
	BIT_OR    = "|"
	OR        = "||"
	BIT_XOR   = "^"
	LSHIFT    = "<<"
	RSHIFT    = ">>"
	BIT_CLEAR = "&^"

	LT  = "<"
	GT  = ">"
	EQ  = "=="
	NEQ = "!="
	LTE = "<="
	GTE = ">="

	ADD_ASSIGN   = "+="
	SUB_ASSIGN   = "-="
	MUL_ASSIGN   = "*="
	DIV_ASSIGN   = "/="
	MOD_ASSIGN   = "%="
	AND_ASSIGN   = "&="
	OR_ASSIGN    = "|="
	XOR_ASSIGN   = "^="
	SHL_ASSIGN   = "<<="
	SHR_ASSIGN   = ">>="
	CLEAR_ASSIGN = "&^="

	// Delimiters
	COMMA     = ","
	DOT       = "."
	ELLIPSIS  = "..."
	SEMICOLON = ";"
	COLON     = ":"

	LPAREN = "("
	RPAREN = ")"
	LBRACE = "{"
	RBRACE = "}"

	LBRACKET = "["
	RBRACKET = "]"

	// Keywords
	PACKAGE  = "PACKAGE"
	IMPORT   = "IMPORT"
	CONST    = "CONST"
	TYPE     = "TYPE"
	STRUCT   = "STRUCT"
	VAR      = "VAR"
	FUNC     = "FUNC"
	IF       = "IF"
	ELSE     = "ELSE"
	FOR      = "FOR"
	RANGE    = "RANGE"
	RETURN   = "RETURN"
	BREAK    = "BREAK"
	CONTINUE = "CONTINUE"
	DEFER    = "DEFER"
	GOTO     = "GOTO"

	PRAGMA = "PRAGMA"
	NIL    = "NIL"
)

var keywords = map[string]TokenType{
	"package":  PACKAGE,
	"import":   IMPORT,
	"const":    CONST,
	"type":     TYPE,
	"struct":   STRUCT,
	"var":      VAR,
	"func":     FUNC,
	"if":       IF,
	"else":     ELSE,
	"for":      FOR,
	"range":    RANGE,
	"return":   RETURN,
	"break":    BREAK,
	"continue": CONTINUE,
	"defer":    DEFER,
	"goto":     GOTO,
	"nil":      NIL,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
