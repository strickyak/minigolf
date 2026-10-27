package main

import (
	"bytes"
	"strings"
	"testing"
)

func assembleRaw(t *testing.T, src string) []byte {
	t.Helper()
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
	return buf.Bytes()
}

func assembleDECB(t *testing.T, src string) []byte {
	t.Helper()
	asm := NewAssembler()
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	var buf bytes.Buffer
	if err := asm.EmitDECB(&buf); err != nil {
		t.Fatalf("EmitDECB failed: %v", err)
	}
	return buf.Bytes()
}

func TestAsm8086BasicEncodings(t *testing.T) {
	src := `
	SEGMENT CODE
	ORG $0200
cstart:
	CLI
	CLD
	XOR AX, AX
	MOV DS, AX
	MOV SS, AX
	MOV ES, AX
	MOV SP, $FE00
	PUSH BP
	MOV BP, SP
	SUB SP, 16
	MOV AX, [BP+4]
	MOV [BP-2], AX
	MOV AL, [BP-3]
	MOV [BP], AX
	MOV AX, [BP+256]
	LEA SI, [BP-8]
	LEA DI, [BX+SI+4]
	MOV BYTE PTR [BX], 0
	MOV WORD PTR [BP-4], $1234
	MUL DX
	DIV BX
	IDIV BX
	CWD
	SHL AX, 1
	SHR AX, CL
	SAR AX, CL
	REP MOVSB
	INT $30
	DB  $06
	IRET
	MOV SP, BP
	POP BP
	RET
`
	got := assembleRaw(t, src)
	want := []byte{
		0xFA,             // CLI
		0xFC,             // CLD
		0x33, 0xC0,       // XOR AX, AX (or 31 C0; 03+8*6 = 0x33 C0)
		0x8E, 0xD8,       // MOV DS, AX
		0x8E, 0xD0,       // MOV SS, AX
		0x8E, 0xC0,       // MOV ES, AX
		0xBC, 0x00, 0xFE, // MOV SP, $FE00
		0x55,                   // PUSH BP
		0x8B, 0xEC,             // MOV BP, SP
		0x83, 0xEC, 0x10,       // SUB SP, 16
		0x8B, 0x46, 0x04,       // MOV AX, [BP+4]
		0x89, 0x46, 0xFE,       // MOV [BP-2], AX
		0x8A, 0x46, 0xFD,       // MOV AL, [BP-3]
		0x89, 0x46, 0x00,       // MOV [BP], AX (Mod=01, RM=6, disp8=0)
		0x8B, 0x86, 0x00, 0x01, // MOV AX, [BP+256] (Mod=10, RM=6, disp16=256)
		0x8D, 0x76, 0xF8,       // LEA SI, [BP-8]
		0x8D, 0x78, 0x04,       // LEA DI, [BX+SI+4]
		0xC6, 0x07, 0x00,       // MOV BYTE PTR [BX], 0
		0xC7, 0x46, 0xFC, 0x34, 0x12, // MOV WORD PTR [BP-4], $1234
		0xF7, 0xE2,       // MUL DX
		0xF7, 0xF3,       // DIV BX
		0xF7, 0xFB,       // IDIV BX
		0x99,             // CWD
		0xD1, 0xE0,       // SHL AX, 1
		0xD3, 0xE8,       // SHR AX, CL
		0xD3, 0xF8,       // SAR AX, CL
		0xF3, 0xA4,       // REP MOVSB
		0xCD, 0x30,       // INT $30
		0x06,             // DB $06
		0xCF,             // IRET
		0x8B, 0xE5,       // MOV SP, BP
		0x5D,             // POP BP
		0xC3,             // RET
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("Encoding mismatch:\ngot:  % X\nwant: % X", got, want)
	}
}

func TestAsm8086BranchRelaxation(t *testing.T) {
	src := `
	SEGMENT CODE
	ORG $0200
start:
	CMP AX, 0
	JE  near_label
	NOP
near_label:
	CMP AX, 1
	JE  far_label
	DS  200, $90
far_label:
	RET
`
	got := assembleRaw(t, src)
	// CMP AX, 0 -> 83 F8 00 (3 bytes)
	// JE near_label -> 74 01 (2 bytes, skips 1 NOP)
	// NOP -> 90 (1 byte)
	// CMP AX, 1 -> 83 F8 01 (3 bytes)
	// JE far_label -> relaxed to JNE $+5 (75 03) + JMP NEAR far_label (E9 C8 00) (5 bytes, skips 200 NOPs)
	if len(got) != 3+2+1+3+5+200+1 {
		t.Fatalf("unexpected total length %d, want %d", len(got), 3+2+1+3+5+200+1)
	}
	if got[3] != 0x74 || got[4] != 0x01 {
		t.Errorf("expected short JE (74 01), got %02X %02X", got[3], got[4])
	}
	if got[9] != 0x75 || got[10] != 0x03 || got[11] != 0xE9 || got[12] != 0xC8 || got[13] != 0x00 {
		t.Errorf("expected relaxed JE (75 03 E9 C8 00), got % X", got[9:14])
	}
}

func TestAsm8086MultiSegmentDECB(t *testing.T) {
	src := `
	SEGMENT CODE
	ORG $0200
cstart:
	MOV AL, [v_msg]
	MOV [$FF00], AL
	RET

	SEGMENT DATA
	ORG $0400
v_msg:
	DB "A", 0
	END cstart
`
	decb := assembleDECB(t, src)
	want := []byte{
		// 1. Magic header: FD 00 00 'x' '0'
		0xFD, 0x00, 0x00, 'x', '0',
		// 2. Code segment High16 = $0001: FE 00 00 00 01
		0xFE, 0x00, 0x00, 0x00, 0x01,
		// 3. Code chunk at $0200 (len = 7): MOV AL, [$0400] (A0 00 04); MOV [$FF00], AL (A2 00 FF); RET (C3)
		0x00, 0x00, 0x07, 0x02, 0x00,
		0xA0, 0x00, 0x04,
		0xA2, 0x00, 0xFF,
		0xC3,
		// 4. Data segment High16 = $0000: FE 00 00 00 00
		0xFE, 0x00, 0x00, 0x00, 0x00,
		// 5. Data chunk at $0400 (len = 2): "A", 0
		0x00, 0x00, 0x02, 0x04, 0x00,
		'A', 0x00,
		// 6. Restore Code segment High16 = $0001 before END/EXEC
		0xFE, 0x00, 0x00, 0x00, 0x01,
		// 7. Postamble: FF 00 00 02 00
		0xFF, 0x00, 0x00, 0x02, 0x00,
	}
	if !bytes.Equal(decb, want) {
		t.Fatalf("DECB mismatch:\ngot:  % X\nwant: % X", decb, want)
	}
}
