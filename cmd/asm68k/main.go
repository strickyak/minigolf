package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OpSize represents operand size: Byte (1), Word (2), Long (4)
type OpSize int

const (
	SizeByte OpSize = 1
	SizeWord OpSize = 2
	SizeLong OpSize = 4
)

// EA represents an Effective Address
type EA struct {
	Mode   uint8
	Reg    uint8
	ExtLen int      // bytes of extension data
	ExtWords []uint16 // extension words
	SymName string   // if unresolved symbol
	SymDisp int32    // displacement added to symbol
	IsPCRel bool     // PC-relative
	BranchTarget string // for branch instructions
}

type SourceLine struct {
	File     string
	LineNum  int
	Raw      string
	PC       uint32
	HasPC    bool
	Encoded  []byte
}

type Statement struct {
	Label    string
	Mnemonic string
	Size     OpSize
	OpsStr   string
	LineNum  int
	File     string
	PC       uint32
	Encoded  []byte
	SrcLine  *SourceLine
}

type Assembler struct {
	symbols     map[string]uint32
	statements  []*Statement
	sourceLines []*SourceLine
	currPC      uint32
	entryPoint  uint32
	hasEntry    bool
	pass        int
}

func NewAssembler() *Assembler {
	return &Assembler{
		symbols: make(map[string]uint32),
		currPC:  0x00001000, // Default load address
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

	// Handle hex $ prefix
	if strings.HasPrefix(expr, "$") {
		val, err := strconv.ParseUint(expr[1:], 16, 64)
		return int64(val), err
	}
	if strings.HasPrefix(expr, "0x") || strings.HasPrefix(expr, "0X") {
		val, err := strconv.ParseUint(expr[2:], 16, 64)
		return int64(val), err
	}

	// Character constant 'x'
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
		if len(inner) == 1 {
			return int64(inner[0]), nil
		}
	}

	// Decimal or simple symbol / symbol +/- offset
	if val, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return val, nil
	}
	if val, err := strconv.ParseUint(expr, 10, 64); err == nil {
		return int64(val), nil
	}

	// Check if symbol + offset or symbol - offset
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

	// Lookup symbol
	if val, ok := a.symbols[expr]; ok {
		return int64(val), nil
	}

	if a.pass == 1 {
		return 0, nil // Unresolved in pass 1 is ok
	}
	return 0, fmt.Errorf("undefined symbol %q", expr)
}

func parseReg(s string) (isData bool, isAddr bool, regNum uint8, ok bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "SP" {
		return false, true, 7, true
	}
	if len(s) == 2 {
		if s[0] == 'D' && s[1] >= '0' && s[1] <= '7' {
			return true, false, s[1] - '0', true
		}
		if s[0] == 'A' && s[1] >= '0' && s[1] <= '7' {
			return false, true, s[1] - '0', true
		}
	}
	return false, false, 0, false
}

func (a *Assembler) parseEA(s string, opSize OpSize) (EA, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return EA{}, fmt.Errorf("empty operand")
	}

	// 1. Direct register: Dn or An
	if isD, isA, r, ok := parseReg(s); ok {
		if isD {
			return EA{Mode: 0, Reg: r}, nil
		}
		if isA {
			return EA{Mode: 1, Reg: r}, nil
		}
	}

	// 2. Predecrement: -(An)
	if strings.HasPrefix(s, "-(") && strings.HasSuffix(s, ")") {
		inner := s[2 : len(s)-1]
		if _, isA, r, ok := parseReg(inner); ok && isA {
			return EA{Mode: 4, Reg: r}, nil
		}
	}

	// 3. Postincrement: (An)+
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")+") {
		inner := s[1 : len(s)-2]
		if _, isA, r, ok := parseReg(inner); ok && isA {
			return EA{Mode: 3, Reg: r}, nil
		}
	}

	// 4. Register Indirect or Displacement: (An) or d(An) or (d, An) or d(An, Xi)
	if strings.HasSuffix(s, ")") {
		openIdx := strings.Index(s, "(")
		if openIdx != -1 {
			prefix := strings.TrimSpace(s[:openIdx])
			inner := strings.TrimSpace(s[openIdx+1 : len(s)-1])
			parts := strings.Split(inner, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}

			// Case: (An)
			if prefix == "" && len(parts) == 1 {
				if _, isA, r, ok := parseReg(parts[0]); ok && isA {
					return EA{Mode: 2, Reg: r}, nil
				}
			}

			// Case: d(An)
			if prefix != "" && len(parts) == 1 {
				if _, isA, r, ok := parseReg(parts[0]); ok && isA {
					disp, err := a.evalExpr(prefix)
					if err != nil && a.pass == 2 {
						return EA{}, fmt.Errorf("invalid displacement %q: %w", prefix, err)
					}
					return EA{
						Mode:     5,
						Reg:      r,
						ExtLen:   2,
						ExtWords: []uint16{uint16(int16(disp))},
					}, nil
				}
			}

			// Case: (d, An)
			if prefix == "" && len(parts) == 2 {
				if _, isA, r, ok := parseReg(parts[1]); ok && isA {
					disp, err := a.evalExpr(parts[0])
					if err != nil && a.pass == 2 {
						return EA{}, fmt.Errorf("invalid displacement %q: %w", parts[0], err)
					}
					return EA{
						Mode:     5,
						Reg:      r,
						ExtLen:   2,
						ExtWords: []uint16{uint16(int16(disp))},
					}, nil
				}
			}

			// Case: (d, An, Xi) or d(An, Xi)
			var dispStr string
			var anStr string
			var xiStr string
			if prefix != "" && len(parts) == 2 {
				dispStr = prefix
				anStr = parts[0]
				xiStr = parts[1]
			} else if prefix == "" && len(parts) == 3 {
				dispStr = parts[0]
				anStr = parts[1]
				xiStr = parts[2]
			}

			if anStr != "" {
				if _, isA, r, ok := parseReg(anStr); ok && isA {
					disp, _ := a.evalExpr(dispStr)
					// Parse Xi (e.g. D1, A1, D1.W, D1.L)
					xiParts := strings.Split(xiStr, ".")
					isLong := true
					if len(xiParts) == 2 && strings.ToUpper(xiParts[1]) == "W" {
						isLong = false
					}
					_, isXiA, xiReg, xiOk := parseReg(xiParts[0])
					if xiOk {
						var extWord uint16
						if isXiA {
							extWord |= 0x8000
						}
						extWord |= uint16(xiReg) << 12
						if isLong {
							extWord |= 0x0800
						}
						extWord |= uint16(byte(int8(disp)))
						return EA{
							Mode:     6,
							Reg:      r,
							ExtLen:   2,
							ExtWords: []uint16{extWord},
						}, nil
					}
				}
			}
		}
	}

	// 5. Immediate: #<expr>
	if strings.HasPrefix(s, "#") {
		val, err := a.evalExpr(s[1:])
		if err != nil && a.pass == 2 {
			return EA{}, fmt.Errorf("invalid immediate %q: %w", s, err)
		}
		if opSize == SizeLong {
			hi := uint16(uint32(val) >> 16)
			lo := uint16(uint32(val) & 0xFFFF)
			return EA{
				Mode:     7,
				Reg:      4,
				ExtLen:   4,
				ExtWords: []uint16{hi, lo},
			}, nil
		}
		// Byte or Word immediate is a 16-bit extension word
		return EA{
			Mode:     7,
			Reg:      4,
			ExtLen:   2,
			ExtWords: []uint16{uint16(val & 0xFFFF)},
		}, nil
	}

	// 6. Absolute Address: ($1000).W or ($1000).L or label / expression
	// Default to Absolute Long (mode 7, reg 1)
	val, err := a.evalExpr(s)
	if err != nil && a.pass == 2 {
		return EA{}, fmt.Errorf("cannot resolve address %q: %w", s, err)
	}
	uval := uint32(val)
	hi := uint16(uval >> 16)
	lo := uint16(uval & 0xFFFF)
	return EA{
		Mode:     7,
		Reg:      1,
		ExtLen:   4,
		ExtWords: []uint16{hi, lo},
	}, nil
}

func parseOperands(s string) []string {
	var ops []string
	var cur strings.Builder
	inParen := 0
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' || ch == '"' {
			inQuote = !inQuote
			cur.WriteByte(ch)
			continue
		}
		if !inQuote {
			if ch == '(' {
				inParen++
			} else if ch == ')' {
				inParen--
			} else if ch == ',' && inParen == 0 {
				ops = append(ops, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		ops = append(ops, strings.TrimSpace(cur.String()))
	}
	return ops
}

func (a *Assembler) assembleLine(st *Statement) ([]byte, error) {
	mnem := strings.ToUpper(st.Mnemonic)
	size := st.Size
	ops := parseOperands(st.OpsStr)

	// Directives
	switch mnem {
	case "ORG":
		val, err := a.evalExpr(ops[0])
		if err != nil {
			return nil, err
		}
		a.currPC = uint32(val)
		return nil, nil

	case "EVEN":
		if a.currPC%2 != 0 {
			return []byte{0}, nil
		}
		return nil, nil

	case "DS":
		count, err := a.evalExpr(ops[0])
		if err != nil {
			return nil, err
		}
		multiplier := 1
		if size == SizeWord {
			multiplier = 2
		} else if size == SizeLong {
			multiplier = 4
		}
		return make([]byte, int(count)*multiplier), nil

	case "DC":
		var buf bytes.Buffer
		for _, op := range ops {
			if size == SizeByte && len(op) >= 2 && op[0] == '"' && op[len(op)-1] == '"' {
				strVal := op[1 : len(op)-1]
				buf.WriteString(strVal)
			} else {
				val, err := a.evalExpr(op)
				if err != nil && a.pass == 2 {
					return nil, err
				}
				if size == SizeByte {
					buf.WriteByte(byte(val))
				} else if size == SizeWord {
					w := uint16(val)
					buf.WriteByte(byte(w >> 8))
					buf.WriteByte(byte(w))
				} else {
					l := uint32(val)
					buf.WriteByte(byte(l >> 24))
					buf.WriteByte(byte(l >> 16))
					buf.WriteByte(byte(l >> 8))
					buf.WriteByte(byte(l))
				}
			}
		}
		return buf.Bytes(), nil

	case "END":
		if len(ops) > 0 && ops[0] != "" {
			val, err := a.evalExpr(ops[0])
			if err == nil {
				a.entryPoint = uint32(val)
				a.hasEntry = true
			}
		}
		return nil, nil
	}

	// Instructions
	var out []uint16

	switch mnem {
	case "NOP":
		out = append(out, 0x4E71)
	case "RTS":
		out = append(out, 0x4E75)
	case "RTR":
		out = append(out, 0x4E77)
	case "RTE":
		out = append(out, 0x4E73)
	case "STOP":
		val, _ := a.evalExpr(ops[0])
		out = append(out, 0x4E72, uint16(val))

	case "SWAP":
		_, _, r, _ := parseReg(ops[0])
		out = append(out, 0x4840|uint16(r))

	case "EXT":
		_, _, r, _ := parseReg(ops[0])
		base := uint16(0x4880) // EXT.W
		if size == SizeLong {
			base = 0x48C0 // EXT.L
		}
		out = append(out, base|uint16(r))

	case "LINK":
		_, _, r, _ := parseReg(ops[0])
		disp, _ := a.evalExpr(ops[1])
		out = append(out, 0x4E50|uint16(r), uint16(int16(disp)))

	case "UNLK":
		_, _, r, _ := parseReg(ops[0])
		out = append(out, 0x4E58|uint16(r))

	case "PEA":
		ea, err := a.parseEA(ops[0], SizeLong)
		if err != nil {
			return nil, err
		}
		out = append(out, 0x4840|uint16(ea.Mode<<3)|uint16(ea.Reg))
		out = append(out, ea.ExtWords...)

	case "LEA":
		srcEA, err := a.parseEA(ops[0], SizeLong)
		if err != nil {
			return nil, err
		}
		_, _, dstR, _ := parseReg(ops[1])
		out = append(out, 0x41C0|(uint16(dstR)<<9)|(uint16(srcEA.Mode)<<3)|uint16(srcEA.Reg))
		out = append(out, srcEA.ExtWords...)

	case "JSR":
		ea, err := a.parseEA(ops[0], SizeLong)
		if err != nil {
			return nil, err
		}
		out = append(out, 0x4E80|(uint16(ea.Mode)<<3)|uint16(ea.Reg))
		out = append(out, ea.ExtWords...)

	case "JMP":
		ea, err := a.parseEA(ops[0], SizeLong)
		if err != nil {
			return nil, err
		}
		out = append(out, 0x4EC0|(uint16(ea.Mode)<<3)|uint16(ea.Reg))
		out = append(out, ea.ExtWords...)

	case "CLR", "NEG", "NOT", "TST":
		ea, err := a.parseEA(ops[0], size)
		if err != nil {
			return nil, err
		}
		var base uint16
		switch mnem {
		case "CLR":
			base = 0x4200
		case "NEG":
			base = 0x4400
		case "NOT":
			base = 0x4600
		case "TST":
			base = 0x4A00
		}
		var szBits uint16 = 1 // word
		if size == SizeByte {
			szBits = 0
		} else if size == SizeLong {
			szBits = 2
		}
		out = append(out, base|(szBits<<6)|(uint16(ea.Mode)<<3)|uint16(ea.Reg))
		out = append(out, ea.ExtWords...)

	case "MOVEQ":
		val, _ := a.evalExpr(ops[0])
		_, _, r, _ := parseReg(ops[1])
		out = append(out, 0x7000|(uint16(r)<<9)|uint16(byte(val)))

	case "MOVE", "MOVEA":
		srcEA, err := a.parseEA(ops[0], size)
		if err != nil {
			return nil, err
		}
		dstEA, err := a.parseEA(ops[1], size)
		if err != nil {
			return nil, err
		}

		var szBits uint16
		switch size {
		case SizeByte:
			szBits = 1 // 01
		case SizeWord:
			szBits = 3 // 11
		case SizeLong:
			szBits = 2 // 10
		}

		// Destination EA in MOVE is dst_reg[11:9] | dst_mode[8:6]
		op := (szBits << 12) | (uint16(dstEA.Reg) << 9) | (uint16(dstEA.Mode) << 6) | (uint16(srcEA.Mode) << 3) | uint16(srcEA.Reg)
		out = append(out, op)
		out = append(out, srcEA.ExtWords...)
		out = append(out, dstEA.ExtWords...)

	case "ADDQ", "SUBQ":
		val, _ := a.evalExpr(ops[0])
		dstEA, err := a.parseEA(ops[1], size)
		if err != nil {
			return nil, err
		}
		var szBits uint16
		switch size {
		case SizeByte:
			szBits = 0
		case SizeWord:
			szBits = 1
		case SizeLong:
			szBits = 2
		}
		isSub := uint16(0)
		if mnem == "SUBQ" {
			isSub = 1
		}
		data := uint16(val & 7)
		op := 0x5000 | (data << 9) | (isSub << 8) | (szBits << 6) | (uint16(dstEA.Mode) << 3) | uint16(dstEA.Reg)
		out = append(out, op)
		out = append(out, dstEA.ExtWords...)

	case "MULU", "MULS", "DIVU", "DIVS":
		srcEA, err := a.parseEA(ops[0], SizeWord)
		if err != nil {
			return nil, err
		}
		_, _, dReg, _ := parseReg(ops[1])
		var base uint16
		switch mnem {
		case "MULU":
			base = 0xC0C0
		case "MULS":
			base = 0xC1C0
		case "DIVU":
			base = 0x80C0
		case "DIVS":
			base = 0x81C0
		}
		out = append(out, base|(uint16(dReg)<<9)|(uint16(srcEA.Mode)<<3)|uint16(srcEA.Reg))
		out = append(out, srcEA.ExtWords...)

	case "ADDX", "SUBX":
		_, _, rx, _ := parseReg(ops[1])
		_, _, ry, _ := parseReg(ops[0])
		var szBits uint16 = 2 // long default
		if size == SizeByte {
			szBits = 0
		} else if size == SizeWord {
			szBits = 1
		}
		base := uint16(0xD100) // ADDX
		if mnem == "SUBX" {
			base = 0x9100
		}
		out = append(out, base|(uint16(rx)<<9)|(szBits<<6)|uint16(ry))

	case "ADD", "ADDA", "SUB", "SUBA", "CMP", "CMPA", "AND", "OR", "EOR":
		// Check if destination is An -> ADDA / SUBA / CMPA
		_, isA, aReg, dstIsA := parseReg(ops[1])
		if dstIsA && isA {
			srcEA, err := a.parseEA(ops[0], size)
			if err != nil {
				return nil, err
			}
			var base uint16
			switch mnem {
			case "ADD", "ADDA":
				base = 0xD0C0
			case "SUB", "SUBA":
				base = 0x90C0
			case "CMP", "CMPA":
				base = 0xB0C0
			}
			isLong := uint16(1)
			if size == SizeWord {
				isLong = 0
			}
			out = append(out, base|(uint16(aReg)<<9)|(isLong<<8)|(uint16(srcEA.Mode)<<3)|uint16(srcEA.Reg))
			out = append(out, srcEA.ExtWords...)
			break
		}

		// Immediate operation check: if src is #imm and dst is not Dn (or for CMPI / ADDI / SUBI)
		if strings.HasPrefix(ops[0], "#") {
			immVal, _ := a.evalExpr(ops[0][1:])
			dstEA, err := a.parseEA(ops[1], size)
			if err != nil {
				return nil, err
			}
			var immBase uint16
			switch mnem {
			case "ADD":
				immBase = 0x0600
			case "SUB":
				immBase = 0x0400
			case "CMP":
				immBase = 0x0C00
			case "AND":
				immBase = 0x0200
			case "OR":
				immBase = 0x0000
			case "EOR":
				immBase = 0x0A00
			}
			var szBits uint16 = 1 // word
			if size == SizeByte {
				szBits = 0
			} else if size == SizeLong {
				szBits = 2
			}
			out = append(out, immBase|(szBits<<6)|(uint16(dstEA.Mode)<<3)|uint16(dstEA.Reg))
			if size == SizeLong {
				out = append(out, uint16(uint32(immVal)>>16), uint16(uint32(immVal)&0xFFFF))
			} else {
				out = append(out, uint16(immVal&0xFFFF))
			}
			out = append(out, dstEA.ExtWords...)
			break
		}

		// Standard Dn, <ea> or <ea>, Dn
		srcEA, err := a.parseEA(ops[0], size)
		if err != nil {
			return nil, err
		}
		dstEA, err := a.parseEA(ops[1], size)
		if err != nil {
			return nil, err
		}

		var szBits uint16 = 1
		if size == SizeByte {
			szBits = 0
		} else if size == SizeLong {
			szBits = 2
		}

		var base uint16
		switch mnem {
		case "ADD":
			base = 0xD000
		case "SUB":
			base = 0x9000
		case "CMP":
			base = 0xB000
		case "AND":
			base = 0xC000
		case "OR":
			base = 0x8000
		case "EOR":
			base = 0xB100
		}

		if mnem == "EOR" {
			if srcEA.Mode != 0 {
				return nil, fmt.Errorf("EOR source must be data register: %s", ops[0])
			}
			out = append(out, base|(uint16(srcEA.Reg)<<9)|(szBits<<6)|(uint16(dstEA.Mode)<<3)|uint16(dstEA.Reg))
			out = append(out, dstEA.ExtWords...)
		} else if dstEA.Mode == 0 { // <ea>, Dn
			out = append(out, base|(uint16(dstEA.Reg)<<9)|(szBits<<6)|(uint16(srcEA.Mode)<<3)|uint16(srcEA.Reg))
			out = append(out, srcEA.ExtWords...)
		} else if srcEA.Mode == 0 { // Dn, <ea>
			out = append(out, base|(uint16(srcEA.Reg)<<9)|(1<<8)|(szBits<<6)|(uint16(dstEA.Mode)<<3)|uint16(dstEA.Reg))
			out = append(out, dstEA.ExtWords...)
		} else {
			return nil, fmt.Errorf("invalid operands for %s: %s, %s", mnem, ops[0], ops[1])
		}

	case "LSL", "LSR", "ASL", "ASR", "ROL", "ROR":
		// Register shift: cnt, Dn
		_, _, dstR, _ := parseReg(ops[1])
		var szBits uint16 = 1
		if size == SizeByte {
			szBits = 0
		} else if size == SizeLong {
			szBits = 2
		}
		isLeft := uint16(0)
		if mnem == "LSL" || mnem == "ASL" || mnem == "ROL" {
			isLeft = 1
		}
		var typeBits uint16
		switch mnem {
		case "ASL", "ASR":
			typeBits = 0
		case "LSL", "LSR":
			typeBits = 1
		case "ROL", "ROR":
			typeBits = 3
		}

		if strings.HasPrefix(ops[0], "#") {
			cnt, _ := a.evalExpr(ops[0][1:])
			cntBits := uint16(cnt & 7)
			out = append(out, 0xE000|(cntBits<<9)|(isLeft<<8)|(szBits<<6)|(0<<5)|(typeBits<<3)|uint16(dstR))
		} else {
			_, _, cntR, _ := parseReg(ops[0])
			out = append(out, 0xE000|(uint16(cntR)<<9)|(isLeft<<8)|(szBits<<6)|(1<<5)|(typeBits<<3)|uint16(dstR))
		}

	case "DBRA", "DBF":
		_, _, r, _ := parseReg(ops[0])
		targetVal, err := a.evalExpr(ops[1])
		disp := int16(0)
		if err == nil {
			disp = int16(int32(targetVal) - int32(a.currPC+2))
		}
		out = append(out, 0x51C8|uint16(r), uint16(disp))

	default:
		// Branches: BRA, BSR, Bcc
		condMap := map[string]uint16{
			"BRA": 0, "BSR": 1, "BHI": 2, "BLS": 3,
			"BCC": 4, "BHS": 4, "BCS": 5, "BLO": 5,
			"BNE": 6, "BEQ": 7, "BVC": 8, "BVS": 9,
			"BPL": 10, "BMI": 11, "BGE": 12, "BLT": 13,
			"BGT": 14, "BLE": 15,
		}
		if cond, ok := condMap[mnem]; ok {
			targetVal, err := a.evalExpr(ops[0])
			disp := int16(0)
			if err == nil {
				disp = int16(int32(targetVal) - int32(a.currPC+2))
			}
			// Always emit 16-bit displacement word: op word with disp8=0, followed by disp16
			out = append(out, 0x6000|(cond<<8), uint16(disp))
			break
		}

		return nil, fmt.Errorf("unrecognized mnemonic %q", mnem)
	}

	var res []byte
	for _, w := range out {
		res = append(res, byte(w>>8), byte(w))
	}
	return res, nil
}

func parseLine(line string) (label, mnem string, size OpSize, ops string) {
	// Strip comments (; or //)
	if idx := strings.Index(line, ";"); idx != -1 {
		line = line[:idx]
	}
	if idx := strings.Index(line, "//"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", 0, ""
	}

	// Check for leading label
	fields := strings.Fields(line)
	first := fields[0]

	if strings.HasSuffix(first, ":") {
		label = first[:len(first)-1]
		rest := strings.TrimSpace(line[len(first):])
		if rest == "" {
			return label, "", 0, ""
		}
		subL, subM, subS, subO := parseLine(rest)
		if subL != "" {
			label = label + "\n" + subL
		}
		return label, subM, subS, subO
	}

	// First token is mnemonic, possibly with .B / .W / .L
	mnemPart := first
	ops = strings.TrimSpace(line[len(first):])

	size = SizeLong // default for 68k if unspecified
	if idx := strings.Index(mnemPart, "."); idx != -1 {
		szChar := strings.ToUpper(mnemPart[idx+1:])
		mnem = mnemPart[:idx]
		switch szChar {
		case "B":
			size = SizeByte
		case "W":
			size = SizeWord
		case "L":
			size = SizeLong
		}
	} else {
		mnem = mnemPart
	}

	return label, mnem, size, ops
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

		lbl, mnem, sz, ops := parseLine(raw)
		if lbl != "" {
			labels := strings.Split(lbl, "\n")
			for _, l := range labels {
				st := &Statement{
					Label:   strings.TrimSpace(l),
					LineNum: lineNum,
					File:    filename,
					SrcLine: srcLine,
				}
				a.statements = append(a.statements, st)
			}
		}
		if mnem != "" {
			st := &Statement{
				Mnemonic: mnem,
				Size:     sz,
				OpsStr:   ops,
				LineNum:  lineNum,
				File:     filename,
				SrcLine:  srcLine,
			}
			a.statements = append(a.statements, st)
		}
	}
	return scanner.Err()
}

func (a *Assembler) Assemble() error {
	// Pass 1: compute addresses and assign symbols
	a.pass = 1
	a.currPC = 0x00001000

	for _, st := range a.statements {
		st.PC = a.currPC
		if st.Label != "" {
			a.symbols[st.Label] = a.currPC
		}
		if st.Mnemonic != "" {
			bytes, err := a.assembleLine(st)
			if err != nil {
				return fmt.Errorf("%s:%d: error in pass 1: %w", st.File, st.LineNum, err)
			}
			st.Encoded = bytes
			if st.Mnemonic == "ORG" {
				st.PC = a.currPC
			}
			a.currPC += uint32(len(bytes))
		}
	}

	// Pass 2: generate final encoded bytes with resolved symbols
	a.pass = 2
	a.currPC = 0x00001000

	for _, st := range a.statements {
		st.PC = a.currPC
		if st.SrcLine != nil && !st.SrcLine.HasPC {
			st.SrcLine.PC = a.currPC
			st.SrcLine.HasPC = true
		}
		if st.Mnemonic != "" {
			bytes, err := a.assembleLine(st)
			if err != nil {
				return fmt.Errorf("%s:%d: error in pass 2: %w", st.File, st.LineNum, err)
			}
			st.Encoded = bytes
			if st.Mnemonic == "ORG" {
				st.PC = a.currPC
				if st.SrcLine != nil {
					st.SrcLine.PC = a.currPC
					st.SrcLine.HasPC = true
				}
			}
			if st.SrcLine != nil && len(bytes) > 0 {
				st.SrcLine.Encoded = append(st.SrcLine.Encoded, bytes...)
			}
			a.currPC += uint32(len(bytes))
		}
	}

	return nil
}

func (a *Assembler) EmitSRecords(w io.Writer) error {
	// Header: S0
	headerBytes := []byte("minigolf-asm68k")
	writeSRecord(w, '0', 0, headerBytes)

	// Collect chunks of code/data by contiguous addresses
	var currentAddr uint32
	var currentBuf []byte

	flush := func() {
		if len(currentBuf) == 0 {
			return
		}
		// Break into max 32-byte chunks
		for i := 0; i < len(currentBuf); i += 32 {
			end := i + 32
			if end > len(currentBuf) {
				end = len(currentBuf)
			}
			writeSRecord(w, '3', currentAddr+uint32(i), currentBuf[i:end])
		}
		currentBuf = nil
	}

	for _, st := range a.statements {
		if len(st.Encoded) == 0 {
			continue
		}
		if len(currentBuf) > 0 && st.PC != currentAddr+uint32(len(currentBuf)) {
			flush()
		}
		if len(currentBuf) == 0 {
			currentAddr = st.PC
		}
		currentBuf = append(currentBuf, st.Encoded...)
	}
	flush()

	// Termination: S7 with entry point
	entry := a.entryPoint
	if !a.hasEntry {
		if cstartAddr, ok := a.symbols["cstart"]; ok {
			entry = cstartAddr
		} else if mainAddr, ok := a.symbols["_main"]; ok {
			entry = mainAddr
		} else {
			entry = 0x00001000
		}
	}
	writeSRecord(w, '7', entry, nil)
	return nil
}

func writeSRecord(w io.Writer, recType byte, addr uint32, data []byte) {
	// Count = address length (4 bytes for S3/S7) + data length + 1 (checksum)
	count := 4 + len(data) + 1
	var payload []byte
	payload = append(payload, byte(count))
	payload = append(payload, byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr))
	payload = append(payload, data...)

	var sum byte
	for _, b := range payload {
		sum += b
	}
	csum := ^sum

	fmt.Fprintf(w, "S%c%02X%s%02X\n", recType, count, strings.ToUpper(hex.EncodeToString(payload[1:])), csum)
}

func (a *Assembler) EmitListing(w io.Writer) error {
	for _, sl := range a.sourceLines {
		base := filepath.Base(sl.File)
		fileLineStr := fmt.Sprintf("(%17s):%05d", base, sl.LineNum)

		var addrStr string
		if sl.HasPC {
			// On the M68000, 24-bit addresses will take 6 hex characters in the first column
			addrStr = fmt.Sprintf("%06X", sl.PC&0xFFFFFF)
		} else {
			addrStr = "      "
		}

		enc := sl.Encoded
		if len(enc) == 0 {
			if _, err := fmt.Fprintf(w, "%s                  %s %s\n", addrStr, fileLineStr, sl.Raw); err != nil {
				return err
			}
			continue
		}

		chunkLen := len(enc)
		if chunkLen > 8 {
			chunkLen = 8
		}
		hexStr := strings.ToUpper(hex.EncodeToString(enc[:chunkLen]))
		if _, err := fmt.Fprintf(w, "%s %-16s %s %s\n", addrStr, hexStr, fileLineStr, sl.Raw); err != nil {
			return err
		}

		for i := 8; i < len(enc); i += 8 {
			end := i + 8
			if end > len(enc) {
				end = len(enc)
			}
			contHex := strings.ToUpper(hex.EncodeToString(enc[i:end]))
			if _, err := fmt.Fprintf(w, "       %s\n", contHex); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	outFlag := flag.String("o", "", "Output S-Record file path")
	listFlag := flag.String("l", "", "Assembly listing output file path")
	listLongFlag := flag.String("list", "", "Assembly listing output file path")
	flag.Parse()

	actualList := *listFlag
	if actualList == "" && *listLongFlag != "" {
		actualList = *listLongFlag
	}

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: asm68k -o <output.srec> [-l <output.list>] <source.s>...\n")
		os.Exit(1)
	}

	asm := NewAssembler()
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
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *outFlag, err)
			os.Exit(1)
		}
		defer outFile.Close()
		outWriter = outFile
	}

	if err := asm.EmitSRecords(outWriter); err != nil {
		fmt.Fprintf(os.Stderr, "Error emitting S-records: %v\n", err)
		os.Exit(1)
	}

	if actualList != "" {
		listFile, err := os.Create(actualList)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating listing file %s: %v\n", actualList, err)
			os.Exit(1)
		}
		defer listFile.Close()
		if err := asm.EmitListing(listFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error emitting listing: %v\n", err)
			os.Exit(1)
		}
	}
}
