package ast

import "minigo/token"

// Node is the base interface for all AST nodes.
type Node interface {
	TokenLiteral() string
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
}

// Program is the root node of an AST for a single file.
type Program struct {
	Statements []Statement
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

func (s *PackageStatement) statementNode()       {}
func (s *PackageStatement) TokenLiteral() string { return s.Token.Literal }

type ImportStatement struct {
	Token token.Token // The 'import' token
	Path  *StringLiteral
}

func (s *ImportStatement) statementNode()       {}
func (s *ImportStatement) TokenLiteral() string { return s.Token.Literal }

type ConstStatement struct {
	Token token.Token // The 'const' token
	Name  *Identifier
	Value Expression
}

func (s *ConstStatement) statementNode()       {}
func (s *ConstStatement) TokenLiteral() string { return s.Token.Literal }

type TypeStatement struct {
	Token    token.Token // The 'type' token
	Name     *Identifier
	BaseType *Identifier // 'byte' or 'word'
}

func (s *TypeStatement) statementNode()       {}
func (s *TypeStatement) TokenLiteral() string { return s.Token.Literal }

type VarStatement struct {
	Token     token.Token // The 'var' token
	Name      *Identifier
	ValueType *Identifier // Optional, e.g. 'byte' or 'word'
	Value     Expression  // Optional
}

func (s *VarStatement) statementNode()       {}
func (s *VarStatement) TokenLiteral() string { return s.Token.Literal }

type FuncStatement struct {
	Token      token.Token // The 'func' token
	Name       *Identifier
	Parameters []*Parameter
	ReturnType *Identifier // Optional
	Body       *BlockStatement
}

func (s *FuncStatement) statementNode()       {}
func (s *FuncStatement) TokenLiteral() string { return s.Token.Literal }

// Parameter represents a single parameter in a function declaration.
type Parameter struct {
	Name *Identifier
	Type *Identifier
}

// ============================================================================
// Function-Level Statements
// ============================================================================

type BlockStatement struct {
	Token      token.Token // The '{' token
	Statements []Statement
}

func (s *BlockStatement) statementNode()       {}
func (s *BlockStatement) TokenLiteral() string { return s.Token.Literal }

// AssignStatement handles `x = 5`, `x, y = 1, 2`, and `x := 5`
// Because we have no pointers or arrays, the left-hand side is strictly identifiers.
type AssignStatement struct {
	Token  token.Token // The '=' or ':=' token
	Names  []*Identifier
	Values []Expression
}

func (s *AssignStatement) statementNode()       {}
func (s *AssignStatement) TokenLiteral() string { return s.Token.Literal }

type IfStatement struct {
	Token       token.Token // The 'if' token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement // Optional 'else' block
}

func (s *IfStatement) statementNode()       {}
func (s *IfStatement) TokenLiteral() string { return s.Token.Literal }

type ForStatement struct {
	Token     token.Token // The 'for' token
	Condition Expression
	Body      *BlockStatement
}

func (s *ForStatement) statementNode()       {}
func (s *ForStatement) TokenLiteral() string { return s.Token.Literal }

type ReturnStatement struct {
	Token       token.Token // The 'return' token
	ReturnValue Expression  // Optional
}

func (s *ReturnStatement) statementNode()       {}
func (s *ReturnStatement) TokenLiteral() string { return s.Token.Literal }

// ExpressionStatement allows expressions (like function calls: `print(x)`) to stand alone
type ExpressionStatement struct {
	Token      token.Token // The first token of the expression
	Expression Expression
}

func (s *ExpressionStatement) statementNode()       {}
func (s *ExpressionStatement) TokenLiteral() string { return s.Token.Literal }

// ============================================================================
// Expressions
// ============================================================================

type Identifier struct {
	Token token.Token // The token.IDENT token
	Value string
}

func (e *Identifier) expressionNode()      {}
func (e *Identifier) TokenLiteral() string { return e.Token.Literal }

type IntegerLiteral struct {
	Token token.Token // The token.INT token
	Value int64       // Parsed as int64, semantic analysis will enforce byte/word limits
}

func (e *IntegerLiteral) expressionNode()      {}
func (e *IntegerLiteral) TokenLiteral() string { return e.Token.Literal }

type StringLiteral struct {
	Token token.Token
	Value string
}

func (e *StringLiteral) expressionNode()      {}
func (e *StringLiteral) TokenLiteral() string { return e.Token.Literal }

type PrefixExpression struct {
	Token    token.Token // The prefix token, e.g., '!' or '-'
	Operator string
	Right    Expression
}

func (e *PrefixExpression) expressionNode()      {}
func (e *PrefixExpression) TokenLiteral() string { return e.Token.Literal }

type InfixExpression struct {
	Token    token.Token // The operator token, e.g., '+', '<'
	Left     Expression
	Operator string
	Right    Expression
}

func (e *InfixExpression) expressionNode()      {}
func (e *InfixExpression) TokenLiteral() string { return e.Token.Literal }

// CallExpression handles both function calls and type casts (e.g. `byte(10)`)
type CallExpression struct {
	Token     token.Token // The '(' token
	Function  Expression  // Usually an *Identifier
	Arguments []Expression
}

func (e *CallExpression) expressionNode()      {}
func (e *CallExpression) TokenLiteral() string { return e.Token.Literal }
