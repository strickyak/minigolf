package par4

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
	KindBuffer TypeKind = -1
)

func (k TypeKind) String() string {
	switch k {
	case KindSlice:
		return "slice"
	case KindVoid:
		return "void"
	case KindBuffer:
		return "buffer"
	default:
		return "scalar"
	}
}

type loopContext struct {
	contLabel string
	exitLabel string
}

type fieldInfo struct {
	name   string
	offset int
	size   int
	typ    ast.Expression
}

type structInfo struct {
	name   string
	fields []fieldInfo
	size   int
}

// Generator generates NPCode assembly (.npasm) directly from a MiniGolf AST.
type Generator struct {
	program      *ast.Program
	out          strings.Builder
	labelCounter int

	// Global declarations across modules
	globals         map[string]TypeKind
	globalSizes     map[string]int
	globalElemSizes map[string]int
	globalTypes     map[string]string
	funcs           map[string]*ast.FuncStatement
	consts          map[string]ast.Expression
	structs         map[string]structInfo

	// Per-function state
	currentFunc *ast.FuncStatement
	funcPkg     string
	params      map[string]TypeKind
	paramTypes  map[string]string
	paramSizes  map[string]int
	paramOrder  []string
	locals      map[string]TypeKind
	localTypes  map[string]string
	localSizes  map[string]int
	localOrder  []string
	dicts       map[string]bool

	// Control flow stack for break/continue
	loopStack []loopContext
}

// New creates a new NPCode code generator.
func New() *Generator {
	return &Generator{
		globals:         make(map[string]TypeKind),
		globalSizes:     make(map[string]int),
		globalElemSizes: make(map[string]int),
		globalTypes:     make(map[string]string),
		funcs:           make(map[string]*ast.FuncStatement),
		consts:          make(map[string]ast.Expression),
		structs:         make(map[string]structInfo),
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
			// Skip prelude declarations if compiling an application
			if s.GetToken() != nil && (strings.HasSuffix(s.GetToken().Filename, "prelude.golf") || strings.HasSuffix(s.GetToken().Filename, "prelude.par3") || strings.HasSuffix(s.GetToken().Filename, "prelude.par4")) {
				continue
			}
			g.generateFunc(s, currentPkg)
		}
	}

	return g.out.String()
}

func (g *Generator) collectDeclarations(program *ast.Program) {
	// Pre-pass: collect and resolve struct types
	for pass := 0; pass < 3; pass++ {
		currentPkg := ""
		for _, stmt := range program.Statements {
			if ps, ok := stmt.(*ast.PackageStatement); ok {
				currentPkg = ps.Name.Value
			}
			if s, ok := stmt.(*ast.TypeStatement); ok {
				if st, ok := s.BaseType.(*ast.StructType); ok {
					var info structInfo
					info.name = s.Name.Value
					currOffset := 0
					for _, f := range st.Fields {
						fSize := g.getTypeSize(f.Type)
						info.fields = append(info.fields, fieldInfo{
							name:   f.Name.Value,
							offset: currOffset,
							size:   fSize,
							typ:    f.Type,
						})
						currOffset += fSize
					}
					info.size = currOffset
					g.structs[info.name] = info
					if currentPkg != "" && currentPkg != "main" {
						g.structs[currentPkg+"."+info.name] = info
						g.structs[currentPkg+"_"+info.name] = info
					}
				}
			}
		}
	}

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
			size := 2
			tStr := "word"
			if s.ValueType != nil {
				tStr = g.exprToString(s.ValueType)
				kind = g.getTypeKind(s.ValueType)
				if arr, ok := s.ValueType.(*ast.ArrayType); ok && arr.Length != nil {
					elemSize := g.getTypeSize(arr.Elt)
					g.globalElemSizes[qname] = elemSize
					count := g.evalIntConst(arr.Length)
					if count > 0 {
						size = count * elemSize
						kind = KindBuffer
					} else if intLit, ok := arr.Length.(*ast.IntegerLiteral); ok {
						size = int(intLit.Value) * elemSize
						kind = KindBuffer
					}
				} else if kind == KindSlice {
					size = 6
				}
			} else if s.Value != nil {
				tStr = g.getArgTypeString(s.Value)
				kind = g.inferType(s.Value)
				if kind == KindSlice {
					size = 6
				}
			}
			g.globals[qname] = kind
			g.globalSizes[qname] = size
			g.globalTypes[qname] = tStr
		case *ast.FuncStatement:
			if s.GetToken() != nil && strings.HasSuffix(s.GetToken().Filename, "prelude.golf") {
				continue
			}
			qname := s.Name.Value
			if s.Receiver != nil {
				rType := g.exprToString(s.Receiver.Type)
				rType = strings.TrimPrefix(rType, "*")
				rType = strings.ReplaceAll(rType, ".", "_")
				if currentPkg != "" && currentPkg != "main" && !strings.HasPrefix(rType, currentPkg) {
					rType = currentPkg + "_" + rType
				}
				qname = rType + "_" + s.Name.Value
			} else if currentPkg != "" && currentPkg != "main" {
				qname = currentPkg + "_" + s.Name.Value
			}
			g.funcs[qname] = s
			if currentPkg != "" && currentPkg != "main" {
				g.funcs[currentPkg+"."+s.Name.Value] = s
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
		sz := g.globalSizes[name]
		if sz == 0 {
			sz = 2
		}
		if kind == KindBuffer || sz > 6 {
			g.emit(".global %s: %d", name, sz)
		} else if kind == KindSlice || sz == 6 {
			g.emit(".global %s: slice", name)
		} else if sz == 1 {
			g.emit(".global %s: byte", name)
		} else {
			g.emit(".global %s: word", name)
		}
	}
	g.emit("")
}

func (g *Generator) generateFunc(fs *ast.FuncStatement, pkg string) {
	fnName := fs.Name.Value
	if fs.Receiver != nil {
		rType := g.exprToString(fs.Receiver.Type)
		rType = strings.TrimPrefix(rType, "*")
		rType = strings.ReplaceAll(rType, ".", "_")
		if pkg != "" && pkg != "main" && !strings.HasPrefix(rType, pkg) {
			rType = pkg + "_" + rType
		}
		fnName = rType + "_" + fs.Name.Value
	} else if pkg != "" && pkg != "main" {
		fnName = pkg + "_" + fs.Name.Value
	}

	isEntry := fnName == "main" || strings.Contains(fs.Linkage, "entry")

	g.currentFunc = fs
	g.funcPkg = pkg
	g.params = make(map[string]TypeKind)
	g.paramTypes = make(map[string]string)
	g.paramSizes = make(map[string]int)
	g.paramOrder = nil
	g.locals = make(map[string]TypeKind)
	g.localTypes = make(map[string]string)
	g.localSizes = make(map[string]int)
	g.localOrder = nil
	g.dicts = make(map[string]bool)
	g.loopStack = nil

	// Register parameters
	if fs.Receiver != nil {
		rKind := g.getTypeKind(fs.Receiver.Type)
		rTypeStr := g.exprToString(fs.Receiver.Type)
		g.addParamWithType(fs.Receiver.Name.Value, rKind, rTypeStr)
	}
	for _, param := range fs.Parameters {
		pKind := g.getTypeKind(param.Type)
		pTypeStr := g.exprToString(param.Type)
		g.addParamWithType(param.Name.Value, pKind, pTypeStr)
	}

	// Named return parameters are not supported in Par3
	for _, ret := range fs.ReturnParameters {
		if ret.Name != nil && ret.Name.Value != "" && ret.Name.Value != "_" {
			line := 0
			if ret.Name.Token.Line != 0 {
				line = ret.Name.Token.Line
			}
			panic(fmt.Sprintf("named return parameters are not supported in Par3 (line %d): %s", line, ret.Name.Value))
		}
	}

	// Pre-scan function body to discover and register all local variables
	if fs.Body != nil {
		g.collectLocals(fs.Body)
	}
	g.addLocal("_pbuf", KindScalar)
	g.addLocal("_tmp_base", KindScalar)
	g.addLocal("_tmp_cap", KindScalar)
	g.addLocal("_tmp_len", KindScalar)
	g.addLocal("_tmp_addr", KindScalar)
	g.addLocal("_swap_0", KindSlice)
	g.addLocal("_swap_1", KindSlice)
	g.addLocal("_swap_2", KindSlice)
	g.addLocal("_swap_3", KindSlice)

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
		sz := g.paramSizes[pName]
		if sz == 6 {
			g.emit("    .param %s: slice", pName)
		} else if sz == 2 {
			g.emit("    .param %s: word", pName)
		} else if sz == 1 {
			g.emit("    .param %s: byte", pName)
		} else {
			g.emit("    .param %s: %d", pName, sz)
		}
	}

	// Emit .local directives
	for _, lName := range g.localOrder {
		sz := g.localSizes[lName]
		if sz == 6 {
			g.emit("    .local %s: slice", lName)
		} else if sz == 2 {
			g.emit("    .local %s: word", lName)
		} else if sz == 1 {
			g.emit("    .local %s: byte", lName)
		} else {
			g.emit("    .local %s: %d", lName, sz)
		}
	}
	if len(g.paramOrder) > 0 || len(g.localOrder) > 0 {
		g.emit("")
	}

	// Emit global variable initializers if this is the entry function
	if isEntry {
		currPkg := ""
		for _, stmt := range g.program.Statements {
			switch s := stmt.(type) {
			case *ast.PackageStatement:
				currPkg = s.Name.Value
			case *ast.VarStatement:
				if s.GetToken() != nil && (strings.HasSuffix(s.GetToken().Filename, "prelude.golf") || strings.HasSuffix(s.GetToken().Filename, "prelude.par3") || strings.HasSuffix(s.GetToken().Filename, "prelude.par4")) {
					continue
				}
				if s.Value != nil {
					qname := s.Name.Value
					if currPkg != "" && currPkg != "main" {
						qname = currPkg + "_" + s.Name.Value
					}
					g.emitComment("init global %s", qname)
					g.compileExpression(s.Value)
					g.emit("    STORE_GLOBAL %s", qname)
				}
			}
		}
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
	g.addLocalWithType(name, kind, "word")
}

func (g *Generator) addLocalWithType(name string, kind TypeKind, typeStr string) {
	if name == "_" || name == "" {
		return
	}
	if _, exists := g.params[name]; exists {
		return
	}
	if _, exists := g.locals[name]; !exists {
		g.locals[name] = kind
		g.localTypes[name] = typeStr
		sz := g.getTypeSizeByName(typeStr)
		if kind == KindSlice && sz < 6 {
			sz = 6
		}
		g.localSizes[name] = sz
		g.localOrder = append(g.localOrder, name)
	}
}

func (g *Generator) addParamWithType(name string, kind TypeKind, typeStr string) {
	if name == "_" || name == "" {
		return
	}
	if _, exists := g.params[name]; !exists {
		g.params[name] = kind
		g.paramTypes[name] = typeStr
		sz := g.getTypeSizeByName(typeStr)
		if kind == KindSlice && sz < 6 {
			sz = 6
		}
		g.paramSizes[name] = sz
		g.paramOrder = append(g.paramOrder, name)
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
		if arr, ok := s.ValueType.(*ast.ArrayType); ok && arr.Length != nil {
			panic(fmt.Sprintf("buffers on the stack are not supported in MiniGolf-NP (line %d): declare '%s' as a global variable or allocate with alloc()", s.Token.Line, s.Name.Value))
		}
		kind := KindScalar
		tStr := "word"
		if s.ValueType != nil {
			tStr = g.exprToString(s.ValueType)
			kind = g.getTypeKind(s.ValueType)
		} else if s.Value != nil {
			tStr = g.getArgTypeString(s.Value)
			kind = g.inferType(s.Value)
		}
		g.addLocalWithType(s.Name.Value, kind, tStr)

	case *ast.AssignStatement:
		if s.Token.Literal == ":=" {
			for i, lhs := range s.Names {
				if id, ok := lhs.(*ast.Identifier); ok && id.Value != "_" {
					kind := KindScalar
					tStr := "word"
					if i < len(s.Values) {
						if lit, ok := s.Values[i].(*ast.CompositeLit); ok {
							if _, ok := lit.Type.(*ast.ArrayType); ok {
								panic(fmt.Sprintf("buffers on the stack are not supported in MiniGolf-NP (line %d): declare '%s' as a global variable or allocate with alloc()", s.Token.Line, id.Value))
							}
						}
						if g.isDictExpr(s.Values[i]) {
							g.dicts[id.Value] = true
							kind = KindSlice
							tStr = "dict"
						} else {
							kind = g.inferType(s.Values[i])
							tStr = g.getArgTypeString(s.Values[i])
						}
					} else if len(s.Values) == 1 {
						kind = g.inferCallReturnType(s.Values[0], i)
						if kind == KindSlice {
							tStr = "string"
						} else {
							tStr = "word"
						}
					}
					if kind == KindSlice && tStr == "word" {
						tStr = "string"
					}
					g.addLocalWithType(id.Value, kind, tStr)
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
					tStr := "word"
					if rngKind == KindSlice {
						elemKind, _ = g.getElemType(s.RangeValue)
						if elemKind == KindSlice {
							tStr = "string"
						} else {
							tStr = "byte"
						}
					}
					g.addLocalWithType(valId.Value, elemKind, tStr)
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

func (g *Generator) isMemTarget(expr ast.Expression) bool {
	switch expr.(type) {
	case *ast.SelectorExpression, *ast.IndexExpression:
		return true
	case *ast.PrefixExpression, *ast.PointerType:
		return true
	}
	return false
}

func (g *Generator) compileAssign(s *ast.AssignStatement) {
	// Single assignment: lhs = rhs or lhs := rhs
	if len(s.Names) == 1 && len(s.Values) == 1 {
		lhs := s.Names[0]
		rhs := s.Values[0]

		if pref, ok := lhs.(*ast.PrefixExpression); ok && pref.Operator == "*" {
			isByte := g.isBytePointer(pref.Right)
			g.compileExpression(rhs)
			g.compileExpression(pref.Right)
			g.emit("    SWAP")
			if isByte {
				g.emit("    POKE1")
			} else {
				g.emit("    POKE2")
			}
			return
		}

		if ptrType, ok := lhs.(*ast.PointerType); ok {
			isByte := g.isBytePointer(ptrType.Elt)
			g.compileExpression(rhs)
			g.compileExpression(ptrType.Elt)
			g.emit("    SWAP")
			if isByte {
				g.emit("    POKE1")
			} else {
				g.emit("    POKE2")
			}
			return
		}

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
		tempVars := make([]string, len(s.Values))
		for i, rhs := range s.Values {
			tempVars[i] = fmt.Sprintf("_swap_%d", i)
			g.compileExpression(rhs)
			g.emit("    STORE_LOCAL %s", tempVars[i])
		}
		for i, lhs := range s.Names {
			g.emit("    LOAD_LOCAL %s", tempVars[i])
			g.storeTarget(lhs, s.Values[i])
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
		if qname, isGlobal := g.resolveGlobal(target.Value); isGlobal {
			g.emit("    STORE_GLOBAL %s", qname)
			return
		}
		// Fallback to local
		g.emit("    STORE_LOCAL %s", target.Value)

	case *ast.PrefixExpression:
		// *ptr = val
		if target.Operator == "*" {
			isByte := g.isBytePointer(target.Right)
			tempName := "_tmp_base"
			g.emit("    STORE_LOCAL %s", tempName)
			g.compileExpression(target.Right)
			g.emit("    LOAD_LOCAL %s", tempName)
			if isByte {
				g.emit("    POKE1")
			} else {
				g.emit("    POKE2")
			}
			return
		}
		panic(fmt.Sprintf("invalid assignment target prefix operator %q", target.Operator))

	case *ast.PointerType:
		// *ptr = val
		isByte := g.isBytePointer(target.Elt)
		tempName := "_tmp_base"
		g.emit("    STORE_LOCAL %s", tempName)
		g.compileExpression(target.Elt)
		g.emit("    LOAD_LOCAL %s", tempName)
		if isByte {
			g.emit("    POKE1")
		} else {
			g.emit("    POKE2")
		}
		return

	case *ast.SelectorExpression, *ast.IndexExpression:
		_, size := g.compileAddress(target)
		if size == 1 {
			g.emit("    SWAP")
			g.emit("    POKE1")
		} else if size == 2 {
			g.emit("    SWAP")
			g.emit("    POKE2")
		} else if size == 6 {
			g.emit("    STORE_LOCAL _tmp_addr")
			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    PUSH_I16 4")
			g.emit("    ADD")
			g.emit("    SWAP")
			g.emit("    POKE2")

			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    PUSH_I16 2")
			g.emit("    ADD")
			g.emit("    SWAP")
			g.emit("    POKE2")

			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    SWAP")
			g.emit("    POKE2")
		} else {
			g.emit("    STORE_LOCAL _tmp_addr")
			for off := size - 2; off >= 0; off -= 2 {
				g.emit("    LOAD_LOCAL _tmp_addr")
				if off != 0 {
					g.emit("    PUSH_I16 %d", off)
					g.emit("    ADD")
				}
				g.emit("    SWAP")
				g.emit("    POKE2")
			}
		}

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

	if sel, ok := s.Name.(*ast.SelectorExpression); ok {
		_, size := g.compileAddress(sel)
		g.emit("    STORE_LOCAL _tmp_addr")
		g.emit("    LOAD_LOCAL _tmp_addr")
		if size == 1 {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}
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
		g.emit("    LOAD_LOCAL _tmp_addr")
		g.emit("    SWAP")
		if size == 1 {
			g.emit("    POKE1")
		} else {
			g.emit("    POKE2")
		}
		return
	}

	if idx, ok := s.Name.(*ast.IndexExpression); ok {
		_, size := g.compileAddress(idx)
		g.emit("    STORE_LOCAL _tmp_addr")
		g.emit("    LOAD_LOCAL _tmp_addr")
		if size == 1 {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}
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
		g.emit("    LOAD_LOCAL _tmp_addr")
		g.emit("    SWAP")
		if size == 1 {
			g.emit("    POKE1")
		} else {
			g.emit("    POKE2")
		}
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

	if sel, ok := s.Name.(*ast.SelectorExpression); ok {
		_, size := g.compileAddress(sel)
		g.emit("    DUP")
		if size == 1 {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}
		g.emit("    PUSH_1")
		if s.Token.Literal == "++" {
			g.emit("    ADD")
		} else {
			g.emit("    SUB")
		}
		if size == 1 {
			g.emit("    POKE1")
		} else {
			g.emit("    POKE2")
		}
		return
	}

	if idx, ok := s.Name.(*ast.IndexExpression); ok {
		_, size := g.compileAddress(idx)
		g.emit("    DUP")
		if size == 1 {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}
		g.emit("    PUSH_1")
		if s.Token.Literal == "++" {
			g.emit("    ADD")
		} else {
			g.emit("    SUB")
		}
		if size == 1 {
			g.emit("    POKE1")
		} else {
			g.emit("    POKE2")
		}
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

	isSlice := (g.inferType(s.RangeValue) == KindSlice)

	if !isSlice {
		// Integer range: for k := range N
		g.compileExpression(s.RangeValue)
		g.emit("    STORE_LOCAL %s", lenVar)
	} else {
		// Slice range: for i, val := range slice
		g.compileExpression(s.RangeValue)
		g.emit("    STORE_LOCAL %s", valVar)
		g.emit("    LOAD_LOCAL %s", valVar)
		g.emit("    SLICE_LEN")
		g.emit("    STORE_LOCAL %s", lenVar)
	}

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
			elemKind, elemSize := g.getElemType(s.RangeValue)
			// valVar is a slice (Base, Cap, Len)
			// Base is at offset 4 from top of slice (pop Len, pop Cap)
			g.emit("    LOAD_LOCAL %s", valVar)
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    LOAD_LOCAL %s", idxVar)
			if elemSize == 2 {
				g.emit("    SHL1_ADD")
			} else if elemSize == 1 {
				g.emit("    ADD")
			} else {
				g.emit("    PUSH_I16 %d", elemSize)
				g.emit("    MUL")
				g.emit("    ADD")
			}

			if elemKind == KindSlice && elemSize == 6 {
				tmpAddr := "_tmp_addr"
				g.emit("    STORE_LOCAL %s", tmpAddr)
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 2")
				g.emit("    ADD")
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 4")
				g.emit("    ADD")
				g.emit("    PEEK2")
				g.emit("    STORE_LOCAL %s", id.Value)
			} else if elemSize == 1 {
				g.emit("    PEEK1")
				g.emit("    STORE_LOCAL %s", id.Value)
			} else {
				g.emit("    PEEK2")
				g.emit("    STORE_LOCAL %s", id.Value)
			}
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
		if g.currentFunc != nil && len(g.currentFunc.ReturnParameters) > 0 {
			line := 0
			if s.Token.Line != 0 {
				line = s.Token.Line
			}
			panic(fmt.Sprintf("bare return is not supported in Par3 (line %d): return value required", line))
		}
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

	case *ast.PointerType:
		g.compileExpression(e.Elt)
		if g.isBytePointer(e.Elt) {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}

	case *ast.InfixExpression:
		g.compileInfix(e)

	case *ast.CallExpression:
		g.compileCall(e)

	case *ast.IndexExpression:
		if e.IsSlice || len(e.Indices) >= 2 {
			g.compileSubSlice(e)
		} else if len(e.Indices) == 1 {
			elemKind, elemSize := g.getElemType(e.Left)
			g.compileElemAddress(e.Left, e.Indices[0], elemSize)
			if elemKind == KindSlice && elemSize == 6 {
				tmpAddr := "_tmp_addr"
				g.emit("    STORE_LOCAL %s", tmpAddr)
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 2")
				g.emit("    ADD")
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 4")
				g.emit("    ADD")
				g.emit("    PEEK2")
			} else if elemSize == 1 {
				g.emit("    PEEK1")
			} else if elemSize == 2 {
				g.emit("    PEEK2")
			} else {
				tmpAddr := "_tmp_addr"
				g.emit("    STORE_LOCAL %s", tmpAddr)
				for off := 0; off < elemSize; off += 2 {
					g.emit("    LOAD_LOCAL %s", tmpAddr)
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    PEEK2")
				}
			}
		}

	case *ast.SelectorExpression:
		_, size := g.compileAddress(e)
		if size == 1 {
			g.emit("    PEEK1")
		} else if size == 2 {
			g.emit("    PEEK2")
		} else if size == 6 {
			tmpAddr := "_tmp_addr"
			g.emit("    STORE_LOCAL %s", tmpAddr)
			g.emit("    LOAD_LOCAL %s", tmpAddr)
			g.emit("    PEEK2")
			g.emit("    LOAD_LOCAL %s", tmpAddr)
			g.emit("    PUSH_I16 2")
			g.emit("    ADD")
			g.emit("    PEEK2")
			g.emit("    LOAD_LOCAL %s", tmpAddr)
			g.emit("    PUSH_I16 4")
			g.emit("    ADD")
			g.emit("    PEEK2")
		} else {
			tmpAddr := "_tmp_addr"
			g.emit("    STORE_LOCAL %s", tmpAddr)
			for off := 0; off < size; off += 2 {
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				if off != 0 {
					g.emit("    PUSH_I16 %d", off)
					g.emit("    ADD")
				}
				g.emit("    PEEK2")
			}
		}

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
	if qname, isGlobal := g.resolveGlobal(name); isGlobal {
		kind := g.globals[qname]
		if kind == KindBuffer {
			g.emit("    ADDR_OF_GLOBAL %s", qname)
			return
		}
		g.emit("    LOAD_GLOBAL %s", qname)
		return
	}

	// Fallback to local
	g.emit("    LOAD_LOCAL %s", name)
}

func (g *Generator) compilePrefix(e *ast.PrefixExpression) {
	switch e.Operator {
	case "&":
		_, _ = g.compileAddress(e.Right)
		return
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
		if g.isBytePointer(e.Right) {
			g.emit("    PEEK1")
		} else {
			g.emit("    PEEK2")
		}
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
			g.emit("    PUSH_NEG1")
			g.emit("    CMP_EQ")
		case "<=":
			g.emit("    PUSH_1")
			g.emit("    CMP_NE")
		case ">":
			g.emit("    PUSH_1")
			g.emit("    CMP_EQ")
		case ">=":
			g.emit("    PUSH_NEG1")
			g.emit("    CMP_NE")
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
	if id, ok := e.Left.(*ast.Identifier); ok && g.globals[id.Value] == KindBuffer {
		g.emit("    ADDR_OF_GLOBAL %s", id.Value)
		sz := g.globalSizes[id.Value]
		g.emit("    PUSH_I16 %d", sz)
		g.emit("    PUSH_I16 %d", sz)
		g.emit("    SLICE_NEW")
	} else {
		g.compileExpression(e.Left)
	}

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

		// Check if sel.Left is an imported package name (e.g. btree.New, smap.New)
		if pkgIdent, isPkg := recv.(*ast.Identifier); isPkg {
			if pkgIdent.Value == "smap" && method == "New" {
				n := 8
				if len(call.Arguments) > 0 {
					n = g.evalIntConst(call.Arguments[0])
				}
				g.emit("    DICT_NEW %d", n)
				return
			}
			qname := pkgIdent.Value + "_" + method
			if _, exists := g.funcs[qname]; exists {
				for _, arg := range call.Arguments {
					g.compileExpression(arg)
				}
				g.emit("    CALL %s", qname)
				return
			}
			if _, exists := g.funcs[pkgIdent.Value+"."+method]; exists {
				for _, arg := range call.Arguments {
					g.compileExpression(arg)
				}
				g.emit("    CALL %s", qname)
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

		case "Get":
			_, elemSize := g.getElemTypeInfo(recv)
			g.compileElemAddress(recv, call.Arguments[0], elemSize)
			if elemSize == 6 {
				tmpAddr := "_tmp_addr"
				g.emit("    STORE_LOCAL %s", tmpAddr)
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 2")
				g.emit("    ADD")
				g.emit("    PEEK2")
				g.emit("    LOAD_LOCAL %s", tmpAddr)
				g.emit("    PUSH_I16 4")
				g.emit("    ADD")
				g.emit("    PEEK2")
			} else if elemSize == 1 {
				g.emit("    PEEK1")
			} else if elemSize == 2 {
				g.emit("    PEEK2")
			} else {
				tmpAddr := "_tmp_addr"
				g.emit("    STORE_LOCAL %s", tmpAddr)
				for off := 0; off < elemSize; off += 2 {
					g.emit("    LOAD_LOCAL %s", tmpAddr)
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    PEEK2")
				}
			}
			return

		case "Append":
			g.compileExpression(recv)
			g.compileExpression(call.Arguments[0])
			elemTypeStr, elemSize := g.getElemTypeInfo(recv)
			if elemSize == 1 || elemTypeStr == "byte" {
				g.emit("    STR_APPEND")
			} else if elemSize == 6 || elemTypeStr == "string" {
				g.emit("    SLICE_APPEND_STR")
			} else {
				g.emit("    LIST_APPEND")
			}
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
			rTypeName := g.inferTypeName(recv)
			targetMethod := rTypeName + "_" + method
			passAddr := false
			var fs *ast.FuncStatement
			if f, exists := g.funcs[targetMethod]; exists {
				fs = f
			} else if g.funcPkg != "" && g.funcPkg != "main" {
				candidate := g.funcPkg + "_" + targetMethod
				if f2, exists2 := g.funcs[candidate]; exists2 {
					targetMethod = candidate
					fs = f2
				}
			}
			if fs != nil && fs.Receiver != nil {
				if _, isPtr := fs.Receiver.Type.(*ast.PointerType); isPtr {
					recvType := g.getArgTypeString(recv)
					if !strings.HasPrefix(recvType, "*") {
						passAddr = true
					}
				}
			}
			if passAddr {
				g.compileAddress(recv)
			} else {
				g.compileExpression(recv)
			}
			for _, arg := range call.Arguments {
				g.compileExpression(arg)
			}
			g.emit("    CALL %s", targetMethod)
			return
		}
	}

	// 2. Generic/index call: e.g. sizeof[T]() or smap.New[string](8)
	if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
		if id, ok := idxExpr.Left.(*ast.Identifier); ok && id.Value == "sizeof" {
			sz := g.getTypeSize(idxExpr.Indices[0])
			g.emit("    PUSH_I16 %d", sz)
			return
		}
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

	// 2b. Pointer type cast: (*Node)(expr)
	if _, ok := call.Function.(*ast.PointerType); ok {
		g.compileExpression(call.Arguments[0])
		return
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

	case "strdup":
		g.compileExpression(call.Arguments[0])
		return

	case "print":
		g.compilePrint(call, false)
		return

	case "println":
		g.compilePrint(call, true)
		return

	case "alloc", "malloc", "zalloc":
		g.compileExpression(call.Arguments[0])
		g.emit("    BUF_ALLOC")
		return

	case "free":
		g.compileExpression(call.Arguments[0])
		g.emit("    BUF_FREE")
		return

	case "memcpy", "mem_copy":
		g.compileExpression(call.Arguments[1]) // src
		g.compileExpression(call.Arguments[0]) // dst
		g.compileExpression(call.Arguments[2]) // count
		g.emit("    MEM_COPY")
		return

	case "memset", "mem_set":
		g.compileExpression(call.Arguments[0]) // dst
		g.compileExpression(call.Arguments[1]) // val
		g.compileExpression(call.Arguments[2]) // count
		g.emit("    MEM_SET")
		return

	case "hatvan_trap", "os_call", "sys_trap":
		g.compileExpression(call.Arguments[0]) // A
		g.compileExpression(call.Arguments[1]) // B
		g.compileExpression(call.Arguments[2]) // X
		g.compileExpression(call.Arguments[3]) // Y
		g.compileExpression(call.Arguments[4]) // U
		trapNum := g.evalIntConst(call.Arguments[5])
		g.emit("    HATVAN_TRAP 0x%02X", trapNum)
		return

	case "sys_poll_flag":
		flagNum := g.evalIntConst(call.Arguments[0])
		g.emit("    SYS_POLL_FLAG %d", flagNum)
		return

	case "sys_dma_copy":
		for _, arg := range call.Arguments {
			g.compileExpression(arg)
		}
		g.emit("    SYS_DMA_COPY")
		return

	case "peekb", "peek_byte":
		g.compileExpression(call.Arguments[0])
		g.emit("    PEEK1")
		return

	case "peek", "peekw", "peek_word":
		g.compileExpression(call.Arguments[0])
		g.emit("    PEEK2")
		return

	case "pokeb", "poke_byte":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    POKE1")
		return

	case "poke", "pokew", "poke_word":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    POKE2")
		return

	case "sys_exit", "exit":
		if len(call.Arguments) > 0 {
			g.compileExpression(call.Arguments[0])
		} else {
			g.emit("    PUSH_0")
		}
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

	case "file_open_read", "file_open":
		g.emit("    PUSH_1")
		g.emit("    PUSH_0")
		g.compileExpression(call.Arguments[0])
		if g.inferType(call.Arguments[0]) == KindSlice {
			g.emit("    POP")
			g.emit("    POP")
		}
		g.emit("    PUSH_0")
		g.emit("    PUSH_0")
		g.emit("    HATVAN_TRAP I$Open")
		g.emit("    POP")
		g.emit("    POP")
		g.emit("    POP")
		return

	case "file_open_write", "file_create":
		g.emit("    PUSH_I16 2")
		g.emit("    PUSH_0")
		g.compileExpression(call.Arguments[0])
		if g.inferType(call.Arguments[0]) == KindSlice {
			g.emit("    POP")
			g.emit("    POP")
		}
		g.emit("    PUSH_0")
		g.emit("    PUSH_0")
		g.emit("    HATVAN_TRAP I$Create")
		g.emit("    POP")
		g.emit("    POP")
		g.emit("    POP")
		return

	case "file_close":
		g.compileExpression(call.Arguments[0])
		g.emit("    PUSH_0")
		g.emit("    PUSH_0")
		g.emit("    PUSH_0")
		g.emit("    PUSH_0")
		g.emit("    HATVAN_TRAP I$Close")
		g.emit("    POP")
		g.emit("    POP")
		g.emit("    POP")
		g.emit("    POP")
		return

	case "map_new":
		n := 8
		if len(call.Arguments) > 0 {
			n = g.evalIntConst(call.Arguments[0])
		}
		g.emit("    DICT_NEW %d", n)
		return

	case "map_get":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    DICT_GET")
		return

	case "map_put":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.compileExpression(call.Arguments[2])
		g.emit("    DICT_SET")
		return

	case "map_count":
		g.compileExpression(call.Arguments[0])
		g.emit("    DICT_LEN")
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

	case "int", "int16":
		panic(fmt.Sprintf("signed 'int' is not supported in MiniGolf-NP (line %d): use unsigned 'word' or 'byte'", call.Token.Line))

	case "word", "uint", "uint16":
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
	} else if g.funcPkg != "" && g.funcPkg != "main" {
		pkgFn := g.funcPkg + "_" + fnName
		if _, exists := g.funcs[pkgFn]; exists {
			targetName = pkgFn
		}
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
		case "print", "println", "free", "poke", "pokeb", "pokew", "poke_byte", "poke_word", "memcpy", "mem_copy", "memset", "mem_set", "file_close", "sys_exit", "exit":
			return true
		}
		if fs, exists := g.funcs[id.Value]; exists && len(fs.ReturnParameters) == 0 {
			return true
		}
		if g.funcPkg != "" && g.funcPkg != "main" {
			pkgFn := g.funcPkg + "_" + id.Value
			if fs, exists := g.funcs[pkgFn]; exists && len(fs.ReturnParameters) == 0 {
				return true
			}
		}
	}
	if sel, ok := call.Function.(*ast.SelectorExpression); ok {
		switch sel.Right.Value {
		case "Insert", "Append":
			return true
		}
		rTypeName := g.inferTypeName(sel.Left)
		methodKey := rTypeName + "_" + sel.Right.Value
		if fs, exists := g.funcs[methodKey]; exists && len(fs.ReturnParameters) == 0 {
			return true
		}
		if id, ok := sel.Left.(*ast.Identifier); ok {
			key := id.Value + "_" + sel.Right.Value
			if fs, exists := g.funcs[key]; exists && len(fs.ReturnParameters) == 0 {
				return true
			}
			key2 := id.Value + "." + sel.Right.Value
			if fs, exists := g.funcs[key2]; exists && len(fs.ReturnParameters) == 0 {
				return true
			}
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

func (g *Generator) lookupStruct(name string) (*structInfo, bool) {
	name = strings.TrimPrefix(name, "*")
	if g.funcPkg != "" && g.funcPkg != "main" && !strings.Contains(name, ".") && !strings.Contains(name, "_") {
		if s, ok := g.structs[g.funcPkg+"."+name]; ok {
			return &s, true
		}
		if s, ok := g.structs[g.funcPkg+"_"+name]; ok {
			return &s, true
		}
	}
	if s, ok := g.structs[name]; ok {
		return &s, true
	}
	if strings.Contains(name, ".") {
		if s, ok := g.structs[strings.ReplaceAll(name, ".", "_")]; ok {
			return &s, true
		}
	}
	if strings.Contains(name, "_") {
		if s, ok := g.structs[strings.ReplaceAll(name, "_", ".")]; ok {
			return &s, true
		}
	}
	return nil, false
}

func (g *Generator) resolveGlobal(name string) (string, bool) {
	if _, ok := g.globals[name]; ok {
		return name, true
	}
	if g.funcPkg != "" && g.funcPkg != "main" && !strings.Contains(name, "_") {
		qname := g.funcPkg + "_" + name
		if _, ok := g.globals[qname]; ok {
			return qname, true
		}
	}
	return "", false
}

func (g *Generator) getTypeSizeByName(name string) int {
	if strings.HasPrefix(name, "*") {
		return 2
	}
	if name == "byte" || name == "uint8" || name == "bool" {
		return 1
	}
	if name == "string" || strings.HasPrefix(name, "slice[") || strings.HasPrefix(name, "[]") || strings.HasPrefix(name, "smap[") || strings.HasPrefix(name, "map[") || name == "dict" {
		return 6
	}
	if sInfo, ok := g.lookupStruct(name); ok {
		return sInfo.size
	}
	return 2
}

func (g *Generator) getTypeKind(typ ast.Expression) TypeKind {
	if typ == nil {
		return KindScalar
	}
	switch t := typ.(type) {
	case *ast.Identifier:
		name := t.Value
		if name == "int" || name == "int16" {
			panic(fmt.Sprintf("signed 'int' is not supported in MiniGolf-NP (line %d): use unsigned 'word' or 'byte'", t.Token.Line))
		}
		if name == "string" {
			return KindSlice
		}
		if name == "noreturn" || name == "void" {
			return KindVoid
		}
		if _, ok := g.lookupStruct(name); ok {
			return KindBuffer
		}
		return KindScalar
	case *ast.SelectorExpression:
		sName := g.exprToString(t)
		if _, ok := g.lookupStruct(sName); ok {
			return KindBuffer
		}
		return KindScalar
	case *ast.ArrayType:
		if t.Length != nil {
			return KindBuffer
		}
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

func (g *Generator) getTypeSize(typ ast.Expression) int {
	if typ == nil {
		return 2
	}
	switch t := typ.(type) {
	case *ast.Identifier:
		if t.Value == "byte" || t.Value == "uint8" || t.Value == "bool" {
			return 1
		}
		if t.Value == "string" {
			return 6
		}
		if sInfo, ok := g.lookupStruct(t.Value); ok {
			return sInfo.size
		}
		return 2
	case *ast.SelectorExpression:
		sName := g.exprToString(t)
		if sInfo, ok := g.lookupStruct(sName); ok {
			return sInfo.size
		}
		return 2
	case *ast.ArrayType:
		if t.Length != nil {
			if count := g.evalIntConst(t.Length); count > 0 {
				return count * g.getTypeSize(t.Elt)
			}
			if intLit, ok := t.Length.(*ast.IntegerLiteral); ok {
				return int(intLit.Value) * g.getTypeSize(t.Elt)
			}
		}
		return 6
	case *ast.PointerType:
		return 2
	case *ast.IndexExpression:
		return 6
	default:
		return 2
	}
}

func (g *Generator) getElemTypeInfo(expr ast.Expression) (string, int) {
	if expr == nil {
		return "word", 2
	}
	tStr := g.getArgTypeString(expr)
	if id, ok := expr.(*ast.Identifier); ok {
		if qname, isGlobal := g.resolveGlobal(id.Value); isGlobal && g.globals[qname] == KindBuffer {
			elemSz := g.globalElemSizes[qname]
			if elemSz == 0 {
				elemSz = 1
			}
			if strings.HasPrefix(tStr, "slice[") && strings.HasSuffix(tStr, "]") {
				elemTypeName := tStr[6 : len(tStr)-1]
				return elemTypeName, elemSz
			}
			if idx := strings.Index(tStr, "]"); idx >= 0 && idx < len(tStr)-1 {
				elemTypeName := tStr[idx+1:]
				return elemTypeName, elemSz
			}
			return "word", elemSz
		}
	}
	if strings.HasPrefix(tStr, "slice[") && strings.HasSuffix(tStr, "]") {
		elemTypeName := tStr[6 : len(tStr)-1]
		elemSz := g.getTypeSizeByName(elemTypeName)
		return elemTypeName, elemSz
	}
	if strings.Contains(tStr, "slice[string]") || strings.Contains(tStr, "[]string") || strings.HasSuffix(tStr, "]string") {
		return "string", 6
	}
	if idx := strings.Index(tStr, "]"); idx >= 0 && idx < len(tStr)-1 {
		elemTypeName := tStr[idx+1:]
		elemSz := g.getTypeSizeByName(elemTypeName)
		return elemTypeName, elemSz
	}
	if tStr == "string" || strings.Contains(tStr, "byte") || strings.Contains(tStr, "uint8") {
		return "byte", 1
	}
	return "word", 2
}

func (g *Generator) getElemType(expr ast.Expression) (TypeKind, int) {
	_, elemSz := g.getElemTypeInfo(expr)
	if elemSz == 6 {
		return KindSlice, 6
	}
	if elemSz == 1 {
		return KindScalar, 1
	}
	if elemSz == 2 {
		return KindScalar, 2
	}
	return KindBuffer, elemSz
}

func (g *Generator) compileAddress(expr ast.Expression) (string, int) {
	switch e := expr.(type) {
	case *ast.Identifier:
		name := e.Value
		if qname, isGlobal := g.resolveGlobal(name); isGlobal {
			g.emit("    ADDR_OF_GLOBAL %s", qname)
			tStr := g.globalTypes[qname]
			return tStr, g.getTypeSizeByName(tStr)
		}
		if _, isLocal := g.locals[name]; isLocal {
			g.emit("    ADDR_OF_LOCAL %s", name)
			tStr := g.localTypes[name]
			return tStr, g.getTypeSizeByName(tStr)
		}
		if _, isParam := g.params[name]; isParam {
			g.emit("    ADDR_OF_LOCAL %s", name)
			tStr := g.paramTypes[name]
			return tStr, g.getTypeSizeByName(tStr)
		}
		panic(fmt.Sprintf("unknown variable for address: %s", name))

	case *ast.IndexExpression:
		elemTypeStr, elemSize := g.getElemTypeInfo(e.Left)
		g.compileElemAddress(e.Left, e.Indices[0], elemSize)
		return elemTypeStr, elemSize

	case *ast.CallExpression:
		g.compileExpression(e)
		tStr := ""
		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			rTypeName := g.inferTypeName(sel.Left)
			methodKey := rTypeName + "_" + sel.Right.Value
			if fs, exists := g.funcs[methodKey]; exists && len(fs.ReturnParameters) > 0 {
				tStr = g.exprToString(fs.ReturnParameters[0].Type)
			}
		} else if id, ok := e.Function.(*ast.Identifier); ok {
			fnName := id.Value
			if g.funcPkg != "" && g.funcPkg != "main" {
				if fs, exists := g.funcs[g.funcPkg+"_"+fnName]; exists && len(fs.ReturnParameters) > 0 {
					tStr = g.exprToString(fs.ReturnParameters[0].Type)
				}
			}
			if tStr == "" {
				if fs, exists := g.funcs[fnName]; exists && len(fs.ReturnParameters) > 0 {
					tStr = g.exprToString(fs.ReturnParameters[0].Type)
				}
			}
		}
		return tStr, 2

	case *ast.SelectorExpression:
		fieldName := e.Right.Value
		leftTypeStr := g.getArgTypeString(e.Left)
		isSlice := (leftTypeStr == "string" || strings.HasPrefix(leftTypeStr, "[]") || strings.HasPrefix(leftTypeStr, "slice[") || leftTypeStr == "dict" || strings.HasPrefix(leftTypeStr, "smap[") || strings.HasPrefix(leftTypeStr, "map["))
		if isSlice && (fieldName == "Base" || fieldName == "Cap" || fieldName == "Len") {
			g.compileAddress(e.Left)
			offset := 0
			switch fieldName {
			case "Base":
				offset = 0
			case "Cap":
				offset = 2
			case "Len":
				offset = 4
			}
			if offset != 0 {
				g.emit("    PUSH_I16 %d", offset)
				g.emit("    ADD")
			}
			return "word", 2
		}

		leftType, _ := g.compileAddress(e.Left)
		if strings.HasPrefix(leftType, "*") {
			g.emit("    PEEK2")
			leftType = strings.TrimPrefix(leftType, "*")
		}
		sInfo, ok := g.lookupStruct(leftType)
		if !ok {
			panic(fmt.Sprintf("unknown struct type %q for selector .%s", leftType, fieldName))
		}
		for _, f := range sInfo.fields {
			if f.name == fieldName {
				if f.offset != 0 {
					g.emit("    PUSH_I16 %d", f.offset)
					g.emit("    ADD")
				}
				fTypeStr := g.exprToString(f.typ)
				return fTypeStr, f.size
			}
		}
		panic(fmt.Sprintf("field %s not found in struct %s", fieldName, leftType))

	default:
		panic(fmt.Sprintf("unsupported address-of expression: %T", expr))
	}
}

func (g *Generator) compileElemAddress(target ast.Expression, idx ast.Expression, elemSize int) {
	if id, ok := target.(*ast.Identifier); ok {
		if qname, isGlobal := g.resolveGlobal(id.Value); isGlobal {
			if g.globals[qname] == KindBuffer {
				g.emit("    ADDR_OF_GLOBAL %s", qname)
			} else {
				g.emit("    ADDR_OF_GLOBAL %s", qname)
				g.emit("    PEEK2")
			}
		} else if _, isLocal := g.locals[id.Value]; isLocal {
			g.emit("    ADDR_OF_LOCAL %s", id.Value)
			g.emit("    PEEK2")
		} else if _, isParam := g.params[id.Value]; isParam {
			g.emit("    ADDR_OF_LOCAL %s", id.Value)
			g.emit("    PEEK2")
		} else {
			g.compileExpression(target)
			g.emit("    PEEK2")
		}
	} else if sel, ok := target.(*ast.SelectorExpression); ok {
		fTypeStr, _ := g.compileAddress(sel)
		if !strings.HasPrefix(fTypeStr, "[") || strings.HasPrefix(fTypeStr, "[]") {
			g.emit("    PEEK2")
		}
	} else {
		g.compileExpression(target)
		g.emit("    PEEK2")
	}
	g.compileExpression(idx)
	if elemSize == 2 {
		g.emit("    SHL1_ADD")
	} else if elemSize == 1 {
		g.emit("    ADD")
	} else {
		g.emit("    PUSH_I16 %d", elemSize)
		g.emit("    MUL")
		g.emit("    ADD")
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
		if qname, isGlobal := g.resolveGlobal(e.Value); isGlobal {
			return g.globals[qname]
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
		elemKind, _ := g.getElemType(e.Left)
		return elemKind
	case *ast.SelectorExpression:
		leftType := g.getArgTypeString(e.Left)
		isSlice := (leftType == "string" || strings.HasPrefix(leftType, "[]") || strings.HasPrefix(leftType, "slice[") || leftType == "dict" || strings.HasPrefix(leftType, "smap[") || strings.HasPrefix(leftType, "map["))
		if isSlice && (e.Right.Value == "Base" || e.Right.Value == "Cap" || e.Right.Value == "Len") {
			return KindScalar
		}
		leftType = strings.TrimPrefix(leftType, "*")
		if sInfo, ok := g.lookupStruct(leftType); ok {
			for _, f := range sInfo.fields {
				if f.name == e.Right.Value {
					return g.getTypeKind(f.typ)
				}
			}
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
		if id.Value == "file_readline" {
			if retIdx == 0 {
				return KindSlice
			}
			return KindScalar
		}
		switch id.Value {
		case "rstrip", "lstrip", "strip", "replace_ident", "splitlines", "sys_args", "make", "makeslice", "strdup", "map_str":
			return KindSlice
		case "len", "cap", "find", "startswith", "endswith", "strcmp", "streq", "file_open", "file_create", "file_open_read", "file_open_write", "file_read", "file_write", "file_close", "os_isfile", "os_makedirs", "alloc", "free", "peek", "peekw", "peekb", "peek_word", "peek_byte", "poke", "pokew", "pokeb", "poke_word", "poke_byte", "byte", "word", "int", "uint", "bool", "map_new", "map_get", "map_put", "map_count", "memcpy", "mem_copy", "memset", "mem_set", "hatvan_trap", "sys_poll_flag", "sys_dma_copy":
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
		case "Get":
			elemKind, _ := g.getElemType(sel.Left)
			return elemKind
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
	rawType := g.getArgTypeString(expr)
	rawType = strings.TrimPrefix(rawType, "*")
	rawType = strings.ReplaceAll(rawType, ".", "_")
	if g.funcPkg != "" && g.funcPkg != "main" && !strings.Contains(rawType, "_") {
		candidate := g.funcPkg + "_" + rawType
		return candidate
	}
	return rawType
}

func (g *Generator) isBytePointer(expr ast.Expression) bool {
	t := g.getArgTypeString(expr)
	return t == "*byte" || t == "*uint8" || t == "*bool"
}

func (g *Generator) exprToString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if e.Package != "" && e.Package != "main" && e.Package != "builtin" && e.Package != "prelude" {
			if e.ShortName != "" {
				return e.Package + "." + e.ShortName
			}
			return e.Package + "." + e.Value
		}
		return e.Value
	case *ast.PointerType:
		return "*" + g.exprToString(e.Elt)
	case *ast.ArrayType:
		if e.Length != nil {
			return fmt.Sprintf("[%s]%s", g.nodeToString(e.Length), g.exprToString(e.Elt))
		}
		return "[]" + g.exprToString(e.Elt)
	case *ast.IndexExpression:
		res := g.exprToString(e.Left)
		if len(e.Indices) > 0 {
			res += "["
			for i, idx := range e.Indices {
				if i > 0 {
					res += ", "
				}
				res += g.exprToString(idx)
			}
			res += "]"
		}
		return res
	case *ast.SelectorExpression:
		return g.exprToString(e.Left) + "." + e.Right.Value
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

func (g *Generator) compilePrint(call *ast.CallExpression, isPrintln bool) {
	nArgs := len(call.Arguments)
	if nArgs == 0 {
		g.emit("    PUSH_NIL_SLICE")
		if isPrintln {
			g.emit("    PRINTLN")
		} else {
			g.emit("    PRINT")
		}
		return
	}

	typeStrings := make([]string, nArgs)
	valSizes := make([]int, nArgs)
	totalValBytes := 0

	for i, arg := range call.Arguments {
		tStr := g.getArgTypeString(arg)
		if g.inferType(arg) == KindSlice && tStr != "string" {
			tStr = "string"
		}
		typeStrings[i] = tStr
		if tStr == "string" || g.inferType(arg) == KindSlice {
			valSizes[i] = 6
		} else {
			valSizes[i] = 2
		}
		totalValBytes += valSizes[i]
	}

	totalBufSize := nArgs*4 + totalValBytes
	pbufVar := "_pbuf"

	cmdName := "print"
	if isPrintln {
		cmdName = "println"
	}
	g.emitComment("%s(%s)", cmdName, g.argsToString(call.Arguments))

	g.emit("    PUSH_I16 %d", totalBufSize)
	g.emit("    BUF_ALLOC")
	g.emit("    STORE_LOCAL %s", pbufVar)

	valOffset := nArgs * 4
	for i, arg := range call.Arguments {
		tStr := typeStrings[i]
		curValOff := valOffset
		curValSize := valSizes[i]
		valOffset += curValSize

		// 1. Evaluate argument and store value at curValOff
		if curValSize == 6 {
			// Slice value (3 words)
			tmpBase := "_tmp_base"
			tmpCap := "_tmp_cap"
			tmpLen := "_tmp_len"

			g.compileExpression(arg)
			g.emit("    STORE_LOCAL %s", tmpLen)
			g.emit("    STORE_LOCAL %s", tmpCap)
			g.emit("    STORE_LOCAL %s", tmpBase)

			g.emit("    LOAD_LOCAL %s", pbufVar)
			if curValOff != 0 {
				g.emit("    PUSH_I16 %d", curValOff)
				g.emit("    ADD")
			}
			g.emit("    LOAD_LOCAL %s", tmpBase)
			g.emit("    POKE2")

			g.emit("    LOAD_LOCAL %s", pbufVar)
			g.emit("    PUSH_I16 %d", curValOff+2)
			g.emit("    ADD")
			g.emit("    LOAD_LOCAL %s", tmpCap)
			g.emit("    POKE2")

			g.emit("    LOAD_LOCAL %s", pbufVar)
			g.emit("    PUSH_I16 %d", curValOff+4)
			g.emit("    ADD")
			g.emit("    LOAD_LOCAL %s", tmpLen)
			g.emit("    POKE2")
		} else {
			// Scalar value (1 word)
			tmpVal := "_tmp_base"
			g.compileExpression(arg)
			g.emit("    STORE_LOCAL %s", tmpVal)
			g.emit("    LOAD_LOCAL %s", pbufVar)
			if curValOff != 0 {
				g.emit("    PUSH_I16 %d", curValOff)
				g.emit("    ADD")
			}
			g.emit("    LOAD_LOCAL %s", tmpVal)
			g.emit("    POKE2")
		}

		// 2. Set any[i].BaseAddr (offset i * 4) = pbuf + curValOff (+ 1 if byte)
		g.emit("    LOAD_LOCAL %s", pbufVar)
		if i*4 != 0 {
			g.emit("    PUSH_I16 %d", i*4)
			g.emit("    ADD")
		}
		g.emit("    LOAD_LOCAL %s", pbufVar)
		baseOff := curValOff
		if tStr == "byte" || tStr == "uint8" {
			baseOff = curValOff + 1
		}
		if baseOff != 0 {
			g.emit("    PUSH_I16 %d", baseOff)
			g.emit("    ADD")
		}
		g.emit("    POKE2")

		// 3. Set any[i].TypeStr (offset i * 4 + 2) = pointer to type name string
		g.emit("    LOAD_LOCAL %s", pbufVar)
		g.emit("    PUSH_I16 %d", i*4+2)
		g.emit("    ADD")
		g.emit("    PUSH_STR %s", strconv.Quote(tStr))
		g.emit("    POP")
		g.emit("    POP")
		g.emit("    POKE2")
	}

	// Push Slice[any] { pbuf, nArgs, nArgs }
	g.emit("    LOAD_LOCAL %s", pbufVar)
	if nArgs == 1 {
		g.emit("    PUSH_1")
		g.emit("    PUSH_1")
	} else {
		g.emit("    PUSH_I16 %d", nArgs)
		g.emit("    PUSH_I16 %d", nArgs)
	}

	if isPrintln {
		g.emit("    PRINTLN")
	} else {
		g.emit("    PRINT")
	}

	// Free temporary buffer
	g.emit("    LOAD_LOCAL %s", pbufVar)
	g.emit("    BUF_FREE")
}

func (g *Generator) getArgTypeString(expr ast.Expression) string {
	if expr == nil {
		return "word"
	}
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return "string"
	case *ast.IntegerLiteral:
		return "word"
	case *ast.Identifier:
		if e.Value == "true" || e.Value == "false" {
			return "bool"
		}
		if t, ok := g.localTypes[e.Value]; ok && t != "" {
			return t
		}
		if t, ok := g.paramTypes[e.Value]; ok && t != "" {
			return t
		}
		if qname, ok := g.resolveGlobal(e.Value); ok && g.globalTypes[qname] != "" {
			return g.globalTypes[qname]
		}
		if resolved := e.GetResolvedType(); resolved != nil {
			tStr := g.exprToString(resolved)
			if tStr != "" {
				return tStr
			}
		}
		return "word"
	case *ast.PointerType:
		subType := g.getArgTypeString(e.Elt)
		if strings.HasPrefix(subType, "*") {
			return strings.TrimPrefix(subType, "*")
		}
		return "word"
	case *ast.PrefixExpression:
		if e.Operator == "&" {
			return "*" + g.getArgTypeString(e.Right)
		}
		if e.Operator == "*" {
			subType := g.getArgTypeString(e.Right)
			if strings.HasPrefix(subType, "*") {
				return strings.TrimPrefix(subType, "*")
			}
			return "word"
		}
		if e.Operator == "!" {
			return "bool"
		}
		return g.getArgTypeString(e.Right)
	case *ast.IndexExpression:
		if e.IsSlice || len(e.Indices) >= 2 {
			return "string"
		}
		elemTypeStr, _ := g.getElemTypeInfo(e.Left)
		return elemTypeStr
	case *ast.SelectorExpression:
		leftType := g.getArgTypeString(e.Left)
		isSlice := (leftType == "string" || strings.HasPrefix(leftType, "[]") || strings.HasPrefix(leftType, "slice[") || leftType == "dict" || strings.HasPrefix(leftType, "smap[") || strings.HasPrefix(leftType, "map["))
		if isSlice && (e.Right.Value == "Base" || e.Right.Value == "Cap" || e.Right.Value == "Len") {
			return "word"
		}
		leftType = strings.TrimPrefix(leftType, "*")
		if sInfo, ok := g.lookupStruct(leftType); ok {
			for _, f := range sInfo.fields {
				if f.name == e.Right.Value {
					return g.exprToString(f.typ)
				}
			}
		}
		return "word"
	case *ast.CallExpression:
		if ptrType, ok := e.Function.(*ast.PointerType); ok {
			return g.exprToString(ptrType)
		}
		if id, ok := e.Function.(*ast.Identifier); ok {
			if id.Value == "byte" || id.Value == "uint8" {
				return "byte"
			}
			if id.Value == "word" || id.Value == "uint" || id.Value == "uint16" {
				return "word"
			}
			if id.Value == "string" || id.Value == "strdup" {
				return "string"
			}
			if id.Value == "sys_args" || id.Value == "splitlines" {
				return "slice[string]"
			}
			if id.Value == "make" || id.Value == "makeslice" {
				if len(e.Arguments) > 0 {
					return g.exprToString(e.Arguments[0])
				}
			}
			if id.Value == "int" || id.Value == "int16" {
				panic(fmt.Sprintf("signed 'int' is not supported in MiniGolf-NP (line %d): use unsigned 'word' or 'byte'", e.Token.Line))
			}
			if g.funcPkg != "" && g.funcPkg != "main" {
				if fs, exists := g.funcs[g.funcPkg+"_"+id.Value]; exists && len(fs.ReturnParameters) > 0 {
					return g.exprToString(fs.ReturnParameters[0].Type)
				}
			}
			if fs, exists := g.funcs[id.Value]; exists && len(fs.ReturnParameters) > 0 {
				return g.exprToString(fs.ReturnParameters[0].Type)
			}
		}
		if sel, ok := e.Function.(*ast.SelectorExpression); ok {
			if sel.Right.Value == "Chop" {
				return "string"
			}
			if sel.Right.Value == "Get" {
				elemTypeStr, _ := g.getElemTypeInfo(sel.Left)
				return elemTypeStr
			}
			if pkgIdent, isPkg := sel.Left.(*ast.Identifier); isPkg {
				qname := pkgIdent.Value + "_" + sel.Right.Value
				if fs, exists := g.funcs[qname]; exists && len(fs.ReturnParameters) > 0 {
					return g.exprToString(fs.ReturnParameters[0].Type)
				}
				if fs, exists := g.funcs[pkgIdent.Value+"."+sel.Right.Value]; exists && len(fs.ReturnParameters) > 0 {
					return g.exprToString(fs.ReturnParameters[0].Type)
				}
			}
			rTypeName := g.inferTypeName(sel.Left)
			methodKey := rTypeName + "_" + sel.Right.Value
			if fs, exists := g.funcs[methodKey]; exists && len(fs.ReturnParameters) > 0 {
				return g.exprToString(fs.ReturnParameters[0].Type)
			}
		}
		return "word"
	default:
		if g.inferType(expr) == KindSlice {
			return "string"
		}
		return "word"
	}
}

func (g *Generator) argsToString(args []ast.Expression) string {
	var parts []string
	for _, a := range args {
		parts = append(parts, g.nodeToString(a))
	}
	return strings.Join(parts, ", ")
}
