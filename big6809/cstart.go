package big6809

// CStartTemplate contains the fixed runtime in Slot 6 ($C000..$DFFF)
// and Direct Page definitions in Slot 0 ($0000..$1FFF).
const CStartTemplate = `
; ==============================================================================
; EMBIGGEN 6809 Runtime & Process Initialization
; Conforms to Hatvan OS EMBIGGEN Specification (doc/embiggen-process-mode.md)
; ==============================================================================

	pragma cescapes

; --- Slot 0 Direct Page Allocations ($0000..$00FF) ---
active_win_0    equ $0002   ; Block ID mapped in Slot 2 ($FF42)
active_win_1    equ $0003   ; Block ID mapped in Slot 3 ($FF43)
active_win_2    equ $0004   ; Block ID mapped in Slot 4 ($FF44)
next_win_slot   equ $0005   ; Round-robin eviction pointer (0, 1, or 2)
active_code_blk equ $0006   ; Current block mapped in Slot 5 ($FF45)
far_ret_sp      equ $0008   ; 16-bit pointer to Far Return Stack frame
far_ret_tmp     equ $000A   ; 16-bit scratch in Direct Page for far return PC
far_ret_stack   equ $0100   ; 512-byte Far Return Stack in Slot 0 ($0100..$02FF)

; --- Slot 6 Fixed Runtime Entry Point ($C000) ---
	org $C000

cstart_embiggen:
    ; 1. Initialize Process Stack in Slot 1 ($3FFE growing down to $2000)
    lds   #$3FFE

    ; 2. Initialize Direct Page register to Slot 0 ($0000)
    clra
    tfr   a,dp

    ; Initialize Far Return Stack in Slot 0
    ldx   #far_ret_stack
    stx   <far_ret_sp

    ; 3. Setup Initial 8KB MMAP Vector at $FF40..$FF47
    ;    Slot 0: Block 0 (Fixed Data / DP)
    ;    Slot 1: Block 1 (Fixed Stack)
    ;    Slot 2: Block 2 (Far Data Window 0)
    ;    Slot 3: Block 3 (Far Data Window 1)
    ;    Slot 4: Block 4 (Far Data Window 2)
    ;    Slot 5: Block 8 (Active Far Code Block)
    ;    Slot 6: Block 6 (Fixed Runtime & Trampolines)
    ;    Slot 7: Block 7 (Fixed System / I/O)
    clr   $FF40
    lda   #1
    sta   $FF41
    lda   #2
    sta   $FF42
    lda   #3
    sta   $FF43
    lda   #4
    sta   $FF44
    lda   #8
    sta   $FF45
    lda   #6
    sta   $FF46
    lda   #7
    sta   $FF47

    ; 4. Initialize Far Data Window Manager Cache in Direct Page
    lda   #2
    sta   <active_win_0
    lda   #3
    sta   <active_win_1
    lda   #4
    sta   <active_win_2
    clr   <next_win_slot
    lda   #8
    sta   <active_code_blk

    ; 5. Ensure Slot 5 is mapped to entry block (Block 8) and call main()
    lda   #8
    sta   $FF45
    sta   <active_code_blk

    jsr   f_main__main
    bra   __exit0

f_main:
    jmp   f_main__main

__exit0:
    clra
    clrb
    tfr   d,x

__exit:
    tfr   x,d
    stb   $FF05        ; Hatvan Exit Port ($FF05)
.stuck:
    bra   .stuck

; --- Far Call Dispatcher (Slot 6) ---
; Input: B = Target Block ID (8..127), X = Virtual Entry Address in Slot 5 ($A000..$BFFF)
; On entry, 0,s on process stack holds caller_return_pc.
; We push (caller_block_id, caller_return_pc) to far_ret_stack, replace 0,s with
; __far_return_trampoline, map target block into Slot 5, and jump to callee.
__far_call_dispatcher:
    pshs  x                     ; Save target entry address on stack
    ldx   <far_ret_sp           ; X points to next free slot in Far Return Stack
    lda   $FF45                 ; A = caller's currently active block ID
    sta   ,x+                   ; Save caller block ID (1 byte)

    stb   $FF45                 ; Map callee's block into Slot 5 NOW!
    stb   <active_code_blk

    ldd   2,s                   ; D = caller return PC (past saved X)
    std   ,x++                  ; Save caller return PC (2 bytes)
    stx   <far_ret_sp           ; Update far_ret_sp

    ; Replace caller return PC at 2,s with address of __far_return_trampoline
    ldd   #__far_return_trampoline
    std   2,s

    puls  x                     ; Restore target entry address
    jmp   ,x                    ; Jump directly to callee in Slot 5!

; --- Far Return Trampoline (Slot 6) ---
; Callee returns here via standard RTS!
; Register D holds return value.
; We pop (caller_block_id, caller_return_pc) from far_ret_stack, restore caller's block
; into Slot 5 ($FF45), preserve D, and jump to caller_return_pc.
__far_return_trampoline:
    pshs  d                     ; Preserve return value D (2 bytes on S)
    ldd   <far_ret_sp
    subd  #3
    std   <far_ret_sp
    tfr   d,x                   ; X points to this Far Return frame

    lda   ,x                    ; A = caller's block ID
    sta   $FF45                 ; Restore caller's block into Slot 5
    sta   <active_code_blk

    ldx   1,x                   ; X = caller return PC
    stx   <far_ret_tmp          ; Save caller return PC in DP ($000A)

    puls  d                     ; Restore return value D
    jmp   [far_ret_tmp]         ; Indirect jump to caller in Slot 5!

; --- Far Data 3-Window Round-Robin Manager (Slot 6) ---
; Input: B = Target Far Data Block ID (128..255)
; Output: X = Virtual Base Address of mapped window ($4000, $6000, or $8000)
__far_resolve_window:
    cmpb  <active_win_0
    beq   .win0
    cmpb  <active_win_1
    beq   .win1
    cmpb  <active_win_2
    beq   .win2

    ; Cache Miss: Evict slot indicated by next_win_slot
    lda   <next_win_slot
    cmpa  #1
    beq   .evict1
    cmpa  #2
    beq   .evict2

.evict0:
    stb   <active_win_0
    stb   $FF42
    lda   #1
    sta   <next_win_slot
    ldx   #$4000
    rts

.evict1:
    stb   <active_win_1
    stb   $FF43
    lda   #2
    sta   <next_win_slot
    ldx   #$6000
    rts

.evict2:
    stb   <active_win_2
    stb   $FF44
    clr   <next_win_slot
    ldx   #$8000
    rts

.win0:
    ldx   #$4000
    rts

.win1:
    ldx   #$6000
    rts

.win2:
    ldx   #$8000
    rts

; --- 8-Byte Slice Helpers ---
; Slice descriptor on stack:
;   far_ref (2B), offset (2B), length (2B), capacity (2B)

; __slice_get_byte:
;   Input: X points to 8-byte slice, Y = element index
;   Output: A = byte value
__slice_get_byte:
    ldd   ,x                ; D = far_ref
    beq   .near_get_byte    ; If far_ref == 0: Near / Fixed RAM fast path

    ; Far Data Path:
    ; far_ref (word) has block_id in B (128..255)
    pshs  x,y
    bsr   __far_resolve_window ; X = Window base ($4000, $6000, $8000)
    puls  y                 ; Y = original slice pointer (was X)
    ldd   2,y               ; D = slice.offset
    leax  d,x               ; Add slice offset to window base
    puls  y                 ; Y = element index (was Y)
    tfr   y,d
    leax  d,x               ; Add index
    lda   ,x                ; Load byte
    rts

.near_get_byte:
    ; Near Fast Path: VAddr = slice.offset + index
    ldd   2,x               ; D = slice.offset (virtual address)
    tfr   y,x               ; X = index
    leax  d,x               ; X = offset + index
    lda   ,x                ; Load byte directly
    rts

; --- Basic Builtins and IO ---

f_prelude__putchar:
f_putchar:
    ldb   3,s          ; character parameter from stack
putchar:
_putchar:
    stb   $FF00        ; Hatvan console output port
    rts

getchar:
_getchar:
f_getchar:
f_prelude__getchar:
    ldb   $FF01        ; Hatvan console input port
    clra
    rts

_printf:
    leax  2,s
    fcb   $12,$21,111  ; Hyper Printf
    rts

__fmt_d:
    fcc   "%d"
    fcb   0
__fmt_u:
    fcc   "%u"
    fcb   0

; --- Core Arithmetic Helpers (Slot 6) ---

__mul16:
    pshs  d,x
    lda   1,s
    ldb   3,s
    mul
    tfr   d,x
    lda   0,s
    ldb   3,s
    mul
    tfr   b,a
    clrb
    leax  d,x
    lda   1,s
    ldb   2,s
    mul
    tfr   b,a
    clrb
    leax  d,x
    tfr   x,d
    leas  4,s
    rts

_div0_msg:
    fdb   0, _div0_text, 16, 16
_div0_text:
    fcc   "division by zero"
    fcb   0

__divmod16:
    cmpd  #0
    beq   __div0_error
    pshs  u,d
    ldu   #16
    clra
    clrb
.L_divloop:
    exg   d,x
    aslb
    rola
    exg   d,x
    rolb
    rola
    cmpd  ,s
    blo   .L_divnosub
    subd  ,s
    leax  1,x
.L_divnosub:
    leau  -1,u
    cmpu  #0
    bne   .L_divloop
    leas  2,s
    puls  u,pc

__div0_error:
    ldx   #_div0_msg
    jsr   builtin_panic
    rts

__div16:
    lbsr  __divmod16
    tfr   x,d
    rts

__mod16:
    lbsr  __divmod16
    rts

__shl16:
    cmpx  #0
    beq   .shl_done
.shl_loop:
    aslb
    rola
    leax  -1,x
    cmpx  #0
    bne   .shl_loop
.shl_done:
    rts

__shr16:
    cmpx  #0
    beq   .shr_done
.shr_loop:
    lsra
    rorb
    leax  -1,x
    cmpx  #0
    bne   .shr_loop
.shr_done:
    rts


; Prints an 8-byte string slice:
;   Input: X = pointer to 8-byte slice descriptor
builtin_print_string:
    pshs  u,y
    tfr   x,u               ; U = pointer to 8-byte slice
    ldd   4,u               ; D = length
    beq   .print_done
    ldy   #0                ; Y = index (0)
.print_loop:
    pshs  y
    tfr   u,x
    bsr   __slice_get_byte  ; A = byte
    puls  y
    tfr   a,b
    jsr   putchar
    leay  1,y
    cmpy  4,u               ; Compare with length
    bne   .print_loop
.print_done:
    puls  u,y
    rts

; Prints an 8-byte string slice followed by a newline:
;   Input: X = pointer to 8-byte slice descriptor (or 0 for newline only)
builtin_println:
    cmpx  #0
    beq   .newline_only
    bsr   builtin_print_string
.newline_only:
    ldb   #10               ; Newline '\n'
    jmp   putchar

builtin_panic:
    cmpx  #0
    beq   .panic_no_arg
    bsr   builtin_println
.panic_no_arg:
    ldx   #1
    jmp   __exit

builtin_exit:
    tfr   d,x
    jmp   __exit

builtin__propagate_panic_:
builtin__unlink_jmp_:
    rts

; --- EMBIGGEN String Compare Helper (Slot 6) ---
; Input: X = pointer to 8-byte slice A, Y = pointer to 8-byte slice B
; Output: B = 1 if equal, 0 if not equal; Condition Codes (Z/NZ) set according to B
__far_streq:
    ; 1. Compare lengths (offset 4 in descriptor)
    ldd   4,x               ; D = len(A)
    subd  4,y               ; compare with len(B)
    bne   .streq_false      ; if lengths differ, not equal

    ldd   4,x               ; D = len(A)
    beq   .streq_true       ; if both len == 0, equal

    ; 2. Iterate index 0 .. len-1
    pshs  u
    ldu   #0                ; U = current index (0)
.streq_loop:
    pshs  x,y
    tfr   u,y               ; Y = index
    bsr   __slice_get_byte  ; A = byte from slice A
    puls  x,y
    pshs  a                 ; Save byte from A

    pshs  x,y
    tfr   y,x               ; X = slice B
    tfr   u,y               ; Y = index
    bsr   __slice_get_byte  ; A = byte from slice B
    puls  x,y
    cmpa  ,s+               ; compare A (from B) with byte from A on stack
    bne   .streq_diff

    leau  1,u               ; index++
    cmpu  4,x               ; reached len?
    bne   .streq_loop

    puls  u
.streq_true:
    clra
    ldb   #1
    tstb                    ; Set NZ flag
    rts

.streq_diff:
    puls  u
.streq_false:
    clra
    clrb                    ; Set Z flag
    rts
`
