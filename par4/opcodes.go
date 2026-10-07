package par4

// Binary Format Constants for Par4
var (
	Magic       = []byte("P4P\x01")
	FormatVer   = byte(1)
	HeaderSize  = 16
	FuncEntrySz = 10
)

const (
	Flag1802Optimized = byte(0x01)
	FlagCurtainAccess = byte(0x02)
)

// Opcode constants
const (
	// Group 1: Stack & Literals (0x00 .. 0x1F)
	OpNOP            = byte(0x00)
	OpPUSH_NIL       = byte(0x01)
	OpPUSH_NIL_SLICE = byte(0x02)
	OpPUSH_TRUE      = byte(0x03)
	OpPUSH_FALSE     = byte(0x04)
	OpPUSH_0         = byte(0x05)
	OpPUSH_1         = byte(0x06)
	OpPUSH_NEG1      = byte(0x07)
	OpPUSH_I8        = byte(0x08)
	OpPUSH_U8        = byte(0x09)
	OpPUSH_I16       = byte(0x0A)
	OpPUSH_STR       = byte(0x0B)
	OpPOP            = byte(0x0C)
	OpPOP_SLICE      = byte(0x0D)
	OpDUP            = byte(0x0E)
	OpSWAP           = byte(0x0F)
	OpDUP2           = byte(0x10)
	OpROT            = byte(0x11)

	// Group 2: Variables, Pointers & Memory (0x20 .. 0x3F)
	OpLOAD_LOCAL_0   = byte(0x20)
	OpLOAD_LOCAL_1   = byte(0x21)
	OpLOAD_LOCAL_2   = byte(0x22)
	OpLOAD_LOCAL_3   = byte(0x23)
	OpSTORE_LOCAL_0  = byte(0x24)
	OpSTORE_LOCAL_1  = byte(0x25)
	OpSTORE_LOCAL_2  = byte(0x26)
	OpSTORE_LOCAL_3  = byte(0x27)
	OpLOAD_LOCAL     = byte(0x28)
	OpSTORE_LOCAL    = byte(0x29)
	OpLOAD_GLOBAL    = byte(0x2A)
	OpSTORE_GLOBAL   = byte(0x2B)
	OpBUF_ALLOC      = byte(0x2C)
	OpBUF_FREE       = byte(0x2D)
	OpLOAD_FIELD     = byte(0x2E)
	OpSTORE_FIELD    = byte(0x2F)
	OpADDR_OF_GLOBAL = byte(0x30)
	OpPEEK2          = byte(0x31)
	OpPOKE2          = byte(0x32)
	OpPEEK1          = byte(0x33)
	OpPOKE1          = byte(0x34)
	OpADDR_OF_LOCAL  = byte(0x35)
	OpSHL1_ADD       = byte(0x36)
	OpMEM_COPY       = byte(0x37)
	OpMEM_SET        = byte(0x38)

	// Group 3: Arithmetic, Logic & Comparisons (0x40 .. 0x5F)
	OpADD       = byte(0x40)
	OpSUB       = byte(0x41)
	OpMUL       = byte(0x42)
	OpDIV       = byte(0x43)
	OpMOD       = byte(0x44)
	OpNEG       = byte(0x45)
	OpBIT_AND   = byte(0x46)
	OpBIT_OR    = byte(0x47)
	OpBIT_XOR   = byte(0x48)
	OpBIT_NOT   = byte(0x49)
	OpSHL       = byte(0x4A)
	OpSHR       = byte(0x4B)
	OpCMP_EQ    = byte(0x4C)
	OpCMP_NE    = byte(0x4D)
	OpCMP_LT    = byte(0x4E)
	OpCMP_LE    = byte(0x4F)
	OpCMP_GT    = byte(0x50)
	OpCMP_GE    = byte(0x51)
	OpNOT       = byte(0x52)
	OpMIN       = byte(0x53)
	OpMAX       = byte(0x54)
	OpPARSE_INT = byte(0x55)

	// Group 4: Control Flow, Calls & Switches (0x60 .. 0x6F)
	OpJUMP          = byte(0x60)
	OpJUMP_IF_TRUE  = byte(0x61)
	OpJUMP_IF_FALSE = byte(0x62)
	OpCALL          = byte(0x63)
	OpRET           = byte(0x64)
	OpRET_SLICE     = byte(0x65)
	OpRET_VOID      = byte(0x66)
	OpHALT          = byte(0x67)
	OpPANIC         = byte(0x68)
	OpSWITCH_LOOKUP = byte(0x69)

	// Group 5: Slices & Strings (0x70 .. 0x8F)
	OpSLICE_NEW         = byte(0x70)
	OpSLICE_LEN         = byte(0x71)
	OpSLICE_CAP         = byte(0x72)
	OpSLICE_SUB         = byte(0x73)
	OpSLICE_GET_BYTE    = byte(0x74)
	OpSLICE_GET_WORD    = byte(0x75)
	OpSLICE_SET_WORD    = byte(0x76)
	OpSTR_CMP           = byte(0x77)
	OpSTR_STARTSWITH    = byte(0x78)
	OpSTR_ENDSWITH      = byte(0x79)
	OpSTR_FIND          = byte(0x7A)
	OpSTR_LSTRIP        = byte(0x7B)
	OpSTR_RSTRIP        = byte(0x7C)
	OpSTR_STRIP         = byte(0x7D)
	OpSTR_SPLITLINES    = byte(0x7E)
	OpSTR_REPLACE_IDENT = byte(0x7F)

	// Group 6: Collections (Dicts & Lists) (0x90 .. 0xAF)
	OpDICT_NEW         = byte(0x90)
	OpDICT_GET         = byte(0x91)
	OpDICT_SET         = byte(0x92)
	OpDICT_HAS         = byte(0x93)
	OpDICT_KEYS        = byte(0x94)
	OpDICT_LEN         = byte(0x95)
	OpLIST_NEW         = byte(0x96)
	OpLIST_APPEND      = byte(0x97)
	OpLIST_POP         = byte(0x98)
	OpLIST_SORT_BY_LEN = byte(0x99)
	OpSTR_APPEND       = byte(0x9A)
	OpSLICE_APPEND_STR = byte(0x9B)

	// Group 7: Regular Expressions (0xB0 .. 0xBF)
	OpREG_MATCH     = byte(0xB0)
	OpREG_SEARCH    = byte(0xB1)
	OpREG_GROUP     = byte(0xB2)
	OpREG_GROUP_END = byte(0xB3)

	// Group 8: Hatvan OS & Hardware Primitives (0xC0 .. 0xCF)
	OpHATVAN_TRAP   = byte(0xC0)
	OpSYS_POLL_FLAG = byte(0xC1)
	OpSYS_DMA_COPY  = byte(0xC2)
	OpSYS_EXIT      = byte(0xC8)
	OpIO_PRINT      = byte(0xC9)
	OpPRINT         = byte(0xCA)
	OpPRINTLN       = byte(0xCB)
)

// Hatvan / OS-9 System Call Numbers
const (
	SysLink   = byte(0x00) // F$Link
	SysLoad   = byte(0x01) // F$Load
	SysFork   = byte(0x03) // F$Fork
	SysWait   = byte(0x04) // F$Wait
	SysChain  = byte(0x05) // F$Chain
	SysExit   = byte(0x06) // F$Exit
	SysMem    = byte(0x07) // F$Mem
	SysSleep  = byte(0x0A) // F$Sleep
	SysTime   = byte(0x15) // F$Time
	SysAttach = byte(0x80) // I$Attach
	SysDetach = byte(0x81) // I$Detach
	SysDup    = byte(0x82) // I$Dup
	SysCreate = byte(0x83) // I$Create
	SysOpen   = byte(0x84) // I$Open
	SysMakDir = byte(0x85) // I$MakDir
	SysChgDir = byte(0x86) // I$ChgDir
	SysDelete = byte(0x87) // I$Delete
	SysSeek   = byte(0x88) // I$Seek
	SysRead   = byte(0x89) // I$Read
	SysWrite  = byte(0x8A) // I$Write
	SysReadLn = byte(0x8B) // I$ReadLn
	SysWritLn = byte(0x8C) // I$WritLn
	SysGetStt = byte(0x8D) // I$GetStt
	SysSetStt = byte(0x8E) // I$SetStt
	SysClose  = byte(0x8F) // I$Close
)

type ArgType string

const (
	ArgNone     ArgType = "none"
	ArgImm8S    ArgType = "imm8_s"
	ArgImm8U    ArgType = "imm8_u"
	ArgImm16    ArgType = "imm16"
	ArgRel16    ArgType = "rel16"
	ArgStr16    ArgType = "str16"
	ArgFunc16   ArgType = "func16"
	ArgLocal8   ArgType = "local8"
	ArgGlobal16 ArgType = "global16"
	ArgField8   ArgType = "field8"
)

type OpcodeInfo struct {
	Mnemonic string
	ArgBytes int
	ArgType  ArgType
	Desc     string
}

var OpcodeTable = map[byte]OpcodeInfo{
	OpNOP:            {"NOP", 0, ArgNone, "No operation"},
	OpPUSH_NIL:       {"PUSH_NIL", 0, ArgNone, "Push scalar nil ($0000)"},
	OpPUSH_NIL_SLICE: {"PUSH_NIL_SLICE", 0, ArgNone, "Push nil slice"},
	OpPUSH_TRUE:      {"PUSH_TRUE", 0, ArgNone, "Push boolean true ($0001)"},
	OpPUSH_FALSE:     {"PUSH_FALSE", 0, ArgNone, "Push boolean false ($0000)"},
	OpPUSH_0:         {"PUSH_0", 0, ArgNone, "Push integer 0"},
	OpPUSH_1:         {"PUSH_1", 0, ArgNone, "Push integer 1"},
	OpPUSH_NEG1:      {"PUSH_NEG1", 0, ArgNone, "Push integer -1"},
	OpPUSH_I8:        {"PUSH_I8", 1, ArgImm8S, "Push signed 8-bit integer"},
	OpPUSH_U8:        {"PUSH_U8", 1, ArgImm8U, "Push unsigned 8-bit integer"},
	OpPUSH_I16:       {"PUSH_I16", 2, ArgImm16, "Push 16-bit integer"},
	OpPUSH_STR:       {"PUSH_STR", 2, ArgStr16, "Push string slice"},
	OpPOP:            {"POP", 0, ArgNone, "Pop word"},
	OpPOP_SLICE:      {"POP_SLICE", 0, ArgNone, "Pop 3-word slice"},
	OpDUP:            {"DUP", 0, ArgNone, "Duplicate top word"},
	OpSWAP:           {"SWAP", 0, ArgNone, "Swap top two words"},
	OpDUP2:           {"DUP2", 0, ArgNone, "Duplicate top two words"},
	OpROT:            {"ROT", 0, ArgNone, "Rotate top three words"},

	OpLOAD_LOCAL_0:   {"LOAD_LOCAL_0", 0, ArgNone, "Fast load local 0"},
	OpLOAD_LOCAL_1:   {"LOAD_LOCAL_1", 0, ArgNone, "Fast load local 1"},
	OpLOAD_LOCAL_2:   {"LOAD_LOCAL_2", 0, ArgNone, "Fast load local 2"},
	OpLOAD_LOCAL_3:   {"LOAD_LOCAL_3", 0, ArgNone, "Fast load local 3"},
	OpSTORE_LOCAL_0:  {"STORE_LOCAL_0", 0, ArgNone, "Fast store local 0"},
	OpSTORE_LOCAL_1:  {"STORE_LOCAL_1", 0, ArgNone, "Fast store local 1"},
	OpSTORE_LOCAL_2:  {"STORE_LOCAL_2", 0, ArgNone, "Fast store local 2"},
	OpSTORE_LOCAL_3:  {"STORE_LOCAL_3", 0, ArgNone, "Fast store local 3"},
	OpLOAD_LOCAL:     {"LOAD_LOCAL", 1, ArgLocal8, "Load local variable"},
	OpSTORE_LOCAL:    {"STORE_LOCAL", 1, ArgLocal8, "Store local variable"},
	OpLOAD_GLOBAL:    {"LOAD_GLOBAL", 2, ArgGlobal16, "Load global variable"},
	OpSTORE_GLOBAL:   {"STORE_GLOBAL", 2, ArgGlobal16, "Store global variable"},
	OpBUF_ALLOC:      {"BUF_ALLOC", 0, ArgNone, "Allocate fixed buffer from heap"},
	OpBUF_FREE:       {"BUF_FREE", 0, ArgNone, "Free buffer back to heap"},
	OpLOAD_FIELD:     {"LOAD_FIELD", 1, ArgField8, "Load field from struct"},
	OpSTORE_FIELD:    {"STORE_FIELD", 1, ArgField8, "Store field to struct"},
	OpADDR_OF_GLOBAL: {"ADDR_OF_GLOBAL", 2, ArgGlobal16, "Address of global variable"},
	OpPEEK2:          {"PEEK2", 0, ArgNone, "Read 16-bit word from memory"},
	OpPOKE2:          {"POKE2", 0, ArgNone, "Write 16-bit word to memory"},
	OpPEEK1:          {"PEEK1", 0, ArgNone, "Read 8-bit byte from memory"},
	OpPOKE1:          {"POKE1", 0, ArgNone, "Write 8-bit byte to memory"},
	OpADDR_OF_LOCAL:  {"ADDR_OF_LOCAL", 1, ArgLocal8, "Address of local variable"},
	OpSHL1_ADD:       {"SHL1_ADD", 0, ArgNone, "Index word array: base + idx*2"},
	OpMEM_COPY:       {"MEM_COPY", 0, ArgNone, "Copy count bytes from src to dst"},
	OpMEM_SET:        {"MEM_SET", 0, ArgNone, "Set count bytes at dst to val"},

	OpADD:       {"ADD", 0, ArgNone, "16-bit addition"},
	OpSUB:       {"SUB", 0, ArgNone, "16-bit subtraction"},
	OpMUL:       {"MUL", 0, ArgNone, "16-bit unsigned multiplication"},
	OpDIV:       {"DIV", 0, ArgNone, "16-bit unsigned division"},
	OpMOD:       {"MOD", 0, ArgNone, "16-bit unsigned modulo"},
	OpNEG:       {"NEG", 0, ArgNone, "16-bit negation"},
	OpBIT_AND:   {"BIT_AND", 0, ArgNone, "16-bit bitwise AND"},
	OpBIT_OR:    {"BIT_OR", 0, ArgNone, "16-bit bitwise OR"},
	OpBIT_XOR:   {"BIT_XOR", 0, ArgNone, "16-bit bitwise XOR"},
	OpBIT_NOT:   {"BIT_NOT", 0, ArgNone, "16-bit bitwise NOT"},
	OpSHL:       {"SHL", 0, ArgNone, "16-bit shift left"},
	OpSHR:       {"SHR", 0, ArgNone, "16-bit arithmetic shift right"},
	OpCMP_EQ:    {"CMP_EQ", 0, ArgNone, "16-bit equal (==)"},
	OpCMP_NE:    {"CMP_NE", 0, ArgNone, "16-bit not equal (!=)"},
	OpCMP_LT:    {"CMP_LT", 0, ArgNone, "16-bit unsigned less than (<)"},
	OpCMP_LE:    {"CMP_LE", 0, ArgNone, "16-bit unsigned less than or equal (<=)"},
	OpCMP_GT:    {"CMP_GT", 0, ArgNone, "16-bit unsigned greater than (>)"},
	OpCMP_GE:    {"CMP_GE", 0, ArgNone, "16-bit unsigned greater than or equal (>=)"},
	OpNOT:       {"NOT", 0, ArgNone, "Logical NOT"},
	OpMIN:       {"MIN", 0, ArgNone, "Signed minimum"},
	OpMAX:       {"MAX", 0, ArgNone, "Signed maximum"},
	OpPARSE_INT: {"PARSE_INT", 0, ArgNone, "Parse string to integer"},

	OpJUMP:          {"JUMP", 2, ArgRel16, "Unconditional relative branch"},
	OpJUMP_IF_TRUE:  {"JUMP_IF_TRUE", 2, ArgRel16, "Branch if condition is true"},
	OpJUMP_IF_FALSE: {"JUMP_IF_FALSE", 2, ArgRel16, "Branch if condition is false"},
	OpCALL:          {"CALL", 2, ArgFunc16, "Call function"},
	OpRET:           {"RET", 0, ArgNone, "Return word to caller"},
	OpRET_SLICE:     {"RET_SLICE", 0, ArgNone, "Return 3-word slice to caller"},
	OpRET_VOID:      {"RET_VOID", 0, ArgNone, "Return void/nil to caller"},
	OpHALT:          {"HALT", 0, ArgNone, "Halt VM cleanly"},
	OpPANIC:         {"PANIC", 0, ArgNone, "Print panic message and exit"},
	OpSWITCH_LOOKUP: {"SWITCH_LOOKUP", 2, ArgImm16, "Table-driven switch jump"},

	OpSLICE_NEW:         {"SLICE_NEW", 0, ArgNone, "Create 3-word slice"},
	OpSLICE_LEN:         {"SLICE_LEN", 0, ArgNone, "Extract length from slice"},
	OpSLICE_CAP:         {"SLICE_CAP", 0, ArgNone, "Extract capacity from slice"},
	OpSLICE_SUB:         {"SLICE_SUB", 0, ArgNone, "Sub-slice view"},
	OpSLICE_GET_BYTE:    {"SLICE_GET_BYTE", 0, ArgNone, "Get byte from string slice"},
	OpSLICE_GET_WORD:    {"SLICE_GET_WORD", 0, ArgNone, "Get word from word slice"},
	OpSLICE_SET_WORD:    {"SLICE_SET_WORD", 0, ArgNone, "Set word in word slice"},
	OpSTR_CMP:           {"STR_CMP", 0, ArgNone, "String slice comparison"},
	OpSTR_STARTSWITH:    {"STR_STARTSWITH", 0, ArgNone, "String prefix check"},
	OpSTR_ENDSWITH:      {"STR_ENDSWITH", 0, ArgNone, "String suffix check"},
	OpSTR_FIND:          {"STR_FIND", 0, ArgNone, "String linear search"},
	OpSTR_LSTRIP:        {"STR_LSTRIP", 0, ArgNone, "Strip leading characters"},
	OpSTR_RSTRIP:        {"STR_RSTRIP", 0, ArgNone, "Strip trailing characters"},
	OpSTR_STRIP:         {"STR_STRIP", 0, ArgNone, "Strip leading and trailing characters"},
	OpSTR_SPLITLINES:    {"STR_SPLITLINES", 0, ArgNone, "Split text by newline"},
	OpSTR_REPLACE_IDENT: {"STR_REPLACE_IDENT", 0, ArgNone, "Word-boundary replace"},

	OpDICT_NEW:         {"DICT_NEW", 2, ArgImm16, "Allocate dict buffer"},
	OpDICT_GET:         {"DICT_GET", 0, ArgNone, "Dict key lookup"},
	OpDICT_SET:         {"DICT_SET", 0, ArgNone, "Dict key insert/update"},
	OpDICT_HAS:         {"DICT_HAS", 0, ArgNone, "Dict key membership test"},
	OpDICT_KEYS:        {"DICT_KEYS", 0, ArgNone, "List of dict string keys"},
	OpDICT_LEN:         {"DICT_LEN", 0, ArgNone, "Dict entry count"},
	OpLIST_NEW:         {"LIST_NEW", 2, ArgImm16, "Allocate list buffer"},
	OpLIST_APPEND:      {"LIST_APPEND", 0, ArgNone, "Append word to list"},
	OpLIST_POP:         {"LIST_POP", 0, ArgNone, "Pop word from list"},
	OpLIST_SORT_BY_LEN: {"LIST_SORT_BY_LEN", 0, ArgNone, "Sort string list by length"},
	OpSTR_APPEND:       {"STR_APPEND", 0, ArgNone, "Append byte to string slice"},
	OpSLICE_APPEND_STR: {"SLICE_APPEND_STR", 0, ArgNone, "Append string slice to list"},

	OpREG_MATCH:     {"REG_MATCH", 2, ArgImm16, "Anchored regex match"},
	OpREG_SEARCH:    {"REG_SEARCH", 2, ArgImm16, "Unanchored regex search"},
	OpREG_GROUP:     {"REG_GROUP", 0, ArgNone, "Extract capture group"},
	OpREG_GROUP_END: {"REG_GROUP_END", 0, ArgNone, "End position of capture"},

	OpHATVAN_TRAP:   {"HATVAN_TRAP", 1, ArgImm8U, "Hatvan OS / OS-9 syscall trap"},
	OpSYS_POLL_FLAG: {"SYS_POLL_FLAG", 1, ArgImm8U, "Poll hardware status line"},
	OpSYS_DMA_COPY:  {"SYS_DMA_COPY", 0, ArgNone, "Cross-task DMA copy"},
	OpSYS_EXIT:      {"SYS_EXIT", 0, ArgNone, "Exit process with status code"},
	OpIO_PRINT:      {"IO_PRINT", 0, ArgNone, "Print string slice with newline"},
	OpPRINT:         {"PRINT", 0, ArgNone, "Formatted print without newline"},
	OpPRINTLN:       {"PRINTLN", 0, ArgNone, "Formatted print with newline"},
}

var MnemonicToOpcode = map[string]byte{}

func init() {
	for op, info := range OpcodeTable {
		MnemonicToOpcode[info.Mnemonic] = op
	}
	// Aliases
	MnemonicToOpcode["OS_CALL"] = OpHATVAN_TRAP
	MnemonicToOpcode["TRAP"] = OpHATVAN_TRAP
	MnemonicToOpcode["PEEK"] = OpPEEK2
	MnemonicToOpcode["POKE"] = OpPOKE2
	MnemonicToOpcode["PEEKW"] = OpPEEK2
	MnemonicToOpcode["POKEW"] = OpPOKE2
	MnemonicToOpcode["PEEKB"] = OpPEEK1
	MnemonicToOpcode["POKEB"] = OpPOKE1
	MnemonicToOpcode["ADDR_OF_LOCAL"] = OpADDR_OF_LOCAL
	MnemonicToOpcode["ADDROFLOCAL"] = OpADDR_OF_LOCAL
	MnemonicToOpcode["SHL1ADD"] = OpSHL1_ADD
	MnemonicToOpcode["INDEX2"] = OpSHL1_ADD
}
