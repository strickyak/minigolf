package parser

import (
	"fmt"
	"minigo/ast"
	"minigo/token"
	"strconv"
)

// Precedence levels for Pratt parsing
const (
	_ int = iota
	LOWEST
	EQUALS      // ==
	LESSGREATER // > or <
	SUM         // +
	PRODUCT     // *
	PREFIX      // -X or !X
	CALL        // myFunction(X)
	INDEX       // array[index]
)

var precedences = map[token.TokenType]int{
	token.EQ:       EQUALS,
	token.NEQ:      EQUALS,
	token.LT:       LESSGREATER,
	token.GT:       LESSGREATER,
	token.LTE:      LESSGREATER,
	token.GTE:      LESSGREATER,
	token.PLUS:     SUM,
	token.MINUS:    SUM,
	token.SLASH:    PRODUCT,
	token.ASTERISK: PRODUCT,
	token.MOD:      PRODUCT,
	token.BIT_AND:  PRODUCT,
	token.BIT_OR:   SUM,
	token.BIT_XOR:  SUM,
	token.LSHIFT:   PRODUCT,
	token.RSHIFT:   PRODUCT,
	token.LPAREN:   CALL,
	token.LBRACKET: INDEX,
	token.DOT:      INDEX,
}

type Parser struct {
	tokens []token.Token
	pos    int

	curToken  token.Token
	peekToken token.Token

	errors []string

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn
}

type prefixParseFn func() ast.Expression
type infixParseFn func(ast.Expression) ast.Expression

func New(tokens []token.Token) *Parser {
	p := &Parser{
		tokens: tokens,
		pos:    0,
		errors: []string{},
	}

	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.BIT_AND, p.parsePrefixExpression)
	p.registerPrefix(token.ASTERISK, p.parsePointerType)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.LBRACKET, p.parseArrayType)
	p.registerPrefix(token.STRUCT, p.parseStructType)
	p.registerPrefix(token.RANGE, p.parseRangeExpression)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.MOD, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NEQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.LTE, p.parseInfixExpression)
	p.registerInfix(token.GTE, p.parseInfixExpression)
	p.registerInfix(token.BIT_AND, p.parseInfixExpression)
	p.registerInfix(token.BIT_OR, p.parseInfixExpression)
	p.registerInfix(token.BIT_XOR, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.RSHIFT, p.parseInfixExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.DOT, p.parseSelectorExpression)

	// Read two tokens, so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	if p.pos < len(p.tokens) {
		p.peekToken = p.tokens[p.pos]
		p.pos++
	} else {
		p.peekToken = token.Token{Type: token.EOF, Literal: ""}
	}
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t token.TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead at line %d:%d",
		t, p.peekToken.Type, p.peekToken.Line, p.peekToken.Column)
	p.errors = append(p.errors, msg)
}

func (p *Parser) curTokenIs(t token.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	} else {
		p.peekError(t)
		return false
	}
}

// ============================================================================
// Top-Level Statements Parsing
// ============================================================================

func (p *Parser) ParseProgram(overridePackage string) *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}

	for p.curToken.Type != token.EOF {
		stmt := p.parseTopLevelStatement(overridePackage)
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseTopLevelStatement(overridePackage string) ast.Statement {
	switch p.curToken.Type {
	case token.PACKAGE:
		return p.parsePackageStatement(overridePackage)
	case token.IMPORT:
		return p.parseImportStatement()
	case token.CONST:
		return p.parseConstStatement()
	case token.TYPE:
		return p.parseTypeStatement()
	case token.VAR:
		return p.parseVarStatement()
	case token.FUNC:
		return p.parseFuncStatement()
	case token.SEMICOLON:
		return nil
	default:
		msg := fmt.Sprintf("unexpected top-level token: %s at line %d", p.curToken.Type, p.curToken.Line)
		p.errors = append(p.errors, msg)
		return nil
	}
}

func (p *Parser) parsePackageStatement(overridePackage string) *ast.PackageStatement {
	stmt := &ast.PackageStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	pkg := p.curToken.Literal
	if overridePackage != "" {
		// The Identifier may have Token and Value fields
		// that don't match up.  Let's hope Value gets used.
		pkg = overridePackage
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: pkg}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseImportStatement() *ast.ImportStatement {
	stmt := &ast.ImportStatement{Token: p.curToken}

	if !p.expectPeek(token.STRING) {
		return nil
	}
	stmt.Path = &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseConstStatement() *ast.ConstStatement {
	stmt := &ast.ConstStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}
	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseTypeStatement() *ast.TypeStatement {
	stmt := &ast.TypeStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.LBRACKET) {
		p.nextToken() // consume '['
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		typeParam := p.curToken.Literal
		stmt.TypeParameters = append(stmt.TypeParameters, &ast.Identifier{Token: p.curToken, Value: typeParam})

		if !p.expectPeek(token.IDENT) || p.curToken.Literal != "any" {
			p.errors = append(p.errors, "expected 'any' constraint")
			return nil
		}
		if !p.expectPeek(token.RBRACKET) {
			return nil
		}

		p.nextToken() // move to start of base type
		startPos := p.pos - 2
		stmt.BaseType = p.parseExpression(LOWEST)
		endPos := p.pos - 1

		stmt.Tokens = make([]token.Token, endPos-startPos)
		copy(stmt.Tokens, p.tokens[startPos:endPos])
	} else {
		p.nextToken()
		stmt.BaseType = p.parseExpression(LOWEST)
	}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) ParseExpressionForGeneric() ast.Expression {
	return p.parseExpression(LOWEST)
}

func (p *Parser) ParseStatementForGeneric() ast.Statement {
	return p.parseTopLevelStatement("")
}

func (p *Parser) parseVarStatement() *ast.VarStatement {
	stmt := &ast.VarStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.LBRACKET) || p.peekTokenIs(token.ASTERISK) {
		p.nextToken()
		stmt.ValueType = p.parseExpression(LOWEST)
	}

	if p.peekTokenIs(token.ASSIGN) {
		p.nextToken() // advance to '='
		p.nextToken() // advance to value
		stmt.Value = p.parseExpression(LOWEST)
	}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseFuncStatement() *ast.FuncStatement {
	startPos := p.pos - 2
	stmt := &ast.FuncStatement{Token: p.curToken}

	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // move to '('
		p.nextToken() // move to receiver name

		receiver := &ast.Parameter{}
		receiver.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // move to type (e.g. '*')
		receiver.Type = p.parseExpression(LOWEST)
		stmt.Receiver = receiver

		if !p.expectPeek(token.RPAREN) {
			return nil
		}
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.LBRACKET) {
		p.nextToken() // move to '['
		for !p.peekTokenIs(token.RBRACKET) {
			p.nextToken()
			if p.curToken.Type == token.IDENT && p.curToken.Literal != "any" {
				stmt.TypeParameters = append(stmt.TypeParameters, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			} else if p.curToken.Type == token.COMMA {
				continue
			}
		}
		if !p.expectPeek(token.RBRACKET) {
			return nil
		}
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	stmt.Parameters = p.parseFunctionParameters()

	// Optional return type
	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // '('
		for !p.peekTokenIs(token.RPAREN) {
			p.nextToken()
			stmt.ReturnTypes = append(stmt.ReturnTypes, p.parseExpression(LOWEST))
			if p.peekTokenIs(token.COMMA) {
				p.nextToken()
			}
		}
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
	} else if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.LBRACKET) || p.peekTokenIs(token.ASTERISK) {
		p.nextToken()
		stmt.ReturnTypes = []ast.Expression{p.parseExpression(LOWEST)}
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	if len(stmt.TypeParameters) > 0 {
		endPos := p.pos - 1
		stmt.Tokens = make([]token.Token, endPos-startPos)
		copy(stmt.Tokens, p.tokens[startPos:endPos])
	}

	return stmt
}

func (p *Parser) parseFunctionParameters() []*ast.Parameter {
	var parameters []*ast.Parameter

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		return parameters
	}

	p.nextToken()

	// Parse first parameter
	param := &ast.Parameter{}
	param.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	p.nextToken()
	param.Type = p.parseExpression(LOWEST)
	parameters = append(parameters, param)

	// Parse subsequent parameters
	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // comma
		p.nextToken() // next ident

		param := &ast.Parameter{}
		param.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		p.nextToken()
		param.Type = p.parseExpression(LOWEST)
		parameters = append(parameters, param)
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return parameters
}

// ============================================================================
// Function-Level Statements Parsing
// ============================================================================

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.VAR:
		return p.parseVarStatement()
	case token.RETURN:
		return p.parseReturnStatement()
	case token.IF:
		return p.parseIfStatement()
	case token.FOR:
		return p.parseForStatement()
	case token.SEMICOLON:
		return nil
	default:
		return p.parseExpressionOrAssignStatement()
	}
}

func (p *Parser) parseExpressionOrAssignStatement() ast.Statement {
	startToken := p.curToken
	var lefts []ast.Expression

	lefts = append(lefts, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // comma
		p.nextToken() // next expression
		lefts = append(lefts, p.parseExpression(LOWEST))
	}

	if p.peekTokenIs(token.INC) || p.peekTokenIs(token.DEC) {
		if len(lefts) > 1 {
			return nil
		}
		stmt := &ast.IncDecStatement{Token: p.peekToken, Name: lefts[0]}
		p.nextToken() // move to ++ or --
		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	if p.peekTokenIs(token.ASSIGN) || p.peekTokenIs(token.DECLARE) {
		stmt := &ast.AssignStatement{Names: lefts}
		p.nextToken()
		stmt.Token = p.curToken // = or :=
		p.nextToken()

		stmt.Values = append(stmt.Values, p.parseExpression(LOWEST))

		for p.peekTokenIs(token.COMMA) {
			p.nextToken() // comma
			p.nextToken() // next expression
			stmt.Values = append(stmt.Values, p.parseExpression(LOWEST))
		}

		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	stmt := &ast.ExpressionStatement{Token: startToken, Expression: lefts[0]}
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.curToken}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
		return stmt
	}

	if p.peekTokenIs(token.RBRACE) {
		return stmt
	}

	p.nextToken()
	stmt.ReturnValues = append(stmt.ReturnValues, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // skip comma
		p.nextToken() // go to next expression
		stmt.ReturnValues = append(stmt.ReturnValues, p.parseExpression(LOWEST))
	}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken() // move to else

		if !p.expectPeek(token.LBRACE) {
			return nil
		}

		stmt.Alternative = p.parseBlockStatement()
	}

	return stmt
}

func (p *Parser) parseForStatement() ast.Statement {
	startToken := p.curToken

	p.nextToken()

	if p.curTokenIs(token.LBRACE) {
		stmt := &ast.ForStatement{Token: startToken, Condition: nil}
		stmt.Body = p.parseBlockStatement()
		return stmt
	}

	var firstStmt ast.Statement
	if !p.curTokenIs(token.SEMICOLON) {
		firstStmt = p.parseStatement()
	}

	if assignStmt, ok := firstStmt.(*ast.AssignStatement); ok {
		if len(assignStmt.Values) == 1 {
			if rangeExpr, ok := assignStmt.Values[0].(*ast.RangeExpression); ok {
				stmt := &ast.ForRangeStatement{
					Token:      startToken,
					Key:        assignStmt.Names[0],
					IsDecl:     assignStmt.Token.Type == token.DECLARE,
					RangeValue: rangeExpr.Value,
				}
				if !p.expectPeek(token.LBRACE) {
					return nil
				}
				stmt.Body = p.parseBlockStatement()
				return stmt
			}
		}
	}

	if p.curTokenIs(token.SEMICOLON) {
		stmt := &ast.For3Statement{Token: startToken, Init: firstStmt}
		p.nextToken() // Skip first semicolon

		if !p.curTokenIs(token.SEMICOLON) {
			stmt.Condition = p.parseExpression(LOWEST)
			if !p.expectPeek(token.SEMICOLON) {
				return nil
			}
		}

		p.nextToken() // Skip second semicolon

		if !p.curTokenIs(token.LBRACE) {
			stmt.Increment = p.parseStatement()
			if !p.expectPeek(token.LBRACE) {
				return nil
			}
		}

		stmt.Body = p.parseBlockStatement()
		return stmt
	}

	stmt := &ast.ForStatement{Token: startToken}
	if exprStmt, ok := firstStmt.(*ast.ExpressionStatement); ok {
		stmt.Condition = exprStmt.Expression
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// parseExpressionStatement is replaced by parseExpressionOrAssignStatement

// ============================================================================
// Expressions Parsing (Pratt Parser)
// ============================================================================

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(token.SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()

		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found at line %d:%d", t, p.curToken.Line, p.curToken.Column)
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseRangeExpression() ast.Expression {
	expr := &ast.RangeExpression{Token: p.curToken}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	return expr
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer at line %d:%d", p.curToken.Literal, p.curToken.Line, p.curToken.Column)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parsePointerType() ast.Expression {
	node := &ast.PointerType{Token: p.curToken}
	p.nextToken()
	node.Elt = p.parseExpression(PREFIX)
	return node
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	return exp
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	var list []ast.Expression

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // comma
		p.nextToken() // next expression
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) parseArrayType() ast.Expression {
	arrayType := &ast.ArrayType{Token: p.curToken}

	p.nextToken()

	arrayType.Length = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	p.nextToken() // move to element type

	arrayType.Elt = p.parseExpression(LOWEST)

	return arrayType
}

func (p *Parser) parseStructType() ast.Expression {
	structType := &ast.StructType{Token: p.curToken}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		field := &ast.Field{}

		if !p.curTokenIs(token.IDENT) {
			p.errors = append(p.errors, fmt.Sprintf("expected field name to be IDENT, got %s", p.curToken.Type))
			return nil
		}
		field.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		p.nextToken() // move to type

		field.Type = p.parseExpression(LOWEST)
		structType.Fields = append(structType.Fields, field)

		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken() // Consume semicolon
		}
		p.nextToken() // move to next field or RBRACE
	}

	return structType
}

func (p *Parser) parseSelectorExpression(left ast.Expression) ast.Expression {
	exp := &ast.SelectorExpression{Token: p.curToken, Left: left}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	exp.Right = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	return exp
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}

	p.nextToken()
	exp.Indices = append(exp.Indices, p.parseExpression(LOWEST))

	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // skip comma
		p.nextToken() // next expression
		exp.Indices = append(exp.Indices, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}
