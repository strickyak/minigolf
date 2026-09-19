package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAssemblerBasic(t *testing.T) {
	src := `
		org $8000
cstart:
		lds #$8000
		clra
		clrb
		tfr d,x
		lbsr _main
stuck:
		bra stuck
_main:
		ldb #42
		rts
		end cstart
`
	asm := NewAssembler()
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var decbBuf bytes.Buffer
	if err := asm.EmitDECB(&decbBuf); err != nil {
		t.Fatalf("EmitDECB failed: %v", err)
	}

	data := decbBuf.Bytes()
	if len(data) < 5 {
		t.Fatalf("DECB output too short: %d bytes", len(data))
	}
	if data[0] != 0x00 {
		t.Errorf("Expected chunk header 0x00, got 0x%02X", data[0])
	}
}

func TestBranchRelaxation(t *testing.T) {
	// lbsr to nearby function should relax to bsr (2 bytes instead of 3)
	// lbeq to nearby target should relax to beq (2 bytes instead of 4)
	src := `
		org $8000
start:
		lbeq target
		lbsr helper
		rts
helper:
		rts
target:
		rts
`
	asm := NewAssembler()
	asm.enableRelax = true
	if err := asm.LoadSource("relax.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Verify that lbeq was relaxed to beq (opcode $27) and lbsr to bsr (opcode $8D)
	foundBEQ := false
	foundBSR := false
	for _, stmt := range asm.statements {
		if stmt.Mnemonic == "lbeq" {
			if len(stmt.Encoded) == 2 && stmt.Encoded[0] == 0x27 {
				foundBEQ = true
			} else {
				t.Errorf("Expected lbeq to relax to 2-byte beq ($27), got % X", stmt.Encoded)
			}
		}
		if stmt.Mnemonic == "lbsr" {
			if len(stmt.Encoded) == 2 && stmt.Encoded[0] == 0x8D {
				foundBSR = true
			} else {
				t.Errorf("Expected lbsr to relax to 2-byte bsr ($8D), got % X", stmt.Encoded)
			}
		}
	}
	if !foundBEQ {
		t.Errorf("lbeq statement not relaxed to beq")
	}
	if !foundBSR {
		t.Errorf("lbsr statement not relaxed to bsr")
	}
}

func TestDirectPageRelaxation(t *testing.T) {
	// Variable at address 16 ($10) should use direct page addressing ($DC 10 for ldd, $DD 10 for std)
	// instead of extended addressing ($FC 00 10 / $FD 00 10)
	src := `
v_count	equ	16
v_far	equ	$1000

		org $8000
		ldd v_count
		std v_count
		ldd v_far
`
	asm := NewAssembler()
	asm.enableRelax = true
	asm.dp = 0x00
	if err := asm.LoadSource("dp.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	for _, stmt := range asm.statements {
		if stmt.Mnemonic == "ldd" && stmt.RawOp == "v_count" {
			if len(stmt.Encoded) != 2 || stmt.Encoded[0] != 0xDC || stmt.Encoded[1] != 0x10 {
				t.Errorf("Expected ldd v_count to relax to direct page [$DC 10], got % X", stmt.Encoded)
			}
		}
		if stmt.Mnemonic == "std" && stmt.RawOp == "v_count" {
			if len(stmt.Encoded) != 2 || stmt.Encoded[0] != 0xDD || stmt.Encoded[1] != 0x10 {
				t.Errorf("Expected std v_count to relax to direct page [$DD 10], got % X", stmt.Encoded)
			}
		}
		if stmt.Mnemonic == "ldd" && stmt.RawOp == "v_far" {
			if len(stmt.Encoded) != 3 || stmt.Encoded[0] != 0xFC {
				t.Errorf("Expected ldd v_far to remain extended [$FC 10 00], got % X", stmt.Encoded)
			}
		}
	}
}

func TestListingGeneration(t *testing.T) {
	src := `
		org $8000
cstart:
		ldb #42
		rts
`
	asm := NewAssembler()
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var listBuf bytes.Buffer
	if err := asm.EmitListing(&listBuf); err != nil {
		t.Fatalf("EmitListing failed: %v", err)
	}

	listOut := listBuf.String()
	if !strings.Contains(listOut, "8000  C62A") {
		t.Errorf("Expected '8000  C62A' in listing, got:\n%s", listOut)
	}
}
