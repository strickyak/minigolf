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

    ; Initialize panic and jmp_chain to 0
    ldd   #0
    std   v_prelude___jmp_chain_
    std   v_prelude___panic_

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
    ; Map slice.far_ref into Slot 2 ($4000) directly (4 cycles!)
    stb   $FF42             ; Map block into Slot 2 ($4000..$5FFF)
    ldd   2,x               ; D = slice.offset
    addd  #$4000            ; Add Slot 2 window base
    tfr   y,x               ; X = index
    leax  d,x               ; X = $4000 + slice.offset + index
    lda   ,x                ; Load byte
    rts

.near_get_byte:
    ; Near Fast Path: VAddr = slice.offset + index
    ldd   2,x               ; D = slice.offset (virtual address)
    tfr   y,x               ; X = index
    leax  d,x               ; X = offset + index
    lda   ,x                ; Load byte directly
    rts

; __slice_to_ptr:
;   Input: X points to 8-byte slice descriptor {far_ref, offset, len, cap}
;   Output: D = 16-bit virtual pointer.
;           If far_ref != 0: maps far_ref into Slot 2 ($FF42) and adds $4000.
;           If far_ref == 0: returns offset directly (near RAM / rodata).
__slice_to_ptr:
    ldd   ,x                ; D = far_ref
    beq   .near_slice_ptr
    stb   $FF42             ; Map far_ref into Slot 2 ($4000..$5FFF)
    ldd   2,x               ; D = offset
    addd  #$4000            ; Add Slot 2 window base
    rts
.near_slice_ptr:
    ldd   2,x               ; D = offset (near virtual address)
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
    pshs  d,x,y,u
    ldx   10,s          ; format string pointer
    lda   ,x+           ; first char
    cmpa  #'%'
    bne   .printf_done
    lda   ,x+           ; format specifier char
    cmpa  #'s'
    beq   .printf_s
    cmpa  #'d'
    beq   .printf_d
    cmpa  #'u'
    beq   .printf_u
    bra   .printf_done

.printf_s:
    ldx   12,s          ; string pointer (null-terminated)
    beq   .printf_done
.printf_s_loop:
    ldb   ,x+
    beq   .printf_done
    jsr   putchar
    bra   .printf_s_loop

.printf_d:
    ldd   12,s          ; signed 16-bit value
    bpl   .printf_u_val
    pshs  d
    ldb   #45           ; ASCII '-'
    jsr   putchar
    puls  d
    coma
    comb
    addd  #1
    bra   .printf_u_val

.printf_u:
    ldd   12,s          ; unsigned 16-bit value
.printf_u_val:
    jsr   __print_u16_d

.printf_done:
    puls  d,x,y,u
    rts

__print_u16_d:
    ; Input: D = 16-bit unsigned integer
    pshs  d,x,y,u       ; Save caller registers (8B)
    clra
    clrb
    pshs  d             ; 0,s = digit count (1B), 1,s = leading zero flag (1B)
    ldd   2,s           ; D = original value to print (was pushed in pshs d,x,y,u)
    pshs  d             ; 0,s = running value (2B), 2,s = digit count (1B), 3,s = leading zero flag (1B)
    ldu   #__pow10_table
.u16_next_div:
    ldy   ,u++          ; Y = divisor (10000, 1000, 100, 10, 1)
    beq   .u16_done
    clr   2,s           ; digit count = 0
.u16_sub_loop:
    ldd   0,s           ; D = running value
    subd  -2,u          ; subtract divisor
    bcs   .u16_digit_done
    std   0,s           ; update running value
    inc   2,s           ; digit count++
    bra   .u16_sub_loop
.u16_digit_done:
    lda   2,s           ; A = digit count (0..9)
    bne   .u16_emit
    tst   3,s           ; leading zero flag set?
    bne   .u16_emit
    cmpy  #1            ; 1s place?
    bne   .u16_next_div ; skip leading zero
.u16_emit:
    inc   3,s           ; mark leading zero flag = true
    adda  #'0'
    tfr   a,b
    jsr   putchar
    bra   .u16_next_div
.u16_done:
    leas  4,s           ; drop local vars (running value 2B, digit 1B, flag 1B)
    puls  d,x,y,u       ; restore caller registers
    rts

__pow10_table:
    fdb   10000
    fdb   1000
    fdb   100
    fdb   10
    fdb   1
    fdb   0

__fmt_d:
    fcc   "%d"
    fcb   0
__fmt_u:
    fcc   "%u"
    fcb   0
__fmt_s:
    fcc   "%s"
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

__str_panic_2002:
    fdb   0, __str_panic_2002_text, 4, 4
__str_panic_2002_text:
    fcc   "2002"
    fcb   0

__str_panic_2003:
    fdb   0, __str_panic_2003_text, 4, 4
__str_panic_2003_text:
    fcc   "2003"
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
    tfr   d,y               ; Y = remaining count
    ldd   ,u                ; D = far_ref
    beq   .near_print
    stb   $FF42             ; Map block into Slot 2 ($4000)
    ldx   2,u               ; X = slice.offset
    leax  $4000,x           ; X = $4000 + slice.offset
    bra   .print_loop
.near_print:
    ldx   2,u               ; X = slice.offset (direct RAM)
.print_loop:
    ldb   ,x+
    jsr   putchar
    leay  -1,y
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

__panic_prefix:
    fcc   "\n*PANIC* "
    fcb   0

__abort_empty_re_chain:
    fcc   "\n*** ABORT\n\n*** EMPTY_RE_CHAIN\n"
    fcb   0

__print_asciz:
.asciz_loop:
    ldb   ,x+
    beq   .asciz_done
    jsr   putchar
    bra   .asciz_loop
.asciz_done:
    rts

builtin_panic:
    stx   v_prelude___panic_
    cmpx  #0
    bne   .panic_has_arg
    ldx   #1
    stx   v_prelude___panic_
.panic_has_arg:
    ldx   #__panic_prefix
    jsr   __print_asciz
    ldx   v_prelude___panic_
    cmpx  #1
    beq   .panic_after_msg
    jsr   builtin_print_string
.panic_after_msg:
    ldb   #10               ; '\n'
    jsr   putchar

    ldx   v_prelude___jmp_chain_
    cmpx  #0
    beq   .panic_abort
    jmp   __longjmp_to_x

.panic_abort:
    ldx   #__abort_empty_re_chain
    jsr   __print_asciz
    ldx   #1
    jmp   __exit

builtin_exit:
    tfr   d,x
    jmp   __exit

builtin__propagate_panic_:
    ldd   v_prelude___panic_
    beq   .propagate_done

    ldx   v_prelude___jmp_chain_
    cmpx  #0
    beq   .propagate_abort
    jmp   __longjmp_to_x

.propagate_abort:
    ldx   #__abort_empty_re_chain
    jsr   __print_asciz
    ldx   #1
    jmp   __exit

.propagate_done:
    rts

builtin__unlink_jmp_:
    ldx   v_prelude___jmp_chain_
    cmpx  #0
    beq   .unlink_done
    ldd   0,x
    std   v_prelude___jmp_chain_
.unlink_done:
    rts

__longjmp_to_x:
    ldd   2,x
    std   far_ret_tmp
    ldd   12,x
    std   <far_ret_sp
    lda   10,x
    sta   $FF45
    sta   <active_code_blk
    ldy   8,x
    ldu   6,x
    lds   4,x
    clra
    ldb   #1
    jmp   [far_ret_tmp]


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

    pshs  u
    tfr   d,u               ; U = remaining byte count

    ; Resolve pointer A -> Slot 2 if Far, direct if Near
    ldd   ,x                ; D = far_ref(A)
    beq   .streq_near_a
    stb   $FF42             ; Map block into Slot 2 ($4000)
    ldd   2,x               ; D = offset(A)
    addd  #$4000
    tfr   d,x
    bra   .streq_prep_b
.streq_near_a:
    ldx   2,x               ; X = offset(A) (direct RAM)

.streq_prep_b:
    ; Resolve pointer B -> Slot 3 if Far, direct if Near
    ldd   ,y                ; D = far_ref(B)
    beq   .streq_near_b
    stb   $FF43             ; Map block into Slot 3 ($6000)
    ldd   2,y               ; D = offset(B)
    addd  #$6000
    tfr   d,y
    bra   .streq_loop
.streq_near_b:
    ldy   2,y               ; Y = offset(B) (direct RAM)

.streq_loop:
    lda   ,x+
    cmpa  ,y+
    bne   .streq_diff
    leau  -1,u
    cmpu  #0
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
