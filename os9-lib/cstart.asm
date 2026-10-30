; ==============================================================================
; MiniGolf 6809 Position-Independent Runtime for Microware OS-9 / NitrOS-9
; ==============================================================================

	pragma cescapes

; --- Direct Page Runtime Scratch ($0000..$001F) ---
os9_args_ptr equ $000C   ; 16-bit pointer to command line args (X at entry)
os9_args_len equ $000E   ; 16-bit parameter area size (D at entry)
os9_mem_end  equ $0010   ; 16-bit top of process memory (Y at entry)

cstart:
	; Save OS-9 entry registers in Direct Page ($000C..$0011)
	stx   <os9_args_ptr
	std   <os9_args_len
	sty   <os9_mem_end

	; Initialize Y to point to the Direct Page base (data area base)
	tfr   dp,a
	clrb
	tfr   d,y

	; Zero global variables area (from $0020 to _globals_size)
	leax  32,y
	ldd   #_globals_size
	subd  #32
	bls   .cstart_after_zero
.cstart_zero_loop:
	clr   ,x+
	subd  #1
	bne   .cstart_zero_loop
.cstart_after_zero:

	lbsr  _main

__exit0:
	clrb
__exit:
	os9   F$Exit
.os9_stuck:
	bra   .os9_stuck

; --- Basic Console I/O and Printf (OS-9 via SWI2) ---

f_prelude__putchar:
f_putchar:
putchar:
_putchar:
	pshs  d,x,y,u
	stb   ,-s          ; push character on stack
	leax  ,s           ; X points to character
	ldy   #1           ; 1 byte
	lda   #1           ; path 1 (stdout)
	os9   I$Write
	leas  1,s          ; pop character
	puls  d,x,y,u,pc

getchar:
_getchar:
f_getchar:
f_prelude__getchar:
	pshs  x,y,u
	leas  -1,s         ; buffer for 1 byte
	leax  ,s
	ldy   #1
	clra               ; path 0 (stdin)
	os9   I$Read
	bcs   .os9_getchar_eof
	ldb   ,s
	clra
	leas  1,s
	puls  x,y,u,pc
.os9_getchar_eof:
	ldd   #-1          ; EOF
	leas  1,s
	puls  x,y,u,pc

_printf:
	pshs  d,x,y,u
	ldx   10,s         ; format string pointer
	leau  12,s         ; pointer to first variadic argument on stack
.printf_loop:
	ldb   ,x+
	beq   .printf_done
	cmpb  #'%'
	beq   .printf_fmt
	bsr   putchar
	bra   .printf_loop
.printf_fmt:
	ldb   ,x+
	beq   .printf_done
	cmpb  #'s'
	beq   .printf_s
	cmpb  #'d'
	beq   .printf_d
	cmpb  #'u'
	beq   .printf_u
	cmpb  #'c'
	beq   .printf_c
	bsr   putchar
	bra   .printf_loop
.printf_c:
	ldd   ,u++
	bsr   putchar
	bra   .printf_loop
.printf_s:
	ldy   ,u++         ; string pointer (null-terminated)
	beq   .printf_loop
.printf_s_loop:
	ldb   ,y+
	beq   .printf_loop
	bsr   putchar
	bra   .printf_s_loop
.printf_d:
	ldd   ,u++         ; signed 16-bit value
	bpl   .printf_u_val
	pshs  d
	ldb   #45          ; ASCII '-'
	bsr   putchar
	puls  d
	coma
	comb
	addd  #1
	bra   .printf_u_val
.printf_u:
	ldd   ,u++         ; unsigned 16-bit value
.printf_u_val:
	bsr   __print_u16_d
	bra   .printf_loop
.printf_done:
	puls  d,x,y,u,pc

__print_u16_d:
	pshs  d,x,y,u
	clra
	clrb
	pshs  d            ; 0,s = digit count (1B), 1,s = leading zero flag (1B)
	ldd   2,s          ; original D from pshs d,x,y,u
	pshs  d            ; 0,s = running value (2B), 2,s = digit count (1B), 3,s = leading zero flag (1B)
	leau  __pow10_table,pcr
.u16_next_div:
	ldy   ,u++
	beq   .u16_done
	clr   2,s
.u16_sub_loop:
	ldd   0,s
	subd  -2,u
	bcs   .u16_digit_done
	std   0,s
	inc   2,s
	bra   .u16_sub_loop
.u16_digit_done:
	lda   2,s
	bne   .u16_emit
	tst   3,s
	bne   .u16_emit
	cmpy  #1
	bne   .u16_next_div
.u16_emit:
	inc   3,s
	adda  #'0'
	tfr   a,b
	lbsr  putchar
	bra   .u16_next_div
.u16_done:
	leas  4,s
	puls  d,x,y,u,pc

__pow10_table:
	fdb   10000
	fdb   1000
	fdb   100
	fdb   10
	fdb   1
	fdb   0

f_prelude__mul_byte:
	lda   2,s
	ldb   3,s
	mul
	tfr   d,x
	rts

; ==============================================================================
; Low-Level Subroutines for os9.golf (Microware OS-9 / NitrOS-9)
; Every system call returns byte errnum in B (0 if CC.C=0, B if CC.C=1).
; ==============================================================================

f_os9__SysArgsPtr:
	ldd   <os9_args_ptr
	rts

f_os9__SysArgsLen:
	ldd   <os9_args_len
	rts

f_os9__SysMemEnd:
	ldd   <os9_mem_end
	rts

; SysOpen(path *byte, mode word, outPath *byte) byte
f_os9__SysOpen:
	pshs  y,u
	ldx   6,s          ; path pointer
	lda   9,s          ; mode
	os9   I$Open
	bcs   .os9_ret_err
	ldx   10,s         ; outPath
	sta   ,x
	clra
	clrb
	puls  y,u,pc

; SysCreate(path *byte, mode word, attrs word, outPath *byte) byte
f_os9__SysCreate:
	pshs  y,u
	ldx   6,s          ; path pointer
	lda   9,s          ; mode
	ldb   11,s         ; attrs
	os9   I$Create
	bcs   .os9_ret_err
	ldx   12,s         ; outPath
	sta   ,x
	clra
	clrb
	puls  y,u,pc

; SysRead(path word, buf *byte, maxLen word, outRead *word) byte
f_os9__SysRead:
	pshs  y,u
	lda   7,s          ; path
	ldx   8,s          ; buf
	ldy   10,s         ; maxLen
	os9   I$Read
	bcs   .os9_io_err
	ldx   12,s         ; outRead
	sty   ,x
	clra
	clrb
	puls  y,u,pc

; SysReadLn(path word, buf *byte, maxLen word, outRead *word) byte
f_os9__SysReadLn:
	pshs  y,u
	lda   7,s          ; path
	ldx   8,s          ; buf
	ldy   10,s         ; maxLen
	os9   I$ReadLn
	bcs   .os9_io_err
	ldx   12,s         ; outRead
	sty   ,x
	clra
	clrb
	puls  y,u,pc

; SysWrite(path word, buf *byte, count word, outWritten *word) byte
f_os9__SysWrite:
	pshs  y,u
	lda   7,s          ; path
	ldx   8,s          ; buf
	ldy   10,s         ; count
	os9   I$Write
	bcs   .os9_io_err
	ldx   12,s         ; outWritten
	sty   ,x
	clra
	clrb
	puls  y,u,pc

; SysWritLn(path word, buf *byte, count word, outWritten *word) byte
f_os9__SysWritLn:
	pshs  y,u
	lda   7,s          ; path
	ldx   8,s          ; buf
	ldy   10,s         ; count
	os9   I$WritLn
	bcs   .os9_io_err
	ldx   12,s         ; outWritten
	sty   ,x
	clra
	clrb
	puls  y,u,pc

.os9_io_err:
	ldx   12,s         ; outRead / outWritten = 0 on error
	clr   ,x
	clr   1,x
.os9_ret_err:
	clra               ; B holds OS-9 error code
	puls  y,u,pc

; SysClose(path word) byte
f_os9__SysClose:
	pshs  y,u
	lda   7,s          ; path
	os9   I$Close
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysDup(path word, outPath *byte) byte
f_os9__SysDup:
	pshs  y,u
	lda   7,s          ; path
	os9   I$Dup
	bcs   .os9_ret_err
	ldx   8,s          ; outPath
	sta   ,x
	clra
	clrb
	puls  y,u,pc

; SysDelete(path *byte) byte
f_os9__SysDelete:
	pshs  y,u
	ldx   6,s          ; path
	os9   I$Delete
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysDeletX(path *byte, mode word) byte
f_os9__SysDeletX:
	pshs  y,u
	ldx   6,s          ; path
	lda   9,s          ; mode
	os9   I$DeletX
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysMakDir(path *byte, attrs word) byte
f_os9__SysMakDir:
	pshs  y,u
	ldx   6,s          ; path
	ldb   9,s          ; attrs
	os9   I$MakDir
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysChgDir(path *byte, mode word) byte
f_os9__SysChgDir:
	pshs  y,u
	ldx   6,s          ; path
	lda   9,s          ; mode
	os9   I$ChgDir
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysSeek(path word, posHi word, posLo word) byte
f_os9__SysSeek:
	pshs  y,u
	lda   7,s          ; path
	ldx   8,s          ; posHi (MSW)
	ldu   10,s         ; posLo (LSW)
	os9   I$Seek
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysGetStt(path word, code word, outX *word, outU *word) byte
f_os9__SysGetStt:
	pshs  y,u
	lda   7,s          ; path
	ldb   9,s          ; code
	ldx   12,s         ; outU ptr
	ldu   ,x           ; initial U value
	ldx   10,s         ; outX ptr
	ldx   ,x           ; initial X value
	os9   I$GetStt
	bcs   .os9_ret_err
	pshs  u            ; save returned U
	ldu   12,s         ; outX ptr (shifted by 2 due to pshs u)
	stx   ,u           ; *outX = returned X
	ldu   14,s         ; outU ptr (shifted by 2 due to pshs u)
	puls  x            ; X = returned U
	stx   ,u           ; *outU = returned U
	clra
	clrb
	puls  y,u,pc

; SysSetStt(path word, code word, regX word, regU word) byte
f_os9__SysSetStt:
	pshs  y,u
	lda   7,s          ; path
	ldb   9,s          ; code
	ldx   10,s         ; regX
	ldu   12,s         ; regU
	os9   I$SetStt
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysFork(modName *byte, params *byte, paramSize word, langType word, memPages word, outChildID *byte) byte
f_os9__SysFork:
	pshs  y,u
	ldx   6,s          ; modName
	ldu   8,s          ; params
	ldy   10,s         ; paramSize
	lda   13,s         ; langType
	ldb   15,s         ; memPages
	os9   F$Fork
	bcs   .os9_ret_err
	ldx   16,s         ; outChildID
	sta   ,x
	clra
	clrb
	puls  y,u,pc

; SysWait(outChildID *byte, outStatus *byte) byte
f_os9__SysWait:
	pshs  y,u
	os9   F$Wait
	bcs   .os9_ret_err
	ldx   6,s          ; outChildID
	sta   ,x
	ldx   8,s          ; outStatus
	stb   ,x
	clra
	clrb
	puls  y,u,pc

; SysChain(modName *byte, params *byte, paramSize word, langType word, memPages word) byte
f_os9__SysChain:
	pshs  y,u
	ldx   6,s          ; modName
	ldu   8,s          ; params
	ldy   10,s         ; paramSize
	lda   13,s         ; langType
	ldb   15,s         ; memPages
	os9   F$Chain
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysMem(requested word, outSize *word, outUpper *word) byte
f_os9__SysMem:
	pshs  y,u
	ldd   6,s          ; requested size
	os9   F$Mem
	bcs   .os9_ret_err
	ldx   8,s          ; outSize
	std   ,x
	ldx   10,s         ; outUpper
	sty   ,x
	clra
	clrb
	puls  y,u,pc

; SysSend(procID word, signal word) byte
f_os9__SysSend:
	pshs  y,u
	lda   7,s          ; procID
	ldb   9,s          ; signal
	os9   F$Send
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysSleep(ticks word, outRemaining *word) byte
f_os9__SysSleep:
	pshs  y,u
	ldx   6,s          ; ticks
	os9   F$Sleep
	bcs   .os9_ret_err
	tfr   x,d
	ldx   8,s          ; outRemaining
	std   ,x
	clra
	clrb
	puls  y,u,pc

; SysID(outProcID *byte, outUserID *word) byte
f_os9__SysID:
	pshs  y,u
	os9   F$ID
	bcs   .os9_ret_err
	ldx   6,s          ; outProcID
	sta   ,x
	ldx   8,s          ; outUserID
	sty   ,x
	clra
	clrb
	puls  y,u,pc

; SysTime(buf *byte) byte
f_os9__SysTime:
	pshs  y,u
	ldx   6,s          ; buf (6 bytes)
	os9   F$Time
	bcs   .os9_ret_err
	clra
	clrb
	puls  y,u,pc

; SysExit(status word)
f_os9__SysExit:
	ldb   3,s          ; status
	os9   F$Exit
.os9_exit_stuck:
	bra   .os9_exit_stuck
