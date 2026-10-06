package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAssemblerBasic(t *testing.T) {
	src := `
		org $1000
start:
		ld   sp, $8000
		ld   a, 42
		ld   hl, $FF00
		ld   (hl), a
		out  (0x60), a
		defb $84
		retn
`
	asm := NewAssembler()
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitRaw(&buf); err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}

	data := buf.Bytes()
	// Expected:
	// ld sp, $8000 -> 31 00 80
	// ld a, 42     -> 3E 2A
	// ld hl, $FF00 -> 21 00 FF
	// ld (hl), a   -> 77
	// out (0x60), a-> D3 60
	// defb $84     -> 84
	// retn         -> ED 45
	expected := []byte{
		0x31, 0x00, 0x80,
		0x3E, 0x2A,
		0x21, 0x00, 0xFF,
		0x77,
		0xD3, 0x60,
		0x84,
		0xED, 0x45,
	}

	if !bytes.Equal(data, expected) {
		t.Fatalf("Binary mismatch.\nGot:      %X\nExpected: %X", data, expected)
	}
}

func TestAssemblerBranchRelaxation(t *testing.T) {
	// A short forward jump should relax to JR (2 bytes: 18 disp)
	src := `
		org $0100
start:
		jmp  target
		nop
		nop
target:
		ret
`
	asm := NewAssembler()
	if err := asm.LoadSource("test_relax.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitRaw(&buf); err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}

	data := buf.Bytes()
	// jmp target -> 18 02 (disp = 0x0104 - 0x0102 = 2)
	// nop        -> 00
	// nop        -> 00
	// ret        -> C9
	expected := []byte{
		0x18, 0x02,
		0x00,
		0x00,
		0xC9,
	}

	if !bytes.Equal(data, expected) {
		t.Fatalf("Branch relaxation mismatch.\nGot:      %X\nExpected: %X", data, expected)
	}
}

func TestAssemblerLongBranch(t *testing.T) {
	// A jump target that exceeds 127 bytes should be encoded as JP (3 bytes: C3 lo hi)
	src := `
		org $0100
start:
		jmp  target
		defs 200, 0
target:
		ret
`
	asm := NewAssembler()
	if err := asm.LoadSource("test_long.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitRaw(&buf); err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}

	data := buf.Bytes()
	// Target is at 0x0100 + 3 + 200 = 0x01CB.
	// jmp target -> C3 CB 01
	if len(data) < 3 || data[0] != 0xC3 || data[1] != 0xCB || data[2] != 0x01 {
		t.Fatalf("Expected JP target (C3 CB 01), got: %X", data[:3])
	}
}

func TestAssemblerIndexedAndMath(t *testing.T) {
	src := `
		org $2000
		ld   a, (ix+4)
		ld   (iy-2), b
		add  hl, de
		add  ix, bc
		inc  hl
		dec  ix
		push ix
		pop  iy
		ex   de, hl
		ex   af, af'
		ldir
`
	asm := NewAssembler()
	if err := asm.LoadSource("test_math.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitRaw(&buf); err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}

	data := buf.Bytes()
	// ld a, (ix+4)   -> DD 7E 04
	// ld (iy-2), b   -> FD 70 FE
	// add hl, de     -> 19
	// add ix, bc     -> DD 09
	// inc hl         -> 23
	// dec ix         -> DD 2B
	// push ix        -> DD E5
	// pop iy         -> FD E1
	// ex de, hl      -> EB
	// ex af, af'     -> 08
	// ldir           -> ED B0
	expected := []byte{
		0xDD, 0x7E, 0x04,
		0xFD, 0x70, 0xFE,
		0x19,
		0xDD, 0x09,
		0x23,
		0xDD, 0x2B,
		0xDD, 0xE5,
		0xFD, 0xE1,
		0xEB,
		0x08,
		0xED, 0xB0,
	}

	if !bytes.Equal(data, expected) {
		t.Fatalf("Instruction mismatch.\nGot:      %X\nExpected: %X", data, expected)
	}
}

func TestAssemblerDECBOutput(t *testing.T) {
	src := `
		org $0500
start:
		ld   a, 1
		ret
		end  start
`
	asm := NewAssembler()
	if err := asm.LoadSource("test_decb.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitDECB(&buf); err != nil {
		t.Fatalf("EmitDECB failed: %v", err)
	}

	decb := buf.Bytes()
	// Tag 253, len 0, 'x' 'z': FD 00 00 78 7A
	// Tag 0, len 3, addr $0500: 00 00 03 05 00
	// Data: 3E 01 C9
	// Tag 255, len 0, entry $0500: FF 00 00 05 00
	expected := []byte{
		0xFD, 0x00, 0x00, 'x', 'z',
		0x00, 0x00, 0x03, 0x05, 0x00,
		0x3E, 0x01, 0xC9,
		0xFF, 0x00, 0x00, 0x05, 0x00,
	}

	if !bytes.Equal(decb, expected) {
		t.Fatalf("DECB mismatch.\nGot:      %X\nExpected: %X", decb, expected)
	}
}
