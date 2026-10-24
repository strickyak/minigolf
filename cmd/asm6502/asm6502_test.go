package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAsm6502AddressingModes(t *testing.T) {
	src := `
; Test all 13 MOS 6502 addressing modes
ZP_PTR  EQU $10
IO_PUTC EQU $FF00

        ORG $0200
start:
        CLD                 ; D8 (Implied)
        LDX #$FF            ; A2 FF (Immediate)
        TXS                 ; 9A (Implied)
        LDA #<start         ; A9 00 (Immediate low byte)
        LDY #>start         ; A0 02 (Immediate high byte)
        STA ZP_PTR          ; 85 10 (Zero Page)
        STY ZP_PTR+1        ; 84 11 (Zero Page)
        LDA ZP_PTR,X        ; B5 10 (Zero Page,X)
        LDX $20,Y           ; B6 20 (Zero Page,Y)
        STX $20,Y           ; 96 20 (Zero Page,Y)
        STA IO_PUTC         ; 8D 00 FF (Absolute little-endian)
        LDA $1234,X         ; BD 34 12 (Absolute,X)
        LDA $1234,Y         ; B9 34 12 (Absolute,Y)
        LDA (ZP_PTR,X)      ; A1 10 (Indexed Indirect)
        STA (ZP_PTR),Y      ; 91 10 (Indirect Indexed)
        ASL A               ; 0A (Accumulator explicit)
        LSR                 ; 4A (Accumulator implicit)
        BNE start           ; D0 E1 (Relative backward)
        JMP ($FFFC)         ; 6C FC FF (Indirect)
        JSR subr            ; 20 27 02 (Absolute JSR)
subr:
        RTS                 ; 60 (Implied)
        DW  start, $ABCD    ; 00 02 CD AB (Little-endian words)
        END start
`
	asm := NewAssembler()
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var raw bytes.Buffer
	if err := asm.EmitRaw(&raw); err != nil {
		t.Fatalf("EmitRaw failed: %v", err)
	}

	expected := []byte{
		0xD8,       // CLD
		0xA2, 0xFF, // LDX #$FF
		0x9A,       // TXS
		0xA9, 0x00, // LDA #<start
		0xA0, 0x02, // LDY #>start
		0x85, 0x10, // STA $10
		0x84, 0x11, // STY $11
		0xB5, 0x10, // LDA $10,X
		0xB6, 0x20, // LDX $20,Y
		0x96, 0x20, // STX $20,Y
		0x8D, 0x00, 0xFF, // STA $FF00
		0xBD, 0x34, 0x12, // LDA $1234,X
		0xB9, 0x34, 0x12, // LDA $1234,Y
		0xA1, 0x10, // LDA ($10,X)
		0x91, 0x10, // STA ($10),Y
		0x0A,       // ASL A
		0x4A,       // LSR
		0xD0, 0xDD, // BNE start ($0200 - $0223 = -35 = 0xDD)
		0x6C, 0xFC, 0xFF, // JMP ($FFFC)
		0x20, 0x29, 0x02, // JSR subr ($0229)
		0x60,                   // RTS
		0x00, 0x02, 0xCD, 0xAB, // DW start, $ABCD
	}

	if !bytes.Equal(raw.Bytes(), expected) {
		t.Fatalf("Raw bytes mismatch:\nGot:      % X\nExpected: % X", raw.Bytes(), expected)
	}
}

func TestAsm6502DECBOutputAndBranchRelaxation(t *testing.T) {
	src := `
        ORG $0200
cstart:
        BEQ far_target      ; Should relax to BNE *+5 (D0 03) + JMP far_target (4C 90 02)
        DS  135
far_target:
        LDA #'5'
        STA $FF00
        RTS
`
	asm := NewAssembler()
	if err := asm.LoadSource("relax.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var decb bytes.Buffer
	if err := asm.EmitDECB(&decb); err != nil {
		t.Fatalf("EmitDECB failed: %v", err)
	}
	out := decb.Bytes()

	// Check Hatvan DECB magic header: 0xFD 0x00 0x00 'x' '5'
	if len(out) < 5 || out[0] != 0xFD || out[1] != 0x00 || out[2] != 0x00 || out[3] != 'x' || out[4] != '5' {
		t.Fatalf("Invalid DECB magic header: % X", out[:5])
	}

	// Check relaxed branch at start of data block (offset 10..15)
	// far_target is at $0200 + 5 + 135 = $028C
	expectedBranch := []byte{0xD0, 0x03, 0x4C, 0x8C, 0x02}
	if !bytes.Equal(out[10:15], expectedBranch) {
		t.Fatalf("Branch relaxation mismatch: got % X, want % X", out[10:15], expectedBranch)
	}

	// Check DECB trailer: 0xFF 0x00 0x00 0x02 0x00
	trailer := out[len(out)-5:]
	expectedTrailer := []byte{0xFF, 0x00, 0x00, 0x02, 0x00}
	if !bytes.Equal(trailer, expectedTrailer) {
		t.Fatalf("DECB trailer mismatch: got % X, want % X", trailer, expectedTrailer)
	}
}
