package bigir

import (
	"fmt"
	"strings"
)

// TypeKind identifies the category of a BIGIR type.
type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindVoid
	KindNoReturn
	KindBool
	KindByte
	KindWord
	KindInt
	KindNearPtr   // 16-bit direct virtual address ($0000..$FFFF)
	KindFarRef    // 16-bit Far Reference (block 128..255, chunk 0..511)
	KindFarSlice  // 8-byte slice: {far_ref, offset, length, capacity}
	KindFarString // 8-byte string alias for FarSlice[byte]
	KindFarFunc   // Far function reference {block_id, entry_point}
	KindArray     // Fixed-size array [N]T
	KindStruct    // Struct {field1, field2, ...}
)

// Field represents a named field in a struct type.
type Field struct {
	Name   string
	Type   Type
	Offset int
}

// Type represents a data type in BIGIR.
type Type struct {
	Kind        TypeKind
	Name        string
	Size        int   // Size in bytes
	ElementType *Type // For Slice, Array, NearPtr
	ArrayLen    int   // For Array
	Fields      []Field // For Struct
}

func (t Type) String() string {
	if t.Name != "" {
		return t.Name
	}
	switch t.Kind {
	case KindVoid:
		return "void"
	case KindNoReturn:
		return "noreturn"
	case KindBool:
		return "bool"
	case KindByte:
		return "byte"
	case KindWord:
		return "word"
	case KindInt:
		return "int"
	case KindNearPtr:
		if t.ElementType != nil {
			return "*" + t.ElementType.String()
		}
		return "*near"
	case KindFarRef:
		return "far_ref"
	case KindFarSlice:
		if t.ElementType != nil {
			return "slice[" + t.ElementType.String() + "]"
		}
		return "slice"
	case KindFarString:
		return "string"
	case KindFarFunc:
		return "far_func"
	case KindArray:
		if t.ElementType != nil {
			return fmt.Sprintf("[%d]%s", t.ArrayLen, t.ElementType.String())
		}
		return fmt.Sprintf("[%d]unknown", t.ArrayLen)
	case KindStruct:
		var sb strings.Builder
		sb.WriteString("struct{")
		for i, f := range t.Fields {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(f.Name)
			sb.WriteString(" ")
			sb.WriteString(f.Type.String())
		}
		sb.WriteString("}")
		return sb.String()
	default:
		return "unknown"
	}
}

var (
	TypeVoid     = Type{Kind: KindVoid, Name: "void", Size: 0}
	TypeNoReturn = Type{Kind: KindNoReturn, Name: "noreturn", Size: 0}
	TypeBool     = Type{Kind: KindBool, Name: "bool", Size: 1}
	TypeByte     = Type{Kind: KindByte, Name: "byte", Size: 1}
	TypeWord     = Type{Kind: KindWord, Name: "word", Size: 2}
	TypeInt      = Type{Kind: KindInt, Name: "int", Size: 2}
	TypeFarRef   = Type{Kind: KindFarRef, Name: "far_ref", Size: 2}
	TypeFarString = Type{
		Kind:        KindFarString,
		Name:        "string",
		Size:        8, // 8 bytes: far_ref (2B), offset (2B), length (2B), capacity (2B)
		ElementType: &TypeByte,
	}
)

// MakeNearPtr creates a 16-bit near pointer type.
func MakeNearPtr(elem Type) Type {
	return Type{
		Kind:        KindNearPtr,
		Name:        "*" + elem.String(),
		Size:        2,
		ElementType: &elem,
	}
}

// MakeFarSlice creates an 8-byte far slice type.
func MakeFarSlice(elem Type) Type {
	if elem.Kind == KindByte {
		return TypeFarString
	}
	return Type{
		Kind:        KindFarSlice,
		Name:        "slice[" + elem.String() + "]",
		Size:        8, // 8 bytes
		ElementType: &elem,
	}
}

// MakeArray creates a fixed array type.
func MakeArray(elem Type, len int) Type {
	return Type{
		Kind:        KindArray,
		Name:        fmt.Sprintf("[%d]%s", len, elem.String()),
		Size:        elem.Size * len,
		ElementType: &elem,
		ArrayLen:    len,
	}
}

// MakeStruct creates a struct type with computed offsets.
func MakeStruct(name string, fields []Field) Type {
	offset := 0
	for i := range fields {
		fields[i].Offset = offset
		offset += fields[i].Type.Size
	}
	return Type{
		Kind:   KindStruct,
		Name:   name,
		Size:   offset,
		Fields: fields,
	}
}
