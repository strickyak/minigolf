// Copyright 2021 The CC Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cc // import "modernc.org/cc/v5"

import (
	"bytes"
	"fmt"
	"strings"
)

var defaultKeywords = map[string]rune{
	// C99
	"_Bool":      rune(BOOL),
	"_Complex":   rune(COMPLEX),
	"_Imaginary": rune(IMAGINARY),
	"auto":       rune(AUTO),
	"break":      rune(BREAK),
	"case":       rune(CASE),
	"char":       rune(CHAR),
	"const":      rune(CONST),
	"continue":   rune(CONTINUE),
	"default":    rune(DEFAULT),
	"do":         rune(DO),
	"double":     rune(DOUBLE),
	"else":       rune(ELSE),
	"enum":       rune(ENUM),
	"extern":     rune(EXTERN),
	"float":      rune(FLOAT),
	"for":        rune(FOR),
	"goto":       rune(GOTO),
	"if":         rune(IF),
	"inline":     rune(INLINE),
	"int":        rune(INT),
	"long":       rune(LONG),
	"register":   rune(REGISTER),
	"restrict":   rune(RESTRICT),
	"return":     rune(RETURN),
	"short":      rune(SHORT),
	"signed":     rune(SIGNED),
	"sizeof":     rune(SIZEOF),
	"static":     rune(STATIC),
	"struct":     rune(STRUCT),
	"switch":     rune(SWITCH),
	"typedef":    rune(TYPEDEF),
	"union":      rune(UNION),
	"unsigned":   rune(UNSIGNED),
	"void":       rune(VOID),
	"volatile":   rune(VOLATILE),
	"while":      rune(WHILE),

	// C11
	"_Alignas":       rune(ALIGNAS),
	"_Alignof":       rune(ALIGNOF),
	"_Atomic":        rune(ATOMIC),
	"_Generic":       rune(GENERIC),
	"_Noreturn":      rune(NORETURN),
	"_Static_assert": rune(STATICASSERT),
	"_Thread_local":  rune(THREADLOCAL),

	// GCC/clang/other extensions.
	"_Decimal128":   rune(DECIMAL128),
	"_Decimal32":    rune(DECIMAL32),
	"_Decimal64":    rune(DECIMAL64),
	"_Float128":     rune(FLOAT128),
	"_Float128x":    rune(FLOAT128X),
	"_Float16":      rune(FLOAT16),
	"_Float32":      rune(FLOAT32),
	"_Float32x":     rune(FLOAT32X),
	"_Float64":      rune(FLOAT64),
	"_Float64x":     rune(FLOAT64X),
	"_Nonnull":      rune(NONNULL),
	"__alignof":     rune(ALIGNOF),
	"__alignof__":   rune(ALIGNOF),
	"__asm":         rune(ASM),
	"__asm__":       rune(ASM),
	"__attribute":   rune(ATTRIBUTE),
	"__attribute__": rune(ATTRIBUTE),
	"__auto_type":   rune(AUTOTYPE),
	"__complex":     rune(COMPLEX),
	"__complex__":   rune(COMPLEX),
	"__const":       rune(CONST),
	"__declspec":    rune(DECLSPEC),
	"__float128":    rune(FLOAT128),
	"__imag":        rune(IMAG),
	"__imag__":      rune(IMAG),
	"__inline":      rune(INLINE),
	"__inline__":    rune(INLINE),
	"__cc_int8":     rune(INT8),
	"__cc_int16":    rune(INT16),
	"__cc_int32":    rune(INT32),
	"__cc_int64":    rune(INT64),
	"__int128":      rune(INT128),
	"__cc_int8_t":   rune(INT8),
	"__cc_int16_t":  rune(INT16),
	"__cc_int32_t":  rune(INT32),
	"__cc_int64_t":  rune(INT64),
	"__int128_t":    rune(INT128),
	"__label__":     rune(LABEL),
	"__real":        rune(REAL),
	"__real__":      rune(REAL),
	"__restrict":    rune(RESTRICT),
	"__restrict__":  rune(RESTRICT),
	"__signed":      rune(SIGNED),
	"__signed__":    rune(SIGNED),
	"__thread":      rune(THREADLOCAL),
	"__typeof":      rune(TYPEOF),
	"__typeof__":    rune(TYPEOF),
	"__cc_uint8_t":  rune(UINT8),
	"__cc_uint16_t": rune(UINT16),
	"__cc_uint32_t": rune(UINT32),
	"__cc_uint64_t": rune(UINT64),
	"__uint128_t":   rune(UINT128),
	"__volatile":    rune(VOLATILE),
	"__volatile__":  rune(VOLATILE),
	"asm":           rune(ASM),
	"typeof":        rune(TYPEOF),
}

type parser struct {
	cpp        *cpp
	fnScope    *Scope
	funcTokens []Token
	gotos      []*JumpStatement
	keywords   map[string]rune
	prevNL     Token
	scope      *Scope
	switchCtx  *SelectionStatement
	toks       []Token

	seq int32
}

func newParser(cfg *Config, fset *fset, sources []Source) (*parser, error) {
	cpp, err := newCPP(cfg, fset, sources, nil)
	if err != nil {
		return nil, err
	}

	funcTokens := []Token{
		{Ch: rune(STATIC)},
		{Ch: rune(CONST)},
		{Ch: rune(CHAR)},
		{Ch: rune(IDENTIFIER)},
		{Ch: '['},
		{Ch: ']'},
		{Ch: '='},
		{Ch: rune(STRINGLITERAL)},
		{Ch: ';'},
	}
	if cfg.keywords == nil {
		cfg.keywords = defaultKeywords
	}
	funcTokens = funcTokens[:len(funcTokens):len(funcTokens)]
	return &parser{
		cpp:        cpp,
		funcTokens: funcTokens,
		scope:      &Scope{},
		keywords:   cfg.keywords,
	}, nil
}

func (p *parser) newScope() { p.scope = newScope(p.scope) }

func (p *parser) closeScope() {
	if parent := p.scope.Parent; parent != nil {
		p.scope = parent
		return
	}

	p.rune(false)
	p.cpp.eh("%v: internal error (%v:)", p.toks[0].Position(), origin(1))
	if isTesting {
		trc("%v: internal error (%v:)", p.toks[0].Position(), origin(1))
	}
}

func (p *parser) isKeyword(s []byte) (r rune, ok bool) {
	r, ok = p.keywords[string(s)]
	return r, ok
}

func (p *parser) next() {
	var b []byte
	set := false
	if p.prevNL.Ch != 0 {
		b = p.prevNL.Sep()
		set = true
	}
	for {
		p.cpp.rune()
		t := p.cpp.shift()
		switch t.Ch {
		case ' ':
			// nop
		case '\n':
			if p.prevNL.Ch != 0 {
				b = append(b, p.prevNL.Src()...)
				set = true
			}
			p.prevNL = t
			b = append(b, t.Sep()...)
		default:
			if set && !bytes.Equal(b, p.prevNL.Sep()) {
				p.prevNL.Set(b, p.prevNL.Src())
			}
			p.seq++
			t.seq = p.seq
			p.toks = append(p.toks, p.tok(t))
			return
		}
	}
}

func (p *parser) rune(checkTypeName bool) (r rune) {
	if len(p.toks) == 0 {
		p.next()
	}
	switch {
	case checkTypeName:
		t := p.toks[0]
		p.checkTypeName(&t)
		return t.Ch
	default:
		return p.toks[0].Ch
	}
}

func (p *parser) checkTypeName(t *Token) (r bool) {
	if t.Ch != rune(IDENTIFIER) {
		return false
	}

	if x, ok := p.scope.ident(*t).(*Declarator); ok {
		if x.isTypename {
			t.Ch = rune(TYPENAME)
			return true
		}
	}

	return false
}

func (p *parser) tok(t Token) (r Token) {
	r = t
	switch t.Ch {
	case rune(IDENTIFIER):
		if c, ok := p.isKeyword(r.Src()); ok {
			r.Ch = c
			break
		}
	case rune(PPNUMBER):
		switch s := r.SrcStr(); {
		case strings.ContainsAny(s, ".+-ijpIJP"):
			r.Ch = rune(FLOATCONST)
		case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
			r.Ch = rune(INTCONST)
		case strings.ContainsAny(s, "Ee"):
			r.Ch = rune(FLOATCONST)
		default:
			r.Ch = rune(INTCONST)
		}
	}
	return r
}

func (p *parser) shift0() (r Token) {
	r = p.toks[0]
	if r.Ch != '\n' && p.prevNL.Ch != 0 {
		sep := append(p.prevNL.Sep(), p.prevNL.Src()...)
		sep = append(sep, r.Sep()...)
		r.Set(sep, r.Src())
	}
	if r.Ch != eof {
		switch {
		case len(p.toks) == 1:
			p.toks = p.toks[:0]
		default:
			p.toks = p.toks[1:]
		}
		p.prevNL.Ch = 0
		p.rune(false)
	}
	return r
}

func (p *parser) shift(checkTypeName bool) (r Token) {
	r = p.shift0()
	// if r.Ch != ' ' && r.Ch != '\n' {
	// 	trc("%v: %v", pos(r), r)
	// }
	switch {
	case r.Ch == rune(STRINGLITERAL) && p.rune(false) == rune(STRINGLITERAL):
		sep := r.Sep()
		src := r.Src()
		for p.rune(false) == rune(STRINGLITERAL) {
			t := p.shift0()
			sep = append(sep, t.Sep()...)
			src = append(src[:len(src)-1], t.Src()[1:]...)
		}
		r.Set(sep, src)
	case r.Ch == rune(LONGSTRINGLITERAL) && (p.rune(false) == rune(LONGSTRINGLITERAL) || p.rune(false) == rune(STRINGLITERAL)):
		sep := r.Sep()
		src := r.Src()
	out:
		for {
			switch p.rune(false) {
			case rune(LONGSTRINGLITERAL):
				t := p.shift0()
				sep = append(sep, t.Sep()...)
				src = append(src[:len(src)-1], t.Src()[2:]...)
			case rune(STRINGLITERAL):
				t := p.shift0()
				sep = append(sep, t.Sep()...)
				src = append(src[:len(src)-1], t.Src()[1:]...)
			default:
				break out
			}
		}
		r.Set(sep, src)
	}
	if checkTypeName {
		p.checkTypeName(&r)
	}
	return r
}

func (p *parser) peek(i int, checkTypeName bool) (r Token) {
	for len(p.toks) <= i {
		p.next()
	}
	switch {
	case checkTypeName:
		r = p.toks[i]
		p.checkTypeName(&r)
		return r
	default:
		return p.toks[i]
	}
}

func (p *parser) must(r rune) Token {
	c := p.rune(false)
	t := p.shift(false)
	if c != r {
		p.cpp.eh("%v: unexpected %v, expected %v", t.Position(), runeName(c), runeName(r))
	}
	return t
}

func (p *parser) parse() (ast *AST, err error) {
	var errors errors
	p.cpp.eh = func(msg string, args ...interface{}) {
		s := fmt.Sprintf(msg, args...)
		if isTesting {
			s += fmt.Sprintf(" (%v: %v)", origin(2), origin(3))
			if traceFails {
				trc("%s FAIL (%v)", s, origin(1))
				// trc("\n%s", debug.Stack())
			}
		}
		errors = append(errors, s)
	}
	p.cpp.rune()
	tu := p.translationUnit()
	p.cpp.mmap = nil
	if p.scope == nil && p.scope.Parent != nil {
		p.cpp.eh("internal error (%v:)", origin(1))
	}
	switch p.rune(false) {
	case eof:
		var pd *Declarator
		if a := p.scope.Nodes["__predefined_declarator"]; len(a) != 0 {
			pd, _ = a[0].(*Declarator)
		}
		if pd == nil && !p.cpp.cfg.noPredefinedDeclarator {
			errors.add(errorf("<bultin>: missing definition of '__predefined_declarator'"))
		}
		t := p.shift(false)
		t.Set(append(p.prevNL.Sep(), p.prevNL.Src()...), nil)
		return &AST{
			ABI:                   p.cpp.cfg.ABI,
			EOF:                   t,
			Macros:                p.cpp.macros,
			Scope:                 p.scope,
			Declarations:          tu,
			predefinedDeclarator0: pd,
		}, errors.err()
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected EOF", t.Position(), runeName(t.Ch))
		return nil, errors.err()
	}
}

// [0], 6.9 External definitions
//
//  translation-unit:
// 	external-declaration
// 	translation-unit external-declaration
func (p *parser) translationUnit() (r []ExternalDeclaration) {
	for p.rune(false) != eof {
		tu := p.externalDeclaration()
		if tu == nil {
			return r
		}
		r = append(r, tu)
	}
	return r
}

//  external-declaration:
// 	function-definition
// 	declaration
// 	asm-function-definition
//	asm-statement
// 	;
func (p *parser) externalDeclaration() ExternalDeclaration {
again:
	switch p.rune(false) {
	case eof:
		return nil
	case rune(ASM):
		return p.asmStatement()
	case rune(STATICASSERT), rune(AUTOTYPE):
		return p.declaration(nil, nil, false)
	}

	ds, ok := p.declarationSpecifiers()
	if !ok {
		goto again
	}

	var d *Declarator
	if p.rune(false) != ';' {
		d = p.declarator(nil, ds, true)
	}
	switch p.rune(false) {
	case
		',',
		';',
		'=',
		rune(ASM),
		rune(ATTRIBUTE):

		//TODO wrong parse of pr37573.c
		return p.declaration(ds, d, true)
	case '{':
		return p.functionDefinition(ds, d)
	default:
		if d.isFn() {
			return p.functionDefinition(ds, d)
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected external declaration", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.9.1 Function definitions
//
//  function-definition:
// 	declaration-specifiers declarator declaration-list_opt compound-statement
func (p *parser) functionDefinition(ds DeclarationSpecifiers, d *Declarator) (r *FunctionDefinition) {
	defer func() {
		r.scope = p.fnScope
		p.fnScope = nil
	}()
	d.isFuncDef = true
	return &FunctionDefinition{Specifiers: ds, Declarator: d, Declarations: p.declarationListOpt(), Body: p.compoundStatement(false, true, d)}
}

//  declaration-list:
// 	declaration
// 	declaration-list declaration
func (p *parser) declarationListOpt() (r []Declaration) {
	for {
		switch p.rune(false) {
		case '{', eof:
			return r
		}
		r = append(r, p.declaration(nil, nil, false))
	}
}

// [0], 6.8.2 Compound statement
//
//  compound-statement:
// 	{ label-declaration_opt block-item-list_opt }
func (p *parser) compoundStatement(inFreshNewScope, isFnScope bool, d *Declarator) (r *CompoundStatement) {
	if !inFreshNewScope {
		p.newScope()

		defer p.closeScope()
	}

	if isFnScope {
		p.fnScope = p.scope
		gotos := p.gotos
		p.gotos = nil
		if d != nil {
			for dd := d.DirectDeclarator; dd != nil; {
				if s := dd.params; s != nil {
					for nm, nodes := range s.Nodes {
						for _, node := range nodes {
							p.scope.declare(p.cpp.eh, nm, node)
						}
					}
				}
				switch dd.Case {
				case DirectDeclaratorIdent:
					dd = nil
				case DirectDeclaratorDecl:
					dd = dd.Declarator.DirectDeclarator
				default:
					dd = dd.DirectDeclarator
				}
			}
		}
		defer func() {
			r.gotos = p.gotos
		next:
			for _, s := range r.Gotos() {
				switch s.Case {
				case JumpStatementGoto:
					nm := s.Token2.SrcStr()
					for sc := s.LexicalScope(); sc != nil && sc.Parent != nil; sc = sc.Parent {
						for _, n := range sc.Nodes[nm] {
							switch x := n.(type) {
							case *LabeledStatement:
								if nm == x.Token.SrcStr() {
									s.label = x
									continue next
								}
							case *LabelDeclaration:
								s.label = x
								continue next
							}
						}
					}
					p.cpp.eh("%v: label undefined: %s", s.Token2.Position(), nm)
				}
			}
			p.gotos = gotos
		}()
	}
	lbrace := p.must('{')
	if isFnScope && d != nil && !p.cpp.cfg.doNotInjectFunc {
		p.injectFuncTokens(lbrace, d.Name())
	}
	return &CompoundStatement{Lbrace: lbrace, List: p.blockItemListOpt(), Rbrace: p.must('}'), lexicalScope: (*lexicalScope)(p.scope)}
}

var funcTokensText = [][]byte{
	[]byte("static"),        // 0
	[]byte("const"),         // 1
	[]byte("char"),          // 2
	[]byte("__func__"),      // 3
	[]byte("["),             // 4
	[]byte("]"),             // 5
	[]byte("="),             // 6
	[]byte("function-name"), // 7
	[]byte(";"),             // 8
}

func (p *parser) injectFuncTokens(lbrace Token, nm string) {
	seq := lbrace.seq
	for i := range p.funcTokens {
		pt := &p.funcTokens[i]
		pt.s = lbrace.s
		pt.off = lbrace.off
		seq++
		pt.seq = seq
		if i != 7 {
			pt.Set(sp, funcTokensText[i])
		}
	}
	p.funcTokens[7].Set(sp, []byte(`"`+nm+`"`))
	p.rune(false)
	for i := range p.toks {
		seq++
		p.toks[i].seq = seq
	}
	p.seq = seq + 1
	p.toks = append(p.funcTokens, p.toks...)
}

//  block-item-list:
// 	block-item
// 	block-item-list block-item
func (p *parser) blockItemListOpt() (r []BlockItem) {
	for {
		switch p.rune(false) {
		case '}', eof:
			return r
		default:
			r = append(r, p.blockItem())
		}
	}
}

//  block-item:
// 	declaration
// 	statement
// 	label-declaration
// 	declaration-specifiers declarator compound-statement
func (p *parser) blockItem() BlockItem {
again:
	switch ch := p.rune(true); {
	case p.isStatement(ch) || p.isExpression(ch) || ch == rune(TYPENAME) && p.peek(1, false).Ch == ':':
		return p.statement(false)
	case p.isDeclarationSpecifier(ch, true):
		r0 := p.rune(false)
		ds, ok := p.declarationSpecifiers()
		if !ok {
			goto again
		}

		if r0 == rune(ATTRIBUTE) && p.rune(false) == ';' {
			return &ExpressionStatement{declarationSpecifiers: ds, AttributeSpecifiers: p.attributeSpecifierListOpt(), Token: p.shift(false)}
		}

		var d *Declarator
		if p.rune(false) != ';' {
			d = p.declarator(nil, ds, true)
		}
		switch p.rune(false) {
		case '{':
			fd := &FunctionDefinition{Specifiers: ds, Declarator: d, Body: p.compoundStatement(false, true, d)}
			fd.scope = fd.Body.LexicalScope()
			return fd
		default:
			return p.declaration(ds, d, true)
		}
	case ch == rune(LABEL):
		return p.labelDeclaration()
	case ch == rune(STATICASSERT), ch == rune(AUTOTYPE):
		return p.declaration(nil, nil, false)
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected block item", t.Position(), runeName(t.Ch))
		return nil
	}
}

//  label-declaration
// 	__label__ identifier-list ;
func (p *parser) labelDeclaration() (r *LabelDeclaration) {
	r = &LabelDeclaration{Token: p.shift(false), IdentifierList: p.identifierList(false), Token2: p.must(';')}
	for l := r.IdentifierList; l != nil; l = l.IdentifierList {
		p.scope.declare(p.cpp.eh, l.Token2.SrcStr(), r)
	}
	return r
}

// [0], 6.8 Statements and blocks
//
//  statement:
// 	labeled-statement
// 	compound-statement
// 	expression-statement
// 	selection-statement
// 	iteration-statement
// 	jump-statement
//	asm-statement
func (p *parser) statement(newBlock bool) Statement {
	if newBlock {
		p.newScope()

		defer p.closeScope()
	}
	switch ch := p.rune(false); {
	case ch == rune(IDENTIFIER):
		switch p.peek(1, false).Ch {
		case ':':
			return p.labeledStatement()
		default:
			return p.expressionStatement()
		}
	case p.isExpression(ch) || ch == ';' || ch == rune(ATTRIBUTE):
		return p.expressionStatement()
	}

	switch p.rune(true) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '{':
		return p.compoundStatement(newBlock, false, nil)
	case rune(ASM):
		return p.asmStatement()
	case
		rune(DO),
		rune(FOR),
		rune(WHILE):

		return p.iterationStatement()
	case
		rune(BREAK),
		rune(CONTINUE),
		rune(GOTO),
		rune(RETURN):

		return p.jumpStatement()
	case
		rune(IF),
		rune(SWITCH):

		return p.selectionStatement()
	case
		rune(CASE),
		rune(DEFAULT):

		return p.labeledStatement()
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected statement", t.Position(), runeName(t.Ch))
		return nil
	}
}

func (p *parser) isStatement(ch rune) bool {
	switch ch {
	case
		'(',
		'*',
		';',
		'{',
		rune(ASM),
		rune(BREAK),
		rune(CASE),
		rune(CONTINUE),
		rune(DEC),
		rune(DEFAULT),
		rune(DO),
		rune(FOR),
		rune(GOTO),
		rune(IDENTIFIER),
		rune(IF),
		rune(INC),
		rune(LONGSTRINGLITERAL),
		rune(RETURN),
		rune(STRINGLITERAL),
		rune(SWITCH),
		rune(WHILE):

		return true
	}
	return false
}

// [0], 6.8.4 Selection statements
//
//  selection-statement:
//  	if ( expression ) statement
//  	if ( expression ) statement else statement
//  	switch ( expression ) statement
func (p *parser) selectionStatement() (r *SelectionStatement) {
	// [0]6.8.4/3
	//
	// A selection statement is a block whose scope is a strict subset of the scope
	// of its enclosing block. Each associated substatement is also a block whose
	// scope is a strict subset of the scope of the selection statement.
	p.newScope()

	defer p.closeScope()

	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(IF):
		r = &SelectionStatement{Case: SelectionStatementIf, Token: p.shift(false), Token2: p.must('('), ExpressionList: p.expression(false), Token3: p.must(')'), Statement: p.statement(true), lexicalScope: (*lexicalScope)(p.scope)}
		switch p.rune(false) {
		case rune(ELSE):
			r.Case = SelectionStatementIfElse
			r.Token4 = p.shift(false)
			r.Statement2 = p.statement(true)
			return r
		default:
			return r
		}
	case rune(SWITCH):
		r = &SelectionStatement{Case: SelectionStatementSwitch, Token: p.shift(false), Token2: p.must('('), ExpressionList: p.expression(false), Token3: p.must(')'), lexicalScope: (*lexicalScope)(p.scope)}
		ctx := p.switchCtx
		p.switchCtx = r

		defer func() { p.switchCtx = ctx }()

		r.Statement = p.statement(true)
		return r
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected selection statement", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.8.6 Jump statements
//
//  jump-statement:
// 	goto identifier ;
// 	goto * expression ;
// 	continue ;
// 	break ;
// 	return expression_opt ;
func (p *parser) jumpStatement() (r *JumpStatement) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(BREAK):
		return &JumpStatement{Case: JumpStatementBreak, Token: p.shift(false), Token2: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(CONTINUE):
		return &JumpStatement{Case: JumpStatementContinue, Token: p.shift(false), Token2: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(GOTO):
		switch p.peek(1, false).Ch {
		case '*':
			r = &JumpStatement{Case: JumpStatementGotoExpr, Token: p.shift(false), Token2: p.shift(false), ExpressionList: p.expression(false), Token3: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
		default:
			r = &JumpStatement{Case: JumpStatementGoto, Token: p.shift(false), Token2: p.must(rune(IDENTIFIER)), Token3: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
		}
		p.gotos = append(p.gotos, r)
		return r
	case rune(RETURN):
		return &JumpStatement{Case: JumpStatementReturn, Token: p.shift(false), ExpressionList: p.expression(true), Token2: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected jump statement", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.8.1 Labeled statements
//
//  labeled-statement:
// 	identifier : statement
// 	case constant-expression : statement
// 	case constant-expression ... constant-expression : statement
// 	default : statement
func (p *parser) labeledStatement() (r *LabeledStatement) {
	defer func() {
		if ctx := p.switchCtx; ctx != nil && r != nil {
			ctx.labeled = append(ctx.labeled, r)
		}
	}()
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(CASE):
		r = &LabeledStatement{Token: p.shift(false), Expression: p.constantExpression(), lexicalScope: (*lexicalScope)(p.scope), switchCtx: p.switchCtx}
		switch p.rune(false) {
		case rune(DDD):
			r.Case = LabeledStatementRange
			r.Token2 = p.shift(false)
			r.Expression2 = p.constantExpression()
			r.Token3 = p.must(':')
			r.Statement = p.statement(false)
			return r
		default:
			r.Case = LabeledStatementCaseLabel
			r.Token2 = p.must(':')
			r.Statement = p.statement(false)
			return r
		}
	case rune(DEFAULT):
		return &LabeledStatement{Case: LabeledStatementDefault, Token: p.shift(false), Token2: p.must(':'), Statement: p.statement(false), lexicalScope: (*lexicalScope)(p.scope), switchCtx: p.switchCtx}
	case rune(IDENTIFIER):
		r = &LabeledStatement{Case: LabeledStatementLabel, Token: p.shift(false), Token2: p.must(':'), Statement: p.statement(false), lexicalScope: (*lexicalScope)(p.scope), switchCtx: p.switchCtx}
		switch {
		case p.fnScope == nil:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected label", t.Position())
		default:
			p.fnScope.declare(p.cpp.eh, r.Token.SrcStr(), r)
		}
		return r
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected labeled statement", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.8.5 Iteration statements
//
//  iteration-statement:
// 	while ( expression ) statement
// 	do statement while ( expression ) ;
// 	for ( expression_opt ; expression_opt ; expression_opt ) statement
// 	for ( declaration expression_opt ; expression_opt ) statement
func (p *parser) iterationStatement() (r *IterationStatement) {
	// [0]6.8.5/5
	//
	// An iteration statement is a block whose scope is a strict subset of the
	// scope of its enclosing block. The loop body is also a block whose scope is a
	// strict subset of the scope of the iteration statement.
	p.newScope()

	defer p.closeScope()

	switch p.rune(false) {
	case rune(WHILE):
		return &IterationStatement{Case: IterationStatementWhile, Token: p.shift(false), Token2: p.must('('), ExpressionList: p.expression(false), Token3: p.must(')'), Statement: p.statement(true), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(DO):
		return &IterationStatement{Case: IterationStatementDo, Token: p.shift(false), Statement: p.statement(true), Token2: p.must(rune(WHILE)), Token3: p.must('('), ExpressionList: p.expression(false), Token4: p.must(')'), Token5: p.must(';'), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(FOR):
		switch ch := p.peek(2, true).Ch; {
		case p.isDeclarationSpecifier(ch, true):
			return &IterationStatement{Case: IterationStatementForDecl, Token: p.shift(false), Token2: p.must('('), Declaration: p.declaration(nil, nil, true), ExpressionList: p.expression(true), Token3: p.must(';'), ExpressionList2: p.expression(true), Token4: p.must(')'), Statement: p.statement(true), lexicalScope: (*lexicalScope)(p.scope)}
		}

		return &IterationStatement{Case: IterationStatementFor, Token: p.shift(false), Token2: p.must('('), ExpressionList: p.expression(true), Token3: p.must(';'), ExpressionList2: p.expression(true), Token4: p.must(';'), ExpressionList3: p.expression(true), Token5: p.must(')'), Statement: p.statement(true), lexicalScope: (*lexicalScope)(p.scope)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected iteration statement", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.8.3 Expression and null statements
//
//  expression-statement:
// 	expression_opt;
func (p *parser) expressionStatement() *ExpressionStatement {
	switch p.rune(false) {
	case rune(ATTRIBUTE):
		t := p.shift(false)
		p.cpp.eh("%v: internal error: TODO", t.Position(), runeName(t.Ch))
		return nil
	default:
		return &ExpressionStatement{ExpressionList: p.expression(true), Token: p.must(';')}
	}
}

//  asm-statement:
//  	asm ;
func (p *parser) asmStatement() *AsmStatement {
	return &AsmStatement{Asm: p.asm(), Token: p.must(';')}
}

//  asm:
// 	asm asm-qualifier-list_opt ( string-literal asm-arg-list_opt )
func (p *parser) asm() *Asm {
	return &Asm{Token: p.must(rune(ASM)), AsmQualifierList: p.asmQualifierListOpt(), Token2: p.must('('), Token3: p.must(rune(STRINGLITERAL)), AsmArgList: p.asmArgListOpt(), Token4: p.must(')')}
}

//  asm-qualifier-list:
// 	asm-qualifier
// 	asm-qualifier-list asm-qualifier
func (p *parser) asmQualifierListOpt() (r *AsmQualifierList) {
	var prev *AsmQualifierList
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return r
		case rune(GOTO), rune(INLINE), rune(VOLATILE):
			aql := &AsmQualifierList{AsmQualifier: p.asmQualifier()}
			switch {
			case r == nil:
				r = aql
			default:
				prev.AsmQualifierList = aql
			}
			prev = aql
		default:
			return r
		}
	}
}

//  asm-qualifier:
// 	volatile
//  	inline
// 	goto"
func (p *parser) asmQualifier() *AsmQualifier {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(VOLATILE):
		return &AsmQualifier{Case: AsmQualifierVolatile, Token: p.shift(false)}
	case rune(INLINE):
		return &AsmQualifier{Case: AsmQualifierInline, Token: p.shift(false)}
	case rune(GOTO):
		return &AsmQualifier{Case: AsmQualifierGoto, Token: p.shift(false)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected asm qualifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

//  asm-arg-list:
// 	: asm-expression-list
// 	asm-arg-list : asm-expression-list
func (p *parser) asmArgListOpt() (r *AsmArgList) {
	var prev *AsmArgList
	for p.rune(false) == ':' {
		var aal *AsmArgList
		switch p.peek(1, false).Ch {
		case
			')',
			':':

			aal = &AsmArgList{Token: p.shift(false)}
		default:
			aal = &AsmArgList{Token: p.shift(false), AsmExpressionList: p.asmExpressionList()}
		}
		switch {
		case r == nil:
			r = aal
		default:
			prev.AsmArgList = aal
		}
		prev = aal
	}
	return r
}

//  asm-expression-list:
// 	asm-index_opt assignment-expression
// 	asm-expression-list , asm-index_opt assignment-expression
func (p *parser) asmExpressionList() (r *AsmExpressionList) {
	r = &AsmExpressionList{AsmIndex: p.asmIndexOpt(), Expression: p.assignmentExpression(true)}
	prev := r
	for p.rune(false) == ',' {
		ael := &AsmExpressionList{Token: p.shift(false), AsmIndex: p.asmIndexOpt(), Expression: p.assignmentExpression(true)}
		prev.AsmExpressionList = ael
		prev = ael
	}
	return r
}

//  asm-index:
// 	[ expression ]
func (p *parser) asmIndexOpt() *AsmIndex {
	if p.rune(false) != '[' {
		return nil
	}

	return &AsmIndex{Token: p.shift(false), ExpressionList: p.expression(false), Token2: p.must(']')}
}

// [0], 6.5.17 Comma operator
//
//  expression:
// 	assignment-expression
// 	expression , assignment-expression
func (p *parser) expression(opt bool) Expression {
	var r *ExpressionList
	if p.isExpression(p.rune(false)) {
		ae := p.assignmentExpression(true)
		if p.rune(false) != ',' {
			return ae
		}

		r = &ExpressionList{List: []Expression{ae}}
		for p.rune(false) == ',' {
			r.Tokens = append(r.Tokens, p.shift(false))
			r.List = append(r.List, p.assignmentExpression(true))
		}
		return r
	}

	if opt {
		return nil
	}

	t := p.shift(false)
	p.cpp.eh("%v: unexpected %v, expected asm expression", t.Position(), runeName(t.Ch))
	return nil
}

func (p *parser) isExpression(ch rune) bool {
	switch ch {
	case
		'!',
		'&',
		'(',
		'*',
		'+',
		'-',
		'~',
		rune(ALIGNOF),
		rune(ANDAND),
		rune(CHARCONST),
		rune(DEC),
		rune(FLOATCONST),
		rune(GENERIC),
		rune(IDENTIFIER),
		rune(IMAG),
		rune(INC),
		rune(INTCONST),
		rune(LONGCHARCONST),
		rune(LONGSTRINGLITERAL),
		rune(REAL),
		rune(SIZEOF),
		rune(STRINGLITERAL):

		return true
	}
	return false
}

// [0], 6.7 Declarations
//
//  declaration:
// 	declaration-specifiers init-declarator-list_opt attribute-specifier-list_opt;
//	static-assert-declaration
//	__auto_type ident = initializer ;
func (p *parser) declaration(ds DeclarationSpecifiers, d *Declarator, declare bool) (r Declaration) {
	switch p.rune(false) {
	case rune(STATICASSERT):
		return p.staticAssertDeclaration()
	case rune(AUTOTYPE):
		nm := p.peek(1, true)
		if nm.Ch != rune(IDENTIFIER) {
			p.cpp.eh("%v: unexpected %v, expected identifier", nm.Position(), runeName(nm.Ch))
			p.shift(false)
			p.shift(false)
			return nil
		}

		a := &AutoDeclaration{Token: p.shift(false), Declarator: p.declarator(nil, nil, false)}
		r = a
		a.Token2 = p.must('=')
		a.Initializer = p.initializer(nil)
		a.Token3 = p.must(';')
		a.Declarator.visible = visible(a.Token3.seq)
		p.scope.declare(p.cpp.eh, nm.SrcStr(), a.Declarator)
		return r
	}

	if len(ds) == 0 {
		var ok bool
		if ds, ok = p.declarationSpecifiers(); !ok {
			return nil
		}
	}

	return &CommonDeclaration{DeclarationSpecifiers: ds, InitDeclarators: p.initDeclaratorListOpt(ds, d, declare), AttributeSpecifiers: p.attributeSpecifierListOpt(), Token: p.must(';')}
}

//  static-assert-declaration
//	_Static_assert ( constant-expression , string-literal )
func (p *parser) staticAssertDeclaration() *StaticAssertDeclaration {
	return &StaticAssertDeclaration{Token: p.must(rune(STATICASSERT)), Token2: p.must('('), Expression: p.constantExpression(), Token3: p.must(','), Token4: p.must(rune(STRINGLITERAL)), Token5: p.must(')')}
}

//  attribute-specifier-list:
// 	attribute-specifier
// 	attribute-specifier-list attribute-specifier
func (p *parser) attributeSpecifierListOpt() (r []*AttributeSpecifier) {
	for p.rune(false) == rune(ATTRIBUTE) {
		r = append(r, p.attributeSpecifier())
	}
	return r
}

//  attribute-specifier:
// 	__attribute__ (( attribute-value-list_opt ))
func (p *parser) attributeSpecifier() (r *AttributeSpecifier) {
	return &AttributeSpecifier{Token: p.must(rune(ATTRIBUTE)), Token2: p.must('('), Token3: p.must('('), AttributeValueList: p.attributeValueListOpt(), Token4: p.must(')'), Token5: p.must(')')}
}

//  attribute-value-list:
// 	attribute-value
// 	attribute-value-list , attribute-value
func (p *parser) attributeValueListOpt() (r *AttributeValueList) {
	switch p.rune(false) {
	case eof:
		return r
	case rune(IDENTIFIER):
		r = &AttributeValueList{AttributeValue: p.attributeValue()}
	case ')':
		return nil
	default:
		if _, ok := p.isKeyword(p.toks[0].Src()); ok {
			r = &AttributeValueList{AttributeValue: p.attributeValue()}
			break
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected attribute value", t.Position(), runeName(t.Ch))
	}
	prev := r
	for p.rune(false) == ',' {
		avl := &AttributeValueList{Token: p.shift(false), AttributeValue: p.attributeValue()}
		prev.AttributeValueList = avl
	}
	return r
}

//  attribute-value:
// 	identifier
// 	identifier ( expression-list_opt )
func (p *parser) attributeValue() (r *AttributeValue) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(IDENTIFIER):
		switch p.peek(1, false).Ch {
		case '(':
			return &AttributeValue{Case: AttributeValueExpr, Token: p.shift(false), Token2: p.shift(false), ArgumentExpressionList: p.argumentExpressionListOpt(false), Token3: p.must(')')}
		default:
			return &AttributeValue{Case: AttributeValueIdent, Token: p.shift(false)}
		}
	default:
		if _, ok := p.isKeyword(p.toks[0].Src()); ok {
			r = &AttributeValue{Case: AttributeValueIdent, Token: p.shift(false)}
			r.Token.Ch = rune(IDENTIFIER)
			return r
		}

		p.cpp.eh("%v: unexpected %v, expected identifier", p.toks[0].Position(), runeName(p.rune(false)))
		p.shift(false)
		return nil
	}
}

//  init-declarator-list:
// 	init-declarator
// 	init-declarator-list , init-declarator
func (p *parser) initDeclaratorListOpt(ds DeclarationSpecifiers, d *Declarator, declare bool) (r []*InitDeclarator) {
	switch {
	case d == nil:
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case ';':
			return nil
		case
			'(',
			'*',
			rune(IDENTIFIER):

			d = p.declarator(nil, ds, declare)
		default:
			p.cpp.eh("%v: unexpected %v, expected direct-declarator", p.toks[0].Position(), runeName(p.rune(false)))
			p.shift(false)
			return nil
		}
	}
	var list []*InitDeclarator
	prev := p.initDeclarator(ds, d, declare)
	list = append(list, prev)
	for p.rune(false) == ',' {
		prev.Token2 = p.shift(false)
		attrs := p.attributeSpecifierListOpt()
		prev = p.initDeclarator(ds, nil, declare)
		prev.AttributeSpecifiers = attrs
		list = append(list, prev)
	}
	return list
}

//  init-declarator:
// 	declarator asm-register-var_opt
// 	declarator asm-register-var_opt = initializer
func (p *parser) initDeclarator(ds DeclarationSpecifiers, d *Declarator, declare bool) (r *InitDeclarator) {
	if d == nil {
		d = p.declarator(nil, ds, declare)
	}

	r = &InitDeclarator{Declarator: d}
	if p.rune(false) == rune(ASM) {
		r.Asm = p.asm()
	}
	if p.rune(false) == rune(ATTRIBUTE) {
		r.AttributeSpecifiers = p.attributeSpecifierListOpt()
	}
	if p.rune(false) == '=' {
		d.hasInitializer = true
		if d.isTypename {
			p.cpp.eh("%v: typedef '%s' is initialized", p.toks[0].Position(), d.Name())
		}
		r.Token = p.shift(false)
		r.Initializer = p.initializer(nil)
	}
	return r
}

// [0], 6.7.8 Initialization
//
//  initializer:
// 	assignment-expression
// 	{ initializer-list }
// 	{ initializer-list , }
func (p *parser) initializer(parent *Initializer) (r *Initializer) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '{':
		r = &Initializer{Case: InitializerInitList, Token: p.shift(false), parent: parent}
		r.InitializerList = p.initializerList(r)
		if p.rune(false) == ',' {
			r.Token2 = p.shift(false)
		}

		r.Token3 = p.must('}')
		return r
	default:
		return &Initializer{Case: InitializerExpr, Expression: p.assignmentExpression(true), parent: parent}
	}
}

//  initializer-list:
// 	designation_opt initializer
// 	initializer-list , designation_opt initializer
func (p *parser) initializerList(parent *Initializer) (r *InitializerList) {
	switch p.rune(true) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '[', '.':
		r = &InitializerList{Designation: p.designation(), Initializer: p.initializer(parent)}
	case rune(IDENTIFIER):
		switch p.peek(1, false).Ch {
		case ':':
			r = &InitializerList{Designation: p.designation(), Initializer: p.initializer(parent)}
		default:
			r = &InitializerList{Initializer: p.initializer(parent)}
		}
	case '{':
		r = &InitializerList{Initializer: p.initializer(parent)}
	case '}':
		return nil
	default:
		if p.isExpression(p.rune(true)) {
			r = &InitializerList{Initializer: p.initializer(parent)}
			break
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected designation or initializer", t.Position(), runeName(t.Ch))
	}
	prev := r
	for p.rune(false) == ',' {
		var il *InitializerList
		switch p.peek(1, true).Ch {
		case
			'.',
			'[':

			il = &InitializerList{Token: p.shift(false), Designation: p.designation(), Initializer: p.initializer(parent)}
		case rune(IDENTIFIER):
			switch p.peek(2, false).Ch {
			case ':':
				il = &InitializerList{Token: p.shift(false), Designation: p.designation(), Initializer: p.initializer(parent)}
			default:
				il = &InitializerList{Token: p.shift(false), Initializer: p.initializer(parent)}
			}
		case '}':
			return r
		default:
			il = &InitializerList{Token: p.shift(false), Initializer: p.initializer(parent)}
		}
		prev.InitializerList = il
		prev = il
	}
	return r
}

//  designation:
// 	designator-list =
func (p *parser) designation() (r *Designation) {
	p.rune(false)
	dl, last := p.designatorList()
	r = &Designation{Designators: dl}
	if last != DesignatorField2 {
		r.Token = p.must('=')
	}
	return r
}

//  designator-list:
// 	designator
// 	designator-list designator
func (p *parser) designatorList() (r []*Designator, last DesignatorCase) {
	for {
		var dl *Designator
		switch p.rune(true) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, 0
		case '[', '.':
			dl = p.designator()
		case rune(IDENTIFIER):
			switch p.peek(1, false).Ch {
			case ':':
				dl = p.designator()
			default:
				return r, last
			}
		default:
			return r, last
		}
		last = dl.Case
		r = append(r, dl)
	}
}

//  designator:
// 	[ constant-expression ]
// 	[ constant-expression ... constant-expression]
// 	. identifier
//	identifier :
func (p *parser) designator() (r *Designator) {
	switch p.rune(true) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '[':
		r = &Designator{Case: DesignatorIndex, Token: p.shift(false), ConstantExpression: p.constantExpression()}
		switch p.rune(false) {
		case rune(DDD):
			r.Case = DesignatorIndex2
			r.Token2 = p.shift(false)
			r.ConstantExpression2 = p.constantExpression()
			r.Token3 = p.must(']')
			return r
		default:
			r.Token2 = p.must(']')
			return r
		}
	case '.':
		return &Designator{Case: DesignatorField, Token: p.shift(false), Token2: p.must(rune(IDENTIFIER))}
	case rune(IDENTIFIER):
		switch p.peek(1, false).Ch {
		case ':':
			return &Designator{Case: DesignatorField2, Token: p.shift(false), Token2: p.shift(false)}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected designator", t.Position(), runeName(t.Ch))
			return nil
		}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected designator", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.5.16 Assignment operators
//
//  assignment-expression:
// 	conditional-expression
// 	unary-expression assignment-operator assignment-expression
//
//  assignment-operator: one of
// 	= *= /= %= += -= <<= >>= &= ^= |=
func (p *parser) assignmentExpression(checkTypeName bool) Expression {
	lhs, u := p.conditionalExpression(checkTypeName)
	var r *AssignmentExpression
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '=':
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationAssign, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(MULASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationMul, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(DIVASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationDiv, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(MODASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationMod, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(ADDASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationAdd, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(SUBASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationSub, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(LSHASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationLsh, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(RSHASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationRsh, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(ANDASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationAnd, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(XORASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationXor, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	case rune(ORASSIGN):
		r = &AssignmentExpression{Lhs: u, Op: AssignmentOperationOr, Token: p.shift(false), Rhs: p.assignmentExpression(checkTypeName)}
	default:
		return lhs
	}
	if u == nil {
		p.cpp.eh("%v: LHS must be unary-expression", r.Token.Position())
	}
	return r
}

// [0], 6.5.15 Conditional operator
//
//  conditional-expression:
// 	logical-OR-expression
// 	logical-OR-expression ? expression : conditional-expression
func (p *parser) conditionalExpression(checkTypeName bool) (r, u Expression) {
	lhs, u := p.logicalOrExpression(checkTypeName)
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil, nil
	case '?':
		r := &ConditionalExpression{Condition: lhs, Token: p.shift(false), Then: p.expression(true), Token2: p.must(':')}
		r.Else, _ = p.conditionalExpression(checkTypeName)
		return r, nil
	default:
		return lhs, u
	}
}

// [0], 6.5.14 Logical OR operator
//
//  logical-OR-expression:
// 	logical-AND-expression
// 	logical-OR-expression || logical-AND-expression
func (p *parser) logicalOrExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.logicalAndExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case rune(OROR):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationLOr, Token: p.shift(false)}
			s.Rhs, _ = p.logicalAndExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.13 Logical AND operator
//
//  logical-AND-expression:
// 	inclusive-OR-expression
// 	logical-AND-expression && inclusive-OR-expression
func (p *parser) logicalAndExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.inclusiveOrExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case rune(ANDAND):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationLAnd, Token: p.shift(false)}
			s.Rhs, _ = p.inclusiveOrExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.12 Bitwise inclusive OR operator
//
//  inclusive-OR-expression:
// 	exclusive-OR-expression
// 	inclusive-OR-expression | exclusive-OR-expression
func (p *parser) inclusiveOrExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.exclusiveOrExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '|':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationOr, Token: p.shift(false)}
			s.Rhs, _ = p.exclusiveOrExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.11 Bitwise exclusive OR operator
//
//  exclusive-OR-expression:
// 	AND-expression
// 	exclusive-OR-expression ^ AND-expression
func (p *parser) exclusiveOrExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.andExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '^':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationXor, Token: p.shift(false)}
			s.Rhs, _ = p.andExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.10 Bitwise AND operator
//
//  AND-expression:
// 	equality-expression
// 	AND-expression & equality-expression
func (p *parser) andExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.equalityExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '&':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationAnd, Token: p.shift(false)}
			s.Rhs, _ = p.equalityExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.9 Equality operators
//
//  equality-expression:
// 	relational-expression
// 	equality-expression == relational-expression
// 	equality-expression != relational-expression
func (p *parser) equalityExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.relationalExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case rune(EQ):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationEq, Token: p.shift(false)}
			s.Rhs, _ = p.relationalExpression(checkTypeName)
		case rune(NEQ):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationNeq, Token: p.shift(false)}
			s.Rhs, _ = p.relationalExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.8 Relational operators
//
//  relational-expression:
// 	shift-expression
// 	relational-expression <  shift-expression
// 	relational-expression >  shift-expression
// 	relational-expression <= shift-expression
// 	relational-expression >= shift-expression
func (p *parser) relationalExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.shiftExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '<':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationLt, Token: p.shift(false)}
			s.Rhs, _ = p.shiftExpression(checkTypeName)
		case '>':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationGt, Token: p.shift(false)}
			s.Rhs, _ = p.shiftExpression(checkTypeName)
		case rune(LEQ):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationLeq, Token: p.shift(false)}
			s.Rhs, _ = p.shiftExpression(checkTypeName)
		case rune(GEQ):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationGeq, Token: p.shift(false)}
			s.Rhs, _ = p.shiftExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.7 Bitwise shift operators
//
//  shift-expression:
// 	additive-expression
// 	shift-expression << additive-expression
// 	shift-expression >> additive-expression
func (p *parser) shiftExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.additiveExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case rune(LSH):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationLsh, Token: p.shift(false)}
			s.Rhs, _ = p.additiveExpression(checkTypeName)
		case rune(RSH):
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationRsh, Token: p.shift(false)}
			s.Rhs, _ = p.additiveExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.6 Additive operators
//
//  additive-expression:
// 	multiplicative-expression
// 	additive-expression + multiplicative-expression
// 	additive-expression - multiplicative-expression
func (p *parser) additiveExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.multiplicativeExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch op := p.rune(false); op {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '+':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationAdd, Token: p.shift(false)}
			s.Rhs, _ = p.multiplicativeExpression(checkTypeName)
		case '-':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationSub, Token: p.shift(false)}
			s.Rhs, _ = p.multiplicativeExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.5 Multiplicative operators
//
//  multiplicative-expression:
// 	cast-expression
// 	multiplicative-expression * cast-expression
// 	multiplicative-expression / cast-expression
// 	multiplicative-expression % cast-expression
func (p *parser) multiplicativeExpression(checkTypeName bool) (r, u Expression) {
	r, u = p.castExpression(checkTypeName)
	var s *BinaryExpression
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, nil
		case '*':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationMul, Token: p.shift(false)}
			s.Rhs, _ = p.castExpression(checkTypeName)
		case '/':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationDiv, Token: p.shift(false)}
			s.Rhs, _ = p.castExpression(checkTypeName)
		case '%':
			s = &BinaryExpression{Lhs: r, Op: BinaryOperationMod, Token: p.shift(false)}
			s.Rhs, _ = p.castExpression(checkTypeName)
		default:
			return r, u
		}
		r = s
		u = nil
	}
}

// [0], 6.5.4 Cast operators
//
//  cast-expression:
// 	unary-expression
// 	( type-name ) cast-expression
func (p *parser) castExpression(checkTypeName bool) (r, u Expression) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil, nil
	case '(':
		switch ch := p.peek(1, true).Ch; {
		case p.isSpecifierQualifer(ch, true) || ch == rune(ATTRIBUTE):
			lparen := p.shift(false)
			tn := p.typeName()
			rparen := p.must(')')
			switch p.rune(false) {
			case eof:
				p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
				return nil, nil
			case '{':
				u = p.unaryExpression(lparen, tn, rparen, checkTypeName)
				return u, u
			default:
				r := &CastExpr{Lparen: lparen, TypeName: tn, Rparen: rparen}
				r.Expr, _ = p.castExpression(checkTypeName)
				return r, nil
			}
		case p.isExpression(ch) || ch == '{':
			r = p.unaryExpression(Token{}, nil, Token{}, checkTypeName)
			return r, r
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected cast expression", t.Position(), runeName(t.Ch))
		return nil, nil
	default:
		r := p.unaryExpression(Token{}, nil, Token{}, checkTypeName)
		return r, r
	}
}

// [0], 6.7.6 Type names
//
//  type-name:
// 	specifier-qualifier-list abstract-declarator_opt
func (p *parser) typeName() *TypeName {
	return &TypeName{SpecifierQualifiers: p.specifierQualifierList(), AbstractDeclarator: p.abstractDeclarator(nil, true)}
}

//  abstract-declarator:
// 	pointer
// 	pointer_opt direct-abstract-declarator
func (p *parser) abstractDeclarator(ptr *Pointer, opt bool) *AbstractDeclarator {
	if ptr == nil {
		p.attributeSpecifierListOpt() //TODO not stored yet
		ptr = p.pointer(true)
	}
	switch {
	case ptr == nil:
		switch p.rune(false) {
		case
			')',
			':':

			if opt {
				return nil
			}

			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected abstract declarator", t.Position(), runeName(t.Ch))
			return nil
		case '[':
			return &AbstractDeclarator{Case: AbstractDeclaratorDecl, DirectAbstractDeclarator: p.directAbstractDeclarator()}
		case '(':
			switch p.peek(1, true).Ch {
			case
				'*',
				rune(ATTRIBUTE):

				return &AbstractDeclarator{Case: AbstractDeclaratorPtr, DirectAbstractDeclarator: p.directAbstractDeclarator()}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		default:
			switch p.rune(true) {
			case rune(IDENTIFIER):
				t := p.shift(true)
				p.cpp.eh("%v: unexpected %v, expected ';'", t.Position(), runeName(t.Ch))
				return nil
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		}
	default:
		switch p.rune(false) {
		case
			')',
			':':

			return &AbstractDeclarator{Case: AbstractDeclaratorPtr, Pointer: ptr}
		case '[':
			return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr, DirectAbstractDeclarator: p.directAbstractDeclarator()}
		case '(':
			switch ch := p.peek(1, true).Ch; {
			case ch == '*' || p.isDeclarationSpecifier(ch, true):
				return &AbstractDeclarator{Case: AbstractDeclaratorPtr, Pointer: ptr, DirectAbstractDeclarator: p.directAbstractDeclarator()}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected abstract declarator", t.Position(), runeName(t.Ch))
			return nil
		}
	}
}

//  direct-abstract-declarator:
// 	( abstract-declarator )
// 	direct-abstract-declarator_opt [ type-qualifier-list_opt assignment-expression_opt ]
// 	direct-abstract-declarator_opt [ static type-qualifier-list_opt assignment-expression ]
// 	direct-abstract-declarator_opt [ type-qualifier-list static assignment-expression ]
// 	direct-abstract-declarator_opt [ * ]
// 	direct-abstract-declarator_opt ( parameter-type-list_opt )
func (p *parser) directAbstractDeclarator() (r *DirectAbstractDeclarator) {
out:
	switch p.rune(false) {
	case '[':
		switch ch := p.peek(1, false).Ch; {
		case p.isTypeQualifier(ch):
			lbracket := p.shift(false)
			tql := p.typeQualifierList(false, false)
			switch p.rune(false) {
			case eof:
				p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
				return nil
			case rune(INTCONST):
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, Token: lbracket, TypeQualifiers: tql, Expression: p.assignmentExpression(true), Token2: p.must(']')}
				break out
			case rune(STATIC):
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, Token: lbracket, TypeQualifiers: tql, Token2: p.shift(false), Expression: p.assignmentExpression(true), Token3: p.must(']')}
				break out
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
			}
		case ch != '*' && p.isExpression(ch):
			r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, Token: p.shift(false), Expression: p.assignmentExpression(true), Token2: p.must(']')}
			break out
		}

		switch p.peek(1, false).Ch {
		case ']':
			r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, Token: p.shift(false), Token2: p.shift(false)}
		case rune(STATIC):
			lbracket := p.shift(false)
			switch {
			case p.isTypeQualifier(p.peek(1, false).Ch):
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorStaticArr, Token: lbracket, Token2: p.shift(false), TypeQualifiers: p.typeQualifierList(false, false), Expression: p.assignmentExpression(true), Token3: p.must(']')}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
			}
		case '*':
			switch p.peek(2, false).Ch {
			case ']':
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArrStar, Token: p.shift(false), Token2: p.shift(false), Token3: p.shift(false)}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
			}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
		}
	case '(':
		switch ch := p.peek(1, true).Ch; {
		case p.in(ch, '*', rune(ATTRIBUTE)):
			r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorDecl, Token: p.shift(false), AbstractDeclarator: p.abstractDeclarator(nil, false), Token2: p.must(')')}
		case p.isDeclarationSpecifier(ch, true):
			p.newScope()

			func() {
				defer p.closeScope()
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorFunc, Token: p.shift(false), ParameterTypeList: p.parameterTypeListOpt(), Token2: p.must(')'), params: p.scope}
			}()
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
		}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
	}
	return p.directAbstractDeclarator2(r)
}

func (p *parser) directAbstractDeclarator2(dad *DirectAbstractDeclarator) (r *DirectAbstractDeclarator) {
	r = dad
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case
			')',
			',',
			':':

			return r
		case '(':
			r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorFunc, DirectAbstractDeclarator: r, Token: p.shift(false), ParameterTypeList: p.parameterTypeListOpt(), Token2: p.must(')')}
		case '[':
			ch := p.peek(1, true).Ch
			switch {
			case ch == ']':
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, DirectAbstractDeclarator: r, Token: p.shift(false), Token2: p.must(']')}
			case p.isExpression(ch):
				r = &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorArr, DirectAbstractDeclarator: r, Token: p.shift(false), Expression: p.assignmentExpression(true), Token2: p.must(']')}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
			}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected direct abstract declarator", t.Position(), runeName(t.Ch))
		}
	}
}

//  specifier-qualifier-list:
// 	type-specifier specifier-qualifier-list_opt
// 	type-qualifier specifier-qualifier-list_opt
// 	alignment-specifier specifier-qualifier-list_opt
func (p *parser) specifierQualifierList() (r []SpecifierQualifier) {
	acceptTypeName := true
	for {
		var sql SpecifierQualifier
		switch ch := p.rune(false); {
		case ch == rune(IDENTIFIER):
			if !acceptTypeName || !p.checkTypeName(&p.toks[0]) {
				return r
			}

			sql = p.typeSpecifier()
			acceptTypeName = false
		case ch == rune(ATOMIC) && p.peek(1, false).Ch == '(':
			sql = p.typeSpecifier()
			acceptTypeName = false
		case p.isTypeSpecifier(ch, false):
			sql = p.typeSpecifier()
			acceptTypeName = false
		case p.isTypeQualifier(ch) || ch == rune(ATTRIBUTE):
			sql = p.typeQualifier(true)
		case ch == rune(ALIGNAS):
			sql = p.alignmentSpecifier()
		default:
			return r
		}
		r = append(r, sql)
	}
}

func (p *parser) isSpecifierQualifer(ch rune, typenameOk bool) bool {
	return p.isTypeSpecifier(ch, typenameOk) || p.isTypeQualifier(ch)
}

// [0], 6.5.3 Unary operators
//
//  unary-expression:
// 	postfix-expression
// 	++ unary-expression
// 	-- unary-expression
// 	unary-operator cast-expression
// 	sizeof unary-expression
// 	sizeof ( type-name )
// 	&& identifier
// 	_Alignof unary-expression
// 	_Alignof ( type-name )
// 	__imag__ unary-expression
// 	__real__ unary-expression
//
//  unary-operator: one of
// 	& * + - ~ !
func (p *parser) unaryExpression(lp Token, tn *TypeName, rp Token, checkTypeName bool) Expression {
	if tn != nil {
		return p.postfixExpression(lp, tn, rp, checkTypeName)
	}

	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '&':
		e := &UnaryExpr{Case: UnaryExpressionAddrof, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case '*':
		e := &UnaryExpr{Case: UnaryExpressionDeref, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case '+':
		e := &UnaryExpr{Case: UnaryExpressionPlus, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case '-':
		e := &UnaryExpr{Case: UnaryExpressionMinus, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case '~':
		e := &UnaryExpr{Case: UnaryExpressionCpl, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case '!':
		e := &UnaryExpr{Case: UnaryExpressionNot, Token: p.shift(false)}
		e.Expr, _ = p.castExpression(checkTypeName)
		return e
	case rune(SIZEOF):
		switch p.peek(1, false).Ch {
		case '(':
			switch {
			case p.isSpecifierQualifer(p.peek(2, true).Ch, true):
				sz := p.shift(false)
				lp := p.shift(false)
				tn := p.typeName()
				rp := p.must(')')
				switch p.rune(false) {
				case '{':
					return &SizeOfExpr{Token: sz, Expr: p.unaryExpression(lp, tn, rp, checkTypeName)}
				default:
					return &SizeOfTypeExpr{Token: sz, LeftParen: lp, TypeName: tn, RightParen: rp}
				}
			default:
				return &SizeOfExpr{Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName)}
			}
		default:
			return &SizeOfExpr{Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName)}
		}
	case
		'(',
		rune(CHARCONST),
		rune(FLOATCONST),
		rune(GENERIC),
		rune(IDENTIFIER),
		rune(INTCONST),
		rune(LONGCHARCONST),
		rune(LONGSTRINGLITERAL),
		rune(STRINGLITERAL):

		return p.postfixExpression(Token{}, nil, Token{}, checkTypeName)
	case rune(INC):
		return &PrefixExpr{Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName), Dec: false}
	case rune(DEC):
		return &PrefixExpr{Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName), Dec: true}
	case rune(ALIGNOF):
		switch p.peek(1, false).Ch {
		case '(':
			switch {
			case p.isSpecifierQualifer(p.peek(2, true).Ch, true):
				return &AlignOfTypeExpr{Token: p.shift(false), LeftParen: p.shift(false), TypeName: p.typeName(), RightParen: p.must(')')}
			default:
				return &AlignOfExpr{Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName)}
			}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected unary expression or (type name)", t.Position(), runeName(t.Ch))
			return nil
		}
	case rune(ANDAND):
		return &LabelAddrExpr{Token: p.shift(false), Label: p.must(rune(IDENTIFIER))}
	case rune(REAL):
		return &UnaryExpr{Case: UnaryExpressionReal, Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName)}
	case rune(IMAG):
		return &UnaryExpr{Case: UnaryExpressionImag, Token: p.shift(false), Expr: p.unaryExpression(Token{}, nil, Token{}, checkTypeName)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected unary expression", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.5.2 Postfix operators
//
//  postfix-expression:
// 	primary-expression
// 	postfix-expression [ expression ]
// 	postfix-expression ( argument-expression-list_opt )
// 	postfix-expression . identifier
// 	postfix-expression -> identifier
// 	postfix-expression ++
// 	postfix-expression --
// 	( type-name ) { initializer-list }
// 	( type-name ) { initializer-list , }
// 	__builtin_types_compatible_p ( type-name , type-name )
func (p *parser) postfixExpression(lp Token, tn *TypeName, rp Token, checkTypeName bool) (r Expression) {
	var r0 Expression
	switch {
	case tn != nil:
		cl := &CompositeLitExpr{LeftParen: lp, TypeName: tn, RightParen: rp, LeftBrace: p.must('{'), InitializerList: p.initializerList(nil)}
		r0 = cl
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case ',':
			cl.Comma = p.shift(false)
			fallthrough
		default:
			cl.RightBrace = p.must('}')
		}
	default:
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case '(':
			switch ch := p.peek(1, true).Ch; {
			case p.isExpression(ch) || ch == '{':
				r0 = p.primaryExpression(checkTypeName)
			case p.isSpecifierQualifer(ch, true):
				cl := &CompositeLitExpr{LeftParen: p.shift(false), TypeName: p.typeName(), RightParen: p.must(')'), LeftBrace: p.must('{'), InitializerList: p.initializerList(nil)}
				r0 = cl
				switch p.rune(false) {
				case eof:
					p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
					return nil
				case ',':
					cl.Comma = p.shift(false)
					fallthrough
				default:
					cl.RightBrace = p.must('}')
					return r0
				}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected postfix expression", t.Position(), runeName(t.Ch))
				return nil
			}
		default:
			r0 = p.primaryExpression(checkTypeName)
		}
	}
	r = r0
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case '[':
			r = &IndexExpr{Expr: r, LeftBrace: p.shift(false), Index: p.expression(false), RightBrace: p.must(']')}
		case '(':
			switch ch := p.peek(1, true).Ch; {
			case p.isExpression(ch) || ch == ')':
				r = &CallExpr{Func: r, LeftParen: p.shift(false), Arguments: p.argumentExpressionListOpt(true), RightParen: p.must(')')}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected postfix expression", t.Position(), runeName(t.Ch))
			}
		case '.':
			r = &SelectorExpr{Expr: r, Token: p.shift(false), Sel: p.must(rune(IDENTIFIER)), Ptr: false}
		case rune(ARROW):
			r = &SelectorExpr{Expr: r, Token: p.shift(false), Sel: p.must(rune(IDENTIFIER)), Ptr: true}
		case rune(INC):
			r = &PostfixExpr{Expr: r, Token: p.shift(false), Dec: false}
		case rune(DEC):
			r = &PostfixExpr{Expr: r, Token: p.shift(false), Dec: true}
		default:
			return r
		}
	}
}

//  argument-expression-list:
// 	assignment-expression
// 	argument-expression-list , assignment-expression
func (p *parser) argumentExpressionListOpt(checkTypeName bool) (r *ArgumentExpressionList) {
	p.rune(false)
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case ')':
		return nil
	}

	r = &ArgumentExpressionList{Expression: p.assignmentExpression(checkTypeName)}
	prev := r
	for p.rune(false) == ',' {
		ael := &ArgumentExpressionList{Token: p.shift(false), Expression: p.assignmentExpression(checkTypeName)}
		prev.ArgumentExpressionList = ael
		prev = ael
		p.rune(false)
	}
	p.rune(false)
	return r
}

// [0], 6.5.1 Primary expressions
//
//  primary-expression:
// 	identifier
// 	constant
// 	string-literal
// 	( expression )
// 	( compound-statement )
//	generic-selection
func (p *parser) primaryExpression(checkTypeName bool) (r *PrimaryExpression) {
	switch p.rune(checkTypeName) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(CHARCONST):
		r = &PrimaryExpression{Case: PrimaryExpressionChar, Token: p.shift(false)}
	case rune(FLOATCONST):
		r = &PrimaryExpression{Case: PrimaryExpressionFloat, Token: p.shift(false)}
	case rune(IDENTIFIER):
		r = &PrimaryExpression{Case: PrimaryExpressionIdent, Token: p.shift(false), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(INTCONST):
		r = &PrimaryExpression{Case: PrimaryExpressionInt, Token: p.shift(false)}
	case rune(LONGCHARCONST):
		r = &PrimaryExpression{Case: PrimaryExpressionLChar, Token: p.shift(false)}
	case rune(LONGSTRINGLITERAL):
		r = &PrimaryExpression{Case: PrimaryExpressionLString, Token: p.shift(false)}
	case rune(STRINGLITERAL):
		r = &PrimaryExpression{Case: PrimaryExpressionString, Token: p.shift(false)}
	case '(':
		switch p.peek(1, false).Ch {
		case '{':
			return &PrimaryExpression{Case: PrimaryExpressionStmt, Token: p.shift(false), CompoundStatement: p.compoundStatement(false, false, nil), Token2: p.must(')')}
		default:
			return &PrimaryExpression{Case: PrimaryExpressionExpr, Token: p.shift(false), ExpressionList: p.expression(false), Token2: p.must(')')}
		}
	case rune(GENERIC):
		return &PrimaryExpression{Case: PrimaryExpressionGeneric, GenericSelection: p.genericSelection()}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected primary expression", t.Position(), runeName(t.Ch))
		return nil
	}
	r.m = p.cpp.mmap[mmapKey{r.Token.s, r.Token.off}]
	return r
}

// generic-selection:
//	_Generic ( assignment-expression , generic-assoc-list )
func (p *parser) genericSelection() (r *GenericSelection) {
	return &GenericSelection{Token: p.must(rune(GENERIC)), Token2: p.must('('), Expression: p.assignmentExpression(true), Token3: p.must(','), GenericAssociationList: p.genericAssociationList(), Token4: p.must(')')}
}

// generic-assoc-list:
// 	generic-association
// 	generic-assoc-list , generic-association
func (p *parser) genericAssociationList() (r *GenericAssociationList) {
	r = &GenericAssociationList{GenericAssociation: p.genericAssociation()}
	prev := r
	for p.rune(false) == ',' {
		gal := &GenericAssociationList{Token: p.shift(false), GenericAssociation: p.genericAssociation()}
		prev.GenericAssociationList = gal
		prev = gal
	}
	return r
}

// generic-association:
//	type-name : assignment-expression
//	default : assignment-expression
func (p *parser) genericAssociation() (r *GenericAssociation) {
	switch {
	case p.rune(false) == rune(DEFAULT):
		return &GenericAssociation{Case: GenericAssociationDefault, Token: p.shift(false), Token2: p.must(':'), Expression: p.assignmentExpression(true)}
	default:
		return &GenericAssociation{Case: GenericAssociationType, TypeName: p.typeName(), Token: p.must(':'), Expression: p.assignmentExpression(true)}
	}
}

// [0], 6.7.5 Declarators
//
//  declarator:
// 	pointer_opt direct-declarator
func (p *parser) declarator(ptr *Pointer, ds DeclarationSpecifiers, declare bool) (r *Declarator) {
	if ptr == nil {
		ptr = p.pointer(true)
	}
	r = &Declarator{Pointer: ptr, DirectDeclarator: p.directDeclarator(declare), lexicalScope: (*lexicalScope)(p.scope), isTypename: ds.IsTypedef()}
	if declare {
		r.visible = visible(p.seq) // [0]6.2.1,7
		p.scope.declare(p.cpp.eh, r.Name(), r)
	}
	return r
}

//  direct-declarator:
// 	identifier
// 	( declarator )
// 	direct-declarator [ type-qualifier-list_opt assignment-expression_opt ]
// 	direct-declarator [ static type-qualifier-list_opt assignment-expression ]
// 	direct-declarator [ type-qualifier-list static assignment-expression ]
// 	direct-declarator [ type-qualifier-list_opt * ]
// 	direct-declarator ( parameter-type-list_opt )
// 	direct-declarator ( identifier-list_opt )
func (p *parser) directDeclarator(declare bool) (r *DirectDeclarator) {
	switch p.rune(false) {
	case rune(IDENTIFIER):
		r = &DirectDeclarator{Case: DirectDeclaratorIdent, Token: p.shift(false)}
	case '(':
		r = &DirectDeclarator{Case: DirectDeclaratorDecl, Token: p.shift(false)}
		p.attributeSpecifierListOpt() //TODO not stored yet
		r.Declarator = p.declarator(nil, nil, false)
		r.Token2 = p.must(')')
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected direct declarator", t.Position(), runeName(t.Ch))
	}
	return p.directDeclarator2(r, declare)
}

func (p *parser) directDeclarator2(dd *DirectDeclarator, declare bool) (r *DirectDeclarator) {
	r = dd
	for {
		switch p.rune(false) {
		case '(':
			p.newScope()

			func() {
				defer p.closeScope()

				switch p.peek(1, true).Ch {
				case rune(IDENTIFIER):
					r = &DirectDeclarator{Case: DirectDeclaratorFuncIdent, DirectDeclarator: r, Token: p.shift(false), IdentifierList: p.identifierList(true), Token2: p.must(')')}
				default:
					r = &DirectDeclarator{Case: DirectDeclaratorFuncParam, DirectDeclarator: r, Token: p.shift(false), ParameterTypeList: p.parameterTypeListOpt(), Token2: p.must(')')}
				}
				r.params = p.scope
			}()
		case '[':
			ch := p.peek(1, true).Ch
			switch {
			case p.isExpression(ch):
				r = &DirectDeclarator{Case: DirectDeclaratorArr, DirectDeclarator: r, Token: p.shift(false), Expression: p.assignmentExpression(true), Token2: p.must(']')}
				continue
			case p.isTypeQualifier(ch):
				lbracket := p.shift(false)
				tql := p.typeQualifierList(false, false)
				switch p.rune(false) {
				case rune(STATIC):
					r = &DirectDeclarator{Case: DirectDeclaratorArrStatic, DirectDeclarator: r, Token: lbracket, TypeQualifiers: tql, Token2: p.shift(false), Expression: p.assignmentExpression(true), Token3: p.must(']')}
				case '*':
					r = &DirectDeclarator{Case: DirectDeclaratorStar, DirectDeclarator: r, Token: lbracket, TypeQualifiers: tql, Token2: p.shift(false), Token3: p.must(']')}
				case ']':
					r = &DirectDeclarator{Case: DirectDeclaratorArr, DirectDeclarator: r, Token: lbracket, TypeQualifiers: tql, Token3: p.must(']')}
				default:
					r = &DirectDeclarator{Case: DirectDeclaratorArr, DirectDeclarator: r, Token: lbracket, TypeQualifiers: tql, Expression: p.assignmentExpression(true), Token3: p.must(']')}
				}
				continue
			}

			switch ch {
			case ']':
				r = &DirectDeclarator{Case: DirectDeclaratorArr, DirectDeclarator: r, Token: p.shift(false), Token2: p.shift(false)}
			case rune(STATIC):
				r = &DirectDeclarator{Case: DirectDeclaratorStaticArr, DirectDeclarator: r, Token: p.shift(false), Token2: p.shift(false), TypeQualifiers: p.typeQualifierList(true, false), Expression: p.assignmentExpression(true), Token3: p.must(']')}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected direct declarator", t.Position(), runeName(t.Ch))
			}
		default:
			return r
		}
	}
	return r
}

//  identifier-list:
// 	identifier
// 	identifier-list , identifier
func (p *parser) identifierList(declare bool) (r *IdentifierList) {
	switch p.rune(true) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(IDENTIFIER):
		r = &IdentifierList{Token2: p.shift(false)}
		if declare {
			param := &Parameter{name: r.Token2, visible: visible(r.Token2.seq)}
			p.scope.declare(p.cpp.eh, r.Token2.SrcStr(), param)
			r.parameters = append(r.parameters, param)
		}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected identifier", t.Position(), runeName(t.Ch))
		return nil
	}

	r0 := r
	prev := r
	for p.rune(false) == ',' {
		il := &IdentifierList{Token: p.shift(false), Token2: p.must(rune(IDENTIFIER))}
		if declare {
			param := &Parameter{name: il.Token2, visible: visible(il.Token2.seq)}
			p.scope.declare(p.cpp.eh, il.Token2.SrcStr(), param)
			r0.parameters = append(r0.parameters, param)
		}
		prev.IdentifierList = il
		prev = il
	}
	return r
}

//  parameter-type-list:
// 	parameter-list
// 	parameter-list , ...
func (p *parser) parameterTypeListOpt() (r *ParameterTypeList) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case ')':
		return nil
	}

	pl := p.parameterList()
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case ',':
		return &ParameterTypeList{Case: ParameterTypeListVar, ParameterList: pl, Token: p.shift(false), Token2: p.must(rune(DDD))}
	default:
		return &ParameterTypeList{Case: ParameterTypeListList, ParameterList: pl}
	}
}

//  parameter-list:
// 	parameter-declaration
// 	parameter-list , parameter-declaration
func (p *parser) parameterList() (r *ParameterList) {
	r = &ParameterList{ParameterDeclaration: p.parameterDeclaration()}
	prev := r
	for p.rune(false) == ',' {
		if p.peek(1, false).Ch == rune(DDD) {
			return r
		}

		pl := &ParameterList{Token: p.shift(false), ParameterDeclaration: p.parameterDeclaration()}
		prev.ParameterList = pl
		prev = pl
	}
	return r
}

//  parameter-declaration:
// 	declaration-specifiers declarator
// 	declaration-specifiers abstract-declarator_opt
func (p *parser) parameterDeclaration() (r *ParameterDeclaration) {
	ds, ok := p.declarationSpecifiers()
	if !ok {
		return nil
	}

	switch p.rune(false) {
	case
		')',
		',':

		return &ParameterDeclaration{Case: ParameterDeclarationAbstract, DeclarationSpecifiers: ds}
	default:
		switch x := p.declaratorOrAbstractDeclarator(true).(type) {
		case *AbstractDeclarator:
			return &ParameterDeclaration{Case: ParameterDeclarationAbstract, DeclarationSpecifiers: ds, AbstractDeclarator: x}
		case *Declarator:
			return &ParameterDeclaration{Case: ParameterDeclarationDecl, DeclarationSpecifiers: ds, Declarator: x, AttributeSpecifiers: p.attributeSpecifierListOpt()}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected parameter declaration", t.Position(), runeName(t.Ch))
			return nil
		}
	}
}

func (p *parser) declaratorOrAbstractDeclarator(declare bool) (r Node) {
	// state 2
	ptr0 := p.pointer(true)
	switch p.rune(false) {
	case
		')',
		',':

		if ptr0 != nil {
			return &AbstractDeclarator{Case: AbstractDeclaratorPtr, Pointer: ptr0}
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
		return nil
	case '(':
		lparen := p.shift(false)
		p.attributeSpecifierListOpt() //TODO not stored yet
		if p.isSpecifierQualifer(p.rune(true), true) {
			dad := &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorFunc, Token: lparen, ParameterTypeList: p.parameterTypeListOpt(), Token2: p.must(')')}
			switch {
			case p.in(p.rune(false), ',', ')'):
				return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: dad}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		}

		switch p.rune(true) {
		case ')':
			dad := &DirectAbstractDeclarator{Case: DirectAbstractDeclaratorFunc, Token: lparen, Token2: p.shift(false)}
			return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: p.directAbstractDeclarator2(dad)}
		case '(':
			switch x := p.declaratorOrAbstractDeclarator(false).(type) {
			case *AbstractDeclarator:
				dad := &DirectAbstractDeclarator{
					Case:               DirectAbstractDeclaratorDecl,
					Token:              lparen,
					AbstractDeclarator: x,
					Token2:             p.must(')'),
				}
				return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: p.directAbstractDeclarator2(dad)}
			case *Declarator:
				dd := &DirectDeclarator{
					Case:       DirectDeclaratorDecl,
					Token:      lparen,
					Declarator: x,
					Token2:     p.must(')'),
				}
				r := &Declarator{Pointer: ptr0, DirectDeclarator: p.directDeclarator2(dd, declare)}
				if declare {
					r.visible = visible(p.seq) // [0]6.2.1,7
					p.scope.declare(p.cpp.eh, r.Name(), r)
				}
				return r
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		case rune(IDENTIFIER):
			dd := p.directDeclarator(declare)
			switch p.rune(false) {
			case ')':
				dd = &DirectDeclarator{
					Case:  DirectDeclaratorDecl,
					Token: lparen,
					Declarator: &Declarator{
						DirectDeclarator: dd,
					},
					Token2: p.must(')'),
				}
				r := &Declarator{Pointer: ptr0, DirectDeclarator: p.directDeclarator2(dd, declare)}
				if declare {
					r.visible = visible(p.seq) // [0]6.2.1,7
					p.scope.declare(p.cpp.eh, r.Name(), r)
				}
				return r
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		case '*':
			ptr := p.pointer(false)
			switch p.rune(false) {
			case
				')',
				'[':

				dad := &DirectAbstractDeclarator{
					Case:               DirectAbstractDeclaratorDecl,
					Token:              lparen,
					AbstractDeclarator: p.abstractDeclarator(ptr, false),
					Token2:             p.must(')'),
				}
				return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: p.directAbstractDeclarator2(dad)}
			case rune(IDENTIFIER):
				dd := p.directDeclarator(declare)
				switch p.rune(false) {
				case ')':
					dd = &DirectDeclarator{
						Case:  DirectDeclaratorDecl,
						Token: lparen,
						Declarator: &Declarator{
							Pointer:          ptr,
							DirectDeclarator: dd,
						},
						Token2: p.must(')'),
					}
					r := &Declarator{Pointer: ptr0, DirectDeclarator: p.directDeclarator2(dd, declare)}
					if declare {
						r.visible = visible(p.seq) // [0]6.2.1,7
						p.scope.declare(p.cpp.eh, r.Name(), r)
					}
					return r
				default:
					t := p.shift(false)
					p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
					return nil
				}
			case '(':
				switch x := p.declaratorOrAbstractDeclarator(false).(type) {
				case *Declarator:
					x.Pointer = ptr
					dd := &DirectDeclarator{Case: DirectDeclaratorDecl, Token: lparen, Declarator: x, Token2: p.must(')')}
					r := &Declarator{Pointer: ptr0, DirectDeclarator: dd}
					if declare {
						r.visible = visible(p.seq) // [0]6.2.1,7
						p.scope.declare(p.cpp.eh, r.Name(), r)
					}
					return r
				default:
					t := p.shift(false)
					p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
					return nil
				}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		case '^':
			ptr := p.pointer(false)
			switch p.rune(false) {
			case ')':
				dad := &DirectAbstractDeclarator{
					Case:               DirectAbstractDeclaratorDecl,
					Token:              lparen,
					AbstractDeclarator: p.abstractDeclarator(ptr, false),
					Token2:             p.must(')'),
				}
				return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: p.directAbstractDeclarator2(dad)}
			case rune(IDENTIFIER):
				dd := p.directDeclarator(declare)
				switch p.rune(false) {
				case ')':
					dd = &DirectDeclarator{
						Case:  DirectDeclaratorDecl,
						Token: lparen,
						Declarator: &Declarator{
							Pointer:          ptr,
							DirectDeclarator: dd,
						},
						Token2: p.must(')'),
					}
					r := &Declarator{Pointer: ptr0, DirectDeclarator: p.directDeclarator2(dd, declare)}
					if declare {
						r.visible = visible(p.seq) // [0]6.2.1,7
						p.scope.declare(p.cpp.eh, r.Name(), r)
					}
					return r
				default:
					t := p.shift(false)
					p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
					return nil
				}
			default:
				t := p.shift(false)
				p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
				return nil
			}
		case '[':
			dad := &DirectAbstractDeclarator{
				Case:               DirectAbstractDeclaratorDecl,
				Token:              lparen,
				AbstractDeclarator: p.abstractDeclarator(nil, false),
				Token2:             p.must(')'),
			}
			return &AbstractDeclarator{Case: AbstractDeclaratorDecl, Pointer: ptr0, DirectAbstractDeclarator: p.directAbstractDeclarator2(dad)}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
			return nil
		}
	case '[':
		return p.abstractDeclarator(ptr0, false)
	case rune(IDENTIFIER):
		return p.declarator(ptr0, nil, declare)
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected declarator or abstract declarator", t.Position(), runeName(t.Ch))
		return nil
	}
}

func (p *parser) in(ch rune, set ...rune) bool {
	for _, v := range set {
		if ch == v {
			return true
		}
	}
	return false
}

//  type-qualifier-list:
// 	type-qualifier
// 	type-qualifier-list type-qualifier
// 	type-qualifier-list
func (p *parser) typeQualifierList(opt, acceptAttributes bool) (r []*TypeQualifier) {
	switch ch := p.rune(true); {
	case p.isTypeQualifier(ch):
		r = append(r, p.typeQualifier(false))
	case p.in(ch, ':', ')', ',', '[', ']', rune(STATIC)) || p.isExpression(ch) || ch == rune(TYPENAME):
		if opt {
			return nil
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected type qualifier", t.Position(), runeName(t.Ch))
	case ch == rune(ATTRIBUTE):
		if acceptAttributes {
			r = append(r, p.typeQualifier(true))
			break
		}

		fallthrough
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected type qualifier", t.Position(), runeName(t.Ch))
	}
	for {
		switch ch := p.rune(false); {
		case p.in(ch, ':', ')', ',', '[', ']', rune(STATIC)) || p.isExpression(ch) || ch == rune(TYPENAME):
			return r
		case p.isTypeQualifier(ch):
			r = append(r, p.typeQualifier(acceptAttributes))
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected type qualifier", t.Position(), runeName(t.Ch))
		}
	}
}

func (p *parser) isTypeQualifier(ch rune) bool {
	switch ch {
	case
		rune(ATOMIC),
		rune(CONST),
		rune(NONNULL),
		rune(RESTRICT),
		rune(VOLATILE):

		return true
	}
	return false
}

//  pointer:
// 	* type-qualifier-list_opt
// 	* type-qualifier-list_opt pointer
//      ^ type-qualifier-list_opt
func (p *parser) pointer(opt bool) (r *Pointer) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '*':
		r = &Pointer{Case: PointerTypeQual, Token: p.shift(false), TypeQualifiers: p.typeQualifierList(true, true)}
	case '^':
		return &Pointer{Case: PointerBlock, Token: p.shift(false), TypeQualifiers: p.typeQualifierList(true, true)}
	default:
		if opt {
			return nil
		}

		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected type pointer", t.Position(), runeName(t.Ch))
	}
	prev := r
	for p.rune(false) == '*' {
		p := p.pointer(false)
		prev.Pointer = p
		prev.Case = PointerPtr
		prev = p
	}
	return r
}

//  declaration-specifiers:
// 	storage-class-specifier declaration-specifiers_opt
// 	type-specifier declaration-specifiers_opt
// 	type-qualifier declaration-specifiers_opt
// 	function-specifier declaration-specifiers_opt
//	alignment-specifier declaration-specifiers_opt
func (p *parser) declarationSpecifiers() (r DeclarationSpecifiers, ok bool) {
	acceptTypeName := true
	for {
		var ds DeclarationSpecifier
		switch ch := p.rune(false); {
		case ch == eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil, false
		case p.isTypeSpecifier(ch, false):
			ds = p.typeSpecifier()
			acceptTypeName = false
		case ch == rune(IDENTIFIER):
			if !acceptTypeName || !p.checkTypeName(&p.toks[0]) {
				return r, true
			}

			ds = p.typeSpecifier()
			acceptTypeName = false
		case ch == rune(ATOMIC):
			switch p.peek(1, false).Ch {
			case '(':
				ds = p.typeSpecifier()
				acceptTypeName = false
			default:
				ds = p.typeQualifier(false)
			}
		case p.isStorageClassSpecifier(ch):
			ds = p.storageClassSpecifier()
		case p.isTypeQualifier(ch):
			ds = p.typeQualifier(false)
		case p.in(ch, rune(INLINE), rune(NORETURN)):
			ds = p.functionSpecifier()
		case ch == rune(ATTRIBUTE):
			for _, a := range p.attributeSpecifierListOpt() {
				r = append(r, a)
			}
			continue
		case ch == rune(ALIGNAS):
			ds = p.alignmentSpecifier()
		default:
			return r, true
		}
		r = append(r, ds)
	}
}

func (p *parser) isDeclarationSpecifier(ch rune, typenameOk bool) bool {
	return p.isTypeSpecifier(ch, typenameOk) || p.isTypeQualifier(ch) || p.isStorageClassSpecifier(ch) ||
		ch == rune(ATTRIBUTE) || ch == rune(ALIGNAS) || p.isFunctionSpecifier(ch)
}

// [2], 6.7.5 Alignment specifier
//
// alignment-specifier:
// 	_Alignas ( type-name )
// 	_Alignas ( constant-expression )
func (p *parser) alignmentSpecifier() *AlignmentSpecifier {
	switch ch := p.peek(2, true).Ch; {
	case p.isTypeSpecifier(ch, true):
		return &AlignmentSpecifier{Case: AlignmentSpecifierType, Token: p.must(rune(ALIGNAS)), Token2: p.must('('), TypeName: p.typeName(), Token3: p.must(')')}
	case p.isExpression(ch):
		return &AlignmentSpecifier{Case: AlignmentSpecifierExpr, Token: p.must(rune(ALIGNAS)), Token2: p.must('('), Expression: p.constantExpression(), Token3: p.must(')')}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected alignment specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.7.4 Function specifiers
//
//  function-specifier:
// 	inline
// 	_Noreturn
func (p *parser) functionSpecifier() *FunctionSpecifier {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(INLINE):
		return &FunctionSpecifier{Case: FunctionSpecifierInline, Token: p.shift(false)}
	case rune(NORETURN):
		return &FunctionSpecifier{Case: FunctionSpecifierNoreturn, Token: p.shift(false)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected function specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

func (p *parser) isFunctionSpecifier(ch rune) bool { return ch == rune(INLINE) || ch == rune(NORETURN) }

// [0], 6.7.3 Type qualifiers
//
//  type-qualifier:
// 	const
// 	restrict
// 	volatile
// 	_Atomic
//	__attribute__
func (p *parser) typeQualifier(acceptAttributes bool) *TypeQualifier {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(CONST):
		return &TypeQualifier{Case: TypeQualifierConst, Token: p.shift(false)}
	case rune(RESTRICT):
		return &TypeQualifier{Case: TypeQualifierRestrict, Token: p.shift(false)}
	case rune(VOLATILE):
		return &TypeQualifier{Case: TypeQualifierVolatile, Token: p.shift(false)}
	case rune(ATOMIC):
		return &TypeQualifier{Case: TypeQualifierAtomic, Token: p.shift(false)}
	case rune(NONNULL):
		return &TypeQualifier{Case: TypeQualifierNonnull, Token: p.shift(false)}
	case rune(ATTRIBUTE):
		if acceptAttributes {
			return &TypeQualifier{Case: TypeQualifierAttr, AttributeSpecifiers: p.attributeSpecifierListOpt()}
		}

		fallthrough
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected type qualifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

// [0], 6.7.1 Storage-class specifiers
//
//  storage-class-specifier:
// 	typedef
// 	extern
// 	static
// 	auto
// 	register
//	__declspec ( ... )
func (p *parser) storageClassSpecifier() (r *StorageClassSpecifier) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(AUTO):
		return &StorageClassSpecifier{Case: StorageClassSpecifierAuto, Token: p.shift(false)}
	case rune(EXTERN):
		return &StorageClassSpecifier{Case: StorageClassSpecifierExtern, Token: p.shift(false)}
	case rune(REGISTER):
		return &StorageClassSpecifier{Case: StorageClassSpecifierRegister, Token: p.shift(false)}
	case rune(STATIC):
		return &StorageClassSpecifier{Case: StorageClassSpecifierStatic, Token: p.shift(false)}
	case rune(TYPEDEF):
		return &StorageClassSpecifier{Case: StorageClassSpecifierTypedef, Token: p.shift(false)}
	case rune(THREADLOCAL):
		return &StorageClassSpecifier{Case: StorageClassSpecifierThreadLocal, Token: p.shift(false)}
	case rune(DECLSPEC):
		r = &StorageClassSpecifier{Case: StorageClassSpecifierDeclspec, Token: p.shift(false), Token2: p.must('(')}
	out:
		for lvl := 0; ; {
			switch p.rune(false) {
			case '(':
				lvl++
			case ')':
				if lvl == 0 {
					break out
				}

				lvl--
			case eof:
				break out
			}
			r.Declspecs = append(r.Declspecs, p.shift(false))
		}
		r.Token3 = p.must(')')
		return r
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected storage class specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

func (p *parser) isStorageClassSpecifier(ch rune) bool {
	switch ch {
	case
		rune(AUTO),
		rune(DECLSPEC),
		rune(EXTERN),
		rune(REGISTER),
		rune(STATIC),
		rune(THREADLOCAL),
		rune(TYPEDEF):

		return true
	}
	return false
}

// [0], 6.7.2 Type specifiers
//
//  type-specifier:
//	atomic-type-specifier
// 	_Bool
// 	_Complex
// 	_Decimal128
// 	_Decimal32
// 	_Decimal64
// 	_Float128
// 	_Float16
// 	_Float32
// 	_Float64
// 	__float80
// 	__fp16
// 	__m256d
// 	char
// 	double
// 	enum-specifier
// 	float
// 	int
// 	long
// 	short
// 	signed
// 	struct-or-union-specifier
// 	typedef-name
// 	typeof ( expression )
// 	typeof ( type-name )
// 	unsigned
// 	void
func (p *parser) typeSpecifier() TypeSpecifier {
	switch p.rune(true) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(CHAR):
		return &TypeSpecName{Case: TypeSpecifierChar, Name: p.shift(false)}
	case rune(DOUBLE):
		return &TypeSpecName{Case: TypeSpecifierDouble, Name: p.shift(false)}
	case rune(ENUM):
		return &TypeSpecEnum{Enum: p.enumSpecifier(), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(FLOAT):
		return &TypeSpecName{Case: TypeSpecifierFloat, Name: p.shift(false)}
	case rune(INT):
		return &TypeSpecName{Case: TypeSpecifierInt, Name: p.shift(false)}
	case rune(LONG):
		return &TypeSpecName{Case: TypeSpecifierLong, Name: p.shift(false)}
	case rune(SIGNED):
		return &TypeSpecName{Case: TypeSpecifierSigned, Name: p.shift(false)}
	case rune(SHORT):
		return &TypeSpecName{Case: TypeSpecifierShort, Name: p.shift(false)}
	case rune(TYPENAME):
		return &TypeSpecName{Case: TypeSpecifierTypeName, Name: p.shift(true), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(UNSIGNED):
		return &TypeSpecName{Case: TypeSpecifierUnsigned, Name: p.shift(false)}
	case
		rune(STRUCT),
		rune(UNION):

		return &TypeSpecStructOrUnion{StructOrUnion: p.structOrUnionSpecifier()}
	case rune(VOID):
		return &TypeSpecName{Case: TypeSpecifierVoid, Name: p.shift(false)}
	case rune(BOOL):
		return &TypeSpecName{Case: TypeSpecifierBool, Name: p.shift(false)}
	case rune(COMPLEX):
		return &TypeSpecName{Case: TypeSpecifierComplex, Name: p.shift(false)}
	case rune(IMAGINARY):
		return &TypeSpecName{Case: TypeSpecifierImaginary, Name: p.shift(false)}
	case rune(FLOAT16):
		return &TypeSpecName{Case: TypeSpecifierFloat16, Name: p.shift(false)}
	case rune(FLOAT32):
		return &TypeSpecName{Case: TypeSpecifierFloat32, Name: p.shift(false)}
	case rune(FLOAT32X):
		return &TypeSpecName{Case: TypeSpecifierFloat32x, Name: p.shift(false)}
	case rune(FLOAT64):
		return &TypeSpecName{Case: TypeSpecifierFloat64, Name: p.shift(false)}
	case rune(FLOAT64X):
		return &TypeSpecName{Case: TypeSpecifierFloat64x, Name: p.shift(false)}
	case rune(FLOAT128):
		return &TypeSpecName{Case: TypeSpecifierFloat128, Name: p.shift(false)}
	case rune(FLOAT128X):
		return &TypeSpecName{Case: TypeSpecifierFloat128x, Name: p.shift(false)}
	case rune(ATOMIC):
		return &TypeSpecAtomic{Atomic: p.atomicTypeSpecifier()}
	case rune(UINT8):
		return &TypeSpecName{Case: TypeSpecifierUint8, Name: p.shift(false)}
	case rune(UINT16):
		return &TypeSpecName{Case: TypeSpecifierUint16, Name: p.shift(false)}
	case rune(UINT32):
		return &TypeSpecName{Case: TypeSpecifierUint32, Name: p.shift(false)}
	case rune(UINT64):
		return &TypeSpecName{Case: TypeSpecifierUint64, Name: p.shift(false)}
	case rune(UINT128):
		return &TypeSpecName{Case: TypeSpecifierUint128, Name: p.shift(false)}
	case rune(INT8):
		return &TypeSpecName{Case: TypeSpecifierInt8, Name: p.shift(false)}
	case rune(INT16):
		return &TypeSpecName{Case: TypeSpecifierInt16, Name: p.shift(false)}
	case rune(INT32):
		return &TypeSpecName{Case: TypeSpecifierInt32, Name: p.shift(false)}
	case rune(INT64):
		return &TypeSpecName{Case: TypeSpecifierInt64, Name: p.shift(false)}
	case rune(INT128):
		return &TypeSpecName{Case: TypeSpecifierInt128, Name: p.shift(false)}
	case rune(DECIMAL32):
		return &TypeSpecName{Case: TypeSpecifierDecimal32, Name: p.shift(false)}
	case rune(DECIMAL64):
		return &TypeSpecName{Case: TypeSpecifierDecimal64, Name: p.shift(false)}
	case rune(DECIMAL128):
		return &TypeSpecName{Case: TypeSpecifierDecimal128, Name: p.shift(false)}
	case rune(TYPEOF):
		switch ch := p.peek(2, true).Ch; {
		case p.isTypeSpecifier(ch, true):
			return &TypeSpecTypeOfType{TypeOf: p.shift(false), LeftParen: p.must('('), Name: p.typeName(), RightParen: p.must(')')}
		case p.isExpression(ch):
			return &TypeSpecTypeOfExpr{TypeOf: p.shift(false), LeftParen: p.must('('), Expr: p.expression(false), RightParen: p.must(')')}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected type specifier", t.Position(), runeName(t.Ch))
			return nil
		}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected type specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

func (p *parser) isTypeSpecifier(ch rune, typenameOk bool) bool {
	switch ch {
	case
		rune(BOOL),
		rune(CHAR),
		rune(COMPLEX),
		rune(DECIMAL128),
		rune(DECIMAL32),
		rune(DECIMAL64),
		rune(DOUBLE),
		rune(ENUM),
		rune(FLOAT),
		rune(FLOAT128),
		rune(FLOAT128X),
		rune(FLOAT16),
		rune(FLOAT32),
		rune(FLOAT32X),
		rune(FLOAT64),
		rune(FLOAT64X),
		rune(IMAGINARY),
		rune(INT),
		rune(INT8),
		rune(INT16),
		rune(INT32),
		rune(INT64),
		rune(INT128),
		rune(LONG),
		rune(SHORT),
		rune(SIGNED),
		rune(STRUCT),
		rune(TYPEOF),
		rune(UINT8),
		rune(UINT16),
		rune(UINT32),
		rune(UINT64),
		rune(UINT128),
		rune(UNION),
		rune(UNSIGNED),
		rune(VOID):

		return true
	case rune(TYPENAME):
		return typenameOk
	}
	return false
}

// [2], 6.7.2.4 Atomic type specifiers
//
//  atomic-type-specifier:
// 	_Atomic ( type-name )
func (p *parser) atomicTypeSpecifier() (r *AtomicTypeSpecifier) {
	return &AtomicTypeSpecifier{Token: p.must(rune(ATOMIC)), Token2: p.must('('), TypeName: p.typeName(), Token3: p.must(')')}
}

// [0], 6.7.2.2 Enumeration specifiers
//
//  enum-specifier:
// 	enum identifier_opt { enumerator-list }
// 	enum identifier_opt { enumerator-list , }
// 	enum identifier
func (p *parser) enumSpecifier() (r *EnumSpecifier) {
	switch p.peek(1, false).Ch {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(IDENTIFIER):
		switch p.peek(2, true).Ch {
		case '{':
			r = &EnumSpecifier{Case: EnumSpecifierDef, Token: p.shift(false), Token2: p.shift(false), Token3: p.shift(false), EnumeratorList: p.enumeratorList(), lexicalScope: (*lexicalScope)(p.scope)}
			r.visible = visible(r.Token.seq + 1) // [0]6.2.1,7
			p.scope.declare(p.cpp.eh, r.Token2.SrcStr(), r)
		default:
			return &EnumSpecifier{Case: EnumSpecifierTag, Token: p.shift(false), Token2: p.shift(false), lexicalScope: (*lexicalScope)(p.scope)}
		}
	case '{':
		r = &EnumSpecifier{Case: EnumSpecifierDef, Token: p.shift(false), Token3: p.shift(false), EnumeratorList: p.enumeratorList(), lexicalScope: (*lexicalScope)(p.scope)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected enum specifier", t.Position(), runeName(t.Ch))
		return nil
	}
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case ',':
		r.Token4 = p.shift(false)
		r.Token5 = p.must('}')
		return r
	case '}':
		r.Token5 = p.shift(false)
		return r
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected enum specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

//  enumerator-list:
// 	enumerator
// 	enumerator-list , enumerator
func (p *parser) enumeratorList() (r *EnumeratorList) {
	r = &EnumeratorList{Enumerator: p.enumerator()}
	prev := r
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case '}':
			return r
		case ',':
			switch p.peek(1, true).Ch {
			case rune(IDENTIFIER):
				e := &EnumeratorList{Token: p.shift(false), Enumerator: p.enumerator()}
				prev.EnumeratorList = e
				prev = e
			default:
				return r
			}
		default:
			t := p.shift(false)
			p.cpp.eh("%v: unexpected %v, expected enumerator", t.Position(), runeName(t.Ch))
		}
	}
}

//  enumerator:
// 	enumeration-constant
// 	enumeration-constant = constant-expression
func (p *parser) enumerator() (r *Enumerator) {
	switch p.peek(1, false).Ch {
	case '}', ',':
		r = &Enumerator{Case: EnumeratorIdent, Token: p.must(rune(IDENTIFIER))}
	case '=':
		r = &Enumerator{Case: EnumeratorExpr, Token: p.must(rune(IDENTIFIER)), Token2: p.shift(false), Expression: p.constantExpression()}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected enumerator", t.Position(), runeName(t.Ch))
		return nil
	}
	r.visible = visible(r.Token.seq + 1)
	p.scope.declare(p.cpp.eh, r.Token.SrcStr(), r)
	return r
}

// [0], 6.7.2.1 Structure and union specifiers
//
//  struct-or-union-specifier:
// 	struct-or-union identifier_opt { struct-declaration-list }
// 	struct-or-union identifier
func (p *parser) structOrUnionSpecifier() (r *StructOrUnionSpecifier) {
	sou := p.structOrUnion()
	attrs := p.attributeSpecifierListOpt()
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case '{':
		return &StructOrUnionSpecifier{Case: StructOrUnionSpecifierDef, StructOrUnion: sou, AttributeSpecifiers: attrs, Token2: p.shift(false), StructDeclarationList: p.structDeclarationList(), Token3: p.must('}'), AttributeSpecifiers2: p.attributeSpecifierListOpt(), lexicalScope: (*lexicalScope)(p.scope)}
	case rune(IDENTIFIER):
		r = &StructOrUnionSpecifier{StructOrUnion: sou, AttributeSpecifiers: attrs, Token: p.shift(false), lexicalScope: (*lexicalScope)(p.scope)}
		r.visible = visible(r.Token.seq + 1) // [0]6.2.1,7
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case '{':
			p.scope.declare(p.cpp.eh, r.Token.SrcStr(), r)
			r.Case = StructOrUnionSpecifierDef
			r.Token2 = p.shift(false)
			r.StructDeclarationList = p.structDeclarationList()
			r.Token3 = p.must('}')
			r.AttributeSpecifiers = p.attributeSpecifierListOpt()
			return r
		default:
			r.Case = StructOrUnionSpecifierTag
			return r
		}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected struct or union specifier", t.Position(), runeName(t.Ch))
		return nil
	}
}

//  struct-declaration-list:
// 	struct-declaration
// 	struct-declaration-list struct-declaration
func (p *parser) structDeclarationList() (r *StructDeclarationList) {
	var prev *StructDeclarationList
	for {
		switch p.rune(false) {
		case eof:
			p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
			return nil
		case '}':
			return r
		default:
			sdl := &StructDeclarationList{StructDeclaration: p.structDeclaration()}
			switch {
			case r == nil:
				r = sdl
			default:
				prev.StructDeclarationList = sdl
			}
			prev = sdl
		}
	}
}

//  struct-declaration:
// 	specifier-qualifier-list struct-declarator-list_opt ;
//	static-assert-declaration ;
func (p *parser) structDeclaration() (r *StructDeclaration) {
	if p.rune(false) == rune(STATICASSERT) {
		r = &StructDeclaration{Case: StructDeclarationAssert, StaticAssertDeclaration: p.staticAssertDeclaration()}
		switch p.rune(false) {
		case ';':
			r.Token = p.shift(false)
		default:
			t := p.toks[0]
			p.cpp.eh("%v: unexpected %v, expected ';'", t.Position(), runeName(t.Ch))
		}
		return r
	}

	r = &StructDeclaration{Case: StructDeclarationDecl, SpecifierQualifiers: p.specifierQualifierList(), StructDeclaratorList: p.structDeclaratorListOpt(), AttributeSpecifiers: p.attributeSpecifierListOpt()}
	switch p.rune(false) {
	case ';':
		r.Token = p.shift(false)
		return r
	case '}':
		return r
	default:
		t := p.toks[0]
		p.cpp.eh("%v: unexpected %v, expected ';'", t.Position(), runeName(t.Ch))
		return r
	}
}

//  struct-declarator-list:
// 	struct-declarator
// 	struct-declarator-list , struct-declarator
func (p *parser) structDeclaratorListOpt() (r *StructDeclaratorList) {
	switch p.rune(false) {
	case
		'(',
		'*',
		':',
		rune(IDENTIFIER):

		// ok
	case ';':
		return nil
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected struct declarator list", t.Position(), runeName(t.Ch))
		return nil
	}

	r = &StructDeclaratorList{StructDeclarator: p.structDeclarator()}
	prev := r
	for p.rune(false) == ',' {
		sdl := &StructDeclaratorList{Token: p.shift(false), StructDeclarator: p.structDeclarator()}
		prev.StructDeclaratorList = sdl
		prev = sdl
	}
	return r
}

//  struct-declarator:
// 	declarator
// 	declarator_opt : constant-expression
func (p *parser) structDeclarator() (r *StructDeclarator) {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case ':':
		return &StructDeclarator{Case: StructDeclaratorBitField, Token: p.shift(false), Expression: p.constantExpression()}
	default:
		r = &StructDeclarator{Case: StructDeclaratorDecl, Declarator: p.declarator(nil, nil, false)}
		if p.rune(false) == ':' {
			r.Case = StructDeclaratorBitField
			r.Token = p.shift(false)
			r.Expression = p.constantExpression()
		}
		return r
	}
}

// [0], 6.6 Constant expressions
//
//  constant-expression:
// 	conditional-expression
func (p *parser) constantExpression() (r *ConstantExpression) {
	r = &ConstantExpression{}
	r.Expression, _ = p.conditionalExpression(true)
	return r
}

// struct-or-union:
//	struct
//	union
func (p *parser) structOrUnion() *StructOrUnion {
	switch p.rune(false) {
	case eof:
		p.cpp.eh("%v: unexpected EOF", p.toks[0].Position())
		return nil
	case rune(STRUCT):
		return &StructOrUnion{Case: StructOrUnionStruct, Token: p.shift(false)}
	case rune(UNION):
		return &StructOrUnion{Case: StructOrUnionUnion, Token: p.shift(false)}
	default:
		t := p.shift(false)
		p.cpp.eh("%v: unexpected %v, expected 'struct' or 'union'", t.Position(), runeName(t.Ch))
		return nil
	}
}

// Scope binds names to declaring nodes.
//
// The dynamic type of a Node in the Nodes map is one of
//
//  Node type                   Note
//  -----------------           ----
//  *Declarator
//  *EnumSpecifier              binds the tag
//  *Enumerator
//  *LabelDeclaration
//  *LabeledStatement           case LabeledStatementLabel
//  *Parameter
//  *StructOrUnionSpecifier     binds the tag
type Scope struct {
	Children []*Scope
	Nodes    map[string][]Node
	Parent   *Scope
}

func newScope(parent *Scope) (r *Scope) {
	r = &Scope{Parent: parent}
	if parent != nil {
		parent.Children = append(parent.Children, r)
	}
	return r
}

func (s *Scope) declare(eh interface{}, nm string, n Node) {
	// trc("%v: %q@%p %T, visible %v (scope %p) '%s' (%v: %v: %v:)", n.Position(), nm, n, n, n.(interface{ Visible() int }).Visible(), s, NodeSource(n), origin(4), origin(3), origin(2))
	if s.Nodes != nil {
		s.Nodes[nm] = append(s.Nodes[nm], n)
		return
	}

	s.Nodes = map[string][]Node{nm: {n}}
}

func (s *Scope) ident(t Token) (r Node) {
	a := s.idents(t)
	switch len(a) {
	case 1:
		return a[0]
	case 0:
		return nil
	}

	if p, ok := a[0].(*Parameter); ok {
		return p.Declarator
	}

	d, ok := a[0].(*Declarator)
	if !ok {
		return a[0]
	}

	if d.Type().Kind() == Function {
		// trc("%v: %q@%p %s %v %v", d.Position(), d.Name(), d, d.Type(), d.isFuncDef, len(a))
		f := d.Type().(*FunctionType)
		for _, v := range a {
			x, ok := v.(*Declarator)
			if !ok {
				continue
			}

			// trc("%v: %q@%p %s %v", d.Position(), d.Name(), d, d.Type(), d.isFuncDef)
			if d.isFuncDef {
				return d
			}

			g, ok := x.Type().(*FunctionType)
			if !ok {
				continue
			}

			if lessFunctionType(f, g) {
				return v
			}
		}

		return a[0]
	}

	for _, v := range a {
		if x, ok := v.(*Declarator); ok && !x.Type().IsIncomplete() {
			return x
		}
	}

	return a[0]
}

func (s *Scope) idents(t Token) (r []Node) {
	nm := t.Src()
	for ; s != nil; s = s.Parent {
		for _, v := range s.Nodes[string(nm)] {
			switch x := v.(type) {
			case *Declarator:
				if t.seq >= int32(x.visible) {
					x.resolved = s
					r = append(r, x)
					switch {
					case x.isExtern:
						for s := s.Parent; s != nil; s = s.Parent {
							for _, v := range s.Nodes[string(nm)] {
								if x, ok := v.(*Declarator); ok && x.isExtern {
									x.resolved = s
									r = append(r, x)
								}
							}
						}
					case x.Type().Kind() == Function && !x.isFuncDef:
						for s := s.Parent; s != nil; s = s.Parent {
							for _, v := range s.Nodes[string(nm)] {
								if x, ok := v.(*Declarator); ok && x.Type().Kind() == Function {
									x.resolved = s
									r = append(r, x)
								}
							}
						}
					}
				}
			case *Enumerator:
				if t.seq >= int32(x.visible) {
					x.resolved = s
					r = append(r, x)
				}
			case *Parameter:
				if t.seq >= int32(x.visible) {
					x.resolved = s
					r = append(r, x)
				}
			}
		}
		if len(r) != 0 {
			return r
		}
	}
	return nil
}

func (s *Scope) builtin(t Token) *Declarator {
	nm := "__builtin_" + t.SrcStr()
	for ; s != nil; s = s.Parent {
		for _, v := range s.Nodes[nm] {
			switch x := v.(type) {
			case *Declarator:
				if t.seq >= int32(x.visible) {
					return x
				}
			}
		}
	}
	return nil
}

func (s *Scope) enum(t Token) *EnumSpecifier {
	for ; s != nil; s = s.Parent {
		for _, v := range s.Nodes[string(t.Src())] {
			switch x := v.(type) {
			case *EnumSpecifier:
				return x
			}
		}
	}
	return nil
}

func (s *Scope) structOrUnion(t Token) *StructOrUnionSpecifier {
	for ; s != nil; s = s.Parent {
		for _, v := range s.Nodes[string(t.Src())] {
			switch x := v.(type) {
			case *StructOrUnionSpecifier:
				return x
			}
		}
	}
	return nil
}

type visible int32

// Visible reports the token sequence number where the visibility of a name starts.
func (v visible) Visible() int { return int(v) }

type lexicalScope Scope

// LexicalScope provides the scope a node appears in.
func (n *lexicalScope) LexicalScope() *Scope { return (*Scope)(n) }
