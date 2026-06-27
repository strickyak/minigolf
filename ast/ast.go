package ast

import "github.com/strickyak/minigolf/token"

// Node is the base interface for all AST nodes.
type Node interface {
	TokenLiteral() string
	GetToken() *token.Token
}

// Statement is the interface for all statement nodes.
type Statement interface {
	Node
	statementNode()
}

// Expression is the interface for all expression nodes.
type Expression interface {
	Node
	expressionNode()
	GetResolvedType() Expression
	SetResolvedType(Expression)
}

type BaseExpression struct {
	ResolvedType Expression
}

func (b *BaseExpression) GetResolvedType() Expression  { return b.ResolvedType }
func (b *BaseExpression) SetResolvedType(t Expression) { b.ResolvedType = t }

// Program is the root node of an AST for a single file.
type Program struct {
	Statements []Statement
}

func (p *Program) GetToken() *token.Token {
	if len(p.Statements) > 0 {
		return p.Statements[0].GetToken()
	}
	return nil
}
func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

// ============================================================================
// Top-Level Statements
// ============================================================================

type PackageStatement struct {
	Token token.Token // The 'package' token
	Name  *Identifier
}

func (s *PackageStatement) statementNode()         {}
func (s *PackageStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *PackageStatement) GetToken() *token.Token { return &s.Token }

type ImportStatement struct {
	Token token.Token // The 'import' token
	Path  *StringLiteral
}

func (s *ImportStatement) statementNode()         {}
func (s *ImportStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ImportStatement) GetToken() *token.Token { return &s.Token }

type ConstStatement struct {
	Token token.Token // The 'const' token
	Name  *Identifier
	Value Expression
}

func (s *ConstStatement) statementNode()         {}
func (s *ConstStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ConstStatement) GetToken() *token.Token { return &s.Token }

type TypeStatement struct {
	Token          token.Token
	Name           *Identifier
	TypeParameters []*Identifier
	Tokens         []token.Token
	BaseType       Expression
	IsAlias        bool
	resolvedType   Expression
}

func (s *TypeStatement) statementNode()               {}
func (s *TypeStatement) expressionNode()              {}
func (s *TypeStatement) TokenLiteral() string         { return s.Token.Literal }
func (s *TypeStatement) GetToken() *token.Token       { return &s.Token }
func (s *TypeStatement) GetResolvedType() Expression  { return s.resolvedType }
func (s *TypeStatement) SetResolvedType(e Expression) { s.resolvedType = e }

type VarStatement struct {
	Token     token.Token // The 'var' token
	Name      *Identifier
	ValueType Expression // Optional, e.g. 'byte' or 'word'
	Value     Expression // Optional
	Linkage   string     // If non-empty, override the emitted symbol name (from // minigolf:linkage(...))
}

func (s *VarStatement) statementNode()         {}
func (s *VarStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *VarStatement) GetToken() *token.Token { return &s.Token }

type FuncStatement struct {
	Token            token.Token // The 'func' token
	Name             *Identifier
	TypeParameters   []*Identifier
	Tokens           []token.Token
	Receiver         *Parameter // Optional
	Parameters       []*Parameter
	ReturnParameters []*Parameter // Optional

	Body       *BlockStatement
	IsVariadic bool
	TrunkLevel int
	Popularity int
	Linkage    string // If non-empty, override the emitted symbol name (from // minigolf:linkage(...))
}

func (s *FuncStatement) statementNode()         {}
func (s *FuncStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *FuncStatement) GetToken() *token.Token { return &s.Token }

type Parameter struct {
	Name       *Identifier
	Type       Expression
	IsVariadic bool
}

// ============================================================================
// Function-Level Statements
// ============================================================================

type BlockStatement struct {
	Token      token.Token // The '{' token
	Statements []Statement
}

func (s *BlockStatement) statementNode()         {}
func (s *BlockStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *BlockStatement) GetToken() *token.Token { return &s.Token }

// AssignStatement handles `x = 5`, `x, y = 1, 2`, and `x := 5`
// Left-hand side is expressions (Identifiers or IndexExpressions).
type AssignStatement struct {
	Token  token.Token // The '=' or ':=' token
	Names  []Expression
	Values []Expression
}

func (s *AssignStatement) statementNode()         {}
func (s *AssignStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *AssignStatement) GetToken() *token.Token { return &s.Token }

type OpAssignStatement struct {
	Token    token.Token // The '+=', '-=', etc token
	Name     Expression
	Operator string // The underlying operator ('+', '-', etc)
	Value    Expression
}

func (s *OpAssignStatement) statementNode()         {}
func (s *OpAssignStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *OpAssignStatement) GetToken() *token.Token { return &s.Token }

type IfStatement struct {
	Token       token.Token // The 'if' token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement // Optional 'else' block
}

func (s *IfStatement) statementNode()         {}
func (s *IfStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *IfStatement) GetToken() *token.Token { return &s.Token }

type ForStatement struct {
	Token     token.Token // The 'for' token
	Condition Expression
	Body      *BlockStatement
}

func (s *ForStatement) statementNode()         {}
func (s *ForStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ForStatement) GetToken() *token.Token { return &s.Token }

type For3Statement struct {
	Token     token.Token // The 'for' token
	Init      Statement
	Condition Expression
	Increment Statement
	Body      *BlockStatement
}

func (s *For3Statement) statementNode()         {}
func (s *For3Statement) TokenLiteral() string   { return s.Token.Literal }
func (s *For3Statement) GetToken() *token.Token { return &s.Token }

type IncDecStatement struct {
	Token token.Token // The '++' or '--' token
	Name  Expression  // The identifier or expression being modified
}

func (s *IncDecStatement) statementNode()         {}
func (s *IncDecStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *IncDecStatement) GetToken() *token.Token { return &s.Token }

type ForRangeStatement struct {
	Token      token.Token // The 'for' token
	Key        Expression  // The identifier assigned (e.g. `i`)
	Value      Expression  // Optional identifier for value (e.g. `v` in `i, v := range`)
	IsDecl     bool        // true if `:=`, false if `=`
	RangeValue Expression  // e.g. `byte(5)`
	Body       *BlockStatement
}

func (s *ForRangeStatement) statementNode()         {}
func (s *ForRangeStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ForRangeStatement) GetToken() *token.Token { return &s.Token }

type DeferStatement struct {
	Token token.Token // The 'defer' token
	Call  Expression
	Block *BlockStatement // Used for `defer func() { ... }()`
}

func (s *DeferStatement) statementNode()         {}
func (s *DeferStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *DeferStatement) GetToken() *token.Token { return &s.Token }

type ReturnStatement struct {
	Token        token.Token  // The 'return' token
	ReturnValues []Expression // Optional
}

func (s *ReturnStatement) statementNode()         {}
func (s *ReturnStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ReturnStatement) GetToken() *token.Token { return &s.Token }

type BreakStatement struct {
	Token token.Token // The 'break' token
}

func (s *BreakStatement) statementNode()         {}
func (s *BreakStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *BreakStatement) GetToken() *token.Token { return &s.Token }

type ContinueStatement struct {
	Token token.Token // The 'continue' token
}

func (s *ContinueStatement) statementNode()         {}
func (s *ContinueStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ContinueStatement) GetToken() *token.Token { return &s.Token }

// GotoStatement represents `goto labelName`
// NOTE: if we later support labeled break/continue, the LabelStatement will
// need to track the following for-statement it owns.
type GotoStatement struct {
	Token token.Token // The 'goto' token
	Label string      // Target label name
}

func (s *GotoStatement) statementNode()         {}
func (s *GotoStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *GotoStatement) GetToken() *token.Token { return &s.Token }

// LabelStatement represents a label definition `labelName:` as a standalone
// statement.  The statement(s) that follow it are separate siblings in the
// enclosing block's statement list.
type LabelStatement struct {
	Token token.Token // The IDENT token (the label name)
	Label string      // The label name
}

func (s *LabelStatement) statementNode()         {}
func (s *LabelStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *LabelStatement) GetToken() *token.Token { return &s.Token }

// ExpressionStatement allows expressions (like function calls: `print(x)`) to stand alone
type ExpressionStatement struct {
	Token      token.Token // The first token of the expression
	Expression Expression
}

func (s *ExpressionStatement) statementNode()         {}
func (s *ExpressionStatement) TokenLiteral() string   { return s.Token.Literal }
func (s *ExpressionStatement) GetToken() *token.Token { return &s.Token }

// ============================================================================
// Expressions
// ============================================================================

type Identifier struct {
	BaseExpression
	Token      token.Token // The token.IDENT token
	Value      string
	Package    string
	ShortName  string
	IsResolved bool
}

func (e *Identifier) FullName() string {
	if !e.IsResolved || e.Package == "" || e.Package == "builtin" {
		if e.ShortName != "" {
			return e.ShortName
		}
		return e.Value
	}
	return e.Package + "." + e.ShortName
}

func (e *Identifier) expressionNode()        {}
func (e *Identifier) TokenLiteral() string   { return e.Token.Literal }
func (e *Identifier) GetToken() *token.Token { return &e.Token }

type IntegerLiteral struct {
	BaseExpression
	Token token.Token // The token.INT token
	Value int64       // Parsed as int64, semantic analysis will enforce byte/word limits
}

func (e *IntegerLiteral) expressionNode()        {}
func (il *IntegerLiteral) TokenLiteral() string  { return il.Token.Literal }
func (il *IntegerLiteral) String() string        { return il.Token.Literal }
func (e *IntegerLiteral) GetToken() *token.Token { return &e.Token }

// NilLiteral represents a nil keyword
type NilLiteral struct {
	BaseExpression
	Token token.Token // the 'nil' token
}

func (nl *NilLiteral) expressionNode()        {}
func (nl *NilLiteral) GetToken() *token.Token { return &nl.Token }
func (nl *NilLiteral) TokenLiteral() string   { return nl.Token.Literal }
func (nl *NilLiteral) String() string         { return "nil" }

type StringLiteral struct {
	BaseExpression
	Token token.Token
	Value string
}

func (e *StringLiteral) expressionNode()        {}
func (e *StringLiteral) TokenLiteral() string   { return e.Token.Literal }
func (e *StringLiteral) GetToken() *token.Token { return &e.Token }

type PrefixExpression struct {
	BaseExpression
	Token    token.Token // The prefix token, e.g., '!' or '-'
	Operator string
	Right    Expression
}

func (e *PrefixExpression) expressionNode()        {}
func (e *PrefixExpression) TokenLiteral() string   { return e.Token.Literal }
func (e *PrefixExpression) GetToken() *token.Token { return &e.Token }

type InfixExpression struct {
	BaseExpression
	Token    token.Token // The operator token, e.g., '+', '<'
	Left     Expression
	Operator string
	Right    Expression
}

func (e *InfixExpression) expressionNode()        {}
func (e *InfixExpression) TokenLiteral() string   { return e.Token.Literal }
func (e *InfixExpression) GetToken() *token.Token { return &e.Token }

// CallExpression handles both function calls and type casts (e.g. `byte(10)`)
type CallExpression struct {
	BaseExpression
	Token     token.Token // The '(' token
	Function  Expression  // Usually an *Identifier
	Arguments []Expression
}

func (e *CallExpression) expressionNode()        {}
func (e *CallExpression) TokenLiteral() string   { return e.Token.Literal }
func (e *CallExpression) GetToken() *token.Token { return &e.Token }

type FuncType struct {
	BaseExpression
	Token            token.Token // The 'func' token
	Parameters       []*Parameter
	ReturnParameters []*Parameter

	IsVariadic bool
}

func (e *FuncType) expressionNode()        {}
func (e *FuncType) TokenLiteral() string   { return e.Token.Literal }
func (e *FuncType) GetToken() *token.Token { return &e.Token }

type ArrayType struct {
	BaseExpression
	Token  token.Token // The '[' token
	Length Expression
	Elt    Expression
}

func (e *ArrayType) expressionNode()        {}
func (e *ArrayType) TokenLiteral() string   { return e.Token.Literal }
func (e *ArrayType) GetToken() *token.Token { return &e.Token }

type StructType struct {
	BaseExpression
	Token  token.Token // The 'struct' token
	Fields []*Field
}

type Field struct {
	Name *Identifier
	Type Expression
}

func (s *StructType) expressionNode()        {}
func (s *StructType) TokenLiteral() string   { return s.Token.Literal }
func (s *StructType) GetToken() *token.Token { return &s.Token }

type SelectorExpression struct {
	BaseExpression
	Token token.Token // The '.' token
	Left  Expression
	Right *Identifier
}

func (s *SelectorExpression) expressionNode()        {}
func (s *SelectorExpression) TokenLiteral() string   { return s.Token.Literal }
func (s *SelectorExpression) GetToken() *token.Token { return &s.Token }

type RangeExpression struct {
	BaseExpression
	Token token.Token // The 'range' token
	Value Expression
}

func (e *RangeExpression) expressionNode()        {}
func (e *RangeExpression) TokenLiteral() string   { return e.Token.Literal }
func (e *RangeExpression) GetToken() *token.Token { return &e.Token }

type IndexExpression struct {
	BaseExpression
	Token   token.Token // The '[' token
	Left    Expression
	Indices []Expression
	IsSlice bool
}

func (e *IndexExpression) expressionNode()        {}
func (e *IndexExpression) TokenLiteral() string   { return e.Token.Literal }
func (e *IndexExpression) GetToken() *token.Token { return &e.Token }

type PointerType struct {
	BaseExpression
	Token token.Token // The '*' token
	Elt   Expression
}

func (pt *PointerType) expressionNode()        {}
func (pt *PointerType) TokenLiteral() string   { return pt.Token.Literal }
func (pt *PointerType) GetToken() *token.Token { return &pt.Token }

type KeyValueExpr struct {
	BaseExpression
	Key   Expression
	Value Expression
}

func (e *KeyValueExpr) expressionNode()        {}
func (e *KeyValueExpr) TokenLiteral() string   { return e.Key.TokenLiteral() }
func (e *KeyValueExpr) GetToken() *token.Token { return e.Key.GetToken() }

type CompositeLit struct {
	BaseExpression
	Type     Expression   // The type being instantiated
	Token    token.Token  // The '{' token
	Elements []Expression // List of expressions or KeyValueExpr
}

func (e *CompositeLit) expressionNode()        {}
func (e *CompositeLit) TokenLiteral() string   { return e.Token.Literal }
func (e *CompositeLit) GetToken() *token.Token { return &e.Token }

type PragmaStatement struct {
	Token token.Token // The PRAGMA token
	Value string
}

func (ps *PragmaStatement) statementNode()         {}
func (ps *PragmaStatement) TokenLiteral() string   { return ps.Token.Literal }
func (ps *PragmaStatement) GetToken() *token.Token { return &ps.Token }
func (ps *PragmaStatement) String() string         { return "// minigolf: " + ps.Value }
