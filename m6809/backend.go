package m6809

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/opt"
)

var GLOBAL_VAR_OFFSET = flag.Int("global_var_offset", 16, "must be positive, so address 0 is not used, that is nil")

func align(sz int) int {
	if sz == 0 {
		return 1
	}
	return sz
}

func (b *Backend) getTypeSizeUsingIrt(irt *ir.Type) int {
	return b.getTypeSizeUsingIrt9(irt)
}

func (b *Backend) getTypeSizeUsingIrt9(irt *ir.Type) int {
	if irt.IsAPointer() {
		return 2
	}
	if irt.IsAnArray() {
		et := irt.ArrayElementType()
		length := irt.ArrayLength()
		return length * b.getTypeSizeByType(et)
	}
	if irt.IsAStruct() {
		fields := irt.FieldsOfStruct()
		if len(fields) > 0 {
			size := 0
			for _, f := range fields {
				size += b.getTypeSizeByType(f.Type)
			}
			return size
		}

		content := irt.Name[7 : len(irt.Name)-1]
		if strings.HasPrefix(irt.Name, "tuple_") {
			content = irt.Name[6 : len(irt.Name)-1]
		}
		size := 0
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				size += b.getTypeSize(content[start:i], nil)
				start = i + 1
			}
		}
		return size
	}

	switch irt.Name {
	case "void", "byte", "bool":
		return 1
	case "word", "int", "const_integer", "uint", "noreturn":
		return 2
	}
	if irt.IsAFuncPtr() || irt.Name == "func" {
		return 2
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[irt.Name]; ok {
			return b.getTypeSizeByType(def)
		}
	}
	log.Panicf("M6809 getTypeSizeUsingIrt: unknown case: %#v", *irt)
	panic(0)
}

func (b *Backend) getTypeSizeByType(irt ir.Type) int {
	return b.getTypeSize(irt.Name, &irt)
}

func (b *Backend) getTypeSize(typ string, irt *ir.Type) int {
	if irt != nil {
		return b.getTypeSizeUsingIrt(irt)
	}
	log.Panicf("getTypeSize: %q, %#v", typ, irt)
	panic("NOT REACHED")
}

func (b *Backend) getEltSizeUsingIrt(irt ir.Type) int {
	switch {
	case irt.IsAPointer():
		pt := irt.PointedType()
		if pt.IsAnArray() {
			et := pt.ArrayElementType()
			return b.getTypeSizeUsingIrt(&et)
		}
		log.Panicf("getEltSizeUsingIrt: called on pointer that does not point to an array: irt=%#v", irt)
	case irt.IsAnArray():
		et := irt.ArrayElementType()
		return b.getTypeSizeUsingIrt(&et)
	default:
		log.Panicf("M6809 getEltSizeUsingIrt: unknown case: %#v", irt)
	}
	panic("NOT REACHED")
}

func (b *Backend) getFieldOffsetAndSize(structTyp ir.Type, fieldIndex int) (int, int) {
	fields := structTyp.FieldsOfStruct()
	if len(fields) == 0 {
		if def, ok := b.program.TypeDefs[structTyp.Name]; ok {
			fields = def.FieldsOfStruct()
		}
	}
	if len(fields) == 0 {
		log.Panicf("getFieldOffsetAndSize: not a struct or no fields: %q", structTyp.Name)
	}

	byteOffset := 0
	for i, f := range fields {
		sz := b.getTypeSizeByType(f.Type)
		if i < fieldIndex {
			byteOffset += sz
		} else if i == fieldIndex {
			return byteOffset, sz
		}
	}
	log.Panicf("getFieldOffsetAndSize: field not found: %q . %d", structTyp.Name, fieldIndex)
	panic(0)
}

type Backend struct {
	program           *ir.Program
	useFramePointer   bool
	globalsAtY        bool
	picMode           bool
	frameOffset       int
	buf               bytes.Buffer
	dataBuf           bytes.Buffer
	rodataBuf         bytes.Buffer
	helpersBuf        bytes.Buffer
	helpersEmitted    map[string]bool
	stackSize         int
	pushedBytes       int
	slots             map[int]int    // SSA value ID -> byte offset in local frame (0 <= offset < stackSize)
	slotSizes         map[int]int    // SSA value ID -> size in bytes
	paramOffsets      map[string]int // param name -> byte offset in arguments block (0 for arg0)
	jmpSlots          map[int]int    // setjmp slot -> byte offset in local frame
	globalOffsets     map[string]int // global name -> offset from Y (when globalsAtY is true)
	fmtCount          int
	lblCount          int
	retSlot           int // byte offset in arguments block where return buffer is located (if retSize > 2)
	f                 *ir.Function
	fusedCompares     map[int]bool
	addressTaken      map[int]bool
	escapeRes         opt.EscapeAnalysisResult
	needsFP           bool
	uses              map[int]int
	localAddressTaken map[int]bool

	curInstr ir.Instruction
	valInD   ir.Value
	valInB   ir.Value
	instrs   map[int]ir.Instruction

	NoBranchLayout    bool
	NoFusedCompares   bool
	NoLeafOpt         bool
	NoSlotSharing     bool
	NoLocalRegAlloc   bool
	NoCSSALowering    bool
	NoGlobalRegAlloc  bool
	globalRegs        map[int]string
	cssaScratchOffset int
	calleeSaveRegs    []string
	savedRegBytes     int
	saveYFP           bool

	// Tunable optimization and code-generation thresholds (time vs space)
	InlineMul16           bool // Inline 16-bit multiplication instead of calling __mul16 helper
	InlineDivMod16        bool // Inline 16-bit division/modulus instead of calling __divmod16 helper
	MemcpyUnrollThreshold int  // Max bytes to copy inline before calling __memcpy or looping (default: 4)
	MemsetUnrollThreshold int  // Max bytes to zero inline before calling __memset0 or looping (default: 2)
	ShiftUnrollThreshold  int  // Max bit count to shift inline before looping (default: 4)
}

func New(useFramePointer bool, globalsAtY bool, picMode bool) *Backend {
	frameOff := 0
	if useFramePointer {
		frameOff = 2
	}
	b := &Backend{
		useFramePointer:       useFramePointer,
		globalsAtY:            globalsAtY,
		picMode:               picMode,
		frameOffset:           frameOff,
		slots:                 make(map[int]int),
		slotSizes:             make(map[int]int),
		paramOffsets:          make(map[string]int),
		jmpSlots:              make(map[int]int),
		globalOffsets:         make(map[string]int),
		helpersEmitted:        make(map[string]bool),
		fusedCompares:         make(map[int]bool),
		NoBranchLayout:        os.Getenv("NO_BRANCH_LAYOUT6809") != "",
		NoFusedCompares:       os.Getenv("NO_FUSED_COMPARES6809") != "",
		NoLeafOpt:             os.Getenv("NO_LEAF_OPT6809") != "",
		NoSlotSharing:         os.Getenv("NO_SLOT_SHARING6809") != "",
		NoLocalRegAlloc:       os.Getenv("NO_LOCAL_REGALLOC6809") != "",
		NoCSSALowering:        os.Getenv("NO_CSSA_LOWERING6809") != "",
		NoGlobalRegAlloc:      os.Getenv("NO_GLOBAL_REGALLOC6809") != "",
		cssaScratchOffset:     -1,
		InlineMul16:           os.Getenv("INLINE_MUL16") != "",
		InlineDivMod16:        os.Getenv("INLINE_DIVMOD16") != "",
		MemcpyUnrollThreshold: 4,
		MemsetUnrollThreshold: 2,
		ShiftUnrollThreshold:  4,
	}
	if v := os.Getenv("MEMCPY_UNROLL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			b.MemcpyUnrollThreshold = n
		}
	}
	if v := os.Getenv("MEMSET_UNROLL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			b.MemsetUnrollThreshold = n
		}
	}
	if v := os.Getenv("SHIFT_UNROLL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			b.ShiftUnrollThreshold = n
		}
	}
	return b
}

func (b *Backend) canTrack(val ir.Value) bool {
	if b.NoLocalRegAlloc || val == nil {
		return false
	}
	if inst, ok := val.(ir.Instruction); ok {
		if b.addressTaken != nil && b.addressTaken[inst.GetID()] {
			return false
		}
		return true
	}
	return false
}

func (b *Backend) clobberAllRegs() {
	b.valInD = nil
	b.valInB = nil
}

func (b *Backend) clobberD() {
	b.valInD = nil
	b.valInB = nil
}

func (b *Backend) clobberB() {
	b.valInB = nil
	b.valInD = nil
}

func (b *Backend) setD(val ir.Value) {
	if b.canTrack(val) {
		b.valInD = val
	} else {
		b.valInD = nil
	}
	b.valInB = nil
}

func (b *Backend) setB(val ir.Value) {
	if b.canTrack(val) {
		b.valInB = val
	} else {
		b.valInB = nil
	}
	b.valInD = nil
}

func (b *Backend) pushBytes(n int) {
	b.pushedBytes += n
}

func (b *Backend) popBytes(n int) {
	b.pushedBytes -= n
}

func (b *Backend) nextLabel() string {
	b.lblCount++
	return fmt.Sprintf(".LL%d", b.lblCount)
}

func (b *Backend) offsetAddr(off int, sz int) string {
	if b.useFramePointer {
		extra := 0
		if b.saveYFP {
			extra = 2
		}
		// In U frame: locals are allocated below saved U (and saved Y if present), so base address is -(extra + off + sz) from U
		return fmt.Sprintf("-%d,u", extra+off+sz)
	}
	// In S frame: locals start at S + pushedBytes + off
	return fmt.Sprintf("%d,s", off+b.pushedBytes)
}

func (b *Backend) cssaScratchAddr() string {
	if b.cssaScratchOffset < 0 {
		return ""
	}
	return b.offsetAddr(b.cssaScratchOffset, 2)
}

func (b *Backend) resolveSlot(id int) int {
	if b.NoSlotSharing {
		return id
	}
	if b.f != nil && b.f.SlotAlias != nil {
		for {
			if alias, ok := b.f.SlotAlias[id]; ok {
				id = alias
			} else {
				break
			}
		}
	}
	return id
}

func (b *Backend) localAddr(slotId int) string {
	canon := b.resolveSlot(slotId)
	off, ok := b.slots[canon]
	if !ok {
		off, ok = b.slots[slotId]
	}
	if !ok {
		log.Panicf("localAddr: slot not found for id %d (canon %d)", slotId, canon)
	}
	sz := b.slotSizes[canon]
	if sz == 0 {
		sz = b.slotSizes[slotId]
	}
	return b.offsetAddr(off, sz)
}

func (b *Backend) paramAddr(paramName string) string {
	off, ok := b.paramOffsets[paramName]
	if !ok {
		log.Panicf("paramAddr: param not found %q", paramName)
	}
	if b.needsFP {
		// With FP: 0,u=saved U, 2,u=return PC, 4,u=arg0
		return fmt.Sprintf("%d,u", 4+off)
	}
	// Without FP: stackSize bytes of locals + pushedBytes + savedRegBytes + 2 bytes return PC + off
	return fmt.Sprintf("%d,s", b.stackSize+b.pushedBytes+b.savedRegBytes+2+off)
}

func (b *Backend) retBufAddr() string {
	if b.needsFP {
		return fmt.Sprintf("%d,u", 4+b.retSlot)
	}
	return fmt.Sprintf("%d,s", b.stackSize+b.pushedBytes+b.savedRegBytes+2+b.retSlot)
}

func (b *Backend) jmpChainAddr() string {
	if b.globalsAtY {
		if off, ok := b.globalOffsets["prelude._jmp_chain_"]; ok {
			return fmt.Sprintf("%d,y", off)
		}
		return "16,y"
	}
	if b.picMode {
		return "v_prelude._jmp_chain_,pcr"
	}
	return "v_prelude._jmp_chain_"
}

func (b *Backend) panicAddr() string {
	if b.globalsAtY {
		if off, ok := b.globalOffsets["prelude._panic_"]; ok {
			return fmt.Sprintf("%d,y", off)
		}
		return "18,y"
	}
	if b.picMode {
		return "v_prelude._panic_,pcr"
	}
	return "v_prelude._panic_"
}

func (b *Backend) getAddrStr(val ir.Value) string {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.Parameter:
		return b.paramAddr(v.Name)
	case ir.Instruction:
		return b.localAddr(v.GetID())
	case *ir.Global:
		if b.globalsAtY {
			return fmt.Sprintf("%d,y", b.globalOffsets[v.Name])
		}
		if b.picMode {
			return fmt.Sprintf("v_%s,pcr", v.Name)
		}
		return fmt.Sprintf("v_%s", v.Name)
	default:
		log.Panicf("getAddrStr: unhandled type %T (%v)", val, val)
	}
	return ""
}

func cleanName(name string) string {
	if idx := strings.Index(name, "$"); idx != -1 {
		return name[:idx]
	}
	return name
}

func (b *Backend) getValName(id int) string {
	if b.instrs != nil {
		if instr, ok := b.instrs[id]; ok {
			if n := instr.GetName(); n != "" {
				return cleanName(n)
			}
		}
	}
	if b.f != nil && b.f.SlotAlias != nil {
		if canon, ok := b.f.SlotAlias[id]; ok && canon != id {
			if instr, ok := b.instrs[canon]; ok {
				if n := instr.GetName(); n != "" {
					return cleanName(n)
				}
			}
		}
	}
	return ""
}

func (b *Backend) describeSlot(id int) string {
	name := b.getValName(id)
	if name != "" {
		return fmt.Sprintf("'%s' (v%d)", name, id)
	}
	return fmt.Sprintf("v%d", id)
}

func (b *Backend) describeVal(val ir.Value) string {
	if val == nil {
		return "nil"
	}
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ConstByte:
		return fmt.Sprintf("#%d", v.Val)
	case *ir.ConstWord:
		return fmt.Sprintf("#%d", v.Val)
	case *ir.Parameter:
		return fmt.Sprintf("param '%s'", v.Name)
	case *ir.Global:
		return fmt.Sprintf("global '%s'", v.Name)
	case *ir.Sizeof:
		return fmt.Sprintf("sizeof(%s)", v.TargetTyp.Name)
	case *ir.AddressOfGlobal:
		return fmt.Sprintf("&global '%s'", v.Global.Name)
	case *ir.AddressOfLocal:
		if loc, ok := v.Local.(ir.Instruction); ok {
			return fmt.Sprintf("&local %s", b.describeSlot(loc.GetID()))
		}
		return "&local"
	case *ir.AddressOfFunc:
		return fmt.Sprintf("&func '%s'", v.Func.Name)
	case ir.Instruction:
		id := v.GetID()
		name := b.getValName(id)
		if len(b.globalRegs) > 0 {
			if reg, ok := b.globalRegs[id]; ok {
				if name != "" {
					return fmt.Sprintf("%s (%s)", strings.ToUpper(reg), name)
				}
				return fmt.Sprintf("%s (v%d)", strings.ToUpper(reg), id)
			}
		}
		if name != "" {
			return fmt.Sprintf("'%s' (v%d)", name, id)
		}
		return fmt.Sprintf("v%d", id)
	default:
		return val.String()
	}
}

func (b *Backend) emitFunctionHeader(f *ir.Function) {
	b.buf.WriteString("; " + strings.Repeat("=", 76) + "\n")
	var paramStrs []string
	for _, p := range f.Parameters {
		paramStrs = append(paramStrs, fmt.Sprintf("%s: %s", p.Name, p.Typ.Name))
	}
	retType := f.ReturnType.Name
	if retType == "" {
		retType = "void"
	}
	b.buf.WriteString(fmt.Sprintf("; func %s(%s) %s\n", f.Name, strings.Join(paramStrs, ", "), retType))
	b.buf.WriteString(fmt.Sprintf("; Frame: %d bytes locals", b.stackSize))
	if len(b.calleeSaveRegs) > 0 {
		b.buf.WriteString(fmt.Sprintf(", saved regs: [%s]", strings.Join(b.calleeSaveRegs, ", ")))
	}
	if b.needsFP {
		b.buf.WriteString(", frame pointer: U")
	}
	b.buf.WriteString("\n")

	if len(f.Parameters) > 0 {
		b.buf.WriteString("; Parameters:\n")
		for _, p := range f.Parameters {
			addr := b.paramAddr(p.Name)
			b.buf.WriteString(fmt.Sprintf(";   %-8s : %s (%s)\n", addr, p.Name, p.Typ.Name))
		}
	}

	type slotGroup struct {
		addr  string
		names []string
	}
	slotMap := make(map[int]*slotGroup)
	var slotOrder []int
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			canon := b.resolveSlot(id)
			if off, ok := b.slots[canon]; ok {
				sz := b.slotSizes[canon]
				if sz == 0 {
					sz = b.slotSizes[id]
				}
				addr := b.offsetAddr(off, sz)
				grp, exists := slotMap[off]
				if !exists {
					grp = &slotGroup{addr: addr}
					slotMap[off] = grp
					slotOrder = append(slotOrder, off)
				}
				name := b.describeSlot(id)
				found := false
				for _, n := range grp.names {
					if n == name {
						found = true
						break
					}
				}
				if !found {
					grp.names = append(grp.names, name)
				}
			}
		}
	}
	if len(slotOrder) > 0 {
		sort.Ints(slotOrder)
		b.buf.WriteString("; Locals:\n")
		for _, off := range slotOrder {
			grp := slotMap[off]
			b.buf.WriteString(fmt.Sprintf(";   %-8s : %s\n", grp.addr, strings.Join(grp.names, ", ")))
		}
	}

	if len(b.globalRegs) > 0 {
		b.buf.WriteString("; Register Allocations:\n")
		regGroups := make(map[string][]string)
		var regNames []string
		for id, reg := range b.globalRegs {
			regUpper := strings.ToUpper(reg)
			if _, exists := regGroups[regUpper]; !exists {
				regNames = append(regNames, regUpper)
			}
			desc := b.describeSlot(id)
			found := false
			for _, d := range regGroups[regUpper] {
				if d == desc {
					found = true
					break
				}
			}
			if !found {
				regGroups[regUpper] = append(regGroups[regUpper], desc)
			}
		}
		sort.Strings(regNames)
		for _, r := range regNames {
			b.buf.WriteString(fmt.Sprintf(";   %-8s : %s\n", r, strings.Join(regGroups[r], ", ")))
		}
	}
	b.buf.WriteString("; " + strings.Repeat("=", 76) + "\n")
}

func offsetAddrStr(valStr string, offset int) string {
	if offset == 0 {
		return valStr
	}
	if strings.HasPrefix(valStr, "$") && !strings.Contains(valStr, ",") {
		var hexVal uint32
		if _, err := fmt.Sscanf(valStr, "$%x", &hexVal); err == nil {
			return fmt.Sprintf("$%04X", uint16(int(hexVal)+offset))
		}
	}
	if strings.HasSuffix(valStr, ",pcr") {
		base := strings.TrimSuffix(valStr, ",pcr")
		return fmt.Sprintf("%s+%d,pcr", base, offset)
	}
	if idx := strings.Index(valStr, ","); idx != -1 {
		numPart := valStr[:idx]
		regPart := valStr[idx:]
		var baseNum int
		fmt.Sscanf(numPart, "%d", &baseNum)
		return fmt.Sprintf("%d%s", baseNum+offset, regPart)
	}
	return fmt.Sprintf("%s+%d", valStr, offset)
}

func (b *Backend) getDirectEA(ptrVal ir.Value) (string, bool) {
	ptrVal = b.resolveVal(ptrVal)
	switch v := ptrVal.(type) {
	case *ir.AddressOfLocal:
		if b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[v.GetID()] {
			if locInst, ok := v.Local.(ir.Instruction); ok && len(b.globalRegs) > 0 {
				if _, inReg := b.globalRegs[locInst.GetID()]; inReg {
					return "", false // Value is in a physical register, not memory!
				}
			}
			return b.getAddrStr(v.Local), true
		}
	case *ir.AddressOfGlobal:
		return b.getAddrStr(v.Global), true
	case *ir.AddressOfField:
		if baseEA, ok := b.getDirectEA(v.Ptr); ok {
			structType := v.Ptr.Type().PointedType()
			byteOffset, _ := b.getFieldOffsetAndSize(structType, v.FieldIndex)
			return offsetAddrStr(baseEA, byteOffset), true
		}
	default:
		if c, ok := b.asConstWord(ptrVal); ok {
			return fmt.Sprintf("$%04X", c), true
		}
	}
	return "", false
}

func (b *Backend) canDirectEA(val ir.Value, opSize int) bool {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ConstByte:
		return true
	case *ir.ConstWord:
		return opSize == 2 || (v.Val >= 0 && v.Val <= 255)
	case *ir.Sizeof:
		sz := b.getTypeSizeByType(v.TargetTyp)
		return opSize == 2 || (sz >= 0 && sz <= 255)
	case *ir.Parameter:
		return b.getValSize(v) == opSize
	case ir.Instruction:
		if len(b.globalRegs) > 0 {
			if _, ok := b.globalRegs[v.GetID()]; ok {
				return false
			}
		}
		canon := b.resolveSlot(v.GetID())
		if _, ok := b.slots[canon]; !ok {
			if _, ok := b.slots[v.GetID()]; !ok {
				return false
			}
		}
		return b.getValSize(v) == opSize
	case *ir.Global:
		return b.getValSize(v) == opSize
	default:
		return false
	}
}

func (b *Backend) getRightEA(val ir.Value, opSize int) string {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ConstByte:
		if opSize == 1 {
			return fmt.Sprintf("#%d", uint8(v.Val))
		}
		return fmt.Sprintf("#%d", uint16(v.Val))
	case *ir.ConstWord:
		if opSize == 1 {
			return fmt.Sprintf("#%d", uint8(v.Val))
		}
		return fmt.Sprintf("#%d", v.Val)
	case *ir.Sizeof:
		sz := b.getTypeSizeByType(v.TargetTyp)
		if opSize == 1 {
			return fmt.Sprintf("#%d", uint8(sz))
		}
		return fmt.Sprintf("#%d", sz)
	case *ir.Parameter, ir.Instruction, *ir.Global:
		return b.getAddrStr(v)
	default:
		log.Panicf("getRightEA: unhandled type %T (%v)", val, val)
		return ""
	}
}

func (b *Backend) asConstByte(val ir.Value) (byte, bool) {
	val = b.resolveVal(val)
	if c, ok := val.(*ir.ConstByte); ok {
		return c.Val, true
	}
	if c, ok := val.(*ir.ConstWord); ok && c.Val <= 255 {
		return byte(c.Val), true
	}
	if sz, ok := val.(*ir.Sizeof); ok {
		s := b.getTypeSizeByType(sz.TargetTyp)
		if s <= 255 {
			return byte(s), true
		}
	}
	return 0, false
}

func (b *Backend) asConstWord(val ir.Value) (uint16, bool) {
	val = b.resolveVal(val)
	if c, ok := val.(*ir.ConstWord); ok {
		return uint16(c.Val), true
	}
	if c, ok := val.(*ir.ConstByte); ok {
		return uint16(c.Val), true
	}
	if sz, ok := val.(*ir.Sizeof); ok {
		return uint16(b.getTypeSizeByType(sz.TargetTyp)), true
	}
	return 0, false
}

func visitOperands(instr ir.Instruction, visitor func(ir.Value)) {
	if instr == nil {
		return
	}
	switch i := instr.(type) {
	case *ir.Store:
		visitor(i.Val)
	case *ir.BinaryOp:
		visitor(i.Left)
		visitor(i.Right)
	case *ir.Compare:
		visitor(i.Left)
		visitor(i.Right)
	case *ir.UnaryOp:
		visitor(i.Operand)
	case *ir.ExtractElement:
		visitor(i.Array)
		visitor(i.Index)
	case *ir.InsertElement:
		visitor(i.Array)
		visitor(i.Index)
		visitor(i.Val)
	case *ir.ExtractField:
		visitor(i.Struct)
	case *ir.InsertField:
		visitor(i.Struct)
		visitor(i.Val)
	case *ir.AddressOfLocal:
		visitor(i.Local)
	case *ir.AddressOfField:
		visitor(i.Ptr)
	case *ir.AddressOfElement:
		visitor(i.ArrayPtr)
		visitor(i.Index)
	case *ir.ExtractFieldPtr:
		visitor(i.Ptr)
	case *ir.InsertFieldPtr:
		visitor(i.Ptr)
		visitor(i.Val)
	case *ir.LoadPtr:
		visitor(i.Ptr)
	case *ir.StorePtr:
		visitor(i.Ptr)
		visitor(i.Val)
	case *ir.Phi:
		for _, edge := range i.Edges {
			visitor(edge.Value)
		}
	case *ir.Call:
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.IndirectCall:
		visitor(i.FuncPtr)
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.BuiltinCall:
		for _, arg := range i.Args {
			visitor(arg)
		}
	case *ir.Cast:
		visitor(i.Operand)
	case *ir.Branch:
		visitor(i.Condition)
	case *ir.Return:
		if i.Val != nil {
			visitor(i.Val)
		}
	case *ir.SetJmp:
		visitor(i.JmpBuf)
	case *ir.LongJmp:
		visitor(i.JmpBuf)
	case *ir.ConstArray:
		for _, el := range i.Elements {
			visitor(el)
		}
	case *ir.ConstStruct:
		for _, el := range i.Fields {
			visitor(el)
		}
	}
}

func (b *Backend) countUses(f *ir.Function) map[int]int {
	uses := make(map[int]int)
	addUse := func(v ir.Value) {
		if v == nil {
			return
		}
		if instr, ok := v.(ir.Instruction); ok {
			uses[instr.GetID()]++
		}
	}
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			visitOperands(instr, addUse)
		}
		if blk.Terminator != nil {
			inInstrs := false
			for _, instr := range blk.Instructions {
				if instr == blk.Terminator {
					inInstrs = true
					break
				}
			}
			if !inInstrs {
				visitOperands(blk.Terminator, addUse)
			}
		}
	}
	return uses
}

func (b *Backend) emitLoadAddr(reg string, addrStr string) {
	if strings.HasPrefix(addrStr, "v_") && !strings.Contains(addrStr, ",") {
		b.buf.WriteString(fmt.Sprintf("\tld%s #%s\n", reg, addrStr))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tlea%s %s\n", reg, addrStr))
	}
}

func (b *Backend) emitCall(target string) {
	b.clobberAllRegs()
	if b.picMode {
		b.buf.WriteString(fmt.Sprintf("\tlbsr %s\n", target))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tjsr %s\n", target))
	}
}

func (b *Backend) callHelper(name string) {
	if b.helpersEmitted == nil {
		b.helpersEmitted = make(map[string]bool)
	}
	b.helpersEmitted[name] = true
	b.clobberAllRegs()
	var comment string
	switch name {
	case "__mul16":
		comment = "\t; D = D * X"
	case "__div16":
		comment = "\t; D = D / X"
	case "__mod16":
		comment = "\t; D = D % X"
	case "__divmod16":
		comment = "\t; D = D / X, X = D % X"
	case "__memcpy":
		comment = "\t; copy D bytes from X to Y"
	case "__memset0":
		comment = "\t; zero D bytes at X"
	}
	if b.picMode {
		b.buf.WriteString(fmt.Sprintf("\tlbsr %s%s\n", name, comment))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tjsr %s%s\n", name, comment))
	}
}

func (b *Backend) resolveVal(val ir.Value) ir.Value {
	for {
		if sz, ok := val.(*ir.Sizeof); ok {
			val = &ir.ConstWord{Val: uint64(b.getTypeSizeByType(sz.TargetTyp))}
			continue
		}
		break
	}
	return val
}

func (b *Backend) getValSize(val ir.Value) int {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ConstByte:
		return 1
	case *ir.ConstWord:
		return 2
	case *ir.Parameter:
		return b.getTypeSizeByType(v.Typ)
	case ir.Instruction:
		if sz, ok := b.slotSizes[v.GetID()]; ok && sz > 0 {
			return sz
		}
		return b.getTypeSizeByType(v.Type())
	case *ir.Global:
		return b.getTypeSizeByType(v.Typ)
	case *ir.AddressOfGlobal, *ir.AddressOfFunc:
		return 2
	default:
		return b.getTypeSizeByType(val.Type())
	}
}

func (b *Backend) loadVal(val ir.Value) {
	val = b.resolveVal(val)
	if !b.NoLocalRegAlloc && b.canTrack(val) {
		sz := b.getValSize(val)
		if sz == 1 && b.valInB == val {
			return
		}
		if sz == 2 && b.valInD == val {
			return
		}
	}
	if inst, ok := val.(ir.Instruction); ok && len(b.globalRegs) > 0 {
		if reg, ok := b.globalRegs[inst.GetID()]; ok {
			b.buf.WriteString(fmt.Sprintf("\ttfr %s,d\t; D = %s\n", reg, b.describeVal(val)))
			if !b.NoLocalRegAlloc {
				b.setD(val)
			}
			return
		}
	}
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", v.Val))
	case *ir.Sizeof:
		sz := b.getTypeSizeByType(v.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\t; sizeof(%s)\n", sz, v.TargetTyp.Name))
	case *ir.Parameter:
		sz := b.getTypeSizeByType(v.Typ)
		addr := b.paramAddr(v.Name)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tldb %s\t; param '%s'\n", addr, v.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\t; param '%s'\n", addr, v.Name))
		}
	case *ir.AddressOfLocal:
		b.emitLoadAddr("x", b.getAddrStr(v.Local))
		b.buf.WriteString("\ttfr x,d\n")
	case *ir.AddressOfGlobal:
		if b.globalsAtY {
			offset := b.globalOffsets[v.Global.Name]
			if offset == 0 {
				b.buf.WriteString(fmt.Sprintf("\ttfr y,d\t; &global '%s'\n", v.Global.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\ttfr y,d\n\taddd #%d\t; &global '%s'\n", offset, v.Global.Name))
			}
		} else if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax v_%s,pcr\n\ttfr x,d\t; &global '%s'\n", v.Global.Name, v.Global.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd #v_%s\t; &global '%s'\n", v.Global.Name, v.Global.Name))
		}
	case *ir.AddressOfFunc:
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n\ttfr x,d\t; &func '%s'\n", v.Func.EmitName(), v.Func.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd #%s\t; &func '%s'\n", v.Func.EmitName(), v.Func.Name))
		}
	case ir.Instruction:
		sz := b.getTypeSizeByType(v.Type())
		addr := b.localAddr(v.GetID())
		desc := b.describeSlot(v.GetID())
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tldb %s\t; %s\n", addr, desc))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\t; %s\n", addr, desc))
		}
	case *ir.Global:
		sz := b.getTypeSizeByType(v.Typ)
		addr := b.getAddrStr(v)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tldb %s\t; global '%s'\n", addr, v.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\t; global '%s'\n", addr, v.Name))
		}
	default:
		log.Panicf("loadVal: unhandled %T (%v)", val, val)
	}
	if !b.NoLocalRegAlloc {
		sz := b.getValSize(val)
		if sz == 1 {
			b.setB(val)
		} else if sz == 2 {
			b.setD(val)
		}
	}
}

func (b *Backend) loadVal16(reg string, val ir.Value) {
	val = b.resolveVal(val)
	if !b.NoLocalRegAlloc && b.canTrack(val) {
		if reg == "d" {
			if b.valInD == val {
				return
			}
		} else if reg == "x" {
			if b.valInD == val {
				b.buf.WriteString("\ttfr d,x\n")
				return
			}
		}
	}
	if inst, ok := val.(ir.Instruction); ok && len(b.globalRegs) > 0 {
		if srcReg, ok := b.globalRegs[inst.GetID()]; ok {
			if srcReg == reg {
				return
			}
			b.buf.WriteString(fmt.Sprintf("\ttfr %s,%s\t; %s = %s\n", srcReg, reg, strings.ToUpper(reg), b.describeVal(val)))
			if !b.NoLocalRegAlloc && reg == "d" {
				b.setD(val)
			}
			return
		}
	}
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tld%s #%d\n", reg, uint8(v.Val)))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tld%s #%d\n", reg, v.Val))
	case *ir.Sizeof:
		sz := b.getTypeSizeByType(v.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tld%s #%d\t; sizeof(%s)\n", reg, sz, v.TargetTyp.Name))
	case *ir.Parameter:
		if b.getValSize(v) == 1 {
			b.loadVal(v)
			b.buf.WriteString("\tclra\n")
			b.clobberD()
			if reg != "d" {
				b.buf.WriteString(fmt.Sprintf("\ttfr d,%s\n", reg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld%s %s\t; param '%s'\n", reg, b.paramAddr(v.Name), v.Name))
		}
	case *ir.AddressOfGlobal:
		b.emitLoadAddr(reg, b.getAddrStr(v.Global))
	case *ir.AddressOfLocal:
		b.emitLoadAddr(reg, b.getAddrStr(v.Local))
	case *ir.AddressOfFunc:
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tlea%s %s,pcr\t; &func '%s'\n", reg, v.Func.EmitName(), v.Func.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld%s #%s\t; &func '%s'\n", reg, v.Func.EmitName(), v.Func.Name))
		}
	case ir.Instruction:
		if b.getValSize(v) == 1 {
			b.loadVal(v)
			b.buf.WriteString("\tclra\n")
			b.clobberD()
			if reg != "d" {
				b.buf.WriteString(fmt.Sprintf("\ttfr d,%s\n", reg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld%s %s\t; %s\n", reg, b.localAddr(v.GetID()), b.describeSlot(v.GetID())))
		}
	case *ir.Global:
		if b.getValSize(v) == 1 {
			b.loadVal(v)
			b.buf.WriteString("\tclra\n")
			b.clobberD()
			if reg != "d" {
				b.buf.WriteString(fmt.Sprintf("\ttfr d,%s\n", reg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld%s %s\t; global '%s'\n", reg, b.getAddrStr(v), v.Name))
		}
	default:
		b.loadVal(val)
		if reg != "d" {
			b.buf.WriteString(fmt.Sprintf("\ttfr d,%s\n", reg))
		}
	}
	if !b.NoLocalRegAlloc {
		if reg == "d" {
			b.setD(val)
		}
	}
}

func (b *Backend) storeResult(id int) {
	desc := b.describeSlot(id)
	if len(b.globalRegs) > 0 {
		if reg, ok := b.globalRegs[id]; ok {
			b.buf.WriteString(fmt.Sprintf("\ttfr d,%s\t; %s = %s\n", reg, strings.ToUpper(reg), desc))
			if !b.NoLocalRegAlloc {
				if b.curInstr != nil && b.curInstr.GetID() == id {
					b.setD(b.curInstr)
				} else {
					b.clobberD()
				}
			}
			return
		}
	}
	canon := b.resolveSlot(id)
	if _, ok := b.slots[canon]; !ok {
		if _, ok := b.slots[id]; !ok {
			return
		}
	}
	sz := b.slotSizes[id]
	addr := b.localAddr(id)
	if sz == 1 {
		b.buf.WriteString(fmt.Sprintf("\tstb %s\t; store %s\n", addr, desc))
		if !b.NoLocalRegAlloc {
			if b.curInstr != nil && b.curInstr.GetID() == id {
				b.setB(b.curInstr)
			} else {
				b.clobberB()
			}
		}
	} else {
		b.buf.WriteString(fmt.Sprintf("\tstd %s\t; store %s\n", addr, desc))
		if !b.NoLocalRegAlloc {
			if b.curInstr != nil && b.curInstr.GetID() == id {
				b.setD(b.curInstr)
			} else {
				b.clobberD()
			}
		}
	}
}

func (b *Backend) emitCopy(destReg string, srcReg string, size int) {
	if size <= 0 {
		return
	}
	defer func() {
		if b.globalsAtY {
			b.buf.WriteString("\tldy #0\n")
		}
	}()
	b.clobberAllRegs()
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tlda ,%s\n\tsta ,%s\n", srcReg, destReg))
		return
	}
	if size == 2 {
		b.buf.WriteString(fmt.Sprintf("\tldd ,%s\n\tstd ,%s\n", srcReg, destReg))
		return
	}
	if size <= b.MemcpyUnrollThreshold {
		for i := 0; i < size; i++ {
			b.buf.WriteString(fmt.Sprintf("\tlda %d,%s\n\tsta %d,%s\n", i, srcReg, i, destReg))
		}
		return
	}
	if destReg == "x" && srcReg == "y" {
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", size))
		b.callHelper("__memcpy")
		return
	}
	lbl := b.nextLabel()
	b.buf.WriteString("\tpshs u\n")
	b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
	b.buf.WriteString(fmt.Sprintf("\tlda ,%s+\n", srcReg))
	b.buf.WriteString(fmt.Sprintf("\tsta ,%s+\n", destReg))
	b.buf.WriteString("\tleau -1,u\n\tcmpu #0\n")
	b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
	b.buf.WriteString("\tpuls u\n")
}

func (b *Backend) emitMemset0(destReg string, size int) {
	if size <= 0 {
		return
	}
	b.clobberAllRegs()
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tclr ,%s\n", destReg))
		return
	}
	if size == 2 {
		b.buf.WriteString("\tclra\n\tclrb\n")
		b.buf.WriteString(fmt.Sprintf("\tstd ,%s\n", destReg))
		return
	}
	if size <= b.MemsetUnrollThreshold {
		for i := 0; i < size; i++ {
			b.buf.WriteString(fmt.Sprintf("\tclr %d,%s\n", i, destReg))
		}
		return
	}
	if destReg == "x" {
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", size))
		b.callHelper("__memset0")
		return
	}
	lbl := b.nextLabel()
	b.buf.WriteString("\tpshs u\n")
	b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
	b.buf.WriteString("\tclra\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
	b.buf.WriteString(fmt.Sprintf("\tsta ,%s+\n", destReg))
	b.buf.WriteString("\tleau -1,u\n\tcmpu #0\n")
	b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
	b.buf.WriteString("\tpuls u\n")
}

func (b *Backend) computeElementAddr(destReg string, arrayVal ir.Value, indexVal ir.Value, eltSize int) {
	b.emitLoadAddr(destReg, b.getAddrStr(arrayVal))
	if cIdx, ok := indexVal.(*ir.ConstWord); ok {
		byteOffset := int(cIdx.Val) * eltSize
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tlea%s %d,%s\n", destReg, byteOffset, destReg))
		}
	} else if cIdx, ok := indexVal.(*ir.ConstByte); ok {
		byteOffset := int(cIdx.Val) * eltSize
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tlea%s %d,%s\n", destReg, byteOffset, destReg))
		}
	} else {
		b.loadVal(indexVal) // in D
		if b.getTypeSizeByType(indexVal.Type()) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if eltSize == 1 {
			b.buf.WriteString(fmt.Sprintf("\tlea%s d,%s\n", destReg, destReg))
		} else if eltSize == 2 {
			b.buf.WriteString(fmt.Sprintf("\taslb\n\trola\n\tlea%s d,%s\n", destReg, destReg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpshs %s\n", destReg))
			b.buf.WriteString(fmt.Sprintf("\tldx #%d\n", eltSize))
			if b.InlineMul16 {
				b.emitInlineMul16()
			} else {
				b.callHelper("__mul16")
			}
			b.buf.WriteString(fmt.Sprintf("\tpuls %s\n", destReg))
			b.buf.WriteString(fmt.Sprintf("\tlea%s d,%s\n", destReg, destReg))
		}
		b.clobberD()
	}
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	b.buf.WriteString(fmt.Sprintf("\t; %s = %s %s %s\n", b.describeSlot(i.GetID()), b.describeVal(i.Left), i.Op, b.describeVal(i.Right)))
	sz := b.getTypeSizeByType(i.Typ)
	leftVal := i.Left
	rightVal := b.resolveVal(i.Right)
	if sz == 1 {
		if i.Op == "add" || i.Op == "mul" || i.Op == "and" || i.Op == "or" || i.Op == "xor" {
			_, leftConst := b.asConstByte(leftVal)
			_, rightConst := b.asConstByte(rightVal)
			if leftConst && !rightConst {
				leftVal, rightVal = rightVal, leftVal
			} else if !b.canDirectEA(rightVal, 1) && b.canDirectEA(leftVal, 1) {
				leftVal, rightVal = rightVal, leftVal
			}
		}

		switch i.Op {
		case "add":
			b.loadVal(leftVal)
			if c, ok := b.asConstByte(rightVal); ok {
				if c == 1 {
					b.buf.WriteString("\tincb\n")
				} else if c == 255 {
					b.buf.WriteString("\tdecb\n")
				} else if c != 0 {
					b.buf.WriteString(fmt.Sprintf("\taddb #%d\n", c))
				}
			} else if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\taddb %s\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(rightVal)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(leftVal)
				b.buf.WriteString("\taddb ,s+\n")
			}
		case "sub":
			b.loadVal(i.Left)
			if c, ok := b.asConstByte(rightVal); ok {
				if c == 1 {
					b.buf.WriteString("\tdecb\n")
				} else if c == 255 {
					b.buf.WriteString("\tincb\n")
				} else if c != 0 {
					b.buf.WriteString(fmt.Sprintf("\tsubb #%d\n", c))
				}
			} else if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\tsubb %s\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(i.Right)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(i.Left)
				b.buf.WriteString("\tsubb ,s+\n")
			}
		case "and":
			b.loadVal(leftVal)
			if c, ok := b.asConstByte(rightVal); ok {
				if c == 0 {
					b.buf.WriteString("\tclrb\n")
				} else if c != 255 {
					b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", c))
				}
			} else if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\tandb %s\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(rightVal)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(leftVal)
				b.buf.WriteString("\tandb ,s+\n")
			}
		case "or":
			b.loadVal(leftVal)
			if c, ok := b.asConstByte(rightVal); ok {
				if c != 0 {
					b.buf.WriteString(fmt.Sprintf("\torb #%d\n", c))
				}
			} else if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\torb %s\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(rightVal)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(leftVal)
				b.buf.WriteString("\torb ,s+\n")
			}
		case "xor":
			b.loadVal(leftVal)
			if c, ok := b.asConstByte(rightVal); ok {
				if c == 255 {
					b.buf.WriteString("\tcomb\n")
				} else if c != 0 {
					b.buf.WriteString(fmt.Sprintf("\teorb #%d\n", c))
				}
			} else if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\teorb %s\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(rightVal)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(leftVal)
				b.buf.WriteString("\teorb ,s+\n")
			}
		case "andnot":
			if c, ok := b.asConstByte(rightVal); ok {
				b.loadVal(i.Left)
				inv := byte(^c)
				if inv == 0 {
					b.buf.WriteString("\tclrb\n")
				} else if inv != 255 {
					b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", inv))
				}
			} else {
				b.loadVal(i.Right)
				b.buf.WriteString("\tcomb\n")
				if c, ok := b.asConstByte(i.Left); ok {
					b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", c))
				} else if b.canDirectEA(i.Left, 1) {
					b.buf.WriteString(fmt.Sprintf("\tandb %s\n", b.getRightEA(i.Left, 1)))
				} else {
					b.buf.WriteString("\tpshs b\n")
					b.loadVal(i.Left)
					b.buf.WriteString("\tandb ,s+\n")
				}
			}
		case "mul":
			b.loadVal(leftVal)
			if b.canDirectEA(rightVal, 1) {
				b.buf.WriteString(fmt.Sprintf("\tlda %s\n\tmul\n", b.getRightEA(rightVal, 1)))
			} else {
				b.loadVal(rightVal)
				b.buf.WriteString("\tpshs b\n")
				b.loadVal(leftVal)
				b.buf.WriteString("\tlda ,s+\n\tmul\n")
			}
		case "div":
			b.loadVal(i.Left)
			b.buf.WriteString("\tclra\n\ttfr d,x\n")
			b.loadVal(i.Right)
			b.buf.WriteString("\tclra\n")
			if b.InlineDivMod16 {
				b.emitInlineDivMod16(true)
			} else {
				b.callHelper("__div16")
			}
		case "mod":
			b.loadVal(i.Left)
			b.buf.WriteString("\tclra\n\ttfr d,x\n")
			b.loadVal(i.Right)
			b.buf.WriteString("\tclra\n")
			if b.InlineDivMod16 {
				b.emitInlineDivMod16(false)
			} else {
				b.callHelper("__mod16")
			}
		case "shl":
			if c, ok := b.asConstByte(rightVal); ok {
				b.loadVal(i.Left)
				k := int(c)
				if k == 0 {
					// no-op
				} else if k <= b.ShiftUnrollThreshold {
					for s := 0; s < k; s++ {
						b.buf.WriteString("\taslb\n")
					}
				} else if k >= 8 {
					b.buf.WriteString("\tclrb\n")
				} else {
					lblLoop := b.nextLabel()
					b.buf.WriteString(fmt.Sprintf("\tlda #%d\n%s:\n\taslb\n\tdeca\n\tbne %s\n", k, lblLoop, lblLoop))
				}
			} else {
				lblLoop := b.nextLabel()
				lblDone := b.nextLabel()
				if b.canDirectEA(rightVal, 1) {
					b.buf.WriteString(fmt.Sprintf("\tlda %s\n", b.getRightEA(rightVal, 1)))
					b.loadVal(i.Left)
				} else {
					b.loadVal(i.Right)
					b.buf.WriteString("\tpshs b\n")
					b.loadVal(i.Left)
					b.buf.WriteString("\tpuls a\n")
				}
				b.buf.WriteString(fmt.Sprintf("\ttsta\n\tbeq %s\n%s:\n\taslb\n\tdeca\n\tbne %s\n%s:\n", lblDone, lblLoop, lblLoop, lblDone))
			}
		case "shr":
			isInt := i.Typ.Equals(ir.TypeInt)
			shiftInst := "\tlsrb\n"
			if isInt {
				shiftInst = "\tasrb\n"
			}
			if c, ok := b.asConstByte(rightVal); ok {
				b.loadVal(i.Left)
				k := int(c)
				if k == 0 {
					// no-op
				} else if k <= b.ShiftUnrollThreshold {
					for s := 0; s < k; s++ {
						b.buf.WriteString(shiftInst)
					}
				} else if k >= 8 && !isInt {
					b.buf.WriteString("\tclrb\n")
				} else {
					lblLoop := b.nextLabel()
					b.buf.WriteString(fmt.Sprintf("\tlda #%d\n%s:\n%s\tdeca\n\tbne %s\n", k, lblLoop, shiftInst, lblLoop))
				}
			} else {
				lblLoop := b.nextLabel()
				lblDone := b.nextLabel()
				if b.canDirectEA(rightVal, 1) {
					b.buf.WriteString(fmt.Sprintf("\tlda %s\n", b.getRightEA(rightVal, 1)))
					b.loadVal(i.Left)
				} else {
					b.loadVal(i.Right)
					b.buf.WriteString("\tpshs b\n")
					b.loadVal(i.Left)
					b.buf.WriteString("\tpuls a\n")
				}
				b.buf.WriteString(fmt.Sprintf("\ttsta\n\tbeq %s\n%s:\n%s\tdeca\n\tbne %s\n%s:\n", lblDone, lblLoop, shiftInst, lblLoop, lblDone))
			}
		default:
			log.Panicf("unhandled 1-byte op: %s", i.Op)
		}
		b.storeResult(i.GetID())
		return
	}

	// 2-byte binary operation
	if i.Op == "add" || i.Op == "mul" || i.Op == "and" || i.Op == "or" || i.Op == "xor" {
		_, leftConst := b.asConstWord(leftVal)
		_, rightConst := b.asConstWord(rightVal)
		if leftConst && !rightConst {
			leftVal, rightVal = rightVal, leftVal
		} else if !b.canDirectEA(rightVal, 2) && b.canDirectEA(leftVal, 2) {
			leftVal, rightVal = rightVal, leftVal
		}
	}

	destReg := ""
	if len(b.globalRegs) > 0 {
		destReg = b.globalRegs[i.GetID()]
	}

	if destReg != "" {
		if i.Op == "add" {
			if c, ok := b.asConstWord(rightVal); ok {
				if leftInst, ok := leftVal.(ir.Instruction); ok && len(b.globalRegs) > 0 {
					if srcReg, ok := b.globalRegs[leftInst.GetID()]; ok && srcReg != "" {
						if destReg != srcReg || c != 0 {
							b.buf.WriteString(fmt.Sprintf("\tlea%s %d,%s\n", destReg, c, srcReg))
						}
						b.clobberD()
						return
					}
				}
			}
			var srcReg string
			var otherVal ir.Value
			if rightInst, ok := rightVal.(ir.Instruction); ok && len(b.globalRegs) > 0 && b.globalRegs[rightInst.GetID()] != "" {
				srcReg = b.globalRegs[rightInst.GetID()]
				otherVal = leftVal
			} else if leftInst, ok := leftVal.(ir.Instruction); ok && len(b.globalRegs) > 0 && b.globalRegs[leftInst.GetID()] != "" {
				srcReg = b.globalRegs[leftInst.GetID()]
				otherVal = rightVal
			}
			if srcReg != "" {
				b.loadVal(otherVal)
				if b.getValSize(otherVal) == 1 {
					b.buf.WriteString("\tclra\n")
				}
				b.buf.WriteString(fmt.Sprintf("\tlea%s d,%s\n", destReg, srcReg))
				b.clobberD()
				return
			}
		} else if i.Op == "sub" {
			if c, ok := b.asConstWord(i.Right); ok {
				if leftInst, ok := i.Left.(ir.Instruction); ok && len(b.globalRegs) > 0 {
					if srcReg, ok := b.globalRegs[leftInst.GetID()]; ok && srcReg != "" {
						if destReg != srcReg || c != 0 {
							b.buf.WriteString(fmt.Sprintf("\tlea%s %d,%s\n", destReg, -c, srcReg))
						}
						b.clobberD()
						return
					}
				}
			}
		}
	}

	switch i.Op {
	case "add":
		b.loadVal(leftVal)
		if b.getValSize(leftVal) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if c, ok := b.asConstWord(rightVal); ok {
			if c != 0 {
				b.buf.WriteString(fmt.Sprintf("\taddd #%d\n", c))
			}
		} else if b.canDirectEA(rightVal, 2) {
			b.buf.WriteString(fmt.Sprintf("\taddd %s\n", b.getRightEA(rightVal, 2)))
		} else if rightInst, ok := rightVal.(ir.Instruction); ok && len(b.globalRegs) > 0 && b.globalRegs[rightInst.GetID()] != "" {
			srcReg := b.globalRegs[rightInst.GetID()]
			b.buf.WriteString(fmt.Sprintf("\tleax d,%s\n\ttfr x,d\n", srcReg))
		} else {
			b.loadVal16("x", rightVal)
			b.buf.WriteString("\tstx ,--s\n\taddd ,s++\n")
		}
	case "sub":
		b.loadVal(i.Left)
		if b.getValSize(i.Left) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if c, ok := b.asConstWord(rightVal); ok {
			if c != 0 {
				b.buf.WriteString(fmt.Sprintf("\tsubd #%d\n", c))
			}
		} else if b.canDirectEA(rightVal, 2) {
			b.buf.WriteString(fmt.Sprintf("\tsubd %s\n", b.getRightEA(rightVal, 2)))
		} else {
			rightReg := ""
			if rightInst, ok := i.Right.(ir.Instruction); ok && len(b.globalRegs) > 0 {
				rightReg = b.globalRegs[rightInst.GetID()]
			}
			if rightReg != "" {
				b.buf.WriteString(fmt.Sprintf("\tst%s ,--s\n\tsubd ,s++\n", rightReg))
			} else {
				b.loadVal16("x", i.Right)
				b.buf.WriteString("\tstx ,--s\n\tsubd ,s++\n")
			}
		}
	case "and":
		b.loadVal(leftVal)
		if b.getValSize(leftVal) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if c, ok := b.asConstWord(rightVal); ok {
			hi := byte(c >> 8)
			lo := byte(c & 0xff)
			if hi == 0 {
				b.buf.WriteString("\tclra\n")
			} else if hi != 255 {
				b.buf.WriteString(fmt.Sprintf("\tanda #%d\n", hi))
			}
			if lo == 0 {
				b.buf.WriteString("\tclrb\n")
			} else if lo != 255 {
				b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", lo))
			}
		} else if b.canDirectEA(rightVal, 2) {
			addr := b.getRightEA(rightVal, 2)
			b.buf.WriteString(fmt.Sprintf("\tanda %s\n\tandb %s\n", addr, offsetAddrStr(addr, 1)))
		} else {
			b.loadVal16("x", rightVal)
			b.buf.WriteString("\tpshs x\n\tanda 0,s\n\tandb 1,s\n\tleas 2,s\n")
		}
	case "or":
		b.loadVal(leftVal)
		if b.getValSize(leftVal) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if c, ok := b.asConstWord(rightVal); ok {
			hi := byte(c >> 8)
			lo := byte(c & 0xff)
			if hi != 0 {
				b.buf.WriteString(fmt.Sprintf("\tora #%d\n", hi))
			}
			if lo != 0 {
				b.buf.WriteString(fmt.Sprintf("\torb #%d\n", lo))
			}
		} else if b.canDirectEA(rightVal, 2) {
			addr := b.getRightEA(rightVal, 2)
			b.buf.WriteString(fmt.Sprintf("\tora %s\n\torb %s\n", addr, offsetAddrStr(addr, 1)))
		} else {
			b.loadVal16("x", rightVal)
			b.buf.WriteString("\tpshs x\n\tora 0,s\n\torb 1,s\n\tleas 2,s\n")
		}
	case "xor":
		b.loadVal(leftVal)
		if b.getValSize(leftVal) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if c, ok := b.asConstWord(rightVal); ok {
			hi := byte(c >> 8)
			lo := byte(c & 0xff)
			if hi == 255 {
				b.buf.WriteString("\tcoma\n")
			} else if hi != 0 {
				b.buf.WriteString(fmt.Sprintf("\teora #%d\n", hi))
			}
			if lo == 255 {
				b.buf.WriteString("\tcomb\n")
			} else if lo != 0 {
				b.buf.WriteString(fmt.Sprintf("\teorb #%d\n", lo))
			}
		} else if b.canDirectEA(rightVal, 2) {
			addr := b.getRightEA(rightVal, 2)
			b.buf.WriteString(fmt.Sprintf("\teora %s\n\teorb %s\n", addr, offsetAddrStr(addr, 1)))
		} else {
			b.loadVal16("x", rightVal)
			b.buf.WriteString("\tpshs x\n\teora 0,s\n\teorb 1,s\n\tleas 2,s\n")
		}
	case "andnot":
		if c, ok := b.asConstWord(rightVal); ok {
			b.loadVal(i.Left)
			if b.getValSize(i.Left) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			inv := ^c
			hi := byte(inv >> 8)
			lo := byte(inv & 0xff)
			if hi == 0 {
				b.buf.WriteString("\tclra\n")
			} else if hi != 255 {
				b.buf.WriteString(fmt.Sprintf("\tanda #%d\n", hi))
			}
			if lo == 0 {
				b.buf.WriteString("\tclrb\n")
			} else if lo != 255 {
				b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", lo))
			}
		} else {
			b.loadVal(i.Right)
			if b.getValSize(i.Right) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString("\tcoma\n\tcomb\n")
			if c, ok := b.asConstWord(i.Left); ok {
				hi := byte(c >> 8)
				lo := byte(c & 0xff)
				if hi == 0 {
					b.buf.WriteString("\tclra\n")
				} else if hi != 255 {
					b.buf.WriteString(fmt.Sprintf("\tanda #%d\n", hi))
				}
				if lo == 0 {
					b.buf.WriteString("\tclrb\n")
				} else if lo != 255 {
					b.buf.WriteString(fmt.Sprintf("\tandb #%d\n", lo))
				}
			} else if b.canDirectEA(i.Left, 2) {
				addr := b.getRightEA(i.Left, 2)
				b.buf.WriteString(fmt.Sprintf("\tanda %s\n\tandb %s\n", addr, offsetAddrStr(addr, 1)))
			} else {
				b.loadVal16("x", i.Left)
				b.buf.WriteString("\tpshs x\n\tanda 0,s\n\tandb 1,s\n\tleas 2,s\n")
			}
		}
	case "mul":
		b.loadVal(leftVal)
		if b.getValSize(leftVal) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		b.loadVal16("x", rightVal)
		if b.InlineMul16 {
			b.emitInlineMul16()
		} else {
			b.callHelper("__mul16")
		}
	case "div":
		b.loadVal16("x", i.Left)
		b.loadVal(i.Right)
		if b.getValSize(i.Right) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if b.InlineDivMod16 {
			b.emitInlineDivMod16(true)
		} else {
			b.callHelper("__div16")
		}
	case "mod":
		b.loadVal16("x", i.Left)
		b.loadVal(i.Right)
		if b.getValSize(i.Right) == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if b.InlineDivMod16 {
			b.emitInlineDivMod16(false)
		} else {
			b.callHelper("__mod16")
		}
	case "shl":
		if c, ok := b.asConstWord(rightVal); ok {
			b.loadVal(i.Left)
			if b.getValSize(i.Left) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			k := int(c)
			if k == 0 {
				// no-op
			} else if k <= b.ShiftUnrollThreshold {
				for s := 0; s < k; s++ {
					b.buf.WriteString("\taslb\n\trola\n")
				}
			} else if k == 8 {
				b.buf.WriteString("\ttfr b,a\n\tclrb\n")
			} else if k >= 16 {
				b.buf.WriteString("\tclra\n\tclrb\n")
			} else {
				lblLoop := b.nextLabel()
				lblDone := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("\tldx #%d\n%s:\n\taslb\n\trola\n\tleax -1,x\n\tbne %s\n%s:\n", k, lblLoop, lblLoop, lblDone))
			}
		} else {
			lblLoop := b.nextLabel()
			lblDone := b.nextLabel()
			b.loadVal16("x", i.Right)
			b.loadVal(i.Left)
			if b.getValSize(i.Left) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString(fmt.Sprintf("\tcmpx #0\n\tbeq %s\n%s:\n\taslb\n\trola\n\tleax -1,x\n\tbne %s\n%s:\n", lblDone, lblLoop, lblLoop, lblDone))
		}
	case "shr":
		isInt := i.Typ.Equals(ir.TypeInt)
		shiftInst := "\tlsra\n\trorb\n"
		if isInt {
			shiftInst = "\tasra\n\trorb\n"
		}
		if c, ok := b.asConstWord(rightVal); ok {
			b.loadVal(i.Left)
			if b.getValSize(i.Left) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			k := int(c)
			if k == 0 {
				// no-op
			} else if k <= b.ShiftUnrollThreshold {
				for s := 0; s < k; s++ {
					b.buf.WriteString(shiftInst)
				}
			} else if k == 8 && !isInt {
				b.buf.WriteString("\ttfr a,b\n\tclra\n")
			} else if k >= 16 && !isInt {
				b.buf.WriteString("\tclra\n\tclrb\n")
			} else {
				lblLoop := b.nextLabel()
				lblDone := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("\tldx #%d\n%s:\n%s\tleax -1,x\n\tbne %s\n%s:\n", k, lblLoop, shiftInst, lblLoop, lblDone))
			}
		} else {
			lblLoop := b.nextLabel()
			lblDone := b.nextLabel()
			b.loadVal16("x", i.Right)
			b.loadVal(i.Left)
			if b.getValSize(i.Left) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString(fmt.Sprintf("\tcmpx #0\n\tbeq %s\n%s:\n%s\tleax -1,x\n\tbne %s\n%s:\n", lblDone, lblLoop, shiftInst, lblLoop, lblDone))
		}
	default:
		log.Panicf("unhandled 2-byte op: %s", i.Op)
	}
	b.storeResult(i.GetID())
}

func (b *Backend) emitCompare16(leftVal ir.Value, rightVal ir.Value) {
	leftVal = b.resolveVal(leftVal)
	rightVal = b.resolveVal(rightVal)
	leftSize := b.getValSize(leftVal)

	leftReg := ""
	if leftInst, ok := leftVal.(ir.Instruction); ok && len(b.globalRegs) > 0 {
		leftReg = b.globalRegs[leftInst.GetID()]
	}
	rightReg := ""
	if rightInst, ok := rightVal.(ir.Instruction); ok && len(b.globalRegs) > 0 {
		rightReg = b.globalRegs[rightInst.GetID()]
	}

	if leftReg != "" && rightReg != "" {
		b.buf.WriteString(fmt.Sprintf("\tst%s ,--s\n\tcmp%s ,s++\n", rightReg, leftReg))
	} else if leftReg != "" && b.canDirectEA(rightVal, 2) {
		b.buf.WriteString(fmt.Sprintf("\tcmp%s %s\n", leftReg, b.getRightEA(rightVal, 2)))
	} else if leftReg != "" {
		b.loadVal16("x", rightVal)
		b.buf.WriteString(fmt.Sprintf("\tstx ,--s\n\tcmp%s ,s++\n", leftReg))
	} else if rightReg != "" {
		b.loadVal(leftVal)
		if leftSize == 1 {
			b.buf.WriteString("\tclra\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tst%s ,--s\n\tcmpd ,s++\n", rightReg))
	} else {
		b.loadVal(leftVal)
		if leftSize == 1 {
			b.buf.WriteString("\tclra\n")
		}
		if b.canDirectEA(rightVal, 2) {
			b.buf.WriteString(fmt.Sprintf("\tcmpd %s\n", b.getRightEA(rightVal, 2)))
		} else {
			b.loadVal16("x", rightVal)
			b.buf.WriteString("\tstx ,--s\n\tcmpd ,s++\n")
		}
	}
}

func (b *Backend) emitCompare(i *ir.Compare) {
	if b.fusedCompares[i.GetID()] {
		return
	}
	leftVal := b.resolveVal(i.Left)
	rightVal := b.resolveVal(i.Right)
	leftSize := b.getValSize(leftVal)
	rightSize := b.getValSize(rightVal)

	if leftSize == 1 && rightSize == 1 {
		b.loadVal(i.Left)
		if c, ok := b.asConstByte(rightVal); ok && c == 0 {
			b.buf.WriteString("\ttstb\n")
		} else if b.canDirectEA(rightVal, 1) {
			b.buf.WriteString(fmt.Sprintf("\tcmpb %s\n", b.getRightEA(rightVal, 1)))
		} else {
			b.loadVal(i.Right)
			b.buf.WriteString("\tpshs b\n")
			b.loadVal(i.Left)
			b.buf.WriteString("\tcmpb ,s+\n")
		}
	} else {
		b.emitCompare16(i.Left, i.Right)
	}

	lblTrue := b.nextLabel()
	lblEnd := b.nextLabel()

	isInt := i.Left.Type().Equals(ir.TypeInt) || i.Right.Type().Equals(ir.TypeInt)
	if !isInt && b.isZeroVal(rightVal) {
		switch i.Op {
		case "eq":
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblTrue))
		case "neq":
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblTrue))
		case "gt":
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblTrue))
		case "lte":
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblTrue))
		case "gte":
			b.buf.WriteString(fmt.Sprintf("\tbra %s\n", lblTrue))
		case "lt":
			// Unsigned < 0 is never true, do not branch to lblTrue
		default:
			log.Panicf("emitCompare: unhandled unsigned cmp op against zero %s", i.Op)
		}
	} else {
		switch i.Op {
		case "eq":
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblTrue))
		case "neq":
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblTrue))
		case "lt":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tblt %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tblo %s\n", lblTrue))
			}
		case "lte":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tble %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbls %s\n", lblTrue))
			}
		case "gt":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tbgt %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbhi %s\n", lblTrue))
			}
		case "gte":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tbge %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbhs %s\n", lblTrue))
			}
		default:
			log.Panicf("emitCompare: unknown op %s", i.Op)
		}
	}
	b.buf.WriteString(fmt.Sprintf("\tclrb\n\tbra %s\n%s:\n\tldb #1\n%s:\n", lblEnd, lblTrue, lblEnd))
	b.storeResult(i.GetID())
}

func (b *Backend) emitCallInstr(i *ir.Call) {
	retSize := b.getTypeSizeByType(i.Typ)

	if retSize > 2 {
		alignedRet := align(retSize)
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\t; reserve %d bytes for return buffer\n", alignedRet, alignedRet))
		b.pushBytes(alignedRet)
	}

	totalArgBytes := 0
	for idx := len(i.Args) - 1; idx >= 0; idx-- {
		arg := i.Args[idx]
		sz := b.getTypeSizeByType(arg.Type())
		aligned := align(sz)
		totalArgBytes += aligned
		argDesc := b.describeVal(arg)
		paramName := ""
		if idx < len(i.Func.Parameters) {
			paramName = i.Func.Parameters[idx].Name
		}
		var comment string
		if paramName != "" {
			comment = fmt.Sprintf("push arg %d '%s' (%s)", idx, paramName, argDesc)
		} else {
			comment = fmt.Sprintf("push arg %d (%s)", idx, argDesc)
		}

		if sz == 1 {
			b.loadVal(arg)
			b.buf.WriteString(fmt.Sprintf("\tpshs b\t; %s\n", comment))
			b.pushBytes(1)
		} else if sz == 2 {
			b.loadVal(arg)
			if b.getValSize(arg) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString(fmt.Sprintf("\tpshs d\t; %s\n", comment))
			b.pushBytes(2)
		} else {
			b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\t; %s\n", aligned, comment))
			b.pushBytes(aligned)
			b.emitLoadAddr("y", b.getAddrStr(arg))
			b.buf.WriteString("\tleax ,s\n")
			b.emitCopy("x", "y", sz)
		}
	}

	b.buf.WriteString(fmt.Sprintf("\t; call %s\n", i.Func.Name))
	b.emitCall(i.Func.EmitName())

	if totalArgBytes > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas %d,s\t; pop %d bytes args\n", totalArgBytes, totalArgBytes))
		b.popBytes(totalArgBytes)
	}

	if !i.Typ.Equals(ir.TypeVoid) {
		if retSize == 1 {
			b.storeResult(i.GetID())
		} else if retSize == 2 {
			b.storeResult(i.GetID())
		} else {
			b.emitLoadAddr("x", b.localAddr(i.GetID()))
			b.buf.WriteString("\tleay ,s\n")
			b.emitCopy("x", "y", retSize)
			alignedRet := align(retSize)
			b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", alignedRet))
			b.popBytes(alignedRet)
		}
	}
}

func (b *Backend) emitIndirectCall(i *ir.IndirectCall) {
	retSize := b.getTypeSizeByType(i.Typ)

	if retSize > 2 {
		alignedRet := align(retSize)
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", alignedRet))
		b.pushBytes(alignedRet)
	}

	totalArgBytes := 0
	for idx := len(i.Args) - 1; idx >= 0; idx-- {
		arg := i.Args[idx]
		sz := b.getTypeSizeByType(arg.Type())
		aligned := align(sz)
		totalArgBytes += aligned
		if sz == 1 {
			b.loadVal(arg)
			b.buf.WriteString("\tpshs b\n")
			b.pushBytes(1)
		} else if sz == 2 {
			b.loadVal(arg)
			if b.getValSize(arg) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString("\tpshs d\n")
			b.pushBytes(2)
		} else {
			b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", aligned))
			b.pushBytes(aligned)
			b.emitLoadAddr("y", b.getAddrStr(arg))
			b.buf.WriteString("\tleax ,s\n")
			b.emitCopy("x", "y", sz)
		}
	}

	b.loadVal16("x", i.FuncPtr)
	b.buf.WriteString("\tjsr ,x\n")

	if totalArgBytes > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", totalArgBytes))
		b.popBytes(totalArgBytes)
	}

	if !i.Typ.Equals(ir.TypeVoid) {
		if retSize == 1 {
			b.storeResult(i.GetID())
		} else if retSize == 2 {
			b.storeResult(i.GetID())
		} else {
			b.emitLoadAddr("x", b.localAddr(i.GetID()))
			b.buf.WriteString("\tleay ,s\n")
			b.emitCopy("x", "y", retSize)
			alignedRet := align(retSize)
			b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", alignedRet))
			b.popBytes(alignedRet)
		}
	}
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		b.loadVal(i.Args[0])
		b.buf.WriteString("\ttfr d,x\n\tjmp __exit\n")
	case "panic":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.fmtCount++
				lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
				b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", lbl, strLit.Value))
				if b.picMode {
					b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lbl))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lbl))
				}
				b.buf.WriteString(fmt.Sprintf("\tstx %s\n", b.panicAddr()))
			} else {
				b.loadVal(i.Args[0])
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.panicAddr()))
			}
		} else {
			b.buf.WriteString("\tclra\n\tclrb\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.panicAddr()))
		}

		b.fmtCount++
		lblPanicMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*PANIC* %%s\\n\"\n", lblPanicMsg))

		b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.panicAddr()))
		b.buf.WriteString("\tpshs d\n")
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblPanicMsg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblPanicMsg))
		}
		b.buf.WriteString("\tpshs x\n")
		b.emitCall("_printf")
		b.buf.WriteString("\tleas 4,s\n")

		b.buf.WriteString(fmt.Sprintf("\tldx %s\n", b.jmpChainAddr()))
		b.buf.WriteString("\tcmpx #0\n")
		lblNext := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext))
		b.buf.WriteString("\tclra\n\tldb #1\n")
		b.buf.WriteString("\tldy 8,x\n\tldu 6,x\n\tlds 4,x\n\tjmp [2,x]\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))

		b.fmtCount++
		lblAbortMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n", lblAbortMsg))
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblAbortMsg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblAbortMsg))
		}
		b.buf.WriteString("\tpshs x\n")
		b.emitCall("_printf")
		b.buf.WriteString("\tleas 2,s\n\tldx #1\n\tjmp __exit\n")

	case "_unlink_jmp_":
		b.buf.WriteString(fmt.Sprintf("\tldx %s\n", b.jmpChainAddr()))
		b.buf.WriteString("\tcmpx #0\n")
		lblNext := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext))
		b.buf.WriteString("\tldd 0,x\n")
		b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.jmpChainAddr()))
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))

	case "_propagate_panic_":
		b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.panicAddr()))
		b.buf.WriteString("\tcmpd #0\n")
		lblNext3 := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext3))

		b.buf.WriteString(fmt.Sprintf("\tldx %s\n", b.jmpChainAddr()))
		b.buf.WriteString("\tcmpx #0\n")
		lblNext2 := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext2))

		b.buf.WriteString("\tclra\n\tldb #1\n")
		b.buf.WriteString("\tldy 8,x\n\tldu 6,x\n\tlds 4,x\n\tjmp [2,x]\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext2))

		b.fmtCount++
		lblAbortMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n", lblAbortMsg))
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblAbortMsg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblAbortMsg))
		}
		b.buf.WriteString("\tpshs x\n")
		b.emitCall("_printf")
		b.buf.WriteString("\tleas 2,s\n\tldx #1\n\tjmp __exit\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext3))
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpOffset := b.jmpSlots[id]
	b.emitLoadAddr("x", b.offsetAddr(jmpOffset, 10))

	b.buf.WriteString(fmt.Sprintf("\tldd %s\n\tstd 0,x\n\tstx %s\n", b.jmpChainAddr(), b.jmpChainAddr()))

	b.buf.WriteString("\tsts 4,x\n\tstu 6,x\n\tsty 8,x\n")
	lblNext := b.nextLabel()
	if b.picMode {
		b.buf.WriteString(fmt.Sprintf("\tleau %s,pcr\n\tstu 2,x\n\tldu 6,x\n", lblNext))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tldd #%s\n\tstd 2,x\n", lblNext))
	}
	b.buf.WriteString("\tclra\n\tclrb\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))
	b.storeResult(id)
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf)
	b.buf.WriteString("\ttfr d,x\n")
	b.buf.WriteString("\tclra\n\tldb #1\n")
	b.buf.WriteString("\tldy 8,x\n\tldu 6,x\n\tlds 4,x\n\tjmp [2,x]\n")
}

func (b *Backend) emitCast(i *ir.Cast) {
	id := i.GetID()
	dstSz := b.getTypeSizeByType(i.Typ)
	srcSz := b.getTypeSizeByType(i.Operand.Type())
	switch i.Op {
	case "word_to_ptr", "ptr_to_word", "bitcast":
		if dstSz == srcSz {
			b.loadVal(i.Operand)
			b.storeResult(id)
			return
		}
		b.loadVal(i.Operand)
		if dstSz == 1 {
			b.storeResult(id)
		} else {
			if srcSz == 1 {
				if i.Operand.Type().Equals(ir.TypeInt) {
					b.buf.WriteString("\tsex\n")
				} else {
					b.buf.WriteString("\tclra\n")
				}
			}
			b.storeResult(id)
		}
	case "trunc":
		b.loadVal(i.Operand)
		b.storeResult(id)
	case "zext", "zero_ext":
		opSz := b.getTypeSizeByType(i.Operand.Type())
		b.loadVal(i.Operand)
		if opSz == 1 {
			b.buf.WriteString("\tclra\n")
		}
		b.storeResult(id)
	case "sext", "sign_ext":
		opSz := b.getTypeSizeByType(i.Operand.Type())
		b.loadVal(i.Operand)
		if opSz == 1 {
			b.buf.WriteString("\tsex\n")
		}
		b.storeResult(id)
	default:
		sz := b.getTypeSizeByType(i.Typ)
		b.loadVal(i.Operand)
		if sz == 1 {
			b.storeResult(id)
		} else {
			if b.getTypeSizeByType(i.Operand.Type()) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.storeResult(id)
		}
	}
}

func (b *Backend) emitPrint(newline bool, args []ir.Value) {
	b.fmtCount++
	fmtLabel := fmt.Sprintf(".Lfmt%d", b.fmtCount)

	var formatStrs []string
	var dataArgs []ir.Value

	for _, arg := range args {
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, strLit)
		} else if arg.Type().Equals(ir.TypeInt) {
			formatStrs = append(formatStrs, "%d")
			dataArgs = append(dataArgs, arg)
		} else if strings.HasSuffix(arg.Type().Name, "slice_byte") || arg.Type().Name == "*byte" {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, arg)
		} else {
			formatStrs = append(formatStrs, "%u")
			dataArgs = append(dataArgs, arg)
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\n"
	}

	b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", fmtLabel, format))

	for i := len(dataArgs) - 1; i >= 0; i-- {
		if strLit, ok := dataArgs[i].(*ir.StringLiteral); ok {
			b.fmtCount++
			lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", lbl, strLit.Value))
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lbl))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lbl))
			}
			b.buf.WriteString("\tpshs x\n")
			b.pushBytes(2)
		} else {
			b.loadVal(dataArgs[i])
			if b.getTypeSizeByType(dataArgs[i].Type()) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.buf.WriteString("\tpshs d\n")
			b.pushBytes(2)
		}
	}

	if b.picMode {
		b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", fmtLabel))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", fmtLabel))
	}
	b.buf.WriteString("\tpshs x\n")
	b.pushBytes(2)

	b.emitCall("_printf")

	cleanup := 2 + len(dataArgs)*2
	b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", cleanup))
	b.popBytes(cleanup)
}

func isReg(loc string) bool {
	return loc == "u" || loc == "y" || loc == "x" || loc == "d"
}

func (b *Backend) getPhiSrcLoc(val ir.Value) string {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ConstByte, *ir.ConstWord, *ir.ZeroInit, *ir.StringLiteral:
		return fmt.Sprintf("$const_%p", v)
	case *ir.Parameter:
		return b.paramAddr(v.Name)
	case *ir.Global:
		if b.globalsAtY {
			return fmt.Sprintf("%d,y", b.globalOffsets[v.Name])
		}
		if b.picMode {
			return fmt.Sprintf("v_%s,pcr", v.Name)
		}
		return fmt.Sprintf("v_%s", v.Name)
	case ir.Instruction:
		if len(b.globalRegs) > 0 {
			if reg, ok := b.globalRegs[v.GetID()]; ok {
				return reg
			}
		}
		canon := b.resolveSlot(v.GetID())
		if _, ok := b.slots[canon]; ok {
			return b.localAddr(v.GetID())
		}
		return fmt.Sprintf("$instr_%d", v.GetID())
	default:
		return fmt.Sprintf("$val_%p", val)
	}
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	if b.NoCSSALowering {
		for _, instr := range to.Instructions {
			if phi, ok := instr.(*ir.Phi); ok {
				for _, edge := range phi.Edges {
					if edge.Block == from {
						sz := b.getTypeSizeByType(phi.Typ)
						var destAddr string
						if len(b.globalRegs) > 0 {
							if reg, ok := b.globalRegs[phi.GetID()]; ok {
								destAddr = reg
							}
						}
						if destAddr == "" {
							canon := b.resolveSlot(phi.GetID())
							if _, ok := b.slots[canon]; !ok {
								if _, ok := b.slots[phi.GetID()]; !ok {
									continue
								}
							}
							destAddr = b.localAddr(phi.GetID())
						}
						if isReg(destAddr) {
							b.loadVal16(destAddr, edge.Value)
						} else if sz == 1 {
							b.loadVal(edge.Value)
							b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destAddr))
						} else if sz == 2 {
							b.loadVal(edge.Value)
							if b.getValSize(edge.Value) == 1 {
								b.buf.WriteString("\tclra\n")
							}
							b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destAddr))
						} else {
							b.emitLoadAddr("y", b.getAddrStr(edge.Value))
							b.emitLoadAddr("x", destAddr)
							b.emitCopy("x", "y", sz)
						}
					}
				}
			}
		}
		return
	}

	type phiAssignment struct {
		phi      *ir.Phi
		val      ir.Value
		sz       int
		destAddr string
		srcAddr  string
	}

	var assignments []phiAssignment
	var moves []opt.ParallelMove

	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					sz := b.getTypeSizeByType(phi.Typ)
					var destAddr string
					if len(b.globalRegs) > 0 {
						if reg, ok := b.globalRegs[phi.GetID()]; ok {
							destAddr = reg
						}
					}
					if destAddr == "" {
						canon := b.resolveSlot(phi.GetID())
						if _, ok := b.slots[canon]; !ok {
							if _, ok := b.slots[phi.GetID()]; !ok {
								continue
							}
						}
						destAddr = b.localAddr(phi.GetID())
					}
					srcAddr := b.getPhiSrcLoc(edge.Value)
					assignments = append(assignments, phiAssignment{
						phi:      phi,
						val:      edge.Value,
						sz:       sz,
						destAddr: destAddr,
						srcAddr:  srcAddr,
					})
					moves = append(moves, opt.ParallelMove{
						Dest: destAddr,
						Src:  srcAddr,
						Size: sz,
					})
				}
			}
		}
	}

	if len(assignments) == 0 {
		return
	}

	assignMap := make(map[string]phiAssignment, len(assignments))
	for _, a := range assignments {
		assignMap[a.destAddr] = a
	}

	steps := opt.SequentializeParallelCopies(moves)

	for _, step := range steps {
		switch step.Kind {
		case opt.StepCopy:
			destAddr := step.Dest.(string)
			sz := step.Size
			if assign, ok := assignMap[destAddr]; ok {
				if isReg(destAddr) {
					b.loadVal16(destAddr, assign.val)
				} else if sz == 1 {
					b.loadVal(assign.val)
					b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destAddr))
				} else if sz == 2 {
					b.loadVal(assign.val)
					if b.getValSize(assign.val) == 1 {
						b.buf.WriteString("\tclra\n")
					}
					b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destAddr))
				} else {
					if _, isZero := assign.val.(*ir.ZeroInit); isZero || strings.HasPrefix(assign.srcAddr, "$const_") {
						b.emitLoadAddr("x", destAddr)
						b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", sz))
						b.callHelper("__memset0")
					} else {
						b.emitLoadAddr("y", assign.srcAddr)
						b.emitLoadAddr("x", destAddr)
						b.emitCopy("x", "y", sz)
					}
				}
			} else {
				srcAddr := step.Src.(string)
				if isReg(destAddr) {
					if isReg(srcAddr) {
						if destAddr != srcAddr {
							b.buf.WriteString(fmt.Sprintf("\ttfr %s,%s\n", srcAddr, destAddr))
						}
					} else {
						b.buf.WriteString(fmt.Sprintf("\tld%s %s\n", destAddr, srcAddr))
					}
				} else if isReg(srcAddr) {
					b.buf.WriteString(fmt.Sprintf("\tst%s %s\n", srcAddr, destAddr))
				} else if sz == 1 {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n", srcAddr))
					b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destAddr))
				} else if sz == 2 {
					b.buf.WriteString(fmt.Sprintf("\tldd %s\n", srcAddr))
					b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destAddr))
				} else {
					if strings.HasPrefix(srcAddr, "$const_") {
						b.emitLoadAddr("x", destAddr)
						b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", sz))
						b.callHelper("__memset0")
					} else {
						b.emitLoadAddr("y", srcAddr)
						b.emitLoadAddr("x", destAddr)
						b.emitCopy("x", "y", sz)
					}
				}
			}

		case opt.StepSwap:
			loc1 := step.Loc1.(string)
			loc2 := step.Loc2.(string)
			sz := step.Size
			if isReg(loc1) && isReg(loc2) {
				b.buf.WriteString(fmt.Sprintf("\texg %s,%s\n", loc1, loc2))
			} else if isReg(loc1) {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", loc2))
				b.buf.WriteString(fmt.Sprintf("\texg %s,d\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", loc2))
			} else if isReg(loc2) {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\texg %s,d\n", loc2))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", loc1))
			} else if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tlda %s\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", loc2))
				b.buf.WriteString("\texg a,b\n")
				b.buf.WriteString(fmt.Sprintf("\tsta %s\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\tstb %s\n", loc2))
			} else if sz == 2 {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\tldx %s\n", loc2))
				b.buf.WriteString("\texg d,x\n")
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", loc1))
				b.buf.WriteString(fmt.Sprintf("\tstx %s\n", loc2))
			} else {
				scratch := b.cssaScratchAddr()
				b.emitLoadAddr("y", loc1)
				b.emitLoadAddr("x", scratch)
				b.emitCopy("x", "y", sz)
				b.emitLoadAddr("y", loc2)
				b.emitLoadAddr("x", loc1)
				b.emitCopy("x", "y", sz)
				b.emitLoadAddr("y", scratch)
				b.emitLoadAddr("x", loc2)
				b.emitCopy("x", "y", sz)
			}
			b.clobberAllRegs()

		case opt.StepSave:
			srcAddr := step.Src.(string)
			scratch := b.cssaScratchAddr()
			sz := step.Size
			if isReg(srcAddr) {
				b.buf.WriteString(fmt.Sprintf("\tst%s %s\n", srcAddr, scratch))
			} else if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", srcAddr))
				b.buf.WriteString(fmt.Sprintf("\tstb %s\n", scratch))
			} else if sz == 2 {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", srcAddr))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", scratch))
			} else {
				b.emitLoadAddr("y", srcAddr)
				b.emitLoadAddr("x", scratch)
				b.emitCopy("x", "y", sz)
			}
			b.clobberAllRegs()

		case opt.StepRestore:
			destAddr := step.Dest.(string)
			scratch := b.cssaScratchAddr()
			sz := step.Size
			if isReg(destAddr) {
				b.buf.WriteString(fmt.Sprintf("\tld%s %s\n", destAddr, scratch))
			} else if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", scratch))
				b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destAddr))
			} else if sz == 2 {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", scratch))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destAddr))
			} else {
				b.emitLoadAddr("y", scratch)
				b.emitLoadAddr("x", destAddr)
				b.emitCopy("x", "y", sz)
			}
			b.clobberAllRegs()
		}
	}
	b.clobberAllRegs()
}

func (b *Backend) isZeroVal(val ir.Value) bool {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.ZeroInit:
		return true
	case *ir.ConstByte:
		return v.Val == 0
	case *ir.ConstWord:
		return v.Val == 0
	}
	return false
}

func (b *Backend) isLeafFunc(f *ir.Function) bool {
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			switch instr.(type) {
			case *ir.Call, *ir.IndirectCall, *ir.BuiltinCall:
				return false
			}
		}
	}
	return true
}

func (b *Backend) invertCondOp(op string) string {
	switch op {
	case "beq":
		return "bne"
	case "bne":
		return "beq"
	case "blt":
		return "bge"
	case "ble":
		return "bgt"
	case "bgt":
		return "ble"
	case "bge":
		return "blt"
	case "blo":
		return "bhs"
	case "bls":
		return "bhi"
	case "bhi":
		return "bls"
	case "bhs":
		return "blo"
	case "lbeq":
		return "lbne"
	case "lbne":
		return "lbeq"
	case "lblt":
		return "lbge"
	case "lble":
		return "lbgt"
	case "lbgt":
		return "lble"
	case "lbge":
		return "lblt"
	case "lblo":
		return "lbhs"
	case "lbls":
		return "lbhi"
	case "lbhi":
		return "lbls"
	case "lbhs":
		return "lblo"
	default:
		log.Panicf("invertCondOp: unknown op %s", op)
		return ""
	}
}

func (b *Backend) hasPhiAssignments(from, to *ir.BasicBlock) bool {
	if to == nil {
		return false
	}
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					return true
				}
			}
		}
	}
	return false
}

func (b *Backend) emitBranchWithLayout(blk, trueBlk, falseBlk, nextBlk *ir.BasicBlock, condOp, invOp string) {
	trueHasPhis := b.hasPhiAssignments(blk, trueBlk)
	falseHasPhis := b.hasPhiAssignments(blk, falseBlk)

	if !b.NoBranchLayout {
		// Case A: true block is next in layout -> invert condition, jump to false block on inverted condition, fall through to true block
		if trueBlk == nextBlk {
			if !falseHasPhis {
				b.buf.WriteString(fmt.Sprintf("\t%s .L_%s_b%d\n", invOp, b.f.Name, falseBlk.ID))
				b.emitPhiAssignments(blk, trueBlk)
				return
			}
			lblFalse := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\t%s %s\n", invOp, lblFalse))
			b.emitPhiAssignments(blk, trueBlk)
			lblEnd := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tlbra %s\n", lblEnd))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblFalse))
			b.emitPhiAssignments(blk, falseBlk)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, falseBlk.ID))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblEnd))
			return
		}

		// Case B: false block is next in layout -> jump to true block on condition, fall through to false block
		if falseBlk == nextBlk {
			if !trueHasPhis {
				b.buf.WriteString(fmt.Sprintf("\t%s .L_%s_b%d\n", condOp, b.f.Name, trueBlk.ID))
				b.emitPhiAssignments(blk, falseBlk)
				return
			}
			lblTrue := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\t%s %s\n", condOp, lblTrue))
			b.emitPhiAssignments(blk, falseBlk)
			lblEnd := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tlbra %s\n", lblEnd))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblTrue))
			b.emitPhiAssignments(blk, trueBlk)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, trueBlk.ID))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblEnd))
			return
		}

		// Case C: neither is nextBlk
		if !trueHasPhis && !falseHasPhis {
			b.buf.WriteString(fmt.Sprintf("\t%s .L_%s_b%d\n", condOp, b.f.Name, trueBlk.ID))
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, falseBlk.ID))
			return
		}
	}

	lblTrue := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("\t%s %s\n", condOp, lblTrue))
	b.emitPhiAssignments(blk, falseBlk)
	b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, falseBlk.ID))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblTrue))
	b.emitPhiAssignments(blk, trueBlk)
	b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, trueBlk.ID))
}

func (b *Backend) emitTerminator(blk *ir.BasicBlock, term ir.Terminator, nextBlk *ir.BasicBlock) {
	switch t := term.(type) {
	case *ir.Jump:
		b.emitPhiAssignments(blk, t.Target)
		if b.NoBranchLayout || t.Target != nextBlk {
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, t.Target.ID))
		}

	case *ir.Branch:
		condVal := t.Condition
		if cmp2, ok := condVal.(*ir.Compare); ok && cmp2.Op == "neq" && b.fusedCompares[cmp2.GetID()] {
			if b.isZeroVal(cmp2.Right) {
				leftVal := cmp2.Left
				for {
					leftVal = b.resolveVal(leftVal)
					if c, ok := leftVal.(*ir.Cast); ok {
						leftVal = c.Operand
						continue
					}
					break
				}
				leftVal = b.resolveVal(leftVal)
				if innerCmp, ok := leftVal.(*ir.Compare); ok && b.fusedCompares[innerCmp.GetID()] {
					condVal = innerCmp
				}
			}
		}
		if cmp, ok := condVal.(*ir.Compare); ok && b.fusedCompares[cmp.GetID()] {
			leftVal := b.resolveVal(cmp.Left)
			rightVal := b.resolveVal(cmp.Right)
			leftSize := b.getValSize(leftVal)
			rightSize := b.getValSize(rightVal)

			if leftSize == 1 && rightSize == 1 {
				b.loadVal(cmp.Left)
				if c, ok := b.asConstByte(rightVal); ok && c == 0 {
					b.buf.WriteString("\ttstb\n")
				} else if b.canDirectEA(rightVal, 1) {
					b.buf.WriteString(fmt.Sprintf("\tcmpb %s\n", b.getRightEA(rightVal, 1)))
				} else {
					b.loadVal(cmp.Right)
					b.buf.WriteString("\tpshs b\n")
					b.loadVal(cmp.Left)
					b.buf.WriteString("\tcmpb ,s+\n")
				}
			} else {
				b.emitCompare16(cmp.Left, cmp.Right)
			}

			isInt := cmp.Left.Type().Equals(ir.TypeInt) || cmp.Right.Type().Equals(ir.TypeInt)
			var condOp string
			if !isInt && b.isZeroVal(rightVal) {
				switch cmp.Op {
				case "eq":
					condOp = "lbeq"
				case "neq":
					condOp = "lbne"
				case "gt":
					condOp = "lbne"
				case "lte":
					condOp = "lbeq"
				case "gte":
					b.emitPhiAssignments(blk, t.TrueBlock)
					if b.NoBranchLayout || t.TrueBlock != nextBlk {
						b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, t.TrueBlock.ID))
					}
					return
				case "lt":
					b.emitPhiAssignments(blk, t.FalseBlock)
					if b.NoBranchLayout || t.FalseBlock != nextBlk {
						b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", b.f.Name, t.FalseBlock.ID))
					}
					return
				default:
					log.Panicf("emitTerminator: unhandled unsigned cmp op against zero %s", cmp.Op)
				}
			} else {
				switch cmp.Op {
				case "eq":
					condOp = "lbeq"
				case "neq":
					condOp = "lbne"
				case "lt":
					if isInt {
						condOp = "lblt"
					} else {
						condOp = "lblo"
					}
				case "lte":
					if isInt {
						condOp = "lble"
					} else {
						condOp = "lbls"
					}
				case "gt":
					if isInt {
						condOp = "lbgt"
					} else {
						condOp = "lbhi"
					}
				case "gte":
					if isInt {
						condOp = "lbge"
					} else {
						condOp = "lbhs"
					}
				default:
					log.Panicf("emitTerminator: unknown cmp op %s", cmp.Op)
				}
			}

			invOp := b.invertCondOp(condOp)
			b.emitBranchWithLayout(blk, t.TrueBlock, t.FalseBlock, nextBlk, condOp, invOp)
			return
		}

		b.loadVal(t.Condition)
		b.buf.WriteString("\ttstb\n")
		b.emitBranchWithLayout(blk, t.TrueBlock, t.FalseBlock, nextBlk, "lbne", "lbeq")

	case *ir.Return:
		if t.Val != nil {
			b.buf.WriteString(fmt.Sprintf("\t; return %s\n", b.describeVal(t.Val)))
			retSize := b.getTypeSizeByType(t.Val.Type())
			if retSize == 1 {
				b.loadVal(t.Val)
			} else if retSize == 2 {
				b.loadVal(t.Val)
				if b.getValSize(t.Val) == 1 {
					b.buf.WriteString("\tclra\n")
				}
				b.buf.WriteString("\ttfr d,x\n")
			} else {
				b.emitLoadAddr("y", b.getAddrStr(t.Val))
				b.emitLoadAddr("x", b.retBufAddr())
				b.emitCopy("x", "y", retSize)
			}
		}
		if b.needsFP {
			if b.saveYFP {
				b.buf.WriteString("\tleas -2,u\t; restore frame and return\n\tpuls y\n\tpuls u,pc\n")
			} else {
				b.buf.WriteString("\tleas 0,u\t; restore frame and return\n\tpuls u,pc\n")
			}
		} else {
			if b.stackSize > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleas %d,s\t; free local frame\n", b.stackSize))
			}
			if len(b.calleeSaveRegs) > 0 {
				b.buf.WriteString(fmt.Sprintf("\tpuls %s,pc\t; restore regs and return\n", strings.Join(b.calleeSaveRegs, ",")))
			} else {
				b.buf.WriteString("\trts\t; return\n")
			}
		}
	default:
		log.Panicf("emitTerminator: unhandled %T", term)
	}
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()
	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))

	case *ir.ConstByte:
		if _, inReg := b.globalRegs[id]; inReg {
			b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", i.Val))
			b.storeResult(id)
		} else if b.localAddressTaken[id] {
			b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", i.Val))
			b.storeResult(id)
		}

	case *ir.ConstWord:
		if _, inReg := b.globalRegs[id]; inReg {
			b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", i.Val))
			b.storeResult(id)
		} else if b.localAddressTaken[id] {
			b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", i.Val))
			b.storeResult(id)
		}

	case *ir.Sizeof:
		if _, inReg := b.globalRegs[id]; inReg {
			sz := b.getTypeSizeByType(i.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", sz))
			b.storeResult(id)
		} else if b.localAddressTaken[id] {
			sz := b.getTypeSizeByType(i.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", sz))
			b.storeResult(id)
		}

	case *ir.Load:
		sz := b.getTypeSizeByType(i.Global.Typ)
		srcStr := b.getAddrStr(i.Global)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tldb %s\n", srcStr))
			b.storeResult(id)
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\n", srcStr))
			b.storeResult(id)
		} else {
			destStr := b.localAddr(id)
			b.emitLoadAddr("y", srcStr)
			b.emitLoadAddr("x", destStr)
			b.emitCopy("x", "y", sz)
			b.clobberAllRegs()
		}

	case *ir.Store:
		sz := b.getTypeSizeByType(i.Global.Typ)
		destStr := b.getAddrStr(i.Global)
		if sz == 1 {
			b.loadVal(i.Val)
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		} else if sz == 2 {
			b.loadVal(i.Val)
			if b.getValSize(i.Val) == 1 {
				b.buf.WriteString("\tclra\n")
				b.clobberD()
			}
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else {
			b.emitLoadAddr("x", destStr)
			b.emitLoadAddr("y", b.getAddrStr(i.Val))
			b.emitCopy("x", "y", sz)
			b.clobberAllRegs()
		}

	case *ir.ZeroInit:
		sz := b.getTypeSizeByType(i.Typ)
		destStr := b.localAddr(id)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tclr %s\n", destStr))
		} else if sz == 2 {
			b.buf.WriteString("\tclra\n\tclrb\n")
			b.storeResult(id)
		} else {
			b.emitLoadAddr("x", destStr)
			b.emitMemset0("x", sz)
			b.clobberAllRegs()
		}

	case *ir.ExtractElement:
		eltSize := b.getEltSizeUsingIrt(i.Array.Type())
		if eltSize == 1 {
			b.computeElementAddr("x", i.Array, i.Index, eltSize)
			b.buf.WriteString("\tldb ,x\n")
			b.storeResult(id)
		} else if eltSize == 2 {
			b.computeElementAddr("x", i.Array, i.Index, eltSize)
			b.buf.WriteString("\tldd ,x\n")
			b.storeResult(id)
		} else {
			destStr := b.localAddr(id)
			b.computeElementAddr("y", i.Array, i.Index, eltSize)
			b.emitLoadAddr("x", destStr)
			b.emitCopy("x", "y", eltSize)
			b.clobberAllRegs()
		}

	case *ir.InsertElement:
		arrSize := b.getTypeSizeByType(i.Array.Type())
		eltSize := b.getEltSizeUsingIrt(i.Array.Type())
		destStr := b.localAddr(id)
		b.emitLoadAddr("x", destStr)
		b.emitLoadAddr("y", b.getAddrStr(i.Array))
		b.emitCopy("x", "y", arrSize)
		b.computeElementAddr("x", i, i.Index, eltSize)
		if eltSize == 1 {
			b.loadVal(i.Val)
			b.buf.WriteString("\tstb ,x\n")
		} else if eltSize == 2 {
			b.loadVal(i.Val)
			b.buf.WriteString("\tstd ,x\n")
		} else {
			b.emitLoadAddr("y", b.getAddrStr(i.Val))
			b.emitCopy("x", "y", eltSize)
		}
		b.clobberAllRegs()

	case *ir.ExtractField:
		structType := i.Struct.Type()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		srcStr := b.getAddrStr(i.Struct)
		if fieldSize == 1 {
			if byteOffset == 0 {
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", srcStr))
			} else {
				b.emitLoadAddr("x", srcStr)
				b.buf.WriteString(fmt.Sprintf("\tldb %d,x\n", byteOffset))
			}
			b.storeResult(id)
		} else if fieldSize == 2 {
			if byteOffset == 0 {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", srcStr))
			} else {
				b.emitLoadAddr("x", srcStr)
				b.buf.WriteString(fmt.Sprintf("\tldd %d,x\n", byteOffset))
			}
			b.storeResult(id)
		} else {
			destStr := b.localAddr(id)
			b.emitLoadAddr("x", destStr)
			b.emitLoadAddr("y", srcStr)
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
			}
			b.emitCopy("x", "y", fieldSize)
			b.clobberAllRegs()
		}

	case *ir.InsertField:
		structType := i.Struct.Type()
		structSize := b.getTypeSizeByType(structType)
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		destStr := b.localAddr(id)
		b.emitLoadAddr("x", destStr)
		b.emitLoadAddr("y", b.getAddrStr(i.Struct))
		b.emitCopy("x", "y", structSize)
		if fieldSize == 1 {
			b.loadVal(i.Val)
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", offsetAddrStr(destStr, byteOffset)))
		} else if fieldSize == 2 {
			b.loadVal(i.Val)
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", offsetAddrStr(destStr, byteOffset)))
		} else {
			b.emitLoadAddr("x", destStr)
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
			b.emitLoadAddr("y", b.getAddrStr(i.Val))
			b.emitCopy("x", "y", fieldSize)
		}

	case *ir.AddressOfGlobal:
		if _, inReg := b.globalRegs[id]; inReg {
			if b.globalsAtY {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,y\n\ttfr x,d\n", b.globalOffsets[i.Global.Name]))
			} else if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax v_%s,pcr\n\ttfr x,d\n", i.Global.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #v_%s\n", i.Global.Name))
			}
			b.storeResult(id)
		} else if b.localAddressTaken[id] {
			if b.globalsAtY {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,y\n\ttfr x,d\n", b.globalOffsets[i.Global.Name]))
			} else if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax v_%s,pcr\n\ttfr x,d\n", i.Global.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #v_%s\n", i.Global.Name))
			}
			b.storeResult(id)
		}

	case *ir.AddressOfFunc:
		if _, inReg := b.globalRegs[id]; inReg {
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n\ttfr x,d\n", i.Func.EmitName()))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%s\n", i.Func.EmitName()))
			}
			b.storeResult(id)
		} else if b.localAddressTaken[id] {
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n\ttfr x,d\n", i.Func.EmitName()))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%s\n", i.Func.EmitName()))
			}
			b.storeResult(id)
		}

	case *ir.AddressOfLocal:
		if b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[id] {
			break // Synthetic address: never materialized in machine code!
		}
		targetStr := b.getAddrStr(i.Local)
		b.emitLoadAddr("x", targetStr)
		b.buf.WriteString("\ttfr x,d\n")
		b.storeResult(id)

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal16("x", i.Ptr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
		}
		b.buf.WriteString("\ttfr x,d\n")
		b.storeResult(id)

	case *ir.AddressOfElement:
		eltSize := b.getEltSizeUsingIrt(i.ArrayPtr.Type())
		b.loadVal16("x", i.ArrayPtr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
		} else if cIdx, ok := i.Index.(*ir.ConstByte); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index) // in D
			if b.getTypeSizeByType(i.Index.Type()) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			if eltSize == 1 {
				b.buf.WriteString("\tleax d,x\n")
			} else if eltSize == 2 {
				b.buf.WriteString("\taslb\n\trola\n\tleax d,x\n")
			} else {
				b.buf.WriteString("\tpshs x\n")
				b.buf.WriteString(fmt.Sprintf("\tldx #%d\n", eltSize))
				if b.InlineMul16 {
					b.emitInlineMul16()
				} else {
					b.callHelper("__mul16")
				}
				b.buf.WriteString("\tpuls x\n\tleax d,x\n")
			}
		}
		b.buf.WriteString("\ttfr x,d\n")
		b.storeResult(id)

	case *ir.ExtractFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal16("x", i.Ptr)
			b.buf.WriteString(fmt.Sprintf("\tldb %d,x\n", byteOffset))
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.loadVal16("x", i.Ptr)
			b.buf.WriteString(fmt.Sprintf("\tldd %d,x\n", byteOffset))
			b.storeResult(id)
		} else {
			destStr := b.localAddr(id)
			b.loadVal16("y", i.Ptr)
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
			}
			b.emitLoadAddr("x", destStr)
			b.emitCopy("x", "y", fieldSize)
			b.clobberAllRegs()
		}

	case *ir.InsertFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Val)
			b.loadVal16("x", i.Ptr)
			b.buf.WriteString(fmt.Sprintf("\tstb %d,x\n", byteOffset))
		} else if fieldSize == 2 {
			b.loadVal(i.Val)
			if b.getValSize(i.Val) == 1 {
				b.buf.WriteString("\tclra\n")
			}
			b.loadVal16("x", i.Ptr)
			b.buf.WriteString(fmt.Sprintf("\tstd %d,x\n", byteOffset))
		} else {
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
			b.emitLoadAddr("y", b.getAddrStr(i.Val))
			b.emitCopy("x", "y", fieldSize)
		}
		b.clobberAllRegs()

	case *ir.LoadPtr:
		sz := b.getTypeSizeByType(i.Typ)
		if sz <= 2 {
			ptrVal := b.resolveVal(i.Ptr)
			// Check if pointer is a non-escaping local held in a physical register
			if aol, ok := ptrVal.(*ir.AddressOfLocal); ok && b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[aol.GetID()] {
				if locInst, ok := aol.Local.(ir.Instruction); ok && len(b.globalRegs) > 0 {
					if srcReg, ok := b.globalRegs[locInst.GetID()]; ok {
						if srcReg != "d" {
							b.buf.WriteString(fmt.Sprintf("\ttfr %s,d\n", srcReg))
						}
						b.storeResult(id)
						break
					}
				}
			}

			// Check if pointer has a direct effective address (stack slot or global)
			if directEA, ok := b.getDirectEA(ptrVal); ok {
				if sz == 1 {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n", directEA))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tldd %s\n", directEA))
				}
				b.storeResult(id)
				break
			}

			ptrReg := "x"
			if ptrInst, ok := i.Ptr.(ir.Instruction); ok && len(b.globalRegs) > 0 {
				if r, ok := b.globalRegs[ptrInst.GetID()]; ok {
					ptrReg = r
				}
			}
			if ptrReg == "x" {
				b.loadVal16("x", i.Ptr)
			}
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb ,%s\n", ptrReg))
				b.storeResult(id)
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd ,%s\n", ptrReg))
				b.storeResult(id)
			}
		} else {
			destStr := b.localAddr(id)
			b.loadVal16("y", i.Ptr)
			b.emitLoadAddr("x", destStr)
			b.emitCopy("x", "y", sz)
			b.clobberAllRegs()
		}

	case *ir.StorePtr:
		sz := b.getTypeSizeByType(i.Val.Type())
		if sz <= 2 {
			ptrVal := b.resolveVal(i.Ptr)
			// Check if pointer is a non-escaping local held in a physical register
			if aol, ok := ptrVal.(*ir.AddressOfLocal); ok && b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[aol.GetID()] {
				if locInst, ok := aol.Local.(ir.Instruction); ok && len(b.globalRegs) > 0 {
					if destReg, ok := b.globalRegs[locInst.GetID()]; ok {
						b.loadVal16(destReg, i.Val)
						break
					}
				}
			}

			// Check if pointer has a direct effective address (stack slot or global)
			if directEA, ok := b.getDirectEA(ptrVal); ok {
				b.loadVal(i.Val)
				if sz == 1 {
					b.buf.WriteString(fmt.Sprintf("\tstb %s\n", directEA))
				} else {
					if b.getValSize(i.Val) == 1 {
						b.buf.WriteString("\tclra\n")
					}
					b.buf.WriteString(fmt.Sprintf("\tstd %s\n", directEA))
				}
				break
			}

			ptrReg := "x"
			if ptrInst, ok := i.Ptr.(ir.Instruction); ok && len(b.globalRegs) > 0 {
				if r, ok := b.globalRegs[ptrInst.GetID()]; ok {
					ptrReg = r
				}
			}
			if sz == 1 {
				b.loadVal(i.Val)
			} else {
				b.loadVal(i.Val)
				if b.getValSize(i.Val) == 1 {
					b.buf.WriteString("\tclra\n")
				}
			}
			if ptrReg == "x" {
				b.loadVal16("x", i.Ptr)
			}
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tstb ,%s\n", ptrReg))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tstd ,%s\n", ptrReg))
			}
		} else {
			b.loadVal16("x", i.Ptr)
			b.emitLoadAddr("y", b.getAddrStr(i.Val))
			b.emitCopy("x", "y", sz)
		}

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.Call:
		b.emitCallInstr(i)

	case *ir.IndirectCall:
		b.emitIndirectCall(i)

	case *ir.BuiltinCall:
		b.emitBuiltinCall(i)

	case *ir.SetJmp:
		b.emitSetJmp(i)

	case *ir.LongJmp:
		b.emitLongJmp(i)

	case *ir.Cast:
		b.emitCast(i)

	default:
		log.Panicf("emitInstr: unknown instruction %T", instr)
	}
}

func (b *Backend) emitFunc(f *ir.Function) {
	if len(f.Blocks) == 0 {
		return
	}
	b.f = f
	b.stackSize = 0
	b.pushedBytes = 0
	b.slots = make(map[int]int)
	b.slotSizes = make(map[int]int)
	b.paramOffsets = make(map[string]int)
	b.jmpSlots = make(map[int]int)
	b.fusedCompares = make(map[int]bool)
	b.escapeRes = opt.AnalyzeEscape(f)
	b.addressTaken = b.escapeRes.EscapingLocals
	b.uses = b.countUses(f)
	b.localAddressTaken = make(map[int]bool)
	b.instrs = make(map[int]ir.Instruction)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			b.instrs[instr.GetID()] = instr
			if aol, ok := instr.(*ir.AddressOfLocal); ok {
				if loc, ok := aol.Local.(ir.Instruction); ok {
					b.localAddressTaken[loc.GetID()] = true
				}
			}
		}
		if blk.Terminator != nil {
			b.instrs[blk.Terminator.GetID()] = blk.Terminator
		}
	}

	b.globalRegs = b.AllocateRegisters(f)

	if !b.NoFusedCompares {
		uses := b.uses
		for _, blk := range f.Blocks {
			if br, ok := blk.Terminator.(*ir.Branch); ok {
				condVal := br.Condition
				if cmp2, ok := condVal.(*ir.Compare); ok && cmp2.Op == "neq" && uses[cmp2.GetID()] == 1 {
					if b.isZeroVal(cmp2.Right) {
						leftVal := cmp2.Left
						var casts []*ir.Cast
						for {
							if c, ok := leftVal.(*ir.Cast); ok {
								casts = append(casts, c)
								leftVal = c.Operand
								continue
							}
							break
						}
						if innerCmp, ok := leftVal.(*ir.Compare); ok && uses[innerCmp.GetID()] == 1 {
							condVal = innerCmp
							b.fusedCompares[cmp2.GetID()] = true
							if zi, ok := cmp2.Right.(*ir.ZeroInit); ok {
								b.fusedCompares[zi.GetID()] = true
							}
							for _, c := range casts {
								b.fusedCompares[c.GetID()] = true
							}
						}
					}
				}
				if cmp, ok := condVal.(*ir.Compare); ok && uses[cmp.GetID()] == 1 {
					var lastInstr ir.Instruction
					for j := len(blk.Instructions) - 1; j >= 0; j-- {
						if _, isMarker := blk.Instructions[j].(*ir.SourceMarker); !isMarker {
							id := blk.Instructions[j].GetID()
							if id == cmp.GetID() || b.fusedCompares[id] {
								lastInstr = blk.Instructions[j]
								break
							}
						}
					}
					if lastInstr != nil && (lastInstr.GetID() == cmp.GetID() || b.fusedCompares[lastInstr.GetID()]) {
						b.fusedCompares[cmp.GetID()] = true
					}
				}
			}
		}
	}

	paramOffset := 0
	for _, p := range f.Parameters {
		sz := b.getTypeSizeByType(p.Typ)
		b.paramOffsets[p.Name] = paramOffset
		paramOffset += align(sz)
	}

	retSize := b.getTypeSizeByType(f.ReturnType)
	b.retSlot = -1
	if retSize > 2 {
		b.retSlot = paramOffset
		paramOffset += align(retSize)
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if b.fusedCompares[instr.GetID()] {
				continue
			}

			if setjmp, ok := instr.(*ir.SetJmp); ok {
				b.jmpSlots[setjmp.GetID()] = b.allocateRawSlot(10)
			}
			if aol, ok := instr.(*ir.AddressOfLocal); ok && b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[aol.GetID()] {
				continue
			}
			if !b.localAddressTaken[instr.GetID()] {
				switch instr.(type) {
				case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof, *ir.AddressOfGlobal, *ir.AddressOfFunc:
					continue
				}
			}
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				sz := b.getTypeSizeByType(instr.Type())
				b.allocateSlot(sz, instr.GetID())
			}
		}
	}

	needsScratch := false
	for _, blk := range f.Blocks {
		for _, succ := range blk.Successors {
			var moves []opt.ParallelMove
			for _, instr := range succ.Instructions {
				if phi, ok := instr.(*ir.Phi); ok {
					for _, edge := range phi.Edges {
						if edge.Block == blk {
							sz := b.getTypeSizeByType(phi.Typ)
							var destAddr string
							if len(b.globalRegs) > 0 {
								if reg, ok := b.globalRegs[phi.GetID()]; ok {
									destAddr = reg
								}
							}
							if destAddr == "" {
								canon := b.resolveSlot(phi.GetID())
								if _, ok := b.slots[canon]; !ok {
									if _, ok := b.slots[phi.GetID()]; !ok {
										continue
									}
								}
								destAddr = b.localAddr(phi.GetID())
							}
							srcAddr := b.getPhiSrcLoc(edge.Value)
							moves = append(moves, opt.ParallelMove{
								Dest: destAddr,
								Src:  srcAddr,
								Size: sz,
							})
						}
					}
				}
			}
			if len(moves) >= 3 {
				steps := opt.SequentializeParallelCopies(moves)
				for _, s := range steps {
					if s.Kind == opt.StepSave {
						needsScratch = true
						break
					}
				}
			}
			if needsScratch {
				break
			}
		}
		if needsScratch {
			break
		}
	}
	if needsScratch {
		b.cssaScratchOffset = b.allocateRawSlot(2)
	} else {
		b.cssaScratchOffset = -1
	}

	usedRegs := make(map[string]bool)
	for _, reg := range b.globalRegs {
		usedRegs[reg] = true
	}
	saveU := usedRegs["u"] && !b.useFramePointer
	saveY := usedRegs["y"] && !b.globalsAtY

	if b.NoLeafOpt {
		b.needsFP = b.useFramePointer
	} else {
		b.needsFP = b.useFramePointer && (b.stackSize > 0 || len(f.Parameters) > 0 || saveY)
	}

	b.saveYFP = saveY && b.needsFP
	b.calleeSaveRegs = nil
	if saveU && !b.needsFP {
		b.calleeSaveRegs = append(b.calleeSaveRegs, "u")
	}
	if saveY && !b.needsFP {
		b.calleeSaveRegs = append(b.calleeSaveRegs, "y")
	}
	b.savedRegBytes = len(b.calleeSaveRegs) * 2

	b.buf.WriteString("\n")
	b.emitFunctionHeader(f)
	b.buf.WriteString(fmt.Sprintf("%s:\n", f.EmitName()))

	if b.needsFP {
		b.buf.WriteString("\tpshs u\n\ttfr s,u\n")
		if b.saveYFP {
			b.buf.WriteString("\tpshs y\n")
		}
	} else if len(b.calleeSaveRegs) > 0 {
		b.buf.WriteString(fmt.Sprintf("\tpshs %s\n", strings.Join(b.calleeSaveRegs, ",")))
	}
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\t; allocate %d bytes local frame\n", b.stackSize, b.stackSize))
	}

	for idx, blk := range f.Blocks {
		var nextBlk *ir.BasicBlock
		if idx+1 < len(f.Blocks) {
			nextBlk = f.Blocks[idx+1]
		}
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))
		b.clobberAllRegs()
		for _, instr := range blk.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if b.fusedCompares[instr.GetID()] {
				continue
			}
			b.curInstr = instr
			b.emitInstr(instr)
		}
		b.curInstr = nil
		b.clobberAllRegs()
		if blk.Terminator != nil {
			b.emitTerminator(blk, blk.Terminator, nextBlk)
		}
	}
}

func (b *Backend) allocateRawSlot(sz int) int {
	aligned := align(sz)
	offset := b.stackSize
	b.stackSize += aligned
	return offset
}

func (b *Backend) allocateSlot(sz int, id int) int {
	canon := b.resolveSlot(id)
	if offset, ok := b.slots[canon]; ok {
		b.slots[id] = offset
		b.slotSizes[id] = sz
		return offset
	}
	offset := b.allocateRawSlot(sz)
	b.slots[canon] = offset
	b.slotSizes[canon] = sz
	if canon != id {
		b.slots[id] = offset
		b.slotSizes[id] = sz
	}
	return offset
}

func (b *Backend) emitInlineMul16() {
	b.clobberAllRegs()
	b.buf.WriteString("\tpshs d,x\n" +
		"\tlda 1,s\n" +
		"\tldb 3,s\n" +
		"\tmul\n" +
		"\ttfr d,x\n" +
		"\tlda 0,s\n" +
		"\tldb 3,s\n" +
		"\tmul\n" +
		"\ttfr b,a\n" +
		"\tclrb\n" +
		"\tleax d,x\n" +
		"\tlda 1,s\n" +
		"\tldb 2,s\n" +
		"\tmul\n" +
		"\ttfr b,a\n" +
		"\tclrb\n" +
		"\tleax d,x\n" +
		"\ttfr x,d\n" +
		"\tleas 4,s\n")
}

func (b *Backend) emitInlineDivMod16(isDiv bool) {
	b.clobberAllRegs()
	if b.helpersEmitted == nil {
		b.helpersEmitted = make(map[string]bool)
	}
	b.helpersEmitted["__div0_error"] = true
	lblLoop := b.nextLabel()
	lblNoSub := b.nextLabel()
	lblNonZero := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("\tcmpd #0\n"+
		"\tbne %s\n"+
		"\tjmp __div0_error\n"+
		"%s:\n"+
		"\tpshs u,d\n"+
		"\tldu #16\n"+
		"\tclra\n\tclrb\n"+
		"%s:\n"+
		"\texg d,x\n"+
		"\taslb\n\trola\n"+
		"\texg d,x\n"+
		"\trolb\n\trola\n"+
		"\tcmpd ,s\n"+
		"\tblo %s\n"+
		"\tsubd ,s\n"+
		"\tleax 1,x\n"+
		"%s:\n"+
		"\tleau -1,u\n"+
		"\tcmpu #0\n"+
		"\tbne %s\n"+
		"\tleas 2,s\n"+
		"\tpuls u\n",
		lblNonZero, lblNonZero, lblLoop, lblNoSub, lblNoSub, lblLoop))
	if isDiv {
		b.buf.WriteString("\ttfr x,d\n")
	}
}

func (b *Backend) emitHelpers() {
	if b.helpersEmitted["__mul16"] {
		b.helpersBuf.WriteString(`
__mul16:
	pshs d,x
	lda 1,s
	ldb 3,s
	mul
	tfr d,x
	lda 0,s
	ldb 3,s
	mul
	tfr b,a
	clrb
	leax d,x
	lda 1,s
	ldb 2,s
	mul
	tfr b,a
	clrb
	leax d,x
	tfr x,d
	leas 4,s
	rts
`)
	}

	if b.helpersEmitted["__div16"] || b.helpersEmitted["__mod16"] || b.helpersEmitted["__div0_error"] {
		b.fmtCount++
		lblDiv0Msg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"division by zero\"\n", lblDiv0Msg))

		b.fmtCount++
		lblPanicMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*PANIC* %%s\\n\"\n", lblPanicMsg))

		b.fmtCount++
		lblAbortMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n", lblAbortMsg))

		lblDiv0Abort := b.nextLabel()

		var loadDiv0Msg, loadPanicMsg, loadAbortMsg string
		if b.picMode {
			loadDiv0Msg = fmt.Sprintf("\tleax %s,pcr\n", lblDiv0Msg)
			loadPanicMsg = fmt.Sprintf("\tleax %s,pcr\n", lblPanicMsg)
			loadAbortMsg = fmt.Sprintf("\tleax %s,pcr\n", lblAbortMsg)
		} else {
			loadDiv0Msg = fmt.Sprintf("\tldx #%s\n", lblDiv0Msg)
			loadPanicMsg = fmt.Sprintf("\tldx #%s\n", lblPanicMsg)
			loadAbortMsg = fmt.Sprintf("\tldx #%s\n", lblAbortMsg)
		}

		callPrintf := "\tjsr _printf\n"
		if b.picMode {
			callPrintf = "\tlbsr _printf\n"
		}

		b.helpersBuf.WriteString(fmt.Sprintf(`
__divmod16:
	cmpd #0
	beq __div0_error
	pshs u,d
	ldu #16
	clra
	clrb
.L_divloop:
	exg d,x
	aslb
	rola
	exg d,x
	rolb
	rola
	cmpd ,s
	blo .L_divnosub
	subd ,s
	leax 1,x
.L_divnosub:
	leau -1,u
	cmpu #0
	bne .L_divloop
	leas 2,s
	puls u,pc
__div0_error:
%s`+
			fmt.Sprintf("\tstx %s\n", b.panicAddr())+
			"\tpshs x\n"+
			loadPanicMsg+
			"\tpshs x\n"+
			callPrintf+
			"\tleas 4,s\n"+
			fmt.Sprintf("\tldx %s\n", b.jmpChainAddr())+
			"\tcmpx #0\n"+
			fmt.Sprintf("\tbeq %s\n", lblDiv0Abort)+
			"\tclra\n\tldb #1\n"+
			"\tldy 8,x\n\tldu 6,x\n\tlds 4,x\n\tjmp [2,x]\n"+
			fmt.Sprintf("%s:\n", lblDiv0Abort)+
			loadAbortMsg+
			"\tpshs x\n"+
			callPrintf+
			"\tleas 2,s\n\tldx #1\n\tjmp __exit\n"+
			`
__div16:
	lbsr __divmod16
	tfr x,d
	rts

__mod16:
	lbsr __divmod16
	rts
`, loadDiv0Msg))
	}

	if b.helpersEmitted["__memcpy"] {
		b.helpersBuf.WriteString(`
__memcpy:
	pshs u
	tfr d,u
.L_cpy_loop:
	lda ,y+
	sta ,x+
	leau -1,u
	cmpu #0
	bne .L_cpy_loop
	puls u,pc
`)
	}

	if b.helpersEmitted["__memset0"] {
		b.helpersBuf.WriteString(`
__memset0:
	pshs u
	tfr d,u
	clra
.L_set_loop:
	sta ,x+
	leau -1,u
	cmpu #0
	bne .L_set_loop
	puls u,pc
`)
	}
}

func (b *Backend) Generate(program *ir.Program) string {
	b.program = program
	b.buf.WriteString("\tpragma cescapes\n")

	b.globalOffsets = make(map[string]int)
	if !b.globalsAtY && len(program.Globals) > 0 {
		addr := *GLOBAL_VAR_OFFSET
		for _, g := range program.Globals {
			if g.IsInit {
				b.dataBuf.WriteString(fmt.Sprintf("*** global var init: name=%q type=%q init=%#v\n", g.Name, g.Typ.Name, g.InitString))
				b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
				if g.InitVal != nil {
					b.emitData(g.InitVal)
				} else {
					for i := 0; i < len(g.InitString); i++ {
						x := g.InitString[i]
						c := byte('~')
						if ' ' <= x && x < '~' {
							c = x
						}
						b.dataBuf.WriteString(fmt.Sprintf("\tfcb %d ; [%d] <%c>\n", g.InitString[i], i, c))
					}
				}
			} else {
				size := b.getTypeSizeByType(g.Typ)
				b.dataBuf.WriteString(fmt.Sprintf("v_%s\tequ\t%d\t; size=%d type=%q [no init]\n\n", g.Name, addr, size, g.Typ.Name))
				addr += size
			}
		}
	} else if b.globalsAtY {
		offset := *GLOBAL_VAR_OFFSET
		for _, g := range program.Globals {
			b.globalOffsets[g.Name] = offset
			if g.IsInit {
				b.dataBuf.WriteString(fmt.Sprintf("\torg %d\n", offset))
				b.dataBuf.WriteString(fmt.Sprintf("*** global var init: name=%q type=%q init=%#v\n", g.Name, g.Typ.Name, g.InitString))
				b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
				if g.InitVal != nil {
					b.emitData(g.InitVal)
					offset += b.getTypeSizeByType(g.Typ)
				} else {
					for i := 0; i < len(g.InitString); i++ {
						x := g.InitString[i]
						c := byte('~')
						if ' ' <= x && x < '~' {
							c = x
						}
						b.dataBuf.WriteString(fmt.Sprintf("\tfcb %d ; [%d] <%c>\n", g.InitString[i], i, c))
					}
					offset += len(g.InitString)
				}
			} else {
				size := b.getTypeSizeByType(g.Typ)
				offset += size
			}
		}
	}

	usesPanic := false
	for _, f := range program.Functions {
		for _, blk := range f.Blocks {
			for _, i := range blk.Instructions {
				switch instr := i.(type) {
				case *ir.SetJmp, *ir.LongJmp:
					usesPanic = true
				case *ir.BuiltinCall:
					if instr.Name == "panic" || instr.Name == "_propagate_panic_" || instr.Name == "_unlink_jmp_" {
						usesPanic = true
					}
				}
			}
		}
	}

	b.buf.WriteString("_main:\n")
	if b.globalsAtY {
		b.buf.WriteString("\tldy #0\n")
	}

	if usesPanic {
		b.buf.WriteString("\tleas -10,s\t; Allocate 10 bytes for jumper_main\n")
		b.buf.WriteString("\tldd #0\n")
		b.buf.WriteString("\tstd 0,s\t; jumper_main.prev = NULL\n")

		b.buf.WriteString("\tleax ,s\n")
		b.buf.WriteString(fmt.Sprintf("\tstx %s\n", b.jmpChainAddr()))

		b.buf.WriteString("\tsts 4,x\n\tstu 6,x\n\tsty 8,x\n")
		lblNext := b.nextLabel()
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleau %s,pcr\n\tstu 2,x\n\tldu 6,x\n", lblNext))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd #%s\n\tstd 2,x\n", lblNext))
		}

		b.buf.WriteString("\tclra\n\tclrb\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))
		if b.globalsAtY {
			b.buf.WriteString("\tldy #0\n")
		}
		b.buf.WriteString("\tcmpd #0\n")
		lblCallMain := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblCallMain))

		b.fmtCount++
		lblUncaught := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** UNCAUGHT_PANIC\\n\"\n", lblUncaught))
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblUncaught))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblUncaught))
		}
		b.buf.WriteString("\tpshs x\n")
		b.emitCall("_printf")
		b.buf.WriteString("\tleas 2,s\n")

		b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.panicAddr()))
		b.buf.WriteString("\tcmpd #0\n")
		lblAbort := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblAbort))

		b.fmtCount++
		lblPanicMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"*** %%s\\n\"\n", lblPanicMsg))

		b.buf.WriteString("\tpshs d\n")
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblPanicMsg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblPanicMsg))
		}
		b.buf.WriteString("\tpshs x\n")
		b.emitCall("_printf")
		b.buf.WriteString("\tleas 4,s\n")

		b.buf.WriteString(fmt.Sprintf("%s:\n", lblAbort))
		b.buf.WriteString("\tldx #1\n\tjmp __exit\n")

		b.buf.WriteString(fmt.Sprintf("%s:\n", lblCallMain))
	}

	b.emitCall("f_main__main")

	if usesPanic {
		b.buf.WriteString("\tleas 10,s\n")
	}
	b.buf.WriteString("\tldd #0\n\tldx #0\n\trts\n")

	for _, f := range program.Functions {
		b.emitFunc(f)
	}

	b.emitHelpers()

	rawCode := b.buf.String() + "\n" + b.rodataBuf.String() + "\n" + b.helpersBuf.String() + "\n" + b.dataBuf.String()
	return peepholeOptimize(rawCode)
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.dataBuf.WriteString(fmt.Sprintf("\tfcb %d\n", v.Val))
	case *ir.ConstWord:
		b.dataBuf.WriteString(fmt.Sprintf("\tfdb %d\n", v.Val))
	case *ir.AddressOfGlobal:
		b.dataBuf.WriteString(fmt.Sprintf("\tfdb v_%s\n", v.Global.Name))
	case *ir.ConstStruct:
		for _, f := range v.Fields {
			b.emitData(f)
		}
	case *ir.ConstArray:
		for _, el := range v.Elements {
			b.emitData(el)
		}
	default:
		log.Panicf("unsupported init value type %T", val)
	}
}

func Assert(pred bool) {
	if !pred {
		panic("Assertion Failed")
	}
}
