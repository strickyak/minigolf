package main

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestOS9MfreeModule(t *testing.T) {
	// Extracted from mfree.0128f947b5 listing
	src := `
C$LF equ $0A
C$CR equ $0D
C$SPAC equ $20
D.FMBM equ $0020
I$WritLn equ $8C
F$Exit equ $06

tylg set Prgrm+Objct
atrv set ReEnt+rev
rev set $00
edition set 1
stdout set 1

 mod eom,name,tylg,atrv,start,size

 org 0
fmbegin rmb 2 pointer to start of free memory
fmend rmb 2 pointer to end of free memory
leading rmb 1 the leading digit to print
totalfree rmb 1 total number of free pages
upper rmb 2 upper boundary of free segment
lower rmb 2 lower boundary of free segment
pages rmb 1 number of free pages for a contiguous segment
bufptr rmb 2 pointer to the buffer
buffer rmb 330 buffer space
stack rmb 200 stack size
size equ .

name fcs /mfree/
 fcb edition

header fcb C$LF
 fcc " Address  pages"
 fcb C$LF
 fcc "--------- -----"
 fcb $80+C$CR
totfree fcb C$LF
 fcs "Total pages free = "

start leay buffer,u point Y to the buffer
 sty <bufptr save off the pointer
 leay <header,pcr point Y to the header
 bsr ApndStr append the header to the buffer
 bsr print then print it
 ldx >D.FMBM get the start of the free memory bitmap
 stx <fmbegin store it in a variable
 ldx >D.FMBM+2 get the end of the free memory bitmap
 stx <fmend store it in a variable
 clra clear A
 clrb and B
 sta <totalfree clear this variable
 std <upper clear upper variable
 std <lower clear lower variable
 stb <pages clear page count
 ldx <fmbegin get the free memory bitmap start
nextbyte lda ,x+ get the byte in the bitmap
 bsr checkbits check the bits
 cmpx <fmend are we at the end?
 bcs nextbyte branch if not
 bsr printmemline print a line of contiguous memory
 leay <totfree,pcr point to the total free memory message
 bsr ApndStr append it to the buffer
 ldb <totalfree get the total number of free pages
 bsr bDeci convert it to decimal
 bsr print print the line
 clrb clear carry and error code
 os9 F$Exit then exit
*
checkbits bsr checkbit1@ bsr
checkbit1@ bsr checkbit2@ then bsr again
checkbit2@ bsr checkbit3@ then bsr again
checkbit3@ lsla shift A left to move high bit into the carry
 bcs printmemline branch if the carry is set
 inc <totalfree else increment the total number of free pages
 inc <pages and the number of free pages for a contiguous segment
 inc <upper and the upper page
 rts return to the caller

printmemline pshs b,a save off registers
 ldb <pages get the number of free pages for a contiguous segment
 beq zero@ branch if zero
 ldd <lower else get the lower address of the free block in D
 bsr dHexa convert to hex
 lda #'- '-' char
 bsr ApndA append it to the buffer
 ldd <upper get the upper variable
 subd #$0001 subtract it by 1
 bsr dHexa put it in the buffer
 bsr aspace append a space to buffer
 bsr aspace append a space to buffer
 ldb <pages get the number of free pages for a contiguous segment
 bsr bDeci append it as a decimal in the buffer
 bsr print then print
zero@ inc <upper increment the upper page count
 ldd <upper get the upper page
 std <lower and store it in the lower page
 clr <pages clear out the number of free pages for a contiguous segment
 puls pc,b,a restore the registers and return to the caller
*
ApndStr lda ,y get byte at Y
 anda #$7F clear high byte
 bsr ApndA append the byte to the buffer
 lda ,y+ get the byte again and increment Y
 bpl ApndStr branch if high bit not set
 rts return to the caller
*
print pshs y,x,a save registers
 lda #C$CR add carriage return
 bsr ApndA to the buffer
 leax buffer,u reset the buffer pointer
 stx <bufptr to the start of the buffer
 ldy #80 maximum bytes to write
 lda #stdout to standard output
 os9 I$WritLn then write it
 puls pc,y,x,a restore registers and return to the caller
*
bDeci lda #$FF start out with all bits set in A
 clr <leading clear the leading digit
loop1@ inca increment the byte in A
 subb #100 subtract 100 from B
 bcc loop1@ branch if no carry
 bsr setlead@ now A holds number of 100's
 lda #10 and add
add10@ deca decrement A
 addb #10 and add 10 to B
 bcc add10@ branch if carry is clear
 bsr setlead@ go set the leading digit
 tfr b,a move B to A
 inc <leading increment the leading digit
setlead@ tsta is A zero?
 beq nonzero@ branch if so
 sta <leading else save it as the leading digit
nonzero@ tst <leading is leading digit zero?
 bne addoff@ branch if not
aspace lda #C$SPAC-'0 else make a space
addoff@ adda #'0 offset to "0" in ascii table
 cmpa #'0+10 greater than ASCII numeric?
 bcs ApndA branch if not
 adda #$07 else add 7 to force as a hex digit
*
ApndA pshs x save X onto the stack
 ldx <bufptr get the buffer pointer
 sta ,x+ add the character in A to the buffer
 stx <bufptr update the buffer pointer
 puls pc,x restore registers and return
*
dHexa clr <leading clear the leading digit
 bsr dodigit@ do first digit
 tfr b,a now do second digit
dodigit@ pshs a preserve registers
 lsra position bits 7-4
 lsra into bits 3-0
 lsra and then bits 7-4
 lsra become 0
 bsr convdigit@ convert the digit
 puls a restore the register we pushed earlier
convdigit@ anda #$0F clear the upper 4 bits of A
 bra setlead@ set the leading digit

 emod
eom equ *
 end
`
	asm := NewAssembler()
	asm.os9Mode = true
	if err := asm.LoadSource("mfree.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	var buf bytes.Buffer
	if err := asm.EmitOS9(&buf); err != nil {
		t.Fatalf("EmitOS9 failed: %v", err)
	}
	out := buf.Bytes()
	if len(out) != 0x0128 {
		t.Fatalf("expected module size 0x0128 (296), got 0x%04X (%d)", len(out), len(out))
	}

	wantHdr, _ := hex.DecodeString("87cd0128000d1180000048021f")
	if !bytes.Equal(out[:13], wantHdr) {
		t.Errorf("header mismatch:\n got: %x\nwant: %x", out[:13], wantHdr)
	}

	wantCRC, _ := hex.DecodeString("f947b5")
	if !bytes.Equal(out[len(out)-3:], wantCRC) {
		var listBuf bytes.Buffer
		_ = asm.EmitListing(&listBuf)
		t.Errorf("CRC mismatch:\n got: %x\nwant: %x\nlisting:\n%s", out[len(out)-3:], wantCRC, listBuf.String())
	}
}

func TestOS9RelaxUpdatesEomAndPCR(t *testing.T) {
	src := `
 mod eom,name,Prgrm+Objct,ReEnt+1,start,256
name fcs /test/
 fcb 1
msg fcs "hi"
start
 lbra target
 nop
target
 leax msg,pcr
 clrb
 os9 F$Exit
 emod
eom equ *
`
	asm := NewAssembler()
	asm.os9Mode = true
	if err := asm.LoadSource("test.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	var buf bytes.Buffer
	if err := asm.EmitOS9(&buf); err != nil {
		t.Fatalf("EmitOS9 failed: %v", err)
	}
	out := buf.Bytes()

	// Check M$Size in header matches actual output length
	hdrSize := int(out[2])<<8 | int(out[3])
	if hdrSize != len(out) {
		t.Errorf("header M$Size (%d) != actual output length (%d) after lbra/pcr relaxation", hdrSize, len(out))
	}

	// Verify CRC over module verifies to $800063 (standard OS-9 CRC check residue)
	crc := computeOS9CRC(out[:len(out)-3])
	if !bytes.Equal(out[len(out)-3:], crc[:]) {
		t.Errorf("CRC mismatch: got %x, want %x", out[len(out)-3:], crc)
	}
}

func TestOS9WrapperMode(t *testing.T) {
	src := `
cstart:
 clrb
 os9 F$Exit
_globals_size equ 128
`
	asm := NewAssembler()
	asm.os9Mode = true
	asm.os9Name = "hello"
	if err := asm.LoadSource("hello.asm", strings.NewReader(src)); err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}
	if err := asm.Assemble(); err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	var buf bytes.Buffer
	if err := asm.EmitOS9(&buf); err != nil {
		t.Fatalf("EmitOS9 failed: %v", err)
	}
	out := buf.Bytes()

	// 13 header + 5 ("hello") + 1 (edition) + 4 (clrb + os9 F$Exit) + 3 (CRC) = 26 bytes
	if len(out) != 26 {
		t.Fatalf("expected 26 bytes, got %d", len(out))
	}
	mMem := int(out[11])<<8 | int(out[12])
	if mMem != 128+1024 {
		t.Errorf("expected M$Mem = %d, got %d", 128+1024, mMem)
	}
}
