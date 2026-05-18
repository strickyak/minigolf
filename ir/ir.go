package ir

import (
	"fmt"
    "log"
	"strconv"
	"strings"
)

// Type represents a primitive or composite type in the IR.
type Type string

const (
	TypeUnknown Type = ""
	TypeByte    Type = "byte"
	TypeWord    Type = "word"
	TypeInt          Type = "int"
	TypeUint         Type = "uint"
	TypeConstInteger Type = "const_integer"
	TypeVoid         Type = "void"
)

func (t Type) IsAPointer() bool {
	return strings.HasPrefix(string(t), "*")
}

func (t Type) PointedType() Type {
	if !t.IsAPointer() {
		panic("PointedType called on non-pointer type: " + t)
	}
	return Type(string(t)[1:])
}

func (t Type) IsAnArray() bool {
	return strings.HasPrefix(string(t), "[")
}

func (t Type) ArrayElementType() Type {
	if !t.IsAnArray() {
		panic("ArrayElementType called on non-array type: " + t)
	}
	idx := strings.Index(string(t), "]")
	return Type(string(t)[idx+1:])
}

func (t Type) IsAStruct() bool {
	return strings.HasPrefix(string(t), "struct{")
}

func GetTypeSize(typ Type) int {
	if typ == "byte" {
		return 1
	}
	if typ == "word" || typ == "int" || typ == "uint" || typ == "const_integer" {
		return 2
	}
	if typ.IsAnArray() {
		str := string(typ)
		idx := strings.Index(str, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(str[1:idx])
			eltSize := GetTypeSize(typ.ArrayElementType())
			return length * eltSize
		}
	}
	if typ.IsAStruct() {
		content := string(typ)[7 : len(string(typ))-1]
		size := 0
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				fTyp := content[start:i]
				size += GetTypeSize(Type(fTyp))
				start = i + 1
			}
		}
		return size
	}
	if typ.IsAPointer() {
		return 2
	}
	// Default to trying as a string if no match
	if string(typ) == "byte" {
		return 1
	}
    log.Panicf("GetTypeSize: unknown type: %q", typ)
    panic(0)
}

func GetEltSize(arrType Type) int {
	if arrType.IsAnArray() {
		return GetTypeSize(arrType.ArrayElementType())
	}
    log.Panicf("GetEltSize: not an array: %q", arrType)
    panic(0)
}

func (t Type) String() string {
	if t == "" {
		return "unknown"
	}
	return string(t)
}

// Value is an interface for anything that can be an operand.
type Value interface {
	Type() Type
	String() string
}

// Program represents the entire compilation unit in SSA form.
type Program struct {
	Globals      []*Global
	Functions    []*Function
	TypeDefs     map[string]string
	TypeDefOrder []string
}

// Function represents a single function in SSA form.
type Function struct {
	Name       string
	Parameters []*Parameter
	ReturnType Type
	Blocks     []*BasicBlock
}

// Parameter represents a function parameter.
type Parameter struct {
	ID   int
	Name string
	Typ  Type
}

func (p *Parameter) Type() Type     { return p.Typ }
func (p *Parameter) String() string { return fmt.Sprintf("p%d", p.ID) }

// BasicBlock is a sequence of non-branching instructions ending in a terminator.
type BasicBlock struct {
	ID           int
	Instructions []Instruction
	Terminator   Terminator
	Predecessors []*BasicBlock
	Successors   []*BasicBlock
}

// Global represents a global variable in the flat namespace.
type Global struct {
	Name string
	Typ  Type
}

func (g *Global) Type() Type     { return g.Typ }
func (g *Global) String() string { return fmt.Sprintf("@%s", g.Name) }

// StringLiteral represents a string constant (mostly for print builtins).
type StringLiteral struct {
	Value string
}

func (s *StringLiteral) Type() Type     { return TypeUnknown }
func (s *StringLiteral) String() string { return fmt.Sprintf("%q", s.Value) }

// Instruction is the interface for all SSA operations.
type Instruction interface {
	Value
	Opcode() string
	SetID(int)
	GetID() int
	GetComment() string
	SetComment(c string)
}

// Terminator represents an instruction that safely ends a BasicBlock.
type Terminator interface {
	Instruction
	IsTerminator()
}

// BaseInstruction provides common Instruction fields.
type BaseInstruction struct {
	ID      int
	Typ     Type
	Comment string
}

func (b *BaseInstruction) Type() Type          { return b.Typ }
func (b *BaseInstruction) String() string      { return fmt.Sprintf("v%d", b.ID) }
func (b *BaseInstruction) SetID(id int)        { b.ID = id }
func (b *BaseInstruction) GetID() int          { return b.ID }
func (b *BaseInstruction) GetComment() string  { return b.Comment }
func (b *BaseInstruction) SetComment(c string) { b.Comment = c }

// --- Constant Instructions ---

type ConstByte struct {
	BaseInstruction
	Val uint8
}

func (i *ConstByte) Opcode() string { return "const_byte" }

type ConstWord struct {
	BaseInstruction
	Val uint64
}

func (i *ConstWord) Opcode() string { return "const_word" }

type Sizeof struct {
	BaseInstruction
	TargetTyp Type
}

func (i *Sizeof) Opcode() string { return "sizeof" }
func (i *Sizeof) String() string { return fmt.Sprintf("sizeof(%s)", i.TargetTyp) }

// --- Memory Operations ---

type Load struct {
	BaseInstruction
	Global *Global
}

func (i *Load) Opcode() string { return "load" }

type Store struct {
	BaseInstruction
	Global *Global
	Val    Value
}

func (i *Store) Opcode() string { return "store" }

// --- Arithmetic & Logic Operations ---

type BinaryOp struct {
	BaseInstruction
	Op    string // "add", "sub", "mul", "div", "mod", "and", "or", "xor", "shl", "shr"
	Left  Value
	Right Value
}

func (i *BinaryOp) Opcode() string { return i.Op }

type Compare struct {
	BaseInstruction        // Comparison returns a byte (0 or 1)
	Op              string // "eq", "neq", "lt", "lte", "gt", "gte"
	Left            Value
	Right           Value
}

func (i *Compare) Opcode() string { return i.Op }

type UnaryOp struct {
	BaseInstruction
	Op      string // "not", "neg"
	Operand Value
}

func (i *UnaryOp) Opcode() string { return i.Op }

// --- Array Operations ---

type ExtractElement struct {
	BaseInstruction
	Array Value
	Index Value
}

func (i *ExtractElement) Opcode() string { return "extract" }

type InsertElement struct {
	BaseInstruction
	Array Value
	Index Value
	Val   Value
}

func (i *InsertElement) Opcode() string { return "insert" }

// --- Struct Operations ---

type ExtractField struct {
	BaseInstruction
	Struct     Value
	FieldIndex int
}

func (i *ExtractField) Opcode() string { return "extract_field" }

type InsertField struct {
	BaseInstruction
	Struct     Value
	FieldIndex int
	Val        Value
}

func (i *InsertField) Opcode() string { return "insert_field" }

type AddressOfGlobal struct {
	BaseInstruction
	Global *Global
}

func (i *AddressOfGlobal) Opcode() string { return "addrof" }

type AddressOfLocal struct {
	BaseInstruction
	Local Value
}

func (i *AddressOfLocal) Opcode() string { return "addrof_local" }

type AddressOfField struct {
	BaseInstruction
	Ptr        Value
	FieldIndex int
}

func (i *AddressOfField) Opcode() string { return "addrof_field" }

type AddressOfElement struct {
	BaseInstruction
	ArrayPtr Value
	Index    Value
}

func (i *AddressOfElement) Opcode() string { return "addrof_element" }

type ExtractFieldPtr struct {
	BaseInstruction
	Ptr        Value
	FieldIndex int
}

func (i *ExtractFieldPtr) Opcode() string { return "extract_field_ptr" }

type InsertFieldPtr struct {
	BaseInstruction
	Ptr        Value
	FieldIndex int
	Val        Value
}

func (i *InsertFieldPtr) Opcode() string { return "insert_field_ptr" }

type LoadPtr struct {
	BaseInstruction
	Ptr Value
}

func (i *LoadPtr) Opcode() string { return "load_ptr" }

type StorePtr struct {
	BaseInstruction
	Ptr Value
	Val Value
}

func (i *StorePtr) Opcode() string { return "store_ptr" }

type ZeroInit struct {
	BaseInstruction
}

func (i *ZeroInit) Opcode() string { return "zeroinit" }

type SourceMarker struct {
	BaseInstruction
	Comment string
}

func (i *SourceMarker) Opcode() string { return "source_marker" }

// --- SSA Primitives ---

type Phi struct {
	BaseInstruction
	Edges []PhiEdge
}
type PhiEdge struct {
	Block *BasicBlock
	Value Value
}

func (i *Phi) Opcode() string { return "phi" }

// --- Function Calls ---

type Call struct {
	BaseInstruction
	Func *Function
	Args []Value
}

func (i *Call) Opcode() string { return "call" }

type BuiltinCall struct {
	BaseInstruction
	Name string // "print", "println"
	Args []Value
}

func (i *BuiltinCall) Opcode() string { return "builtin_" + i.Name }

// --- Type Conversions ---

type Cast struct {
	BaseInstruction
	Op      string // "zero_ext", "trunc"
	Operand Value
}

func (i *Cast) Opcode() string { return i.Op }

// --- Terminators ---

type Jump struct {
	BaseInstruction
	Target *BasicBlock
}

func (i *Jump) Opcode() string { return "jmp" }
func (i *Jump) IsTerminator()  {}

type Branch struct {
	BaseInstruction
	Condition  Value
	TrueBlock  *BasicBlock
	FalseBlock *BasicBlock
}

func (i *Branch) Opcode() string { return "br" }
func (i *Branch) IsTerminator()  {}

type Return struct {
	BaseInstruction
	Val Value // nil if void
}

func (i *Return) Opcode() string { return "ret" }
func (i *Return) IsTerminator()  {}
