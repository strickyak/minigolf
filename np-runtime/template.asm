********************************************************************
* NPCode - NitrOS-9 NP VM Interpreter
*
* M6809 assembly interpreter for NPCode bytecode
* Dynamically executes baked NPC payload
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
ftab_ptr        rmb     2       ; pointer to function table in NPC binary
code_base_ptr   rmb     2       ; pointer to bytecode base
frame_ptr       rmb     2       ; pointer to current call frame
fcount          rmb     2       ; function count
entry_func      rmb     2       ; entry function index
call_ret_pc     rmb     2       ; saved return PC during op_call
call_argc       rmb     1       ; saved arg_count during op_call
init_os9_sp     rmb     2       ; initial OS-9 system stack pointer
vm_pc           rmb     2       ; saved VM PC
vm_sp           rmb     2       ; saved VM evaluation stack pointer
vm_len          rmb     2       ; length of any slice
vm_any          rmb     2       ; current any pointer

heap_buf        rmb     64      ; small heap buffer for BUF_ALLOC
line_buf        rmb     128     ; output line buffer for PRINTLN
dispatch_tbl    rmb     512     ; 256 opcode function pointers

vm_stack        rmb     256     ; 128-word evaluation stack
vm_stack_top    equ     .

* System stack room (holds VM call stack frames)
stack_space     rmb     4096
size            equ     .

name    use     modname.asm
        fcb     edition

start
* Save data area base and OS-9 initial stack pointer
        stu     <data_base
        sts     <init_os9_sp

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

        leay    op_sub,pcr
        sty     $41*2,x

        leay    op_bit_and,pcr
        sty     $46*2,x

        leay    op_cmp_lt,pcr
        sty     $4E*2,x

        leay    op_cmp_le,pcr
        sty     $4F*2,x

        leay    op_jump,pcr
        sty     $60*2,x

        leay    op_jump_if_false,pcr
        sty     $62*2,x

        leay    op_call,pcr
        sty     $63*2,x

        leay    op_ret,pcr
        sty     $64*2,x

        leay    op_ret_void,pcr
        sty     $66*2,x

        leay    op_println,pcr
        sty     $CB*2,x

* Initialize VM state by parsing NPC binary header:
        leax    npc_binary,pcr  ; X = binary start

* 1. spool_ptr = X + 16 + globals_count * 2
        ldd     8,x             ; D = globals_count
        aslb
        rola                    ; D = globals_count * 2
        leay    16,x            ; Y = X + 16
        leay    d,y             ; Y = X + 16 + globals_count * 2
        sty     <spool_ptr

* 2. ftab_ptr = spool_ptr + spool_sz
        ldd     6,x             ; D = spool_sz
        leay    d,y             ; Y = ftab
        sty     <ftab_ptr

        ldd     10,x            ; D = func_count
        std     <fcount
        ldd     12,x            ; D = entry_func
        std     <entry_func

* 3. Calculate code_base = ftab + func_count * 10 + total_vars
* where total_vars = sum(arg_count + local_count)
        ldu     <fcount         ; U = loop counter
        ldx     <ftab_ptr       ; X points to current func descriptor
        clra
        clrb                    ; D = total_vars
sum_locals
        cmpu    #0
        beq     done_sum
        addb    ,x              ; arg_count is at offset 0
        adca    #0
        addb    1,x             ; local_count is at offset 1
        adca    #0
        leay    10,y            ; advance Y by 10 (fcount * 10)
        leax    10,x            ; advance X to next func descriptor
        leau    -1,u
        bra     sum_locals
done_sum
        leay    d,y             ; Y = code_base!
        sty     <code_base_ptr

* 4. Locate entry function descriptor: ftab + entry_func * 10
        ldd     <entry_func
        pshs    d               ; save entry_func
        aslb
        rola                    ; * 2
        aslb
        rola                    ; * 4
        aslb
        rola                    ; * 8
        addd    ,s              ; * 9
        addd    ,s++            ; * 10
        ldx     <ftab_ptr
        leax    d,x             ; X points to entry func descriptor!

* 5. Setup VM Program Counter Y = code_base + entry.code_offset
        ldd     4,x             ; D = code_offset
        ldy     <code_base_ptr
        leay    d,y             ; Y = entry point PC!

* 6. Setup VM Evaluation Stack pointer U
        ldu     <data_base
        leau    vm_stack_top,u  ; U = top of evaluation stack

* 7. Setup initial Call Frame on S for entry function:
        ldd     2,x             ; D = entry frame_size
        coma
        comb
        addd    #1              ; D = -entry_frame_size
        leas    d,s             ; allocate entry frame on S
* S is now at start of entry function locals!
        ldd     #0              ; sentinel ($0000)
        pshs    d               ; push sentinel caller frame_ptr at frame_ptr - 2
        pshs    d               ; push sentinel return PC at frame_ptr - 4
        leax    4,s             ; X = frame_ptr!
        stx     <frame_ptr      ; frame_ptr set!
* S is now at frame_ptr - 4! Exactly matches invariant!

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

op_sub
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
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

op_cmp_le
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b (sets C and Z)
        leau    2,u             ; drop b
        bls     cmp_le_true     ; unsigned a <= b
        clra
        clrb
        std     ,u
        lbra    dispatch
cmp_le_true
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

op_call
        ldd     ,y++            ; D = func_idx, Y = caller return PC
        sty     <call_ret_pc    ; save caller return PC

* Look up descriptor: ftab + func_idx * 10
        pshs    d
        aslb
        rola
        aslb
        rola
        aslb
        rola
        addd    ,s
        addd    ,s++            ; D = func_idx * 10
        ldx     <ftab_ptr
        leax    d,x             ; X = target function descriptor

* Read descriptor fields:
        lda     ,x              ; A = arg_count
        sta     <call_argc      ; save arg_count
        ldd     4,x             ; D = target code_offset
        ldy     <code_base_ptr
        leay    d,y             ; Y = target entry PC!
        ldd     2,x             ; D = target frame_size

* Allocate frame_size on S:
        coma
        comb
        addd    #1              ; D = -target_frame_size
        leas    d,s             ; S = S - target_frame_size

* Push caller_frame_ptr and caller_return_pc:
        ldx     <frame_ptr      ; caller frame_ptr
        pshs    x               ; store at new_frame_ptr - 2
        ldx     <call_ret_pc    ; caller return PC
        pshs    x               ; store at new_frame_ptr - 4

* Set callee frame_ptr:
        leax    4,s             ; X = new_frame_ptr
        stx     <frame_ptr      ; <frame_ptr = callee frame_ptr!

* Pop arguments from evaluation stack U into parameter slots:
        ldb     <call_argc      ; B = arg_count
        beq     call_done_args
* Compute offset of last argument: (arg_count - 1) * 2
        decb                    ; B = arg_count - 1
        aslb                    ; B = (arg_count - 1) * 2
        abx                     ; X points to last parameter slot!
pop_arg_loop
        pulu    d               ; pop argument from U
        std     ,x              ; store into slot!
        leax    -2,x            ; point to previous slot
        dec     <call_argc      ; one fewer argument
        bne     pop_arg_loop
call_done_args
        lbra    dispatch

op_ret
        pulu    d               ; D = return value from evaluation stack U
        ldx     <frame_ptr      ; X = callee frame_ptr
        ldy     -4,x            ; Y = return PC
        cmpy    #0              ; is return PC the exit sentinel?
        lbeq    ret_exit        ; yes: main returned, exit program!
        ldx     -2,x            ; X = caller's frame_ptr
        stx     <frame_ptr      ; restore caller's frame_ptr!
        leas    -4,x            ; restore caller's S!
        pshu    d               ; push return value onto caller's U
        lbra    dispatch

op_ret_void
        ldx     <frame_ptr      ; X = callee frame_ptr
        ldy     -4,x            ; Y = return PC
        cmpy    #0              ; is return PC the exit sentinel?
        lbeq    ret_exit        ; yes: main returned, exit program!
        ldx     -2,x            ; X = caller's frame_ptr
        stx     <frame_ptr      ; restore caller's frame_ptr!
        leas    -4,x            ; restore caller's S!
        lbra    dispatch

ret_exit
        lds     <init_os9_sp    ; restore initial OS-9 stack
        clrb                    ; exit status 0
        os9     F$Exit

op_illegal
        lds     <init_os9_sp    ; restore initial OS-9 stack
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
* Baked NPC Binary Payload
********************************************************************
npc_binary
        use     payload.asm
npc_binary_end  equ     *

        emod
eom     equ     *
        end
