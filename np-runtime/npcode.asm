********************************************************************
* NPCode - NitrOS-9 NP VM Interpreter
*
* M6809 assembly interpreter for NPCode bytecode
* Baked test case: test_for3.npc
********************************************************************

        nam     npcode
        ttl     NPCode Interpreter

        ifp1
        use     defsfile
        endc

tylg    set     Prgrm+Objct
atrv    set     ReEnt+rev
rev     set     $00
edition set     1

        mod     eom,name,tylg,atrv,start,size

* Data area allocation (direct page & storage)
        org     0
data_base       rmb     2       ; base of data area
dispatch_ptr    rmb     2       ; pointer to dispatch table
spool_ptr       rmb     2       ; pointer to string pool in NPC binary
frame_ptr       rmb     2       ; pointer to local variables array
vm_pc           rmb     2       ; saved VM PC
vm_sp           rmb     2       ; saved VM evaluation stack pointer
vm_len          rmb     2       ; length of any slice
vm_any          rmb     2       ; current any pointer

heap_buf        rmb     64      ; small heap buffer for BUF_ALLOC
vm_locals       rmb     64      ; local variables (32 slots)
line_buf        rmb     128     ; output line buffer for PRINTLN
dispatch_tbl    rmb     512     ; 256 opcode function pointers

vm_stack        rmb     256     ; 128-word evaluation stack
vm_stack_top    equ     .

                rmb     256     ; OS-9 system stack room
size            equ     .

name    fcs     /npcode/
        fcb     edition

start
* Save data area base
        stu     <data_base

* Initialize dispatch table with op_illegal
        leax    dispatch_tbl,u
        stx     <dispatch_ptr
        leay    op_illegal,pcr
        clrb
init_tbl
        sty     ,x++
        decb
        bne     init_tbl

* Populate supported opcodes into dispatch table
        ldx     <dispatch_ptr

        leay    op_nop,pcr
        sty     $00*2,x

        leay    op_push_0,pcr
        sty     $05*2,x

        leay    op_push_1,pcr
        sty     $06*2,x

        leay    op_push_i8,pcr
        sty     $08*2,x

        leay    op_push_i16,pcr
        sty     $0A*2,x

        leay    op_push_str,pcr
        sty     $0B*2,x

        leay    op_pop,pcr
        sty     $0C*2,x

        leay    op_load_local_0,pcr
        sty     $20*2,x

        leay    op_load_local_1,pcr
        sty     $21*2,x

        leay    op_load_local_2,pcr
        sty     $22*2,x

        leay    op_load_local_3,pcr
        sty     $23*2,x

        leay    op_store_local_0,pcr
        sty     $24*2,x

        leay    op_store_local_1,pcr
        sty     $25*2,x

        leay    op_store_local_2,pcr
        sty     $26*2,x

        leay    op_store_local_3,pcr
        sty     $27*2,x

        leay    op_load_local,pcr
        sty     $28*2,x

        leay    op_store_local,pcr
        sty     $29*2,x

        leay    op_buf_alloc,pcr
        sty     $2C*2,x

        leay    op_buf_free,pcr
        sty     $2D*2,x

        leay    op_store_field,pcr
        sty     $2F*2,x

        leay    op_add,pcr
        sty     $40*2,x

        leay    op_bit_and,pcr
        sty     $46*2,x

        leay    op_cmp_lt,pcr
        sty     $4E*2,x

        leay    op_jump,pcr
        sty     $60*2,x

        leay    op_jump_if_false,pcr
        sty     $62*2,x

        leay    op_ret_void,pcr
        sty     $66*2,x

        leay    op_println,pcr
        sty     $CB*2,x

* Initialize VM state:
* 1. String pool pointer (spool_ptr = npc_binary + 16)
        leax    npc_binary,pcr
        leax    16,x
        stx     <spool_ptr

* 2. Frame pointer (vm_locals)
        ldx     <data_base
        leax    vm_locals,x
        stx     <frame_ptr

* 3. Clear local variable slots
        clra
        clrb
        std     ,x
        std     2,x
        std     4,x
        std     6,x
        std     8,x
        std     10,x
        std     12,x

* 4. Setup VM Evaluation Stack pointer (U register)
        ldx     <data_base
        leax    vm_stack_top,x
        tfr     x,u

* 5. Setup VM Program Counter (Y register)
        leay    npc_code,pcr

* Start execution loop
        bra     dispatch

********************************************************************
* Instruction Dispatch Loop
********************************************************************
dispatch
        ldb     ,y+             ; fetch opcode byte, advance PC
        clra
        aslb
        rola                    ; D = opcode * 2
        ldx     <dispatch_ptr   ; dispatch table base
        jmp     [d,x]           ; jump indirect through table

********************************************************************
* Opcode Handlers
********************************************************************
op_nop
        lbra    dispatch

op_push_0
        clra
        clrb
        pshu    d
        lbra    dispatch

op_push_1
        ldd     #1
        pshu    d
        lbra    dispatch

op_push_i8
        ldb     ,y+
        sex
        pshu    d
        lbra    dispatch

op_push_i16
        ldd     ,y++
        pshu    d
        lbra    dispatch

op_push_str
        ldd     ,y++            ; string pool offset
        ldx     <spool_ptr      ; base of string pool
        leax    d,x             ; X points to string pool entry
        ldd     ,x++            ; D = length, X now points to text
        pshu    x               ; push text_addr (base)
        pshu    d               ; push length (cap)
        pshu    d               ; push length (len)
        lbra    dispatch

op_pop
        leau    2,u
        lbra    dispatch

op_load_local_0
        ldx     <frame_ptr
        ldd     ,x
        pshu    d
        lbra    dispatch

op_load_local_1
        ldx     <frame_ptr
        ldd     2,x
        pshu    d
        lbra    dispatch

op_load_local_2
        ldx     <frame_ptr
        ldd     4,x
        pshu    d
        lbra    dispatch

op_load_local_3
        ldx     <frame_ptr
        ldd     6,x
        pshu    d
        lbra    dispatch

op_store_local_0
        pulu    d
        ldx     <frame_ptr
        std     ,x
        lbra    dispatch

op_store_local_1
        pulu    d
        ldx     <frame_ptr
        std     2,x
        lbra    dispatch

op_store_local_2
        pulu    d
        ldx     <frame_ptr
        std     4,x
        lbra    dispatch

op_store_local_3
        pulu    d
        ldx     <frame_ptr
        std     6,x
        lbra    dispatch

op_load_local
        ldb     ,y+             ; B = slot
        aslb                    ; B = slot * 2
        ldx     <frame_ptr
        abx                     ; X = frame_ptr + slot*2
        ldd     ,x
        pshu    d
        lbra    dispatch

op_store_local
        ldx     <frame_ptr
        ldb     ,y+             ; B = slot
        aslb                    ; B = slot * 2
        abx                     ; X = frame_ptr + slot*2
        pulu    d
        std     ,x
        lbra    dispatch

op_buf_alloc
        pulu    d               ; discard size
        ldx     <data_base
        leax    heap_buf,x
        pshu    x
        lbra    dispatch

op_buf_free
        pulu    d               ; discard buffer pointer
        lbra    dispatch

op_store_field
        ldb     ,y+             ; B = offset
        ldx     2,u             ; X = obj_ptr
        abx                     ; X = obj_ptr + offset
        pulu    d               ; D = val
        leau    2,u             ; drop obj_ptr
        std     ,x              ; store val at obj_ptr + offset
        lbra    dispatch

op_add
        pulu    d               ; D = b
        addd    ,u              ; D = b + a
        std     ,u              ; replace on stack
        lbra    dispatch

op_bit_and
        pulu    d               ; D = b
        anda    ,u
        andb    1,u
        std     ,u
        lbra    dispatch

op_cmp_lt
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b (borrow -> C=1)
        leau    2,u             ; drop b
        bcs     cmp_lt_true
        clra
        clrb
        std     ,u
        lbra    dispatch
cmp_lt_true
        ldd     #1
        std     ,u
        lbra    dispatch

op_jump
        ldd     ,y++            ; signed relative offset
        leay    d,y
        lbra    dispatch

op_jump_if_false
        ldd     ,y++            ; signed relative offset
        pulu    x               ; X = condition
        cmpx    #0              ; is condition false (0)?
        lbne    dispatch        ; true: do not jump
        leay    d,y             ; false: jump!
        lbra    dispatch

op_ret_void
        clrb                    ; exit status 0
        os9     F$Exit

op_illegal
        ldb     #1              ; exit status 1
        os9     F$Exit

********************************************************************
* op_println: Print Slice[any] to standard output
********************************************************************
op_println
        pulu    d               ; D = len
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base (pointer to array of any structs)
        stu     <vm_sp          ; save VM stack pointer
        sty     <vm_pc          ; save VM PC
        std     <vm_len         ; save len
        stx     <vm_any         ; save any pointer

        ldx     <data_base
        leay    line_buf,x      ; Y = output pointer in line_buf
        ldd     <vm_len
        beq     pl_done         ; empty slice

pl_elem_loop
        ldx     <vm_any         ; X points to any struct
        ldx     2,x             ; X = type_addr (points into string pool)
        lda     ,x              ; A = first character of type string
        cmpa    #'b'
        beq     pl_is_byte

pl_is_word
        ldx     <vm_any
        ldx     ,x              ; X = val_addr
        ldd     ,x              ; D = 16-bit word value
        tfr     y,x             ; X = output pointer
        bsr     format_u16      ; format D into X, updates X
        tfr     x,y             ; Y = updated output pointer
        bra     pl_next_elem

pl_is_byte
        ldx     <vm_any
        ldx     ,x              ; X = val_addr
        ldb     ,x              ; B = 8-bit byte value
        clra                    ; D = zero-extended word
        tfr     y,x             ; X = output pointer
        bsr     format_u16      ; format D into X, updates X
        tfr     x,y             ; Y = updated output pointer

pl_next_elem
        ldx     <vm_any
        leax    4,x             ; advance to next any struct (4 bytes)
        stx     <vm_any
        ldd     <vm_len
        subd    #1
        std     <vm_len
        beq     pl_done         ; all elements formatted
        lda     #' '            ; add space between arguments
        sta     ,y+
        bra     pl_elem_loop

pl_done
        lda     #$0D            ; append CR ($0D, OS-9 line terminator)
        sta     ,y+

* Write line to stdout
        ldx     <data_base
        leax    line_buf,x      ; X = start of line_buf
        tfr     y,d             ; D = end of line_buf
        pshs    x
        subd    ,s++            ; D = count of bytes
        tfr     d,y             ; Y = length for I$WritLn
        lda     #1              ; path 1 = stdout
        os9     I$WritLn

* Restore VM registers and continue dispatch
        ldu     <vm_sp
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* format_u16: Convert unsigned 16-bit word in D to decimal string.
* Input:  D = 16-bit unsigned number (0..65535)
*         X = destination buffer pointer
* Output: X = updated destination pointer (past written characters)
* Preserves: other registers
********************************************************************
format_u16
        pshs    d,y,u
        clr     ,-s             ; 1,s = flag (0 = suppress leading zeros)
        clr     ,-s             ; 0,s = digit (0..9)
        leay    p10_tbl,pcr     ; table of powers of 10
f_p_loop
        ldd     ,y              ; is table entry 0?
        beq     f_units         ; yes: done with powers, format units
        clr     ,s              ; digit count = 0
f_sub_loop
        ldd     2,s             ; D = current remainder value
        subd    ,y              ; D = D - power
        blo     f_borrow        ; borrow -> value < power, done with this digit
        std     2,s             ; save new remainder
        inc     ,s              ; increment digit count
        bra     f_sub_loop

f_borrow
        lda     ,s              ; A = digit count (0..9)
        bne     f_emit_digit    ; if non-zero, always emit
        tst     1,s             ; has a non-zero digit been emitted yet?
        bne     f_emit_zero     ; yes: emit '0'
        bra     f_advance_p     ; no: suppress leading zero
f_emit_digit
        inc     1,s             ; flag = 1
f_emit_zero
        adda    #'0             ; A = '0'..'9'
        sta     ,x+             ; write digit character
f_advance_p
        leay    2,y             ; advance to next power of 10
        bra     f_p_loop

f_units
        ldb     3,s             ; low byte of remainder (0..9)
        addb    #'0             ; B = '0'..'9'
        stb     ,x+             ; always emit units digit
        leas    2,s             ; drop digit and flag
        puls    d,y,u,pc        ; restore registers and return

p10_tbl
        fdb     10000,1000,100,10,0

********************************************************************
* Baked test_for3.npc Binary
********************************************************************
npc_binary
        fcb     $4E,$50,$43,$01,$01,$00,$00,$0E,$00,$00,$00,$01,$00,$00,$00,$D4
        fcb     $00,$04,$62,$79,$74,$65,$00,$00,$04,$77,$6F,$72,$64,$00,$00,$07
        fcb     $00,$0E,$00,$00,$00,$D4,$00,$00,$02,$02,$02,$02,$02,$02,$02
npc_code
        fcb     $05
        fcb     $0A,$00,$FF,$46,$24,$20,$08,$05,$4E,$62,$00,$2A,$0A,$00,$06,$2C
        fcb     $27,$20,$29,$04,$23,$28,$04,$2F,$04,$23,$23,$0A,$00,$05,$40,$2F
        fcb     $00,$23,$0B,$00,$00,$0C,$0C,$2F,$02,$23,$06,$06,$CB,$23,$2D,$20
        fcb     $06,$40,$24,$60,$FF,$CF,$08,$05,$25,$21,$08,$0A,$4E,$62,$00,$2A
        fcb     $0A,$00,$06,$2C,$27,$21,$29,$04,$23,$28,$04,$2F,$04,$23,$23,$0A
        fcb     $00,$04,$40,$2F,$00,$23,$0B,$00,$07,$0C,$0C,$2F,$02,$23,$06,$06
        fcb     $CB,$23,$2D,$21,$06,$40,$25,$60,$FF,$CF,$08,$05,$25,$21,$08,$0A
        fcb     $4E,$62,$00,$2A,$0A,$00,$06,$2C,$27,$21,$29,$04,$23,$28,$04,$2F
        fcb     $04,$23,$23,$0A,$00,$04,$40,$2F,$00,$23,$0B,$00,$07,$0C,$0C,$2F
        fcb     $02,$23,$06,$06,$CB,$23,$2D,$21,$06,$40,$25,$60,$FF,$CF,$08,$0D
        fcb     $26,$22,$08,$0F,$4E,$62,$00,$2A,$0A,$00,$06,$2C,$27,$22,$29,$04
        fcb     $23,$28,$04,$2F,$04,$23,$23,$0A,$00,$04,$40,$2F,$00,$23,$0B,$00
        fcb     $07,$0C,$0C,$2F,$02,$23,$06,$06,$CB,$23,$2D,$22,$06,$40,$26,$60
        fcb     $FF,$CF,$66

        emod
eom     equ     *
        end
