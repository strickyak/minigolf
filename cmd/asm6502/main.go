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

type AddrMode int

const (
	ModeImp AddrMode = iota
	ModeAcc
	ModeImm
	ModeZP
	ModeZPX
	ModeZPY
	ModeAbs
	ModeAbsX
	ModeAbsY
	ModeInd
	ModeIndX
	ModeIndY
	ModeRel
)

type OpEncodings struct {
	Imp  int
	Acc  int
	Imm  int
	ZP   int
	ZPX  int
	ZPY  int
	Abs  int
	AbsX int
	AbsY int
	Ind  int
	IndX int
	IndY int
	Rel  int
}

func newOpEnc() *OpEncodings {
	return &OpEncodings{
		Imp: -1, Acc: -1, Imm: -1,
		ZP: -1, ZPX: -1, ZPY: -1,
		Abs: -1, AbsX: -1, AbsY: -1,
		Ind: -1, IndX: -1, IndY: -1,
		Rel: -1,
	}
}

var opcodeTable = map[string]*OpEncodings{
	"ADC":  {Imm: 0x69, ZP: 0x65, ZPX: 0x75, Abs: 0x6D, AbsX: 0x7D, AbsY: 0x79, IndX: 0x61, IndY: 0x71, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"AND":  {Imm: 0x29, ZP: 0x25, ZPX: 0x35, Abs: 0x2D, AbsX: 0x3D, AbsY: 0x39, IndX: 0x21, IndY: 0x31, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"ASL":  {Acc: 0x0A, Imp: 0x0A, ZP: 0x06, ZPX: 0x16, Abs: 0x0E, AbsX: 0x1E, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"BCC":  {Rel: 0x90, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BLT":  {Rel: 0x90, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BCS":  {Rel: 0xB0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BGE":  {Rel: 0xB0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BEQ":  {Rel: 0xF0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BIT":  {ZP: 0x24, Abs: 0x2C, Imp: -1, Acc: -1, Imm: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"BMI":  {Rel: 0x30, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BNE":  {Rel: 0xD0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BPL":  {Rel: 0x10, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BRK":  {Imp: 0x00, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"BVC":  {Rel: 0x50, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"BVS":  {Rel: 0x70, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"CLC":  {Imp: 0x18, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"CLD":  {Imp: 0xD8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"CLI":  {Imp: 0x58, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"CLV":  {Imp: 0xB8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"CMP":  {Imm: 0xC9, ZP: 0xC5, ZPX: 0xD5, Abs: 0xCD, AbsX: 0xDD, AbsY: 0xD9, IndX: 0xC1, IndY: 0xD1, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"CPX":  {Imm: 0xE0, ZP: 0xE4, Abs: 0xEC, Imp: -1, Acc: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"CPY":  {Imm: 0xC0, ZP: 0xC4, Abs: 0xCC, Imp: -1, Acc: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"DEC":  {ZP: 0xC6, ZPX: 0xD6, Abs: 0xCE, AbsX: 0xDE, Imp: -1, Acc: -1, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"DEX":  {Imp: 0xCA, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"DEY":  {Imp: 0x88, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"EOR":  {Imm: 0x49, ZP: 0x45, ZPX: 0x55, Abs: 0x4D, AbsX: 0x5D, AbsY: 0x59, IndX: 0x41, IndY: 0x51, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"INC":  {ZP: 0xE6, ZPX: 0xF6, Abs: 0xEE, AbsX: 0xFE, Imp: -1, Acc: -1, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"INX":  {Imp: 0xE8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"INY":  {Imp: 0xC8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"JMP":  {Abs: 0x4C, Ind: 0x6C, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, IndX: -1, IndY: -1, Rel: -1},
	"BRA":  {Abs: 0x4C, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"LBRA": {Abs: 0x4C, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"JSR":  {Abs: 0x20, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"LDA":  {Imm: 0xA9, ZP: 0xA5, ZPX: 0xB5, Abs: 0xAD, AbsX: 0xBD, AbsY: 0xB9, IndX: 0xA1, IndY: 0xB1, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"LDX":  {Imm: 0xA2, ZP: 0xA6, ZPY: 0xB6, Abs: 0xAE, AbsY: 0xBE, Imp: -1, Acc: -1, ZPX: -1, AbsX: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"LDY":  {Imm: 0xA0, ZP: 0xA4, ZPX: 0xB4, Abs: 0xAC, AbsX: 0xBC, Imp: -1, Acc: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"LSR":  {Acc: 0x4A, Imp: 0x4A, ZP: 0x46, ZPX: 0x56, Abs: 0x4E, AbsX: 0x5E, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"NOP":  {Imp: 0xEA, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"ORA":  {Imm: 0x09, ZP: 0x05, ZPX: 0x15, Abs: 0x0D, AbsX: 0x1D, AbsY: 0x19, IndX: 0x01, IndY: 0x11, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"PHA":  {Imp: 0x48, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"PHP":  {Imp: 0x08, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"PLA":  {Imp: 0x68, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"PLP":  {Imp: 0x28, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"ROL":  {Acc: 0x2A, Imp: 0x2A, ZP: 0x26, ZPX: 0x36, Abs: 0x2E, AbsX: 0x3E, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"ROR":  {Acc: 0x6A, Imp: 0x6A, ZP: 0x66, ZPX: 0x76, Abs: 0x6E, AbsX: 0x7E, Imm: -1, ZPY: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"RTI":  {Imp: 0x40, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"RTS":  {Imp: 0x60, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"SBC":  {Imm: 0xE9, ZP: 0xE5, ZPX: 0xF5, Abs: 0xED, AbsX: 0xFD, AbsY: 0xF9, IndX: 0xE1, IndY: 0xF1, Imp: -1, Acc: -1, ZPY: -1, Ind: -1, Rel: -1},
	"SEC":  {Imp: 0x38, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"SED":  {Imp: 0xF8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"SEI":  {Imp: 0x78, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"STA":  {ZP: 0x85, ZPX: 0x95, Abs: 0x8D, AbsX: 0x9D, AbsY: 0x99, IndX: 0x81, IndY: 0x91, Imp: -1, Acc: -1, Imm: -1, ZPY: -1, Ind: -1, Rel: -1},
	"STX":  {ZP: 0x86, ZPY: 0x96, Abs: 0x8E, Imp: -1, Acc: -1, Imm: -1, ZPX: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"STY":  {ZP: 0x84, ZPX: 0x94, Abs: 0x8C, Imp: -1, Acc: -1, Imm: -1, ZPY: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TAX":  {Imp: 0xAA, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TAY":  {Imp: 0xA8, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TSX":  {Imp: 0xBA, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TXA":  {Imp: 0x8A, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TXS":  {Imp: 0x9A, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},
	"TYA":  {Imp: 0x98, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1, Rel: -1},

	// Long-branch aliases (relaxable conditional branches)
	"LBPL": {Rel: 0x10, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBMI": {Rel: 0x30, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBVC": {Rel: 0x50, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBVS": {Rel: 0x70, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBCC": {Rel: 0x90, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBLT": {Rel: 0x90, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBCS": {Rel: 0xB0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBGE": {Rel: 0xB0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBNE": {Rel: 0xD0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
	"LBEQ": {Rel: 0xF0, Imp: -1, Acc: -1, Imm: -1, ZP: -1, ZPX: -1, ZPY: -1, Abs: -1, AbsX: -1, AbsY: -1, Ind: -1, IndX: -1, IndY: -1},
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
	StmtAlign
	StmtEnd
	StmtComment
)

type Statement struct {
	Label      string
	Mnemonic   string
	RawOps     string
	LineNum    int
	File       string
	Type       StmtType
	PC         uint32
	Size       int
	Encoded    []byte
	SrcLine    *SourceLine
	ForceZP    bool
	ForceAbs   bool
	ParsedMode AddrMode
	Expr       string
	BranchLong bool // true if conditional branch relaxed to 5-byte B!cc *+5; JMP target
	UseZP      bool // true if operand fits in ZP and instruction supports ZP/ZPX/ZPY
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

func stripComment(line string) string {
	inQuote := false
	quoteChar := byte(0)
	escaped := false

	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) > 0 && trimmed[0] == '*' {
		// Allow "* = $0200" PC assignment, otherwise treat leading '*' as comment
		rest := strings.TrimLeft(trimmed[1:], " \t")
		if !strings.HasPrefix(rest, "=") {
			return ""
		}
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
			}
		}
	}
	return line
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	inQuote := false
	quoteChar := byte(0)
	escaped := false
	parenDepth := 0
	start := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
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
			} else if ch == '(' {
				parenDepth++
			} else if ch == ')' {
				if parenDepth > 0 {
					parenDepth--
				}
			} else if ch == ',' && parenDepth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

// isEnclosedInParens returns true if s starts with '(' and ends with the matching ')'.
func isEnclosedInParens(s string) bool {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return false
	}
	depth := 0
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
		} else if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 && i < len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// parse6502Operand determines the syntactic addressing mode category and extracts the expression.
func parse6502Operand(mnem string, rawOps string, enc *OpEncodings) (AddrMode, string, bool, bool, error) {
	ops := strings.TrimSpace(rawOps)
	if ops == "" {
		if enc.Imp != -1 {
			return ModeImp, "", false, false, nil
		}
		if enc.Acc != -1 {
			return ModeAcc, "", false, false, nil
		}
		return ModeImp, "", false, false, fmt.Errorf("%s requires an operand", mnem)
	}

	if strings.EqualFold(ops, "A") && enc.Acc != -1 {
		return ModeAcc, "", false, false, nil
	}

	// Relative branch instructions
	if enc.Rel != -1 {
		return ModeRel, ops, false, false, nil
	}

	// Immediate: #expr
	if strings.HasPrefix(ops, "#") {
		if enc.Imm == -1 {
			return ModeImm, "", false, false, fmt.Errorf("%s does not support immediate addressing", mnem)
		}
		return ModeImm, strings.TrimSpace(ops[1:]), false, false, nil
	}

	// Check for top-level comma (outside parentheses)
	topParts := splitTopLevelCommas(ops)
	if len(topParts) == 2 {
		lhs := topParts[0]
		rhs := strings.ToUpper(topParts[1])

		// Indirect Indexed: (expr),Y
		if rhs == "Y" && isEnclosedInParens(lhs) && enc.IndY != -1 {
			inner := strings.TrimSpace(lhs[1 : len(lhs)-1])
			return ModeIndY, inner, true, false, nil
		}

		forceZP := false
		forceAbs := false
		if strings.HasPrefix(lhs, "<") {
			forceZP = true
			lhs = strings.TrimSpace(lhs[1:])
		} else if strings.HasPrefix(lhs, ">") || strings.HasPrefix(lhs, "!") {
			forceAbs = true
			lhs = strings.TrimSpace(lhs[1:])
		}

		if rhs == "X" {
			return ModeAbsX, lhs, forceZP, forceAbs, nil
		}
		if rhs == "Y" {
			return ModeAbsY, lhs, forceZP, forceAbs, nil
		}
		return ModeAbs, "", false, false, fmt.Errorf("invalid index register %q in %s %s", topParts[1], mnem, rawOps)
	}

	if len(topParts) == 1 {
		// Check for (expr,X) or JMP (expr)
		if isEnclosedInParens(ops) {
			inner := strings.TrimSpace(ops[1 : len(ops)-1])
			innerParts := splitTopLevelCommas(inner)
			if len(innerParts) == 2 && strings.EqualFold(innerParts[1], "X") {
				if enc.IndX == -1 {
					return ModeIndX, "", false, false, fmt.Errorf("%s does not support (zp,X) addressing", mnem)
				}
				return ModeIndX, innerParts[0], true, false, nil
			}
			if len(innerParts) == 1 && enc.Ind != -1 {
				return ModeInd, inner, false, true, nil
			}
		}

		forceZP := false
		forceAbs := false
		expr := ops
		if strings.HasPrefix(expr, "<") {
			forceZP = true
			expr = strings.TrimSpace(expr[1:])
		} else if strings.HasPrefix(expr, "!") {
			forceAbs = true
			expr = strings.TrimSpace(expr[1:])
		}
		return ModeAbs, expr, forceZP, forceAbs, nil
	}

	return ModeAbs, "", false, false, fmt.Errorf("invalid operand syntax %q for %s", rawOps, mnem)
}

func (a *Assembler) LoadSource(filename string, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		rawLine := scanner.Text()
		srcLine := &SourceLine{
			File:    filename,
			LineNum: lineNum,
			Raw:     rawLine,
		}
		a.sourceLines = append(a.sourceLines, srcLine)

		clean := stripComment(rawLine)
		if strings.TrimSpace(clean) == "" {
			continue
		}

		stmt, err := a.parseLine(clean, filename, lineNum, srcLine)
		if err != nil {
			return fmt.Errorf("%s:%d: %v", filename, lineNum, err)
		}
		if stmt != nil {
			a.statements = append(a.statements, stmt)
		}
	}
	return scanner.Err()
}

func isDirectiveOrMnemonic(word string) bool {
	u := strings.ToUpper(strings.TrimPrefix(word, "."))
	if _, ok := opcodeTable[u]; ok {
		return true
	}
	switch u {
	case "ORG", "EQU", "DB", "FCB", "BYTE", "DEFB", "ASC", "ASCII", "FCC", "ASCIZ", "STRINGZ", "FCN",
		"DW", "FDB", "FCW", "WORD", "DEFW",
		"DS", "RMB", "ZMB", "BSZ", "DEFS", "RES",
		"ALIGN", "END", "GLOBAL", "GLOBL", "EXTERN", "EXTRN":
		return true
	}
	return false
}

func (a *Assembler) parseLine(line, filename string, lineNum int, srcLine *SourceLine) (*Statement, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil, nil
	}

	// Check for "* = expr" ORG assignment
	if strings.HasPrefix(trimmed, "*") {
		rest := strings.TrimSpace(trimmed[1:])
		if strings.HasPrefix(rest, "=") {
			return &Statement{
				Mnemonic: "ORG",
				RawOps:   strings.TrimSpace(rest[1:]),
				LineNum:  lineNum,
				File:     filename,
				Type:     StmtOrg,
				SrcLine:  srcLine,
			}, nil
		}
	}

	var label, mnem, rawOps string

	// Check for "LABEL = EXPR" direct assignment
	if eqIdx := strings.Index(trimmed, "="); eqIdx > 0 {
		lhs := strings.TrimSpace(trimmed[:eqIdx])
		rhs := strings.TrimSpace(trimmed[eqIdx+1:])
		if !strings.ContainsAny(lhs, " \t,;()+-*/") && lhs != "" {
			return &Statement{
				Label:    strings.TrimSuffix(lhs, ":"),
				Mnemonic: "EQU",
				RawOps:   rhs,
				LineNum:  lineNum,
				File:     filename,
				Type:     StmtEqu,
				SrcLine:  srcLine,
			}, nil
		}
	}

	hasLeadingSpace := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil, nil
	}

	first := fields[0]
	if strings.HasSuffix(first, ":") {
		label = strings.TrimSuffix(first, ":")
		rest := strings.TrimSpace(trimmed[len(first):])
		if rest == "" {
			return &Statement{
				Label:   label,
				LineNum: lineNum,
				File:    filename,
				Type:    StmtComment,
				SrcLine: srcLine,
			}, nil
		}
		rFields := strings.Fields(rest)
		mnem = rFields[0]
		rawOps = strings.TrimSpace(rest[len(mnem):])
	} else if !hasLeadingSpace && !isDirectiveOrMnemonic(first) {
		label = first
		rest := strings.TrimSpace(trimmed[len(first):])
		if rest == "" {
			return &Statement{
				Label:   label,
				LineNum: lineNum,
				File:    filename,
				Type:    StmtComment,
				SrcLine: srcLine,
			}, nil
		}
		rFields := strings.Fields(rest)
		mnem = rFields[0]
		rawOps = strings.TrimSpace(rest[len(mnem):])
	} else {
		mnem = first
		rawOps = strings.TrimSpace(trimmed[len(mnem):])
	}

	upperMnem := strings.ToUpper(strings.TrimPrefix(mnem, "."))
	stmt := &Statement{
		Label:    label,
		Mnemonic: upperMnem,
		RawOps:   rawOps,
		LineNum:  lineNum,
		File:     filename,
		SrcLine:  srcLine,
	}

	switch upperMnem {
	case "ORG":
		stmt.Type = StmtOrg
	case "EQU":
		stmt.Type = StmtEqu
	case "DB", "FCB", "BYTE", "DEFB", "ASC", "ASCII", "FCC", "ASCIZ", "STRINGZ", "FCN":
		stmt.Type = StmtDataByte
	case "DW", "FDB", "FCW", "WORD", "DEFW":
		stmt.Type = StmtDataWord
	case "DS", "RMB", "ZMB", "BSZ", "DEFS", "RES":
		stmt.Type = StmtReserve
	case "ALIGN":
		stmt.Type = StmtAlign
	case "END":
		stmt.Type = StmtEnd
	case "GLOBAL", "GLOBL", "EXTERN", "EXTRN":
		stmt.Type = StmtComment
	default:
		enc, ok := opcodeTable[upperMnem]
		if !ok {
			return nil, fmt.Errorf("unknown mnemonic or directive %q", mnem)
		}
		stmt.Type = StmtInstruction
		mode, expr, forceZP, forceAbs, err := parse6502Operand(upperMnem, rawOps, enc)
		if err != nil {
			return nil, err
		}
		stmt.ParsedMode = mode
		stmt.Expr = expr
		stmt.ForceZP = forceZP
		stmt.ForceAbs = forceAbs
	}

	return stmt, nil
}

// Expression Evaluator supporting +, -, *, /, %, &, |, ^, <<, >>, unary +, -, ~, < (low byte), > (high byte)
type exprParser struct {
	s    string
	pos  int
	asm  *Assembler
	pc   uint32
	pass int
}

func (a *Assembler) EvalExpr(expr string, pc uint32) (int64, bool, error) {
	p := &exprParser{
		s:    strings.TrimSpace(expr),
		pos:  0,
		asm:  a,
		pc:   pc,
		pass: a.pass,
	}
	if p.s == "" {
		return 0, false, fmt.Errorf("empty expression")
	}
	val, known, err := p.parseBitwiseOr()
	if err != nil {
		return 0, false, err
	}
	p.skipSpaces()
	if p.pos < len(p.s) {
		return 0, false, fmt.Errorf("unexpected trailing characters %q in expression %q", p.s[p.pos:], expr)
	}
	return val, known, nil
}

func (p *exprParser) skipSpaces() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) parseBitwiseOr() (int64, bool, error) {
	left, known, err := p.parseBitwiseXor()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.s) && p.s[p.pos] == '|' {
			p.pos++
			right, rKnown, err := p.parseBitwiseXor()
			if err != nil {
				return 0, false, err
			}
			left |= right
			known = known && rKnown
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseBitwiseXor() (int64, bool, error) {
	left, known, err := p.parseBitwiseAnd()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.s) && p.s[p.pos] == '^' {
			p.pos++
			right, rKnown, err := p.parseBitwiseAnd()
			if err != nil {
				return 0, false, err
			}
			left ^= right
			known = known && rKnown
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseBitwiseAnd() (int64, bool, error) {
	left, known, err := p.parseShift()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.s) && p.s[p.pos] == '&' {
			p.pos++
			right, rKnown, err := p.parseShift()
			if err != nil {
				return 0, false, err
			}
			left &= right
			known = known && rKnown
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseShift() (int64, bool, error) {
	left, known, err := p.parseAddSub()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos+1 < len(p.s) && (p.s[p.pos:p.pos+2] == "<<" || p.s[p.pos:p.pos+2] == ">>") {
			op := p.s[p.pos : p.pos+2]
			p.pos += 2
			right, rKnown, err := p.parseAddSub()
			if err != nil {
				return 0, false, err
			}
			if op == "<<" {
				left <<= uint(right)
			} else {
				left >>= uint(right)
			}
			known = known && rKnown
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseAddSub() (int64, bool, error) {
	left, known, err := p.parseMulDiv()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
			op := p.s[p.pos]
			p.pos++
			right, rKnown, err := p.parseMulDiv()
			if err != nil {
				return 0, false, err
			}
			if op == '+' {
				left += right
			} else {
				left -= right
			}
			known = known && rKnown
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseMulDiv() (int64, bool, error) {
	left, known, err := p.parseUnary()
	if err != nil {
		return 0, false, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.s) && (p.s[p.pos] == '*' || p.s[p.pos] == '/' || p.s[p.pos] == '%') {
			op := p.s[p.pos]
			p.pos++
			right, rKnown, err := p.parseUnary()
			if err != nil {
				return 0, false, err
			}
			known = known && rKnown
			if op == '*' {
				left *= right
			} else if op == '/' {
				if right != 0 {
					left /= right
				}
			} else {
				if right != 0 {
					left %= right
				}
			}
		} else {
			break
		}
	}
	return left, known, nil
}

func (p *exprParser) parseUnary() (int64, bool, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return 0, false, fmt.Errorf("unexpected end of expression")
	}
	ch := p.s[p.pos]
	if ch == '+' {
		p.pos++
		return p.parseUnary()
	}
	if ch == '-' {
		p.pos++
		v, k, err := p.parseUnary()
		return -v, k, err
	}
	if ch == '~' {
		p.pos++
		v, k, err := p.parseUnary()
		return ^v, k, err
	}
	if ch == '<' {
		p.pos++
		v, k, err := p.parseUnary()
		return v & 0xFF, k, err
	}
	if ch == '>' {
		p.pos++
		v, k, err := p.parseUnary()
		return (v >> 8) & 0xFF, k, err
	}
	return p.parsePrimary()
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_' || ch == '.' || ch == '$'
}

func (p *exprParser) parsePrimary() (int64, bool, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return 0, false, fmt.Errorf("unexpected end of expression")
	}

	ch := p.s[p.pos]

	// Parentheses
	if ch == '(' {
		p.pos++
		val, known, err := p.parseBitwiseOr()
		if err != nil {
			return 0, false, err
		}
		p.skipSpaces()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, false, fmt.Errorf("missing closing ')'")
		}
		p.pos++
		return val, known, nil
	}

	// Current PC: '*'
	if ch == '*' {
		p.pos++
		return int64(p.pc), true, nil
	}

	// Character literal: 'A'
	if ch == '\'' {
		p.pos++
		if p.pos >= len(p.s) {
			return 0, false, fmt.Errorf("unterminated char literal")
		}
		var val byte
		if p.s[p.pos] == '\\' {
			p.pos++
			if p.pos >= len(p.s) {
				return 0, false, fmt.Errorf("unterminated escape in char literal")
			}
			switch p.s[p.pos] {
			case 'n':
				val = '\n'
			case 'r':
				val = '\r'
			case 't':
				val = '\t'
			case '0':
				val = 0
			default:
				val = p.s[p.pos]
			}
			p.pos++
		} else {
			val = p.s[p.pos]
			p.pos++
		}
		if p.pos < len(p.s) && p.s[p.pos] == '\'' {
			p.pos++
		}
		return int64(val), true, nil
	}

	// Hex literal: $1234
	if ch == '$' {
		p.pos++
		start := p.pos
		for p.pos < len(p.s) && ((p.s[p.pos] >= '0' && p.s[p.pos] <= '9') ||
			(p.s[p.pos] >= 'a' && p.s[p.pos] <= 'f') ||
			(p.s[p.pos] >= 'A' && p.s[p.pos] <= 'F')) {
			p.pos++
		}
		if p.pos == start {
			return int64(p.pc), true, nil
		}
		v, err := strconv.ParseUint(p.s[start:p.pos], 16, 64)
		return int64(v), true, err
	}

	// Binary literal: %10101010
	if ch == '%' {
		p.pos++
		start := p.pos
		for p.pos < len(p.s) && (p.s[p.pos] == '0' || p.s[p.pos] == '1' || p.s[p.pos] == '_') {
			p.pos++
		}
		cleanBin := strings.ReplaceAll(p.s[start:p.pos], "_", "")
		v, err := strconv.ParseUint(cleanBin, 2, 64)
		return int64(v), true, err
	}

	// Numeric literal (0x..., 0b..., decimal, or trailing H)
	if ch >= '0' && ch <= '9' {
		start := p.pos
		for p.pos < len(p.s) && isIdentChar(p.s[p.pos]) {
			p.pos++
		}
		tok := p.s[start:p.pos]
		if strings.HasPrefix(tok, "0x") || strings.HasPrefix(tok, "0X") {
			v, err := strconv.ParseUint(tok[2:], 16, 64)
			return int64(v), true, err
		}
		if strings.HasPrefix(tok, "0b") || strings.HasPrefix(tok, "0B") {
			v, err := strconv.ParseUint(tok[2:], 2, 64)
			return int64(v), true, err
		}
		if strings.HasSuffix(tok, "h") || strings.HasSuffix(tok, "H") {
			v, err := strconv.ParseUint(tok[:len(tok)-1], 16, 64)
			return int64(v), true, err
		}
		v, err := strconv.ParseInt(tok, 10, 64)
		return v, true, err
	}

	// Symbol / Identifier
	start := p.pos
	for p.pos < len(p.s) && isIdentChar(p.s[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return 0, false, fmt.Errorf("unexpected token at %q", p.s[p.pos:])
	}
	sym := p.s[start:p.pos]
	if val, ok := p.asm.symbols[sym]; ok {
		return int64(val), true, nil
	}
	if p.pass >= 2 {
		return 0, false, fmt.Errorf("undefined symbol %q", sym)
	}
	return 0, false, nil
}

func (a *Assembler) computeDataBytes(rawOps string, pc uint32) ([]byte, error) {
	parts := splitTopLevelCommas(rawOps)
	var out []byte
	for _, part := range parts {
		if part == "" {
			continue
		}
		if len(part) >= 2 && (part[0] == '"' || part[0] == '\'') && part[len(part)-1] == part[0] {
			q := part[1 : len(part)-1]
			for i := 0; i < len(q); i++ {
				if q[i] == '\\' && i+1 < len(q) {
					i++
					switch q[i] {
					case 'n':
						out = append(out, '\n')
					case 'r':
						out = append(out, '\r')
					case 't':
						out = append(out, '\t')
					case '0':
						out = append(out, 0)
					case 'x':
						if i+2 < len(q) {
							if v, err := strconv.ParseUint(q[i+1:i+3], 16, 8); err == nil {
								out = append(out, byte(v))
								i += 2
								continue
							}
						}
						out = append(out, 'x')
					default:
						out = append(out, q[i])
					}
				} else {
					out = append(out, q[i])
				}
			}
			continue
		}
		val, _, err := a.EvalExpr(part, pc)
		if err != nil && a.pass >= 2 {
			return nil, err
		}
		out = append(out, byte(val&0xFF))
	}
	return out, nil
}

func (a *Assembler) computeInstructionSize(stmt *Statement) int {
	enc := opcodeTable[stmt.Mnemonic]
	switch stmt.ParsedMode {
	case ModeImp, ModeAcc:
		return 1
	case ModeImm, ModeZP, ModeZPX, ModeZPY, ModeIndX, ModeIndY:
		return 2
	case ModeRel:
		if stmt.BranchLong {
			return 5 // B!cc *+5 (2 bytes) + JMP target (3 bytes)
		}
		return 2
	case ModeInd:
		return 3
	case ModeAbs, ModeAbsX, ModeAbsY:
		if stmt.ForceZP {
			return 2
		}
		if stmt.ForceAbs {
			return 3
		}
		// Check if ZP mode exists for this instruction and mode
		hasZP := false
		switch stmt.ParsedMode {
		case ModeAbs:
			hasZP = enc.ZP != -1
		case ModeAbsX:
			hasZP = enc.ZPX != -1
		case ModeAbsY:
			hasZP = enc.ZPY != -1
		}
		if hasZP && stmt.UseZP {
			return 2
		}
		return 3
	}
	return 3
}

func (a *Assembler) Assemble() error {
	a.pass = 1

	// Initial pass: collect EQU and estimate sizes
	for _, stmt := range a.statements {
		if stmt.Type == StmtEqu {
			val, known, err := a.EvalExpr(stmt.RawOps, a.currPC)
			if err == nil && known {
				a.symbols[stmt.Label] = uint32(val) & 0xFFFF
			}
		}
		if stmt.Type == StmtInstruction {
			enc := opcodeTable[stmt.Mnemonic]
			// Check if operand is already known to be ZP in pass 1
			if !stmt.ForceAbs && (stmt.ParsedMode == ModeAbs || stmt.ParsedMode == ModeAbsX || stmt.ParsedMode == ModeAbsY) {
				hasZP := (stmt.ParsedMode == ModeAbs && enc.ZP != -1) ||
					(stmt.ParsedMode == ModeAbsX && enc.ZPX != -1) ||
					(stmt.ParsedMode == ModeAbsY && enc.ZPY != -1)
				if hasZP {
					if stmt.ForceZP {
						stmt.UseZP = true
					} else if val, known, err := a.EvalExpr(stmt.Expr, a.currPC); err == nil && known && val >= 0 && val <= 0xFF {
						stmt.UseZP = true
					}
				}
			}
		}
	}

	// Iterative relaxation loop (resolves symbol addresses, ZP vs Abs, and short vs long conditional branches)
	const maxIters = 20
	for iter := 0; iter < maxIters; iter++ {
		changed := false
		a.currPC = 0x0000

		for _, stmt := range a.statements {
			stmt.PC = a.currPC
			if stmt.SrcLine != nil && !stmt.SrcLine.HasPC {
				stmt.SrcLine.PC = a.currPC
				stmt.SrcLine.HasPC = true
			} else if stmt.SrcLine != nil {
				stmt.SrcLine.PC = a.currPC
			}

			if stmt.Label != "" && stmt.Type != StmtEqu {
				if old, ok := a.symbols[stmt.Label]; !ok || old != a.currPC {
					a.symbols[stmt.Label] = a.currPC
					changed = true
				}
			}

			switch stmt.Type {
			case StmtOrg:
				val, _, err := a.EvalExpr(stmt.RawOps, a.currPC)
				if err != nil {
					return fmt.Errorf("%s:%d: ORG error: %v", stmt.File, stmt.LineNum, err)
				}
				a.currPC = uint32(val) & 0xFFFF
				stmt.PC = a.currPC
			case StmtEqu:
				val, known, err := a.EvalExpr(stmt.RawOps, a.currPC)
				if err == nil && known {
					uval := uint32(val) & 0xFFFF
					if old, ok := a.symbols[stmt.Label]; !ok || old != uval {
						a.symbols[stmt.Label] = uval
						changed = true
					}
				}
			case StmtDataByte:
				b, _ := a.computeDataBytes(stmt.RawOps, a.currPC)
				if stmt.Mnemonic == "ASCIZ" || stmt.Mnemonic == "STRINGZ" || stmt.Mnemonic == "FCN" {
					b = append(b, 0)
				}
				stmt.Size = len(b)
				a.currPC += uint32(stmt.Size)
			case StmtDataWord:
				parts := splitTopLevelCommas(stmt.RawOps)
				stmt.Size = len(parts) * 2
				a.currPC += uint32(stmt.Size)
			case StmtReserve:
				val, _, err := a.EvalExpr(stmt.RawOps, a.currPC)
				if err != nil {
					return fmt.Errorf("%s:%d: reserve error: %v", stmt.File, stmt.LineNum, err)
				}
				stmt.Size = int(val)
				a.currPC += uint32(stmt.Size)
			case StmtAlign:
				val, _, err := a.EvalExpr(stmt.RawOps, a.currPC)
				if err != nil || val <= 0 {
					return fmt.Errorf("%s:%d: invalid ALIGN %q", stmt.File, stmt.LineNum, stmt.RawOps)
				}
				align := uint32(val)
				rem := a.currPC % align
				if rem != 0 {
					stmt.Size = int(align - rem)
				} else {
					stmt.Size = 0
				}
				a.currPC += uint32(stmt.Size)
			case StmtEnd:
				if strings.TrimSpace(stmt.RawOps) != "" {
					val, known, err := a.EvalExpr(stmt.RawOps, a.currPC)
					if err == nil && known {
						a.entryPoint = uint32(val) & 0xFFFF
						a.hasEntry = true
					}
				}
			case StmtInstruction:
				enc := opcodeTable[stmt.Mnemonic]
				if iter > 0 && a.relaxEnabled {
					// Check ZP relaxation
					if !stmt.ForceAbs && !stmt.ForceZP && (stmt.ParsedMode == ModeAbs || stmt.ParsedMode == ModeAbsX || stmt.ParsedMode == ModeAbsY) {
						hasZP := (stmt.ParsedMode == ModeAbs && enc.ZP != -1) ||
							(stmt.ParsedMode == ModeAbsX && enc.ZPX != -1) ||
							(stmt.ParsedMode == ModeAbsY && enc.ZPY != -1)
						if hasZP {
							if val, known, err := a.EvalExpr(stmt.Expr, stmt.PC); err == nil && known {
								wantZP := (val >= 0 && val <= 0xFF)
								if wantZP != stmt.UseZP {
									stmt.UseZP = wantZP
									changed = true
								}
							}
						}
					}
					// Check relative branch relaxation
					if stmt.ParsedMode == ModeRel && !stmt.BranchLong {
						if target, known, err := a.EvalExpr(stmt.Expr, stmt.PC); err == nil && known {
							offset := target - int64(stmt.PC+2)
							if offset < -128 || offset > 127 {
								stmt.BranchLong = true
								changed = true
							}
						}
					}
				}
				stmt.Size = a.computeInstructionSize(stmt)
				a.currPC += uint32(stmt.Size)
			}
		}

		if !changed && iter >= 1 {
			break
		}
	}

	// Pass 2: Final encoding
	a.pass = 2
	for _, stmt := range a.statements {
		switch stmt.Type {
		case StmtEqu:
			val, _, err := a.EvalExpr(stmt.RawOps, stmt.PC)
			if err != nil {
				return fmt.Errorf("%s:%d: EQU error: %v", stmt.File, stmt.LineNum, err)
			}
			a.symbols[stmt.Label] = uint32(val) & 0xFFFF
		case StmtDataByte:
			b, err := a.computeDataBytes(stmt.RawOps, stmt.PC)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			if stmt.Mnemonic == "ASCIZ" || stmt.Mnemonic == "STRINGZ" || stmt.Mnemonic == "FCN" {
				b = append(b, 0)
			}
			stmt.Encoded = b
		case StmtDataWord:
			parts := splitTopLevelCommas(stmt.RawOps)
			var out []byte
			for _, part := range parts {
				val, _, err := a.EvalExpr(part, stmt.PC)
				if err != nil {
					return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
				}
				uval := uint16(val & 0xFFFF)
				// 6502 is little-endian!
				out = append(out, byte(uval&0xFF), byte((uval>>8)&0xFF))
			}
			stmt.Encoded = out
		case StmtReserve, StmtAlign:
			stmt.Encoded = make([]byte, stmt.Size)
		case StmtEnd:
			if strings.TrimSpace(stmt.RawOps) != "" {
				val, _, err := a.EvalExpr(stmt.RawOps, stmt.PC)
				if err != nil {
					return fmt.Errorf("%s:%d: END error: %v", stmt.File, stmt.LineNum, err)
				}
				a.entryPoint = uint32(val) & 0xFFFF
				a.hasEntry = true
			}
		case StmtInstruction:
			enc, err := a.encodeInstruction(stmt)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Encoded = enc
		}
		if stmt.SrcLine != nil && len(stmt.Encoded) > 0 {
			stmt.SrcLine.Encoded = stmt.Encoded
		}
	}

	return nil
}

func (a *Assembler) encodeInstruction(stmt *Statement) ([]byte, error) {
	enc := opcodeTable[stmt.Mnemonic]

	switch stmt.ParsedMode {
	case ModeImp:
		if enc.Imp == -1 {
			return nil, fmt.Errorf("%s does not support implied mode", stmt.Mnemonic)
		}
		return []byte{byte(enc.Imp)}, nil
	case ModeAcc:
		if enc.Acc == -1 {
			return nil, fmt.Errorf("%s does not support accumulator mode", stmt.Mnemonic)
		}
		return []byte{byte(enc.Acc)}, nil
	case ModeImm:
		val, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		return []byte{byte(enc.Imm), byte(val & 0xFF)}, nil
	case ModeIndX:
		val, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		if val < 0 || val > 0xFF {
			return nil, fmt.Errorf("(zp,X) address $%X out of zero-page range", val)
		}
		return []byte{byte(enc.IndX), byte(val & 0xFF)}, nil
	case ModeIndY:
		val, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		if val < 0 || val > 0xFF {
			return nil, fmt.Errorf("(zp),Y address $%X out of zero-page range", val)
		}
		return []byte{byte(enc.IndY), byte(val & 0xFF)}, nil
	case ModeInd:
		val, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		uval := uint16(val & 0xFFFF)
		return []byte{byte(enc.Ind), byte(uval & 0xFF), byte((uval >> 8) & 0xFF)}, nil
	case ModeRel:
		target, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		if stmt.BranchLong {
			// Invert branch condition by flipping bit 5 (0x20), branch over 3-byte JMP (+3)
			invOp := byte(enc.Rel) ^ 0x20
			uval := uint16(target & 0xFFFF)
			return []byte{invOp, 0x03, 0x4C, byte(uval & 0xFF), byte((uval >> 8) & 0xFF)}, nil
		}
		offset := target - int64(stmt.PC+2)
		if offset < -128 || offset > 127 {
			return nil, fmt.Errorf("branch target out of range (%d bytes)", offset)
		}
		return []byte{byte(enc.Rel), byte(offset & 0xFF)}, nil
	case ModeAbs, ModeAbsX, ModeAbsY:
		val, _, err := a.EvalExpr(stmt.Expr, stmt.PC)
		if err != nil {
			return nil, err
		}
		if stmt.Size == 2 {
			var op int
			switch stmt.ParsedMode {
			case ModeAbs:
				op = enc.ZP
			case ModeAbsX:
				op = enc.ZPX
			case ModeAbsY:
				op = enc.ZPY
			}
			if op == -1 {
				return nil, fmt.Errorf("%s does not support zero-page addressing for this mode", stmt.Mnemonic)
			}
			return []byte{byte(op), byte(val & 0xFF)}, nil
		}
		var op int
		switch stmt.ParsedMode {
		case ModeAbs:
			op = enc.Abs
		case ModeAbsX:
			op = enc.AbsX
		case ModeAbsY:
			op = enc.AbsY
		}
		if op == -1 {
			// Fallback if only ZP/ZPY exists (e.g. STX zp,Y where AbsX doesn't exist)
			if stmt.ParsedMode == ModeAbsY && enc.ZPY != -1 && val >= 0 && val <= 0xFF {
				return []byte{byte(enc.ZPY), byte(val & 0xFF)}, nil
			}
			if stmt.ParsedMode == ModeAbsX && enc.ZPX != -1 && val >= 0 && val <= 0xFF {
				return []byte{byte(enc.ZPX), byte(val & 0xFF)}, nil
			}
			return nil, fmt.Errorf("%s does not support 16-bit absolute addressing for this index mode", stmt.Mnemonic)
		}
		uval := uint16(val & 0xFFFF)
		return []byte{byte(op), byte(uval & 0xFF), byte((uval >> 8) & 0xFF)}, nil
	}

	return nil, fmt.Errorf("unsupported addressing mode for %s", stmt.Mnemonic)
}

// EmitDECB writes an extended Hatvan DECB binary with magic header 'x', '5' (MOS 6502).
func (a *Assembler) EmitDECB(w io.Writer) error {
	// Hatvan Executable Magic header (Tag 253 = 0xFD): 'x', '5' (MOS 6502)
	magicHdr := []byte{0xFD, 0x00, 0x00, 'x', '5'}
	if _, err := w.Write(magicHdr); err != nil {
		return err
	}

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

	// Postamble trailer (Tag 255 = 0xFF): execution entry point
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
			limit := len(line.Encoded)
			if limit > 6 {
				limit = 6
			}
			hexParts := make([]string, limit)
			for i := 0; i < limit; i++ {
				hexParts[i] = fmt.Sprintf("%02X", line.Encoded[i])
			}
			hexStr = strings.Join(hexParts, " ")
		}
		if line.HasPC {
			fmt.Fprintf(writer, "%04X  %-18s %5d  %s\n", line.PC, hexStr, line.LineNum, line.Raw)
		} else {
			fmt.Fprintf(writer, "      %-18s %5d  %s\n", hexStr, line.LineNum, line.Raw)
		}
	}
	return nil
}

func main() {
	outFlag := flag.String("o", "", "Output binary file path")
	listFlag := flag.String("l", "", "Output listing file path")
	flag.StringVar(listFlag, "list", "", "Output listing file path")
	formatFlag := flag.String("format", "decb", "Output format: decb or raw")
	flag.StringVar(formatFlag, "f", "decb", "Output format: decb or raw")
	decbFlag := flag.Bool("b", false, "Generate DECB format")
	flag.BoolVar(decbFlag, "decb", false, "Generate DECB format")
	rawFlag := flag.Bool("r", false, "Generate raw binary format")
	flag.BoolVar(rawFlag, "raw", false, "Generate raw binary format")
	relaxFlag := flag.Bool("relax", true, "Enable zero-page and branch relaxation")
	noRelaxFlag := flag.Bool("no-relax", false, "Disable relaxation")

	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: asm6502 -o <output> [-l <listing.list>] [-format=decb|raw] [-relax|-no-relax] <source.asm>...\n")
		os.Exit(1)
	}

	if *decbFlag {
		*formatFlag = "decb"
	} else if *rawFlag {
		*formatFlag = "raw"
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
