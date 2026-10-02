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
ltab_ptr        rmb     2       ; pointer to local variable table in NPC binary
code_base_ptr   rmb     2       ; pointer to bytecode base
frame_ptr       rmb     2       ; pointer to current call frame
cur_vtab        rmb     2       ; pointer to current function's var size table
callee_vtab     rmb     2       ; temporary for callee var size table during call
fcount          rmb     2       ; function count
entry_func      rmb     2       ; entry function index
call_ret_pc     rmb     2       ; saved return PC during op_call
call_argc       rmb     1       ; saved arg_count during op_call
gli_slot        rmb     1       ; temporary slot index for get_local_info
init_os9_sp     rmb     2       ; initial OS-9 system stack pointer
vm_pc           rmb     2       ; saved VM PC
vm_sp           rmb     2       ; saved VM evaluation stack pointer
vm_len          rmb     2       ; length of any slice
vm_any          rmb     2       ; current any pointer

heap_buf        rmb     256     ; heap buffer for BUF_ALLOC
line_buf        rmb     256     ; output line buffer for PRINTLN
globals_buf     rmb     2048    ; storage buffer for global variables
global_ptrs     rmb     128     ; pointers to each global variable (up to 64 globals)
dispatch_tbl    rmb     512     ; 256 opcode function pointers

vm_stack        rmb     512     ; 256-word evaluation stack
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

        leay    op_load_global,pcr
        sty     $2A*2,x

        leay    op_store_global,pcr
        sty     $2B*2,x

        leay    op_buf_alloc,pcr
        sty     $2C*2,x

        leay    op_buf_free,pcr
        sty     $2D*2,x

        leay    op_store_field,pcr
        sty     $2F*2,x

        leay    op_addr_of_global,pcr
        sty     $30*2,x

        leay    op_get_word_field,pcr
        sty     $31*2,x

        leay    op_set_word_field,pcr
        sty     $32*2,x

        leay    op_get_char_field,pcr
        sty     $33*2,x

        leay    op_set_char_field,pcr
        sty     $34*2,x

        leay    op_addr_of_local,pcr
        sty     $35*2,x

        leay    op_add,pcr
        sty     $40*2,x

        leay    op_sub,pcr
        sty     $41*2,x

        leay    op_mul,pcr
        sty     $42*2,x

        leay    op_bit_and,pcr
        sty     $46*2,x

        leay    op_cmp_eq,pcr
        sty     $4C*2,x

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

        leay    op_slice_len,pcr
        sty     $71*2,x

        leay    op_str_cmp,pcr
        sty     $77*2,x

        leay    op_println,pcr
        sty     $CB*2,x

* Initialize VM state by parsing NPC binary header:
        leax    npc_binary,pcr  ; X = binary start

* Clear globals_buf:
        ldu     <data_base
        leau    globals_buf,u
        ldd     #2048/2
clr_g_loop
        clr     ,u+
        clr     ,u+
        subd    #1
        bne     clr_g_loop

* 1. Initialize globals pointers in global_ptrs:
* globals_count is at npc_binary + 8
        ldd     8,x             ; D = globals_count
        beq     init_globals_done
        pshs    d               ; 0,s = count of globals
        ldx     <data_base
        leau    global_ptrs,x   ; U = pointer into global_ptrs
        leay    globals_buf,x   ; Y = pointer into globals_buf
        leax    npc_binary+16,pcr ; X = pointer to global sizes in binary
init_g_loop
        sty     ,u++            ; global_ptrs[i] = Y
        ldd     ,x++            ; D = size of this global
        leay    d,y             ; advance Y by size bytes
        ldd     ,s
        subd    #1
        std     ,s
        bne     init_g_loop
        leas    2,s             ; drop count
init_globals_done

* 2. spool_ptr = npc_binary + 16 + globals_count * 2
        leax    npc_binary,pcr
        ldd     8,x             ; D = globals_count
        aslb
        rola                    ; D = globals_count * 2
        leay    16,x            ; Y = X + 16
        leay    d,y             ; Y = X + 16 + globals_count * 2
        sty     <spool_ptr

* 3. ftab_ptr = spool_ptr + spool_sz
        ldd     6,x             ; D = spool_sz
        leay    d,y             ; Y = ftab
        sty     <ftab_ptr

        ldd     10,x            ; D = func_count
        std     <fcount
        ldd     12,x            ; D = entry_func
        std     <entry_func

* 4. ltab_ptr = ftab_ptr + func_count * 10
* and code_base_ptr = ltab_ptr + sum(arg_count + local_count)
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
        sty     <ltab_ptr       ; ltab_ptr = ftab + func_count * 10
        leay    d,y             ; Y = code_base!
        sty     <code_base_ptr

* 5. Locate entry function descriptor: ftab + entry_func * 10
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

* 6. Setup entry function's cur_vtab:
        ldd     8,x             ; D = var_table_offset
        ldy     <ltab_ptr
        leay    d,y
        sty     <cur_vtab       ; cur_vtab set for entry function!

* 7. Setup VM Program Counter Y = code_base + entry.code_offset
        ldd     4,x             ; D = code_offset
        ldy     <code_base_ptr
        leay    d,y             ; Y = entry point PC!

* 8. Setup VM Evaluation Stack pointer U
        ldu     <data_base
        leau    vm_stack_top,u  ; U = top of evaluation stack

* 9. Setup initial Call Frame on S for entry function:
        ldd     2,x             ; D = entry frame_size
        coma
        comb
        addd    #1              ; D = -entry_frame_size
        leas    d,s             ; allocate entry frame on S
* S is now at start of entry function locals!
        ldd     #0              ; sentinel ($0000)
        pshs    d               ; push sentinel caller frame_ptr at frame_ptr - 2
        pshs    d               ; push sentinel return PC at frame_ptr - 4
        pshs    d               ; push sentinel caller cur_vtab at frame_ptr - 6
        leax    6,s             ; X = frame_ptr!
        stx     <frame_ptr      ; frame_ptr set!
* S is now at frame_ptr - 6! Exactly matches invariant!

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

* Helper: get_local_info
* Input:  B = local variable slot index
* Output: X = address of local variable in current frame
*         B = size of local variable in bytes
* Preserves: Y
get_local_info
        stb     <gli_slot
        pshs    y
        ldy     <cur_vtab
        clra
        clrb                    ; D = 0 (offset)
        tst     <gli_slot
        beq     gli_done
gli_loop
        addb    ,y+
        adca    #0
        dec     <gli_slot
        bne     gli_loop
gli_done
        ldx     <frame_ptr
        leax    d,x             ; X = frame_ptr + offset (using intact offset D!)
        ldb     ,y              ; B = size of target slot
        puls    y,pc

op_load_local_0
        clrb
        bra     do_load_local

op_load_local_1
        ldb     #1
        bra     do_load_local

op_load_local_2
        ldb     #2
        bra     do_load_local

op_load_local_3
        ldb     #3
        bra     do_load_local

op_load_local
        ldb     ,y+
do_load_local
        lbsr    get_local_info  ; X = addr, B = size
        cmpb    #2
        bne     load_local_slice
        ldd     ,x
        pshu    d
        lbra    dispatch
load_local_slice
        ldd     ,x
        pshu    d
        ldd     2,x
        pshu    d
        ldd     4,x
        pshu    d
        lbra    dispatch

op_store_local_0
        clrb
        bra     do_store_local

op_store_local_1
        ldb     #1
        bra     do_store_local

op_store_local_2
        ldb     #2
        bra     do_store_local

op_store_local_3
        ldb     #3
        bra     do_store_local

op_store_local
        ldb     ,y+
do_store_local
        lbsr    get_local_info  ; X = addr, B = size
        cmpb    #2
        bne     store_local_slice
        pulu    d
        std     ,x
        lbra    dispatch
store_local_slice
        pulu    d
        std     4,x
        pulu    d
        std     2,x
        pulu    d
        std     ,x
        lbra    dispatch

op_addr_of_local
        ldb     ,y+             ; B = slot
        lbsr    get_local_info  ; X = addr, B = size
        pshu    x               ; push address
        lbra    dispatch

op_addr_of_global
        ldd     ,y++            ; D = slot
        aslb
        rola                    ; D = slot * 2
        ldx     <data_base
        leax    global_ptrs,x
        ldd     d,x             ; D = address of global
        pshu    d
        lbra    dispatch

op_load_global
        ldd     ,y++            ; D = slot
        aslb
        rola                    ; D = slot * 2
        pshs    d               ; save slot*2 on S
        ldx     <data_base
        leax    global_ptrs,x
        ldx     d,x             ; X = address of global
        ldd     ,s++            ; D = slot*2
        pshs    x               ; 0,s = address of global
        leax    npc_binary+16,pcr ; global variable table
        ldd     d,x             ; D = size of global
        puls    x               ; X = address of global
        lsrb                    ; B = word count
        pshs    b               ; save word count on stack
lg_loop
        ldd     ,x++
        pshu    d
        dec     ,s
        bne     lg_loop
        leas    1,s             ; clean stack
        lbra    dispatch

op_store_global
        ldd     ,y++            ; D = slot
        aslb
        rola                    ; D = slot * 2
        pshs    d               ; save slot*2 on S
        ldx     <data_base
        leax    global_ptrs,x
        ldx     d,x             ; X = address of global
        ldd     ,s++            ; D = slot*2
        pshs    x               ; 0,s = address of global
        leax    npc_binary+16,pcr ; global variable table
        ldd     d,x             ; D = size of global
        puls    x               ; X = address of global
        leax    d,x             ; X = address + size
        lsrb                    ; B = word count
        pshs    b               ; save word count on stack
sg_loop
        pulu    d
        std     ,--x
        dec     ,s
        bne     sg_loop
        leas    1,s             ; clean stack
        lbra    dispatch

op_get_word_field
        pulu    d               ; D = offset
        pulu    x               ; X = ptr
        leax    d,x             ; X = ptr + offset
        ldd     ,x              ; D = word at ptr + offset
        pshu    d
        lbra    dispatch

op_set_word_field
        pulu    d               ; D = val
        pshs    d               ; save val on S
        pulu    d               ; D = offset
        pulu    x               ; X = ptr
        leax    d,x             ; X = ptr + offset
        puls    d               ; restore D = val
        std     ,x              ; store val at ptr + offset
        lbra    dispatch

op_get_char_field
        pulu    d               ; D = offset
        pulu    x               ; X = ptr
        leax    d,x             ; X = ptr + offset
        ldb     ,x              ; B = byte
        clra                    ; zero-extend to 16-bit word
        pshu    d
        lbra    dispatch

op_set_char_field
        pulu    d               ; B = val (low byte)
        pshs    b               ; save val on S
        pulu    d               ; D = offset
        pulu    x               ; X = ptr
        leax    d,x             ; X = ptr + offset
        puls    a               ; A = val
        sta     ,x              ; store byte at ptr + offset
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

op_mul
        pulu    d               ; A = Bh, B = Bl
        pshs    d               ; 2,s = Bh, 3,s = Bl
        clr     ,-s             ; 1,s = cross terms sum
        clr     ,-s             ; 0,s = unused

* 1. Ah * Bl:
        lda     ,u              ; Ah
        ldb     3,s             ; Bl
        mul                     ; B = low byte of Ah * Bl
        stb     1,s             ; cross sum = (Ah * Bl).low

* 2. Al * Bh:
        lda     1,u             ; Al
        ldb     2,s             ; Bh
        mul                     ; B = low byte of Al * Bh
        addb    1,s
        stb     1,s             ; cross sum += (Al * Bh).low

* 3. Al * Bl:
        lda     1,u             ; Al
        ldb     3,s             ; Bl
        mul                     ; D = Al * Bl

* Add cross sum to high byte A:
        adda    1,s             ; A = A + cross sum
        std     ,u              ; store 16-bit result onto stack!
        leas    4,s             ; drop locals
        lbra    dispatch

op_bit_and
        pulu    d               ; D = b
        anda    ,u
        andb    1,u
        std     ,u
        lbra    dispatch

op_cmp_eq
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
        beq     cmp_eq_true
        clra
        clrb
        std     ,u
        lbra    dispatch
cmp_eq_true
        ldd     #1
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
        ldd     2,x             ; D = target frame_size
        pshs    d               ; 0,s = target frame_size
        ldd     4,x             ; D = target code_offset
        ldy     <code_base_ptr
        leay    d,y             ; Y = target entry PC!
        ldd     8,x             ; D = target var_table_offset
        ldx     <ltab_ptr
        leax    d,x
        stx     <callee_vtab    ; callee's var_table pointer in DP temp
        puls    d               ; restore target frame_size

* Allocate frame_size on S:
        coma
        comb
        addd    #1              ; D = -target_frame_size
        leas    d,s             ; S = S - target_frame_size

* Push caller context:
        ldx     <frame_ptr      ; caller frame_ptr
        pshs    x               ; store at new_frame_ptr - 2
        ldx     <call_ret_pc    ; caller return PC
        pshs    x               ; store at new_frame_ptr - 4
        ldx     <cur_vtab       ; caller cur_vtab
        pshs    x               ; store at new_frame_ptr - 6

* Set callee frame_ptr and cur_vtab:
        leax    6,s             ; X = new_frame_ptr
        stx     <frame_ptr      ; <frame_ptr = callee frame_ptr!
        ldx     <callee_vtab
        stx     <cur_vtab       ; cur_vtab = callee var_table!

* Pop arguments from evaluation stack U into parameter slots:
        lda     <call_argc      ; A = arg_count
        beq     call_done_args
call_arg_loop
        deca                    ; A = slot index (arg_count - 1 down to 0)
        sta     <call_argc      ; save current slot
        tfr     a,b             ; B = slot index
        lbsr    get_local_info  ; X = addr, B = size
        cmpb    #2
        bne     call_arg_slice
        pulu    d
        std     ,x
        bra     call_next_arg
call_arg_slice
        pulu    d
        std     4,x
        pulu    d
        std     2,x
        pulu    d
        std     ,x
call_next_arg
        lda     <call_argc
        bne     call_arg_loop
call_done_args
        lbra    dispatch

op_ret
        ldx     <frame_ptr      ; X = callee frame_ptr
        ldy     -4,x            ; Y = return PC
        cmpy    #0              ; is return PC the exit sentinel?
        lbeq    ret_exit        ; yes: main returned, exit program!
        ldd     -6,x            ; caller's cur_vtab
        std     <cur_vtab       ; restore caller's cur_vtab!
        ldx     -2,x            ; X = caller's frame_ptr!
        stx     <frame_ptr      ; restore caller's frame_ptr!
        leas    -6,x            ; restore caller's S!
        lbra    dispatch

op_ret_void
        ldx     <frame_ptr      ; X = callee frame_ptr
        ldy     -4,x            ; Y = return PC
        cmpy    #0              ; is return PC the exit sentinel?
        lbeq    ret_exit        ; yes: main returned, exit program!
        ldd     -6,x            ; caller's cur_vtab
        std     <cur_vtab       ; restore caller's cur_vtab!
        ldx     -2,x            ; X = caller's frame_ptr!
        stx     <frame_ptr      ; restore caller's frame_ptr!
        leas    -6,x            ; restore caller's S!
        clra
        clrb
        pshu    d               ; push dummy 0x0000 return value
        lbra    dispatch

ret_exit
        lds     <init_os9_sp    ; restore initial OS-9 stack
        clrb                    ; exit status 0
        os9     F$Exit

op_slice_len
        ldd     ,u              ; D = len
        leau    6,u             ; drop slice (len, cap, ptr)
        pshu    d               ; push len
        lbra    dispatch

op_str_cmp
* Save VM PC (Y):
        sty     <vm_pc

* Stack U has b then a:
        pulu    d               ; D = b_len
        pulu    x               ; discard b_cap
        pulu    y               ; Y = b_ptr
        pshs    d,y             ; 0,s = b_len, 2,s = b_ptr

        pulu    d               ; D = a_len
        pulu    x               ; discard a_cap
        pulu    x               ; X = a_ptr
        pshs    d,x             ; 0,s = a_len, 2,s = a_ptr, 4,s = b_len, 6,s = b_ptr

* Let min_len = min(a_len, b_len)
        ldd     0,s             ; D = a_len
        cmpd    4,s             ; compare with b_len
        blo     sc_min_a
        ldd     4,s             ; b_len is smaller or equal
sc_min_a
        beq     sc_check_lens   ; if min_len == 0, compare lengths directly
        pshs    d               ; 0,s = count loop

        ldx     4,s             ; X = a_ptr
        ldy     8,s             ; Y = b_ptr

sc_byte_loop
        lda     ,x+             ; char from A
        cmpa    ,y+             ; compare with char from B (unsigned)
        blo     sc_less         ; a < b
        bhi     sc_greater      ; a > b
        ldd     ,s
        subd    #1
        std     ,s
        bne     sc_byte_loop

* All bytes in min_len matched! Drop loop count:
        leas    2,s

sc_check_lens
* Compare a_len and b_len to break ties:
        ldd     0,s             ; D = a_len
        cmpd    4,s             ; compare with b_len
        blo     sc_less_lens
        bhi     sc_greater_lens
* Strings are equal:
        leas    8,s             ; clean up stack
        ldd     #0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

sc_less
        leas    2,s             ; drop loop count
sc_less_lens
        leas    8,s             ; clean up stack
        ldd     #$FFFF          ; -1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

sc_greater
        leas    2,s             ; drop loop count
sc_greater_lens
        leas    8,s             ; clean up stack
        ldd     #1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_illegal
        lda     -1,y            ; A = opcode byte
        pshs    a
        lsra
        lsra
        lsra
        lsra
        cmpa    #9
        bls     oi_d1
        adda    #'A'-'0'-10
oi_d1
        adda    #'0
        sta     >$FF89          ; print high hex digit
        puls    a
        anda    #$0F
        cmpa    #9
        bls     oi_d2
        adda    #'A'-'0'-10
oi_d2
        adda    #'0
        sta     >$FF89          ; print low hex digit
        lda     #$0D
        sta     >$FF89          ; print newline
        lda     #$FD
        sta     >$FF87
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
        cmpa    #'s'
        beq     pl_is_string
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
        bra     pl_next_elem

pl_is_string
        ldx     <vm_any
        ldx     ,x              ; X = val_addr (points to string struct: base, cap, len)
        ldd     4,x             ; D = str_len
        beq     pl_next_elem    ; empty string: nothing to copy
        pshs    d               ; 0,s = copy count
        ldx     ,x              ; X = str_base (pointer to characters)
pl_str_copy
        lda     ,x+             ; read char from string
        sta     ,y+             ; write char to line_buf
        ldd     ,s
        subd    #1
        std     ,s
        bne     pl_str_copy
        leas    2,s             ; drop copy count
        bra     pl_next_elem

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
