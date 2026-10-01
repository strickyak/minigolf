package np

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ast"
)

// TypeKind specifies whether a variable/value is a scalar word (2 bytes)
// or a 3-word slice { Base, Cap, Len } (6 bytes).
type TypeKind int

const (
	KindScalar TypeKind = 2
	KindSlice  TypeKind = 6
	KindVoid   TypeKind = 0
)

func (k TypeKind) String() string {
	switch k {
	case KindSlice:
		return "slice"
	case KindVoid:
		return "void"
	default:
		return "scalar"
	}
}

type loopContext struct {
	contLabel string
	exitLabel string
}

// Generator generates NPCode assembly (.npasm) directly from a MiniGolf AST.
type Generator struct {
	program      *ast.Program
	out          strings.Builder
	labelCounter int

	// Global declarations across modules
	globals map[string]TypeKind
	funcs   map[string]*ast.FuncStatement
	consts  map[string]ast.Expression

	// Per-function state
	currentFunc *ast.FuncStatement
	funcPkg     string
	params      map[string]TypeKind
	paramOrder  []string
	locals      map[string]TypeKind
	localOrder  []string
	dicts       map[string]bool

	// Control flow stack for break/continue
	loopStack []loopContext
}

// New creates a new NPCode code generator.
func New() *Generator {
	return &Generator{
		globals: make(map[string]TypeKind),
		funcs:   make(map[string]*ast.FuncStatement),
		consts:  make(map[string]ast.Expression),
	}
}

// Generate translates a whole MiniGolf AST program into NP assembly source.
func (g *Generator) Generate(program *ast.Program) string {
	g.program = program
	g.out.Reset()

	// Pass 1: Collect globals, constants, and function prototypes
	g.collectDeclarations(program)

	// Emit global variable directives
	g.emitGlobals()

	// Pass 2: Emit each function
	currentPkg := ""
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			currentPkg = s.Name.Value
		case *ast.FuncStatement:
			// Skip uninstantiated generic templates
			if len(s.TypeParameters) > 0 {
				continue
			}
			// Skip prelude.golf declarations if compiling an application
			if s.GetToken() != nil && strings.HasSuffix(s.GetToken().Filename, "prelude.golf") {
				continue
			}
			g.generateFunc(s, currentPkg)
		}
	}

	return g.out.String()
}

func (g *Generator) collectDeclarations(program *ast.Program) {
	currentPkg := ""
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			currentPkg = s.Name.Value
		case *ast.ConstStatement:
			qname := s.Name.Value
			if currentPkg != "" && currentPkg != "main" {
				qname = currentPkg + "." + s.Name.Value
			}
			g.consts[qname] = s.Value
			g.consts[s.Name.Value] = s.Value
		case *ast.VarStatement:
			if s.GetToken() != nil && strings.HasSuffix(s.GetToken().Filename, "prelude.golf") {
				continue
			}
			qname := s.Name.Value
			kind := KindScalar
			if s.ValueType != nil {
				kind = g.getTypeKind(s.ValueType)
			} else if s.Value != nil {
				kind = g.inferType(s.Value)
			}
			g.globals[qname] = kind
		case *ast.FuncStatement:
			if s.GetToken() != nil && strings.HasSuffix(s.GetToken().Filename, "prelude.golf") {
				continue
			}
			qname := s.Name.Value
			if s.Receiver != nil {
				rType := g.exprToString(s.Receiver.Type)
				rType = strings.TrimPrefix(rType, "*")
				qname = rType + "_" + s.Name.Value
			}
			g.funcs[qname] = s
			if currentPkg != "" && currentPkg != "main" {
				g.funcs[currentPkg+"."+qname] = s
			}
		}
	}
}

func (g *Generator) emitGlobals() {
	if len(g.globals) == 0 {
		return
	}
	g.emitComment("Global Variable Table")
	var names []string
	for name := range g.globals {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		kind := g.globals[name]
		g.emit(".global %s: %s", name, kind.String())
	}
	g.emit("")
}

func (g *Generator) generateFunc(fs *ast.FuncStatement, pkg string) {
	fnName := fs.Name.Value
	if fs.Receiver != nil {
		rType := g.exprToString(fs.Receiver.Type)
		rType = strings.TrimPrefix(rType, "*")
		fnName = rType + "_" + fs.Name.Value
	}

	isEntry := fnName == "main" || strings.Contains(fs.Linkage, "entry")

	g.currentFunc = fs
	g.funcPkg = pkg
	g.params = make(map[string]TypeKind)
	g.paramOrder = nil
	g.locals = make(map[string]TypeKind)
	g.localOrder = nil
	g.dicts = make(map[string]bool)
	g.loopStack = nil

	// Register parameters
	if fs.Receiver != nil {
		rKind := g.getTypeKind(fs.Receiver.Type)
		g.params[fs.Receiver.Name.Value] = rKind
		g.paramOrder = append(g.paramOrder, fs.Receiver.Name.Value)
	}
	for _, param := range fs.Parameters {
		pKind := g.getTypeKind(param.Type)
		g.params[param.Name.Value] = pKind
		g.paramOrder = append(g.paramOrder, param.Name.Value)
	}

	// Pre-scan function body to discover and register all local variables
	if fs.Body != nil {
		g.collectLocals(fs.Body)
	}

	// Emit function header
	g.emitComment("====================================================================")
	g.emitComment("Function: %s", fs.Name.Value)
	g.emitComment("====================================================================")
	if isEntry {
		g.emit(".function %s entry", fnName)
	} else {
		g.emit(".function %s", fnName)
	}

	// Emit .param directives
	for _, pName := range g.paramOrder {
		kind := g.params[pName]
		g.emit("    .param %s: %s", pName, kind.String())
	}

	// Emit .local directives
	for _, lName := range g.localOrder {
		kind := g.locals[lName]
		g.emit("    .local %s: %s", lName, kind.String())
	}
	if len(g.paramOrder) > 0 || len(g.localOrder) > 0 {
		g.emit("")
	}

	// Emit function body statements
	if fs.Body != nil {
		g.compileBlock(fs.Body)
	}

	// Default return safeguard
	g.emit("    RET_VOID")
	g.emit(".endfunction\n")
}

func (g *Generator) addLocal(name string, kind TypeKind) {
	if name == "_" || name == "" {
		return
	}
	if _, exists := g.params[name]; exists {
		return
	}
	if _, exists := g.locals[name]; !exists {
		g.locals[name] = kind
		g.localOrder = append(g.localOrder, name)
	}
}

func (g *Generator) collectLocals(block *ast.BlockStatement) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		g.scanLocalsFromStatement(stmt)
	}
}

func (g *Generator) scanLocalsFromStatement(stmt ast.Statement) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.VarStatement:
		kind := KindScalar
		if s.ValueType != nil {
			kind = g.getTypeKind(s.ValueType)
		} else if s.Value != nil {
			kind = g.inferType(s.Value)
		}
		g.addLocal(s.Name.Value, kind)

	case *ast.AssignStatement:
		if s.Token.Literal == ":=" {
			for i, lhs := range s.Names {
				if id, ok := lhs.(*ast.Identifier); ok && id.Value != "_" {
					kind := KindScalar
					if i < len(s.Values) {
						if g.isDictExpr(s.Values[i]) {
							g.dicts[id.Value] = true
							kind = KindSlice
						} else {
							kind = g.inferType(s.Values[i])
						}
					} else if len(s.Values) == 1 {
						kind = g.inferCallReturnType(s.Values[0], i)
					}
					g.addLocal(id.Value, kind)
				}
			}
		}

	case *ast.IfStatement:
		g.collectLocals(s.Consequence)
		g.collectLocals(s.Alternative)

	case *ast.ForStatement:
		g.collectLocals(s.Body)

	case *ast.For3Statement:
		if s.Init != nil {
			g.scanLocalsFromStatement(s.Init)
		}
		g.collectLocals(s.Body)

	case *ast.ForRangeStatement:
		line := s.Token.Line
		g.addLocal(fmt.Sprintf("_rng_idx_%d", line), KindScalar)
		g.addLocal(fmt.Sprintf("_rng_len_%d", line), KindScalar)
		rngKind := g.inferType(s.RangeValue)
		g.addLocal(fmt.Sprintf("_rng_val_%d", line), rngKind)

		if s.IsDecl {
			if keyId, ok := s.Key.(*ast.Identifier); ok && keyId.Value != "_" {
				g.addLocal(keyId.Value, KindScalar)
			}
			if s.Value != nil {
				if valId, ok := s.Value.(*ast.Identifier); ok && valId.Value != "_" {
					elemKind := KindScalar
					if rngKind == KindSlice {
						// string element is byte (scalar), slice[T] element is T
					}
					g.addLocal(valId.Value, elemKind)
				}
			}
		}
		g.collectLocals(s.Body)

	case *ast.SwitchStatement:
		if s.Tag != nil {
			tagKind := g.inferType(s.Tag)
			g.addLocal(fmt.Sprintf("_sw_tag_%d", s.Token.Line), tagKind)
		}
		for _, clause := range s.Body {
			for _, st := range clause.Body {
				g.scanLocalsFromStatement(st)
			}
		}

	case *ast.BlockStatement:
		g.collectLocals(s)
	}
}

func (g *Generator) compileBlock(block *ast.BlockStatement) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		g.compileStatement(stmt)
	}
}

func (g *Generator) compileStatement(stmt ast.Statement) {
	if stmt == nil {
		return
	}

	g.emitComment(g.nodeToString(stmt))

	switch s := stmt.(type) {
	case *ast.AssignStatement:
		g.compileAssign(s)

	case *ast.OpAssignStatement:
		g.compileOpAssign(s)

	case *ast.IncDecStatement:
		g.compileIncDec(s)

	case *ast.IfStatement:
		g.compileIf(s)

	case *ast.ForStatement:
		g.compileFor(s)

	case *ast.For3Statement:
		g.compileFor3(s)

	case *ast.ForRangeStatement:
		g.compileForRange(s)

	case *ast.SwitchStatement:
		g.compileSwitch(s)

	case *ast.ReturnStatement:
		g.compileReturn(s)

	case *ast.BreakStatement:
		if len(g.loopStack) == 0 {
			panic(fmt.Sprintf("break statement outside of loop at line %d", s.Token.Line))
		}
		g.emit("    JUMP %s", g.loopStack[len(g.loopStack)-1].exitLabel)

	case *ast.ContinueStatement:
		if len(g.loopStack) == 0 {
			panic(fmt.Sprintf("continue statement outside of loop at line %d", s.Token.Line))
		}
		g.emit("    JUMP %s", g.loopStack[len(g.loopStack)-1].contLabel)

	case *ast.GotoStatement:
		g.emit("    JUMP .L_user_%s", s.Label)

	case *ast.LabelStatement:
		g.emit(".L_user_%s:", s.Label)

	case *ast.ExpressionStatement:
		// Skip calls to uncompiled init_ functions from skipped prelude
		if call, ok := s.Expression.(*ast.CallExpression); ok {
			if id, ok := call.Function.(*ast.Identifier); ok {
				if strings.HasPrefix(id.Value, "init_") {
					if _, exists := g.funcs[id.Value]; !exists {
						return
					}
				}
			}
		}

		g.compileExpression(s.Expression)
		// Discard unused expression result from stack
		if !g.isVoidCall(s.Expression) {
			kind := g.inferType(s.Expression)
			if kind == KindSlice {
				g.emit("    POP_SLICE")
			} else if kind == KindScalar {
				g.emit("    POP")
			}
		}

	case *ast.VarStatement:
		// Initialized var declaration: var x = val
		if s.Value != nil {
			g.compileExpression(s.Value)
			if _, isLocal := g.locals[s.Name.Value]; isLocal {
				g.emit("    STORE_LOCAL %s", s.Name.Value)
			} else {
				g.emit("    STORE_GLOBAL %s", s.Name.Value)
			}
		}

	case *ast.BlockStatement:
		g.compileBlock(s)

	case *ast.DeferStatement:
		panic(fmt.Sprintf("defer statement is not supported in MiniGolf-NP (line %d)", s.Token.Line))

	default:
		panic(fmt.Sprintf("unsupported statement type in MiniGolf-NP: %T at line %d", stmt, stmt.GetToken().Line))
	}
}

func (g *Generator) compileAssign(s *ast.AssignStatement) {
	// Single assignment: lhs = rhs or lhs := rhs
	if len(s.Names) == 1 && len(s.Values) == 1 {
		lhs := s.Names[0]
		rhs := s.Values[0]

		// Special case: smap.Insert or list.Append might be desugared into assignment
		g.compileExpression(rhs)
		g.storeTarget(lhs, rhs)
		return
	}

	// Multiple assignment from single call: a, b := f(...)
	if len(s.Names) > 1 && len(s.Values) == 1 {
		call := s.Values[0]
		g.compileExpression(call)
		// Pop/store in reverse order (top of stack is last returned value)
		for i := len(s.Names) - 1; i >= 0; i-- {
			g.storeTarget(s.Names[i], nil)
		}
		return
	}

	// Multiple assignment: a, b = c, d
	if len(s.Names) == len(s.Values) {
		// Evaluate all RHS values onto stack
		for _, rhs := range s.Values {
			g.compileExpression(rhs)
		}
		// Store in reverse order
		for i := len(s.Names) - 1; i >= 0; i-- {
			g.storeTarget(s.Names[i], s.Values[i])
		}
		return
	}

	panic(fmt.Sprintf("mismatched assignment counts (%d = %d) at line %d", len(s.Names), len(s.Values), s.Token.Line))
}

func (g *Generator) storeTarget(lhs ast.Expression, rhsContext ast.Expression) {
	switch target := lhs.(type) {
	case *ast.Identifier:
		if target.Value == "_" {
			kind := KindScalar
			if rhsContext != nil {
				kind = g.inferType(rhsContext)
			}
			if kind == KindSlice {
				g.emit("    POP_SLICE")
			} else {
				g.emit("    POP")
			}
			return
		}
		if _, isLocal := g.locals[target.Value]; isLocal {
			g.emit("    STORE_LOCAL %s", target.Value)
			return
		}
		if _, isParam := g.params[target.Value]; isParam {
			g.emit("    STORE_LOCAL %s", target.Value)
			return
		}
		if _, isGlobal := g.globals[target.Value]; isGlobal {
			g.emit("    STORE_GLOBAL %s", target.Value)
			return
		}
		// Fallback to local
		g.emit("    STORE_LOCAL %s", target.Value)

	case *ast.PrefixExpression:
		// *ptr = val
		if target.Operator == "*" {
			// Stack currently has `val`.
			// We need `ptr`, then `val`, then `STORE_FIELD 0`.
			// Evaluate ptr to temporary or swap:
			// In NPCode: obj_ptr is popped first, val is popped second in STORE_FIELD.
			// Stack must have: [obj_ptr, val].
			// If stack has [val], we can store val in temp, push ptr, push val, STORE_FIELD 0.
			tempName := g.allocTempLocal(KindScalar)
			g.emit("    STORE_LOCAL %s", tempName)
			g.compileExpression(target.Right)
			g.emit("    LOAD_LOCAL %s", tempName)
			g.emit("    STORE_FIELD 0")
			return
		}
		panic(fmt.Sprintf("invalid assignment target prefix operator %q", target.Operator))

	case *ast.IndexExpression:
		// target.Left[target.Indices[0]] = val
		// SLICE_SET_WORD expects stack: [slice, idx, val]
		tempVal := g.allocTempLocal(KindScalar)
		g.emit("    STORE_LOCAL %s", tempVal)
		g.compileExpression(target.Left)
		g.compileExpression(target.Indices[0])
		g.emit("    LOAD_LOCAL %s", tempVal)
		g.emit("    SLICE_SET_WORD")

	default:
		panic(fmt.Sprintf("unsupported assignment target type: %T", lhs))
	}
}

func (g *Generator) compileOpAssign(s *ast.OpAssignStatement) {
	if id, ok := s.Name.(*ast.Identifier); ok {
		g.compileIdentifier(id)
		g.compileExpression(s.Value)
		switch s.Operator {
		case "+":
			g.emit("    ADD")
		case "-":
			g.emit("    SUB")
		case "*":
			g.emit("    MUL")
		case "/":
			g.emit("    DIV")
		case "%":
			g.emit("    MOD")
		case "&":
			g.emit("    BIT_AND")
		case "|":
			g.emit("    BIT_OR")
		case "^":
			g.emit("    BIT_XOR")
		case "<<":
			g.emit("    SHL")
		case ">>":
			g.emit("    SHR")
		default:
			panic(fmt.Sprintf("unsupported op-assign operator %q", s.Operator))
		}
		g.storeTarget(id, nil)
		return
	}

	panic(fmt.Sprintf("unsupported target for op-assignment: %T", s.Name))
}

func (g *Generator) compileIncDec(s *ast.IncDecStatement) {
	if id, ok := s.Name.(*ast.Identifier); ok {
		g.compileIdentifier(id)
		g.emit("    PUSH_1")
		if s.Token.Literal == "++" {
			g.emit("    ADD")
		} else {
			g.emit("    SUB")
		}
		g.storeTarget(id, nil)
		return
	}
	panic(fmt.Sprintf("unsupported target for increment/decrement: %T", s.Name))
}

func (g *Generator) compileIf(s *ast.IfStatement) {
	g.compileExpression(s.Condition)

	if s.Alternative != nil {
		lblAlt := g.nextLabel("L_else")
		lblEnd := g.nextLabel("L_endif")
		g.emit("    JUMP_IF_FALSE %s", lblAlt)
		g.compileBlock(s.Consequence)
		g.emit("    JUMP %s", lblEnd)
		g.emit("%s:", lblAlt)
		g.compileBlock(s.Alternative)
		g.emit("%s:", lblEnd)
	} else {
		lblEnd := g.nextLabel("L_endif")
		g.emit("    JUMP_IF_FALSE %s", lblEnd)
		g.compileBlock(s.Consequence)
		g.emit("%s:", lblEnd)
	}
}

func (g *Generator) compileFor(s *ast.ForStatement) {
	lblTop := g.nextLabel("L_for_top")
	lblExit := g.nextLabel("L_for_exit")

	g.loopStack = append(g.loopStack, loopContext{contLabel: lblTop, exitLabel: lblExit})

	g.emit("%s:", lblTop)
	if s.Condition != nil {
		g.compileExpression(s.Condition)
		g.emit("    JUMP_IF_FALSE %s", lblExit)
	}
	g.compileBlock(s.Body)
	g.emit("    JUMP %s", lblTop)
	g.emit("%s:", lblExit)

	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

func (g *Generator) compileFor3(s *ast.For3Statement) {
	if s.Init != nil {
		g.compileStatement(s.Init)
	}

	lblTop := g.nextLabel("L_for3_top")
	lblCont := g.nextLabel("L_for3_cont")
	lblExit := g.nextLabel("L_for3_exit")

	g.loopStack = append(g.loopStack, loopContext{contLabel: lblCont, exitLabel: lblExit})

	g.emit("%s:", lblTop)
	if s.Condition != nil {
		g.compileExpression(s.Condition)
		g.emit("    JUMP_IF_FALSE %s", lblExit)
	}
	g.compileBlock(s.Body)
	g.emit("%s:", lblCont)
	if s.Increment != nil {
		g.compileStatement(s.Increment)
	}
	g.emit("    JUMP %s", lblTop)
	g.emit("%s:", lblExit)

	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

func (g *Generator) compileForRange(s *ast.ForRangeStatement) {
	line := s.Token.Line
	idxVar := fmt.Sprintf("_rng_idx_%d", line)
	lenVar := fmt.Sprintf("_rng_len_%d", line)
	valVar := fmt.Sprintf("_rng_val_%d", line)

	// Evaluate range target and store in valVar
	g.compileExpression(s.RangeValue)
	g.emit("    STORE_LOCAL %s", valVar)

	// lenVar = len(valVar)
	g.emit("    LOAD_LOCAL %s", valVar)
	g.emit("    SLICE_LEN")
	g.emit("    STORE_LOCAL %s", lenVar)

	// idxVar = 0
	g.emit("    PUSH_0")
	g.emit("    STORE_LOCAL %s", idxVar)

	lblTop := g.nextLabel("L_range_top")
	lblCont := g.nextLabel("L_range_cont")
	lblExit := g.nextLabel("L_range_exit")

	g.loopStack = append(g.loopStack, loopContext{contLabel: lblCont, exitLabel: lblExit})

	g.emit("%s:", lblTop)
	g.emit("    LOAD_LOCAL %s", idxVar)
	g.emit("    LOAD_LOCAL %s", lenVar)
	g.emit("    CMP_LT")
	g.emit("    JUMP_IF_FALSE %s", lblExit)

	// Bind key (index) if not blank
	if s.Key != nil {
		if id, ok := s.Key.(*ast.Identifier); ok && id.Value != "_" {
			g.emit("    LOAD_LOCAL %s", idxVar)
			g.emit("    STORE_LOCAL %s", id.Value)
		}
	}

	// Bind value (element) if present and not blank
	if s.Value != nil {
		if id, ok := s.Value.(*ast.Identifier); ok && id.Value != "_" {
			g.emit("    LOAD_LOCAL %s", valVar)
			g.emit("    LOAD_LOCAL %s", idxVar)
			if g.inferType(s.RangeValue) == KindSlice {
				g.emit("    SLICE_GET_BYTE")
			} else {
				g.emit("    SLICE_GET_WORD")
			}
			g.emit("    STORE_LOCAL %s", id.Value)
		}
	}

	g.compileBlock(s.Body)

	g.emit("%s:", lblCont)
	g.emit("    LOAD_LOCAL %s", idxVar)
	g.emit("    PUSH_1")
	g.emit("    ADD")
	g.emit("    STORE_LOCAL %s", idxVar)
	g.emit("    JUMP %s", lblTop)
	g.emit("%s:", lblExit)

	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

func (g *Generator) compileSwitch(s *ast.SwitchStatement) {
	lblExit := g.nextLabel("L_switch_exit")

	if s.Tag != nil {
		tagVar := fmt.Sprintf("_sw_tag_%d", s.Token.Line)
		g.compileExpression(s.Tag)
		g.emit("    STORE_LOCAL %s", tagVar)

		var defaultClause *ast.CaseClause

		for _, clause := range s.Body {
			if len(clause.Values) == 0 {
				defaultClause = clause
				continue
			}
			lblBody := g.nextLabel("L_case_body")
			lblNext := g.nextLabel("L_case_next")

			for _, val := range clause.Values {
				g.emit("    LOAD_LOCAL %s", tagVar)
				g.compileExpression(val)
				if g.inferType(s.Tag) == KindSlice || g.inferType(val) == KindSlice {
					g.emit("    STR_CMP")
					g.emit("    NOT")
				} else {
					g.emit("    CMP_EQ")
				}
				g.emit("    JUMP_IF_TRUE %s", lblBody)
			}
			g.emit("    JUMP %s", lblNext)

			g.emit("%s:", lblBody)
			for _, st := range clause.Body {
				g.compileStatement(st)
			}
			g.emit("    JUMP %s", lblExit)
			g.emit("%s:", lblNext)
		}

		if defaultClause != nil {
			for _, st := range defaultClause.Body {
				g.compileStatement(st)
			}
		}
	} else {
		// Boolean switch: switch { case cond1: ... }
		var defaultClause *ast.CaseClause
		for _, clause := range s.Body {
			if len(clause.Values) == 0 {
				defaultClause = clause
				continue
			}
			lblNext := g.nextLabel("L_case_next")
			g.compileExpression(clause.Values[0])
			g.emit("    JUMP_IF_FALSE %s", lblNext)
			for _, st := range clause.Body {
				g.compileStatement(st)
			}
			g.emit("    JUMP %s", lblExit)
			g.emit("%s:", lblNext)
		}
		if defaultClause != nil {
			for _, st := range defaultClause.Body {
				g.compileStatement(st)
			}
		}
	}

	g.emit("%s:", lblExit)
}

func (g *Generator) compileReturn(s *ast.ReturnStatement) {
	if len(s.ReturnValues) == 0 {
		g.emit("    RET_VOID")
		return
	}

	if len(s.ReturnValues) == 1 {
		rv := s.ReturnValues[0]
		g.compileExpression(rv)
		kind := g.inferType(rv)
		if kind == KindSlice {
			g.emit("    RET_SLICE")
		} else {
			g.emit("    RET")
		}
		return
	}

	// Multiple returns: push all values sequentially onto stack
	for _, rv := range s.ReturnValues {
		g.compileExpression(rv)
	}
	lastKind := g.inferType(s.ReturnValues[len(s.ReturnValues)-1])
	if lastKind == KindSlice {
		g.emit("    RET_SLICE")
	} else {
		g.emit("    RET")
	}
}

func (g *Generator) compileExpression(expr ast.Expression) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		val := e.Value
		if val == 0 {
			g.emit("    PUSH_0")
		} else if val == 1 {
			g.emit("    PUSH_1")
		} else if val == -1 {
			g.emit("    PUSH_NEG1")
		} else if val >= -128 && val <= 127 {
			g.emit("    PUSH_I8 %d", val)
		} else {
			g.emit("    PUSH_I16 %d", val)
		}

	case *ast.StringLiteral:
		g.emit("    PUSH_STR %s", strconv.Quote(e.Value))

	case *ast.NilLiteral:
		g.emit("    PUSH_NIL")

	case *ast.Identifier:
		g.compileIdentifier(e)

	case *ast.PrefixExpression:
		g.compilePrefix(e)

	case *ast.InfixExpression:
		g.compileInfix(e)

	case *ast.CallExpression:
		g.compileCall(e)

	case *ast.IndexExpression:
		if e.IsSlice || len(e.Indices) >= 2 {
			g.compileSubSlice(e)
		} else if len(e.Indices) == 1 {
			g.compileExpression(e.Left)
			g.compileExpression(e.Indices[0])
			if g.inferType(e.Left) == KindSlice {
				g.emit("    SLICE_GET_BYTE")
			} else {
				g.emit("    SLICE_GET_WORD")
			}
		}

	case *ast.SelectorExpression:
		// Access struct field
		g.compileExpression(e.Left)
		g.emit("    LOAD_FIELD 0")

	default:
		panic(fmt.Sprintf("unsupported expression type in MiniGolf-NP: %T at line %d", expr, expr.GetToken().Line))
	}
}

func (g *Generator) compileIdentifier(e *ast.Identifier) {
	name := e.Value
	if name == "true" {
		g.emit("    PUSH_TRUE")
		return
	}
	if name == "false" {
		g.emit("    PUSH_FALSE")
		return
	}
	if name == "nil" {
		g.emit("    PUSH_NIL")
		return
	}

	// Constant lookup
	if cVal, exists := g.consts[name]; exists {
		g.compileExpression(cVal)
		return
	}

	// Local or parameter lookup
	if _, isLocal := g.locals[name]; isLocal {
		g.emit("    LOAD_LOCAL %s", name)
		return
	}
	if _, isParam := g.params[name]; isParam {
		g.emit("    LOAD_LOCAL %s", name)
		return
	}

	// Global lookup
	if _, isGlobal := g.globals[name]; isGlobal {
		g.emit("    LOAD_GLOBAL %s", name)
		return
	}

	// Fallback to local
	g.emit("    LOAD_LOCAL %s", name)
}

func (g *Generator) compilePrefix(e *ast.PrefixExpression) {
	switch e.Operator {
	case "!":
		g.compileExpression(e.Right)
		g.emit("    NOT")
	case "-":
		g.compileExpression(e.Right)
		g.emit("    NEG")
	case "+":
		g.compileExpression(e.Right)
	case "^", "~":
		g.compileExpression(e.Right)
		g.emit("    BIT_NOT")
	case "*":
		// Pointer dereference *ptr
		g.compileExpression(e.Right)
		g.emit("    LOAD_FIELD 0")
	default:
		panic(fmt.Sprintf("unsupported prefix operator %q at line %d", e.Operator, e.Token.Line))
	}
}

func (g *Generator) compileInfix(e *ast.InfixExpression) {
	// Short-circuit logical operators
	if e.Operator == "&&" {
		lblFalse := g.nextLabel("L_and_false")
		lblEnd := g.nextLabel("L_and_end")
		g.compileExpression(e.Left)
		g.emit("    DUP")
		g.emit("    JUMP_IF_FALSE %s", lblFalse)
		g.emit("    POP")
		g.compileExpression(e.Right)
		g.emit("    JUMP %s", lblEnd)
		g.emit("%s:", lblFalse)
		g.emit("%s:", lblEnd)
		return
	}

	if e.Operator == "||" {
		lblTrue := g.nextLabel("L_or_true")
		lblEnd := g.nextLabel("L_or_end")
		g.compileExpression(e.Left)
		g.emit("    DUP")
		g.emit("    JUMP_IF_TRUE %s", lblTrue)
		g.emit("    POP")
		g.compileExpression(e.Right)
		g.emit("    JUMP %s", lblEnd)
		g.emit("%s:", lblTrue)
		g.emit("%s:", lblEnd)
		return
	}

	// String comparisons
	isStringComp := g.inferType(e.Left) == KindSlice || g.inferType(e.Right) == KindSlice
	if isStringComp && (e.Operator == "==" || e.Operator == "!=" || e.Operator == "<" || e.Operator == "<=" || e.Operator == ">" || e.Operator == ">=") {
		g.compileExpression(e.Left)
		g.compileExpression(e.Right)
		g.emit("    STR_CMP")
		switch e.Operator {
		case "==":
			g.emit("    NOT")
		case "!=":
			// STR_CMP returns 0 on match, non-zero on mismatch
		case "<":
			g.emit("    PUSH_0")
			g.emit("    CMP_LT")
		case "<=":
			g.emit("    PUSH_0")
			g.emit("    CMP_LE")
		case ">":
			g.emit("    PUSH_0")
			g.emit("    CMP_GT")
		case ">=":
			g.emit("    PUSH_0")
			g.emit("    CMP_GE")
		}
		return
	}

	// General arithmetic and comparisons
	g.compileExpression(e.Left)
	g.compileExpression(e.Right)

	switch e.Operator {
	case "+":
		g.emit("    ADD")
	case "-":
		g.emit("    SUB")
	case "*":
		g.emit("    MUL")
	case "/":
		g.emit("    DIV")
	case "%":
		g.emit("    MOD")
	case "&":
		g.emit("    BIT_AND")
	case "|":
		g.emit("    BIT_OR")
	case "^":
		g.emit("    BIT_XOR")
	case "<<":
		g.emit("    SHL")
	case ">>":
		g.emit("    SHR")
	case "==":
		g.emit("    CMP_EQ")
	case "!=":
		g.emit("    CMP_NE")
	case "<":
		g.emit("    CMP_LT")
	case "<=":
		g.emit("    CMP_LE")
	case ">":
		g.emit("    CMP_GT")
	case ">=":
		g.emit("    CMP_GE")
	default:
		panic(fmt.Sprintf("unsupported infix operator %q at line %d", e.Operator, e.Token.Line))
	}
}

func (g *Generator) compileSubSlice(e *ast.IndexExpression) {
	// e.Left[low:high]
	// Sub-slicing pops [end, start, slice]
	g.compileExpression(e.Left)

	if len(e.Indices) == 2 {
		// low and high explicitly given
		if e.Indices[0] != nil {
			g.compileExpression(e.Indices[0])
		} else {
			g.emit("    PUSH_0")
		}
		if e.Indices[1] != nil {
			g.compileExpression(e.Indices[1])
		} else {
			g.compileExpression(e.Left)
			g.emit("    SLICE_LEN")
		}
	} else if len(e.Indices) == 1 {
		// s[:high]
		g.emit("    PUSH_0")
		g.compileExpression(e.Indices[0])
	} else {
		// s[:]
		g.emit("    PUSH_0")
		g.compileExpression(e.Left)
		g.emit("    SLICE_LEN")
	}

	g.emit("    SLICE_SUB")
}

func (g *Generator) compileCall(call *ast.CallExpression) {
	// 1. Selector call: recv.Method(...)
	if sel, ok := call.Function.(*ast.SelectorExpression); ok {
		method := sel.Right.Value
		recv := sel.Left

		// Check if sel.Left is an imported package name (e.g. smap.New)
		if pkgIdent, isPkg := recv.(*ast.Identifier); isPkg && pkgIdent.Value == "smap" {
			if method == "New" {
				n := 8
				if len(call.Arguments) > 0 {
					n = g.evalIntConst(call.Arguments[0])
				}
				g.emit("    DICT_NEW %d", n)
				return
			}
		}

		// Builtin slice and map methods on receiver
		switch method {
		case "Lookup":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			g.emit("    DICT_GET")
			return

		case "Insert":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			g.compileExpression(call.Arguments[1])
			g.emit("    DICT_SET")
			if id, isId := recv.(*ast.Identifier); isId {
				g.storeTarget(id, nil)
			}
			return

		case "Has":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			g.emit("    DICT_HAS")
			return

		case "Keys":
			g.compileExpression(recv)
			g.emit("    DICT_KEYS")
			return

		case "Len":
			g.compileExpression(recv)
			if g.isDictExpr(recv) {
				g.emit("    DICT_LEN")
			} else {
				g.emit("    SLICE_LEN")
			}
			return

		case "Cap":
			g.compileExpression(recv)
			g.emit("    SLICE_CAP")
			return

		case "Append":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			g.emit("    LIST_APPEND")
			if id, isId := recv.(*ast.Identifier); isId {
				g.storeTarget(id, nil)
			}
			return

		case "Pop":
			g.compileExpression(recv)
			g.emit("    LIST_POP")
			return

		case "Chop":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			g.compileExpression(call.Arguments[1])
			g.emit("    SLICE_SUB")
			return

		default:
			// General method call: push receiver, push args, call Type_Method
			g.compileExpression(recv)
			for _, arg := range call.Arguments {
				g.compileExpression(arg)
			}
			rTypeName := g.inferTypeName(recv)
			g.emit("    CALL %s_%s", rTypeName, method)
			return
		}
	}

	// 2. Generic instantiation call: e.g. smap.New[string](8)
	if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
		if sel, ok := idxExpr.Left.(*ast.SelectorExpression); ok && sel.Right.Value == "New" {
			n := 8
			if len(call.Arguments) > 0 {
				n = g.evalIntConst(call.Arguments[0])
			}
			g.emit("    DICT_NEW %d", n)
			return
		}
		if id, ok := idxExpr.Left.(*ast.Identifier); ok && id.Value == "New" {
			n := 8
			if len(call.Arguments) > 0 {
				n = g.evalIntConst(call.Arguments[0])
			}
			g.emit("    DICT_NEW %d", n)
			return
		}
	}

	// 3. Identifier function call
	id, ok := call.Function.(*ast.Identifier)
	if !ok {
		panic(fmt.Sprintf("unsupported callee expression: %T at line %d", call.Function, call.Token.Line))
	}

	fnName := id.Value

	// Builtin function dispatch
	switch fnName {
	case "len":
		g.compileExpression(call.Arguments[0])
		g.emit("    SLICE_LEN")
		return

	case "cap":
		g.compileExpression(call.Arguments[0])
		g.emit("    SLICE_CAP")
		return

	case "print", "println":
		for _, arg := range call.Arguments {
			g.compileExpression(arg)
			g.emit("    IO_PRINT")
		}
		return

	case "alloc":
		g.compileExpression(call.Arguments[0])
		g.emit("    BUF_ALLOC")
		return

	case "free":
		g.compileExpression(call.Arguments[0])
		g.emit("    BUF_FREE")
		return

	case "peek", "peekb", "peek_byte":
		g.compileExpression(call.Arguments[0])
		g.emit("    LOAD_FIELD 0")
		return

	case "poke", "pokeb", "poke_byte":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STORE_FIELD 0")
		return

	case "sys_exit", "exit":
		g.compileExpression(call.Arguments[0])
		g.emit("    SYS_EXIT")
		return

	case "rstrip":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_RSTRIP")
		return

	case "lstrip":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_LSTRIP")
		return

	case "strip":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_STRIP")
		return

	case "find":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.compileExpression(call.Arguments[2])
		g.emit("    STR_FIND")
		return

	case "startswith":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_STARTSWITH")
		return

	case "endswith":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_ENDSWITH")
		return

	case "replace_ident":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.compileExpression(call.Arguments[2])
		g.emit("    STR_REPLACE_IDENT")
		return

	case "splitlines":
		g.compileExpression(call.Arguments[0])
		g.emit("    STR_SPLITLINES")
		return

	case "strcmp":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_CMP")
		return

	case "streq":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    STR_CMP")
		g.emit("    NOT")
		return

	case "file_open_read":
		g.compileExpression(call.Arguments[0])
		g.emit("    FILE_OPEN_READ")
		return

	case "file_open_write":
		g.compileExpression(call.Arguments[0])
		g.emit("    FILE_OPEN_WRITE")
		return

	case "file_readline":
		g.compileExpression(call.Arguments[0])
		g.emit("    FILE_READLINE")
		return

	case "file_write":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    FILE_WRITE")
		return

	case "file_close":
		g.compileExpression(call.Arguments[0])
		g.emit("    FILE_CLOSE")
		return

	case "os_isfile":
		g.compileExpression(call.Arguments[0])
		g.emit("    OS_ISFILE")
		return

	case "os_makedirs":
		g.compileExpression(call.Arguments[0])
		g.emit("    OS_MAKEDIRS")
		return

	case "sys_args":
		g.emit("    SYS_ARGS")
		return

	case "make", "makeslice", "makelist":
		n := 8
		if len(call.Arguments) > 1 {
			n = g.evalIntConst(call.Arguments[1])
		}
		g.emit("    LIST_NEW %d", n)
		return

	case "append":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    LIST_APPEND")
		return

	case "byte":
		g.compileExpression(call.Arguments[0])
		g.emit("    PUSH_I16 255")
		g.emit("    BIT_AND")
		return

	case "word", "int", "uint", "int16", "uint16":
		g.compileExpression(call.Arguments[0])
		return
	}

	// General user function call
	for _, arg := range call.Arguments {
		g.compileExpression(arg)
	}

	targetName := fnName
	if id.Package != "" && id.Package != "main" && id.Package != "prelude" {
		targetName = id.Package + "_" + id.ShortName
	}
	g.emit("    CALL %s", targetName)
}

func (g *Generator) isVoidCall(expr ast.Expression) bool {
	call, ok := expr.(*ast.CallExpression)
	if !ok {
		return false
	}
	if id, ok := call.Function.(*ast.Identifier); ok {
		switch id.Value {
		case "print", "println", "free", "poke", "pokeb", "poke_byte", "file_close", "sys_exit", "exit":
			return true
		}
	}
	if sel, ok := call.Function.(*ast.SelectorExpression); ok {
		switch sel.Right.Value {
		case "Insert", "Append":
			return true
		}
	}
	return false
}

func (g *Generator) evalIntConst(expr ast.Expression) int {
	if expr == nil {
		return 0
	}
	if il, ok := expr.(*ast.IntegerLiteral); ok {
		return int(il.Value)
	}
	if id, ok := expr.(*ast.Identifier); ok {
		if cVal, exists := g.consts[id.Value]; exists {
			return g.evalIntConst(cVal)
		}
	}
	return 0
}

func (g *Generator) allocTempLocal(kind TypeKind) string {
	g.labelCounter++
	name := fmt.Sprintf("_tmp_%d", g.labelCounter)
	g.addLocal(name, kind)
	return name
}

func (g *Generator) nextLabel(prefix string) string {
	g.labelCounter++
	return fmt.Sprintf(".%s_%d", prefix, g.labelCounter)
}

func (g *Generator) emit(format string, args ...interface{}) {
	if len(args) == 0 {
		g.out.WriteString(format + "\n")
	} else {
		g.out.WriteString(fmt.Sprintf(format, args...) + "\n")
	}
}

func (g *Generator) emitComment(format string, args ...interface{}) {
	g.emit("; " + fmt.Sprintf(format, args...))
}

func (g *Generator) getTypeKind(typ ast.Expression) TypeKind {
	if typ == nil {
		return KindScalar
	}
	switch t := typ.(type) {
	case *ast.Identifier:
		name := t.Value
		if name == "string" {
			return KindSlice
		}
		if name == "noreturn" || name == "void" {
			return KindVoid
		}
		return KindScalar
	case *ast.ArrayType:
		return KindSlice
	case *ast.PointerType:
		return KindScalar
	case *ast.IndexExpression:
		// slice[T] or Smap[T]
		return KindSlice
	default:
		return KindScalar
	}
}

func (g *Generator) inferType(expr ast.Expression) TypeKind {
	if expr == nil {
		return KindVoid
	}
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return KindSlice
	case *ast.IntegerLiteral:
		return KindScalar
	case *ast.NilLiteral:
		return KindScalar
	case *ast.Identifier:
		if k, ok := g.locals[e.Value]; ok {
			return k
		}
		if k, ok := g.params[e.Value]; ok {
			return k
		}
		if k, ok := g.globals[e.Value]; ok {
			return k
		}
		if e.Value == "true" || e.Value == "false" {
			return KindScalar
		}
		return KindScalar
	case *ast.PrefixExpression:
		return KindScalar
	case *ast.InfixExpression:
		if e.Operator == "==" || e.Operator == "!=" || e.Operator == "<" || e.Operator == "<=" || e.Operator == ">" || e.Operator == ">=" || e.Operator == "&&" || e.Operator == "||" {
			return KindScalar
		}
		if e.Operator == "+" && (g.inferType(e.Left) == KindSlice || g.inferType(e.Right) == KindSlice) {
			return KindSlice
		}
		return KindScalar
	case *ast.IndexExpression:
		if e.IsSlice || len(e.Indices) >= 2 {
			return KindSlice
		}
		return KindScalar
	case *ast.CallExpression:
		return g.inferCallReturnType(e, 0)
	default:
		return KindScalar
	}
}

func (g *Generator) inferCallReturnType(expr ast.Expression, retIdx int) TypeKind {
	call, ok := expr.(*ast.CallExpression)
	if !ok {
		return KindScalar
	}

	if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
		if sel, ok := idxExpr.Left.(*ast.SelectorExpression); ok && sel.Right.Value == "New" {
			return KindSlice
		}
		if id, ok := idxExpr.Left.(*ast.Identifier); ok && id.Value == "New" {
			return KindSlice
		}
	}

	if id, ok := call.Function.(*ast.Identifier); ok {
		switch id.Value {
		case "rstrip", "lstrip", "strip", "replace_ident", "splitlines", "file_readline", "sys_args", "make", "makeslice":
			return KindSlice
		case "len", "cap", "find", "startswith", "endswith", "strcmp", "streq", "file_open_read", "file_open_write", "file_write", "file_close", "os_isfile", "os_makedirs", "alloc", "free", "peek", "poke", "byte", "word", "int", "uint", "bool":
			return KindScalar
		}

		if fs, exists := g.funcs[id.Value]; exists {
			if retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
		}
	}

	if sel, ok := call.Function.(*ast.SelectorExpression); ok {
		switch sel.Right.Value {
		case "New":
			return KindSlice
		case "Keys", "Chop":
			return KindSlice
		case "Len", "Cap", "Pop", "Has":
			return KindScalar
		case "Lookup":
			return KindScalar
		case "Insert":
			return KindScalar
		}
	}

	return KindScalar
}

func (g *Generator) isDictExpr(expr ast.Expression) bool {
	if expr == nil {
		return false
	}
	if call, ok := expr.(*ast.CallExpression); ok {
		if sel, ok := call.Function.(*ast.SelectorExpression); ok {
			if pkgId, ok := sel.Left.(*ast.Identifier); ok && pkgId.Value == "smap" && sel.Right.Value == "New" {
				return true
			}
		}
		if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
			if sel, ok := idxExpr.Left.(*ast.SelectorExpression); ok && sel.Right.Value == "New" {
				return true
			}
			if id, ok := idxExpr.Left.(*ast.Identifier); ok && id.Value == "New" {
				return true
			}
		}
	}
	if id, ok := expr.(*ast.Identifier); ok {
		return g.dicts[id.Value]
	}
	return false
}

func (g *Generator) inferTypeName(expr ast.Expression) string {
	if expr == nil {
		return "object"
	}
	if id, ok := expr.(*ast.Identifier); ok {
		if k, ok := g.locals[id.Value]; ok {
			return k.String()
		}
		if k, ok := g.params[id.Value]; ok {
			return k.String()
		}
	}
	return "object"
}

func (g *Generator) exprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Value
	case *ast.PointerType:
		return "*" + g.exprToString(e.Elt)
	case *ast.ArrayType:
		return "[]" + g.exprToString(e.Elt)
	case *ast.IndexExpression:
		return g.exprToString(e.Left)
	default:
		return expr.TokenLiteral()
	}
}

func (g *Generator) nodeToString(node ast.Node) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *ast.Identifier:
		return n.Value
	case *ast.IntegerLiteral:
		return strconv.FormatInt(n.Value, 10)
	case *ast.StringLiteral:
		return strconv.Quote(n.Value)
	case *ast.NilLiteral:
		return "nil"
	case *ast.InfixExpression:
		return fmt.Sprintf("%s %s %s", g.nodeToString(n.Left), n.Operator, g.nodeToString(n.Right))
	case *ast.PrefixExpression:
		return fmt.Sprintf("%s%s", n.Operator, g.nodeToString(n.Right))
	case *ast.CallExpression:
		var args []string
		for _, a := range n.Arguments {
			args = append(args, g.nodeToString(a))
		}
		return fmt.Sprintf("%s(%s)", g.nodeToString(n.Function), strings.Join(args, ", "))
	case *ast.IndexExpression:
		return fmt.Sprintf("%s[...]", g.nodeToString(n.Left))
	case *ast.SelectorExpression:
		return fmt.Sprintf("%s.%s", g.nodeToString(n.Left), n.Right.Value)
	case *ast.AssignStatement:
		var names []string
		for _, name := range n.Names {
			names = append(names, g.nodeToString(name))
		}
		var vals []string
		for _, val := range n.Values {
			vals = append(vals, g.nodeToString(val))
		}
		return fmt.Sprintf("%s %s %s", strings.Join(names, ", "), n.Token.Literal, strings.Join(vals, ", "))
	case *ast.OpAssignStatement:
		return fmt.Sprintf("%s %s %s", g.nodeToString(n.Name), n.Token.Literal, g.nodeToString(n.Value))
	case *ast.IncDecStatement:
		return fmt.Sprintf("%s%s", g.nodeToString(n.Name), n.Token.Literal)
	case *ast.IfStatement:
		return fmt.Sprintf("if %s", g.nodeToString(n.Condition))
	case *ast.ForStatement:
		if n.Condition != nil {
			return fmt.Sprintf("for %s", g.nodeToString(n.Condition))
		}
		return "for"
	case *ast.For3Statement:
		return "for <init>; <cond>; <inc>"
	case *ast.ForRangeStatement:
		return "for range ..."
	case *ast.SwitchStatement:
		if n.Tag != nil {
			return fmt.Sprintf("switch %s", g.nodeToString(n.Tag))
		}
		return "switch"
	case *ast.ReturnStatement:
		var vals []string
		for _, v := range n.ReturnValues {
			vals = append(vals, g.nodeToString(v))
		}
		return fmt.Sprintf("return %s", strings.Join(vals, ", "))
	case *ast.BreakStatement:
		return "break"
	case *ast.ContinueStatement:
		return "continue"
	case *ast.GotoStatement:
		return fmt.Sprintf("goto %s", n.Label)
	case *ast.LabelStatement:
		return fmt.Sprintf("%s:", n.Label)
	case *ast.ExpressionStatement:
		return g.nodeToString(n.Expression)
	case *ast.VarStatement:
		return fmt.Sprintf("var %s", n.Name.Value)
	default:
		return n.TokenLiteral()
	}
}
