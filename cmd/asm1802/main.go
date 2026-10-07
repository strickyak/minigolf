package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type SourceLine struct {
	File    string
	LineNum int
	Raw     string
	PC      uint32
	HasPC   bool
	Encoded []byte
}

type StmtType int

const (
	StmtInstruction StmtType = iota
	StmtOrg
	StmtEqu
	StmtDataByte
	StmtDataWord
	StmtReserve
	StmtAlign
	StmtEnd
	StmtComment
)

type Statement struct {
	Label        string
	Mnemonic     string
	RawOps       string
	LineNum      int
	File         string
	Type         StmtType
	PC           uint32
	Size         int
	Encoded      []byte
	SrcLine      *SourceLine
	IsBranch     bool
	IsRelaxable  bool
	BranchTarget string
	ShortOpcode  byte
	LongOpcode   byte
}

type Assembler struct {
	symbols      map[string]uint32
	statements   []*Statement
	sourceLines  []*SourceLine
	currPC       uint32
	pass         int
	entryPoint   uint32
	hasEntry     bool
	relaxEnabled bool
}

func NewAssembler() *Assembler {
	return &Assembler{
		symbols:      make(map[string]uint32),
		currPC:       0x0000,
		relaxEnabled: true,
	}
}

// stripComment removes comments (; or -- or column-0 *) while preserving string literals.
func stripComment(line string) string {
	inQuote := false
	quoteChar := byte(0)
	escaped := false

	// Check for whole-line comment starting with '*' in column 0
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) > 0 && trimmed[0] == '*' {
		return ""
	}

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == quoteChar {
				inQuote = false
			}
		} else {
			if ch == '"' || ch == '\'' {
				inQuote = true
				quoteChar = ch
			} else if ch == ';' {
				return line[:i]
			} else if ch == '-' && i+1 < len(line) && line[i+1] == '-' {
				return line[:i]
			}
		}
	}
	return line
}

// getBranchInfo returns the short opcode (1 byte + 1 byte target) and long opcode (1 byte + 2 byte target)
// for relaxable branch mnemonics.
func getBranchInfo(mnem string) (byte, byte, bool) {
	switch mnem {
	case "BR", "LBR", "JMP":
		return 0x30, 0xC0, true
	case "BZ", "LBZ", "JZ", "JE":
		return 0x32, 0xC2, true
	case "BNZ", "LBNZ", "JNZ", "JNE":
		return 0x3A, 0xCA, true
	case "BDF", "LBDF", "BPZ", "BGE", "JC", "JGE":
		return 0x33, 0xC3, true
	case "BNF", "LBNF", "BM", "BL", "JNC", "JL":
		return 0x3B, 0xCB, true
	case "BQ", "LBQ", "JQ":
		return 0x31, 0xC1, true
	case "BNQ", "LBNQ", "JNQ":
		return 0x39, 0xC9, true
	}
	return 0, 0, false
}

func (a *Assembler) LoadSource(filename string, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()

		srcLine := &SourceLine{
			File:    filename,
			LineNum: lineNum,
			Raw:     raw,
		}
		a.sourceLines = append(a.sourceLines, srcLine)

		codePart := stripComment(raw)
		codePart = strings.TrimRight(codePart, " \t\r\n")
		if strings.TrimSpace(codePart) == "" {
			continue
		}

		var label, mnemonic, opsStr string
		hasLeadingSpace := len(codePart) > 0 && (codePart[0] == ' ' || codePart[0] == '\t')

		if !hasLeadingSpace {
			fields := strings.Fields(codePart)
			if len(fields) > 0 {
				first := fields[0]
				if strings.HasSuffix(first, ":") {
					label = first[:len(first)-1]
					rest := strings.TrimSpace(codePart[len(first):])
					if rest != "" {
						subFields := strings.Fields(rest)
						mnemonic = subFields[0]
						opsStr = strings.TrimSpace(rest[len(mnemonic):])
					}
				} else {
					if len(fields) > 1 && (strings.EqualFold(fields[1], "EQU") || strings.EqualFold(fields[1], "=") || strings.EqualFold(fields[1], ".EQU")) {
						label = first
						mnemonic = fields[1]
						opsStr = strings.TrimSpace(codePart[len(first)+len(fields[1])+1:])
					} else {
						label = first
						rest := strings.TrimSpace(codePart[len(first):])
						if rest != "" {
							subFields := strings.Fields(rest)
							mnemonic = subFields[0]
							opsStr = strings.TrimSpace(rest[len(mnemonic):])
						}
					}
				}
			}
		} else {
			trimmedCode := strings.TrimSpace(codePart)
			fields := strings.Fields(trimmedCode)
			if len(fields) > 0 {
				mnemonic = fields[0]
				opsStr = strings.TrimSpace(trimmedCode[len(mnemonic):])
			}
		}

		stmt := &Statement{
			Label:    label,
			Mnemonic: strings.ToUpper(mnemonic),
			RawOps:   opsStr,
			LineNum:  lineNum,
			File:     filename,
			SrcLine:  srcLine,
		}

		// Classify directives
		switch stmt.Mnemonic {
		case "":
			stmt.Type = StmtComment
		case "ORG", ".ORG":
			stmt.Type = StmtOrg
		case "EQU", ".EQU", "=":
			stmt.Type = StmtEqu
		case "DB", "DEFB", "FCB", ".BYTE", ".DB":
			stmt.Type = StmtDataByte
		case "DW", "DEFW", "FDB", ".WORD", ".DW":
			stmt.Type = StmtDataWord
		case "DS", "DEFS", "RMB", ".BLKB", ".FILL", ".RES", ".SPACE":
			stmt.Type = StmtReserve
		case "ALIGN", ".ALIGN", "PAGE", ".PAGE":
			stmt.Type = StmtAlign
		case "ASCII", "FCC", ".ASCII":
			stmt.Type = StmtDataByte
		case "ASCIZ", ".ASCIZ", ".STRING":
			stmt.Type = StmtDataByte
		case "END", ".END":
			stmt.Type = StmtEnd
		default:
			stmt.Type = StmtInstruction
			if shortOpc, longOpc, ok := getBranchInfo(stmt.Mnemonic); ok {
				stmt.IsBranch = true
				stmt.IsRelaxable = true
				stmt.BranchTarget = strings.TrimSpace(stmt.RawOps)
				stmt.ShortOpcode = shortOpc
				stmt.LongOpcode = longOpc
				if strings.HasPrefix(stmt.Mnemonic, "L") || strings.HasPrefix(stmt.Mnemonic, "J") {
					stmt.Size = 3
				} else {
					stmt.Size = 2
				}
			}
		}

		a.statements = append(a.statements, stmt)
	}

	return scanner.Err()
}

// evalExpr parses and evaluates arithmetic and symbolic expressions.
func (a *Assembler) evalExpr(expr string) (int64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Leading # (immediate prefix in 6809 / 1802 assembly)
	if strings.HasPrefix(expr, "#") {
		expr = strings.TrimSpace(expr[1:])
	}

	// Current PC: * or $ or . (when alone)
	if expr == "*" || expr == "$" || expr == "." {
		return int64(a.currPC), nil
	}

	// Character constant: 'x'
	if len(expr) >= 3 && expr[0] == '\'' && expr[len(expr)-1] == '\'' {
		inner := expr[1 : len(expr)-1]
		if inner == "\\n" {
			return '\n', nil
		}
		if inner == "\\r" {
			return '\r', nil
		}
		if inner == "\\t" {
			return '\t', nil
		}
		if inner == "\\0" {
			return 0, nil
		}
		if inner == "\\\\" {
			return '\\', nil
		}
		if inner == "\\'" {
			return '\'', nil
		}
		if len(inner) == 1 {
			return int64(inner[0]), nil
		}
	}

	// High byte prefix: >expr or HIGH(expr)
	if strings.HasPrefix(expr, ">") {
		v, err := a.evalExpr(expr[1:])
		return (v >> 8) & 0xFF, err
	}
	u := strings.ToUpper(expr)
	if strings.HasPrefix(u, "HIGH(") && strings.HasSuffix(u, ")") {
		v, err := a.evalExpr(expr[5 : len(expr)-1])
		return (v >> 8) & 0xFF, err
	}

	// Low byte prefix: <expr or LOW(expr)
	if strings.HasPrefix(expr, "<") {
		v, err := a.evalExpr(expr[1:])
		return v & 0xFF, err
	}
	if strings.HasPrefix(u, "LOW(") && strings.HasSuffix(u, ")") {
		v, err := a.evalExpr(expr[4 : len(expr)-1])
		return v & 0xFF, err
	}

	// High/Low byte suffix: expr.1 and expr.0
	if strings.HasSuffix(expr, ".1") {
		v, err := a.evalExpr(expr[:len(expr)-2])
		return (v >> 8) & 0xFF, err
	}
	if strings.HasSuffix(expr, ".0") {
		v, err := a.evalExpr(expr[:len(expr)-2])
		return v & 0xFF, err
	}

	// Hex prefix $
	if strings.HasPrefix(expr, "$") && len(expr) > 1 {
		val, err := strconv.ParseUint(expr[1:], 16, 64)
		if err == nil {
			return int64(val), nil
		}
	}
	// Hex prefix 0x or 0X
	if strings.HasPrefix(expr, "0x") || strings.HasPrefix(expr, "0X") {
		val, err := strconv.ParseUint(expr[2:], 16, 64)
		if err == nil {
			return int64(val), nil
		}
	}
	// Hex suffix H: e.g. 0FFH or 1234H
	if strings.HasSuffix(u, "H") && len(u) > 1 {
		val, err := strconv.ParseUint(u[:len(u)-1], 16, 64)
		if err == nil {
			return int64(val), nil
		}
	}
	// Binary prefix %
	if strings.HasPrefix(expr, "%") && len(expr) > 1 {
		val, err := strconv.ParseUint(expr[1:], 2, 64)
		if err == nil {
			return int64(val), nil
		}
	}
	if strings.HasPrefix(expr, "0b") || strings.HasPrefix(expr, "0B") {
		val, err := strconv.ParseUint(expr[2:], 2, 64)
		if err == nil {
			return int64(val), nil
		}
	}
	// Octal prefix @
	if strings.HasPrefix(expr, "@") && len(expr) > 1 {
		val, err := strconv.ParseUint(expr[1:], 8, 64)
		if err == nil {
			return int64(val), nil
		}
	}

	// Plain decimal integer
	if val, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return val, nil
	}
	if val, err := strconv.ParseUint(expr, 10, 64); err == nil {
		return int64(val), nil
	}

	// Parenthesized expression
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return a.evalExpr(expr[1 : len(expr)-1])
	}

	// Binary bitwise and shift operators: | ^ & << >>
	if idx := strings.LastIndex(expr, "|"); idx > 0 {
		left, err1 := a.evalExpr(expr[:idx])
		right, err2 := a.evalExpr(expr[idx+1:])
		if err1 == nil && err2 == nil {
			return left | right, nil
		}
	}
	if idx := strings.LastIndex(expr, "^"); idx > 0 {
		left, err1 := a.evalExpr(expr[:idx])
		right, err2 := a.evalExpr(expr[idx+1:])
		if err1 == nil && err2 == nil {
			return left ^ right, nil
		}
	}
	if idx := strings.LastIndex(expr, "&"); idx > 0 {
		left, err1 := a.evalExpr(expr[:idx])
		right, err2 := a.evalExpr(expr[idx+1:])
		if err1 == nil && err2 == nil {
			return left & right, nil
		}
	}
	if idx := strings.LastIndex(expr, ">>"); idx > 0 {
		left, err1 := a.evalExpr(expr[:idx])
		right, err2 := a.evalExpr(expr[idx+2:])
		if err1 == nil && err2 == nil {
			return left >> uint64(right), nil
		}
	}
	if idx := strings.LastIndex(expr, "<<"); idx > 0 {
		left, err1 := a.evalExpr(expr[:idx])
		right, err2 := a.evalExpr(expr[idx+2:])
		if err1 == nil && err2 == nil {
			return left << uint64(right), nil
		}
	}

	// Binary operations: + and -
	if idx := strings.LastIndexAny(expr, "+-"); idx > 0 {
		symPart := strings.TrimSpace(expr[:idx])
		op := expr[idx]
		offPart := strings.TrimSpace(expr[idx+1:])
		base, err := a.evalExpr(symPart)
		if err == nil {
			off, err2 := a.evalExpr(offPart)
			if err2 == nil {
				if op == '+' {
					return base + off, nil
				}
				return base - off, nil
			}
		}
	}

	// Binary multiplication, division, modulo: * / %
	if idx := strings.LastIndexAny(expr, "*/%"); idx > 0 {
		leftPart := strings.TrimSpace(expr[:idx])
		op := expr[idx]
		rightPart := strings.TrimSpace(expr[idx+1:])
		left, err1 := a.evalExpr(leftPart)
		if err1 == nil {
			right, err2 := a.evalExpr(rightPart)
			if err2 == nil {
				switch op {
				case '*':
					return left * right, nil
				case '/':
					if right != 0 {
						return left / right, nil
					}
				case '%':
					if right != 0 {
						return left % right, nil
					}
				}
			}
		}
	}

	// Register names evaluate to their register index (0..15)
	if reg, ok := parseRegister(expr); ok {
		return int64(reg), nil
	}

	// Symbol lookup
	if val, ok := a.symbols[expr]; ok {
		return int64(val), nil
	}

	if a.pass == 1 {
		return 0, nil // Forward reference in pass 1 is acceptable
	}
	return 0, fmt.Errorf("undefined symbol %q", expr)
}

// parseRegister parses standard 1802 register syntax:
// R0..R15, R0..RF, 0..15, or symbolic alias.
func parseRegister(s string) (byte, bool) {
	s = strings.TrimSpace(s)
	u := strings.ToUpper(s)

	if strings.HasPrefix(u, "R") && len(u) >= 2 {
		numStr := u[1:]
		// Hex register RA..RF
		if len(numStr) == 1 && numStr[0] >= 'A' && numStr[0] <= 'F' {
			return numStr[0] - 'A' + 10, true
		}
		// Decimal or hex 0..15
		if n, err := strconv.Atoi(numStr); err == nil && n >= 0 && n <= 15 {
			return byte(n), true
		}
	}

	// Pure single-digit 0..9 or single hex digit A..F
	if len(u) == 1 {
		if u[0] >= '0' && u[0] <= '9' {
			return u[0] - '0', true
		}
		if u[0] >= 'A' && u[0] <= 'F' {
			return u[0] - 'A' + 10, true
		}
	}

	// Pure decimal 10..15
	if n, err := strconv.Atoi(u); err == nil && n >= 0 && n <= 15 {
		return byte(n), true
	}

	return 0, false
}

func splitOperands(opsStr string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	quoteChar := byte(0)
	parenDepth := 0

	for i := 0; i < len(opsStr); i++ {
		ch := opsStr[i]
		if inQuote {
			cur.WriteByte(ch)
			if ch == quoteChar {
				inQuote = false
			}
		} else {
			if ch == '"' || ch == '\'' {
				inQuote = true
				quoteChar = ch
				cur.WriteByte(ch)
			} else if ch == '(' {
				parenDepth++
				cur.WriteByte(ch)
			} else if ch == ')' {
				if parenDepth > 0 {
					parenDepth--
				}
				cur.WriteByte(ch)
			} else if ch == ',' && parenDepth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
			} else {
				cur.WriteByte(ch)
			}
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}
	return parts
}

func (a *Assembler) encodeDataBytes(opsStr string) ([]byte, error) {
	parts := splitOperands(opsStr)
	var out []byte

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if (strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") && len(p) >= 2) ||
			(strings.HasPrefix(p, "'") && strings.HasSuffix(p, "'") && len(p) > 3) {
			str := p[1 : len(p)-1]
			for i := 0; i < len(str); i++ {
				if str[i] == '\\' && i+1 < len(str) {
					i++
					switch str[i] {
					case 'n':
						out = append(out, '\n')
					case 'r':
						out = append(out, '\r')
					case 't':
						out = append(out, '\t')
					case '0':
						out = append(out, 0)
					default:
						out = append(out, str[i])
					}
				} else {
					out = append(out, str[i])
				}
			}
		} else {
			val, err := a.evalExpr(p)
			if err != nil && a.pass > 1 {
				return nil, fmt.Errorf("invalid byte expression %q: %v", p, err)
			}
			out = append(out, byte(val&0xFF))
		}
	}
	return out, nil
}

func (a *Assembler) encodeDataWords(opsStr string) ([]byte, error) {
	parts := splitOperands(opsStr)
	var out []byte

	for _, p := range parts {
		val, err := a.evalExpr(p)
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid word expression %q: %v", p, err)
		}
		out = append(out, byte((val>>8)&0xFF), byte(val&0xFF))
	}
	return out, nil
}

func (a *Assembler) encodeInstruction(stmt *Statement) ([]byte, error) {
	mnem := stmt.Mnemonic
	ops := stmt.RawOps
	parts := splitOperands(ops)

	// 0. Relaxable branch instructions
	if stmt.IsRelaxable {
		target, err := a.evalExpr(stmt.BranchTarget)
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid branch target %q: %v", stmt.BranchTarget, err)
		}
		if stmt.Size == 2 {
			if a.pass > 1 {
				currPage := ((stmt.PC + 2) >> 8) & 0xFF
				targetPage := (uint32(target) >> 8) & 0xFF
				if currPage != targetPage {
					return nil, fmt.Errorf("short branch %s target $%04X is outside current page $%02X00 (pc $%04X)",
						stmt.Mnemonic, uint32(target), currPage, stmt.PC)
				}
			}
			return []byte{stmt.ShortOpcode, byte(target & 0xFF)}, nil
		}
		// 3-byte long branch
		return []byte{stmt.LongOpcode, byte((target >> 8) & 0xFF), byte(target & 0xFF)}, nil
	}

	// 1. Inherent 1-byte control and ALU instructions
	switch mnem {
	case "IDL":
		return []byte{0x00}, nil
	case "IRX":
		return []byte{0x60}, nil
	case "RET":
		return []byte{0x70}, nil
	case "DIS":
		return []byte{0x71}, nil
	case "LDXA":
		return []byte{0x72}, nil
	case "STXD":
		return []byte{0x73}, nil
	case "ADC":
		return []byte{0x74}, nil
	case "SDB":
		return []byte{0x75}, nil
	case "SHRC", "RSHR":
		return []byte{0x76}, nil
	case "SMB":
		return []byte{0x77}, nil
	case "SAV":
		return []byte{0x78}, nil
	case "MARK":
		return []byte{0x79}, nil
	case "REQ":
		return []byte{0x7A}, nil
	case "SEQ":
		return []byte{0x7B}, nil
	case "SHLC", "RSHL":
		return []byte{0x7E}, nil
	case "NOP":
		return []byte{0xC4}, nil
	case "LSNQ":
		return []byte{0xC5}, nil
	case "LSNZ":
		return []byte{0xC6}, nil
	case "LSNF":
		return []byte{0xC7}, nil
	case "LSKP", "NLBR":
		return []byte{0xC8}, nil
	case "LSIE":
		return []byte{0xCC}, nil
	case "LSQ":
		return []byte{0xCD}, nil
	case "LSZ":
		return []byte{0xCE}, nil
	case "LSDF":
		return []byte{0xCF}, nil
	case "SKP", "NBR":
		return []byte{0x38}, nil
	case "LDX":
		return []byte{0xF0}, nil
	case "OR":
		return []byte{0xF1}, nil
	case "AND":
		return []byte{0xF2}, nil
	case "XOR":
		return []byte{0xF3}, nil
	case "ADD":
		return []byte{0xF4}, nil
	case "SD":
		return []byte{0xF5}, nil
	case "SHR":
		return []byte{0xF6}, nil
	case "SM":
		return []byte{0xF7}, nil
	case "SHL":
		return []byte{0xFE}, nil
	}

	// 2. Register operations: INC, DEC, LDA, STR, GLO, GHI, PLO, PHI, SEP, SEX, LDN
	regOpBase := map[string]byte{
		"LDN": 0x00,
		"INC": 0x10,
		"DEC": 0x20,
		"LDA": 0x40,
		"STR": 0x50,
		"GLO": 0x80,
		"GHI": 0x90,
		"PLO": 0xA0,
		"PHI": 0xB0,
		"SEP": 0xD0,
		"SEX": 0xE0,
	}

	if base, ok := regOpBase[mnem]; ok {
		if len(parts) == 0 || parts[0] == "" {
			return nil, fmt.Errorf("missing register operand for %s", mnem)
		}
		regVal, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid register operand %q: %v", parts[0], err)
		}
		if regVal < 0 || regVal > 15 {
			return nil, fmt.Errorf("register number %d out of range (0..15)", regVal)
		}
		if mnem == "LDN" && regVal == 0 {
			return nil, fmt.Errorf("LDN R0 is illegal (opcode 00 is IDL)")
		}
		return []byte{base | byte(regVal&0x0F)}, nil
	}

	// 3. I/O Port operations: OUT 1..7 / INP 1..7
	if mnem == "OUT" || mnem == "INP" {
		if len(parts) == 0 || parts[0] == "" {
			return nil, fmt.Errorf("missing port operand for %s", mnem)
		}
		portVal, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid port operand %q: %v", parts[0], err)
		}
		if portVal < 1 || portVal > 7 {
			return nil, fmt.Errorf("I/O port %d out of range (1..7)", portVal)
		}
		base := byte(0x60)
		if mnem == "INP" {
			base = 0x68
		}
		return []byte{base | byte(portVal&0x07)}, nil
	}

	// Handle merged OUT1..OUT7 and INP1..INP7
	if strings.HasPrefix(mnem, "OUT") && len(mnem) == 4 && mnem[3] >= '1' && mnem[3] <= '7' {
		port := mnem[3] - '0'
		return []byte{0x60 | port}, nil
	}
	if strings.HasPrefix(mnem, "INP") && len(mnem) == 4 && mnem[3] >= '1' && mnem[3] <= '7' {
		port := mnem[3] - '0'
		return []byte{0x68 | port}, nil
	}

	// 4. ALU Immediate operations (2 bytes)
	immOpCodes := map[string]byte{
		"ADCI": 0x7C,
		"SDBI": 0x7D,
		"SMBI": 0x7F,
		"LDI":  0xF8,
		"ORI":  0xF9,
		"ANI":  0xFA,
		"XRI":  0xFB,
		"ADI":  0xFC,
		"SDI":  0xFD,
		"SMI":  0xFF,
	}

	if opc, ok := immOpCodes[mnem]; ok {
		if len(parts) == 0 || parts[0] == "" {
			return nil, fmt.Errorf("missing immediate operand for %s", mnem)
		}
		val, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid immediate operand %q: %v", parts[0], err)
		}
		return []byte{opc, byte(val & 0xFF)}, nil
	}

	// 5. Short Branches (2 bytes: opcode, 8-bit page target)
	shortBranches := map[string]byte{
		"BR":  0x30,
		"BQ":  0x31,
		"BZ":  0x32,
		"BDF": 0x33,
		"BPZ": 0x33,
		"BGE": 0x33,
		"B1":  0x34,
		"B2":  0x35,
		"B3":  0x36,
		"B4":  0x37,
		"BNQ": 0x39,
		"BNZ": 0x3A,
		"BNF": 0x3B,
		"BM":  0x3B,
		"BL":  0x3B,
		"BN1": 0x3C,
		"BN2": 0x3D,
		"BN3": 0x3E,
		"BN4": 0x3F,
	}

	if opc, ok := shortBranches[mnem]; ok {
		if len(parts) == 0 || parts[0] == "" {
			return nil, fmt.Errorf("missing branch target for %s", mnem)
		}
		target, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid branch target %q: %v", parts[0], err)
		}

		if a.pass > 1 {
			currPage := ((stmt.PC + 1) >> 8) & 0xFF
			targetPage := (uint32(target) >> 8) & 0xFF
			if currPage != targetPage {
				return nil, fmt.Errorf("short branch %s target $%04X is outside current page $%02X00 (pc $%04X)",
					mnem, uint32(target), currPage, stmt.PC)
			}
		}

		return []byte{opc, byte(target & 0xFF)}, nil
	}

	// 6. Long Branches (3 bytes: opcode, target high, target low)
	longBranches := map[string]byte{
		"LBR":  0xC0,
		"LBQ":  0xC1,
		"LBZ":  0xC2,
		"LBDF": 0xC3,
		"LBNQ": 0xC9,
		"LBNZ": 0xCA,
		"LBNF": 0xCB,
	}

	if opc, ok := longBranches[mnem]; ok {
		if len(parts) == 0 || parts[0] == "" {
			return nil, fmt.Errorf("missing target address for %s", mnem)
		}
		target, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid target address %q: %v", parts[0], err)
		}
		return []byte{opc, byte((target >> 8) & 0xFF), byte(target & 0xFF)}, nil
	}

	// 7. Pseudo-instruction: LOAD Rn, <16-bit expr>
	// Expands to: LDI HIGH(expr); PHI Rn; LDI LOW(expr); PLO Rn (6 bytes)
	if mnem == "LOAD" {
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: LOAD Rn, <expr>")
		}
		regVal, err := a.evalExpr(parts[0])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid register operand %q: %v", parts[0], err)
		}
		if regVal < 0 || regVal > 15 {
			return nil, fmt.Errorf("register number %d out of range (0..15)", regVal)
		}
		rn := byte(regVal & 0x0F)

		val, err := a.evalExpr(parts[1])
		if err != nil && a.pass > 1 {
			return nil, fmt.Errorf("invalid load expression %q: %v", parts[1], err)
		}

		hi := byte((val >> 8) & 0xFF)
		lo := byte(val & 0xFF)

		return []byte{
			0xF8, hi, // LDI HIGH
			0xB0 | rn, // PHI Rn
			0xF8, lo, // LDI LOW
			0xA0 | rn, // PLO Rn
		}, nil
	}

	return nil, fmt.Errorf("unknown instruction mnemonic: %q", mnem)
}

func (a *Assembler) Relax() error {
	for iter := 0; iter < 30; iter++ {
		changed := false

		// 1. Recompute PC for all statements and update symbol table
		var currPC uint32 = 0
		for _, stmt := range a.statements {
			if stmt.Type == StmtOrg {
				val, err := a.evalExpr(stmt.RawOps)
				if err == nil {
					currPC = uint32(val)
				}
			}
			stmt.PC = currPC
			if stmt.Label != "" && stmt.Type != StmtEqu {
				a.symbols[stmt.Label] = currPC
				if stmt.Label == "_start" || stmt.Label == "cstart" || stmt.Label == "start" {
					a.entryPoint = currPC
					a.hasEntry = true
				}
			}
			currPC += uint32(stmt.Size)
		}

		// 2. Evaluate each relaxable branch
		for _, stmt := range a.statements {
			if !stmt.IsRelaxable {
				continue
			}
			targetVal, err := a.evalExpr(stmt.BranchTarget)
			if err != nil {
				continue
			}

			targetPage := (uint32(targetVal) >> 8) & 0xFF
			branchPage := ((stmt.PC + 2) >> 8) & 0xFF

			canBeShort := (targetPage == branchPage)

			// If nearing iteration limit, lock to long branch to avoid oscillation
			if iter >= 20 && !canBeShort {
				if stmt.Size != 3 {
					stmt.Size = 3
					changed = true
				}
				continue
			}

			if canBeShort && stmt.Size != 2 {
				stmt.Size = 2
				changed = true
			} else if !canBeShort && stmt.Size != 3 {
				stmt.Size = 3
				changed = true
			}
		}

		if !changed {
			break
		}
	}
	return nil
}

func (a *Assembler) Assemble() error {
	// Pass 1: Measure instruction sizes and collect label addresses
	a.pass = 1
	if err := a.executePass(); err != nil {
		return err
	}

	// Branch relaxation: iterate to fixed point
	if a.relaxEnabled {
		if err := a.Relax(); err != nil {
			return err
		}
	}

	// Pass 2: Resolve all symbols and encode machine code
	a.pass = 2
	if err := a.executePass(); err != nil {
		return err
	}

	return nil
}

func (a *Assembler) executePass() error {
	a.currPC = 0x0000

	for _, stmt := range a.statements {
		stmt.PC = a.currPC
		if stmt.SrcLine != nil && stmt.Type != StmtComment {
			if !stmt.SrcLine.HasPC || a.pass == 2 {
				stmt.SrcLine.PC = stmt.PC
				stmt.SrcLine.HasPC = true
			}
		}

		// Handle labels
		if stmt.Label != "" {
			if stmt.Type == StmtEqu {
				val, err := a.evalExpr(stmt.RawOps)
				if err != nil && a.pass > 1 {
					return fmt.Errorf("%s:%d: invalid EQU expression %q: %v", stmt.File, stmt.LineNum, stmt.RawOps, err)
				}
				a.symbols[stmt.Label] = uint32(val)
			} else {
				a.symbols[stmt.Label] = stmt.PC
			}
		}

		switch stmt.Type {
		case StmtOrg:
			val, err := a.evalExpr(stmt.RawOps)
			if err != nil {
				return fmt.Errorf("%s:%d: invalid ORG expression %q: %v", stmt.File, stmt.LineNum, stmt.RawOps, err)
			}
			a.currPC = uint32(val)
			stmt.PC = a.currPC
			stmt.Size = 0
			continue

		case StmtAlign:
			alignVal := int64(256)
			if stmt.Mnemonic == "ALIGN" || stmt.Mnemonic == ".ALIGN" {
				val, err := a.evalExpr(stmt.RawOps)
				if err != nil || val <= 0 {
					return fmt.Errorf("%s:%d: invalid ALIGN expression %q: %v", stmt.File, stmt.LineNum, stmt.RawOps, err)
				}
				alignVal = val
			}
			rem := a.currPC % uint32(alignVal)
			if rem != 0 {
				pad := uint32(alignVal) - rem
				stmt.Size = int(pad)
				stmt.Encoded = make([]byte, pad)
			} else {
				stmt.Size = 0
				stmt.Encoded = nil
			}

		case StmtEqu, StmtComment:
			stmt.Size = 0
			continue

		case StmtEnd:
			if stmt.RawOps != "" {
				val, err := a.evalExpr(stmt.RawOps)
				if err == nil {
					a.entryPoint = uint32(val)
					a.hasEntry = true
				}
			}
			stmt.Size = 0
			continue

		case StmtDataByte:
			bytesList, err := a.encodeDataBytes(stmt.RawOps)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			if stmt.Mnemonic == "ASCIZ" || stmt.Mnemonic == ".ASCIZ" || stmt.Mnemonic == ".STRING" {
				bytesList = append(bytesList, 0)
			}
			stmt.Size = len(bytesList)
			stmt.Encoded = bytesList
			if stmt.SrcLine != nil && a.pass == 2 {
				stmt.SrcLine.Encoded = bytesList
			}

		case StmtDataWord:
			wordsList, err := a.encodeDataWords(stmt.RawOps)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Size = len(wordsList)
			stmt.Encoded = wordsList
			if stmt.SrcLine != nil && a.pass == 2 {
				stmt.SrcLine.Encoded = wordsList
			}

		case StmtReserve:
			parts := splitOperands(stmt.RawOps)
			if len(parts) == 0 {
				return fmt.Errorf("%s:%d: missing reserve size", stmt.File, stmt.LineNum)
			}
			val, err := a.evalExpr(parts[0])
			if err != nil {
				return fmt.Errorf("%s:%d: invalid reserve expression %q: %v", stmt.File, stmt.LineNum, parts[0], err)
			}
			stmt.Size = int(val)
			stmt.Encoded = make([]byte, val) // Zero-padded

		case StmtInstruction:
			if a.pass == 1 && stmt.IsRelaxable && a.relaxEnabled {
				stmt.Size = 3
			} else {
				enc, err := a.encodeInstruction(stmt)
				if err != nil {
					return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
				}
				stmt.Size = len(enc)
				stmt.Encoded = enc
				if stmt.SrcLine != nil && a.pass == 2 {
					stmt.SrcLine.Encoded = enc
				}
			}
		}

		a.currPC += uint32(stmt.Size)
	}

	return nil
}

// EmitDECB writes an extended Hatvan DECB binary with magic header 'x', 'c'.
func (a *Assembler) EmitDECB(w io.Writer) error {
	// Hatvan Executable Magic header (Tag 253 = 0xFD): 'x', 'c' (COSMAC 1802)
	magicHdr := []byte{0xFD, 0x00, 0x00, 'x', 'c'}
	if _, err := w.Write(magicHdr); err != nil {
		return err
	}

	// Emit contiguous chunks of code
	var chunkBytes []byte
	chunkStartPC := uint32(0)
	inChunk := false

	flushChunk := func() error {
		if !inChunk || len(chunkBytes) == 0 {
			return nil
		}
		length := len(chunkBytes)
		for length > 0 {
			toWrite := length
			if toWrite > 65535 {
				toWrite = 65535
			}
			hdr := []byte{
				0x00, // Tag 0x00: Data block
				byte((toWrite >> 8) & 0xFF),
				byte(toWrite & 0xFF),
				byte((chunkStartPC >> 8) & 0xFF),
				byte(chunkStartPC & 0xFF),
			}
			if _, err := w.Write(hdr); err != nil {
				return err
			}
			if _, err := w.Write(chunkBytes[:toWrite]); err != nil {
				return err
			}
			chunkBytes = chunkBytes[toWrite:]
			chunkStartPC += uint32(toWrite)
			length -= toWrite
		}
		inChunk = false
		return nil
	}

	for _, stmt := range a.statements {
		if stmt.Type == StmtOrg {
			if inChunk && stmt.PC != chunkStartPC+uint32(len(chunkBytes)) {
				if err := flushChunk(); err != nil {
					return err
				}
			}
			continue
		}

		if len(stmt.Encoded) > 0 {
			if !inChunk {
				inChunk = true
				chunkStartPC = stmt.PC
				chunkBytes = nil
			} else if stmt.PC != chunkStartPC+uint32(len(chunkBytes)) {
				if err := flushChunk(); err != nil {
					return err
				}
				inChunk = true
				chunkStartPC = stmt.PC
				chunkBytes = nil
			}
			chunkBytes = append(chunkBytes, stmt.Encoded...)
		}
	}
	if err := flushChunk(); err != nil {
		return err
	}

	// Postamble trailer (Tag 255 = 0xFF): execution entry point
	entry := a.entryPoint
	if !a.hasEntry {
		if sym, ok := a.symbols["_start"]; ok {
			entry = sym
		} else if sym, ok := a.symbols["cstart"]; ok {
			entry = sym
		} else if sym, ok := a.symbols["start"]; ok {
			entry = sym
		} else {
			for _, stmt := range a.statements {
				if len(stmt.Encoded) > 0 {
					entry = stmt.PC
					break
				}
			}
		}
	}

	trailer := []byte{
		0xFF, // Tag 0xFF: Exec entry point
		0x00, 0x00,
		byte((entry >> 8) & 0xFF),
		byte(entry & 0xFF),
	}
	_, err := w.Write(trailer)
	return err
}

// EmitRaw writes a contiguous raw binary file.
func (a *Assembler) EmitRaw(w io.Writer) error {
	minPC := uint32(0xFFFFFFFF)
	maxPC := uint32(0)

	for _, stmt := range a.statements {
		if len(stmt.Encoded) > 0 {
			if stmt.PC < minPC {
				minPC = stmt.PC
			}
			end := stmt.PC + uint32(len(stmt.Encoded))
			if end > maxPC {
				maxPC = end
			}
		}
	}

	if minPC > maxPC {
		return nil
	}

	buf := make([]byte, maxPC-minPC)
	for _, stmt := range a.statements {
		if len(stmt.Encoded) > 0 {
			copy(buf[stmt.PC-minPC:], stmt.Encoded)
		}
	}

	_, err := w.Write(buf)
	return err
}

// EmitHex writes an Intel HEX formatted file.
func (a *Assembler) EmitHex(w io.Writer) error {
	var currentPC uint32
	var currentBytes []byte

	flushLine := func() error {
		if len(currentBytes) == 0 {
			return nil
		}
		for len(currentBytes) > 0 {
			chunkSize := len(currentBytes)
			if chunkSize > 16 {
				chunkSize = 16
			}
			chunk := currentBytes[:chunkSize]
			chk := byte(chunkSize) + byte(currentPC>>8) + byte(currentPC) + 0x00
			line := fmt.Sprintf(":%02X%04X00", chunkSize, currentPC)
			for _, b := range chunk {
				line += fmt.Sprintf("%02X", b)
				chk += b
			}
			checksum := byte(0x100 - uint32(chk))
			line += fmt.Sprintf("%02X\r\n", checksum)
			if _, err := io.WriteString(w, line); err != nil {
				return err
			}
			currentPC += uint32(chunkSize)
			currentBytes = currentBytes[chunkSize:]
		}
		return nil
	}

	for _, stmt := range a.statements {
		if len(stmt.Encoded) > 0 {
			if len(currentBytes) == 0 {
				currentPC = stmt.PC
			} else if stmt.PC != currentPC+uint32(len(currentBytes)) {
				if err := flushLine(); err != nil {
					return err
				}
				currentPC = stmt.PC
			}
			currentBytes = append(currentBytes, stmt.Encoded...)
		}
	}
	if err := flushLine(); err != nil {
		return err
	}

	// End of file record
	_, err := io.WriteString(w, ":00000001FF\r\n")
	return err
}

// EmitListing prints the assembly source with addresses and generated bytes.
func (a *Assembler) EmitListing(w io.Writer) error {
	for _, sl := range a.sourceLines {
		var hexBytes strings.Builder
		for i, b := range sl.Encoded {
			if i > 0 {
				hexBytes.WriteByte(' ')
			}
			hexBytes.WriteString(fmt.Sprintf("%02X", b))
			if i >= 3 && len(sl.Encoded) > 4 {
				hexBytes.WriteString("...")
				break
			}
		}

		pcStr := "    "
		if sl.HasPC {
			pcStr = fmt.Sprintf("%04X", sl.PC)
		}

		_, err := fmt.Fprintf(w, "%-4s  %-12s  %4d  %s\n", pcStr, hexBytes.String(), sl.LineNum, sl.Raw)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	outFlag := flag.String("o", "", "Output executable file name")
	listFlag := flag.String("l", "", "Listing output file name")
	formatFlag := flag.String("format", "decb", "Output format: decb, raw, or hex")
	relaxFlag := flag.Bool("relax", true, "Enable branch relaxation (shorten long branches when on same page)")
	noRelaxFlag := flag.Bool("no-relax", false, "Disable branch relaxation")

	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: asm1802 -o <output> [-l <listing.list>] [-format=decb|raw|hex] [-relax|-no-relax] <source.asm>...\n")
		os.Exit(1)
	}

	if *noRelaxFlag {
		*relaxFlag = false
	}

	asm := NewAssembler()
	asm.relaxEnabled = *relaxFlag

	for _, f := range files {
		file, err := os.Open(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", f, err)
			os.Exit(1)
		}
		err = asm.LoadSource(f, file)
		file.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", f, err)
			os.Exit(1)
		}
	}

	if err := asm.Assemble(); err != nil {
		fmt.Fprintf(os.Stderr, "Assembly error: %v\n", err)
		os.Exit(1)
	}

	var outWriter io.Writer = os.Stdout
	if *outFlag != "" {
		outFile, err := os.Create(*outFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file %s: %v\n", *outFlag, err)
			os.Exit(1)
		}
		defer outFile.Close()
		outWriter = outFile
	}

	format := strings.ToLower(*formatFlag)
	if strings.HasSuffix(*outFlag, ".bin") || strings.HasSuffix(*outFlag, ".rom") {
		format = "raw"
	} else if strings.HasSuffix(*outFlag, ".hex") {
		format = "hex"
	}

	switch format {
	case "raw":
		if err := asm.EmitRaw(outWriter); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting raw binary: %v\n", err)
			os.Exit(1)
		}
	case "hex":
		if err := asm.EmitHex(outWriter); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting Intel HEX: %v\n", err)
			os.Exit(1)
		}
	default: // "decb"
		if err := asm.EmitDECB(outWriter); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting DECB binary: %v\n", err)
			os.Exit(1)
		}
	}

	if *listFlag != "" {
		listFile, err := os.Create(*listFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating listing file %s: %v\n", *listFlag, err)
			os.Exit(1)
		}
		defer listFile.Close()
		if err := asm.EmitListing(listFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting listing: %v\n", err)
			os.Exit(1)
		}
	}
}
