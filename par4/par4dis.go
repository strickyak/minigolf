package par4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Disassembler translates a .p4p binary into human-readable Par4 assembly text.
type Disassembler struct {
	data           []byte
	stringPoolBase int
	stringPoolSize int
	globalsCount   int
	globalSizes    []int
	functions      []*FunctionMetadata
	entryFuncIdx   int
	codeBase       int
	codeSize       int
	labels         map[int]string // funcIdx:pc -> label
}

func NewDisassembler(data []byte) *Disassembler {
	return &Disassembler{
		data:   data,
		labels: make(map[int]string),
	}
}

// Disassemble converts .p4p bytecode into annotated Par4 assembly text.
func Disassemble(data []byte) (string, error) {
	d := NewDisassembler(data)
	return d.Disassemble()
}

func (d *Disassembler) Disassemble() (string, error) {
	if len(d.data) < HeaderSize {
		return "", fmt.Errorf("file too small for Par4 header (%d bytes)", len(d.data))
	}

	magic := d.data[0:4]
	if !bytes.Equal(magic, Magic) && !bytes.Equal(magic, []byte("P3P\x01")) && !bytes.Equal(magic, []byte("NPC\x01")) {
		return "", fmt.Errorf("invalid magic number: %q", string(magic))
	}

	formatVer := d.data[4]
	flags := d.data[5]
	d.stringPoolSize = int(binary.BigEndian.Uint16(d.data[6:8]))
	d.globalsCount = int(binary.BigEndian.Uint16(d.data[8:10]))
	funcCount := int(binary.BigEndian.Uint16(d.data[10:12]))
	d.entryFuncIdx = int(binary.BigEndian.Uint16(d.data[12:14]))
	d.codeSize = int(binary.BigEndian.Uint16(d.data[14:16]))

	var sb strings.Builder
	sb.WriteString("; ====================================================================\n")
	sb.WriteString(fmt.Sprintf("; Par4 Disassembly - FormatVer %d, Flags 0x%02X\n", formatVer, flags))
	sb.WriteString(fmt.Sprintf("; Functions: %d, Globals: %d, StringPool: %d bytes, CodeSize: %d bytes\n",
		funcCount, d.globalsCount, d.stringPoolSize, d.codeSize))
	sb.WriteString("; ====================================================================\n\n")

	offset := HeaderSize
	// Read Global Variable Table
	if offset+d.globalsCount > len(d.data) {
		return "", fmt.Errorf("truncated globals table")
	}
	d.globalSizes = make([]int, d.globalsCount)
	for i := 0; i < d.globalsCount; i++ {
		d.globalSizes[i] = int(d.data[offset+i])
		sb.WriteString(fmt.Sprintf(".global gvar_%d: %d\n", i, d.globalSizes[i]))
	}
	if d.globalsCount > 0 {
		sb.WriteString("\n")
	}
	offset += d.globalsCount

	// String Pool
	if offset+d.stringPoolSize > len(d.data) {
		return "", fmt.Errorf("truncated string pool")
	}
	d.stringPoolBase = offset
	offset += d.stringPoolSize

	// Function Table
	funcTableSize := funcCount * FuncEntrySz
	if offset+funcTableSize > len(d.data) {
		return "", fmt.Errorf("truncated function table")
	}
	funcBytes := d.data[offset : offset+funcTableSize]
	offset += funcTableSize

	// Local Variable Tables
	localVarsSize := len(d.data) - d.codeSize - offset
	if localVarsSize < 0 {
		return "", fmt.Errorf("corrupted local var tables offset")
	}
	localVarsBytes := d.data[offset : offset+localVarsSize]
	offset += localVarsSize

	// Bytecode Section
	if offset+d.codeSize > len(d.data) {
		return "", fmt.Errorf("truncated bytecode section")
	}
	codeBytes := d.data[offset : offset+d.codeSize]

	// Parse Function Metadata
	d.functions = make([]*FunctionMetadata, funcCount)
	for i := 0; i < funcCount; i++ {
		fb := funcBytes[i*FuncEntrySz : (i+1)*FuncEntrySz]
		argCount := int(fb[0])
		localCount := int(fb[1])
		frameSize := int(binary.BigEndian.Uint16(fb[2:4]))
		cOff := int(binary.BigEndian.Uint16(fb[4:6]))
		cSize := int(binary.BigEndian.Uint16(fb[6:8]))
		varOff := int(binary.BigEndian.Uint16(fb[8:10]))

		totalVars := argCount + localCount
		varSizes := make([]int, totalVars)
		for v := 0; v < totalVars; v++ {
			if varOff+v < len(localVarsBytes) {
				varSizes[v] = int(localVarsBytes[varOff+v])
			} else {
				varSizes[v] = 2
			}
		}

		name := fmt.Sprintf("fn_%d", i)
		if i == d.entryFuncIdx {
			name = "main"
		}
		d.functions[i] = &FunctionMetadata{
			Index:          i,
			Name:           name,
			IsEntry:        (i == d.entryFuncIdx),
			ArgCount:       argCount,
			LocalCount:     localCount,
			FrameSize:      frameSize,
			CodeOffset:     cOff,
			CodeSize:       cSize,
			VarTableOffset: varOff,
			VarSizes:       varSizes,
		}
	}

	// Scan functions to find all branch targets for labels
	for fnIdx, fn := range d.functions {
		fnCode := codeBytes[fn.CodeOffset : fn.CodeOffset+fn.CodeSize]
		pc := 0
		for pc < len(fnCode) {
			op := fnCode[pc]
			info := OpcodeTable[op]
			if info.ArgBytes == 2 && (op == OpJUMP || op == OpJUMP_IF_TRUE || op == OpJUMP_IF_FALSE) {
				rel := int(int16(binary.BigEndian.Uint16(fnCode[pc+1 : pc+3])))
				targetPC := (pc + 3) + rel
				key := fnIdx*65536 + targetPC
				if _, ok := d.labels[key]; !ok {
					d.labels[key] = fmt.Sprintf(".L_%s_%04X", fn.Name, targetPC)
				}
			}
			pc += 1 + info.ArgBytes
		}
	}

	// Disassemble each function
	for fnIdx, fn := range d.functions {
		sb.WriteString("; ====================================================================\n")
		sb.WriteString(fmt.Sprintf("; Function: %s (Index %d, FrameSize %d bytes)\n", fn.Name, fn.Index, fn.FrameSize))
		sb.WriteString("; ====================================================================\n")

		entryStr := ""
		if fn.IsEntry {
			entryStr = " entry"
		}
		sb.WriteString(fmt.Sprintf(".function %s%s\n", fn.Name, entryStr))

		// Params
		for p := 0; p < fn.ArgCount; p++ {
			sb.WriteString(fmt.Sprintf("    .param p_%d: %d\n", p, fn.VarSizes[p]))
		}
		// Locals
		for l := 0; l < fn.LocalCount; l++ {
			slot := fn.ArgCount + l
			sb.WriteString(fmt.Sprintf("    .local l_%d: %d\n", l, fn.VarSizes[slot]))
		}
		if fn.ArgCount > 0 || fn.LocalCount > 0 {
			sb.WriteString("\n")
		}

		// Disassemble instructions
		fnCode := codeBytes[fn.CodeOffset : fn.CodeOffset+fn.CodeSize]
		pc := 0
		for pc < len(fnCode) {
			key := fnIdx*65536 + pc
			if lbl, hasLbl := d.labels[key]; hasLbl {
				sb.WriteString(fmt.Sprintf("%s:\n", lbl))
			}

			op := fnCode[pc]
			info, exists := OpcodeTable[op]
			if !exists {
				sb.WriteString(fmt.Sprintf("    ; 0x%04X: UNKNOWN_OPCODE 0x%02X\n", pc, op))
				pc++
				continue
			}

			var arg uint16
			if info.ArgBytes == 1 {
				arg = uint16(fnCode[pc+1])
			} else if info.ArgBytes == 2 {
				arg = binary.BigEndian.Uint16(fnCode[pc+1 : pc+3])
			}

			// Format operand
			operandStr := d.formatOperand(fn, fnIdx, op, arg, pc)
			if operandStr != "" {
				sb.WriteString(fmt.Sprintf("    %-18s %s\n", info.Mnemonic, operandStr))
			} else {
				sb.WriteString(fmt.Sprintf("    %s\n", info.Mnemonic))
			}

			pc += 1 + info.ArgBytes
		}

		sb.WriteString(".endfunction\n\n")
	}

	return sb.String(), nil
}

func (d *Disassembler) formatOperand(fn *FunctionMetadata, fnIdx int, op byte, arg uint16, pc int) string {
	info := OpcodeTable[op]
	if info.ArgBytes == 0 {
		return ""
	}

	switch op {
	case OpPUSH_STR:
		// Decode string from string pool
		strOff := int(arg)
		if d.stringPoolBase+strOff+2 <= len(d.data) {
			strLen := int(binary.BigEndian.Uint16(d.data[d.stringPoolBase+strOff:]))
			strBytes := d.data[d.stringPoolBase+strOff+2 : d.stringPoolBase+strOff+2+strLen]
			return strconv.Quote(string(strBytes))
		}
		return fmt.Sprintf("str_off_%d", arg)

	case OpJUMP, OpJUMP_IF_TRUE, OpJUMP_IF_FALSE:
		rel := int(int16(arg))
		targetPC := (pc + 3) + rel
		key := fnIdx*65536 + targetPC
		if lbl, ok := d.labels[key]; ok {
			return lbl
		}
		return fmt.Sprintf(".L_%04X", targetPC)

	case OpCALL:
		fIdx := int(arg)
		if fIdx < len(d.functions) {
			return d.functions[fIdx].Name
		}
		return fmt.Sprintf("fn_%d", fIdx)

	case OpLOAD_LOCAL, OpSTORE_LOCAL, OpADDR_OF_LOCAL:
		slot := int(arg)
		if slot < fn.ArgCount {
			return fmt.Sprintf("p_%d", slot)
		}
		return fmt.Sprintf("l_%d", slot-fn.ArgCount)

	case OpLOAD_GLOBAL, OpSTORE_GLOBAL, OpADDR_OF_GLOBAL:
		return fmt.Sprintf("gvar_%d", arg)

	case OpHATVAN_TRAP:
		callNum := byte(arg)
		name := d.syscallName(callNum)
		return fmt.Sprintf("%s ; ($%02X)", name, callNum)

	case OpPUSH_I8:
		return fmt.Sprintf("%d", int8(byte(arg)))

	case OpPUSH_U8:
		return fmt.Sprintf("%d", arg)

	case OpPUSH_I16:
		if arg >= 0x8000 {
			return fmt.Sprintf("%d ; ($%04X)", int16(arg), arg)
		}
		return fmt.Sprintf("%d", arg)

	default:
		return fmt.Sprintf("%d", arg)
	}
}

func (d *Disassembler) syscallName(callNum byte) string {
	switch callNum {
	case SysOpen:
		return "I$Open"
	case SysCreate:
		return "I$Create"
	case SysRead:
		return "I$Read"
	case SysWrite:
		return "I$Write"
	case SysReadLn:
		return "I$ReadLn"
	case SysWritLn:
		return "I$WritLn"
	case SysClose:
		return "I$Close"
	case SysExit:
		return "F$Exit"
	case SysSeek:
		return "I$Seek"
	case SysDelete:
		return "I$Delete"
	case SysAttach:
		return "I$Attach"
	case SysDetach:
		return "I$Detach"
	case SysDup:
		return "I$Dup"
	case SysFork:
		return "F$Fork"
	case SysWait:
		return "F$Wait"
	case SysMem:
		return "F$Mem"
	case SysSleep:
		return "F$Sleep"
	case SysTime:
		return "F$Time"
	default:
		return fmt.Sprintf("TRAP_0x%02X", callNum)
	}
}
