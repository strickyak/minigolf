package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAssemblerBasic(t *testing.T) {
	src := `
		org $1000
cstart:
		move.l  #$00080000, sp
		moveq   #0, d0
		move.l  #42, d1
		add.l   d1, d0
		lea     $00FF000A, a0
		move.w  d0, (a0)
		stop    #$2700
`
	asm := NewAssembler()
	if err := asm.LoadSource("test.s", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitSRecords(&buf); err != nil {
		t.Fatalf("EmitSRecords failed: %v", err)
	}

	srec := buf.String()
	if !strings.Contains(srec, "S3") || !strings.Contains(srec, "S7") {
		t.Fatalf("Expected S3 and S7 records in output, got:\n%s", srec)
	}
}

func TestAssemblerExtendedInstructions(t *testing.T) {
	src := `
		org $1000
		movem.l d0-d7/a0-a6, -(sp)
		movem.l (sp)+, d0-d7/a0-a6
		trap    #0
		trap    #15
		move.l  usp, a0
		move.l  a1, usp
		move.w  sr, d0
		move.w  d0, sr
		move.w  d0, ccr
		andi.w  #$0700, sr
		ori.w   #$2000, sr
		btst    #5, 60(sp)
		btst    d0, d1
		bset    #3, (a0)
		bclr    d2, (a1)
		bchg    #1, d3
		exg     d0, d1
		exg     a0, a1
		exg     d2, a3
		seq     d0
		sne     (a0)
`
	asm := NewAssembler()
	if err := asm.LoadSource("ext.s", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
}

