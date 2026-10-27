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

type SegmentID int

const (
	SegCode SegmentID = iota // CS = $1000 -> physical $10000..$1FFFF (High16 = $0001)
	SegData                  // DS = $0000 -> physical $00000..$0FFFF (High16 = $0000)
)

type StmtType int

const (
	StmtInstruction StmtType = iota
	StmtSegment
	StmtOrg
	StmtEqu
	StmtDataByte
	StmtDataWord
	StmtReserve
	StmtAlign
	StmtEnd
	StmtComment
)

type SourceLine struct {
	File    string
	LineNum int
	Raw     string
	Seg     SegmentID
	PC      uint32
	HasPC   bool
	Encoded []byte
}

type Statement struct {
	Type       StmtType
	Seg        SegmentID
	TargetSeg  SegmentID
	Label      string
	Mnemonic   string
	RawOps     string
	LineNum    int
	File       string
	PC         uint32
	Size       int
	Encoded    []byte
	SrcLine    *SourceLine
	BranchLong bool // true when Jcc or JMP is relaxed to near displacement
}

type Assembler struct {
	symbols      map[string]int64
	statements   []*Statement
	sourceLines  []*SourceLine
	activeSeg    SegmentID
	segPC        [2]uint32
	segHasOrg    [2]bool
	pass         int
	entryPoint   uint32
	hasEntry     bool
	relaxEnabled bool
}

func NewAssembler() *Assembler {
	a := &Assembler{
		symbols:      make(map[string]int64),
		activeSeg:    SegCode,
		relaxEnabled: true,
	}
	a.segPC[SegCode] = 0x0200
	a.segPC[SegData] = 0x0200
	return a
}

var jccOpcodes = map[string]byte{
	"JO":   0x70,
	"JNO":  0x71,
	"JB":   0x72,
	"JNAE": 0x72,
	"JC":   0x72,
	"JAE":  0x73,
	"JNB":  0x73,
	"JNC":  0x73,
	"JE":   0x74,
	"JZ":   0x74,
	"JNE":  0x75,
	"JNZ":  0x75,
	"JBE":  0x76,
	"JNA":  0x76,
	"JA":   0x77,
	"JNBE": 0x77,
	"JS":   0x78,
	"JNS":  0x79,
	"JP":   0x7A,
	"JPE":  0x7A,
	"JNP":  0x7B,
	"JPO":  0x7B,
	"JL":   0x7C,
	"JNGE": 0x7C,
	"JGE":  0x7D,
	"JNL":  0x7D,
	"JLE":  0x7E,
	"JNG":  0x7E,
	"JG":   0x7F,
	"JNLE": 0x7F,
}

var loopOpcodes = map[string]byte{
	"LOOPNE": 0xE0,
	"LOOPNZ": 0xE0,
	"LOOPE":  0xE1,
	"LOOPZ":  0xE1,
	"LOOP":   0xE2,
	"JCXZ":   0xE3,
}

var impliedOpcodes = map[string][]byte{
	"NOP":   {0x90},
	"CBW":   {0x98},
	"CWD":   {0x99},
	"PUSHF": {0x9C},
	"POPF":  {0x9D},
	"SAHF":  {0x9E},
	"LAHF":  {0x9F},
	"MOVSB": {0xA4},
	"MOVSW": {0xA5},
	"CMPSB": {0xA6},
	"CMPSW": {0xA7},
	"STOSB": {0xAA},
	"STOSW": {0xAB},
	"LODSB": {0xAC},
	"LODSW": {0xAD},
	"SCASB": {0xAE},
	"SCASW": {0xAF},
	"RET":   {0xC3},
	"RETN":  {0xC3},
	"RETF":  {0xCB},
	"INT3":  {0xCC},
	"INTO":  {0xCE},
	"IRET":  {0xCF},
	"XLAT":  {0xD7},
	"XLATB": {0xD7},
	"REPNE": {0xF2},
	"REPNZ": {0xF2},
	"REP":   {0xF3},
	"REPE":  {0xF3},
	"REPZ":  {0xF3},
	"HLT":   {0xF4},
	"CMC":   {0xF5},
	"CLC":   {0xF8},
	"STC":   {0xF9},
	"CLI":   {0xFA},
	"STI":   {0xFB},
	"CLD":   {0xFC},
	"STD":   {0xFD},
}

var aluTable = map[string]byte{
	"ADD": 0,
	"OR":  1,
	"ADC": 2,
	"SBB": 3,
	"AND": 4,
	"SUB": 5,
	"XOR": 6,
	"CMP": 7,
}

var shiftTable = map[string]byte{
	"ROL": 0,
	"ROR": 1,
	"RCL": 2,
	"RCR": 3,
	"SHL": 4,
	"SAL": 4,
	"SHR": 5,
	"SAR": 7,
}

var grp3Table = map[string]byte{
	"NOT":  2,
	"NEG":  3,
	"MUL":  4,
	"IMUL": 5,
	"DIV":  6,
	"IDIV": 7,
}

func parseReg8(s string) (byte, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "AL":
		return 0, true
	case "CL":
		return 1, true
	case "DL":
		return 2, true
	case "BL":
		return 3, true
	case "AH":
		return 4, true
	case "CH":
		return 5, true
	case "DH":
		return 6, true
	case "BH":
		return 7, true
	}
	return 0, false
}

func parseReg16(s string) (byte, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "AX":
		return 0, true
	case "CX":
		return 1, true
	case "DX":
		return 2, true
	case "BX":
		return 3, true
	case "SP":
		return 4, true
	case "BP":
		return 5, true
	case "SI":
		return 6, true
	case "DI":
		return 7, true
	}
	return 0, false
}

func parseSegReg(s string) (byte, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ES":
		return 0, true
	case "CS":
		return 1, true
	case "SS":
		return 2, true
	case "DS":
		return 3, true
	}
	return 0, false
}

func stripComment(line string) string {
	inQuote := false
	quoteChar := byte(0)
	escaped := false

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
			} else if ch == ';' || (ch == '/' && i+1 < len(line) && line[i+1] == '/') {
				return line[:i]
			}
		}
	}
	return line
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	var cur bytes.Buffer
	inQuote := false
	quoteChar := byte(0)
	escaped := false
	bracketDepth := 0
	parenDepth := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			cur.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			cur.WriteByte(ch)
			continue
		}
		switch ch {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ',':
			if bracketDepth == 0 && parenDepth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 || len(parts) > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}
	return parts
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

		clean := strings.TrimRight(stripComment(raw), " \t\r\n")
		if strings.TrimSpace(clean) == "" {
			a.statements = append(a.statements, &Statement{
				Type:    StmtComment,
				LineNum: lineNum,
				File:    filename,
				SrcLine: srcLine,
			})
			continue
		}

		// Check for label: either "label:" anywhere at start, or column-0 identifier before EQU/=
		trimmed := strings.TrimSpace(clean)
		var label, mnemonic, rawOps string

		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && (strings.EqualFold(fields[1], "EQU") || fields[1] == "=") {
			label = strings.TrimSuffix(fields[0], ":")
			mnemonic = "EQU"
			idx := strings.Index(trimmed, fields[1])
			rawOps = strings.TrimSpace(trimmed[idx+len(fields[1]):])
		} else {
			rest := trimmed
			if colonIdx := strings.IndexByte(rest, ':'); colonIdx != -1 {
				// Ensure colon is part of a leading label, not inside quotes, brackets, or far address
				prefix := rest[:colonIdx]
				if isValidLabelName(prefix) {
					label = prefix
					rest = strings.TrimSpace(rest[colonIdx+1:])
				}
			} else if clean[0] != ' ' && clean[0] != '\t' {
				// Column 0 label without colon
				f0 := fields[0]
				if !isKnownMnemonicOrDirective(strings.ToUpper(f0)) {
					label = f0
					rest = strings.TrimSpace(rest[len(f0):])
				}
			}

			if rest != "" {
				subFields := strings.Fields(rest)
				mnemonic = strings.ToUpper(subFields[0])
				rawOps = strings.TrimSpace(rest[len(subFields[0]):])
				// Handle REP / REPE / REPZ / REPNE / REPNZ prefix on the same line as string op
				if isRepPrefix(mnemonic) && rawOps != "" {
					if label != "" {
						a.statements = append(a.statements, &Statement{
							Type:    StmtComment,
							Label:   label,
							LineNum: lineNum,
							File:    filename,
							SrcLine: srcLine,
						})
						label = ""
					}
					a.statements = append(a.statements, &Statement{
						Type:     StmtInstruction,
						Mnemonic: mnemonic,
						LineNum:  lineNum,
						File:     filename,
						SrcLine:  srcLine,
					})
					repRest := strings.Fields(rawOps)
					mnemonic = strings.ToUpper(repRest[0])
					rawOps = strings.TrimSpace(rawOps[len(repRest[0]):])
				}
			}
		}

		stmt := &Statement{
			Label:    label,
			Mnemonic: strings.ToUpper(mnemonic),
			RawOps:   rawOps,
			LineNum:  lineNum,
			File:     filename,
			SrcLine:  srcLine,
		}

		switch stmt.Mnemonic {
		case "":
			stmt.Type = StmtComment
		case "SEGMENT", ".SEGMENT", "SECTION":
			stmt.Type = StmtSegment
			segName := strings.ToUpper(strings.TrimSpace(stmt.RawOps))
			switch segName {
			case "CODE", ".TEXT", "TEXT":
				stmt.TargetSeg = SegCode
			case "DATA", ".DATA", "RODATA", ".RODATA", "BSS", ".BSS":
				stmt.TargetSeg = SegData
			default:
				return fmt.Errorf("%s:%d: unknown segment %q (expected CODE or DATA)", filename, lineNum, stmt.RawOps)
			}
		case ".TEXT":
			stmt.Type = StmtSegment
			stmt.TargetSeg = SegCode
		case ".DATA":
			stmt.Type = StmtSegment
			stmt.TargetSeg = SegData
		case "ORG":
			stmt.Type = StmtOrg
		case "EQU", "=":
			stmt.Type = StmtEqu
		case "DB", "DEFB", "FCB", ".BYTE", "ASCIZ":
			stmt.Type = StmtDataByte
		case "DW", "DEFW", "FDB", ".WORD":
			stmt.Type = StmtDataWord
		case "DS", "DEFS", "RMB", "RESB", ".BLKB":
			stmt.Type = StmtReserve
		case "ALIGN":
			stmt.Type = StmtAlign
		case "EVEN":
			stmt.Type = StmtAlign
			stmt.RawOps = "2"
		case "END":
			stmt.Type = StmtEnd
		case "GLOBAL", "GLOBL", "EXTERN", "EXTRN", "BITS", "USE16":
			stmt.Type = StmtComment
		default:
			stmt.Type = StmtInstruction
		}

		a.statements = append(a.statements, stmt)
	}

	return scanner.Err()
}

func isValidLabelName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '.' || ch == '$' {
			continue
		}
		if i > 0 && ch >= '0' && ch <= '9' {
			continue
		}
		return false
	}
	if _, ok := parseSegReg(s); ok {
		return false
	}
	return true
}

func isRepPrefix(m string) bool {
	switch m {
	case "REP", "REPE", "REPZ", "REPNE", "REPNZ":
		return true
	}
	return false
}

func isKnownMnemonicOrDirective(m string) bool {
	if _, ok := impliedOpcodes[m]; ok {
		return true
	}
	if _, ok := jccOpcodes[m]; ok {
		return true
	}
	if _, ok := loopOpcodes[m]; ok {
		return true
	}
	if _, ok := aluTable[m]; ok {
		return true
	}
	if _, ok := shiftTable[m]; ok {
		return true
	}
	if _, ok := grp3Table[m]; ok {
		return true
	}
	switch m {
	case "MOV", "LEA", "PUSH", "POP", "XCHG", "TEST", "INC", "DEC",
		"CALL", "JMP", "RET", "RETN", "RETF", "INT", "IN", "OUT",
		"SEGMENT", ".SEGMENT", "SECTION", ".TEXT", ".DATA",
		"ORG", "EQU", "DB", "DEFB", "FCB", ".BYTE", "ASCIZ",
		"DW", "DEFW", "FDB", ".WORD", "DS", "DEFS", "RMB", "RESB", ".BLKB",
		"ALIGN", "EVEN", "END", "GLOBAL", "GLOBL", "EXTERN", "EXTRN":
		return true
	}
	return false
}

// ── Expression Evaluator ─────────────────────────────────────────────────────

type exprParser struct {
	asm   *Assembler
	input string
	pos   int
	pc    uint32
	known bool
}

func (a *Assembler) EvalExpr(expr string, pc uint32) (int64, bool, error) {
	p := &exprParser{
		asm:   a,
		input: strings.TrimSpace(expr),
		pos:   0,
		pc:    pc,
		known: true,
	}
	if p.input == "" {
		return 0, false, fmt.Errorf("empty expression")
	}
	val, err := p.parseBitOr()
	if err != nil {
		return 0, false, err
	}
	p.skipSpaces()
	if p.pos < len(p.input) {
		return 0, false, fmt.Errorf("unexpected trailing characters %q in expression %q", p.input[p.pos:], expr)
	}
	return val, p.known, nil
}

func (p *exprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) parseBitOr() (int64, error) {
	left, err := p.parseBitXor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.input) && p.input[p.pos] == '|' {
			p.pos++
			right, err := p.parseBitXor()
			if err != nil {
				return 0, err
			}
			left |= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseBitXor() (int64, error) {
	left, err := p.parseBitAnd()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.input) && p.input[p.pos] == '^' {
			p.pos++
			right, err := p.parseBitAnd()
			if err != nil {
				return 0, err
			}
			left ^= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseBitAnd() (int64, error) {
	left, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.input) && p.input[p.pos] == '&' {
			p.pos++
			right, err := p.parseShift()
			if err != nil {
				return 0, err
			}
			left &= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseShift() (int64, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '<' && p.input[p.pos+1] == '<' {
			p.pos += 2
			right, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			left <<= uint(right)
		} else if p.pos+1 < len(p.input) && p.input[p.pos] == '>' && p.input[p.pos+1] == '>' {
			p.pos += 2
			right, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			left >>= uint(right)
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseAddSub() (int64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
			op := p.input[p.pos]
			p.pos++
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			if op == '+' {
				left += right
			} else {
				left -= right
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseMulDiv() (int64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos < len(p.input) && (p.input[p.pos] == '*' || p.input[p.pos] == '/' || p.input[p.pos] == '%') {
			op := p.input[p.pos]
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			switch op {
			case '*':
				left *= right
			case '/':
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				left /= right
			case '%':
				if right == 0 {
					return 0, fmt.Errorf("modulo by zero")
				}
				left %= right
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (int64, error) {
	p.skipSpaces()
	if p.pos < len(p.input) {
		switch p.input[p.pos] {
		case '+':
			p.pos++
			return p.parseUnary()
		case '-':
			p.pos++
			v, err := p.parseUnary()
			return -v, err
		case '~':
			p.pos++
			v, err := p.parseUnary()
			return ^v, err
		case '<':
			p.pos++
			v, err := p.parseUnary()
			return v & 0xFF, err
		case '>':
			p.pos++
			v, err := p.parseUnary()
			return (v >> 8) & 0xFF, err
		}
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	ch := p.input[p.pos]

	if ch == '(' {
		p.pos++
		val, err := p.parseBitOr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing ')'")
		}
		p.pos++
		return val, nil
	}

	if ch == '\'' {
		p.pos++
		if p.pos >= len(p.input) {
			return 0, fmt.Errorf("unterminated char literal")
		}
		var val byte
		if p.input[p.pos] == '\\' {
			p.pos++
			if p.pos >= len(p.input) {
				return 0, fmt.Errorf("unterminated escape in char literal")
			}
			switch p.input[p.pos] {
			case 'n':
				val = '\n'
			case 'r':
				val = '\r'
			case 't':
				val = '\t'
			case '0':
				val = 0
			case '\\':
				val = '\\'
			case '\'':
				val = '\''
			default:
				val = p.input[p.pos]
			}
			p.pos++
		} else {
			val = p.input[p.pos]
			p.pos++
		}
		if p.pos < len(p.input) && p.input[p.pos] == '\'' {
			p.pos++
		}
		return int64(val), nil
	}

	if ch == '$' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && isHexDigit(p.input[p.pos]) {
			p.pos++
		}
		if p.pos == start {
			// '$' alone means current instruction PC
			return int64(p.pc), nil
		}
		v, err := strconv.ParseUint(p.input[start:p.pos], 16, 64)
		return int64(v), err
	}

	if ch >= '0' && ch <= '9' {
		start := p.pos
		if ch == '0' && p.pos+1 < len(p.input) && (p.input[p.pos+1] == 'x' || p.input[p.pos+1] == 'X') {
			p.pos += 2
			hexStart := p.pos
			for p.pos < len(p.input) && isHexDigit(p.input[p.pos]) {
				p.pos++
			}
			v, err := strconv.ParseUint(p.input[hexStart:p.pos], 16, 64)
			return int64(v), err
		}
		if ch == '0' && p.pos+1 < len(p.input) && (p.input[p.pos+1] == 'b' || p.input[p.pos+1] == 'B') {
			// Check if followed by only 0/1 and not trailing 'h'/'H'
			savePos := p.pos
			p.pos += 2
			binStart := p.pos
			for p.pos < len(p.input) && (p.input[p.pos] == '0' || p.input[p.pos] == '1') {
				p.pos++
			}
			if p.pos > binStart && (p.pos >= len(p.input) || !isHexDigit(p.input[p.pos]) && p.input[p.pos] != 'h' && p.input[p.pos] != 'H') {
				v, err := strconv.ParseUint(p.input[binStart:p.pos], 2, 64)
				return int64(v), err
			}
			p.pos = savePos
		}
		for p.pos < len(p.input) && isHexDigit(p.input[p.pos]) {
			p.pos++
		}
		token := p.input[start:p.pos]
		if p.pos < len(p.input) && (p.input[p.pos] == 'h' || p.input[p.pos] == 'H') {
			p.pos++
			v, err := strconv.ParseUint(token, 16, 64)
			return int64(v), err
		}
		if strings.HasSuffix(token, "b") || strings.HasSuffix(token, "B") {
			allBin := true
			for i := 0; i < len(token)-1; i++ {
				if token[i] != '0' && token[i] != '1' {
					allBin = false
					break
				}
			}
			if allBin && len(token) > 1 {
				v, err := strconv.ParseUint(token[:len(token)-1], 2, 64)
				return int64(v), err
			}
		}
		v, err := strconv.ParseInt(token, 10, 64)
		return v, err
	}

	// Symbol / Identifier (including optional "OFFSET" keyword)
	start := p.pos
	for p.pos < len(p.input) && isSymChar(p.input[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("unexpected character %q", p.input[p.pos])
	}
	sym := p.input[start:p.pos]
	if strings.EqualFold(sym, "OFFSET") {
		return p.parseUnary()
	}
	if val, ok := p.asm.symbols[sym]; ok {
		return val, nil
	}
	p.known = false
	if p.asm.pass == 2 {
		return 0, fmt.Errorf("undefined symbol %q", sym)
	}
	return 0, nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isSymChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '$'
}

// isPureLiteral returns true if expr does not reference any symbolic identifier.
func isPureLiteral(expr string) bool {
	s := strings.TrimSpace(expr)
	if s == "" {
		return false
	}
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
		if ch == '\'' || ch == '"' {
			inQuote = true
			quoteChar = ch
			continue
		}
		if ch == '$' {
			// Hex literal $1234
			i++
			for i < len(s) && isHexDigit(s[i]) {
				i++
			}
			i--
			continue
		}
		if ch >= '0' && ch <= '9' {
			if ch == '0' && i+1 < len(s) && (s[i+1] == 'x' || s[i+1] == 'X') {
				i += 2
			}
			for i < len(s) && (isHexDigit(s[i]) || s[i] == 'h' || s[i] == 'H' || s[i] == 'b' || s[i] == 'B') {
				i++
			}
			i--
			continue
		}
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '.' {
			return false
		}
	}
	return true
}

// ── 8086 Operand Parsing & ModR/M Encoding ──────────────────────────────────

type OpKind int

const (
	OpNone OpKind = iota
	OpReg8
	OpReg16
	OpSegReg
	OpMem
	OpImm
)

type ParsedOp struct {
	Kind       OpKind
	SizeHint   int  // 0 = unspecified, 1 = BYTE PTR, 2 = WORD PTR
	Reg        byte // Reg8 (0..7), Reg16 (0..7), or SegReg (0..3)
	HasSegOvr  bool
	SegOvrByte byte
	// Memory fields (OpMem):
	HasBaseIdx bool
	RM         byte
	DispExpr   string
	// Immediate / Expression fields (OpImm):
	Expr      string
	NearHint  bool
	ShortHint bool
}

func parseOperand8086(raw string) (ParsedOp, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ParsedOp{Kind: OpNone}, nil
	}

	var op ParsedOp

	// Parse leading qualifiers: BYTE PTR, WORD PTR, BYTE, WORD, NEAR, SHORT
	for {
		fields := strings.Fields(s)
		if len(fields) == 0 {
			break
		}
		u0 := strings.ToUpper(fields[0])
		if u0 == "BYTE" {
			op.SizeHint = 1
			s = strings.TrimSpace(s[len(fields[0]):])
			if f2 := strings.Fields(s); len(f2) > 0 && strings.EqualFold(f2[0], "PTR") {
				s = strings.TrimSpace(s[len(f2[0]):])
			}
			continue
		}
		if u0 == "WORD" {
			op.SizeHint = 2
			s = strings.TrimSpace(s[len(fields[0]):])
			if f2 := strings.Fields(s); len(f2) > 0 && strings.EqualFold(f2[0], "PTR") {
				s = strings.TrimSpace(s[len(f2[0]):])
			}
			continue
		}
		if u0 == "NEAR" {
			op.NearHint = true
			s = strings.TrimSpace(s[len(fields[0]):])
			if f2 := strings.Fields(s); len(f2) > 0 && strings.EqualFold(f2[0], "PTR") {
				s = strings.TrimSpace(s[len(f2[0]):])
			}
			continue
		}
		if u0 == "SHORT" {
			op.ShortHint = true
			s = strings.TrimSpace(s[len(fields[0]):])
			continue
		}
		if u0 == "OFFSET" {
			s = strings.TrimSpace(s[len(fields[0]):])
			op.Kind = OpImm
			op.Expr = s
			return op, nil
		}
		break
	}

	// Check for optional segment override prefix before '[': e.g. ES:[DI], CS:[BX]
	if len(s) >= 4 && s[2] == ':' && s[3] == '[' {
		if sr, ok := parseSegReg(s[:2]); ok {
			op.HasSegOvr = true
			switch sr {
			case 0:
				op.SegOvrByte = 0x26 // ES:
			case 1:
				op.SegOvrByte = 0x2E // CS:
			case 2:
				op.SegOvrByte = 0x36 // SS:
			case 3:
				op.SegOvrByte = 0x3E // DS:
			}
			s = strings.TrimSpace(s[3:])
		}
	}

	// Memory operand: [ ... ]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		op.Kind = OpMem
		hasBX, hasBP, hasSI, hasDI := false, false, false, false

		terms := splitSignedTerms(inner)
		var dispParts []string
		for _, t := range terms {
			sign := t.sign
			body := strings.TrimSpace(t.body)
			uBody := strings.ToUpper(body)
			if sign == '+' {
				switch uBody {
				case "BX":
					if !hasBX {
						hasBX = true
						continue
					}
				case "BP":
					if !hasBP {
						hasBP = true
						continue
					}
				case "SI":
					if !hasSI {
						hasSI = true
						continue
					}
				case "DI":
					if !hasDI {
						hasDI = true
						continue
					}
				}
			}
			if len(dispParts) == 0 && sign == '+' {
				dispParts = append(dispParts, body)
			} else {
				dispParts = append(dispParts, string(sign)+body)
			}
		}

		op.DispExpr = strings.Join(dispParts, " ")

		if hasBX || hasBP || hasSI || hasDI {
			op.HasBaseIdx = true
			switch {
			case hasBX && !hasBP && hasSI && !hasDI:
				op.RM = 0 // [BX+SI]
			case hasBX && !hasBP && !hasSI && hasDI:
				op.RM = 1 // [BX+DI]
			case !hasBX && hasBP && hasSI && !hasDI:
				op.RM = 2 // [BP+SI]
			case !hasBX && hasBP && !hasSI && hasDI:
				op.RM = 3 // [BP+DI]
			case !hasBX && !hasBP && hasSI && !hasDI:
				op.RM = 4 // [SI]
			case !hasBX && !hasBP && !hasSI && hasDI:
				op.RM = 5 // [DI]
			case !hasBX && hasBP && !hasSI && !hasDI:
				op.RM = 6 // [BP]
			case hasBX && !hasBP && !hasSI && !hasDI:
				op.RM = 7 // [BX]
			default:
				return op, fmt.Errorf("invalid 8086 base/index combination in %q", raw)
			}
		} else {
			op.HasBaseIdx = false
			op.RM = 6 // Direct [disp16]
		}
		return op, nil
	}

	if r8, ok := parseReg8(s); ok {
		op.Kind = OpReg8
		op.Reg = r8
		op.SizeHint = 1
		return op, nil
	}
	if r16, ok := parseReg16(s); ok {
		op.Kind = OpReg16
		op.Reg = r16
		op.SizeHint = 2
		return op, nil
	}
	if sr, ok := parseSegReg(s); ok {
		op.Kind = OpSegReg
		op.Reg = sr
		op.SizeHint = 2
		return op, nil
	}

	op.Kind = OpImm
	op.Expr = s
	return op, nil
}

type signedTerm struct {
	sign byte
	body string
}

func splitSignedTerms(s string) []signedTerm {
	var terms []signedTerm
	sign := byte('+')
	parenDepth := 0
	inQuote := false
	quoteChar := byte(0)
	start := 0

	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		sign = s[i]
		i++
	}
	start = i

	for ; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inQuote = true
			quoteChar = ch
			continue
		}
		if ch == '(' {
			parenDepth++
		} else if ch == ')' && parenDepth > 0 {
			parenDepth--
		} else if (ch == '+' || ch == '-') && parenDepth == 0 {
			body := strings.TrimSpace(s[start:i])
			if body != "" {
				terms = append(terms, signedTerm{sign: sign, body: body})
			}
			sign = ch
			start = i + 1
		}
	}
	body := strings.TrimSpace(s[start:])
	if body != "" {
		terms = append(terms, signedTerm{sign: sign, body: body})
	}
	return terms
}

// encodeModRM encodes the ModR/M byte and displacement bytes for a memory or register operand.
func (a *Assembler) encodeModRM(regField byte, rmOp ParsedOp, pc uint32) ([]byte, error) {
	if rmOp.Kind == OpReg8 || rmOp.Kind == OpReg16 {
		modrm := byte(0xC0) | ((regField & 7) << 3) | (rmOp.Reg & 7)
		return []byte{modrm}, nil
	}
	if rmOp.Kind != OpMem {
		return nil, fmt.Errorf("expected register or memory operand")
	}

	if !rmOp.HasBaseIdx {
		// Direct memory [disp16]: Mod=00, R/M=110
		val, _, err := a.EvalExpr(rmOp.DispExpr, pc)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		uval := uint16(val & 0xFFFF)
		modrm := ((regField & 7) << 3) | 6
		return []byte{modrm, byte(uval & 0xFF), byte(uval >> 8)}, nil
	}

	// Base/Index register addressing
	if strings.TrimSpace(rmOp.DispExpr) == "" {
		if rmOp.RM == 6 {
			// [BP] with 0 displacement requires Mod=01, disp8=0
			modrm := byte(0x40) | ((regField & 7) << 3) | 6
			return []byte{modrm, 0x00}, nil
		}
		modrm := ((regField & 7) << 3) | (rmOp.RM & 7)
		return []byte{modrm}, nil
	}

	val, known, err := a.EvalExpr(rmOp.DispExpr, pc)
	if err != nil && a.pass == 2 {
		return nil, err
	}

	// Use disp8 if displacement is a pure constant (or known) within -128..127
	if isPureLiteral(rmOp.DispExpr) && known {
		if val == 0 && rmOp.RM != 6 {
			modrm := ((regField & 7) << 3) | (rmOp.RM & 7)
			return []byte{modrm}, nil
		}
		if val >= -128 && val <= 127 {
			modrm := byte(0x40) | ((regField & 7) << 3) | (rmOp.RM & 7)
			return []byte{modrm, byte(int8(val))}, nil
		}
	}

	// 16-bit displacement: Mod=10
	uval := uint16(val & 0xFFFF)
	modrm := byte(0x80) | ((regField & 7) << 3) | (rmOp.RM & 7)
	return []byte{modrm, byte(uval & 0xFF), byte(uval >> 8)}, nil
}

func prependSegOvr(prefix []byte, ops ...ParsedOp) []byte {
	for _, op := range ops {
		if op.HasSegOvr {
			return append([]byte{op.SegOvrByte}, prefix...)
		}
	}
	return prefix
}

func (a *Assembler) encodeInstruction(stmt *Statement) ([]byte, error) {
	mnem := stmt.Mnemonic
	opsRaw := splitTopLevelCommas(stmt.RawOps)

	// 1. Implied single-byte instructions (when no operands)
	if len(opsRaw) == 0 {
		if enc, ok := impliedOpcodes[mnem]; ok {
			return enc, nil
		}
		return nil, fmt.Errorf("missing operands for %s", mnem)
	}

	// 2. Conditional jumps (Jcc)
	if ccOp, ok := jccOpcodes[mnem]; ok {
		if len(opsRaw) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		targetVal, known, err := a.EvalExpr(opsRaw[0], stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		if !stmt.BranchLong {
			disp := targetVal - int64(stmt.PC+2)
			if a.pass == 2 && known && (disp < -128 || disp > 127) {
				return nil, fmt.Errorf("%s short branch out of range (%d)", mnem, disp)
			}
			return []byte{ccOp, byte(int8(disp))}, nil
		}
		// Relaxed 5-byte sequence: J!cc $+5 (2 bytes); JMP NEAR target (3 bytes)
		invOp := ccOp ^ 0x01
		rel16 := uint16((targetVal - int64(stmt.PC+5)) & 0xFFFF)
		return []byte{invOp, 0x03, 0xE9, byte(rel16 & 0xFF), byte(rel16 >> 8)}, nil
	}

	// 3. LOOP / JCXZ
	if loopOp, ok := loopOpcodes[mnem]; ok {
		if len(opsRaw) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		targetVal, _, err := a.EvalExpr(opsRaw[0], stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		disp := targetVal - int64(stmt.PC+2)
		if a.pass == 2 && (disp < -128 || disp > 127) {
			return nil, fmt.Errorf("%s displacement %d out of range [-128..127]", mnem, disp)
		}
		return []byte{loopOp, byte(int8(disp))}, nil
	}

	// 4. RET / RETN / RETF with immediate pop count
	if mnem == "RET" || mnem == "RETN" || mnem == "RETF" {
		val, _, err := a.EvalExpr(opsRaw[0], stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		uval := uint16(val & 0xFFFF)
		opByte := byte(0xC2)
		if mnem == "RETF" {
			opByte = 0xCA
		}
		return []byte{opByte, byte(uval & 0xFF), byte(uval >> 8)}, nil
	}

	// 5. INT imm8
	if mnem == "INT" {
		val, _, err := a.EvalExpr(opsRaw[0], stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		return []byte{0xCD, byte(val & 0xFF)}, nil
	}

	// 6. CALL and JMP
	if mnem == "CALL" || mnem == "JMP" {
		if len(opsRaw) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		rawTarget := strings.TrimSpace(opsRaw[0])
		// Check for far seg:off literal (e.g. JMP $1000:$0200)
		if colonIdx := strings.IndexByte(rawTarget, ':'); colonIdx != -1 && !strings.Contains(rawTarget, "[") {
			segStr := strings.TrimSpace(rawTarget[:colonIdx])
			offStr := strings.TrimSpace(rawTarget[colonIdx+1:])
			segVal, _, err1 := a.EvalExpr(segStr, stmt.PC)
			offVal, _, err2 := a.EvalExpr(offStr, stmt.PC)
			if a.pass == 2 && (err1 != nil || err2 != nil) {
				return nil, fmt.Errorf("invalid far target %q", rawTarget)
			}
			opByte := byte(0xEA)
			if mnem == "CALL" {
				opByte = 0x9A
			}
			return []byte{
				opByte,
				byte(offVal & 0xFF), byte((offVal >> 8) & 0xFF),
				byte(segVal & 0xFF), byte((segVal >> 8) & 0xFF),
			}, nil
		}

		op, err := parseOperand8086(rawTarget)
		if err != nil {
			return nil, err
		}
		if op.Kind == OpReg16 || op.Kind == OpMem {
			subOp := byte(2) // CALL r/m16 = FF /2
			if mnem == "JMP" {
				subOp = 4 // JMP r/m16 = FF /4
			}
			mrm, err := a.encodeModRM(subOp, op, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0xFF}, mrm...), op), nil
		}
		// Direct relative target
		targetVal, _, err := a.EvalExpr(op.Expr, stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		if mnem == "CALL" {
			rel16 := uint16((targetVal - int64(stmt.PC+3)) & 0xFFFF)
			return []byte{0xE8, byte(rel16 & 0xFF), byte(rel16 >> 8)}, nil
		}
		// JMP direct: short (2 bytes) unless NearHint or BranchLong
		if !op.NearHint && (op.ShortHint || !stmt.BranchLong) {
			disp := targetVal - int64(stmt.PC+2)
			if a.pass == 2 && (disp < -128 || disp > 127) {
				return nil, fmt.Errorf("JMP SHORT out of range (%d)", disp)
			}
			return []byte{0xEB, byte(int8(disp))}, nil
		}
		rel16 := uint16((targetVal - int64(stmt.PC+3)) & 0xFFFF)
		return []byte{0xE9, byte(rel16 & 0xFF), byte(rel16 >> 8)}, nil
	}

	// Parse operands for 1- or 2-operand instructions
	ops := make([]ParsedOp, len(opsRaw))
	for i, r := range opsRaw {
		p, err := parseOperand8086(r)
		if err != nil {
			return nil, err
		}
		ops[i] = p
	}

	// 7. PUSH / POP
	if mnem == "PUSH" || mnem == "POP" {
		if len(ops) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		op := ops[0]
		switch op.Kind {
		case OpReg16:
			if mnem == "PUSH" {
				return []byte{0x50 + op.Reg}, nil
			}
			return []byte{0x58 + op.Reg}, nil
		case OpSegReg:
			if mnem == "PUSH" {
				return []byte{0x06 | (op.Reg << 3)}, nil
			}
			if op.Reg == 1 {
				return nil, fmt.Errorf("POP CS is not a valid 8086 instruction")
			}
			return []byte{0x07 | (op.Reg << 3)}, nil
		case OpMem:
			if mnem == "PUSH" {
				mrm, err := a.encodeModRM(6, op, stmt.PC)
				if err != nil {
					return nil, err
				}
				return prependSegOvr(append([]byte{0xFF}, mrm...), op), nil
			}
			mrm, err := a.encodeModRM(0, op, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x8F}, mrm...), op), nil
		default:
			return nil, fmt.Errorf("unsupported operand for %s: %q", mnem, stmt.RawOps)
		}
	}

	// 8. INC / DEC
	if mnem == "INC" || mnem == "DEC" {
		if len(ops) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		op := ops[0]
		subOp := byte(0)
		if mnem == "DEC" {
			subOp = 1
		}
		switch op.Kind {
		case OpReg16:
			if mnem == "INC" {
				return []byte{0x40 + op.Reg}, nil
			}
			return []byte{0x48 + op.Reg}, nil
		case OpReg8:
			mrm, _ := a.encodeModRM(subOp, op, stmt.PC)
			return append([]byte{0xFE}, mrm...), nil
		case OpMem:
			opc := byte(0xFF)
			if op.SizeHint == 1 {
				opc = 0xFE
			}
			mrm, err := a.encodeModRM(subOp, op, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{opc}, mrm...), op), nil
		default:
			return nil, fmt.Errorf("invalid operand for %s", mnem)
		}
	}

	// 9. Group 3 Unary & Multiply/Divide: NOT, NEG, MUL, IMUL, DIV, IDIV
	if subOp, ok := grp3Table[mnem]; ok {
		if len(ops) != 1 {
			return nil, fmt.Errorf("%s expects 1 operand", mnem)
		}
		op := ops[0]
		opc := byte(0xF7)
		if op.Kind == OpReg8 || op.SizeHint == 1 {
			opc = 0xF6
		}
		mrm, err := a.encodeModRM(subOp, op, stmt.PC)
		if err != nil {
			return nil, err
		}
		return prependSegOvr(append([]byte{opc}, mrm...), op), nil
	}

	// All remaining instructions expect 2 operands
	if len(ops) != 2 {
		return nil, fmt.Errorf("%s expects 2 operands, got %d", mnem, len(ops))
	}
	dst, src := ops[0], ops[1]

	// 10. MOV
	if mnem == "MOV" {
		// Segment register moves
		if dst.Kind == OpSegReg {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x8E}, mrm...), src), nil
		}
		if src.Kind == OpSegReg {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x8C}, mrm...), dst), nil
		}
		// Short direct memory moves with AL/AX: A0..A3
		if dst.Kind == OpReg8 && dst.Reg == 0 && src.Kind == OpMem && !src.HasBaseIdx {
			val, _, err := a.EvalExpr(src.DispExpr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			uval := uint16(val & 0xFFFF)
			return prependSegOvr([]byte{0xA0, byte(uval & 0xFF), byte(uval >> 8)}, src), nil
		}
		if dst.Kind == OpReg16 && dst.Reg == 0 && src.Kind == OpMem && !src.HasBaseIdx {
			val, _, err := a.EvalExpr(src.DispExpr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			uval := uint16(val & 0xFFFF)
			return prependSegOvr([]byte{0xA1, byte(uval & 0xFF), byte(uval >> 8)}, src), nil
		}
		if dst.Kind == OpMem && !dst.HasBaseIdx && src.Kind == OpReg8 && src.Reg == 0 {
			val, _, err := a.EvalExpr(dst.DispExpr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			uval := uint16(val & 0xFFFF)
			return prependSegOvr([]byte{0xA2, byte(uval & 0xFF), byte(uval >> 8)}, dst), nil
		}
		if dst.Kind == OpMem && !dst.HasBaseIdx && src.Kind == OpReg16 && src.Reg == 0 {
			val, _, err := a.EvalExpr(dst.DispExpr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			uval := uint16(val & 0xFFFF)
			return prependSegOvr([]byte{0xA3, byte(uval & 0xFF), byte(uval >> 8)}, dst), nil
		}
		// Register immediate: B0+r ib / B8+r iw
		if dst.Kind == OpReg8 && src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			return []byte{0xB0 + dst.Reg, byte(val & 0xFF)}, nil
		}
		if dst.Kind == OpReg16 && src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			uval := uint16(val & 0xFFFF)
			return []byte{0xB8 + dst.Reg, byte(uval & 0xFF), byte(uval >> 8)}, nil
		}
		// Reg <- R/M
		if dst.Kind == OpReg8 && (src.Kind == OpReg8 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x8A}, mrm...), src), nil
		}
		if dst.Kind == OpReg16 && (src.Kind == OpReg16 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x8B}, mrm...), src), nil
		}
		// R/M <- Reg
		if dst.Kind == OpMem && src.Kind == OpReg8 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x88}, mrm...), dst), nil
		}
		if dst.Kind == OpMem && src.Kind == OpReg16 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x89}, mrm...), dst), nil
		}
		// Mem <- Imm (C6 /0 ib or C7 /0 iw)
		if dst.Kind == OpMem && src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			mrm, err := a.encodeModRM(0, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			if dst.SizeHint == 1 || src.SizeHint == 1 {
				out := append([]byte{0xC6}, mrm...)
				out = append(out, byte(val&0xFF))
				return prependSegOvr(out, dst), nil
			}
			uval := uint16(val & 0xFFFF)
			out := append([]byte{0xC7}, mrm...)
			out = append(out, byte(uval&0xFF), byte(uval>>8))
			return prependSegOvr(out, dst), nil
		}
		return nil, fmt.Errorf("invalid operands for MOV: %q", stmt.RawOps)
	}

	// 11. LEA reg16, mem
	if mnem == "LEA" {
		if dst.Kind != OpReg16 || src.Kind != OpMem {
			return nil, fmt.Errorf("LEA requires reg16, [mem]")
		}
		mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
		if err != nil {
			return nil, err
		}
		return prependSegOvr(append([]byte{0x8D}, mrm...), src), nil
	}

	// 12. XCHG
	if mnem == "XCHG" {
		if dst.Kind == OpReg16 && src.Kind == OpReg16 {
			if dst.Reg == 0 {
				return []byte{0x90 + src.Reg}, nil
			}
			if src.Reg == 0 {
				return []byte{0x90 + dst.Reg}, nil
			}
		}
		if dst.Kind == OpReg8 && (src.Kind == OpReg8 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x86}, mrm...), src), nil
		}
		if src.Kind == OpReg8 && dst.Kind == OpMem {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x86}, mrm...), dst), nil
		}
		if dst.Kind == OpReg16 && (src.Kind == OpReg16 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x87}, mrm...), src), nil
		}
		if src.Kind == OpReg16 && dst.Kind == OpMem {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x87}, mrm...), dst), nil
		}
		return nil, fmt.Errorf("invalid operands for XCHG: %q", stmt.RawOps)
	}

	// 13. ALU: ADD, OR, ADC, SBB, AND, SUB, XOR, CMP
	if aluIdx, ok := aluTable[mnem]; ok {
		base := aluIdx << 3
		// Reg8, R/M8
		if dst.Kind == OpReg8 && (src.Kind == OpReg8 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{base + 2}, mrm...), src), nil
		}
		// Mem8, Reg8
		if dst.Kind == OpMem && src.Kind == OpReg8 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{base + 0}, mrm...), dst), nil
		}
		// Reg16, R/M16
		if dst.Kind == OpReg16 && (src.Kind == OpReg16 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{base + 3}, mrm...), src), nil
		}
		// Mem16, Reg16
		if dst.Kind == OpMem && src.Kind == OpReg16 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{base + 1}, mrm...), dst), nil
		}
		// R/M, Imm
		if src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			// 8-bit target
			if dst.Kind == OpReg8 || dst.SizeHint == 1 || src.SizeHint == 1 {
				if dst.Kind == OpReg8 && dst.Reg == 0 {
					return []byte{base + 4, byte(val & 0xFF)}, nil
				}
				mrm, err := a.encodeModRM(aluIdx, dst, stmt.PC)
				if err != nil {
					return nil, err
				}
				out := append([]byte{0x80}, mrm...)
				out = append(out, byte(val&0xFF))
				return prependSegOvr(out, dst), nil
			}
			// 16-bit target
			// Sign-extended 8-bit immediate (0x83 /aluIdx ib) when pure literal in -128..127
			if isPureLiteral(src.Expr) && val >= -128 && val <= 127 {
				mrm, err := a.encodeModRM(aluIdx, dst, stmt.PC)
				if err != nil {
					return nil, err
				}
				out := append([]byte{0x83}, mrm...)
				out = append(out, byte(int8(val)))
				return prependSegOvr(out, dst), nil
			}
			uval := uint16(val & 0xFFFF)
			if dst.Kind == OpReg16 && dst.Reg == 0 {
				return []byte{base + 5, byte(uval & 0xFF), byte(uval >> 8)}, nil
			}
			mrm, err := a.encodeModRM(aluIdx, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			out := append([]byte{0x81}, mrm...)
			out = append(out, byte(uval&0xFF), byte(uval>>8))
			return prependSegOvr(out, dst), nil
		}
		return nil, fmt.Errorf("invalid operands for %s: %q", mnem, stmt.RawOps)
	}

	// 14. TEST
	if mnem == "TEST" {
		if dst.Kind == OpReg8 && (src.Kind == OpReg8 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x84}, mrm...), src), nil
		}
		if dst.Kind == OpMem && src.Kind == OpReg8 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x84}, mrm...), dst), nil
		}
		if dst.Kind == OpReg16 && (src.Kind == OpReg16 || src.Kind == OpMem) {
			mrm, err := a.encodeModRM(dst.Reg, src, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x85}, mrm...), src), nil
		}
		if dst.Kind == OpMem && src.Kind == OpReg16 {
			mrm, err := a.encodeModRM(src.Reg, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			return prependSegOvr(append([]byte{0x85}, mrm...), dst), nil
		}
		if src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			if dst.Kind == OpReg8 || dst.SizeHint == 1 || src.SizeHint == 1 {
				if dst.Kind == OpReg8 && dst.Reg == 0 {
					return []byte{0xA8, byte(val & 0xFF)}, nil
				}
				mrm, err := a.encodeModRM(0, dst, stmt.PC)
				if err != nil {
					return nil, err
				}
				out := append([]byte{0xF6}, mrm...)
				out = append(out, byte(val&0xFF))
				return prependSegOvr(out, dst), nil
			}
			uval := uint16(val & 0xFFFF)
			if dst.Kind == OpReg16 && dst.Reg == 0 {
				return []byte{0xA9, byte(uval & 0xFF), byte(uval >> 8)}, nil
			}
			mrm, err := a.encodeModRM(0, dst, stmt.PC)
			if err != nil {
				return nil, err
			}
			out := append([]byte{0xF7}, mrm...)
			out = append(out, byte(uval&0xFF), byte(uval>>8))
			return prependSegOvr(out, dst), nil
		}
		return nil, fmt.Errorf("invalid operands for TEST: %q", stmt.RawOps)
	}

	// 15. Shifts & Rotates: ROL, ROR, RCL, RCR, SHL, SAL, SHR, SAR
	if shiftIdx, ok := shiftTable[mnem]; ok {
		is8 := dst.Kind == OpReg8 || dst.SizeHint == 1
		mrm, err := a.encodeModRM(shiftIdx, dst, stmt.PC)
		if err != nil {
			return nil, err
		}
		if src.Kind == OpReg8 && src.Reg == 1 { // CL
			opc := byte(0xD3)
			if is8 {
				opc = 0xD2
			}
			return prependSegOvr(append([]byte{opc}, mrm...), dst), nil
		}
		if src.Kind == OpImm {
			val, _, err := a.EvalExpr(src.Expr, stmt.PC)
			if err != nil && a.pass == 2 {
				return nil, err
			}
			if val <= 0 {
				return nil, nil
			}
			opc := byte(0xD1)
			if is8 {
				opc = 0xD0
			}
			single := prependSegOvr(append([]byte{opc}, mrm...), dst)
			var out []byte
			for k := int64(0); k < val; k++ {
				out = append(out, single...)
			}
			return out, nil
		}
		return nil, fmt.Errorf("invalid shift count for %s: %q", mnem, stmt.RawOps)
	}

	// 16. IN / OUT
	if mnem == "IN" {
		if dst.Kind == OpReg8 && dst.Reg == 0 {
			if src.Kind == OpReg16 && src.Reg == 2 { // DX
				return []byte{0xEC}, nil
			}
			if src.Kind == OpImm {
				val, _, err := a.EvalExpr(src.Expr, stmt.PC)
				if err != nil && a.pass == 2 {
					return nil, err
				}
				return []byte{0xE4, byte(val & 0xFF)}, nil
			}
		}
		if dst.Kind == OpReg16 && dst.Reg == 0 {
			if src.Kind == OpReg16 && src.Reg == 2 { // DX
				return []byte{0xED}, nil
			}
			if src.Kind == OpImm {
				val, _, err := a.EvalExpr(src.Expr, stmt.PC)
				if err != nil && a.pass == 2 {
					return nil, err
				}
				return []byte{0xE5, byte(val & 0xFF)}, nil
			}
		}
		return nil, fmt.Errorf("invalid operands for IN: %q", stmt.RawOps)
	}
	if mnem == "OUT" {
		if src.Kind == OpReg8 && src.Reg == 0 {
			if dst.Kind == OpReg16 && dst.Reg == 2 { // DX
				return []byte{0xEE}, nil
			}
			if dst.Kind == OpImm {
				val, _, err := a.EvalExpr(dst.Expr, stmt.PC)
				if err != nil && a.pass == 2 {
					return nil, err
				}
				return []byte{0xE6, byte(val & 0xFF)}, nil
			}
		}
		if src.Kind == OpReg16 && src.Reg == 0 {
			if dst.Kind == OpReg16 && dst.Reg == 2 { // DX
				return []byte{0xEF}, nil
			}
			if dst.Kind == OpImm {
				val, _, err := a.EvalExpr(dst.Expr, stmt.PC)
				if err != nil && a.pass == 2 {
					return nil, err
				}
				return []byte{0xE7, byte(val & 0xFF)}, nil
			}
		}
		return nil, fmt.Errorf("invalid operands for OUT: %q", stmt.RawOps)
	}

	return nil, fmt.Errorf("unknown or unsupported 8086 instruction: %s %s", mnem, stmt.RawOps)
}

// ── Multi-Pass Assembly & Branch Relaxation ─────────────────────────────────

func (a *Assembler) encodeDataBytes(stmt *Statement) ([]byte, error) {
	if stmt.Mnemonic == "ASCIZ" {
		s := strings.TrimSpace(stmt.RawOps)
		str, err := strconv.Unquote(s)
		if err != nil {
			return nil, fmt.Errorf("invalid ASCIZ string %q: %v", s, err)
		}
		out := append([]byte(str), 0)
		return out, nil
	}

	parts := splitTopLevelCommas(stmt.RawOps)
	var out []byte
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
			str, err := strconv.Unquote(p)
			if err != nil {
				str = p[1 : len(p)-1]
			}
			out = append(out, []byte(str)...)
			continue
		}
		if len(p) > 3 && p[0] == '\'' && p[len(p)-1] == '\'' {
			out = append(out, []byte(p[1:len(p)-1])...)
			continue
		}
		val, _, err := a.EvalExpr(p, stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		out = append(out, byte(val&0xFF))
	}
	return out, nil
}

func (a *Assembler) encodeDataWords(stmt *Statement) ([]byte, error) {
	parts := splitTopLevelCommas(stmt.RawOps)
	var out []byte
	for _, p := range parts {
		val, _, err := a.EvalExpr(p, stmt.PC)
		if err != nil && a.pass == 2 {
			return nil, err
		}
		uval := uint16(val & 0xFFFF)
		out = append(out, byte(uval&0xFF), byte(uval>>8))
	}
	return out, nil
}

func (a *Assembler) Assemble() error {
	a.pass = 1
	maxPasses := 20

	for iter := 0; iter < maxPasses; iter++ {
		changed := false
		a.activeSeg = SegCode
		a.segPC[SegCode] = 0x0200
		a.segPC[SegData] = 0x0200

		for _, stmt := range a.statements {
			if stmt.Type == StmtSegment {
				a.activeSeg = stmt.TargetSeg
				stmt.Seg = a.activeSeg
				stmt.PC = a.segPC[a.activeSeg]
				continue
			}

			stmt.Seg = a.activeSeg
			currPC := a.segPC[a.activeSeg]

			if stmt.Type == StmtOrg {
				val, _, err := a.EvalExpr(stmt.RawOps, currPC)
				if err != nil {
					return fmt.Errorf("%s:%d: ORG error: %v", stmt.File, stmt.LineNum, err)
				}
				currPC = uint32(val) & 0xFFFF
				a.segPC[a.activeSeg] = currPC
				stmt.PC = currPC
				if stmt.Label != "" {
					if old, ok := a.symbols[stmt.Label]; !ok || old != int64(currPC) {
						a.symbols[stmt.Label] = int64(currPC)
						changed = true
					}
				}
				continue
			}

			stmt.PC = currPC

			if stmt.Label != "" {
				if stmt.Type == StmtEqu {
					val, known, err := a.EvalExpr(stmt.RawOps, currPC)
					if err == nil && known {
						if old, ok := a.symbols[stmt.Label]; !ok || old != val {
							a.symbols[stmt.Label] = val
							changed = true
						}
					}
					continue
				}
				if old, ok := a.symbols[stmt.Label]; !ok || old != int64(currPC) {
					a.symbols[stmt.Label] = int64(currPC)
					changed = true
				}
			}

			oldSize := stmt.Size
			switch stmt.Type {
			case StmtComment, StmtEqu:
				stmt.Size = 0
			case StmtEnd:
				stmt.Size = 0
				if strings.TrimSpace(stmt.RawOps) != "" {
					if val, known, err := a.EvalExpr(stmt.RawOps, currPC); err == nil && known {
						a.entryPoint = uint32(val) & 0xFFFF
						a.hasEntry = true
					}
				}
			case StmtDataByte:
				b, err := a.encodeDataBytes(stmt)
				if err != nil {
					return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
				}
				stmt.Size = len(b)
			case StmtDataWord:
				parts := splitTopLevelCommas(stmt.RawOps)
				stmt.Size = len(parts) * 2
			case StmtReserve:
				parts := splitTopLevelCommas(stmt.RawOps)
				val, _, err := a.EvalExpr(parts[0], currPC)
				if err != nil {
					return fmt.Errorf("%s:%d: reserve error: %v", stmt.File, stmt.LineNum, err)
				}
				stmt.Size = int(val)
			case StmtAlign:
				val, _, err := a.EvalExpr(stmt.RawOps, currPC)
				if err != nil || val <= 0 {
					return fmt.Errorf("%s:%d: invalid ALIGN %q", stmt.File, stmt.LineNum, stmt.RawOps)
				}
				align := uint32(val)
				rem := currPC % align
				if rem != 0 {
					stmt.Size = int(align - rem)
				} else {
					stmt.Size = 0
				}
			case StmtInstruction:
				if _, isJcc := jccOpcodes[stmt.Mnemonic]; isJcc {
					if a.relaxEnabled && !stmt.BranchLong {
						targetVal, known, err := a.EvalExpr(stmt.RawOps, currPC)
						if err == nil && known {
							disp := targetVal - int64(currPC+2)
							if disp < -128 || disp > 127 {
								stmt.BranchLong = true
								changed = true
							}
						}
					}
					if stmt.BranchLong {
						stmt.Size = 5
					} else {
						stmt.Size = 2
					}
				} else if stmt.Mnemonic == "JMP" {
					op, err := parseOperand8086(stmt.RawOps)
					if err != nil {
						return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
					}
					if op.Kind == OpImm && !strings.Contains(stmt.RawOps, ":") {
						if op.NearHint {
							stmt.BranchLong = true
						} else if !op.ShortHint && a.relaxEnabled && !stmt.BranchLong {
							targetVal, known, err := a.EvalExpr(op.Expr, currPC)
							if err == nil && known {
								disp := targetVal - int64(currPC+2)
								if disp < -128 || disp > 127 {
									stmt.BranchLong = true
									changed = true
								}
							} else if iter > 0 && !known {
								// Forward reference that wasn't resolved yet: keep checking, or relax if far
							}
						}
						if stmt.BranchLong {
							stmt.Size = 3
						} else {
							stmt.Size = 2
						}
					} else {
						enc, err := a.encodeInstruction(stmt)
						if err != nil {
							return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
						}
						stmt.Size = len(enc)
					}
				} else {
					enc, err := a.encodeInstruction(stmt)
					if err != nil {
						return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
					}
					stmt.Size = len(enc)
				}
			}

			if stmt.Size != oldSize {
				changed = true
			}
			a.segPC[a.activeSeg] = currPC + uint32(stmt.Size)
		}

		if !changed && iter >= 1 {
			break
		}
	}

	// Pass 2: Final encoding
	a.pass = 2
	for _, stmt := range a.statements {
		if stmt.SrcLine != nil && stmt.Type != StmtComment && stmt.Type != StmtSegment {
			stmt.SrcLine.Seg = stmt.Seg
			stmt.SrcLine.PC = stmt.PC
			stmt.SrcLine.HasPC = true
		}

		switch stmt.Type {
		case StmtEqu:
			val, _, err := a.EvalExpr(stmt.RawOps, stmt.PC)
			if err != nil {
				return fmt.Errorf("%s:%d: EQU error: %v", stmt.File, stmt.LineNum, err)
			}
			a.symbols[stmt.Label] = val
		case StmtEnd:
			if strings.TrimSpace(stmt.RawOps) != "" {
				val, _, err := a.EvalExpr(stmt.RawOps, stmt.PC)
				if err != nil {
					return fmt.Errorf("%s:%d: END error: %v", stmt.File, stmt.LineNum, err)
				}
				a.entryPoint = uint32(val) & 0xFFFF
				a.hasEntry = true
			}
		case StmtDataByte:
			enc, err := a.encodeDataBytes(stmt)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Encoded = enc
		case StmtDataWord:
			enc, err := a.encodeDataWords(stmt)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Encoded = enc
		case StmtReserve:
			parts := splitTopLevelCommas(stmt.RawOps)
			fillByte := byte(0)
			if len(parts) > 1 {
				fv, _, err := a.EvalExpr(parts[1], stmt.PC)
				if err != nil {
					return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
				}
				fillByte = byte(fv & 0xFF)
			}
			stmt.Encoded = make([]byte, stmt.Size)
			if fillByte != 0 {
				for i := range stmt.Encoded {
					stmt.Encoded[i] = fillByte
				}
			}
		case StmtAlign:
			stmt.Encoded = make([]byte, stmt.Size)
			if stmt.Seg == SegCode {
				for i := range stmt.Encoded {
					stmt.Encoded[i] = 0x90 // NOP in code segment
				}
			}
		case StmtInstruction:
			enc, err := a.encodeInstruction(stmt)
			if err != nil {
				return fmt.Errorf("%s:%d: %v", stmt.File, stmt.LineNum, err)
			}
			stmt.Encoded = enc
		}

		if stmt.SrcLine != nil && len(stmt.Encoded) > 0 {
			stmt.SrcLine.Encoded = append(stmt.SrcLine.Encoded, stmt.Encoded...)
		}
	}

	return nil
}

// ── Extended DECB & Listing Output ───────────────────────────────────────────

type SegChunk struct {
	Addr uint32
	Data []byte
}

func (a *Assembler) collectSegmentChunks(seg SegmentID) []SegChunk {
	var chunks []SegChunk
	var cur *SegChunk

	for _, stmt := range a.statements {
		if stmt.Seg != seg {
			continue
		}
		if stmt.Type == StmtOrg {
			if cur != nil && stmt.PC != cur.Addr+uint32(len(cur.Data)) {
				cur = nil
			}
			continue
		}
		if len(stmt.Encoded) > 0 {
			if cur == nil || stmt.PC != cur.Addr+uint32(len(cur.Data)) {
				chunks = append(chunks, SegChunk{
					Addr: stmt.PC,
					Data: append([]byte(nil), stmt.Encoded...),
				})
				cur = &chunks[len(chunks)-1]
			} else {
				cur.Data = append(cur.Data, stmt.Encoded...)
			}
		}
	}
	return chunks
}

func writeDECBChunks(w io.Writer, high16 uint16, chunks []SegChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	// Emit Tag 254 (0xFE) SET_HIGH16_ADDR32 record
	highHdr := []byte{0xFE, 0x00, 0x00, byte(high16 >> 8), byte(high16 & 0xFF)}
	if _, err := w.Write(highHdr); err != nil {
		return err
	}

	for _, c := range chunks {
		data := c.Data
		addr := c.Addr
		for len(data) > 0 {
			toWrite := len(data)
			if toWrite > 65535 {
				toWrite = 65535
			}
			hdr := []byte{
				0x00,
				byte((toWrite >> 8) & 0xFF),
				byte(toWrite & 0xFF),
				byte((addr >> 8) & 0xFF),
				byte(addr & 0xFF),
			}
			if _, err := w.Write(hdr); err != nil {
				return err
			}
			if _, err := w.Write(data[:toWrite]); err != nil {
				return err
			}
			data = data[toWrite:]
			addr += uint32(toWrite)
		}
	}
	return nil
}

// EmitDECB writes a multi-segment Extended DECB binary with magic 'x', '0' (Intel 8086).
func (a *Assembler) EmitDECB(w io.Writer) error {
	// 1. Architecture Magic Record (Tag 253 = 0xFD): 'x', '0' (Intel 8086)
	magicHdr := []byte{0xFD, 0x00, 0x00, 'x', '0'}
	if _, err := w.Write(magicHdr); err != nil {
		return err
	}

	curHigh16 := uint16(0xFFFF)

	// 2. Code Segment (High16 = $0001 -> $10000..$1FFFF, CS = $1000)
	codeChunks := a.collectSegmentChunks(SegCode)
	if len(codeChunks) > 0 {
		if err := writeDECBChunks(w, 0x0001, codeChunks); err != nil {
			return err
		}
		curHigh16 = 0x0001
	}

	// 3. Data Segment (High16 = $0000 -> $00000..$0FFFF, DS = $0000)
	dataChunks := a.collectSegmentChunks(SegData)
	if len(dataChunks) > 0 {
		if err := writeDECBChunks(w, 0x0000, dataChunks); err != nil {
			return err
		}
		curHigh16 = 0x0000
	}

	// 4. Execution Postamble (Tag 255 = 0xFF): Entry IP offset within CS = $1000 (High16 = $0001)
	entryHigh16 := uint16(0x0001)
	if curHigh16 != entryHigh16 {
		highHdr := []byte{
			0xFE,
			0x00,
			0x00,
			byte(entryHigh16 >> 8),
			byte(entryHigh16 & 0xFF),
		}
		if _, err := w.Write(highHdr); err != nil {
			return err
		}
	}
	entry := a.entryPoint
	if !a.hasEntry {
		if sym, ok := a.symbols["cstart"]; ok {
			entry = uint32(sym) & 0xFFFF
		} else if sym, ok := a.symbols["_start"]; ok {
			entry = uint32(sym) & 0xFFFF
		} else if sym, ok := a.symbols["start"]; ok {
			entry = uint32(sym) & 0xFFFF
		} else if len(codeChunks) > 0 {
			entry = codeChunks[0].Addr & 0xFFFF
		} else {
			entry = 0x0200
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
			segChar := 'C'
			if line.Seg == SegData {
				segChar = 'D'
			}
			fmt.Fprintf(writer, "%c:%04X  %-18s %5d  %s\n", segChar, line.PC, hexStr, line.LineNum, line.Raw)
		} else {
			fmt.Fprintf(writer, "        %-18s %5d  %s\n", hexStr, line.LineNum, line.Raw)
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
	relaxFlag := flag.Bool("relax", true, "Enable Jcc and JMP branch relaxation")
	noRelaxFlag := flag.Bool("no-relax", false, "Disable branch relaxation")

	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: asm8086 -o <output.decb> [-l <listing.list>] [-format=decb|raw] <source.asm>...\n")
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
	default:
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
