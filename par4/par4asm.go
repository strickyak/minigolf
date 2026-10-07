package par4

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Assembler compiles Par4 assembly text into .p4p bytecode binary.
type Assembler struct {
	symbols      map[string]int
	funcIndices  map[string]int
	globals      []globalVar
	globalSlots  map[string]int
	functions    []*asmFunction
	currentFunc  *asmFunction
	entryFuncIdx int

	stringPool       []byte
	stringToOffset   map[string]int
	currentFuncIndex int
}

type globalVar struct {
	name string
	size int
}

type asmVar struct {
	name string
	size int
	slot int
}

type asmFunction struct {
	index       int
	name        string
	isEntry     bool
	params      []asmVar
	locals      []asmVar
	paramMap    map[string]int
	localMap    map[string]int
	labels      map[string]int
	lines       []asmLine
	codeSize    int
	frameSize   int
	codeOffset  int
	varTableOff int
}

type asmLine struct {
	lineNo  int
	raw     string
	label   string
	opcode  byte
	isOp    bool
	argStr  string
	pc      int
	codeLen int
}

func NewAssembler() *Assembler {
	return &Assembler{
		symbols:        make(map[string]int),
		funcIndices:    make(map[string]int),
		globalSlots:    make(map[string]int),
		stringToOffset: make(map[string]int),
		entryFuncIdx:   0,
	}
}

// Assemble translates Par4 assembly source into a .p4p binary.
func Assemble(source string) ([]byte, error) {
	a := NewAssembler()
	return a.Assemble(source)
}

func (a *Assembler) Assemble(source string) ([]byte, error) {
	lines := strings.Split(source, "\n")

	// Preload system call symbols
	a.symbols["I$Attach"] = int(SysAttach)
	a.symbols["I$Detach"] = int(SysDetach)
	a.symbols["I$Dup"] = int(SysDup)
	a.symbols["I$Create"] = int(SysCreate)
	a.symbols["I$Open"] = int(SysOpen)
	a.symbols["I$MakDir"] = int(SysMakDir)
	a.symbols["I$ChgDir"] = int(SysChgDir)
	a.symbols["I$Delete"] = int(SysDelete)
	a.symbols["I$Seek"] = int(SysSeek)
	a.symbols["I$Read"] = int(SysRead)
	a.symbols["I$Write"] = int(SysWrite)
	a.symbols["I$ReadLn"] = int(SysReadLn)
	a.symbols["I$WritLn"] = int(SysWritLn)
	a.symbols["I$GetStt"] = int(SysGetStt)
	a.symbols["I$SetStt"] = int(SysSetStt)
	a.symbols["I$Close"] = int(SysClose)
	a.symbols["F$Exit"] = int(SysExit)
	a.symbols["F$Fork"] = int(SysFork)
	a.symbols["F$Wait"] = int(SysWait)
	a.symbols["F$Mem"] = int(SysMem)
	a.symbols["F$Sleep"] = int(SysSleep)
	a.symbols["F$Time"] = int(SysTime)

	// Pass 1: Parse structure, directives, labels, and instruction sizes
	if err := a.pass1(lines); err != nil {
		return nil, err
	}

	// Pass 2: Emit bytecode and resolve relative jumps and symbols
	return a.pass2()
}

func (a *Assembler) addString(s string) int {
	if off, ok := a.stringToOffset[s]; ok {
		return off
	}
	off := len(a.stringPool)
	raw := []byte(s)
	// Pack 2-byte length, bytes, and null terminator
	entry := make([]byte, 2+len(raw)+1)
	binary.BigEndian.PutUint16(entry[0:2], uint16(len(raw)))
	copy(entry[2:], raw)
	entry[len(entry)-1] = 0 // null byte
	a.stringPool = append(a.stringPool, entry...)
	a.stringToOffset[s] = off
	return off
}

func stripComment(line string) string {
	inQuote := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inQuote = false
			}
		} else {
			if ch == '"' {
				inQuote = true
			} else if ch == ';' || ch == '#' {
				return line[:i]
			}
		}
	}
	return line
}

func unquoteString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		cleaned := strings.ReplaceAll(s, `\'`, `'`)
		if unquoted, err := strconv.Unquote(cleaned); err == nil {
			return unquoted
		}
		var b strings.Builder
		content := s[1 : len(s)-1]
		for i := 0; i < len(content); i++ {
			if content[i] == '\\' && i+1 < len(content) {
				i++
				switch content[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				case '\'':
					b.WriteByte('\'')
				case '0':
					b.WriteByte(0)
				case 'x':
					if i+2 < len(content) {
						if val, err := strconv.ParseUint(content[i+1:i+3], 16, 8); err == nil {
							b.WriteByte(byte(val))
							i += 2
							continue
						}
					}
					b.WriteByte('x')
				default:
					b.WriteByte(content[i])
				}
			} else {
				b.WriteByte(content[i])
			}
		}
		return b.String()
	}
	return s
}

func (a *Assembler) pass1(lines []string) error {
	for lineNo, line := range lines {
		raw := line
		// Strip comments (; or #) outside quotes
		line = stripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for label
		if strings.HasSuffix(line, ":") {
			lbl := strings.TrimSuffix(line, ":")
			if a.currentFunc != nil {
				a.currentFunc.labels[lbl] = a.currentFunc.codeSize
			} else {
				a.symbols[lbl] = 0
			}
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		first := parts[0]

		// Global directive: .global <name>: <size>
		if first == ".global" {
			name := strings.TrimSuffix(parts[1], ":")
			size := 2
			if len(parts) >= 3 {
				if parts[2] == "slice" || parts[2] == "string" {
					size = 6
				} else if sz, err := strconv.Atoi(parts[2]); err == nil {
					size = sz
				}
			}
			a.globalSlots[name] = len(a.globals)
			a.globals = append(a.globals, globalVar{name: name, size: size})
			continue
		}

		// Function directive: .function <name> [entry]
		if first == ".function" {
			fnName := parts[1]
			isEntry := false
			if len(parts) >= 3 && parts[2] == "entry" {
				isEntry = true
				a.entryFuncIdx = len(a.functions)
			}
			fn := &asmFunction{
				index:    len(a.functions),
				name:     fnName,
				isEntry:  isEntry,
				paramMap: make(map[string]int),
				localMap: make(map[string]int),
				labels:   make(map[string]int),
			}
			a.currentFunc = fn
			a.funcIndices[fnName] = fn.index
			a.functions = append(a.functions, fn)
			continue
		}

		// Param directive: .param <name>: <size>
		if first == ".param" {
			if a.currentFunc == nil {
				return fmt.Errorf("line %d: .param outside of function", lineNo+1)
			}
			name := strings.TrimSuffix(parts[1], ":")
			size := 2
			if len(parts) >= 3 {
				if parts[2] == "slice" || parts[2] == "string" {
					size = 6
				} else if sz, err := strconv.Atoi(parts[2]); err == nil {
					size = sz
				}
			}
			slot := len(a.currentFunc.params)
			a.currentFunc.paramMap[name] = slot
			a.currentFunc.params = append(a.currentFunc.params, asmVar{name: name, size: size, slot: slot})
			continue
		}

		// Local directive: .local <name>: <size>
		if first == ".local" {
			if a.currentFunc == nil {
				return fmt.Errorf("line %d: .local outside of function", lineNo+1)
			}
			name := strings.TrimSuffix(parts[1], ":")
			size := 2
			if len(parts) >= 3 {
				if parts[2] == "slice" || parts[2] == "string" {
					size = 6
				} else if sz, err := strconv.Atoi(parts[2]); err == nil {
					size = sz
				}
			}
			slot := len(a.currentFunc.params) + len(a.currentFunc.locals)
			a.currentFunc.localMap[name] = slot
			a.currentFunc.locals = append(a.currentFunc.locals, asmVar{name: name, size: size, slot: slot})
			continue
		}

		// Endfunction directive
		if first == ".endfunction" {
			a.currentFunc = nil
			continue
		}

		// Instruction
		if a.currentFunc == nil {
			return fmt.Errorf("line %d: instruction outside of function: %s", lineNo+1, line)
		}

		mnemonic := strings.ToUpper(first)
		op, exists := MnemonicToOpcode[mnemonic]
		if !exists {
			return fmt.Errorf("line %d: unknown instruction mnemonic %q", lineNo+1, first)
		}

		info := OpcodeTable[op]
		argStr := ""
		if len(parts) > 1 {
			// Extract operand after mnemonic, keeping string literal intact
			firstIdx := strings.Index(line, first)
			argStr = strings.TrimSpace(line[firstIdx+len(first):])
		}

		// Auto-optimize LOAD_LOCAL and STORE_LOCAL to fast 0-byte opcodes if slot 0..3
		codeLen := 1 + info.ArgBytes
		if mnemonic == "LOAD_LOCAL" && argStr != "" {
			slot := a.resolveSlot(a.currentFunc, argStr)
			if slot >= 0 && slot <= 3 {
				op = OpLOAD_LOCAL_0 + byte(slot)
				codeLen = 1
			}
		} else if mnemonic == "STORE_LOCAL" && argStr != "" {
			slot := a.resolveSlot(a.currentFunc, argStr)
			if slot >= 0 && slot <= 3 {
				op = OpSTORE_LOCAL_0 + byte(slot)
				codeLen = 1
			}
		}

		a.currentFunc.lines = append(a.currentFunc.lines, asmLine{
			lineNo:  lineNo + 1,
			raw:     raw,
			opcode:  op,
			isOp:    true,
			argStr:  argStr,
			pc:      a.currentFunc.codeSize,
			codeLen: codeLen,
		})
		a.currentFunc.codeSize += codeLen
	}

	return nil
}

func (a *Assembler) resolveSlot(fn *asmFunction, name string) int {
	if fn == nil {
		return -1
	}
	if s, ok := fn.paramMap[name]; ok {
		return s
	}
	if s, ok := fn.localMap[name]; ok {
		return s
	}
	if val, err := strconv.Atoi(name); err == nil {
		return val
	}
	return -1
}

func (a *Assembler) pass2() ([]byte, error) {
	// Calculate layouts
	totalCodeSize := 0
	for _, fn := range a.functions {
		fn.codeOffset = totalCodeSize
		totalCodeSize += fn.codeSize

		frameSize := 0
		for _, p := range fn.params {
			frameSize += p.size
		}
		for _, l := range fn.locals {
			frameSize += l.size
		}
		fn.frameSize = frameSize
	}

	// Local Variable Tables section
	var localVarsBytes []byte
	for _, fn := range a.functions {
		fn.varTableOff = len(localVarsBytes)
		for _, p := range fn.params {
			localVarsBytes = append(localVarsBytes, byte(p.size))
		}
		for _, l := range fn.locals {
			localVarsBytes = append(localVarsBytes, byte(l.size))
		}
	}

	// Bytecode stream
	bytecode := make([]byte, totalCodeSize)

	for _, fn := range a.functions {
		for _, line := range fn.lines {
			if !line.isOp {
				continue
			}

			offset := fn.codeOffset + line.pc
			bytecode[offset] = line.opcode
			info := OpcodeTable[line.opcode]

			if info.ArgBytes == 0 {
				continue
			}

			// Evaluate argument
			argVal, err := a.evaluateArg(fn, line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line.lineNo, err)
			}

			if info.ArgBytes == 1 {
				bytecode[offset+1] = byte(argVal & 0xFF)
			} else if info.ArgBytes == 2 {
				binary.BigEndian.PutUint16(bytecode[offset+1:offset+3], uint16(argVal))
			}
		}
	}

	// Build binary structure
	// 1. Header (16 bytes)
	header := make([]byte, HeaderSize)
	copy(header[0:4], Magic)
	header[4] = FormatVer
	header[5] = 0 // Flags
	binary.BigEndian.PutUint16(header[6:8], uint16(len(a.stringPool)))
	binary.BigEndian.PutUint16(header[8:10], uint16(len(a.globals)))
	binary.BigEndian.PutUint16(header[10:12], uint16(len(a.functions)))
	binary.BigEndian.PutUint16(header[12:14], uint16(a.entryFuncIdx))
	binary.BigEndian.PutUint16(header[14:16], uint16(len(bytecode)))

	// 2. Global Variable Table
	globalsTable := make([]byte, len(a.globals))
	for i, g := range a.globals {
		globalsTable[i] = byte(g.size)
	}

	// 3. String Pool: a.stringPool

	// 4. Function Table
	funcTable := make([]byte, len(a.functions)*FuncEntrySz)
	for i, fn := range a.functions {
		fb := funcTable[i*FuncEntrySz : (i+1)*FuncEntrySz]
		fb[0] = byte(len(fn.params))
		fb[1] = byte(len(fn.locals))
		binary.BigEndian.PutUint16(fb[2:4], uint16(fn.frameSize))
		binary.BigEndian.PutUint16(fb[4:6], uint16(fn.codeOffset))
		binary.BigEndian.PutUint16(fb[6:8], uint16(fn.codeSize))
		binary.BigEndian.PutUint16(fb[8:10], uint16(fn.varTableOff))
	}

	// Assemble final binary
	var out []byte
	out = append(out, header...)
	out = append(out, globalsTable...)
	out = append(out, a.stringPool...)
	out = append(out, funcTable...)
	out = append(out, localVarsBytes...)
	out = append(out, bytecode...)

	return out, nil
}

func (a *Assembler) evaluateArg(fn *asmFunction, line asmLine) (int, error) {
	argStr := strings.TrimSpace(line.argStr)
	info := OpcodeTable[line.opcode]

	// String literal for PUSH_STR
	if line.opcode == OpPUSH_STR {
		return a.addString(unquoteString(argStr)), nil
	}

	// Local slot resolution
	if info.ArgType == ArgLocal8 {
		slot := a.resolveSlot(fn, argStr)
		if slot >= 0 {
			return slot, nil
		}
		return 0, fmt.Errorf("undefined local variable: %q", argStr)
	}

	// Global slot resolution
	if info.ArgType == ArgGlobal16 {
		if s, ok := a.globalSlots[argStr]; ok {
			return s, nil
		}
		if v, err := strconv.Atoi(argStr); err == nil {
			return v, nil
		}
		return 0, fmt.Errorf("undefined global variable: %q", argStr)
	}

	// Function index resolution
	if info.ArgType == ArgFunc16 {
		if fIdx, ok := a.funcIndices[argStr]; ok {
			return fIdx, nil
		}
		if v, err := strconv.Atoi(argStr); err == nil {
			return v, nil
		}
		return 0, fmt.Errorf("undefined function: %q", argStr)
	}

	// Relative branch offset calculation
	if info.ArgType == ArgRel16 {
		targetPC, ok := fn.labels[argStr]
		if !ok {
			return 0, fmt.Errorf("undefined label %q in function %s", argStr, fn.name)
		}
		// Relative offset = targetPC - (instPC + 3)
		offset := targetPC - (line.pc + 3)
		return offset, nil
	}

	// Numeric expressions or syscall symbol
	if val, ok := a.symbols[argStr]; ok {
		return val, nil
	}

	// Check hex, octal, binary prefixes
	if strings.HasPrefix(argStr, "$") {
		v, err := strconv.ParseInt(argStr[1:], 16, 32)
		if err == nil {
			return int(v), nil
		}
	} else if strings.HasPrefix(argStr, "%") {
		v, err := strconv.ParseInt(argStr[1:], 2, 32)
		if err == nil {
			return int(v), nil
		}
	} else if strings.HasPrefix(argStr, "@") {
		v, err := strconv.ParseInt(argStr[1:], 8, 32)
		if err == nil {
			return int(v), nil
		}
	} else if strings.HasPrefix(argStr, "0x") || strings.HasPrefix(argStr, "0X") {
		v, err := strconv.ParseInt(argStr[2:], 16, 32)
		if err == nil {
			return int(v), nil
		}
	}

	// Plain integer
	v, err := strconv.ParseInt(argStr, 10, 32)
	if err == nil {
		return int(v), nil
	}

	return 0, fmt.Errorf("cannot evaluate operand: %q", argStr)
}
