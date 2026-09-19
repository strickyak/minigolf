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

// AddrMode represents a 6809 addressing mode.
type AddrMode int

const (
	ModeInherent AddrMode = iota
	ModeImmediate
	ModeDirect
	ModeExtended
	ModeIndexed
	ModeRelative
	ModeLongRelative
)

type RegID int

const (
	RegD RegID = 0
	RegX RegID = 1
	RegY RegID = 2
	RegU RegID = 3
	RegS RegID = 4
	RegPC RegID = 5
	RegA RegID = 8
	RegB RegID = 9
	RegCC RegID = 10
	RegDP RegID = 11
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
	StmtDataQuad
	StmtReserve
	StmtString
	StmtEnd
	StmtComment
)

type Statement struct {
	Type     StmtType
	Label    string
	Mnemonic string
	RawOp    string
	LineNum  int
	File     string
	PC       uint32
	Size     int
	Encoded  []byte
	SrcLine  *SourceLine

	// Directive info
	OrgAddr uint32
	EquExpr string
	DataExprs []string
	StrData string
	ReserveBytes int

	// Branch relaxation info
	IsBranch          bool
	IsLongBranch      bool
	BranchTarget      string
	ShortBranchOpcode []byte
	LongBranchOpcode  []byte

	// Direct Page relaxation info
	IsDirectRelaxable bool
	DirectSymbol      string
	DirectOffset      int64
	DirOpcode         []byte
	ExtOpcode         []byte
	DirSize           int
	ExtSize           int

	// General instruction info
	Mode       AddrMode
	Opcode     []byte
	ImmVal     int64
	ImmSize    int // 1 or 2
	TargetSym  string
	TargetDisp int64
	Postbyte   byte
	IndexDisp  int64
	IndexDispSize int // 0 (5-bit), 1 (8-bit), 2 (16-bit)
	IsPCRel    bool
	IsIndirect bool
}

// OpInfo stores encoding information for a mnemonic.
type OpInfo struct {
	Mnemonic string

	// Inherent opcode (e.g. clra, rts, mul, sex)
	Inherent []byte

	// Immediate: 8-bit or 16-bit
	Imm       []byte
	ImmIsWord bool

	// Direct, Indexed, Extended
	Dir []byte
	Idx []byte
	Ext []byte

	// For branches: short & long opcodes
	ShortBranch []byte
	LongBranch  []byte
}

var opcodes = map[string]*OpInfo{
	// Inherent instructions
	"clra":  {Inherent: []byte{0x4F}},
	"clrb":  {Inherent: []byte{0x5F}},
	"coma":  {Inherent: []byte{0x43}},
	"comb":  {Inherent: []byte{0x53}},
	"nega":  {Inherent: []byte{0x40}},
	"negb":  {Inherent: []byte{0x50}},
	"inca":  {Inherent: []byte{0x4C}},
	"incb":  {Inherent: []byte{0x5C}},
	"deca":  {Inherent: []byte{0x4A}},
	"decb":  {Inherent: []byte{0x5A}},
	"tsta":  {Inherent: []byte{0x4D}},
	"tstb":  {Inherent: []byte{0x5D}},
	"asla":  {Inherent: []byte{0x48}},
	"aslb":  {Inherent: []byte{0x58}},
	"lsla":  {Inherent: []byte{0x48}},
	"lslb":  {Inherent: []byte{0x58}},
	"asra":  {Inherent: []byte{0x47}},
	"asrb":  {Inherent: []byte{0x57}},
	"lsra":  {Inherent: []byte{0x44}},
	"lsrb":  {Inherent: []byte{0x54}},
	"rola":  {Inherent: []byte{0x49}},
	"rolb":  {Inherent: []byte{0x59}},
	"rora":  {Inherent: []byte{0x46}},
	"rorb":  {Inherent: []byte{0x56}},
	"mul":   {Inherent: []byte{0x3D}},
	"rts":   {Inherent: []byte{0x39}},
	"rti":   {Inherent: []byte{0x3B}},
	"nop":   {Inherent: []byte{0x12}},
	"sex":   {Inherent: []byte{0x1D}},
	"sync":  {Inherent: []byte{0x13}},
	"swi":   {Inherent: []byte{0x3F}},
	"swi2":  {Inherent: []byte{0x10, 0x3F}},
	"swi3":  {Inherent: []byte{0x11, 0x3F}},
	"daa":   {Inherent: []byte{0x19}},

	// Single-operand memory instructions (clr, neg, com, lsr, ror, asr, asl, lsl, rol, dec, inc, tst, jmp, jsr)
	"clr": {Dir: []byte{0x0F}, Idx: []byte{0x6F}, Ext: []byte{0x7F}},
	"neg": {Dir: []byte{0x00}, Idx: []byte{0x60}, Ext: []byte{0x70}},
	"com": {Dir: []byte{0x03}, Idx: []byte{0x63}, Ext: []byte{0x73}},
	"lsr": {Dir: []byte{0x04}, Idx: []byte{0x64}, Ext: []byte{0x74}},
	"ror": {Dir: []byte{0x06}, Idx: []byte{0x66}, Ext: []byte{0x76}},
	"asr": {Dir: []byte{0x07}, Idx: []byte{0x67}, Ext: []byte{0x77}},
	"asl": {Dir: []byte{0x08}, Idx: []byte{0x68}, Ext: []byte{0x78}},
	"lsl": {Dir: []byte{0x08}, Idx: []byte{0x68}, Ext: []byte{0x78}},
	"rol": {Dir: []byte{0x09}, Idx: []byte{0x69}, Ext: []byte{0x79}},
	"dec": {Dir: []byte{0x0A}, Idx: []byte{0x6A}, Ext: []byte{0x7A}},
	"inc": {Dir: []byte{0x0C}, Idx: []byte{0x6C}, Ext: []byte{0x7C}},
	"tst": {Dir: []byte{0x0D}, Idx: []byte{0x6D}, Ext: []byte{0x7D}},
	"jmp": {Dir: []byte{0x0E}, Idx: []byte{0x6E}, Ext: []byte{0x7E}},
	"jsr": {Dir: []byte{0x9D}, Idx: []byte{0xAD}, Ext: []byte{0xBD}},

	// Two-operand 8-bit instructions
	"suba": {Imm: []byte{0x80}, Dir: []byte{0x90}, Idx: []byte{0xA0}, Ext: []byte{0xB0}},
	"cmpa": {Imm: []byte{0x81}, Dir: []byte{0x91}, Idx: []byte{0xA1}, Ext: []byte{0xB1}},
	"sbca": {Imm: []byte{0x82}, Dir: []byte{0x92}, Idx: []byte{0xA2}, Ext: []byte{0xB2}},
	"anda": {Imm: []byte{0x84}, Dir: []byte{0x94}, Idx: []byte{0xA4}, Ext: []byte{0xB4}},
	"bita": {Imm: []byte{0x85}, Dir: []byte{0x95}, Idx: []byte{0xA5}, Ext: []byte{0xB5}},
	"lda":  {Imm: []byte{0x86}, Dir: []byte{0x96}, Idx: []byte{0xA6}, Ext: []byte{0xB6}},
	"sta":  {Dir: []byte{0x97}, Idx: []byte{0xA7}, Ext: []byte{0xB7}},
	"eora": {Imm: []byte{0x88}, Dir: []byte{0x98}, Idx: []byte{0xA8}, Ext: []byte{0xB8}},
	"adca": {Imm: []byte{0x89}, Dir: []byte{0x99}, Idx: []byte{0xA9}, Ext: []byte{0xB9}},
	"ora":  {Imm: []byte{0x8A}, Dir: []byte{0x9A}, Idx: []byte{0xAA}, Ext: []byte{0xBA}},
	"adda": {Imm: []byte{0x8B}, Dir: []byte{0x9B}, Idx: []byte{0xAB}, Ext: []byte{0xBB}},

	"subb": {Imm: []byte{0xC0}, Dir: []byte{0xD0}, Idx: []byte{0xE0}, Ext: []byte{0xF0}},
	"cmpb": {Imm: []byte{0xC1}, Dir: []byte{0xD1}, Idx: []byte{0xE1}, Ext: []byte{0xF1}},
	"sbcb": {Imm: []byte{0xC2}, Dir: []byte{0xD2}, Idx: []byte{0xE2}, Ext: []byte{0xF2}},
	"andb": {Imm: []byte{0xC4}, Dir: []byte{0xD4}, Idx: []byte{0xE4}, Ext: []byte{0xF4}},
	"bitb": {Imm: []byte{0xC5}, Dir: []byte{0xD5}, Idx: []byte{0xE5}, Ext: []byte{0xF5}},
	"ldb":  {Imm: []byte{0xC6}, Dir: []byte{0xD6}, Idx: []byte{0xE6}, Ext: []byte{0xF6}},
	"stb":  {Dir: []byte{0xD7}, Idx: []byte{0xE7}, Ext: []byte{0xF7}},
	"eorb": {Imm: []byte{0xC8}, Dir: []byte{0xD8}, Idx: []byte{0xE8}, Ext: []byte{0xF8}},
	"adcb": {Imm: []byte{0xC9}, Dir: []byte{0xD9}, Idx: []byte{0xE9}, Ext: []byte{0xF9}},
	"orb":  {Imm: []byte{0xCA}, Dir: []byte{0xDA}, Idx: []byte{0xEA}, Ext: []byte{0xFA}},
	"addb": {Imm: []byte{0xCB}, Dir: []byte{0xDB}, Idx: []byte{0xEB}, Ext: []byte{0xFB}},

	// Two-operand 16-bit instructions (Page 1)
	"subd": {Imm: []byte{0x83}, ImmIsWord: true, Dir: []byte{0x93}, Idx: []byte{0xA3}, Ext: []byte{0xB3}},
	"cmpx": {Imm: []byte{0x8C}, ImmIsWord: true, Dir: []byte{0x9C}, Idx: []byte{0xAC}, Ext: []byte{0xBC}},
	"ldx":  {Imm: []byte{0x8E}, ImmIsWord: true, Dir: []byte{0x9E}, Idx: []byte{0xAE}, Ext: []byte{0xBE}},
	"stx":  {Dir: []byte{0x9F}, Idx: []byte{0xAF}, Ext: []byte{0xBF}},
	"addd": {Imm: []byte{0xC3}, ImmIsWord: true, Dir: []byte{0xD3}, Idx: []byte{0xE3}, Ext: []byte{0xF3}},
	"ldd":  {Imm: []byte{0xCC}, ImmIsWord: true, Dir: []byte{0xDC}, Idx: []byte{0xEC}, Ext: []byte{0xFC}},
	"std":  {Dir: []byte{0xDD}, Idx: []byte{0xED}, Ext: []byte{0xFD}},
	"ldu":  {Imm: []byte{0xCE}, ImmIsWord: true, Dir: []byte{0xDE}, Idx: []byte{0xEE}, Ext: []byte{0xFE}},
	"stu":  {Dir: []byte{0xDF}, Idx: []byte{0xEF}, Ext: []byte{0xFF}},

	// Page 2 instructions (Prefix 0x10)
	"cmpd": {Imm: []byte{0x10, 0x83}, ImmIsWord: true, Dir: []byte{0x10, 0x93}, Idx: []byte{0x10, 0xA3}, Ext: []byte{0x10, 0xB3}},
	"cmpy": {Imm: []byte{0x10, 0x8C}, ImmIsWord: true, Dir: []byte{0x10, 0x9C}, Idx: []byte{0x10, 0xAC}, Ext: []byte{0x10, 0xBC}},
	"ldy":  {Imm: []byte{0x10, 0x8E}, ImmIsWord: true, Dir: []byte{0x10, 0x9E}, Idx: []byte{0x10, 0xAE}, Ext: []byte{0x10, 0xBE}},
	"sty":  {Dir: []byte{0x10, 0x9F}, Idx: []byte{0x10, 0xAF}, Ext: []byte{0x10, 0xBF}},
	"lds":  {Imm: []byte{0x10, 0xCE}, ImmIsWord: true, Dir: []byte{0x10, 0xDE}, Idx: []byte{0x10, 0xEE}, Ext: []byte{0x10, 0xFE}},
	"sts":  {Dir: []byte{0x10, 0xDF}, Idx: []byte{0x10, 0xEF}, Ext: []byte{0x10, 0xFF}},

	// Page 3 instructions (Prefix 0x11)
	"cmpu": {Imm: []byte{0x11, 0x83}, ImmIsWord: true, Dir: []byte{0x11, 0x93}, Idx: []byte{0x11, 0xA3}, Ext: []byte{0x11, 0xB3}},
	"cmps": {Imm: []byte{0x11, 0x8C}, ImmIsWord: true, Dir: []byte{0x11, 0x9C}, Idx: []byte{0x11, 0xAC}, Ext: []byte{0x11, 0xBC}},

	// LEA instructions
	"leax": {Idx: []byte{0x30}},
	"leay": {Idx: []byte{0x31}},
	"leas": {Idx: []byte{0x32}},
	"leau": {Idx: []byte{0x33}},

	// Branches: Short & Long
	"bra":  {ShortBranch: []byte{0x20}, LongBranch: []byte{0x16}},
	"lbra": {ShortBranch: []byte{0x20}, LongBranch: []byte{0x16}},
	"bsr":  {ShortBranch: []byte{0x8D}, LongBranch: []byte{0x17}},
	"lbsr": {ShortBranch: []byte{0x8D}, LongBranch: []byte{0x17}},
	"brn":  {ShortBranch: []byte{0x21}, LongBranch: []byte{0x10, 0x21}},
	"lbrn": {ShortBranch: []byte{0x21}, LongBranch: []byte{0x10, 0x21}},
	"bhi":  {ShortBranch: []byte{0x22}, LongBranch: []byte{0x10, 0x22}},
	"lbhi": {ShortBranch: []byte{0x22}, LongBranch: []byte{0x10, 0x22}},
	"bls":  {ShortBranch: []byte{0x23}, LongBranch: []byte{0x10, 0x23}},
	"lbls": {ShortBranch: []byte{0x23}, LongBranch: []byte{0x10, 0x23}},
	"bcc":  {ShortBranch: []byte{0x24}, LongBranch: []byte{0x10, 0x24}},
	"lbcc": {ShortBranch: []byte{0x24}, LongBranch: []byte{0x10, 0x24}},
	"bhs":  {ShortBranch: []byte{0x24}, LongBranch: []byte{0x10, 0x24}},
	"lbhs": {ShortBranch: []byte{0x24}, LongBranch: []byte{0x10, 0x24}},
	"bcs":  {ShortBranch: []byte{0x25}, LongBranch: []byte{0x10, 0x25}},
	"lbcs": {ShortBranch: []byte{0x25}, LongBranch: []byte{0x10, 0x25}},
	"blo":  {ShortBranch: []byte{0x25}, LongBranch: []byte{0x10, 0x25}},
	"lblo": {ShortBranch: []byte{0x25}, LongBranch: []byte{0x10, 0x25}},
	"bne":  {ShortBranch: []byte{0x26}, LongBranch: []byte{0x10, 0x26}},
	"lbne": {ShortBranch: []byte{0x26}, LongBranch: []byte{0x10, 0x26}},
	"beq":  {ShortBranch: []byte{0x27}, LongBranch: []byte{0x10, 0x27}},
	"lbeq": {ShortBranch: []byte{0x27}, LongBranch: []byte{0x10, 0x27}},
	"bvc":  {ShortBranch: []byte{0x28}, LongBranch: []byte{0x10, 0x28}},
	"lbvc": {ShortBranch: []byte{0x28}, LongBranch: []byte{0x10, 0x28}},
	"bvs":  {ShortBranch: []byte{0x29}, LongBranch: []byte{0x10, 0x29}},
	"lbvs": {ShortBranch: []byte{0x29}, LongBranch: []byte{0x10, 0x29}},
	"bpl":  {ShortBranch: []byte{0x2A}, LongBranch: []byte{0x10, 0x2A}},
	"lbpl": {ShortBranch: []byte{0x2A}, LongBranch: []byte{0x10, 0x2A}},
	"bmi":  {ShortBranch: []byte{0x2B}, LongBranch: []byte{0x10, 0x2B}},
	"lbmi": {ShortBranch: []byte{0x2B}, LongBranch: []byte{0x10, 0x2B}},
	"bge":  {ShortBranch: []byte{0x2C}, LongBranch: []byte{0x10, 0x2C}},
	"lbge": {ShortBranch: []byte{0x2C}, LongBranch: []byte{0x10, 0x2C}},
	"blt":  {ShortBranch: []byte{0x2D}, LongBranch: []byte{0x10, 0x2D}},
	"lblt": {ShortBranch: []byte{0x2D}, LongBranch: []byte{0x10, 0x2D}},
	"bgt":  {ShortBranch: []byte{0x2E}, LongBranch: []byte{0x10, 0x2E}},
	"lbgt": {ShortBranch: []byte{0x2E}, LongBranch: []byte{0x10, 0x2E}},
	"ble":  {ShortBranch: []byte{0x2F}, LongBranch: []byte{0x10, 0x2F}},
	"lble": {ShortBranch: []byte{0x2F}, LongBranch: []byte{0x10, 0x2F}},
}

type Assembler struct {
	symbols     map[string]int64
	statements  []*Statement
	sourceLines []*SourceLine
	currPC      uint32
	entryPoint  uint32
	hasEntry    bool
	dp          uint8
	enableRelax bool
}

func NewAssembler() *Assembler {
	return &Assembler{
		symbols:     make(map[string]int64),
		currPC:      0x8000,
		entryPoint:  0x8000,
		dp:          0x00,
		enableRelax: true,
	}
}

func (a *Assembler) evalSimple(token string) (int64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, fmt.Errorf("empty operand")
	}
	if token == "*" {
		return int64(a.currPC), nil
	}
	neg := false
	if strings.HasPrefix(token, "-") {
		neg = true
		token = strings.TrimSpace(token[1:])
	} else if strings.HasPrefix(token, "+") {
		token = strings.TrimSpace(token[1:])
	}

	// Hex: $FF or 0xFF
	if strings.HasPrefix(token, "$") {
		v, err := strconv.ParseUint(token[1:], 16, 64)
		if neg {
			return -int64(v), err
		}
		return int64(v), err
	}
	if strings.HasPrefix(token, "0x") || strings.HasPrefix(token, "0X") {
		v, err := strconv.ParseUint(token[2:], 16, 64)
		if neg {
			return -int64(v), err
		}
		return int64(v), err
	}
	// Binary: %0101
	if strings.HasPrefix(token, "%") {
		v, err := strconv.ParseUint(token[1:], 2, 64)
		if neg {
			return -int64(v), err
		}
		return int64(v), err
	}
	// Character: 'c or 'c'
	if strings.HasPrefix(token, "'") {
		inner := token[1:]
		if strings.HasSuffix(inner, "'") && len(inner) > 1 {
			inner = inner[:len(inner)-1]
		}
		var val int64
		switch inner {
		case "\\n":
			val = '\n'
		case "\\t":
			val = '\t'
		case "\\0":
			val = 0
		case "\\r":
			val = '\r'
		default:
			if len(inner) == 1 {
				val = int64(inner[0])
			} else {
				return 0, fmt.Errorf("invalid character constant: %s", token)
			}
		}
		if neg {
			return -val, nil
		}
		return val, nil
	}
	// Decimal integer
	if v, err := strconv.ParseInt(token, 10, 64); err == nil {
		if neg {
			return -v, nil
		}
		return v, nil
	}
	if u, err := strconv.ParseUint(token, 10, 64); err == nil {
		v := int64(u)
		if neg {
			return -v, nil
		}
		return v, nil
	}
	// Symbol lookup
	if !neg {
		if val, ok := a.symbols[token]; ok {
			return val, nil
		}
	} else {
		if val, ok := a.symbols[token]; ok {
			return -val, nil
		}
	}
	if neg {
		return 0, fmt.Errorf("unresolved symbol: -%s", token)
	}
	return 0, fmt.Errorf("unresolved symbol: %s", token)
}

func (a *Assembler) evalExpr(expr string) (int64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Handle addition and subtraction: sym + N or sym - N
	// Scan from right to left to find operator outside quotes/parens
	for i := len(expr) - 1; i > 0; i-- {
		ch := expr[i]
		if ch == '+' || ch == '-' {
			left := strings.TrimSpace(expr[:i])
			right := strings.TrimSpace(expr[i+1:])
			if left != "" && right != "" {
				lVal, errL := a.evalExpr(left)
				if errL != nil {
					return 0, errL
				}
				rVal, errR := a.evalExpr(right)
				if errR != nil {
					return 0, errR
				}
				if ch == '+' {
					return lVal + rVal, nil
				}
				return lVal - rVal, nil
			}
		}
	}

	return a.evalSimple(expr)
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

		// Parse line
		stmt, err := a.parseLine(raw, lineNum, filename, srcLine)
		if err != nil {
			return fmt.Errorf("%s:%d: %v", filename, lineNum, err)
		}
		if stmt != nil {
			a.statements = append(a.statements, stmt)
		}
	}

	return scanner.Err()
}

func (a *Assembler) parseLine(raw string, lineNum int, filename string, srcLine *SourceLine) (*Statement, error) {
	line := raw
	// Strip comments: either ; or * at start of line
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, ";") {
		return &Statement{Type: StmtComment, SrcLine: srcLine, File: filename, LineNum: lineNum}, nil
	}
	if idx := strings.Index(line, ";"); idx >= 0 {
		line = line[:idx]
	}

	line = strings.TrimRight(line, " \t\r\n")
	if len(line) == 0 {
		return nil, nil
	}

	stmt := &Statement{
		LineNum: lineNum,
		File:    filename,
		SrcLine: srcLine,
	}

	// Check if label in column 0 or before ':'
	var label string
	rest := line

	// If column 0 is not whitespace
	if line[0] != ' ' && line[0] != '\t' {
		// Label present
		idx := strings.IndexAny(line, " \t:")
		if idx >= 0 {
			label = line[:idx]
			if line[idx] == ':' {
				rest = line[idx+1:]
			} else {
				rest = line[idx:]
			}
		} else {
			label = line
			rest = ""
		}
	} else {
		// Might have label followed by ':'
		trimmedRest := strings.TrimSpace(line)
		if colonIdx := strings.Index(trimmedRest, ":"); colonIdx >= 0 {
			candidate := trimmedRest[:colonIdx]
			if !strings.ContainsAny(candidate, " \t") {
				label = candidate
				rest = trimmedRest[colonIdx+1:]
			}
		}
	}

	stmt.Label = label
	rest = strings.TrimSpace(rest)
	if rest == "" {
		if label != "" {
			stmt.Type = StmtComment // label-only statement
			return stmt, nil
		}
		return nil, nil
	}

	// Next token is mnemonic or directive
	parts := strings.Fields(rest)
	mnemonic := strings.ToLower(parts[0])
	var opsStr string
	if len(parts) > 1 {
		opsStr = strings.TrimSpace(rest[len(parts[0]):])
	}

	stmt.Mnemonic = mnemonic
	stmt.RawOp = opsStr

	switch mnemonic {
	case "org":
		stmt.Type = StmtOrg
		val, err := a.evalExpr(opsStr)
		if err == nil {
			stmt.OrgAddr = uint32(val)
		}
		return stmt, nil

	case "equ", ".equ", "=":
		stmt.Type = StmtEqu
		stmt.EquExpr = opsStr
		return stmt, nil

	case "fcb", ".byte", "byte", "db":
		stmt.Type = StmtDataByte
		stmt.DataExprs = splitOperands(opsStr)
		stmt.Size = len(stmt.DataExprs)
		return stmt, nil

	case "fdb", ".word", "word", "dw":
		stmt.Type = StmtDataWord
		stmt.DataExprs = splitOperands(opsStr)
		stmt.Size = len(stmt.DataExprs) * 2
		return stmt, nil

	case "fqb", ".quad", "quad":
		stmt.Type = StmtDataQuad
		stmt.DataExprs = splitOperands(opsStr)
		stmt.Size = len(stmt.DataExprs) * 4
		return stmt, nil

	case "rmb", "zmb", ".space":
		stmt.Type = StmtReserve
		val, err := a.evalExpr(opsStr)
		if err == nil {
			stmt.ReserveBytes = int(val)
			stmt.Size = stmt.ReserveBytes
		}
		return stmt, nil

	case "fill":
		stmt.Type = StmtReserve
		subOps := splitOperands(opsStr)
		if len(subOps) >= 2 {
			count, _ := a.evalExpr(subOps[1])
			stmt.ReserveBytes = int(count)
			stmt.Size = stmt.ReserveBytes
		}
		return stmt, nil

	case ".asciz", "asciz":
		stmt.Type = StmtString
		str := parseStringLiteral(opsStr)
		stmt.StrData = str + "\x00"
		stmt.Size = len(stmt.StrData)
		return stmt, nil

	case "fcc", ".ascii", "ascii":
		stmt.Type = StmtString
		stmt.StrData = parseStringLiteral(opsStr)
		stmt.Size = len(stmt.StrData)
		return stmt, nil

	case "pragma", ".globl", ".global":
		// Ignored directives
		stmt.Type = StmtComment
		return stmt, nil

	case "end":
		stmt.Type = StmtEnd
		if opsStr != "" {
			stmt.BranchTarget = opsStr
		}
		return stmt, nil
	}

	// Otherwise, it's an instruction
	stmt.Type = StmtInstruction
	if err := a.setupInstruction(stmt); err != nil {
		return nil, err
	}

	return stmt, nil
}

func splitOperands(s string) []string {
	var res []string
	var cur strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inQuote = !inQuote
		}
		if c == ',' && !inQuote {
			res = append(res, strings.TrimSpace(cur.String()))
			cur.Reset()
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		res = append(res, strings.TrimSpace(cur.String()))
	}
	return res
}

func parseStringLiteral(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '/' && s[len(s)-1] == '/')) {
		inner := s[1 : len(s)-1]
		// Unescape standard C escapes
		var b strings.Builder
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				i++
				switch inner[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '0':
					b.WriteByte(0)
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				default:
					b.WriteByte(inner[i])
				}
			} else {
				b.WriteByte(inner[i])
			}
		}
		return b.String()
	}
	return s
}

func (a *Assembler) setupInstruction(stmt *Statement) error {
	m := stmt.Mnemonic
	op := strings.TrimSpace(stmt.RawOp)

	info, ok := opcodes[m]
	if !ok {
		// Check for pseudo-ops: tfr, exg, pshs, puls, pshu, pulu, andcc, orcc
		if m == "tfr" || m == "exg" {
			stmt.Size = 2
			return nil
		}
		if m == "pshs" || m == "puls" || m == "pshu" || m == "pulu" {
			stmt.Size = 2
			return nil
		}
		if m == "andcc" || m == "orcc" {
			stmt.Size = 2
			return nil
		}
		return fmt.Errorf("unknown mnemonic: %s", m)
	}

	// 1. Inherent (no operand)
	if op == "" {
		if len(info.Inherent) > 0 {
			stmt.Mode = ModeInherent
			stmt.Opcode = info.Inherent
			stmt.Size = len(info.Inherent)
			return nil
		}
		return fmt.Errorf("instruction %s requires operand", m)
	}

	// 2. Branch instructions
	if len(info.ShortBranch) > 0 || len(info.LongBranch) > 0 {
		stmt.IsBranch = true
		stmt.BranchTarget = op
		stmt.ShortBranchOpcode = info.ShortBranch
		stmt.LongBranchOpcode = info.LongBranch

		// By default, start with long branch if relaxable, or user explicitly requested
		if a.enableRelax && len(info.LongBranch) > 0 {
			stmt.IsLongBranch = true
			stmt.Opcode = info.LongBranch
			// lbra/lbsr: 3 bytes (1 opcode + 2 offset)
			// lbcc: 4 bytes (2 opcode + 2 offset)
			stmt.Size = len(info.LongBranch) + 2
		} else if len(info.ShortBranch) > 0 {
			stmt.IsLongBranch = false
			stmt.Opcode = info.ShortBranch
			stmt.Size = len(info.ShortBranch) + 1
		} else {
			stmt.IsLongBranch = true
			stmt.Opcode = info.LongBranch
			stmt.Size = len(info.LongBranch) + 2
		}
		return nil
	}

	// 3. Immediate mode: #expr
	if strings.HasPrefix(op, "#") {
		if len(info.Imm) == 0 {
			return fmt.Errorf("%s does not support immediate addressing", m)
		}
		stmt.Mode = ModeImmediate
		stmt.Opcode = info.Imm
		expr := op[1:]
		val, err := a.evalExpr(expr)
		if err == nil {
			stmt.ImmVal = val
		} else {
			stmt.TargetSym = expr
		}
		if info.ImmIsWord {
			stmt.ImmSize = 2
		} else {
			stmt.ImmSize = 1
		}
		stmt.Size = len(info.Imm) + stmt.ImmSize
		return nil
	}

	// 4. Indexed mode: contains comma or indirect brackets
	if strings.Contains(op, ",") || (strings.HasPrefix(op, "[") && strings.HasSuffix(op, "]")) {
		if len(info.Idx) == 0 {
			return fmt.Errorf("%s does not support indexed addressing", m)
		}
		stmt.Mode = ModeIndexed
		stmt.Opcode = info.Idx
		if err := a.setupIndexed(stmt, op); err != nil {
			return err
		}
		return nil
	}

	// 5. Direct or Extended mode:
	// Explicit direct: <expr
	if strings.HasPrefix(op, "<") {
		if len(info.Dir) == 0 {
			return fmt.Errorf("%s does not support direct page addressing", m)
		}
		stmt.Mode = ModeDirect
		stmt.Opcode = info.Dir
		stmt.Size = len(info.Dir) + 1
		stmt.TargetSym = strings.TrimSpace(op[1:])
		return nil
	}

	// Explicit extended: >expr
	if strings.HasPrefix(op, ">") {
		if len(info.Ext) == 0 {
			return fmt.Errorf("%s does not support extended addressing", m)
		}
		stmt.Mode = ModeExtended
		stmt.Opcode = info.Ext
		stmt.Size = len(info.Ext) + 2
		stmt.TargetSym = strings.TrimSpace(op[1:])
		return nil
	}

	// Memory reference without prefix: eligible for Direct Page relaxation!
	if len(info.Ext) > 0 {
		stmt.Mode = ModeExtended
		stmt.Opcode = info.Ext
		stmt.TargetSym = op
		stmt.ExtSize = len(info.Ext) + 2
		stmt.DirSize = len(info.Dir) + 1

		if a.enableRelax && len(info.Dir) > 0 {
			stmt.IsDirectRelaxable = true
			stmt.DirectSymbol = op
			stmt.DirOpcode = info.Dir
			stmt.ExtOpcode = info.Ext
			// Start conservatively with extended size
			stmt.Size = stmt.ExtSize
		} else {
			stmt.Size = stmt.ExtSize
		}
		return nil
	}

	if len(info.Dir) > 0 {
		stmt.Mode = ModeDirect
		stmt.Opcode = info.Dir
		stmt.Size = len(info.Dir) + 1
		stmt.TargetSym = op
		return nil
	}

	return fmt.Errorf("unsupported addressing mode for %s: %s", m, op)
}

func (a *Assembler) setupIndexed(stmt *Statement, op string) error {
	isIndirect := false
	if strings.HasPrefix(op, "[") && strings.HasSuffix(op, "]") {
		isIndirect = true
		op = strings.TrimSpace(op[1 : len(op)-1])
	}
	stmt.IsIndirect = isIndirect

	// Extended indirect: [addr] without comma
	if !strings.Contains(op, ",") {
		if !isIndirect {
			return fmt.Errorf("invalid indexed operand: %s", op)
		}
		// [nn]: postbyte 0x9F followed by 16-bit address
		stmt.Postbyte = 0x9F
		stmt.IndexDispSize = 2
		stmt.TargetSym = op
		stmt.Size = len(stmt.Opcode) + 1 + 2
		return nil
	}

	parts := strings.SplitN(op, ",", 2)
	offsetStr := strings.TrimSpace(parts[0])
	regStr := strings.ToLower(strings.TrimSpace(parts[1]))

	// Determine base register (X, Y, U, S, PCR)
	var regCode byte
	isPCR := false

	switch regStr {
	case "x":
		regCode = 0x00
	case "y":
		regCode = 0x20
	case "u":
		regCode = 0x40
	case "s":
		regCode = 0x60
	case "pcr", "pc":
		isPCR = true
	default:
		// Check for auto inc/dec
		if regStr == "x+" {
			stmt.Postbyte = 0x80
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "x++" {
			stmt.Postbyte = 0x81
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "-x" {
			stmt.Postbyte = 0x82
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "--x" {
			stmt.Postbyte = 0x83
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "y+" {
			stmt.Postbyte = 0xA0
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "y++" {
			stmt.Postbyte = 0xA1
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "-y" {
			stmt.Postbyte = 0xA2
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "--y" {
			stmt.Postbyte = 0xA3
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "u+" {
			stmt.Postbyte = 0xC0
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "u++" {
			stmt.Postbyte = 0xC1
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "-u" {
			stmt.Postbyte = 0xC2
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "--u" {
			stmt.Postbyte = 0xC3
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "s+" {
			stmt.Postbyte = 0xE0
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "s++" {
			stmt.Postbyte = 0xE1
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "-s" {
			stmt.Postbyte = 0xE2
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		if regStr == "--s" {
			stmt.Postbyte = 0xE3
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		return fmt.Errorf("unknown index register: %s", regStr)
	}

	// PCR indexed: offset,pcr
	if isPCR {
		stmt.IsPCRel = true
		stmt.TargetSym = offsetStr
		// Conservative default: 16-bit offset ($8D / $9D)
		if isIndirect {
			stmt.Postbyte = 0x9D
		} else {
			stmt.Postbyte = 0x8D
		}
		stmt.IndexDispSize = 2
		stmt.Size = len(stmt.Opcode) + 1 + 2
		return nil
	}

	// Accumulator offset: A, B, D
	switch strings.ToLower(offsetStr) {
	case "a":
		stmt.Postbyte = 0x86 | regCode
		if isIndirect {
			stmt.Postbyte |= 0x10
		}
		stmt.Size = len(stmt.Opcode) + 1
		return nil
	case "b":
		stmt.Postbyte = 0x85 | regCode
		if isIndirect {
			stmt.Postbyte |= 0x10
		}
		stmt.Size = len(stmt.Opcode) + 1
		return nil
	case "d":
		stmt.Postbyte = 0x8B | regCode
		if isIndirect {
			stmt.Postbyte |= 0x10
		}
		stmt.Size = len(stmt.Opcode) + 1
		return nil
	}

	// Zero offset: ,R
	if offsetStr == "" {
		if isIndirect {
			stmt.Postbyte = 0x94 | regCode
		} else {
			stmt.Postbyte = 0x84 | regCode
		}
		stmt.Size = len(stmt.Opcode) + 1
		return nil
	}

	// Constant or symbolic offset
	val, err := a.evalExpr(offsetStr)
	if err == nil {
		stmt.IndexDisp = val
		// If not indirect, can we use 5-bit offset? [-16..15]
		if !isIndirect && val >= -16 && val <= 15 {
			stmt.Postbyte = byte(val&0x1F) | regCode
			stmt.IndexDispSize = 0
			stmt.Size = len(stmt.Opcode) + 1
			return nil
		}
		// 8-bit offset? [-128..127]
		if val >= -128 && val <= 127 {
			stmt.Postbyte = 0x88 | regCode
			if isIndirect {
				stmt.Postbyte |= 0x10
			}
			stmt.IndexDispSize = 1
			stmt.Size = len(stmt.Opcode) + 1 + 1
			return nil
		}
		// 16-bit offset
		stmt.Postbyte = 0x89 | regCode
		if isIndirect {
			stmt.Postbyte |= 0x10
		}
		stmt.IndexDispSize = 2
		stmt.Size = len(stmt.Opcode) + 1 + 2
		return nil
	}

	// Symbolic offset
	stmt.TargetSym = offsetStr
	stmt.Postbyte = 0x89 | regCode
	if isIndirect {
		stmt.Postbyte |= 0x10
	}
	stmt.IndexDispSize = 2
	stmt.Size = len(stmt.Opcode) + 1 + 2
	return nil
}

func parseRegID(r string) (RegID, error) {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "d":
		return RegD, nil
	case "x":
		return RegX, nil
	case "y":
		return RegY, nil
	case "u":
		return RegU, nil
	case "s":
		return RegS, nil
	case "pc":
		return RegPC, nil
	case "a":
		return RegA, nil
	case "b":
		return RegB, nil
	case "cc":
		return RegCC, nil
	case "dp":
		return RegDP, nil
	default:
		return 0, fmt.Errorf("unknown register: %s", r)
	}
}

func parseStackRegMask(regsStr string, isUserStack bool) (byte, error) {
	parts := strings.Split(regsStr, ",")
	var mask byte = 0
	for _, p := range parts {
		r := strings.ToLower(strings.TrimSpace(p))
		switch r {
		case "cc":
			mask |= 0x01
		case "a":
			mask |= 0x02
		case "b":
			mask |= 0x04
		case "d":
			mask |= 0x06
		case "dp":
			mask |= 0x08
		case "x":
			mask |= 0x10
		case "y":
			mask |= 0x20
		case "u":
			if isUserStack {
				return 0, fmt.Errorf("cannot push/pull U to/from user stack")
			}
			mask |= 0x40
		case "s":
			if !isUserStack {
				return 0, fmt.Errorf("cannot push/pull S to/from system stack")
			}
			mask |= 0x40
		case "pc":
			mask |= 0x80
		default:
			return 0, fmt.Errorf("unknown register in push/pull: %s", r)
		}
	}
	return mask, nil
}

func (a *Assembler) Assemble() error {
	// First pass: resolve EQU directives and collect all labels with initial layout
	var currPC uint32 = a.currPC
	for _, stmt := range a.statements {
		if stmt.Type == StmtOrg {
			currPC = stmt.OrgAddr
			stmt.PC = currPC
		} else if stmt.Type == StmtEqu {
			val, err := a.evalExpr(stmt.EquExpr)
			if err != nil {
				return fmt.Errorf("%s:%d: failed to evaluate equ: %v", stmt.File, stmt.LineNum, err)
			}
			a.symbols[stmt.Label] = val
		} else {
			stmt.PC = currPC
			if stmt.Label != "" {
				a.symbols[stmt.Label] = int64(currPC)
				if stmt.Label == "cstart" || stmt.Label == "_main" {
					a.entryPoint = currPC
					a.hasEntry = true
				}
			}
			currPC += uint32(stmt.Size)
		}
	}

	// Whole-Program Relaxation Pass
	if a.enableRelax {
		if err := a.Relax(); err != nil {
			return err
		}
	}

	// Final code generation pass: encode all statements
	for _, stmt := range a.statements {
		if err := a.encodeStatement(stmt); err != nil {
			return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
		}
		if stmt.SrcLine != nil {
			stmt.SrcLine.PC = stmt.PC
			stmt.SrcLine.HasPC = (stmt.Type == StmtInstruction || stmt.Type == StmtDataByte || stmt.Type == StmtDataWord || stmt.Type == StmtString || stmt.Type == StmtReserve)
			stmt.SrcLine.Encoded = stmt.Encoded
		}
	}

	return nil
}

func (a *Assembler) Relax() error {
	for pass := 0; pass < 30; pass++ {
		changed := false

		// 1. Recompute PC for all statements and update symbol table
		var currPC uint32 = a.currPC
		for _, stmt := range a.statements {
			if stmt.Type == StmtOrg {
				currPC = stmt.OrgAddr
			}
			stmt.PC = currPC
			if stmt.Label != "" && stmt.Type != StmtEqu {
				a.symbols[stmt.Label] = int64(currPC)
				if stmt.Label == "cstart" || stmt.Label == "_main" {
					a.entryPoint = currPC
					a.hasEntry = true
				}
			}
			currPC += uint32(stmt.Size)
		}

		// 2. Evaluate relaxation candidates
		for _, stmt := range a.statements {
			// Branch relaxation: LBRA/LBSR/LBcc -> BRA/BSR/Bcc
			if stmt.IsBranch && stmt.Size > 2 {
				targetAddr, ok := a.symbols[stmt.BranchTarget]
				if ok {
					// Short branch is 2 bytes: opcode (1) + offset (1)
					// Target is relative to next instruction PC = stmt.PC + 2
					offset := targetAddr - int64(stmt.PC+2)
					if offset >= -128 && offset <= 127 {
						stmt.IsLongBranch = false
						stmt.Opcode = stmt.ShortBranchOpcode
						stmt.Size = 2
						changed = true
					}
				}
			}

			// Direct Page relaxation: Extended -> Direct
			if stmt.IsDirectRelaxable && stmt.Size == stmt.ExtSize {
				symVal, err := a.evalExpr(stmt.DirectSymbol)
				if err == nil {
					addr := symVal + stmt.DirectOffset
					// If in page 0 [DP*256 .. DP*256 + 255]
					dpStart := int64(a.dp) * 256
					if addr >= dpStart && addr <= dpStart+255 {
						stmt.Mode = ModeDirect
						stmt.Opcode = stmt.DirOpcode
						stmt.Size = stmt.DirSize
						changed = true
					}
				}
			}

			// PCR relaxation: 16-bit PCR -> 8-bit PCR
			if stmt.IsPCRel && stmt.IndexDispSize == 2 {
				targetAddr, ok := a.symbols[stmt.TargetSym]
				if ok {
					// 8-bit PCR is: opcode len + 1 (postbyte 8C/9C) + 1 byte offset = len + 2
					// Next PC is stmt.PC + uint32(len(stmt.Opcode) + 2)
					nextPC := int64(stmt.PC) + int64(len(stmt.Opcode)+2)
					offset := targetAddr - nextPC
					if offset >= -128 && offset <= 127 {
						if stmt.IsIndirect {
							stmt.Postbyte = 0x9C
						} else {
							stmt.Postbyte = 0x8C
						}
						stmt.IndexDispSize = 1
						stmt.Size = len(stmt.Opcode) + 1 + 1
						changed = true
					}
				}
			}
		}

		if !changed {
			break
		}
	}

	return nil
}

func (a *Assembler) encodeStatement(stmt *Statement) error {
	switch stmt.Type {
	case StmtOrg, StmtEqu, StmtComment:
		return nil

	case StmtEnd:
		if stmt.BranchTarget != "" {
			if val, ok := a.symbols[stmt.BranchTarget]; ok {
				a.entryPoint = uint32(val)
				a.hasEntry = true
			}
		}
		return nil

	case StmtDataByte:
		for _, expr := range stmt.DataExprs {
			v, err := a.evalExpr(expr)
			if err != nil {
				return err
			}
			stmt.Encoded = append(stmt.Encoded, byte(v&0xFF))
		}
		return nil

	case StmtDataWord:
		for _, expr := range stmt.DataExprs {
			v, err := a.evalExpr(expr)
			if err != nil {
				return err
			}
			stmt.Encoded = append(stmt.Encoded, byte((v>>8)&0xFF), byte(v&0xFF))
		}
		return nil

	case StmtDataQuad:
		for _, expr := range stmt.DataExprs {
			v, err := a.evalExpr(expr)
			if err != nil {
				return err
			}
			stmt.Encoded = append(stmt.Encoded,
				byte((v>>24)&0xFF),
				byte((v>>16)&0xFF),
				byte((v>>8)&0xFF),
				byte(v&0xFF))
		}
		return nil

	case StmtString:
		stmt.Encoded = []byte(stmt.StrData)
		return nil

	case StmtReserve:
		stmt.Encoded = make([]byte, stmt.ReserveBytes)
		return nil

	case StmtInstruction:
		// Encode instruction
		m := stmt.Mnemonic

		// Special instructions: tfr, exg
		if m == "tfr" || m == "exg" {
			parts := splitOperands(stmt.RawOp)
			if len(parts) != 2 {
				return fmt.Errorf("invalid operand for %s: %s", m, stmt.RawOp)
			}
			r1, err1 := parseRegID(parts[0])
			r2, err2 := parseRegID(parts[1])
			if err1 != nil || err2 != nil {
				return fmt.Errorf("invalid registers for %s: %s", m, stmt.RawOp)
			}
			opByte := byte(0x1F)
			if m == "exg" {
				opByte = 0x1E
			}
			post := (byte(r1) << 4) | byte(r2)
			stmt.Encoded = []byte{opByte, post}
			return nil
		}

		// Stack instructions: pshs, puls, pshu, pulu
		if m == "pshs" || m == "puls" || m == "pshu" || m == "pulu" {
			isUser := (m == "pshu" || m == "pulu")
			mask, err := parseStackRegMask(stmt.RawOp, isUser)
			if err != nil {
				return err
			}
			var opByte byte
			switch m {
			case "pshs":
				opByte = 0x34
			case "puls":
				opByte = 0x35
			case "pshu":
				opByte = 0x36
			case "pulu":
				opByte = 0x37
			}
			stmt.Encoded = []byte{opByte, mask}
			return nil
		}

		if m == "andcc" || m == "orcc" {
			val, err := a.evalExpr(strings.TrimPrefix(stmt.RawOp, "#"))
			if err != nil {
				return err
			}
			opByte := byte(0x1C)
			if m == "orcc" {
				opByte = 0x1A
			}
			stmt.Encoded = []byte{opByte, byte(val & 0xFF)}
			return nil
		}

		// Branches
		if stmt.IsBranch {
			targetAddr, err := a.evalExpr(stmt.BranchTarget)
			if err != nil {
				return fmt.Errorf("unresolved branch target: %s", stmt.BranchTarget)
			}

			if stmt.Size == 2 {
				// Short branch: opcode (1) + 1-byte offset
				offset := targetAddr - int64(stmt.PC+2)
				if offset < -128 || offset > 127 {
					return fmt.Errorf("branch out of range (%d): %s", offset, stmt.BranchTarget)
				}
				stmt.Encoded = append(stmt.Encoded, stmt.ShortBranchOpcode...)
				stmt.Encoded = append(stmt.Encoded, byte(offset&0xFF))
			} else {
				// Long branch
				nextPC := int64(stmt.PC) + int64(len(stmt.LongBranchOpcode)+2)
				offset := targetAddr - nextPC
				stmt.Encoded = append(stmt.Encoded, stmt.LongBranchOpcode...)
				stmt.Encoded = append(stmt.Encoded, byte((offset>>8)&0xFF), byte(offset&0xFF))
			}
			return nil
		}

		// Standard instructions by mode
		stmt.Encoded = append(stmt.Encoded, stmt.Opcode...)

		switch stmt.Mode {
		case ModeInherent:
			return nil

		case ModeImmediate:
			val := stmt.ImmVal
			if stmt.TargetSym != "" {
				v, err := a.evalExpr(stmt.TargetSym)
				if err != nil {
					return err
				}
				val = v
			}
			if stmt.ImmSize == 1 {
				stmt.Encoded = append(stmt.Encoded, byte(val&0xFF))
			} else {
				stmt.Encoded = append(stmt.Encoded, byte((val>>8)&0xFF), byte(val&0xFF))
			}
			return nil

		case ModeDirect:
			target := stmt.TargetSym
			val, err := a.evalExpr(target)
			if err != nil {
				return err
			}
			addr := byte(val & 0xFF)
			stmt.Encoded = append(stmt.Encoded, addr)
			return nil

		case ModeExtended:
			target := stmt.TargetSym
			val, err := a.evalExpr(target)
			if err != nil {
				return err
			}
			stmt.Encoded = append(stmt.Encoded, byte((val>>8)&0xFF), byte(val&0xFF))
			return nil

		case ModeIndexed:
			stmt.Encoded = append(stmt.Encoded, stmt.Postbyte)

			if stmt.IsPCRel {
				targetAddr, err := a.evalExpr(stmt.TargetSym)
				if err != nil {
					return err
				}
				nextPC := int64(stmt.PC) + int64(len(stmt.Encoded)) + int64(stmt.IndexDispSize)
				offset := targetAddr - nextPC
				if stmt.IndexDispSize == 1 {
					stmt.Encoded = append(stmt.Encoded, byte(offset&0xFF))
				} else {
					stmt.Encoded = append(stmt.Encoded, byte((offset>>8)&0xFF), byte(offset&0xFF))
				}
				return nil
			}

			if stmt.IndexDispSize == 1 {
				val := stmt.IndexDisp
				if stmt.TargetSym != "" {
					v, err := a.evalExpr(stmt.TargetSym)
					if err != nil {
						return err
					}
					val = v
				}
				stmt.Encoded = append(stmt.Encoded, byte(val&0xFF))
			} else if stmt.IndexDispSize == 2 {
				val := stmt.IndexDisp
				if stmt.TargetSym != "" {
					v, err := a.evalExpr(stmt.TargetSym)
					if err != nil {
						return err
					}
					val = v
				}
				stmt.Encoded = append(stmt.Encoded, byte((val>>8)&0xFF), byte(val&0xFF))
			}
			return nil
		}
	}

	return nil
}

type Chunk struct {
	Addr uint16
	Data []byte
}

func (a *Assembler) CollectChunks() []Chunk {
	var chunks []Chunk
	var currChunk *Chunk

	for _, stmt := range a.statements {
		if len(stmt.Encoded) == 0 {
			continue
		}
		addr := uint16(stmt.PC)

		if currChunk == nil || addr != currChunk.Addr+uint16(len(currChunk.Data)) {
			chunks = append(chunks, Chunk{
				Addr: addr,
				Data: append([]byte(nil), stmt.Encoded...),
			})
			currChunk = &chunks[len(chunks)-1]
		} else {
			currChunk.Data = append(currChunk.Data, stmt.Encoded...)
		}
	}

	return chunks
}

func (a *Assembler) EmitDECB(w io.Writer) error {
	chunks := a.CollectChunks()

	for _, c := range chunks {
		if len(c.Data) == 0 {
			continue
		}
		length := uint16(len(c.Data))
		header := []byte{
			0x00,
			byte((length >> 8) & 0xFF),
			byte(length & 0xFF),
			byte((c.Addr >> 8) & 0xFF),
			byte(c.Addr & 0xFF),
		}
		if _, err := w.Write(header); err != nil {
			return err
		}
		if _, err := w.Write(c.Data); err != nil {
			return err
		}
	}

	// Postamble: 0xFF 0x00 0x00 entryPoint
	entry := uint16(a.entryPoint)
	postamble := []byte{
		0xFF,
		0x00,
		0x00,
		byte((entry >> 8) & 0xFF),
		byte(entry & 0xFF),
	}
	_, err := w.Write(postamble)
	return err
}

func (a *Assembler) EmitRaw(w io.Writer) error {
	chunks := a.CollectChunks()
	for _, c := range chunks {
		if _, err := w.Write(c.Data); err != nil {
			return err
		}
	}
	return nil
}

func (a *Assembler) EmitSRecords(w io.Writer) error {
	chunks := a.CollectChunks()

	// S0 Header
	s0Header := "S00600004844521B\n"
	if _, err := io.WriteString(w, s0Header); err != nil {
		return err
	}

	for _, c := range chunks {
		data := c.Data
		addr := c.Addr
		for len(data) > 0 {
			chunkLen := len(data)
			if chunkLen > 32 {
				chunkLen = 32
			}
			count := byte(chunkLen + 3) // 2 address bytes + 1 checksum byte
			chk := count + byte((addr>>8)&0xFF) + byte(addr&0xFF)

			var hexBuf bytes.Buffer
			hexBuf.WriteString(fmt.Sprintf("S1%02X%04X", count, addr))
			for i := 0; i < chunkLen; i++ {
				b := data[i]
				chk += b
				hexBuf.WriteString(fmt.Sprintf("%02X", b))
			}
			hexBuf.WriteString(fmt.Sprintf("%02X\n", ^chk))
			if _, err := io.WriteString(w, hexBuf.String()); err != nil {
				return err
			}

			data = data[chunkLen:]
			addr += uint16(chunkLen)
		}
	}

	// S9 End of file
	entry := uint16(a.entryPoint)
	chk := byte(3) + byte((entry>>8)&0xFF) + byte(entry&0xFF)
	s9 := fmt.Sprintf("S903%04X%02X\n", entry, ^chk)
	_, err := io.WriteString(w, s9)
	return err
}

func (a *Assembler) EmitListing(w io.Writer) error {
	for _, line := range a.sourceLines {
		raw := line.Raw
		if !line.HasPC || len(line.Encoded) == 0 {
			if _, err := fmt.Fprintf(w, "                        %5d  %s\n", line.LineNum, raw); err != nil {
				return err
			}
			continue
		}

		enc := line.Encoded
		firstChunk := enc
		if len(firstChunk) > 4 {
			firstChunk = enc[:4]
		}
		hexStr := strings.ToUpper(hex.EncodeToString(firstChunk))
		if _, err := fmt.Fprintf(w, "%04X  %-10s        %5d  %s\n", uint16(line.PC), hexStr, line.LineNum, raw); err != nil {
			return err
		}

		// Continuation lines if more than 4 bytes
		rem := enc[len(firstChunk):]
		for len(rem) > 0 {
			chunk := rem
			if len(chunk) > 4 {
				chunk = rem[:4]
			}
			remHex := strings.ToUpper(hex.EncodeToString(chunk))
			if _, err := fmt.Fprintf(w, "      %-10s\n", remHex); err != nil {
				return err
			}
			rem = rem[len(chunk):]
		}
	}
	return nil
}

func main() {
	outPath := flag.String("o", "", "Output binary file")
	format := flag.String("f", "decb", "Output format: decb, raw, srec")
	listPath := flag.String("l", "", "Generate listing file")
	flag.StringVar(listPath, "list", "", "Generate listing file")
	decbFlag := flag.Bool("b", false, "Generate DECB format (equivalent to -f decb)")
	flag.BoolVar(decbFlag, "decb", false, "Generate DECB format (equivalent to -f decb)")
	rawFlag := flag.Bool("r", false, "Generate raw format (equivalent to -f raw)")
	flag.BoolVar(rawFlag, "raw", false, "Generate raw format (equivalent to -f raw)")
	relaxFlag := flag.Bool("relax", true, "Enable whole-program branch & direct page relaxation")
	noRelaxFlag := flag.Bool("no-relax", false, "Disable relaxation")
	dpAddr := flag.Int("dp", 0, "Direct Page base address (default: 0)")

	// Ignored flags for compatibility with lwasm
	_ = flag.Bool("9", false, "6809 mode")
	_ = flag.String("p", "", "Pragmas (ignored)")
	_ = flag.String("pragma", "", "Pragmas (ignored)")
	_ = flag.String("map", "", "Map file (ignored)")

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <input.asm>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(1)
	}
	inputFile := args[0]

	if *decbFlag {
		*format = "decb"
	} else if *rawFlag {
		*format = "raw"
	}

	if *noRelaxFlag {
		*relaxFlag = false
	}

	asm := NewAssembler()
	asm.enableRelax = *relaxFlag
	asm.dp = uint8(*dpAddr & 0xFF)

	f, err := os.Open(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", inputFile, err)
		os.Exit(1)
	}
	defer f.Close()

	if err := asm.LoadSource(inputFile, f); err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	if err := asm.Assemble(); err != nil {
		fmt.Fprintf(os.Stderr, "Assembly error: %v\n", err)
		os.Exit(1)
	}

	// Emit output
	if *outPath != "" {
		outFile, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *outPath, err)
			os.Exit(1)
		}
		defer outFile.Close()

		switch strings.ToLower(*format) {
		case "decb":
			err = asm.EmitDECB(outFile)
		case "raw":
			err = asm.EmitRaw(outFile)
		case "srec":
			err = asm.EmitSRecords(outFile)
		default:
			fmt.Fprintf(os.Stderr, "Unknown format: %s\n", *format)
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
	}

	// Emit listing
	if *listPath != "" {
		lFile, err := os.Create(*listPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", *listPath, err)
			os.Exit(1)
		}
		defer lFile.Close()
		if err := asm.EmitListing(lFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing listing: %v\n", err)
			os.Exit(1)
		}
	}
}
