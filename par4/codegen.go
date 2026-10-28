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
	managedHeap  bool

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

	// Control flow stack for break/continue
	loopStack []loopContext
}

func sanitizeTypeName(s string) string {
	s = strings.TrimPrefix(s, "*")
	s = strings.ReplaceAll(s, "[", "_")
	s = strings.ReplaceAll(s, "]", "")
	s = strings.ReplaceAll(s, ", ", "_")
	s = strings.ReplaceAll(s, ",", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "main_", "")
	s = strings.ReplaceAll(s, "prelude_", "")
	s = strings.ReplaceAll(s, "builtin_", "")
	return s
}

func isSliceTypeName(tStr string) bool {
	tStr = strings.TrimPrefix(tStr, "*")
	return tStr == "string" || tStr == "prelude.string" ||
		strings.HasPrefix(tStr, "[]") ||
		strings.HasPrefix(tStr, "slice[") || strings.HasPrefix(tStr, "prelude.slice[") ||
		strings.HasPrefix(tStr, "slice_") || strings.HasPrefix(tStr, "prelude.slice_")
}

func isPreludeToken(tok *ast.Node) bool {
	return false
}

func isPreludeFile(filename string) bool {
	return strings.HasSuffix(filename, "prelude.golf") ||
		strings.HasSuffix(filename, "prelude.par3") ||
		strings.HasSuffix(filename, "prelude.par4")
}

func (g *Generator) isCompiledPreludeFunc(s *ast.FuncStatement) bool {
	if !g.managedHeap || s.Body == nil || len(s.TypeParameters) > 0 {
		return false
	}
	if s.Receiver != nil {
		return true
	}
	switch s.Name.Value {
	case "init", "init_0", "malloc_init", "malloc", "zalloc", "free", "BytesFree", "strdup", "strfree", "mul_word", "div_word", "mod_word":
		return true
	}
	return false
}

func (g *Generator) isCompiledPreludeVar(s *ast.VarStatement) bool {
	if !g.managedHeap {
		return false
	}
	switch s.Name.Value {
	case "Heap", "base", "freep":
		return true
	}
	return false
}

func (g *Generator) mangleFuncName(fs *ast.FuncStatement, pkg string) string {
	if fs.Receiver != nil {
		rType := sanitizeTypeName(g.exprToString(fs.Receiver.Type))
		if pkg != "" && pkg != "main" && pkg != "prelude" && !strings.HasPrefix(rType, pkg+"_") {
			rType = pkg + "_" + rType
		}
		return rType + "_" + fs.Name.Value
	}
	if pkg != "" && pkg != "main" {
		return pkg + "_" + fs.Name.Value
	}
	return fs.Name.Value
}

func (g *Generator) resolveMethod(recv ast.Expression, method string) (string, *ast.FuncStatement, bool) {
	rTypeName := g.inferTypeName(recv)
	candidates := []string{
		rTypeName + "_" + method,
	}
	if rTypeName == "string" {
		candidates = append(candidates, "slice_byte_"+method)
	}
	if g.funcPkg != "" && g.funcPkg != "main" && g.funcPkg != "prelude" && !strings.HasPrefix(rTypeName, g.funcPkg+"_") {
		candidates = append(candidates, g.funcPkg+"_"+rTypeName+"_"+method)
	}
	for _, cand := range candidates {
		if fs, ok := g.funcs[cand]; ok && fs.Body != nil {
			return cand, fs, true
		}
	}
	return "", nil, false
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
			// Skip uninstantiated generic templates or bodyless declarations
			if len(s.TypeParameters) > 0 || s.Body == nil {
				continue
			}
			// Skip prelude declarations unless needed for managed heap
			if (currentPkg == "prelude" || (s.GetToken() != nil && isPreludeFile(s.GetToken().Filename))) && !g.isCompiledPreludeFunc(s) {
				continue
			}
			g.generateFunc(s, currentPkg)
		}
	}

	return g.out.String()
}

func (g *Generator) collectDeclarations(program *ast.Program) {
	// Pre-pass 0: collect constants across all packages
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
			if currentPkg != "" && currentPkg != "main" {
				g.consts[currentPkg+"_"+s.Name.Value] = s.Value
			}
		}
	}

	if cVal, ok := g.consts["prelude.HEAP_SIZE"]; ok {
		if intLit, isInt := cVal.(*ast.IntegerLiteral); isInt && intLit.Value > 0 {
			g.managedHeap = true
		}
	}

	// Pre-pass 1: collect and resolve struct types
	for pass := 0; pass < 3; pass++ {
		currentPkg = ""
		for _, stmt := range program.Statements {
			if ps, ok := stmt.(*ast.PackageStatement); ok {
				currentPkg = ps.Name.Value
				g.funcPkg = currentPkg
			}
			if s, ok := stmt.(*ast.TypeStatement); ok {
				if len(s.TypeParameters) > 0 {
					continue
				}
				if (currentPkg == "prelude" || (s.GetToken() != nil && isPreludeFile(s.GetToken().Filename))) && !g.managedHeap {
					continue
				}
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
					g.structs[sanitizeTypeName(info.name)] = info
					if currentPkg != "" && currentPkg != "main" {
						g.structs[currentPkg+"."+info.name] = info
						g.structs[currentPkg+"_"+info.name] = info
						g.structs[sanitizeTypeName(currentPkg+"_"+info.name)] = info
					}
				}
			}
		}
	}
	g.funcPkg = ""

	currentPkg = ""
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.PackageStatement:
			currentPkg = s.Name.Value
		case *ast.ConstStatement:
			// Already collected in Pre-pass 0
		case *ast.VarStatement:
			if (currentPkg == "prelude" || (s.GetToken() != nil && isPreludeFile(s.GetToken().Filename))) && !g.isCompiledPreludeVar(s) {
				continue
			}
			g.funcPkg = currentPkg
			qname := s.Name.Value
			kind := KindScalar
			size := 2
			tStr := "word"
			if s.ValueType != nil {
				tStr = g.exprToString(s.ValueType)
				kind = g.getTypeKind(s.ValueType)
				size = g.getTypeSize(s.ValueType)
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
				size = g.getTypeSizeByName(tStr)
				if kind == KindSlice {
					size = 6
				}
			}
			g.globals[qname] = kind
			g.globalSizes[qname] = size
			g.globalTypes[qname] = tStr
			g.funcPkg = ""
		case *ast.FuncStatement:
			if len(s.TypeParameters) > 0 || s.Body == nil {
				continue
			}
			if (currentPkg == "prelude" || (s.GetToken() != nil && isPreludeFile(s.GetToken().Filename))) && !g.isCompiledPreludeFunc(s) {
				continue
			}
			qname := g.mangleFuncName(s, currentPkg)
			g.funcs[qname] = s
			if s.Receiver == nil && currentPkg != "" && currentPkg != "main" {
				g.funcs[currentPkg+"."+s.Name.Value] = s
				if currentPkg == "prelude" {
					g.funcs[s.Name.Value] = s
				}
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
	fnName := g.mangleFuncName(fs, pkg)

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
	g.addLocal("_pbuf", KindBuffer)
	g.localSizes["_pbuf"] = 8
	g.addLocal("_tmp_base", KindScalar)
	g.addLocal("_tmp_cap", KindScalar)
	g.addLocal("_tmp_len", KindScalar)
	g.addLocal("_tmp_addr", KindScalar)

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

	// Compile function body into a temporary buffer first so we only emit
	// synthetic scratch .local directives (_pbuf, _tmp_*, _swap_*) that are actually used.
	savedOut := g.out
	g.out = strings.Builder{}

	// Emit global variable initializers if this is the entry function
	if isEntry {
		currPkg := ""
		for _, stmt := range g.program.Statements {
			switch s := stmt.(type) {
			case *ast.PackageStatement:
				currPkg = s.Name.Value
			case *ast.VarStatement:
				if (currPkg == "prelude" || (s.GetToken() != nil && isPreludeFile(s.GetToken().Filename))) && !g.isCompiledPreludeVar(s) {
					continue
				}
				if s.Value != nil {
					qname := s.Name.Value
					if _, ok := g.globals[qname]; !ok && currPkg != "" && currPkg != "main" {
						qname = currPkg + "_" + s.Name.Value
					}
					g.emitComment("init global %s", qname)
					g.compileExpression(s.Value)
					g.emit("    STORE_GLOBAL %s", qname)
				}
			}
		}
		initFn := ""
		if _, hasInit0 := g.funcs["prelude_init_0"]; hasInit0 {
			initFn = "prelude_init_0"
		} else if _, hasInit := g.funcs["prelude_init"]; hasInit {
			initFn = "prelude_init"
		}
		if initFn != "" {
			alreadyCallsInit0 := false
			if fs.Body != nil && len(fs.Body.Statements) > 0 {
				if es, ok := fs.Body.Statements[0].(*ast.ExpressionStatement); ok {
					if call, ok2 := es.Expression.(*ast.CallExpression); ok2 {
						if id, ok3 := call.Function.(*ast.Identifier); ok3 && (id.Value == "init_0" || id.Value == "prelude.init_0" || id.Value == "init" || id.Value == "prelude.init") {
							alreadyCallsInit0 = true
						}
					}
				}
			}
			if !alreadyCallsInit0 {
				g.emit("    CALL %s", initFn)
			}
		}
	}

	// Emit function body statements
	if fs.Body != nil {
		g.compileBlock(fs.Body)
	}

	// Default return safeguard
	g.emitLocalDestructors()
	g.emit("    RET_VOID")
	g.emit(".endfunction\n")

	bodyStr := g.out.String()
	g.out = savedOut

	// Emit .local directives (skipping unused synthetic '_' scratch locals)
	emittedLocals := 0
	for _, lName := range g.localOrder {
		if strings.HasPrefix(lName, "_") && !strings.Contains(bodyStr, " "+lName+"\n") {
			continue
		}
		emittedLocals++
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
	if len(g.paramOrder) > 0 || emittedLocals > 0 {
		g.emit("")
	}
	g.out.WriteString(bodyStr)
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
							if arr, ok := lit.Type.(*ast.ArrayType); ok && arr.Length != nil {
								kind = KindBuffer
								tStr = g.exprToString(arr)
							}
						}
						if kind != KindBuffer {
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
						tStr, _ = g.getElemTypeInfo(s.RangeValue)
						if elemKind == KindSlice && tStr == "word" {
							tStr = "string"
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
					_, exists1 := g.funcs[id.Value]
					_, exists2 := g.funcs["prelude_"+id.Value]
					if !exists1 && !exists2 {
						return
					}
				}
			}
		}

		g.compileExpression(s.Expression)
		// Discard unused expression result from stack
		if !g.isVoidCall(s.Expression) {
			if call, ok := s.Expression.(*ast.CallExpression); ok {
				if id, ok2 := call.Function.(*ast.Identifier); ok2 && (id.Value == "hatvan_trap" || id.Value == "os_call" || id.Value == "sys_trap") {
					g.emit("    POP")
					g.emit("    POP")
					g.emit("    POP")
					g.emit("    POP")
					return
				}
			}
			kind := g.inferType(s.Expression)
			if kind == KindSlice {
				g.emit("    POP_SLICE")
			} else if kind > KindScalar {
				for w := 0; w < int(kind)/2; w++ {
					g.emit("    POP")
				}
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
			valKind := g.inferType(rhs)
			valSize := 2
			if isByte {
				valSize = 1
			} else if valKind >= KindSlice {
				valSize = int(valKind)
			}
			g.compileExpression(rhs)
			g.compileExpression(pref.Right)
			if valSize == 1 {
				g.emit("    SWAP")
				g.emit("    POKE1")
			} else if valSize == 2 {
				g.emit("    SWAP")
				g.emit("    POKE2")
			} else {
				g.emit("    STORE_LOCAL _tmp_addr")
				for off := valSize - 2; off >= 0; off -= 2 {
					g.emit("    LOAD_LOCAL _tmp_addr")
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    SWAP")
					g.emit("    POKE2")
				}
			}
			return
		}

		if ptrType, ok := lhs.(*ast.PointerType); ok {
			isByte := g.isBytePointer(ptrType.Elt)
			valKind := g.inferType(rhs)
			valSize := 2
			if isByte {
				valSize = 1
			} else if valKind >= KindSlice {
				valSize = int(valKind)
			}
			g.compileExpression(rhs)
			g.compileExpression(ptrType.Elt)
			if valSize == 1 {
				g.emit("    SWAP")
				g.emit("    POKE1")
			} else if valSize == 2 {
				g.emit("    SWAP")
				g.emit("    POKE2")
			} else {
				g.emit("    STORE_LOCAL _tmp_addr")
				for off := valSize - 2; off >= 0; off -= 2 {
					g.emit("    LOAD_LOCAL _tmp_addr")
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    SWAP")
					g.emit("    POKE2")
				}
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
			g.addLocal(tempVars[i], KindScalar)
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
			if g.locals[target.Value] == KindBuffer {
				g.emit("    ADDR_OF_LOCAL %s", target.Value)
				g.emit("    PUSH_I16 %d", g.localSizes[target.Value])
				g.emit("    MEM_COPY")
				return
			}
			g.emit("    STORE_LOCAL %s", target.Value)
			return
		}
		if _, isParam := g.params[target.Value]; isParam {
			if g.params[target.Value] == KindBuffer {
				g.emit("    ADDR_OF_LOCAL %s", target.Value)
				g.emit("    PUSH_I16 %d", g.paramSizes[target.Value])
				g.emit("    MEM_COPY")
				return
			}
			g.emit("    STORE_LOCAL %s", target.Value)
			return
		}
		if qname, isGlobal := g.resolveGlobal(target.Value); isGlobal {
			if g.globals[qname] == KindBuffer {
				g.emit("    ADDR_OF_GLOBAL %s", qname)
				g.emit("    PUSH_I16 %d", g.globalSizes[qname])
				g.emit("    MEM_COPY")
				return
			}
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
		} else if size >= 4 && size%2 == 0 {
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
		} else {
			g.emit("    PUSH_I16 %d", size)
			g.emit("    MEM_COPY")
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
		case "&^":
			g.emit("    BIT_NOT")
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
		case "&^":
			g.emit("    BIT_NOT")
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
		case "&^":
			g.emit("    BIT_NOT")
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
			_, elemSize := g.getElemType(s.RangeValue)
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

			if elemSize >= 4 && elemSize%2 == 0 {
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

func (g *Generator) emitLocalDestructors() {
	for i := len(g.localOrder) - 1; i >= 0; i-- {
		lName := g.localOrder[i]
		if strings.HasPrefix(lName, "_") {
			continue
		}
		tStr := g.localTypes[lName]
		if tStr == "" || strings.HasPrefix(tStr, "*") {
			continue
		}
		dtorName := sanitizeTypeName(tStr) + "_destructor"
		if _, ok := g.funcs[dtorName]; !ok {
			if g.funcPkg != "" && g.funcPkg != "main" {
				cand := g.funcPkg + "_" + dtorName
				if _, ok2 := g.funcs[cand]; ok2 {
					dtorName = cand
				} else {
					continue
				}
			} else {
				continue
			}
		}
		g.emit("    ADDR_OF_LOCAL %s", lName)
		g.emit("    CALL %s", dtorName)
	}
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
		g.emitLocalDestructors()
		g.emit("    RET_VOID")
		return
	}

	if len(s.ReturnValues) == 1 {
		rv := s.ReturnValues[0]
		g.compileExpression(rv)
		g.emitLocalDestructors()
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
	g.emitLocalDestructors()
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
			_, elemSize := g.getElemType(e.Left)
			g.compileElemAddress(e.Left, e.Indices[0], elemSize)
			if elemSize == 1 {
				g.emit("    PEEK1")
			} else if elemSize == 2 {
				g.emit("    PEEK2")
			} else if elemSize >= 4 && elemSize%2 == 0 {
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
		if _, isCall := e.Left.(*ast.CallExpression); isCall && g.inferType(e.Left) == KindSlice {
			g.compileExpression(e.Left)
			switch e.Right.Value {
			case "Base":
				g.emit("    POP")
				g.emit("    POP")
			case "Cap":
				g.emit("    SLICE_CAP")
			case "Len":
				g.emit("    SLICE_LEN")
			default:
				panic(fmt.Sprintf("unknown slice selector .%s on call expression", e.Right.Value))
			}
			return
		}
		_, size := g.compileAddress(e)
		if size == 1 {
			g.emit("    PEEK1")
		} else if size == 2 {
			g.emit("    PEEK2")
		} else if size >= 4 && size%2 == 0 {
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

	case *ast.CompositeLit:
		g.compileCompositeLit(e)

	default:
		panic(fmt.Sprintf("unsupported expression type in MiniGolf-NP: %T at line %d", expr, expr.GetToken().Line))
	}
}

func (g *Generator) compileCompositeLit(e *ast.CompositeLit) {
	tStr := g.exprToString(e.Type)
	fieldVals := make(map[string]ast.Expression)
	var posVals []ast.Expression
	for _, el := range e.Elements {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok2 := kv.Key.(*ast.Identifier); ok2 {
				fieldVals[id.Value] = kv.Value
			}
		} else {
			posVals = append(posVals, el)
		}
	}

	if isSliceTypeName(tStr) {
		if bExpr, ok := fieldVals["Base"]; ok {
			g.compileExpression(bExpr)
		} else if len(posVals) > 0 {
			g.compileExpression(posVals[0])
		} else {
			g.emit("    PUSH_0")
		}
		if cExpr, ok := fieldVals["Cap"]; ok {
			g.compileExpression(cExpr)
		} else if len(posVals) > 1 {
			g.compileExpression(posVals[1])
		} else {
			g.emit("    PUSH_0")
		}
		if lExpr, ok := fieldVals["Len"]; ok {
			g.compileExpression(lExpr)
		} else if len(posVals) > 2 {
			g.compileExpression(posVals[2])
		} else {
			g.emit("    PUSH_0")
		}
		g.emit("    SLICE_NEW")
		return
	}

	if sInfo, ok := g.lookupStruct(tStr); ok {
		for idx, f := range sInfo.fields {
			if valExpr, hasKey := fieldVals[f.name]; hasKey {
				g.compileExpression(valExpr)
			} else if idx < len(posVals) {
				g.compileExpression(posVals[idx])
			} else if f.size == 6 {
				g.emit("    PUSH_NIL_SLICE")
			} else {
				for w := 0; w < (f.size+1)/2; w++ {
					g.emit("    PUSH_0")
				}
			}
		}
		return
	}

	panic(fmt.Sprintf("unsupported composite literal type %q at line %d", tStr, e.Token.Line))
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
		if g.locals[name] == KindBuffer {
			g.emit("    ADDR_OF_LOCAL %s", name)
			return
		}
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
	case "&^":
		g.emit("    BIT_NOT")
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
	} else if id, ok := e.Left.(*ast.Identifier); ok && g.locals[id.Value] == KindBuffer {
		g.emit("    ADDR_OF_LOCAL %s", id.Value)
		sz := g.localSizes[id.Value]
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

		// Check if sel.Left is an imported package name (e.g. btree.New)
		if pkgIdent, isPkg := recv.(*ast.Identifier); isPkg {
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

		// Check if a compiled method exists in g.funcs first
		if targetMethod, fs, ok := g.resolveMethod(recv, method); ok {
			passAddr := false
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

		// Builtin slice methods on receiver
		switch method {
		case "Len":
			g.compileExpression(recv)
			g.emit("    SLICE_LEN")
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

	// 2. Generic/index call: e.g. sizeof[T](), peek[T](), poke[T](), or smap.New[string](8)
	if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
		genName := ""
		if id, ok2 := idxExpr.Left.(*ast.Identifier); ok2 {
			genName = id.Value
			if id.ShortName != "" {
				genName = id.ShortName
			}
		} else if sel, ok2 := idxExpr.Left.(*ast.SelectorExpression); ok2 {
			genName = sel.Right.Value
		}

		switch genName {
		case "sizeof":
			sz := g.getTypeSize(idxExpr.Indices[0])
			g.emit("    PUSH_I16 %d", sz)
			return

		case "peek":
			sz := g.getTypeSize(idxExpr.Indices[0])
			g.compileExpression(call.Arguments[0])
			if sz == 1 {
				g.emit("    PEEK1")
			} else if sz == 2 {
				g.emit("    PEEK2")
			} else {
				g.emit("    STORE_LOCAL _tmp_addr")
				for off := 0; off < sz; off += 2 {
					g.emit("    LOAD_LOCAL _tmp_addr")
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    PEEK2")
				}
			}
			return

		case "poke":
			sz := g.getTypeSize(idxExpr.Indices[0])
			if sz == 1 {
				g.compileExpression(call.Arguments[0])
				g.compileExpression(call.Arguments[1])
				g.emit("    POKE1")
			} else if sz == 2 {
				g.compileExpression(call.Arguments[0])
				g.compileExpression(call.Arguments[1])
				g.emit("    POKE2")
			} else {
				g.compileExpression(call.Arguments[1])
				g.compileExpression(call.Arguments[0])
				g.emit("    STORE_LOCAL _tmp_addr")
				for off := sz - 2; off >= 0; off -= 2 {
					g.emit("    LOAD_LOCAL _tmp_addr")
					if off != 0 {
						g.emit("    PUSH_I16 %d", off)
						g.emit("    ADD")
					}
					g.emit("    SWAP")
					g.emit("    POKE2")
				}
			}
			return

		case "makeslice":
			if g.managedHeap {
				sz := g.getTypeSize(idxExpr.Indices[0])
				g.compileExpression(call.Arguments[0])
				g.emit("    STORE_LOCAL _tmp_len")
				g.emit("    LOAD_LOCAL _tmp_len")
				g.emit("    PUSH_I16 %d", sz)
				g.emit("    MUL")
				g.emit("    CALL prelude_zalloc")
				g.emit("    LOAD_LOCAL _tmp_len")
				g.emit("    LOAD_LOCAL _tmp_len")
				g.emit("    SLICE_NEW")
				return
			}

		case "freeslice":
			if g.managedHeap {
				g.compileExpression(call.Arguments[0])
				g.emit("    POP")
				g.emit("    POP")
				g.emit("    CALL prelude_free")
				return
			}

		case "clear":
			sz := g.getTypeSize(idxExpr.Indices[0])
			g.compileExpression(call.Arguments[0])
			g.emit("    PUSH_0")
			g.emit("    PUSH_I16 %d", sz)
			g.emit("    MEM_SET")
			return
		}
	}

	// 2b. Type cast or composite conversion: (*Node)(expr) or slice[byte](expr)
	if _, ok := call.Function.(*ast.PointerType); ok {
		g.compileExpression(call.Arguments[0])
		if g.inferType(call.Arguments[0]) == KindSlice {
			g.emit("    POP")
			g.emit("    POP")
		}
		return
	}

	// 3. Identifier function call
	id, ok := call.Function.(*ast.Identifier)
	if !ok {
		panic(fmt.Sprintf("unsupported callee expression: %T at line %d", call.Function, call.Token.Line))
	}

	fnName := id.Value
	if id.ShortName != "" && (id.Package == "prelude" || id.Package == "builtin" || id.Package == "main") {
		fnName = id.ShortName
	}

	if g.managedHeap {
		if _, hasPreludeFn := g.funcs["prelude_"+fnName]; hasPreludeFn {
			switch fnName {
			case "init_0", "malloc_init", "malloc", "zalloc", "free", "BytesFree", "strdup", "strfree", "mul_word", "div_word", "mod_word":
				for _, arg := range call.Arguments {
					g.compileExpression(arg)
				}
				g.emit("    CALL prelude_%s", fnName)
				return
			}
		}
	}

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

	case "peekb", "peek_byte", "vpeekb":
		g.compileExpression(call.Arguments[0])
		g.emit("    PEEK1")
		return

	case "peek", "peekw", "peek_word", "vpeekw":
		g.compileExpression(call.Arguments[0])
		g.emit("    PEEK2")
		return

	case "pokeb", "poke_byte", "vpokeb":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    POKE1")
		return

	case "poke", "pokew", "poke_word", "vpokew":
		g.compileExpression(call.Arguments[0])
		g.compileExpression(call.Arguments[1])
		g.emit("    POKE2")
		return

	case "panic":
		if len(call.Arguments) > 0 {
			g.compileExpression(call.Arguments[0])
		} else {
			g.emit("    PUSH_STR \"panic\"")
		}
		g.emit("    PANIC")
		return

	case "sys_exit", "exit":
		if len(call.Arguments) > 0 {
			g.compileExpression(call.Arguments[0])
		} else {
			g.emit("    PUSH_0")
		}
		g.emit("    SYS_EXIT")
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

	case "byte":
		g.compileExpression(call.Arguments[0])
		g.emit("    PUSH_I16 255")
		g.emit("    BIT_AND")
		return

	case "int", "int16":
		panic(fmt.Sprintf("signed 'int' is not supported in MiniGolf-NP (line %d): use unsigned 'word' or 'byte'", call.Token.Line))

	case "word", "uint", "uint16":
		g.compileExpression(call.Arguments[0])
		if g.inferType(call.Arguments[0]) == KindSlice {
			g.emit("    POP")
			g.emit("    POP")
		}
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
	if idxExpr, ok := call.Function.(*ast.IndexExpression); ok {
		genName := ""
		if id, ok2 := idxExpr.Left.(*ast.Identifier); ok2 {
			genName = id.Value
			if id.ShortName != "" {
				genName = id.ShortName
			}
		} else if sel, ok2 := idxExpr.Left.(*ast.SelectorExpression); ok2 {
			genName = sel.Right.Value
		}
		if genName == "poke" || genName == "freeslice" || genName == "clear" {
			return true
		}
	}
	if id, ok := call.Function.(*ast.Identifier); ok {
		fnName := id.Value
		if id.ShortName != "" && (id.Package == "prelude" || id.Package == "builtin" || id.Package == "main") {
			fnName = id.ShortName
		}
		switch fnName {
		case "print", "println", "free", "strfree", "malloc_init", "init_0", "poke", "pokeb", "pokew", "poke_byte", "poke_word", "vpokeb", "vpokew", "memcpy", "mem_copy", "memset", "mem_set", "file_close", "sys_exit", "exit", "panic":
			return true
		}
		if id.Package != "" && id.Package != "main" && id.Package != "prelude" {
			target := id.Package + "_" + id.ShortName
			if fs, exists := g.funcs[target]; exists && len(fs.ReturnParameters) == 0 {
				return true
			}
			dotTarget := id.Package + "." + id.ShortName
			if fs, exists := g.funcs[dotTarget]; exists && len(fs.ReturnParameters) == 0 {
				return true
			}
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
		if id, ok := sel.Left.(*ast.Identifier); ok {
			key := id.Value + "_" + sel.Right.Value
			if fs, exists := g.funcs[key]; exists {
				return len(fs.ReturnParameters) == 0
			}
			key2 := id.Value + "." + sel.Right.Value
			if fs, exists := g.funcs[key2]; exists {
				return len(fs.ReturnParameters) == 0
			}
		}
		if _, fs, ok := g.resolveMethod(sel.Left, sel.Right.Value); ok {
			return len(fs.ReturnParameters) == 0
		}
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
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return int(e.Value)
	case *ast.Identifier:
		if cVal, exists := g.consts[e.Value]; exists {
			return g.evalIntConst(cVal)
		}
		if g.funcPkg != "" {
			if cVal, exists := g.consts[g.funcPkg+"."+e.Value]; exists {
				return g.evalIntConst(cVal)
			}
			if cVal, exists := g.consts[g.funcPkg+"_"+e.Value]; exists {
				return g.evalIntConst(cVal)
			}
		}
	case *ast.SelectorExpression:
		qname := g.exprToString(e)
		if cVal, exists := g.consts[qname]; exists {
			return g.evalIntConst(cVal)
		}
		underName := strings.ReplaceAll(qname, ".", "_")
		if cVal, exists := g.consts[underName]; exists {
			return g.evalIntConst(cVal)
		}
		if cVal, exists := g.consts[e.Right.Value]; exists {
			return g.evalIntConst(cVal)
		}
	case *ast.InfixExpression:
		left := g.evalIntConst(e.Left)
		right := g.evalIntConst(e.Right)
		switch e.Operator {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			if right != 0 {
				return left / right
			}
		case "<<":
			return left << right
		case ">>":
			return left >> right
		case "|":
			return left | right
		case "&":
			return left & right
		}
	case *ast.PrefixExpression:
		if e.Operator == "-" {
			return -g.evalIntConst(e.Right)
		}
		if e.Operator == "+" {
			return g.evalIntConst(e.Right)
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
	sanitized := sanitizeTypeName(name)
	if g.funcPkg != "" && g.funcPkg != "main" && !strings.Contains(name, ".") && !strings.Contains(name, "_") {
		if s, ok := g.structs[g.funcPkg+"."+name]; ok {
			return &s, true
		}
		if s, ok := g.structs[g.funcPkg+"_"+name]; ok {
			return &s, true
		}
		if s, ok := g.structs[sanitizeTypeName(g.funcPkg+"_"+name)]; ok {
			return &s, true
		}
	}
	if s, ok := g.structs[name]; ok {
		return &s, true
	}
	if s, ok := g.structs[sanitized]; ok {
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
	if name == "byte" || name == "uint8" || name == "bool" || name == "prelude.byte" || name == "prelude.bool" {
		return 1
	}
	if isSliceTypeName(name) {
		return 6
	}
	if !g.managedHeap && (strings.HasPrefix(name, "smap[") || strings.HasPrefix(name, "map[") || name == "dict") {
		return 6
	}
	if strings.HasPrefix(name, "[") && strings.Contains(name, "]") {
		idx := strings.Index(name, "]")
		countStr := name[1:idx]
		elemStr := name[idx+1:]
		if count, err := strconv.Atoi(countStr); err == nil {
			return count * g.getTypeSizeByName(elemStr)
		}
		if cVal, exists := g.consts[countStr]; exists {
			count := g.evalIntConst(cVal)
			if count > 0 {
				return count * g.getTypeSizeByName(elemStr)
			}
		}
		if g.funcPkg != "" {
			if cVal, exists := g.consts[g.funcPkg+"."+countStr]; exists {
				count := g.evalIntConst(cVal)
				if count > 0 {
					return count * g.getTypeSizeByName(elemStr)
				}
			}
			if cVal, exists := g.consts[g.funcPkg+"_"+countStr]; exists {
				count := g.evalIntConst(cVal)
				if count > 0 {
					return count * g.getTypeSizeByName(elemStr)
				}
			}
		}
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
		if isSliceTypeName(name) {
			return KindSlice
		}
		if name == "noreturn" || name == "void" {
			return KindVoid
		}
		if sInfo, ok := g.lookupStruct(g.exprToString(t)); ok && sInfo.size > 0 {
			return TypeKind(sInfo.size)
		}
		if sInfo, ok := g.lookupStruct(name); ok && sInfo.size > 0 {
			return TypeKind(sInfo.size)
		}
		return KindScalar
	case *ast.SelectorExpression:
		sName := g.exprToString(t)
		if isSliceTypeName(sName) {
			return KindSlice
		}
		if sInfo, ok := g.lookupStruct(sName); ok && sInfo.size > 0 {
			return TypeKind(sInfo.size)
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
		sName := g.exprToString(t)
		if isSliceTypeName(sName) {
			return KindSlice
		}
		if sInfo, ok := g.lookupStruct(sName); ok && sInfo.size > 0 {
			return TypeKind(sInfo.size)
		}
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
		if isSliceTypeName(t.Value) {
			return 6
		}
		if sInfo, ok := g.lookupStruct(g.exprToString(t)); ok {
			return sInfo.size
		}
		if sInfo, ok := g.lookupStruct(t.Value); ok {
			return sInfo.size
		}
		return 2
	case *ast.SelectorExpression:
		sName := g.exprToString(t)
		if isSliceTypeName(sName) {
			return 6
		}
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
		sName := g.exprToString(t)
		if isSliceTypeName(sName) {
			return 6
		}
		if sInfo, ok := g.lookupStruct(sName); ok {
			return sInfo.size
		}
		return 6
	default:
		return 2
	}
}

func (g *Generator) getElemTypeInfo(expr ast.Expression) (string, int) {
	if expr == nil {
		return "word", 2
	}
	tStr := strings.TrimPrefix(g.getArgTypeString(expr), "*")
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
	for _, prefix := range []string{"prelude.slice[", "slice[", "mem.List[", "List["} {
		if strings.HasPrefix(tStr, prefix) && strings.HasSuffix(tStr, "]") {
			elemTypeName := tStr[len(prefix) : len(tStr)-1]
			elemSz := g.getTypeSizeByName(elemTypeName)
			return elemTypeName, elemSz
		}
	}
	for _, prefix := range []string{"prelude.slice_", "slice_", "mem.List_", "mem_List_", "List_"} {
		if strings.HasPrefix(tStr, prefix) {
			elemTypeName := strings.TrimPrefix(tStr, prefix)
			elemSz := g.getTypeSizeByName(elemTypeName)
			return elemTypeName, elemSz
		}
	}
	if strings.Contains(tStr, "slice[string]") || strings.Contains(tStr, "[]string") || strings.HasSuffix(tStr, "]string") {
		return "string", 6
	}
	if idx := strings.Index(tStr, "]"); idx >= 0 && idx < len(tStr)-1 {
		elemTypeName := tStr[idx+1:]
		elemSz := g.getTypeSizeByName(elemTypeName)
		return elemTypeName, elemSz
	}
	if tStr == "string" || tStr == "prelude.string" || strings.Contains(tStr, "byte") || strings.Contains(tStr, "uint8") {
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
	if elemSz >= 4 && elemSz%2 == 0 {
		return TypeKind(elemSz), elemSz
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
		if resolved := e.GetResolvedType(); resolved != nil {
			tStr = g.exprToString(resolved)
		}
		if tStr == "" {
			if sel, ok := e.Function.(*ast.SelectorExpression); ok {
				if pkgIdent, ok2 := sel.Left.(*ast.Identifier); ok2 {
					qname := pkgIdent.Value + "_" + sel.Right.Value
					if fs, exists := g.funcs[qname]; exists && len(fs.ReturnParameters) > 0 {
						tStr = g.exprToString(fs.ReturnParameters[0].Type)
					} else if fs, exists2 := g.funcs[pkgIdent.Value+"."+sel.Right.Value]; exists2 && len(fs.ReturnParameters) > 0 {
						tStr = g.exprToString(fs.ReturnParameters[0].Type)
					}
				}
				if tStr == "" {
					rTypeName := g.inferTypeName(sel.Left)
					methodKey := rTypeName + "_" + sel.Right.Value
					if fs, exists := g.funcs[methodKey]; exists && len(fs.ReturnParameters) > 0 {
						tStr = g.exprToString(fs.ReturnParameters[0].Type)
					}
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
		}
		return tStr, 2

	case *ast.SelectorExpression:
		fieldName := e.Right.Value
		leftTypeStr := g.getArgTypeString(e.Left)
		isSlice := isSliceTypeName(leftTypeStr) || (!g.managedHeap && (leftTypeStr == "dict" || strings.HasPrefix(leftTypeStr, "smap[") || strings.HasPrefix(leftTypeStr, "map[")))
		if isSlice && (fieldName == "Base" || fieldName == "Cap" || fieldName == "Len") {
			g.compileAddress(e.Left)
			if strings.HasPrefix(leftTypeStr, "*") {
				if _, isCall := e.Left.(*ast.CallExpression); !isCall {
					g.emit("    PEEK2")
				}
			}
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
			if _, isCall := e.Left.(*ast.CallExpression); !isCall {
				g.emit("    PEEK2")
			}
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
			if g.locals[id.Value] != KindBuffer {
				g.emit("    PEEK2")
			}
		} else if _, isParam := g.params[id.Value]; isParam {
			g.emit("    ADDR_OF_LOCAL %s", id.Value)
			if g.params[id.Value] != KindBuffer {
				g.emit("    PEEK2")
			}
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
		isSlice := isSliceTypeName(leftType) || (!g.managedHeap && (leftType == "dict" || strings.HasPrefix(leftType, "smap[") || strings.HasPrefix(leftType, "map[")))
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
	case *ast.CompositeLit:
		return g.getTypeKind(e.Type)
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
		genName := ""
		if id, ok2 := idxExpr.Left.(*ast.Identifier); ok2 {
			genName = id.Value
			if id.ShortName != "" {
				genName = id.ShortName
			}
		} else if sel, ok2 := idxExpr.Left.(*ast.SelectorExpression); ok2 {
			genName = sel.Right.Value
		}
		switch genName {
		case "New", "makeslice":
			return KindSlice
		case "peek":
			return g.getTypeKind(idxExpr.Indices[0])
		case "sizeof":
			return KindScalar
		}
	}

	if id, ok := call.Function.(*ast.Identifier); ok {
		fnName := id.Value
		if id.ShortName != "" && (id.Package == "prelude" || id.Package == "builtin" || id.Package == "main") {
			fnName = id.ShortName
		}
		if fnName == "file_readline" {
			if retIdx == 0 {
				return KindSlice
			}
			return KindScalar
		}
		switch fnName {
		case "rstrip", "lstrip", "strip", "replace_ident", "splitlines", "sys_args", "make", "makeslice", "strdup", "map_str":
			return KindSlice
		case "len", "cap", "find", "startswith", "endswith", "strcmp", "streq", "file_open", "file_create", "file_open_read", "file_open_write", "file_read", "file_write", "file_close", "os_isfile", "os_makedirs", "alloc", "malloc", "zalloc", "free", "BytesFree", "peek", "peekw", "peekb", "peek_word", "peek_byte", "vpeekb", "vpeekw", "poke", "pokew", "pokeb", "vpokeb", "vpokew", "poke_word", "poke_byte", "byte", "word", "int", "uint", "bool", "map_new", "map_get", "map_put", "map_count", "memcpy", "mem_copy", "memset", "mem_set", "hatvan_trap", "sys_poll_flag", "sys_dma_copy":
			return KindScalar
		}

		if id.Package != "" && id.Package != "main" && id.Package != "prelude" {
			target := id.Package + "_" + id.ShortName
			if fs, exists := g.funcs[target]; exists {
				if retIdx < len(fs.ReturnParameters) {
					return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
				}
			}
			dotTarget := id.Package + "." + id.ShortName
			if fs, exists := g.funcs[dotTarget]; exists {
				if retIdx < len(fs.ReturnParameters) {
					return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
				}
			}
		}
		if fs, exists := g.funcs[id.Value]; exists {
			if retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
		}
		if fs, exists := g.funcs[fnName]; exists {
			if retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
		}
	}

	if sel, ok := call.Function.(*ast.SelectorExpression); ok {
		if pkgIdent, isPkg := sel.Left.(*ast.Identifier); isPkg {
			qname := pkgIdent.Value + "_" + sel.Right.Value
			if fs, exists := g.funcs[qname]; exists && retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
			if fs, exists := g.funcs[pkgIdent.Value+"."+sel.Right.Value]; exists && retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
		}
		if _, fs, ok := g.resolveMethod(sel.Left, sel.Right.Value); ok {
			if retIdx < len(fs.ReturnParameters) {
				return g.getTypeKind(fs.ReturnParameters[retIdx].Type)
			}
			return KindVoid
		}
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

func (g *Generator) inferTypeName(expr ast.Expression) string {
	if expr == nil {
		return "object"
	}
	rawType := sanitizeTypeName(g.getArgTypeString(expr))
	if g.funcPkg != "" && g.funcPkg != "main" && g.funcPkg != "prelude" && !strings.Contains(rawType, "_") {
		candidate := g.funcPkg + "_" + rawType
		if _, ok := g.structs[candidate]; ok {
			return candidate
		}
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
		if isPrintln {
			g.emit("    PUSH_STR \"\"")
			g.emit("    IO_PRINT")
		}
		return
	}

	cmdName := "print"
	if isPrintln {
		cmdName = "println"
	}
	g.emitComment("%s(%s)", cmdName, g.argsToString(call.Arguments))

	if isPrintln && nArgs == 1 {
		arg := call.Arguments[0]
		tStr := g.getArgTypeString(arg)
		if tStr == "string" || g.inferType(arg) == KindSlice {
			g.compileExpression(arg)
			g.emit("    IO_PRINT")
			return
		}
	}

	for i, arg := range call.Arguments {
		if i > 0 {
			g.emit("    PUSH_STR \" \"")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    STORE_LOCAL _tmp_base")
			g.emit("    PUSH_1")
			g.emit("    PUSH_0")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    PUSH_1")
			g.emit("    PUSH_0")
			g.emit("    HATVAN_TRAP I$Write")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
		}

		tStr := g.getArgTypeString(arg)
		if g.inferType(arg) == KindSlice && tStr != "string" {
			tStr = "string"
		}

		if tStr == "string" {
			g.compileExpression(arg)
			g.emit("    STORE_LOCAL _tmp_len")
			g.emit("    POP")
			g.emit("    STORE_LOCAL _tmp_base")
			g.emit("    PUSH_1")
			g.emit("    PUSH_0")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    LOAD_LOCAL _tmp_len")
			g.emit("    PUSH_0")
			g.emit("    HATVAN_TRAP I$Write")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
		} else if tStr == "bool" {
			lblFalse := g.nextLabel("L_pbool_false")
			lblDone := g.nextLabel("L_pbool_done")
			g.compileExpression(arg)
			g.emit("    JUMP_IF_FALSE %s", lblFalse)
			g.emit("    PUSH_STR \"true\"")
			g.emit("    JUMP %s", lblDone)
			g.emit("%s:", lblFalse)
			g.emit("    PUSH_STR \"false\"")
			g.emit("%s:", lblDone)
			g.emit("    STORE_LOCAL _tmp_len")
			g.emit("    POP")
			g.emit("    STORE_LOCAL _tmp_base")
			g.emit("    PUSH_1")
			g.emit("    PUSH_0")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    LOAD_LOCAL _tmp_len")
			g.emit("    PUSH_0")
			g.emit("    HATVAN_TRAP I$Write")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
		} else {
			lblLoop := g.nextLabel("L_pnum_loop")
			g.compileExpression(arg)
			if tStr == "byte" || tStr == "uint8" {
				g.emit("    PUSH_U8 255")
				g.emit("    BIT_AND")
			}
			g.emit("    STORE_LOCAL _tmp_base")
			g.emit("    ADDR_OF_LOCAL _pbuf")
			g.emit("    PUSH_U8 6")
			g.emit("    ADD")
			g.emit("    STORE_LOCAL _tmp_addr")
			g.emit("%s:", lblLoop)
			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    PUSH_1")
			g.emit("    SUB")
			g.emit("    STORE_LOCAL _tmp_addr")
			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    PUSH_U8 10")
			g.emit("    MOD")
			g.emit("    PUSH_U8 48")
			g.emit("    ADD")
			g.emit("    POKE1")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    PUSH_U8 10")
			g.emit("    DIV")
			g.emit("    STORE_LOCAL _tmp_base")
			g.emit("    LOAD_LOCAL _tmp_base")
			g.emit("    JUMP_IF_TRUE %s", lblLoop)
			g.emit("    PUSH_1")
			g.emit("    PUSH_0")
			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    ADDR_OF_LOCAL _pbuf")
			g.emit("    PUSH_U8 6")
			g.emit("    ADD")
			g.emit("    LOAD_LOCAL _tmp_addr")
			g.emit("    SUB")
			g.emit("    PUSH_0")
			g.emit("    HATVAN_TRAP I$Write")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
			g.emit("    POP")
		}
	}

	if isPrintln {
		g.emit("    PUSH_STR \"\"")
		g.emit("    IO_PRINT")
	}
}

func (g *Generator) getArgTypeString(expr ast.Expression) string {
	if expr == nil {
		return "word"
	}
	if id, ok := expr.(*ast.Identifier); ok {
		if id.Value == "true" || id.Value == "false" {
			return "bool"
		}
		if t, ok := g.localTypes[id.Value]; ok && t != "" {
			return t
		}
		if t, ok := g.paramTypes[id.Value]; ok && t != "" {
			return t
		}
		if qname, ok := g.resolveGlobal(id.Value); ok && g.globalTypes[qname] != "" {
			return g.globalTypes[qname]
		}
	}
	if resolved := expr.GetResolvedType(); resolved != nil {
		tStr := g.exprToString(resolved)
		if tStr != "" && tStr != "int" && tStr != "uint" && tStr != "T" && tStr != "DT" && tStr != "UnknownType" && !strings.Contains(tStr, "[T]") && !strings.HasSuffix(tStr, "_T") && !strings.Contains(tStr, "struct") {
			return tStr
		}
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
		isSlice := isSliceTypeName(leftType) || (!g.managedHeap && (leftType == "dict" || strings.HasPrefix(leftType, "smap[") || strings.HasPrefix(leftType, "map[")))
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
	case *ast.CompositeLit:
		return g.exprToString(e.Type)
	case *ast.CallExpression:
		if ptrType, ok := e.Function.(*ast.PointerType); ok {
			return g.exprToString(ptrType)
		}
		if idxExpr, ok := e.Function.(*ast.IndexExpression); ok {
			genName := ""
			if id, ok2 := idxExpr.Left.(*ast.Identifier); ok2 {
				genName = id.Value
				if id.ShortName != "" {
					genName = id.ShortName
				}
			} else if sel, ok2 := idxExpr.Left.(*ast.SelectorExpression); ok2 {
				genName = sel.Right.Value
			}
			switch genName {
			case "peek":
				return g.exprToString(idxExpr.Indices[0])
			case "makeslice":
				return "slice[" + g.exprToString(idxExpr.Indices[0]) + "]"
			}
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
			if pkgIdent, isPkg := sel.Left.(*ast.Identifier); isPkg {
				qname := pkgIdent.Value + "_" + sel.Right.Value
				if fs, exists := g.funcs[qname]; exists && len(fs.ReturnParameters) > 0 {
					return g.exprToString(fs.ReturnParameters[0].Type)
				}
				if fs, exists := g.funcs[pkgIdent.Value+"."+sel.Right.Value]; exists && len(fs.ReturnParameters) > 0 {
					return g.exprToString(fs.ReturnParameters[0].Type)
				}
			}
			if _, fs, ok := g.resolveMethod(sel.Left, sel.Right.Value); ok && len(fs.ReturnParameters) > 0 {
				return g.exprToString(fs.ReturnParameters[0].Type)
			}
			if sel.Right.Value == "Chop" {
				return "string"
			}
			if sel.Right.Value == "Get" {
				elemTypeStr, _ := g.getElemTypeInfo(sel.Left)
				return elemTypeStr
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
