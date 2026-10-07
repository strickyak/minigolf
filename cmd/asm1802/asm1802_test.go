package main

import (
	"bytes"
	"strings"
	"testing"
)

func assembleString(t *testing.T, src string) ([]byte, *Assembler) {
	t.Helper()
	asm := NewAssembler()
	err := asm.LoadSource("test.asm", strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	err = asm.Assemble()
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	err = asm.EmitRaw(&buf)
	if err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}
	return buf.Bytes(), asm
}

func TestRegisterOps(t *testing.T) {
	src := `
	ORG $1000
	INC R0
	INC R15
	INC RF
	DEC R2
	LDA R3
	STR R4
	GLO R5
	GHI R6
	PLO R7
	PHI R8
	SEP RE
	SEX R2
	LDN R1
`
	bin, _ := assembleString(t, src)
	expected := []byte{
		0x10,       // INC R0
		0x1F,       // INC R15
		0x1F,       // INC RF
		0x22,       // DEC R2
		0x43,       // LDA R3
		0x54,       // STR R4
		0x85,       // GLO R5
		0x96,       // GHI R6
		0xA7,       // PLO R7
		0xB8,       // PHI R8
		0xDE,       // SEP RE
		0xE2,       // SEX R2
		0x01,       // LDN R1
	}

	if !bytes.Equal(bin, expected) {
		t.Fatalf("Register ops mismatch:\nGot:  %X\nWant: %X", bin, expected)
	}
}

func TestALUAndImmediateOps(t *testing.T) {
	src := `
	ORG $0000
	LDI $42
	ADI 10
	SDI 5
	SMI 1
	ANI $F0
	ORI $0F
	XRI $FF
	ADCI 0
	SDBI 0
	SMBI 0
	LDX
	ADD
	SD
	SM
	AND
	OR
	XOR
	ADC
	SDB
	SMB
	SHR
	SHL
	SHRC
	SHLC
	LDXA
	STXD
`
	bin, _ := assembleString(t, src)
	expected := []byte{
		0xF8, 0x42, // LDI $42
		0xFC, 0x0A, // ADI 10
		0xFD, 0x05, // SDI 5
		0xFF, 0x01, // SMI 1
		0xFA, 0xF0, // ANI $F0
		0xF9, 0x0F, // ORI $0F
		0xFB, 0xFF, // XRI $FF
		0x7C, 0x00, // ADCI 0
		0x7D, 0x00, // SDBI 0
		0x7F, 0x00, // SMBI 0
		0xF0,       // LDX
		0xF4,       // ADD
		0xF5,       // SD
		0xF7,       // SM
		0xF2,       // AND
		0xF1,       // OR
		0xF3,       // XOR
		0x74,       // ADC
		0x75,       // SDB
		0x77,       // SMB
		0xF6,       // SHR
		0xFE,       // SHL
		0x76,       // SHRC
		0x7E,       // SHLC
		0x72,       // LDXA
		0x73,       // STXD
	}

	if !bytes.Equal(bin, expected) {
		t.Fatalf("ALU ops mismatch:\nGot:  %X\nWant: %X", bin, expected)
	}
}

func TestBranchesAndSkips(t *testing.T) {
	src := `
	ORG $0200
target:
	NOP
	BR target
	BZ target
	BNZ target
	BDF target
	BNF target
	BQ target
	BNQ target
	B1 target
	BN1 target
	SKP
	LBR $1234
	LBZ $5678
	LSKP
	LSZ
	LSNZ
`
	bin, _ := assembleString(t, src)
	expected := []byte{
		0xC4,             // NOP
		0x30, 0x00,       // BR $0200
		0x32, 0x00,       // BZ $0200
		0x3A, 0x00,       // BNZ $0200
		0x33, 0x00,       // BDF $0200
		0x3B, 0x00,       // BNF $0200
		0x31, 0x00,       // BQ $0200
		0x39, 0x00,       // BNQ $0200
		0x34, 0x00,       // B1 $0200
		0x3C, 0x00,       // BN1 $0200
		0x38,             // SKP
		0xC0, 0x12, 0x34, // LBR $1234
		0xC2, 0x56, 0x78, // LBZ $5678
		0xC8,             // LSKP
		0xCE,             // LSZ
		0xC6,             // LSNZ
	}

	if !bytes.Equal(bin, expected) {
		t.Fatalf("Branches mismatch:\nGot:  %X\nWant: %X", bin, expected)
	}
}

func TestShortBranchOutOfPage(t *testing.T) {
	src := `
	ORG $0100
target:
	ORG $0200
	BR target
`
	asm := NewAssembler()
	asm.relaxEnabled = false // disable relaxation to test strict short branch page verification
	_ = asm.LoadSource("test.asm", strings.NewReader(src))
	err := asm.Assemble()
	if err == nil {
		t.Fatalf("Expected error for out-of-page short branch, got nil")
	}
	if !strings.Contains(err.Error(), "outside current page") {
		t.Fatalf("Expected 'outside current page' error, got: %v", err)
	}
}

func TestBranchRelaxation(t *testing.T) {
	// Program with:
	// 1. LBR on same page -> should shorten to BR (2 bytes: 30 xx)
	// 2. LBR across page boundary -> should remain LBR (3 bytes: C0 xx xx)
	// 3. JMP on same page -> should shorten to BR (2 bytes: 30 xx)
	src := `
	ORG $0100
start:
	LBR near_target     ; same page -> BR (2 bytes)
	JMP near_target     ; same page -> BR (2 bytes)
	LBR far_target      ; out of page -> LBR (3 bytes)
near_target:
	NOP
	ORG $0250
far_target:
	IDL
`
	bin, asm := assembleString(t, src)
	// near_target is at:
	// start = $0100
	// LBR near_target: relaxed to BR -> 2 bytes ($0100..$0101)
	// JMP near_target: relaxed to BR -> 2 bytes ($0102..$0103)
	// LBR far_target: stays LBR -> 3 bytes ($0104..$0106)
	// near_target is at $0107!
	expectedNear := uint32(0x0107)
	if asm.symbols["near_target"] != expectedNear {
		t.Fatalf("near_target symbol address mismatch: got $%04X, want $%04X",
			asm.symbols["near_target"], expectedNear)
	}

	expectedFar := uint32(0x0250)
	if asm.symbols["far_target"] != expectedFar {
		t.Fatalf("far_target symbol address mismatch: got $%04X, want $%04X",
			asm.symbols["far_target"], expectedFar)
	}

	// First 8 bytes of binary should be:
	// 30 07 (BR $0107)
	// 30 07 (BR $0107)
	// C0 02 50 (LBR $0250)
	// C4 (NOP)
	expectedFirst8 := []byte{
		0x30, 0x07,
		0x30, 0x07,
		0xC0, 0x02, 0x50,
		0xC4,
	}
	if !bytes.Equal(bin[:8], expectedFirst8) {
		t.Fatalf("Relaxed branch encoding mismatch:\nGot:  %X\nWant: %X", bin[:8], expectedFirst8)
	}
}

func TestPseudoInstructionsAndExpressions(t *testing.T) {
	src := `
MY_VAL EQU $1234
	ORG $0100
	LOAD R3, MY_VAL
	LDI HIGH(MY_VAL)
	LDI LOW(MY_VAL)
	LDI >MY_VAL
	LDI <MY_VAL
	LDI MY_VAL.1
	LDI MY_VAL.0
`
	bin, _ := assembleString(t, src)
	expected := []byte{
		// LOAD R3, $1234:
		0xF8, 0x12, // LDI $12
		0xB3,       // PHI R3
		0xF8, 0x34, // LDI $34
		0xA3,       // PLO R3
		// LDI HIGH(MY_VAL)
		0xF8, 0x12,
		// LDI LOW(MY_VAL)
		0xF8, 0x34,
		// LDI >MY_VAL
		0xF8, 0x12,
		// LDI <MY_VAL
		0xF8, 0x34,
		// LDI MY_VAL.1
		0xF8, 0x12,
		// LDI MY_VAL.0
		0xF8, 0x34,
	}

	if !bytes.Equal(bin, expected) {
		t.Fatalf("Pseudo ops mismatch:\nGot:  %X\nWant: %X", bin, expected)
	}
}

func TestDirectivesAndData(t *testing.T) {
	src := `
	ORG $0100
	DB $01, $02, "ABC", 10
	DW $1234, $5678
	ASCIZ "Hi"
	ALIGN 4
	DS 2
`
	bin, _ := assembleString(t, src)
	expected := []byte{
		0x01, 0x02, 'A', 'B', 'C', 10,
		0x12, 0x34, 0x56, 0x78,
		'H', 'i', 0x00,
		0x00, 0x00, 0x00, // padded by ALIGN 4 (current offset was 13 -> next mult of 4 is 16: 3 pad bytes)
		0x00, 0x00, // DS 2
	}

	if !bytes.Equal(bin, expected) {
		t.Fatalf("Data directives mismatch:\nGot:  %X\nWant: %X", bin, expected)
	}
}

func TestDECBFormat(t *testing.T) {
	src := `
	ORG $0100
start:
	LDI $99
	OUT 6
	IDL
	END start
`
	asm := NewAssembler()
	_ = asm.LoadSource("test.asm", strings.NewReader(src))
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitDECB(&buf); err != nil {
		t.Fatalf("EmitDECB failed: %v", err)
	}

	out := buf.Bytes()
	// Check magic header: FD 00 00 'x' 'c'
	if len(out) < 5 || out[0] != 0xFD || out[3] != 'x' || out[4] != 'c' {
		t.Fatalf("Invalid DECB header: %X", out[:5])
	}

	// Check data block header: tag 0x00, len 4 (LDI $99 (2) + OUT 6 (1) + IDL (1)), addr $0100
	expectedDataHdr := []byte{0x00, 0x00, 0x04, 0x01, 0x00}
	if !bytes.Equal(out[5:10], expectedDataHdr) {
		t.Fatalf("Invalid data header:\nGot:  %X\nWant: %X", out[5:10], expectedDataHdr)
	}

	// Payload: F8 99 66 00
	expectedPayload := []byte{0xF8, 0x99, 0x66, 0x00}
	if !bytes.Equal(out[10:14], expectedPayload) {
		t.Fatalf("Invalid payload:\nGot:  %X\nWant: %X", out[10:14], expectedPayload)
	}

	// Exec block: tag 0xFF, len 0x0000, entry $0100
	expectedExecHdr := []byte{0xFF, 0x00, 0x00, 0x01, 0x00}
	if !bytes.Equal(out[14:19], expectedExecHdr) {
		t.Fatalf("Invalid exec header:\nGot:  %X\nWant: %X", out[14:19], expectedExecHdr)
	}
}
