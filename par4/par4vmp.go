package par4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VMError represents a runtime error during bytecode execution.
type VMError struct {
	Message  string
	PC       int
	FuncName string
}

func (e *VMError) Error() string {
	ctx := ""
	if e.FuncName != "" {
		ctx += fmt.Sprintf(" in function '%s'", e.FuncName)
	}
	if e.PC >= 0 {
		ctx += fmt.Sprintf(" at PC 0x%04X", e.PC)
	}
	return fmt.Sprintf("VM Error%s: %s", ctx, e.Message)
}

// FunctionMetadata holds parsed function header info from .p4p.
type FunctionMetadata struct {
	Index          int
	Name           string
	IsEntry        bool
	ArgCount       int
	LocalCount     int
	FrameSize      int
	CodeOffset     int
	CodeSize       int
	VarTableOffset int
	VarSizes       []int
}

// CallFrame represents an active function execution frame.
type CallFrame struct {
	Func      *FunctionMetadata
	ReturnPC  int
	FrameAddr int
}

// HeapManager implements a first-fit dynamic fixed-size buffer allocator.
type HeapManager struct {
	memory      []byte
	heapStart   int
	heapEnd     int
	allocations map[int]int // addr -> size
}

func NewHeapManager(memory []byte, start, end int) *HeapManager {
	start = (start + 3) &^ 3 // align 4 bytes
	h := &HeapManager{
		memory:      memory,
		heapStart:   start,
		heapEnd:     end,
		allocations: make(map[int]int),
	}
	h.initHeap()
	return h
}

func (h *HeapManager) initHeap() {
	totalSize := h.heapEnd - h.heapStart
	if totalSize >= 4 {
		binary.BigEndian.PutUint16(h.memory[h.heapStart:], uint16(totalSize))
		binary.BigEndian.PutUint16(h.memory[h.heapStart+2:], 1) // is_free = 1
	}
}

func (h *HeapManager) Alloc(size int) int {
	if size <= 0 {
		size = 2
	}
	reqSize := ((size + 1) &^ 1) + 4
	curr := h.heapStart

	for curr+4 <= h.heapEnd {
		blkSize := int(binary.BigEndian.Uint16(h.memory[curr:]))
		isFree := binary.BigEndian.Uint16(h.memory[curr+2:])
		if blkSize == 0 || blkSize > (h.heapEnd-curr) {
			break
		}
		if isFree == 1 && blkSize >= reqSize {
			rem := blkSize - reqSize
			if rem >= 8 {
				binary.BigEndian.PutUint16(h.memory[curr:], uint16(reqSize))
				binary.BigEndian.PutUint16(h.memory[curr+2:], 0) // used
				binary.BigEndian.PutUint16(h.memory[curr+reqSize:], uint16(rem))
				binary.BigEndian.PutUint16(h.memory[curr+reqSize+2:], 1) // free
			} else {
				binary.BigEndian.PutUint16(h.memory[curr+2:], 0) // used
			}
			dataAddr := curr + 4
			for b := 0; b < size; b++ {
				h.memory[dataAddr+b] = 0
			}
			h.allocations[dataAddr] = blkSize - 4
			return dataAddr
		}
		curr += blkSize
	}
	return 0 // OOM
}

func (h *HeapManager) Free(dataAddr int) {
	if _, ok := h.allocations[dataAddr]; !ok {
		return
	}
	delete(h.allocations, dataAddr)
	blkAddr := dataAddr - 4
	binary.BigEndian.PutUint16(h.memory[blkAddr+2:], 1) // is_free = 1

	// Coalesce forward
	curr := h.heapStart
	for curr+4 <= h.heapEnd {
		blkSize := int(binary.BigEndian.Uint16(h.memory[curr:]))
		isFree := binary.BigEndian.Uint16(h.memory[curr+2:])
		if blkSize == 0 || blkSize > (h.heapEnd-curr) {
			break
		}
		next := curr + blkSize
		if isFree == 1 && next+4 <= h.heapEnd {
			nextFree := binary.BigEndian.Uint16(h.memory[next+2:])
			if nextFree == 1 {
				nextSize := int(binary.BigEndian.Uint16(h.memory[next:]))
				binary.BigEndian.PutUint16(h.memory[curr:], uint16(blkSize+nextSize))
				continue
			}
		}
		curr += blkSize
	}
}

// VM is the Par4 virtual machine interpreter (par4vmp).
type VM struct {
	Memory           []byte
	Stack            []uint16
	CallStack        []*CallFrame
	CurrentFrame     *CallFrame
	PC               int
	Running          bool
	ExitCode         int
	InstructionCount int
	MaxInstructions  int

	// Flags
	Verbose    bool
	DumpStack  bool
	StepMode   bool

	// File format sections
	Functions       []*FunctionMetadata
	EntryFuncIndex  int
	StringPoolBase  int
	StringPoolSize  int
	GlobalsBase     int
	GlobalSizes     []int
	CodeBase        int
	CodeSize        int

	Heap    *HeapManager
	FrameSP int

	// OS / File Table
	Files      map[byte]*os.File
	NextFileID byte

	// Dynamic maps / regexes
	RegexPatterns []*regexp.Regexp
	RegexMatches  map[int][]string
	NextMatchID   int
	Maps          map[int]map[string]int
	NextMapID     int

	// I/O streams
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewVM creates a new VM initialized with the given bytecode binary data.
func NewVM(bytecode []byte) (*VM, error) {
	vm := &VM{
		Memory:        make([]byte, 65536),
		Stack:         make([]uint16, 0, 1024),
		CallStack:     make([]*CallFrame, 0, 128),
		Files:         make(map[byte]*os.File),
		NextFileID:    3,
		RegexMatches:  make(map[int][]string),
		NextMatchID:   1,
		Maps:          make(map[int]map[string]int),
		NextMapID:     1,
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
	}

	// Initialize standard file paths
	vm.Files[0] = os.Stdin
	vm.Files[1] = os.Stdout
	vm.Files[2] = os.Stderr

	if err := vm.loadBinary(bytecode); err != nil {
		return nil, err
	}
	return vm, nil
}

func (vm *VM) loadBinary(data []byte) error {
	if len(data) < HeaderSize {
		return fmt.Errorf("file too small for Par4 header (%d bytes)", len(data))
	}

	magic := data[0:4]
	if !bytes.Equal(magic, Magic) && !bytes.Equal(magic, []byte("P3P\x01")) && !bytes.Equal(magic, []byte("NPC\x01")) {
		return fmt.Errorf("invalid magic number: %q", string(magic))
	}

	strPoolSize := int(binary.BigEndian.Uint16(data[6:8]))
	globalsCount := int(binary.BigEndian.Uint16(data[8:10]))
	funcCount := int(binary.BigEndian.Uint16(data[10:12]))
	entryFunc := int(binary.BigEndian.Uint16(data[12:14]))
	codeSize := int(binary.BigEndian.Uint16(data[14:16]))

	vm.EntryFuncIndex = entryFunc
	vm.StringPoolSize = strPoolSize
	vm.CodeSize = codeSize

	offset := HeaderSize
	// Read Global Variable Table
	if offset+globalsCount > len(data) {
		return fmt.Errorf("truncated globals table")
	}
	vm.GlobalSizes = make([]int, globalsCount)
	for i := 0; i < globalsCount; i++ {
		vm.GlobalSizes[i] = int(data[offset+i])
	}
	offset += globalsCount

	// Read String Pool
	if offset+strPoolSize > len(data) {
		return fmt.Errorf("truncated string pool")
	}
	vm.StringPoolBase = 0x0100
	vm.StringPoolSize = strPoolSize
	copy(vm.Memory[vm.StringPoolBase:], data[offset:offset+strPoolSize])
	offset += strPoolSize

	// Read Function Table
	funcTableSize := funcCount * FuncEntrySz
	if offset+funcTableSize > len(data) {
		return fmt.Errorf("truncated function table")
	}
	funcBytes := data[offset : offset+funcTableSize]
	offset += funcTableSize

	// Read Local Variable Tables
	// Bytecode section follows local variable tables
	// Code starts at len(data) - codeSize
	localVarsSize := len(data) - codeSize - offset
	if localVarsSize < 0 {
		return fmt.Errorf("corrupted bytecode offset")
	}
	localVarsBytes := data[offset : offset+localVarsSize]
	offset += localVarsSize

	// Read Bytecode Section
	if offset+codeSize > len(data) {
		return fmt.Errorf("truncated bytecode section")
	}
	vm.CodeBase = (vm.StringPoolBase + vm.StringPoolSize + 0x00FF) &^ 0x00FF // page align
	copy(vm.Memory[vm.CodeBase:], data[offset:offset+codeSize])

	// Parse Functions
	vm.Functions = make([]*FunctionMetadata, funcCount)
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
		if i == entryFunc {
			name = "main"
		}
		vm.Functions[i] = &FunctionMetadata{
			Index:          i,
			Name:           name,
			IsEntry:        (i == entryFunc),
			ArgCount:       argCount,
			LocalCount:     localCount,
			FrameSize:      frameSize,
			CodeOffset:     vm.CodeBase + cOff,
			CodeSize:       cSize,
			VarTableOffset: varOff,
			VarSizes:       varSizes,
		}
	}

	// Allocate space for Globals in VM memory
	vm.GlobalsBase = (vm.CodeBase + vm.CodeSize + 0x00FF) &^ 0x00FF
	totalGlobalsSize := 0
	for _, sz := range vm.GlobalSizes {
		totalGlobalsSize += sz
	}

	// Initialize Heap above Globals, up to $DC00
	heapStart := (vm.GlobalsBase + totalGlobalsSize + 0x00FF) &^ 0x00FF
	heapEnd := 0xDC00
	vm.Heap = NewHeapManager(vm.Memory, heapStart, heapEnd)

	return nil
}

// Push pushes a 16-bit word onto the operand stack.
func (vm *VM) Push(val uint16) {
	vm.Stack = append(vm.Stack, val)
}

// Pop pops a 16-bit word from the operand stack.
func (vm *VM) Pop() uint16 {
	if len(vm.Stack) == 0 {
		funcName := ""
		if vm.CurrentFrame != nil {
			funcName = vm.CurrentFrame.Func.Name
		}
		panic(&VMError{Message: "Operand stack underflow", PC: vm.PC, FuncName: funcName})
	}
	top := vm.Stack[len(vm.Stack)-1]
	vm.Stack = vm.Stack[:len(vm.Stack)-1]
	return top
}

// PushSlice pushes a 3-word slice {ptr, cap, len} onto the stack.
func (vm *VM) PushSlice(ptr, cap, length uint16) {
	vm.Push(ptr)
	vm.Push(cap)
	vm.Push(length)
}

// PopSlice pops a 3-word slice from the stack (returns ptr, cap, len).
func (vm *VM) PopSlice() (ptr, cap, length uint16) {
	length = vm.Pop()
	cap = vm.Pop()
	ptr = vm.Pop()
	return ptr, cap, length
}

// Run executes instructions until HALT, exit, or error.
func (vm *VM) Run() (exitCode int, runErr error) {
	if len(vm.Functions) == 0 || vm.EntryFuncIndex >= len(vm.Functions) {
		return 0, fmt.Errorf("no entry function found")
	}

	entryFn := vm.Functions[vm.EntryFuncIndex]
	vm.FrameSP = 0xFF00 - entryFn.FrameSize
	vm.CurrentFrame = &CallFrame{
		Func:      entryFn,
		ReturnPC:  -1,
		FrameAddr: vm.FrameSP,
	}
	vm.CallStack = []*CallFrame{vm.CurrentFrame}
	vm.PC = entryFn.CodeOffset
	vm.Running = true

	defer func() {
		if r := recover(); r != nil {
			exitCode = 1
			if vme, ok := r.(*VMError); ok {
				runErr = vme
			} else {
				runErr = fmt.Errorf("panic: %v", r)
			}
		}
	}()

	for vm.Running {
		if vm.MaxInstructions > 0 && vm.InstructionCount >= vm.MaxInstructions {
			return 1, &VMError{Message: fmt.Sprintf("instruction limit %d reached", vm.MaxInstructions), PC: vm.PC}
		}
		vm.Step()
	}

	return vm.ExitCode, runErr
}

// Step executes a single bytecode instruction.
func (vm *VM) Step() {
	if vm.PC < 0 || vm.PC >= len(vm.Memory) {
		panic(&VMError{Message: "PC out of bounds", PC: vm.PC})
	}

	instPC := vm.PC
	opcode := vm.Memory[vm.PC]
	vm.PC++
	vm.InstructionCount++

	info, exists := OpcodeTable[opcode]
	if !exists {
		panic(&VMError{Message: fmt.Sprintf("unknown opcode 0x%02X", opcode), PC: instPC})
	}

	var arg uint16
	if info.ArgBytes == 1 {
		arg = uint16(vm.Memory[vm.PC])
		vm.PC++
	} else if info.ArgBytes == 2 {
		arg = binary.BigEndian.Uint16(vm.Memory[vm.PC : vm.PC+2])
		vm.PC += 2
	}

	if vm.Verbose {
		stackPreview := ""
		if len(vm.Stack) > 0 {
			count := len(vm.Stack)
			if count > 4 {
				count = 4
			}
			var items []string
			for i := len(vm.Stack) - count; i < len(vm.Stack); i++ {
				items = append(items, fmt.Sprintf("0x%04X", vm.Stack[i]))
			}
			stackPreview = fmt.Sprintf(" [stack: %s]", strings.Join(items, ", "))
		}
		fmt.Fprintf(vm.Stderr, "0x%04X: %-15s (arg=0x%04X)%s\n", instPC, info.Mnemonic, arg, stackPreview)
	}

	vm.executeOpcode(opcode, arg, instPC)
}

func (vm *VM) executeOpcode(op byte, arg uint16, instPC int) {
	switch op {
	// =========================================================================
	// Group 1: Stack & Literals (0x00 .. 0x1F)
	// =========================================================================
	case OpNOP:
		// No-op

	case OpPUSH_NIL, OpPUSH_0, OpPUSH_FALSE:
		vm.Push(0x0000)

	case OpPUSH_NIL_SLICE:
		vm.PushSlice(0, 0, 0)

	case OpPUSH_TRUE, OpPUSH_1:
		vm.Push(0x0001)

	case OpPUSH_NEG1:
		vm.Push(0xFFFF)

	case OpPUSH_I8:
		// Sign-extended 8-bit integer
		signedVal := int8(byte(arg))
		vm.Push(uint16(int16(signedVal)))

	case OpPUSH_U8:
		vm.Push(arg & 0x00FF)

	case OpPUSH_I16:
		vm.Push(arg)

	case OpPUSH_STR:
		// arg is offset into String Pool
		addr := vm.StringPoolBase + int(arg)
		strLen := int(binary.BigEndian.Uint16(vm.Memory[addr:]))
		ptr := uint16(addr + 2)
		vm.PushSlice(ptr, uint16(strLen), uint16(strLen))

	case OpPOP:
		vm.Pop()

	case OpPOP_SLICE:
		vm.PopSlice()

	case OpDUP:
		w := vm.Pop()
		vm.Push(w)
		vm.Push(w)

	case OpSWAP:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(b)
		vm.Push(a)

	case OpDUP2:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a)
		vm.Push(b)
		vm.Push(a)
		vm.Push(b)

	case OpROT:
		c := vm.Pop()
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(b)
		vm.Push(c)
		vm.Push(a)

	// =========================================================================
	// Group 2: Variables, Pointers & Memory (0x20 .. 0x3F)
	// =========================================================================
	case OpLOAD_LOCAL_0, OpLOAD_LOCAL_1, OpLOAD_LOCAL_2, OpLOAD_LOCAL_3:
		vm.loadLocal(int(op - OpLOAD_LOCAL_0))

	case OpSTORE_LOCAL_0, OpSTORE_LOCAL_1, OpSTORE_LOCAL_2, OpSTORE_LOCAL_3:
		vm.storeLocal(int(op - OpSTORE_LOCAL_0))

	case OpLOAD_LOCAL:
		vm.loadLocal(int(arg))

	case OpSTORE_LOCAL:
		vm.storeLocal(int(arg))

	case OpLOAD_GLOBAL:
		vm.loadGlobal(int(arg))

	case OpSTORE_GLOBAL:
		vm.storeGlobal(int(arg))

	case OpBUF_ALLOC:
		sz := int(vm.Pop())
		addr := vm.Heap.Alloc(sz)
		vm.Push(uint16(addr))

	case OpBUF_FREE:
		addr := int(vm.Pop())
		vm.Heap.Free(addr)

	case OpLOAD_FIELD:
		objPtr := int(vm.Pop())
		offset := int(arg)
		val := binary.BigEndian.Uint16(vm.Memory[objPtr+offset:])
		vm.Push(val)

	case OpSTORE_FIELD:
		objPtr := int(vm.Pop())
		val := vm.Pop()
		offset := int(arg)
		binary.BigEndian.PutUint16(vm.Memory[objPtr+offset:], val)

	case OpADDR_OF_GLOBAL:
		offset := 0
		for i := 0; i < int(arg); i++ {
			offset += vm.GlobalSizes[i]
		}
		vm.Push(uint16(vm.GlobalsBase + offset))

	case OpADDR_OF_LOCAL:
		offset := 0
		for i := 0; i < int(arg); i++ {
			offset += vm.CurrentFrame.Func.VarSizes[i]
		}
		vm.Push(uint16(vm.CurrentFrame.FrameAddr + offset))

	case OpPEEK2:
		addr := int(vm.Pop())
		val := binary.BigEndian.Uint16(vm.Memory[addr:])
		vm.Push(val)

	case OpPOKE2:
		val := vm.Pop()
		addr := int(vm.Pop())
		binary.BigEndian.PutUint16(vm.Memory[addr:], val)

	case OpPEEK1:
		addr := int(vm.Pop())
		val := uint16(vm.Memory[addr])
		vm.Push(val)

	case OpPOKE1:
		val := vm.Pop()
		addr := int(vm.Pop())
		vm.Memory[addr] = byte(val)

	case OpSHL1_ADD:
		idx := vm.Pop()
		base := vm.Pop()
		vm.Push(base + (idx << 1))

	case OpMEM_COPY:
		count := int(vm.Pop())
		dst := int(vm.Pop())
		src := int(vm.Pop())
		copy(vm.Memory[dst:dst+count], vm.Memory[src:src+count])

	case OpMEM_SET:
		count := int(vm.Pop())
		val := byte(vm.Pop())
		dst := int(vm.Pop())
		for i := 0; i < count; i++ {
			vm.Memory[dst+i] = val
		}

	// =========================================================================
	// Group 3: Arithmetic, Logic & Comparisons (0x40 .. 0x5F)
	// =========================================================================
	case OpADD:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a + b)

	case OpSUB:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a - b)

	case OpMUL:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a * b)

	case OpDIV:
		b := vm.Pop()
		a := vm.Pop()
		if b == 0 {
			panic(&VMError{Message: "division by zero", PC: instPC})
		}
		vm.Push(a / b)

	case OpMOD:
		b := vm.Pop()
		a := vm.Pop()
		if b == 0 {
			panic(&VMError{Message: "modulo by zero", PC: instPC})
		}
		vm.Push(a % b)

	case OpNEG:
		a := vm.Pop()
		vm.Push(uint16(-int16(a)))

	case OpBIT_AND:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a & b)

	case OpBIT_OR:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a | b)

	case OpBIT_XOR:
		b := vm.Pop()
		a := vm.Pop()
		vm.Push(a ^ b)

	case OpBIT_NOT:
		a := vm.Pop()
		vm.Push(^a)

	case OpSHL:
		shift := vm.Pop()
		a := vm.Pop()
		vm.Push(a << shift)

	case OpSHR:
		shift := vm.Pop()
		a := vm.Pop()
		// Arithmetic shift right (signed)
		signedA := int16(a)
		vm.Push(uint16(signedA >> shift))

	case OpCMP_EQ:
		b := vm.Pop()
		a := vm.Pop()
		if a == b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpCMP_NE:
		b := vm.Pop()
		a := vm.Pop()
		if a != b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpCMP_LT:
		b := vm.Pop()
		a := vm.Pop()
		if a < b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpCMP_LE:
		b := vm.Pop()
		a := vm.Pop()
		if a <= b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpCMP_GT:
		b := vm.Pop()
		a := vm.Pop()
		if a > b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpCMP_GE:
		b := vm.Pop()
		a := vm.Pop()
		if a >= b {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpNOT:
		a := vm.Pop()
		if a == 0 {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpMIN:
		b := int16(vm.Pop())
		a := int16(vm.Pop())
		if a < b {
			vm.Push(uint16(a))
		} else {
			vm.Push(uint16(b))
		}

	case OpMAX:
		b := int16(vm.Pop())
		a := int16(vm.Pop())
		if a > b {
			vm.Push(uint16(a))
		} else {
			vm.Push(uint16(b))
		}

	case OpPARSE_INT:
		radix := int(vm.Pop())
		sPtr, _, sLen := vm.PopSlice()
		s := string(vm.Memory[sPtr : sPtr+sLen])
		val, err := strconv.ParseInt(strings.TrimSpace(s), radix, 16)
		if err == nil {
			vm.Push(uint16(val))
			vm.Push(0x0001) // true
		} else {
			vm.Push(0x0000)
			vm.Push(0x0000) // false
		}

	// =========================================================================
	// Group 4: Control Flow, Calls & Switches (0x60 .. 0x6F)
	// =========================================================================
	case OpJUMP:
		rel := int16(arg)
		vm.PC = (instPC + 3) + int(rel)

	case OpJUMP_IF_TRUE:
		cond := vm.Pop()
		if cond != 0 {
			rel := int16(arg)
			vm.PC = (instPC + 3) + int(rel)
		}

	case OpJUMP_IF_FALSE:
		cond := vm.Pop()
		if cond == 0 {
			rel := int16(arg)
			vm.PC = (instPC + 3) + int(rel)
		}

	case OpCALL:
		fnIdx := int(arg)
		if fnIdx >= len(vm.Functions) {
			panic(&VMError{Message: fmt.Sprintf("invalid function index: %d", fnIdx), PC: instPC})
		}
		targetFn := vm.Functions[fnIdx]
		vm.FrameSP -= targetFn.FrameSize
		// Zero frame memory
		for i := 0; i < targetFn.FrameSize; i++ {
			vm.Memory[vm.FrameSP+i] = 0
		}
		newFrame := &CallFrame{
			Func:      targetFn,
			ReturnPC:  vm.PC,
			FrameAddr: vm.FrameSP,
		}

		// Pop arguments into parameters in reverse order
		for pIdx := targetFn.ArgCount - 1; pIdx >= 0; pIdx-- {
			sz := targetFn.VarSizes[pIdx]
			offset := 0
			for i := 0; i < pIdx; i++ {
				offset += targetFn.VarSizes[i]
			}
			addr := newFrame.FrameAddr + offset
			if sz == 1 {
				val := vm.Pop()
				vm.Memory[addr] = byte(val)
			} else {
				words := make([]uint16, sz/2)
				for w := sz/2 - 1; w >= 0; w-- {
					words[w] = vm.Pop()
				}
				for w := 0; w < sz/2; w++ {
					binary.BigEndian.PutUint16(vm.Memory[addr+w*2:], words[w])
				}
			}
		}

		vm.CallStack = append(vm.CallStack, newFrame)
		vm.CurrentFrame = newFrame
		vm.PC = targetFn.CodeOffset

	case OpRET, OpRET_SLICE, OpRET_VOID:
		// For RET: scalar already on stack
		// For RET_SLICE: 3 words already on stack
		// For RET_VOID: returns nothing

		retPC := vm.CurrentFrame.ReturnPC
		vm.FrameSP += vm.CurrentFrame.Func.FrameSize
		vm.CallStack = vm.CallStack[:len(vm.CallStack)-1]

		if len(vm.CallStack) == 0 || retPC < 0 {
			vm.Running = false
			return
		}
		vm.CurrentFrame = vm.CallStack[len(vm.CallStack)-1]
		vm.PC = retPC

	case OpHALT:
		vm.Running = false

	case OpPANIC:
		sPtr, _, sLen := vm.PopSlice()
		msg := string(vm.Memory[sPtr : sPtr+sLen])
		fmt.Fprintf(vm.Stderr, "PANIC: %s\n", msg)
		vm.ExitCode = 1
		vm.Running = false

	// =========================================================================
	// Group 5: Slices & Strings (0x70 .. 0x8F)
	// =========================================================================
	case OpSLICE_NEW:
		length := vm.Pop()
		cap := vm.Pop()
		ptr := vm.Pop()
		vm.PushSlice(ptr, cap, length)

	case OpSLICE_LEN:
		_, _, length := vm.PopSlice()
		vm.Push(length)

	case OpSLICE_CAP:
		_, cap, _ := vm.PopSlice()
		vm.Push(cap)

	case OpSLICE_SUB:
		end := vm.Pop()
		start := vm.Pop()
		ptr, cap, _ := vm.PopSlice()
		newPtr := ptr + start
		newCap := cap - start
		newLen := end - start
		vm.PushSlice(newPtr, newCap, newLen)

	case OpSLICE_GET_BYTE:
		idx := int(vm.Pop())
		ptr, _, length := vm.PopSlice()
		if idx < 0 || idx >= int(length) {
			vm.Push(0xFFFF) // -1
		} else {
			vm.Push(uint16(vm.Memory[int(ptr)+idx]))
		}

	case OpSLICE_GET_WORD:
		idx := int(vm.Pop())
		ptr, _, _ := vm.PopSlice()
		addr := int(ptr) + idx*2
		val := binary.BigEndian.Uint16(vm.Memory[addr:])
		vm.Push(val)

	case OpSLICE_SET_WORD:
		val := vm.Pop()
		idx := int(vm.Pop())
		ptr, _, _ := vm.PopSlice()
		addr := int(ptr) + idx*2
		binary.BigEndian.PutUint16(vm.Memory[addr:], val)

	case OpSTR_CMP:
		bPtr, _, bLen := vm.PopSlice()
		aPtr, _, aLen := vm.PopSlice()
		sA := vm.Memory[aPtr : aPtr+aLen]
		sB := vm.Memory[bPtr : bPtr+bLen]
		cmp := bytes.Compare(sA, sB)
		if cmp < 0 {
			vm.Push(0xFFFF) // -1
		} else if cmp > 0 {
			vm.Push(0x0001) // +1
		} else {
			vm.Push(0x0000) // 0
		}

	case OpSTR_STARTSWITH:
		pPtr, _, pLen := vm.PopSlice()
		sPtr, _, sLen := vm.PopSlice()
		s := vm.Memory[sPtr : sPtr+sLen]
		p := vm.Memory[pPtr : pPtr+pLen]
		if bytes.HasPrefix(s, p) {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpSTR_ENDSWITH:
		pPtr, _, pLen := vm.PopSlice()
		sPtr, _, sLen := vm.PopSlice()
		s := vm.Memory[sPtr : sPtr+sLen]
		p := vm.Memory[pPtr : pPtr+pLen]
		if bytes.HasSuffix(s, p) {
			vm.Push(0x0001)
		} else {
			vm.Push(0x0000)
		}

	case OpSTR_FIND:
		start := int(vm.Pop())
		subPtr, _, subLen := vm.PopSlice()
		sPtr, _, sLen := vm.PopSlice()
		if start < int(sLen) {
			s := vm.Memory[int(sPtr)+start : sPtr+sLen]
			sub := vm.Memory[subPtr : subPtr+subLen]
			idx := bytes.Index(s, sub)
			if idx >= 0 {
				vm.Push(uint16(start + idx))
			} else {
				vm.Push(0xFFFF)
			}
		} else {
			vm.Push(0xFFFF)
		}

	// =========================================================================
	// Group 6: Collections (Dicts & Lists) (0x90 .. 0xAF)
	// =========================================================================
	case OpDICT_NEW:
		entries := int(arg)
		byteSize := entries * 4
		addr := vm.Heap.Alloc(byteSize)
		vm.PushSlice(uint16(addr), uint16(entries*2), 0)

	case OpDICT_GET:
		kPtr, _, kLen := vm.PopSlice()
		dPtr, _, dLen := vm.PopSlice()
		keyBytes := vm.Memory[kPtr : kPtr+kLen]

		found := false
		var foundVal uint16
		for i := 0; i < int(dLen); i += 2 {
			entryKeyPtr := binary.BigEndian.Uint16(vm.Memory[int(dPtr)+i*2:])
			candLen := int(binary.BigEndian.Uint16(vm.Memory[entryKeyPtr-2:]))
			candKey := vm.Memory[entryKeyPtr : int(entryKeyPtr)+candLen]
			if bytes.Equal(keyBytes, candKey) {
				foundVal = binary.BigEndian.Uint16(vm.Memory[int(dPtr)+(i+1)*2:])
				found = true
				break
			}
		}

		if found {
			vm.Push(foundVal)
			vm.Push(0x0001) // true
		} else {
			vm.Push(0x0000)
			vm.Push(0x0000) // false
		}

	case OpDICT_SET:
		val := vm.Pop()
		kPtr, _, kLen := vm.PopSlice()
		dPtr, dCap, dLen := vm.PopSlice()
		keyBytes := vm.Memory[kPtr : kPtr+kLen]

		foundIdx := -1
		for i := 0; i < int(dLen); i += 2 {
			entryKeyPtr := binary.BigEndian.Uint16(vm.Memory[int(dPtr)+i*2:])
			candLen := int(binary.BigEndian.Uint16(vm.Memory[entryKeyPtr-2:]))
			candKey := vm.Memory[entryKeyPtr : int(entryKeyPtr)+candLen]
			if bytes.Equal(keyBytes, candKey) {
				foundIdx = i
				break
			}
		}

		if foundIdx >= 0 {
			binary.BigEndian.PutUint16(vm.Memory[int(dPtr)+(foundIdx+1)*2:], val)
			vm.PushSlice(dPtr, dCap, dLen)
		} else {
			if dLen >= dCap {
				// Grow dict buffer
				newCap := dCap * 2
				if newCap < 8 {
					newCap = 8
				}
				newAddr := uint16(vm.Heap.Alloc(int(newCap) * 2))
				copy(vm.Memory[newAddr:int(newAddr)+int(dLen)*2], vm.Memory[dPtr:int(dPtr)+int(dLen)*2])
				vm.Heap.Free(int(dPtr))
				dPtr = newAddr
				dCap = newCap
			}
			binary.BigEndian.PutUint16(vm.Memory[int(dPtr)+int(dLen)*2:], kPtr)
			binary.BigEndian.PutUint16(vm.Memory[int(dPtr)+(int(dLen)+1)*2:], val)
			vm.PushSlice(dPtr, dCap, dLen+2)
		}

	case OpLIST_NEW:
		capacity := int(arg)
		addr := vm.Heap.Alloc(capacity * 2)
		vm.PushSlice(uint16(addr), uint16(capacity), 0)

	case OpLIST_APPEND:
		val := vm.Pop()
		lPtr, lCap, lLen := vm.PopSlice()
		if lLen >= lCap {
			newCap := lCap * 2
			if newCap < 4 {
				newCap = 4
			}
			newAddr := uint16(vm.Heap.Alloc(int(newCap) * 2))
			copy(vm.Memory[newAddr:int(newAddr)+int(lLen)*2], vm.Memory[lPtr:int(lPtr)+int(lLen)*2])
			vm.Heap.Free(int(lPtr))
			lPtr = newAddr
			lCap = newCap
		}
		binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+int(lLen)*2:], val)
		vm.PushSlice(lPtr, lCap, lLen+1)

	case OpLIST_POP:
		lPtr, lCap, lLen := vm.PopSlice()
		if lLen == 0 {
			panic(&VMError{Message: "pop from empty list", PC: instPC})
		}
		val := binary.BigEndian.Uint16(vm.Memory[int(lPtr)+int(lLen-1)*2:])
		vm.PushSlice(lPtr, lCap, lLen-1)
		vm.Push(val)

	case OpSTR_APPEND:
		val := byte(vm.Pop())
		sPtr, sCap, sLen := vm.PopSlice()
		if sLen >= sCap {
			newCap := sCap * 2
			if newCap < 8 {
				newCap = 8
			}
			newAddr := uint16(vm.Heap.Alloc(int(newCap)))
			copy(vm.Memory[newAddr:int(newAddr)+int(sLen)], vm.Memory[sPtr:int(sPtr)+int(sLen)])
			vm.Heap.Free(int(sPtr))
			sPtr = newAddr
			sCap = newCap
		}
		vm.Memory[int(sPtr)+int(sLen)] = val
		vm.PushSlice(sPtr, sCap, sLen+1)

	case OpSLICE_APPEND_STR:
		strPtr, strCap, strLen := vm.PopSlice()
		lPtr, lCap, lLen := vm.PopSlice()
		if lLen >= lCap {
			newCap := lCap * 2
			if newCap < 4 {
				newCap = 4
			}
			newAddr := uint16(vm.Heap.Alloc(int(newCap) * 6))
			copy(vm.Memory[newAddr:int(newAddr)+int(lLen)*6], vm.Memory[lPtr:int(lPtr)+int(lLen)*6])
			vm.Heap.Free(int(lPtr))
			lPtr = newAddr
			lCap = newCap
		}
		binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+int(lLen)*6:], strPtr)
		binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+int(lLen)*6+2:], strCap)
		binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+int(lLen)*6+4:], strLen)
		vm.PushSlice(lPtr, lCap, lLen+1)

	case OpLIST_SORT_BY_LEN:
		lPtr, lCap, lLen := vm.PopSlice()
		type strItem struct {
			ptr, cap, length uint16
		}
		items := make([]strItem, lLen)
		for i := 0; i < int(lLen); i++ {
			items[i].ptr = binary.BigEndian.Uint16(vm.Memory[int(lPtr)+i*6:])
			items[i].cap = binary.BigEndian.Uint16(vm.Memory[int(lPtr)+i*6+2:])
			items[i].length = binary.BigEndian.Uint16(vm.Memory[int(lPtr)+i*6+4:])
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].length > items[j].length
		})
		for i := 0; i < int(lLen); i++ {
			binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+i*6:], items[i].ptr)
			binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+i*6+2:], items[i].cap)
			binary.BigEndian.PutUint16(vm.Memory[int(lPtr)+i*6+4:], items[i].length)
		}
		vm.PushSlice(lPtr, lCap, lLen)

	// =========================================================================
	// Group 8: Hatvan OS & Builtins (0xC0 .. 0xCF)
	// =========================================================================
	case OpHATVAN_TRAP:
		callNum := byte(arg)
		vm.handleHatvanTrap(callNum, instPC)

	case OpSYS_POLL_FLAG:
		// Flag polling: EF1 (term rx ready), EF2 (disk), EF3 (timer), EF4 (DMA)
		// On POSIX host, always ready (return true)
		vm.Push(0x0001)

	case OpSYS_EXIT:
		code := vm.Pop()
		vm.ExitCode = int(code)
		vm.Running = false

	case OpIO_PRINT:
		sPtr, _, sLen := vm.PopSlice()
		text := string(vm.Memory[sPtr : sPtr+sLen])
		fmt.Fprintln(vm.Stdout, text)

	case OpPRINT, OpPRINTLN:
		isPrintln := (op == OpPRINTLN)
		sPtr, _, sLen := vm.PopSlice()
		var parts []string
		for i := 0; i < int(sLen); i++ {
			entryAddr := int(sPtr) + i*4
			valAddr := int(binary.BigEndian.Uint16(vm.Memory[entryAddr:]))
			typeAddr := int(binary.BigEndian.Uint16(vm.Memory[entryAddr+2:]))
			typeName := vm.readCString(typeAddr)

			switch typeName {
			case "word", "uint", "uint16", "int", "int16":
				v := binary.BigEndian.Uint16(vm.Memory[valAddr:])
				parts = append(parts, strconv.Itoa(int(v)))
			case "byte", "uint8":
				v := vm.Memory[valAddr]
				parts = append(parts, strconv.Itoa(int(v)))
			case "bool":
				v := binary.BigEndian.Uint16(vm.Memory[valAddr:])
				if v != 0 {
					parts = append(parts, "true")
				} else {
					parts = append(parts, "false")
				}
			case "string", "slice_byte", "slice[byte]", "prelude.slice_byte":
				sBase := int(binary.BigEndian.Uint16(vm.Memory[valAddr:]))
				sLength := int(binary.BigEndian.Uint16(vm.Memory[valAddr+4:]))
				parts = append(parts, string(vm.Memory[sBase:sBase+sLength]))
			case "*byte", "cstring":
				cptr := int(binary.BigEndian.Uint16(vm.Memory[valAddr:]))
				parts = append(parts, vm.readCString(cptr))
			default:
				if valAddr != 0 {
					v := binary.BigEndian.Uint16(vm.Memory[valAddr:])
					parts = append(parts, strconv.Itoa(int(v)))
				} else {
					parts = append(parts, "<nil>")
				}
			}
		}
		outText := strings.Join(parts, " ")
		if isPrintln {
			fmt.Fprintln(vm.Stdout, outText)
		} else {
			fmt.Fprint(vm.Stdout, outText)
		}

	default:
		panic(&VMError{Message: fmt.Sprintf("unimplemented opcode: %s (0x%02X)", OpcodeTable[op].Mnemonic, op), PC: instPC})
	}
}

// handleHatvanTrap emulates Hatvan OS and OS-9 system calls on POSIX host.
func (vm *VM) handleHatvanTrap(callNum byte, instPC int) {
	// Stack on entry: [A, B, X, Y, U]
	// Pop in reverse order:
	u := vm.Pop()
	y := vm.Pop()
	x := vm.Pop()
	b := vm.Pop()
	a := vm.Pop()

	var retA, retB, retY uint16
	var errBool uint16

	switch callNum {
	case SysOpen: // 0x84: I$Open
		path := vm.readCString(int(x))
		mode := a & 0xFF
		flags := os.O_RDONLY
		if mode == 2 {
			flags = os.O_WRONLY
		} else if mode == 3 {
			flags = os.O_RDWR
		}

		f, err := os.OpenFile(path, flags, 0)
		if err != nil {
			retA = 216 // E$PNNF (Path name not found)
			errBool = 1
		} else {
			pathID := vm.NextFileID
			vm.NextFileID++
			vm.Files[pathID] = f
			retA = uint16(pathID)
			errBool = 0
		}

	case SysCreate: // 0x83: I$Create
		path := vm.readCString(int(x))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			retA = 216
			errBool = 1
		} else {
			pathID := vm.NextFileID
			vm.NextFileID++
			vm.Files[pathID] = f
			retA = uint16(pathID)
			errBool = 0
		}

	case SysRead: // 0x89: I$Read
		pathID := byte(a)
		f, ok := vm.Files[pathID]
		if !ok {
			retA = 216 // E$BPNum
			errBool = 1
		} else {
			buf := make([]byte, y)
			n, err := f.Read(buf)
			if n > 0 {
				copy(vm.Memory[int(x):int(x)+n], buf[:n])
				retY = uint16(n)
			}
			if err == io.EOF && n == 0 {
				retA = 211 // E$EOF
				errBool = 1
			} else if err != nil && err != io.EOF {
				retA = 211
				errBool = 1
			} else {
				retA = 0
				errBool = 0
			}
		}

	case SysWrite: // 0x8A: I$Write
		pathID := byte(a)
		f, ok := vm.Files[pathID]
		if !ok {
			retA = 216
			errBool = 1
		} else {
			data := vm.Memory[int(x) : int(x)+int(y)]
			n, err := f.Write(data)
			retY = uint16(n)
			if err != nil {
				retA = 211
				errBool = 1
			} else {
				retA = 0
				errBool = 0
			}
		}

	case SysReadLn: // 0x8B: I$ReadLn
		// ReadLn accepts \r or \n
		pathID := byte(a)
		f, ok := vm.Files[pathID]
		if !ok {
			retA = 216
			errBool = 1
		} else {
			max := int(y)
			count := 0
			oneByte := make([]byte, 1)
			hitNewline := false

			for count < max {
				n, err := f.Read(oneByte)
				if err != nil || n == 0 {
					break
				}
				ch := oneByte[0]
				if ch == '\r' {
					// Check if next char is \n and consume it
					vm.Memory[int(x)+count] = '\n'
					count++
					hitNewline = true
					break
				} else if ch == '\n' {
					vm.Memory[int(x)+count] = '\n'
					count++
					hitNewline = true
					break
				} else {
					vm.Memory[int(x)+count] = ch
					count++
				}
			}

			if count == 0 && !hitNewline {
				retA = 211 // E$EOF
				errBool = 1
			} else {
				retA = 0
				retY = uint16(count)
				errBool = 0
			}
		}

	case SysWritLn: // 0x8C: I$WritLn
		// WritLn produces unix \n characters
		pathID := byte(a)
		f, ok := vm.Files[pathID]
		if !ok {
			retA = 216
			errBool = 1
		} else {
			data := vm.Memory[int(x) : int(x)+int(y)]
			// Trim any trailing \r or \n
			trimmed := bytes.TrimRight(data, "\r\n")
			_, _ = f.Write(trimmed)
			_, _ = f.Write([]byte("\n"))
			retA = 0
			retY = y
			errBool = 0
		}

	case SysClose: // 0x8F: I$Close
		pathID := byte(a)
		if f, ok := vm.Files[pathID]; ok {
			if pathID > 2 {
				_ = f.Close()
				delete(vm.Files, pathID)
			}
			retA = 0
			errBool = 0
		} else {
			retA = 216
			errBool = 1
		}

	case SysSeek: // 0x88: I$Seek
		pathID := byte(a)
		f, ok := vm.Files[pathID]
		if !ok {
			retA = 216
			errBool = 1
		} else {
			offset := (int64(x) << 16) | int64(y)
			_, err := f.Seek(offset, io.SeekStart)
			if err != nil {
				retA = 211
				errBool = 1
			} else {
				retA = 0
				errBool = 0
			}
		}

	case SysDelete: // 0x87: I$Delete
		path := vm.readCString(int(x))
		err := os.Remove(path)
		if err != nil {
			retA = 216
			errBool = 1
		} else {
			retA = 0
			errBool = 0
		}

	case SysExit: // 0x06: F$Exit
		vm.ExitCode = int(a)
		vm.Running = false
		return

	case SysSleep: // 0x0A: F$Sleep
		ticks := int(x)
		time.Sleep(time.Duration(ticks) * time.Second / 60)
		retA = 0
		errBool = 0

	default:
		// Unsupported or directory ops stubbed out to return error
		retA = 208 // E$UnkSvc
		errBool = 1
	}

	// Push return values onto operand stack: [res_A, res_B, res_Y, err_bool]
	vm.Push(retA)
	vm.Push(retB)
	vm.Push(retY)
	vm.Push(errBool)
	_ = u
	_ = b
}

func (vm *VM) loadLocal(slot int) {
	offset := 0
	for i := 0; i < slot; i++ {
		offset += vm.CurrentFrame.Func.VarSizes[i]
	}
	addr := vm.CurrentFrame.FrameAddr + offset
	sz := vm.CurrentFrame.Func.VarSizes[slot]
	if sz == 1 {
		vm.Push(uint16(vm.Memory[addr]))
	} else {
		for w := 0; w < sz/2; w++ {
			val := binary.BigEndian.Uint16(vm.Memory[addr+w*2:])
			vm.Push(val)
		}
	}
}

func (vm *VM) storeLocal(slot int) {
	offset := 0
	for i := 0; i < slot; i++ {
		offset += vm.CurrentFrame.Func.VarSizes[i]
	}
	addr := vm.CurrentFrame.FrameAddr + offset
	sz := vm.CurrentFrame.Func.VarSizes[slot]
	if sz == 1 {
		val := vm.Pop()
		vm.Memory[addr] = byte(val)
	} else {
		for w := sz/2 - 1; w >= 0; w-- {
			val := vm.Pop()
			binary.BigEndian.PutUint16(vm.Memory[addr+w*2:], val)
		}
	}
}

func (vm *VM) loadGlobal(slot int) {
	offset := 0
	for i := 0; i < slot; i++ {
		offset += vm.GlobalSizes[i]
	}
	addr := vm.GlobalsBase + offset
	sz := vm.GlobalSizes[slot]
	if sz == 1 {
		vm.Push(uint16(vm.Memory[addr]))
	} else {
		for w := 0; w < sz/2; w++ {
			val := binary.BigEndian.Uint16(vm.Memory[addr+w*2:])
			vm.Push(val)
		}
	}
}

func (vm *VM) storeGlobal(slot int) {
	offset := 0
	for i := 0; i < slot; i++ {
		offset += vm.GlobalSizes[i]
	}
	addr := vm.GlobalsBase + offset
	sz := vm.GlobalSizes[slot]
	if sz == 1 {
		val := vm.Pop()
		vm.Memory[addr] = byte(val)
	} else {
		for w := sz/2 - 1; w >= 0; w-- {
			val := vm.Pop()
			binary.BigEndian.PutUint16(vm.Memory[addr+w*2:], val)
		}
	}
}

func (vm *VM) readCString(addr int) string {
	if addr == 0 {
		return ""
	}
	var sb strings.Builder
	for addr < len(vm.Memory) && vm.Memory[addr] != 0 && vm.Memory[addr] != '\r' {
		sb.WriteByte(vm.Memory[addr])
		addr++
	}
	return sb.String()
}
