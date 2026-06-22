package parser

import (
	"fmt"
	"strconv"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/token"
)

// Precedence levels for Pratt parsing
const (
	_ int = iota
	LOWEST
	LOGICAL_OR  // ||
	LOGICAL_AND // &&
	EQUALS      // ==
	LESSGREATER // > or <
	SUM         // +
	PRODUCT     // *
	PREFIX      // -X or !X
	CALL        // myFunction(X)
	INDEX       // array[index]
)

var precedences = map[token.TokenType]int{
	token.OR:        LOGICAL_OR,
	token.AND:       LOGICAL_AND,
	token.EQ:        EQUALS,
	token.NEQ:       EQUALS,
	token.LT:        LESSGREATER,
	token.GT:        LESSGREATER,
	token.LTE:       LESSGREATER,
	token.GTE:       LESSGREATER,
	token.PLUS:      SUM,
	token.MINUS:     SUM,
	token.SLASH:     PRODUCT,
	token.ASTERISK:  PRODUCT,
	token.MOD:       PRODUCT,
	token.BIT_AND:   PRODUCT,
	token.BIT_OR:    SUM,
	token.BIT_XOR:   SUM,
	token.BIT_CLEAR: PRODUCT,
	token.LSHIFT:    PRODUCT,
	token.RSHIFT:    PRODUCT,
	token.LPAREN:    CALL,
	token.LBRACKET:  INDEX,
	token.DOT:       INDEX,
	token.LBRACE:    CALL,
}

type Parser struct {
	tokens []token.Token
	pos    int

	curToken  token.Token
	peekToken token.Token

	errors []string

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn

	allowCompositeLit bool
}

type prefixParseFn func() ast.Expression
type infixParseFn func(ast.Expression) ast.Expression

func New(tokens []token.Token) *Parser {
	p := &Parser{
		tokens:            tokens,
		pos:               0,
		errors:            []string{},
		allowCompositeLit: true,
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
	p.registerPrefix(token.FUNC, p.parseFuncType)
	p.registerPrefix(token.NIL, p.parseNil)
	p.registerPrefix(token.RANGE, p.parseRangeExpression)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.MOD, p.parseInfixExpression)
	p.registerInfix(token.AND, p.parseInfixExpression)
	p.registerInfix(token.OR, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NEQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.LTE, p.parseInfixExpression)
	p.registerInfix(token.GTE, p.parseInfixExpression)
	p.registerInfix(token.BIT_AND, p.parseInfixExpression)
	p.registerInfix(token.BIT_OR, p.parseInfixExpression)
	p.registerInfix(token.BIT_XOR, p.parseInfixExpression)
	p.registerInfix(token.BIT_CLEAR, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.RSHIFT, p.parseInfixExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.DOT, p.parseSelectorExpression)
	p.registerInfix(token.LBRACE, p.parseCompositeLit)

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

func (p *Parser) addError(tok token.Token, msg string) {
	if tok.ExpandedFrom != "" {
		msg += fmt.Sprintf(" [%s]", tok.ExpandedFrom)
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) peekError(t token.TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead at %s line %d:%d",
		t, p.peekToken.Type, p.peekToken.Filename, p.peekToken.Line, p.peekToken.Column)
	p.addError(p.peekToken, msg)
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
	case token.PRAGMA:
		return p.parsePragmaStatement()
	case token.TYPE:
		return p.parseTypeStatement()
	case token.VAR:
		return p.parseVarStatement()
	case token.FUNC:
		return p.parseFuncStatement()
	case token.SEMICOLON:
		return nil
	default:
		msg := fmt.Sprintf("unexpected top-level token: %s at %s line %d", p.curToken.Type, p.curToken.Filename, p.curToken.Line)
		p.addError(p.curToken, msg)
		return nil
	}
}

func (p *Parser) parsePragmaStatement() *ast.PragmaStatement {
	stmt := &ast.PragmaStatement{Token: p.curToken, Value: p.curToken.Literal}
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
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
	startPos := p.pos - 2
	stmt := &ast.TypeStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.LBRACKET) {
		p.nextToken() // consume '['
		for !p.peekTokenIs(token.RBRACKET) && !p.peekTokenIs(token.EOF) {
			p.nextToken()
			if p.curToken.Type == token.IDENT && p.curToken.Literal != "any" {
				stmt.TypeParameters = append(stmt.TypeParameters, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
		}
		if !p.expectPeek(token.RBRACKET) {
			//fmt.Printf("DEBUG PARSETYPE: expected ], got %s\n", p.curToken.Literal)
			return nil
		}

		p.nextToken() // move to start of base type
		stmt.BaseType = p.parseExpression(LOWEST)
		endPos := p.pos - 1

		stmt.Tokens = make([]token.Token, endPos-startPos)
		copy(stmt.Tokens, p.tokens[startPos:endPos])
	} else {
		if p.peekTokenIs(token.ASSIGN) {
			stmt.IsAlias = true
			p.nextToken() // consume '='
		}
		p.nextToken() // move to base type
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
	//fmt.Printf("DEBUG PARSEGENERIC: curToken.Type=%s curToken.Literal=%s\n", p.curToken.Type, p.curToken.Literal)
	return p.parseTopLevelStatement("")
}

func (p *Parser) parseVarStatement() *ast.VarStatement {
	stmt := &ast.VarStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.LBRACKET) || p.peekTokenIs(token.ASTERISK) || p.peekTokenIs(token.FUNC) {
		p.nextToken()
		p.allowCompositeLit = false
		stmt.ValueType = p.parseExpression(LOWEST)
		p.allowCompositeLit = true
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

		// Extract type parameters from generic receivers like `*slice[T]`
		var extractTypeParams func(expr ast.Expression)
		extractTypeParams = func(expr ast.Expression) {
			switch e := expr.(type) {
			case *ast.PointerType:
				extractTypeParams(e.Elt)
			case *ast.IndexExpression:
				for _, idx := range e.Indices {
					if ident, ok := idx.(*ast.Identifier); ok {
						stmt.TypeParameters = append(stmt.TypeParameters, ident)
					}
				}
			}
		}
		extractTypeParams(receiver.Type)
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
	if len(stmt.Parameters) > 0 && stmt.Parameters[len(stmt.Parameters)-1].IsVariadic {
		stmt.IsVariadic = true
	}

	// Optional return type
	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // '('
		stmt.ReturnParameters = p.parseReturnParameters()
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
	} else if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.LBRACKET) || p.peekTokenIs(token.ASTERISK) || p.peekTokenIs(token.FUNC) {
		p.nextToken()
		p.allowCompositeLit = false
		typ := p.parseExpression(LOWEST)
		stmt.ReturnParameters = []*ast.Parameter{{Type: typ}}
		p.allowCompositeLit = true
	}

	if p.peekTokenIs(token.LBRACE) {
		p.nextToken()
		stmt.Body = p.parseBlockStatement()
	}

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
	if p.curTokenIs(token.ELLIPSIS) {
		param.IsVariadic = true
		p.nextToken()
	}
	param.Type = p.parseExpression(LOWEST)
	if param.IsVariadic {
		param.Type = &ast.IndexExpression{
			Left:    &ast.Identifier{Token: p.curToken, Value: "slice"},
			Indices: []ast.Expression{param.Type},
		}
	}
	parameters = append(parameters, param)

	// Parse subsequent parameters
	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // comma
		p.nextToken() // next ident

		param := &ast.Parameter{}
		param.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

		p.nextToken()

		if p.curTokenIs(token.ELLIPSIS) {
			param.IsVariadic = true
			p.nextToken()
		}

		p.allowCompositeLit = false
		param.Type = p.parseExpression(LOWEST)
		p.allowCompositeLit = true

		if param.IsVariadic {
			param.Type = &ast.IndexExpression{
				Left:    &ast.Identifier{Token: p.curToken, Value: "slice"},
				Indices: []ast.Expression{param.Type},
			}
		}

		parameters = append(parameters, param)
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return parameters
}

func (p *Parser) parseReturnParameters() []*ast.Parameter {
	var parameters []*ast.Parameter

	if p.peekTokenIs(token.RPAREN) {
		return parameters
	}

	p.nextToken()

	for {
		param := &ast.Parameter{}

		// Parse the first expression
		p.allowCompositeLit = false
		expr := p.parseExpression(LOWEST)
		p.allowCompositeLit = true

		if ident, ok := expr.(*ast.Identifier); ok && !p.peekTokenIs(token.COMMA) && !p.peekTokenIs(token.RPAREN) {
			// It was a name, now parse the type
			param.Name = ident
			p.nextToken()
			p.allowCompositeLit = false
			param.Type = p.parseExpression(LOWEST)
			p.allowCompositeLit = true
		} else {
			// It was an anonymous type
			param.Type = expr
		}

		parameters = append(parameters, param)

		if p.peekTokenIs(token.COMMA) {
			p.nextToken() // Skip comma
			if p.peekTokenIs(token.RPAREN) {
				break
			}
			p.nextToken()
		} else {
			break
		}
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
	case token.DEFER:
		return p.parseDeferStatement()
	case token.IF:
		return p.parseIfStatement()
	case token.FOR:
		return p.parseForStatement()
	case token.BREAK:
		return p.parseBreakStatement()
	case token.CONTINUE:
		return p.parseContinueStatement()
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

	if p.peekTokenIs(token.ADD_ASSIGN) || p.peekTokenIs(token.SUB_ASSIGN) || p.peekTokenIs(token.MUL_ASSIGN) || p.peekTokenIs(token.DIV_ASSIGN) || p.peekTokenIs(token.MOD_ASSIGN) || p.peekTokenIs(token.AND_ASSIGN) || p.peekTokenIs(token.OR_ASSIGN) || p.peekTokenIs(token.XOR_ASSIGN) || p.peekTokenIs(token.SHL_ASSIGN) || p.peekTokenIs(token.SHR_ASSIGN) || p.peekTokenIs(token.CLEAR_ASSIGN) {
		if len(lefts) > 1 {
			return nil
		}
		stmt := &ast.OpAssignStatement{Name: lefts[0]}
		stmt.Token = p.peekToken
		switch p.peekToken.Type {
		case token.ADD_ASSIGN:
			stmt.Operator = "+"
		case token.SUB_ASSIGN:
			stmt.Operator = "-"
		case token.MUL_ASSIGN:
			stmt.Operator = "*"
		case token.DIV_ASSIGN:
			stmt.Operator = "/"
		case token.MOD_ASSIGN:
			stmt.Operator = "%"
		case token.AND_ASSIGN:
			stmt.Operator = "&"
		case token.OR_ASSIGN:
			stmt.Operator = "|"
		case token.XOR_ASSIGN:
			stmt.Operator = "^"
		case token.SHL_ASSIGN:
			stmt.Operator = "<<"
		case token.SHR_ASSIGN:
			stmt.Operator = ">>"
		case token.CLEAR_ASSIGN:
			stmt.Operator = "&^"
		}
		p.nextToken() // move to operator
		p.nextToken() // move to first token of RHS
		stmt.Value = p.parseExpression(LOWEST)
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

func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	stmt := &ast.DeferStatement{Token: p.curToken}

	p.nextToken()

	// Special case: defer func() { ... }()
	if p.curTokenIs(token.FUNC) && p.peekTokenIs(token.LPAREN) &&
		p.pos < len(p.tokens) && p.tokens[p.pos].Type == token.RPAREN &&
		p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == token.LBRACE {

		p.nextToken() // move to LPAREN
		p.nextToken() // move to RPAREN
		p.nextToken() // move to LBRACE

		stmt.Block = p.parseBlockStatement()

		if p.curTokenIs(token.RBRACE) {
			p.nextToken() // move past RBRACE
		}

		// parse optional () at the end
		if p.curTokenIs(token.LPAREN) {
			p.nextToken() // move to RPAREN
			// we leave curToken on RPAREN, because the outer parser loop calls p.nextToken()
		}

		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	stmt.Call = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseBreakStatement() *ast.BreakStatement {
	stmt := &ast.BreakStatement{Token: p.curToken}
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ast.ContinueStatement {
	stmt := &ast.ContinueStatement{Token: p.curToken}
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken}

	p.nextToken()

	p.allowCompositeLit = false
	stmt.Condition = p.parseExpression(LOWEST)
	p.allowCompositeLit = true

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken() // move to else

		if p.peekTokenIs(token.IF) {
			p.nextToken() // move to if
			ifStmt := p.parseIfStatement()
			stmt.Alternative = &ast.BlockStatement{
				Token:      p.curToken,
				Statements: []ast.Statement{ifStmt},
			}
		} else {
			if !p.expectPeek(token.LBRACE) {
				return nil
			}
			stmt.Alternative = p.parseBlockStatement()
		}
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
		p.allowCompositeLit = false
		firstStmt = p.parseStatement()
		p.allowCompositeLit = true
	}

	if assignStmt, ok := firstStmt.(*ast.AssignStatement); ok {
		if len(assignStmt.Values) == 1 {
			if rangeExpr, ok := assignStmt.Values[0].(*ast.RangeExpression); ok {
				var valExpr ast.Expression
				if len(assignStmt.Names) > 1 {
					valExpr = assignStmt.Names[1]
				}
				stmt := &ast.ForRangeStatement{
					Token:      startToken,
					Key:        assignStmt.Names[0],
					Value:      valExpr,
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
			p.allowCompositeLit = false
			stmt.Condition = p.parseExpression(LOWEST)
			p.allowCompositeLit = true
			if !p.expectPeek(token.SEMICOLON) {
				return nil
			}
		}

		p.nextToken() // Skip second semicolon

		if !p.curTokenIs(token.LBRACE) {
			p.allowCompositeLit = false
			stmt.Increment = p.parseStatement()
			p.allowCompositeLit = true
			if !p.expectPeek(token.LBRACE) {
				return nil
			}
		}

		stmt.Body = p.parseBlockStatement()
		return stmt
	}

	stmt := &ast.ForStatement{Token: startToken}
	if exprStmt, ok := firstStmt.(*ast.ExpressionStatement); ok {
		p.allowCompositeLit = false
		stmt.Condition = exprStmt.Expression
		p.allowCompositeLit = true
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
	if prec, ok := precedences[p.peekToken.Type]; ok {
		if p.peekToken.Type == token.LBRACE && !p.allowCompositeLit {
			return LOWEST
		}
		return prec
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
	msg := fmt.Sprintf("no prefix parse function for %s found at %s line %d:%d", t, p.curToken.Filename, p.curToken.Line, p.curToken.Column)
	p.addError(p.curToken, msg)
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

func (p *Parser) parseNil() ast.Expression {
	return &ast.NilLiteral{Token: p.curToken}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer at %s line %d:%d", p.curToken.Literal, p.curToken.Filename, p.curToken.Line, p.curToken.Column)
		p.addError(p.curToken, msg)
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

func (p *Parser) parseFuncTypeParameters() []*ast.Parameter {
	var parameters []*ast.Parameter

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		return parameters
	}

	p.nextToken()

	parameters = append(parameters, p.parseFuncTypeOneParam())

	// Parse subsequent parameters
	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // comma
		p.nextToken() // first token of next param
		parameters = append(parameters, p.parseFuncTypeOneParam())
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return parameters
}

// parseFuncTypeOneParam parses one parameter of a func type.
// It handles both anonymous types ("*T") and named types ("a *T").
// curToken is the first token of this parameter.
func (p *Parser) parseFuncTypeOneParam() *ast.Parameter {
	param := &ast.Parameter{}

	if p.curTokenIs(token.ELLIPSIS) {
		param.IsVariadic = true
		p.nextToken()
	}

	// If current token is an identifier and peek is ASTERISK, treat it as
	// "name *type" — e.g. "a *T". This is the only unambiguous case:
	// a bare IDENT peeked by * cannot be a type expression (would be multiply).
	// We don't generalize to other peek tokens because e.g. "slice" peeked by
	// "[" is legitimately the type "slice[T]", not a name.
	if !param.IsVariadic && p.curTokenIs(token.IDENT) && p.peekTokenIs(token.ASTERISK) {
		param.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		p.nextToken() // move to * (type token)
	}

	param.Type = p.parseExpression(LOWEST)

	if param.IsVariadic {
		param.Type = &ast.IndexExpression{
			Left:    &ast.Identifier{Token: p.curToken, Value: "slice"},
			Indices: []ast.Expression{param.Type},
		}
	}
	return param
}

func (p *Parser) parseFuncType() ast.Expression {
	node := &ast.FuncType{Token: p.curToken}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	node.Parameters = p.parseFuncTypeParameters()
	if len(node.Parameters) > 0 && node.Parameters[len(node.Parameters)-1].IsVariadic {
		node.IsVariadic = true
	}

	if p.peekTokenIs(token.LPAREN) {
		p.nextToken() // '('
		node.ReturnParameters = p.parseReturnParameters()
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
	} else if p.peekTokenIs(token.IDENT) || p.peekTokenIs(token.LBRACKET) || p.peekTokenIs(token.ASTERISK) || p.peekTokenIs(token.FUNC) {
		p.nextToken()
		node.ReturnParameters = []*ast.Parameter{{Type: p.parseExpression(LOWEST)}}
	}

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

func (p *Parser) parseCompositeLit(left ast.Expression) ast.Expression {
	lit := &ast.CompositeLit{Token: p.curToken, Type: left}
	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		var el ast.Expression
		if p.peekTokenIs(token.COLON) {
			key := p.parseExpression(LOWEST)
			p.nextToken() // skip to ':'
			p.nextToken() // skip past ':'
			val := p.parseExpression(LOWEST)
			el = &ast.KeyValueExpr{Key: key, Value: val}
		} else {
			el = p.parseExpression(LOWEST)
		}
		lit.Elements = append(lit.Elements, el)

		if p.peekTokenIs(token.COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			p.nextToken()
		}
	}
	return lit
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
			p.addError(p.curToken, fmt.Sprintf("expected field name to be IDENT, got %s at %s line %d:%d", p.curToken.Type, p.curToken.Filename, p.curToken.Line, p.curToken.Column))
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

	if p.peekTokenIs(token.COLON) {
		exp.IsSlice = true
		p.nextToken() // move to COLON
		p.nextToken() // skip COLON
		exp.Indices = append(exp.Indices, p.parseExpression(LOWEST))
	} else {
		for p.peekTokenIs(token.COMMA) {
			p.nextToken() // skip comma
			p.nextToken() // next expression
			exp.Indices = append(exp.Indices, p.parseExpression(LOWEST))
		}
	}

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}
