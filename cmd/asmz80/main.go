package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type AddrMode int

const (
	ModeNone AddrMode = iota
	ModeReg8
	ModeReg16
	ModeImm
	ModeMemIndHL
	ModeMemIndBC
	ModeMemIndDE
	ModeMemIndSP
	ModeMemIndIX
	ModeMemIndIY
	ModeMemIndC
	ModeDirectMem
	ModeRelBranch
)

type Operand struct {
	Mode     AddrMode
	Reg8     uint8  // 0=B, 1=C, 2=D, 3=E, 4=H, 5=L, 6=(HL), 7=A
	Reg16    uint8  // 0=BC, 1=DE, 2=HL, 3=SP, 4=AF, 5=IX, 6=IY
	IsIXHalf bool   // true for IXH (4), IXL (5)
	IsIYHalf bool   // true for IYH (4), IYL (5)
	Disp     int32  // For (IX+d) / (IY+d)
	Expr     string // For immediate, direct address, or branch target
	Val      int64  // Evaluated numeric value
	Resolved bool
}

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
	StmtEnd
	StmtComment
)

type Statement struct {
	Type        StmtType
	Label       string
	Mnemonic    string
	RawOps      string
	LineNum     int
	File        string
	PC          uint32
	Size        int
	Encoded     []byte
	SrcLine     *SourceLine
	IsRelaxJump bool
	RelaxTarget string
	RelaxCond   int // -1 for unconditional, 0..3 for NZ, Z, NC, C
}

type Assembler struct {
	symbols      map[string]uint32
	statements   []*Statement
	sourceLines  []*SourceLine
	currPC       uint32
	entryPoint   uint32
	hasEntry     bool
	pass         int
	relaxEnabled bool
}

func NewAssembler() *Assembler {
	return &Assembler{
		symbols:      make(map[string]uint32),
		currPC:       0x0000,
		relaxEnabled: true,
	}
}

func (a *Assembler) evalExpr(expr string) (int64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	if strings.HasPrefix(expr, "#") {
		expr = expr[1:]
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

	// Hex prefix $
	if strings.HasPrefix(expr, "$") {
		val, err := strconv.ParseUint(expr[1:], 16, 64)
		return int64(val), err
	}
	// Hex prefix 0x or 0X
	if strings.HasPrefix(expr, "0x") || strings.HasPrefix(expr, "0X") {
		val, err := strconv.ParseUint(expr[2:], 16, 64)
		return int64(val), err
	}
	// Binary prefix %
	if strings.HasPrefix(expr, "%") {
		val, err := strconv.ParseUint(expr[1:], 2, 64)
		return int64(val), err
	}

	// Decimal integer
	if val, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return val, nil
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

	// Symbol lookup
	if val, ok := a.symbols[expr]; ok {
		return int64(val), nil
	}

	if a.pass == 1 {
		return 0, nil // Forward reference in pass 1 is acceptable
	}
	return 0, fmt.Errorf("undefined symbol %q", expr)
}

func parseReg8(s string) (reg uint8, ok bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "B":
		return 0, true
	case "C":
		return 1, true
	case "D":
		return 2, true
	case "E":
		return 3, true
	case "H":
		return 4, true
	case "L":
		return 5, true
	case "A":
		return 7, true
	}
	return 0, false
}

func parseReg16(s string) (reg uint8, ok bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "BC":
		return 0, true
	case "DE":
		return 1, true
	case "HL":
		return 2, true
	case "SP":
		return 3, true
	case "AF", "AF'":
		return 4, true
	case "IX":
		return 5, true
	case "IY":
		return 6, true
	}
	return 0, false
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func parseCond(s string) (cond int, ok bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "NZ":
		return 0, true
	case "Z":
		return 1, true
	case "NC":
		return 2, true
	case "C":
		return 3, true
	case "PO":
		return 4, true
	case "PE":
		return 5, true
	case "P":
		return 6, true
	case "M":
		return 7, true
	}
	return -1, false
}

func (a *Assembler) parseOperand(s string) (Operand, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Operand{Mode: ModeNone}, nil
	}

	// 1. Indirect memory: (...)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		uInner := strings.ToUpper(inner)

		if uInner == "HL" {
			return Operand{Mode: ModeMemIndHL}, nil
		}
		if uInner == "BC" {
			return Operand{Mode: ModeMemIndBC}, nil
		}
		if uInner == "DE" {
			return Operand{Mode: ModeMemIndDE}, nil
		}
		if uInner == "SP" {
			return Operand{Mode: ModeMemIndSP}, nil
		}
		if uInner == "C" {
			return Operand{Mode: ModeMemIndC}, nil
		}
		if uInner == "IX" {
			return Operand{Mode: ModeMemIndIX, Disp: 0}, nil
		}
		if uInner == "IY" {
			return Operand{Mode: ModeMemIndIY, Disp: 0}, nil
		}

		// Check for (IX+d) / (IX-d)
		if strings.HasPrefix(uInner, "IX") && len(uInner) > 2 && (uInner[2] == '+' || uInner[2] == '-') {
			dispVal, err := a.evalExpr(inner[2:])
			if err != nil && a.pass > 1 {
				return Operand{}, fmt.Errorf("invalid IX displacement in %q: %v", s, err)
			}
			if a.pass > 1 && (dispVal < -128 || dispVal > 127) {
				return Operand{}, fmt.Errorf("IX displacement %d out of range [-128, 127]", dispVal)
			}
			return Operand{Mode: ModeMemIndIX, Disp: int32(dispVal)}, nil
		}
		// Check for (IY+d) / (IY-d)
		if strings.HasPrefix(uInner, "IY") && len(uInner) > 2 && (uInner[2] == '+' || uInner[2] == '-') {
			dispVal, err := a.evalExpr(inner[2:])
			if err != nil && a.pass > 1 {
				return Operand{}, fmt.Errorf("invalid IY displacement in %q: %v", s, err)
			}
			if a.pass > 1 && (dispVal < -128 || dispVal > 127) {
				return Operand{}, fmt.Errorf("IY displacement %d out of range [-128, 127]", dispVal)
			}
			return Operand{Mode: ModeMemIndIY, Disp: int32(dispVal)}, nil
		}

		// Direct memory address: (nn)
		val, err := a.evalExpr(inner)
		return Operand{
			Mode:     ModeDirectMem,
			Expr:     inner,
			Val:      val,
			Resolved: (err == nil),
		}, nil
	}

	u := strings.ToUpper(s)

	// 2. 8-bit registers: A, B, C, D, E, H, L
	if r, ok := parseReg8(u); ok {
		return Operand{Mode: ModeReg8, Reg8: r, Expr: s}, nil
	}

	// 3. Undocumented 8-bit halves of IX/IY: IXH, IXL, IYH, IYL
	if u == "IXH" {
		return Operand{Mode: ModeReg8, Reg8: 4, IsIXHalf: true, Expr: s}, nil
	}
	if u == "IXL" {
		return Operand{Mode: ModeReg8, Reg8: 5, IsIXHalf: true, Expr: s}, nil
	}
	if u == "IYH" {
		return Operand{Mode: ModeReg8, Reg8: 4, IsIYHalf: true, Expr: s}, nil
	}
	if u == "IYL" {
		return Operand{Mode: ModeReg8, Reg8: 5, IsIYHalf: true, Expr: s}, nil
	}

	// 4. 16-bit registers: BC, DE, HL, SP, AF, IX, IY
	if rr, ok := parseReg16(u); ok {
		return Operand{Mode: ModeReg16, Reg16: rr, Expr: s}, nil
	}

	// 5. Immediate / Symbolic constant / Address
	val, err := a.evalExpr(s)
	return Operand{
		Mode:     ModeImm,
		Expr:     s,
		Val:      val,
		Resolved: (err == nil),
	}, nil
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

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
			a.statements = append(a.statements, &Statement{
				Type:    StmtComment,
				LineNum: lineNum,
				File:    filename,
				SrcLine: srcLine,
			})
			continue
		}

		// Strip inline comment
		codePart := raw
		if idx := strings.IndexAny(codePart, ";"); idx != -1 {
			// Ensure not inside a string literal
			inQuote := false
			splitIdx := -1
			for i := 0; i < len(codePart); i++ {
				if codePart[i] == '\'' && i > 0 && isIdentChar(codePart[i-1]) && !inQuote {
					continue
				}
				if codePart[i] == '\'' || codePart[i] == '"' {
					inQuote = !inQuote
				} else if (codePart[i] == ';' || (i+1 < len(codePart) && codePart[i] == '/' && codePart[i+1] == '/')) && !inQuote {
					splitIdx = i
					break
				}
			}
			if splitIdx != -1 {
				codePart = codePart[:splitIdx]
			}
		}

		codePart = strings.TrimRight(codePart, " \t\r\n")
		if strings.TrimSpace(codePart) == "" {
			continue
		}

		var label, mnemonic, opsStr string
		hasLeadingSpace := len(codePart) > 0 && (codePart[0] == ' ' || codePart[0] == '\t')

		if !hasLeadingSpace {
			// Starts at column 0: label present
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
					if len(fields) > 1 && (strings.EqualFold(fields[1], "EQU") || strings.EqualFold(fields[1], "=")) {
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
			// No label at column 0
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

		// Classify directive types
		switch stmt.Mnemonic {
		case "":
			stmt.Type = StmtComment
		case "ORG":
			stmt.Type = StmtOrg
		case "EQU", "=":
			stmt.Type = StmtEqu
		case "DEFB", "DB", "FCB", ".BYTE":
			stmt.Type = StmtDataByte
		case "DEFW", "DW", "FDB", ".WORD":
			stmt.Type = StmtDataWord
		case "DEFS", "DS", "RMB", ".BLKB":
			stmt.Type = StmtReserve
		case "END":
			stmt.Type = StmtEnd
		default:
			stmt.Type = StmtInstruction
		}

		a.statements = append(a.statements, stmt)
	}

	return scanner.Err()
}

func splitOperands(s string) []string {
	var ops []string
	var cur bytes.Buffer
	inQuote := false
	quoteChar := byte(0)
	parenDepth := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && i > 0 && isIdentChar(s[i-1]) && !inQuote {
			cur.WriteByte(c)
			continue
		}
		if (c == '\'' || c == '"') && parenDepth == 0 {
			if inQuote && c == quoteChar {
				inQuote = false
			} else if !inQuote {
				inQuote = true
				quoteChar = c
			}
		}
		if !inQuote {
			if c == '(' {
				parenDepth++
			} else if c == ')' {
				parenDepth--
			} else if c == ',' && parenDepth == 0 {
				ops = append(ops, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		ops = append(ops, strings.TrimSpace(cur.String()))
	}
	return ops
}

func (a *Assembler) Assemble() error {
	// Pass 1: Initial sizing and symbol collection
	a.pass = 1
	if err := a.runPass(); err != nil {
		return err
	}

	// Relaxation iterations: optimize jumps to fixed point
	maxIterations := 10
	for iter := 0; iter < maxIterations; iter++ {
		changed := false
		a.currPC = 0x0000

		for _, stmt := range a.statements {
			if stmt.Type == StmtOrg {
				val, _ := a.evalExpr(stmt.RawOps)
				a.currPC = uint32(val)
				stmt.PC = a.currPC
				continue
			}

			stmt.PC = a.currPC
			if stmt.Label != "" {
				if oldVal, ok := a.symbols[stmt.Label]; !ok || oldVal != stmt.PC {
					a.symbols[stmt.Label] = stmt.PC
					changed = true
				}
			}

			if stmt.Type == StmtInstruction && stmt.IsRelaxJump {
				oldSize := stmt.Size
				targetVal, err := a.evalExpr(stmt.RelaxTarget)
				if err == nil {
					disp := targetVal - int64(stmt.PC+2)
					if disp >= -128 && disp <= 127 && stmt.RelaxCond <= 3 {
						stmt.Size = 2 // JR fits in 2 bytes
					} else {
						stmt.Size = 3 // JP requires 3 bytes
					}
					if stmt.Size != oldSize {
						changed = true
					}
				}
			}

			a.currPC += uint32(stmt.Size)
		}

		if !changed {
			break
		}
	}

	// Pass 2: Final encoding and symbol resolution
	a.pass = 2
	a.currPC = 0x0000
	return a.runPass()
}

func (a *Assembler) runPass() error {
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
				return fmt.Errorf("%s:%d: empty DEFS size", stmt.File, stmt.LineNum)
			}
			countVal, err := a.evalExpr(parts[0])
			if err != nil {
				return fmt.Errorf("%s:%d: invalid DEFS size: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Size = int(countVal)
			fillByte := byte(0)
			if len(parts) > 1 {
				fillVal, err := a.evalExpr(parts[1])
				if err != nil {
					return fmt.Errorf("%s:%d: invalid DEFS fill value: %v", stmt.File, stmt.LineNum, err)
				}
				fillByte = byte(fillVal)
			}
			stmt.Encoded = make([]byte, stmt.Size)
			for i := range stmt.Encoded {
				stmt.Encoded[i] = fillByte
			}

		case StmtInstruction:
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

		a.currPC += uint32(stmt.Size)
	}

	return nil
}

func (a *Assembler) encodeDataBytes(opsStr string) ([]byte, error) {
	parts := splitOperands(opsStr)
	var out []byte

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if (strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"")) ||
			(strings.HasPrefix(p, "'") && strings.HasSuffix(p, "'") && len(p) > 3) {
			// String literal
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
			out = append(out, byte(val))
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
		// Little-endian for Z80: Low byte first, then High byte
		out = append(out, byte(val&0xFF), byte((val>>8)&0xFF))
	}
	return out, nil
}

func (a *Assembler) encodeInstruction(stmt *Statement) ([]byte, error) {
	mnem := stmt.Mnemonic
	rawOps := stmt.RawOps
	ops := splitOperands(rawOps)

	var op1, op2 Operand
	var err error
	if len(ops) > 0 && ops[0] != "" {
		op1, err = a.parseOperand(ops[0])
		if err != nil && a.pass > 1 {
			return nil, err
		}
	}
	if len(ops) > 1 && ops[1] != "" {
		op2, err = a.parseOperand(ops[1])
		if err != nil && a.pass > 1 {
			return nil, err
		}
	}

	switch mnem {
	case "NOP":
		return []byte{0x00}, nil
	case "HALT":
		return []byte{0x76}, nil
	case "DI":
		return []byte{0xF3}, nil
	case "EI":
		return []byte{0xFB}, nil
	case "EXX":
		return []byte{0xD9}, nil
	case "DAA":
		return []byte{0x27}, nil
	case "CPL":
		return []byte{0x2F}, nil
	case "NEG":
		return []byte{0xED, 0x44}, nil
	case "CCF":
		return []byte{0x3F}, nil
	case "SCF":
		return []byte{0x37}, nil
	case "RLCA":
		return []byte{0x07}, nil
	case "RRCA":
		return []byte{0x0F}, nil
	case "RLA":
		return []byte{0x17}, nil
	case "RRA":
		return []byte{0x1F}, nil
	case "RETI":
		return []byte{0xED, 0x4D}, nil
	case "RETN":
		return []byte{0xED, 0x45}, nil
	case "LDI":
		return []byte{0xED, 0xA0}, nil
	case "LDIR":
		return []byte{0xED, 0xB0}, nil
	case "LDD":
		return []byte{0xED, 0xA8}, nil
	case "LDDR":
		return []byte{0xED, 0xB8}, nil
	case "CPI":
		return []byte{0xED, 0xA1}, nil
	case "CPIR":
		return []byte{0xED, 0xB1}, nil
	case "CPD":
		return []byte{0xED, 0xA9}, nil
	case "CPDR":
		return []byte{0xED, 0xB9}, nil
	case "INI":
		return []byte{0xED, 0xA2}, nil
	case "INIR":
		return []byte{0xED, 0xB2}, nil
	case "IND":
		return []byte{0xED, 0xAA}, nil
	case "INDR":
		return []byte{0xED, 0xBA}, nil
	case "OUTI":
		return []byte{0xED, 0xA3}, nil
	case "OTIR":
		return []byte{0xED, 0xB3}, nil
	case "OUTD":
		return []byte{0xED, 0xAB}, nil
	case "OTDR":
		return []byte{0xED, 0xBB}, nil

	case "IM":
		imVal, err := a.evalExpr(op1.Expr)
		if err != nil {
			return nil, err
		}
		switch imVal {
		case 0:
			return []byte{0xED, 0x46}, nil
		case 1:
			return []byte{0xED, 0x56}, nil
		case 2:
			return []byte{0xED, 0x5E}, nil
		default:
			return nil, fmt.Errorf("invalid interrupt mode %d", imVal)
		}

	case "EX":
		return a.encodeEX(op1, op2)

	case "PUSH":
		return a.encodePushPop(op1, 0xC5)
	case "POP":
		return a.encodePushPop(op1, 0xC1)

	case "INC":
		return a.encodeIncDec(op1, 0x04, 0x03, 0x23)
	case "DEC":
		return a.encodeIncDec(op1, 0x05, 0x0B, 0x2B)

	case "ADD":
		return a.encodeAdd(op1, op2)
	case "ADC":
		return a.encodeAdcSbc(op1, op2, 0x88, 0x4A)
	case "SBC":
		return a.encodeAdcSbc(op1, op2, 0x98, 0x42)
	case "SUB":
		return a.encodeAluSingle(op1, op2, 0x90, 0xD6)
	case "AND":
		return a.encodeAluSingle(op1, op2, 0xA0, 0xE6)
	case "XOR":
		return a.encodeAluSingle(op1, op2, 0xA8, 0xEE)
	case "OR":
		return a.encodeAluSingle(op1, op2, 0xB0, 0xF6)
	case "CP":
		return a.encodeAluSingle(op1, op2, 0xB8, 0xFE)

	case "RLC", "RRC", "RL", "RR", "SLA", "SRA", "SRL":
		return a.encodeShiftRotate(mnem, op1)

	case "BIT":
		return a.encodeBitOp(op1, op2, 0x40)
	case "RES":
		return a.encodeBitOp(op1, op2, 0x80)
	case "SET":
		return a.encodeBitOp(op1, op2, 0xC0)

	case "JP", "JMP":
		return a.encodeJp(stmt, op1, op2)
	case "JR":
		return a.encodeJr(stmt, op1, op2)
	case "DJNZ":
		disp := int64(0)
		targetVal, err := a.evalExpr(op1.Expr)
		if err == nil {
			disp = targetVal - int64(stmt.PC+2)
		}
		if a.pass > 1 && (disp < -128 || disp > 127) {
			return nil, fmt.Errorf("DJNZ displacement %d out of range", disp)
		}
		return []byte{0x10, byte(disp)}, nil

	case "CALL":
		return a.encodeCall(op1, op2)
	case "RET":
		return a.encodeRet(op1)
	case "RST":
		rstVal, err := a.evalExpr(op1.Expr)
		if err != nil {
			return nil, err
		}
		if (rstVal & 7) != 0 || rstVal < 0 || rstVal > 0x38 {
			return nil, fmt.Errorf("invalid RST target: %d", rstVal)
		}
		return []byte{byte(0xC7 | rstVal)}, nil

	case "IN":
		return a.encodeIN(op1, op2)
	case "OUT":
		return a.encodeOUT(op1, op2)

	case "LD":
		return a.encodeLD(op1, op2)
	}

	return nil, fmt.Errorf("unrecognized instruction %q", mnem)
}

func (a *Assembler) encodePushPop(op Operand, baseOp uint8) ([]byte, error) {
	if op.Mode == ModeReg16 {
		switch op.Reg16 {
		case 0, 1, 2: // BC, DE, HL
			return []byte{baseOp | (op.Reg16 << 4)}, nil
		case 4: // AF
			return []byte{baseOp | (3 << 4)}, nil
		case 5: // IX
			if baseOp == 0xC5 {
				return []byte{0xDD, 0xE5}, nil
			}
			return []byte{0xDD, 0xE1}, nil
		case 6: // IY
			if baseOp == 0xC5 {
				return []byte{0xFD, 0xE5}, nil
			}
			return []byte{0xFD, 0xE1}, nil
		}
	}
	return nil, fmt.Errorf("invalid operand for PUSH/POP: %+v", op)
}

func (a *Assembler) encodeEX(op1, op2 Operand) ([]byte, error) {
	if op1.Mode == ModeReg16 && op2.Mode == ModeReg16 {
		if (op1.Reg16 == 1 && op2.Reg16 == 2) || (op1.Reg16 == 2 && op2.Reg16 == 1) { // EX DE, HL or EX HL, DE
			return []byte{0xEB}, nil
		}
		if op1.Reg16 == 4 && op2.Reg16 == 4 { // EX AF, AF'
			return []byte{0x08}, nil
		}
	}
	if op1.Mode == ModeMemIndSP && op2.Mode == ModeReg16 {
		switch op2.Reg16 {
		case 2: // EX (SP), HL
			return []byte{0xE3}, nil
		case 5: // EX (SP), IX
			return []byte{0xDD, 0xE3}, nil
		case 6: // EX (SP), IY
			return []byte{0xFD, 0xE3}, nil
		}
	}
	return nil, fmt.Errorf("invalid operands for EX")
}

func (a *Assembler) encodeIncDec(op Operand, regBase uint8, reg16Base uint8, ixBase uint8) ([]byte, error) {
	if op.Mode == ModeReg8 {
		if op.IsIXHalf {
			return []byte{0xDD, regBase | (op.Reg8 << 3)}, nil
		}
		if op.IsIYHalf {
			return []byte{0xFD, regBase | (op.Reg8 << 3)}, nil
		}
		return []byte{regBase | (op.Reg8 << 3)}, nil
	}
	if op.Mode == ModeMemIndHL {
		return []byte{regBase | (6 << 3)}, nil // 0x34 or 0x35
	}
	if op.Mode == ModeMemIndIX {
		return []byte{0xDD, regBase | (6 << 3), byte(op.Disp)}, nil
	}
	if op.Mode == ModeMemIndIY {
		return []byte{0xFD, regBase | (6 << 3), byte(op.Disp)}, nil
	}
	if op.Mode == ModeReg16 {
		if op.Reg16 <= 3 { // BC, DE, HL, SP
			return []byte{reg16Base | (op.Reg16 << 4)}, nil
		}
		if op.Reg16 == 5 { // IX
			return []byte{0xDD, ixBase}, nil
		}
		if op.Reg16 == 6 { // IY
			return []byte{0xFD, ixBase}, nil
		}
	}
	return nil, fmt.Errorf("invalid operand for INC/DEC")
}

func (a *Assembler) encodeAdd(op1, op2 Operand) ([]byte, error) {
	// 16-bit ADD HL, rr
	if op1.Mode == ModeReg16 && op1.Reg16 == 2 && op2.Mode == ModeReg16 && op2.Reg16 <= 3 {
		return []byte{0x09 | (op2.Reg16 << 4)}, nil
	}
	// 16-bit ADD IX, rr
	if op1.Mode == ModeReg16 && op1.Reg16 == 5 && op2.Mode == ModeReg16 {
		var code uint8
		switch op2.Reg16 {
		case 0:
			code = 0
		case 1:
			code = 1
		case 5:
			code = 2
		case 3:
			code = 3
		default:
			return nil, fmt.Errorf("invalid operand for ADD IX, rr")
		}
		return []byte{0xDD, 0x09 | (code << 4)}, nil
	}
	// 16-bit ADD IY, rr
	if op1.Mode == ModeReg16 && op1.Reg16 == 6 && op2.Mode == ModeReg16 {
		var code uint8
		switch op2.Reg16 {
		case 0:
			code = 0
		case 1:
			code = 1
		case 6:
			code = 2
		case 3:
			code = 3
		default:
			return nil, fmt.Errorf("invalid operand for ADD IY, rr")
		}
		return []byte{0xFD, 0x09 | (code << 4)}, nil
	}

	// 8-bit ADD [A,] s
	return a.encodeAluSingle(op1, op2, 0x80, 0xC6)
}

func (a *Assembler) encodeAdcSbc(op1, op2 Operand, base8 uint8, edCode uint8) ([]byte, error) {
	// 16-bit ADC/SBC HL, rr
	if op1.Mode == ModeReg16 && op1.Reg16 == 2 && op2.Mode == ModeReg16 && op2.Reg16 <= 3 {
		return []byte{0xED, edCode | (op2.Reg16 << 4)}, nil
	}
	// 8-bit ADC/SBC A, s
	return a.encodeAluSingle(op1, op2, base8, base8|0x46)
}

func (a *Assembler) encodeAluSingle(op1, op2 Operand, base8 uint8, immOp uint8) ([]byte, error) {
	target := op1
	if op1.Mode == ModeReg8 && op1.Reg8 == 7 && op2.Mode != ModeNone { // "ADD A, s" form
		target = op2
	}

	switch target.Mode {
	case ModeReg8:
		if target.IsIXHalf {
			return []byte{0xDD, base8 | target.Reg8}, nil
		}
		if target.IsIYHalf {
			return []byte{0xFD, base8 | target.Reg8}, nil
		}
		return []byte{base8 | target.Reg8}, nil
	case ModeMemIndHL:
		return []byte{base8 | 6}, nil
	case ModeMemIndIX:
		return []byte{0xDD, base8 | 6, byte(target.Disp)}, nil
	case ModeMemIndIY:
		return []byte{0xFD, base8 | 6, byte(target.Disp)}, nil
	case ModeImm:
		return []byte{immOp, byte(target.Val)}, nil
	}
	return nil, fmt.Errorf("invalid operand for ALU operation")
}

func (a *Assembler) encodeShiftRotate(mnem string, op Operand) ([]byte, error) {
	var opCode uint8
	switch mnem {
	case "RLC":
		opCode = 0x00
	case "RRC":
		opCode = 0x08
	case "RL":
		opCode = 0x10
	case "RR":
		opCode = 0x18
	case "SLA":
		opCode = 0x20
	case "SRA":
		opCode = 0x28
	case "SRL":
		opCode = 0x38
	}

	switch op.Mode {
	case ModeReg8:
		return []byte{0xCB, opCode | op.Reg8}, nil
	case ModeMemIndHL:
		return []byte{0xCB, opCode | 6}, nil
	case ModeMemIndIX:
		return []byte{0xDD, 0xCB, byte(op.Disp), opCode | 6}, nil
	case ModeMemIndIY:
		return []byte{0xFD, 0xCB, byte(op.Disp), opCode | 6}, nil
	}
	return nil, fmt.Errorf("invalid operand for shift/rotate")
}

func (a *Assembler) encodeBitOp(op1, op2 Operand, baseOp uint8) ([]byte, error) {
	bitNum, err := a.evalExpr(op1.Expr)
	if err != nil || bitNum < 0 || bitNum > 7 {
		return nil, fmt.Errorf("invalid bit number: %d", bitNum)
	}

	bOp := baseOp | uint8(bitNum<<3)
	switch op2.Mode {
	case ModeReg8:
		return []byte{0xCB, bOp | op2.Reg8}, nil
	case ModeMemIndHL:
		return []byte{0xCB, bOp | 6}, nil
	case ModeMemIndIX:
		return []byte{0xDD, 0xCB, byte(op2.Disp), bOp | 6}, nil
	case ModeMemIndIY:
		return []byte{0xFD, 0xCB, byte(op2.Disp), bOp | 6}, nil
	}
	return nil, fmt.Errorf("invalid operand for bit instruction")
}

func (a *Assembler) encodeJp(stmt *Statement, op1, op2 Operand) ([]byte, error) {
	// Register indirect jumps: JP (HL), JP (IX), JP (IY)
	if op1.Mode == ModeMemIndHL && op2.Mode == ModeNone {
		return []byte{0xE9}, nil
	}
	if op1.Mode == ModeMemIndIX && op2.Mode == ModeNone {
		return []byte{0xDD, 0xE9}, nil
	}
	if op1.Mode == ModeMemIndIY && op2.Mode == ModeNone {
		return []byte{0xFD, 0xE9}, nil
	}

	// Check if relaxation is active
	cond := -1
	targetExpr := op1.Expr
	if op2.Mode != ModeNone {
		// Conditional: JP cc, target
		c, ok := parseCond(op1.Expr)
		if !ok {
			return nil, fmt.Errorf("invalid condition %q", op1.Expr)
		}
		cond = c
		targetExpr = op2.Expr
	}

	if a.relaxEnabled && (stmt.Mnemonic == "JMP" || stmt.IsRelaxJump) {
		stmt.IsRelaxJump = true
		stmt.RelaxTarget = targetExpr
		stmt.RelaxCond = cond

		if a.pass == 1 {
			targetVal, err := a.evalExpr(targetExpr)
			if err == nil && (targetVal-int64(stmt.PC+2) < -128 || targetVal-int64(stmt.PC+2) > 127 || cond > 3) {
				return make([]byte, 3), nil
			}
			if cond > 3 {
				return make([]byte, 3), nil
			}
			return make([]byte, 2), nil
		}

		if stmt.Size == 2 {
			targetVal, err := a.evalExpr(targetExpr)
			if err != nil {
				return nil, err
			}
			disp := targetVal - int64(stmt.PC+2)
			if disp < -128 || disp > 127 {
				return nil, fmt.Errorf("relaxed jump target out of range: disp=%d", disp)
			}
			if cond == -1 {
				return []byte{0x18, byte(disp)}, nil
			}
			return []byte{byte(0x20 | (cond << 3)), byte(disp)}, nil
		}
	}

	targetVal, err := a.evalExpr(targetExpr)
	if err != nil && a.pass > 1 {
		return nil, err
	}
	lo := byte(targetVal & 0xFF)
	hi := byte((targetVal >> 8) & 0xFF)
	if cond == -1 {
		return []byte{0xC3, lo, hi}, nil
	}
	return []byte{byte(0xC2 | (cond << 3)), lo, hi}, nil
}

func (a *Assembler) encodeJr(stmt *Statement, op1, op2 Operand) ([]byte, error) {
	cond := -1
	targetExpr := op1.Expr
	if op2.Mode != ModeNone {
		c, ok := parseCond(op1.Expr)
		if !ok || c > 3 {
			return nil, fmt.Errorf("invalid JR condition %q (only NZ, Z, NC, C supported)", op1.Expr)
		}
		cond = c
		targetExpr = op2.Expr
	}

	targetVal, err := a.evalExpr(targetExpr)
	disp := int64(0)
	if err == nil {
		disp = targetVal - int64(stmt.PC+2)
	}
	if a.pass > 1 && (disp < -128 || disp > 127) {
		return nil, fmt.Errorf("JR target out of range (%d bytes)", disp)
	}

	if cond == -1 {
		return []byte{0x18, byte(disp)}, nil
	}
	return []byte{byte(0x20 | (cond << 3)), byte(disp)}, nil
}

func (a *Assembler) encodeCall(op1, op2 Operand) ([]byte, error) {
	targetExpr := op1.Expr
	cond := -1
	if op2.Mode != ModeNone {
		c, ok := parseCond(op1.Expr)
		if !ok {
			return nil, fmt.Errorf("invalid condition for CALL: %q", op1.Expr)
		}
		cond = c
		targetExpr = op2.Expr
	}

	targetVal, err := a.evalExpr(targetExpr)
	if err != nil {
		return nil, err
	}
	lo := byte(targetVal & 0xFF)
	hi := byte((targetVal >> 8) & 0xFF)

	if cond == -1 {
		return []byte{0xCD, lo, hi}, nil
	}
	return []byte{byte(0xC4 | (cond << 3)), lo, hi}, nil
}

func (a *Assembler) encodeRet(op1 Operand) ([]byte, error) {
	if op1.Mode == ModeNone {
		return []byte{0xC9}, nil
	}
	cond, ok := parseCond(op1.Expr)
	if !ok {
		return nil, fmt.Errorf("invalid condition for RET: %q", op1.Expr)
	}
	return []byte{byte(0xC0 | (cond << 3))}, nil
}

func (a *Assembler) encodeIN(op1, op2 Operand) ([]byte, error) {
	// IN A, (n)
	if op1.Mode == ModeReg8 && op1.Reg8 == 7 && op2.Mode == ModeDirectMem {
		portVal, err := a.evalExpr(op2.Expr)
		if err != nil {
			return nil, err
		}
		return []byte{0xDB, byte(portVal)}, nil
	}
	// IN r, (C)
	if op1.Mode == ModeReg8 && op2.Mode == ModeMemIndC {
		return []byte{0xED, 0x40 | (op1.Reg8 << 3)}, nil
	}
	return nil, fmt.Errorf("invalid operands for IN")
}

func (a *Assembler) encodeOUT(op1, op2 Operand) ([]byte, error) {
	// OUT (n), A
	if op1.Mode == ModeDirectMem && op2.Mode == ModeReg8 && op2.Reg8 == 7 {
		portVal, err := a.evalExpr(op1.Expr)
		if err != nil {
			return nil, err
		}
		return []byte{0xD3, byte(portVal)}, nil
	}
	// OUT (C), r
	if op1.Mode == ModeMemIndC && op2.Mode == ModeReg8 {
		return []byte{0xED, 0x41 | (op2.Reg8 << 3)}, nil
	}
	return nil, fmt.Errorf("invalid operands for OUT")
}

func (a *Assembler) encodeLD(op1, op2 Operand) ([]byte, error) {
	// 1. 8-bit Register to Register: LD r, r'
	if op1.Mode == ModeReg8 && op2.Mode == ModeReg8 {
		if op1.IsIXHalf || op2.IsIXHalf {
			rDst := op1.Reg8
			rSrc := op2.Reg8
			return []byte{0xDD, 0x40 | (rDst << 3) | rSrc}, nil
		}
		if op1.IsIYHalf || op2.IsIYHalf {
			rDst := op1.Reg8
			rSrc := op2.Reg8
			return []byte{0xFD, 0x40 | (rDst << 3) | rSrc}, nil
		}
		return []byte{0x40 | (op1.Reg8 << 3) | op2.Reg8}, nil
	}

	// 2. 8-bit Immediate: LD r, n
	if op1.Mode == ModeReg8 && op2.Mode == ModeImm {
		if op1.IsIXHalf {
			return []byte{0xDD, 0x06 | (op1.Reg8 << 3), byte(op2.Val)}, nil
		}
		if op1.IsIYHalf {
			return []byte{0xFD, 0x06 | (op1.Reg8 << 3), byte(op2.Val)}, nil
		}
		return []byte{0x06 | (op1.Reg8 << 3), byte(op2.Val)}, nil
	}

	// 3. Indirect (HL): LD r, (HL) and LD (HL), r
	if op1.Mode == ModeReg8 && op2.Mode == ModeMemIndHL {
		return []byte{0x46 | (op1.Reg8 << 3)}, nil
	}
	if op1.Mode == ModeMemIndHL && op2.Mode == ModeReg8 {
		return []byte{0x70 | op2.Reg8}, nil
	}
	if op1.Mode == ModeMemIndHL && op2.Mode == ModeImm {
		return []byte{0x36, byte(op2.Val)}, nil
	}

	// 4. Indirect (BC), (DE): LD A, (BC/DE) and LD (BC/DE), A
	if op1.Mode == ModeReg8 && op1.Reg8 == 7 {
		if op2.Mode == ModeMemIndBC {
			return []byte{0x0A}, nil
		}
		if op2.Mode == ModeMemIndDE {
			return []byte{0x1A}, nil
		}
		if op2.Mode == ModeDirectMem { // LD A, (nn)
			return []byte{0x3A, byte(op2.Val & 0xFF), byte((op2.Val >> 8) & 0xFF)}, nil
		}
	}
	if op2.Mode == ModeReg8 && op2.Reg8 == 7 {
		if op1.Mode == ModeMemIndBC {
			return []byte{0x02}, nil
		}
		if op1.Mode == ModeMemIndDE {
			return []byte{0x12}, nil
		}
		if op1.Mode == ModeDirectMem { // LD (nn), A
			return []byte{0x32, byte(op1.Val & 0xFF), byte((op1.Val >> 8) & 0xFF)}, nil
		}
	}

	// 5. Index Registers with displacement: (IX+d), (IY+d)
	if op1.Mode == ModeReg8 && op2.Mode == ModeMemIndIX {
		return []byte{0xDD, 0x46 | (op1.Reg8 << 3), byte(op2.Disp)}, nil
	}
	if op1.Mode == ModeReg8 && op2.Mode == ModeMemIndIY {
		return []byte{0xFD, 0x46 | (op1.Reg8 << 3), byte(op2.Disp)}, nil
	}
	if op1.Mode == ModeMemIndIX && op2.Mode == ModeReg8 {
		return []byte{0xDD, 0x70 | op2.Reg8, byte(op1.Disp)}, nil
	}
	if op1.Mode == ModeMemIndIY && op2.Mode == ModeReg8 {
		return []byte{0xFD, 0x70 | op2.Reg8, byte(op1.Disp)}, nil
	}
	if op1.Mode == ModeMemIndIX && op2.Mode == ModeImm {
		return []byte{0xDD, 0x36, byte(op1.Disp), byte(op2.Val)}, nil
	}
	if op1.Mode == ModeMemIndIY && op2.Mode == ModeImm {
		return []byte{0xFD, 0x36, byte(op1.Disp), byte(op2.Val)}, nil
	}

	// 6. 16-bit Immediate: LD rr, nn
	if op1.Mode == ModeReg16 && op2.Mode == ModeImm {
		lo := byte(op2.Val & 0xFF)
		hi := byte((op2.Val >> 8) & 0xFF)
		if op1.Reg16 <= 3 { // BC, DE, HL, SP
			return []byte{0x01 | (op1.Reg16 << 4), lo, hi}, nil
		}
		if op1.Reg16 == 5 { // IX
			return []byte{0xDD, 0x21, lo, hi}, nil
		}
		if op1.Reg16 == 6 { // IY
			return []byte{0xFD, 0x21, lo, hi}, nil
		}
	}

	// 7. 16-bit Indirect: LD HL, (nn) / LD rr, (nn) / LD (nn), HL / LD (nn), rr
	if op1.Mode == ModeReg16 && op2.Mode == ModeDirectMem {
		lo := byte(op2.Val & 0xFF)
		hi := byte((op2.Val >> 8) & 0xFF)
		if op1.Reg16 == 2 { // LD HL, (nn)
			return []byte{0x2A, lo, hi}, nil
		}
		if op1.Reg16 <= 3 { // LD rr, (nn)
			return []byte{0xED, 0x4B | (op1.Reg16 << 4), lo, hi}, nil
		}
		if op1.Reg16 == 5 { // LD IX, (nn)
			return []byte{0xDD, 0x2A, lo, hi}, nil
		}
		if op1.Reg16 == 6 { // LD IY, (nn)
			return []byte{0xFD, 0x2A, lo, hi}, nil
		}
	}
	if op1.Mode == ModeDirectMem && op2.Mode == ModeReg16 {
		lo := byte(op1.Val & 0xFF)
		hi := byte((op1.Val >> 8) & 0xFF)
		if op2.Reg16 == 2 { // LD (nn), HL
			return []byte{0x22, lo, hi}, nil
		}
		if op2.Reg16 <= 3 { // LD (nn), rr
			return []byte{0xED, 0x43 | (op2.Reg16 << 4), lo, hi}, nil
		}
		if op2.Reg16 == 5 { // LD (nn), IX
			return []byte{0xDD, 0x22, lo, hi}, nil
		}
		if op2.Reg16 == 6 { // LD (nn), IY
			return []byte{0xFD, 0x22, lo, hi}, nil
		}
	}

	// 8. LD SP, HL / IX / IY
	if op1.Mode == ModeReg16 && op1.Reg16 == 3 {
		if op2.Mode == ModeReg16 {
			if op2.Reg16 == 2 { // LD SP, HL
				return []byte{0xF9}, nil
			}
			if op2.Reg16 == 5 { // LD SP, IX
				return []byte{0xDD, 0xF9}, nil
			}
			if op2.Reg16 == 6 { // LD SP, IY
				return []byte{0xFD, 0xF9}, nil
			}
		}
	}

	return nil, fmt.Errorf("invalid operands for LD: %s, %s", op1.Expr, op2.Expr)
}

func (a *Assembler) EmitDECB(w io.Writer) error {
	// Hatvan Executable Magic header (Tag 253 = 0xFD): 'x', 'z'
	magicHdr := []byte{0xFD, 0x00, 0x00, 'x', 'z'}
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
				0x00,
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

	// Postamble trailer (Tag 255): entry point
	entry := a.entryPoint
	if !a.hasEntry {
		if sym, ok := a.symbols["cstart"]; ok {
			entry = sym
		} else if sym, ok := a.symbols["_start"]; ok {
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
		0xFF,
		0x00,
		0x00,
		byte((entry >> 8) & 0xFF),
		byte(entry & 0xFF),
	}
	_, err := w.Write(trailer)
	return err
}

func (a *Assembler) EmitRaw(w io.Writer) error {
	for _, stmt := range a.statements {
		if len(stmt.Encoded) > 0 {
			if _, err := w.Write(stmt.Encoded); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Assembler) EmitListing(w io.Writer) error {
	writer := bufio.NewWriter(w)
	defer writer.Flush()

	for _, line := range a.sourceLines {
		var hexStr string
		if len(line.Encoded) > 0 {
			hexParts := make([]string, len(line.Encoded))
			for i, b := range line.Encoded {
				hexParts[i] = fmt.Sprintf("%02X", b)
			}
			hexStr = strings.Join(hexParts, " ")
		}

		addrStr := "    "
		if line.HasPC {
			addrStr = fmt.Sprintf("%04X", line.PC)
		}

		if len(hexStr) > 20 {
			fmt.Fprintf(writer, "%s  %-20s  (%d) %s\n", addrStr, hexStr[:20], line.LineNum, line.Raw)
			rem := hexStr[20:]
			for len(rem) > 0 {
				chunk := rem
				if len(chunk) > 20 {
					chunk = rem[:20]
					rem = rem[20:]
				} else {
					rem = ""
				}
				fmt.Fprintf(writer, "      %-20s\n", chunk)
			}
		} else {
			fmt.Fprintf(writer, "%s  %-20s  (%d) %s\n", addrStr, hexStr, line.LineNum, line.Raw)
		}
	}
	return nil
}

func main() {
	outFlag := flag.String("o", "", "Output binary file")
	listFlag := flag.String("l", "", "Output listing file")
	formatFlag := flag.String("format", "decb", "Output format (decb, raw, hex)")
	relaxFlag := flag.Bool("relax", true, "Enable branch relaxation (JMP -> JR optimization)")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: asmz80 -o <output> [-l <listing.list>] [-format=decb|raw] <source.asm>...\n")
		os.Exit(1)
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
	}

	switch format {
	case "raw":
		if err := asm.EmitRaw(outWriter); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting raw binary: %v\n", err)
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
