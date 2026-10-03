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
frame_sz        rmb     2       ; temporary for frame size during call/entry
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
vm_is_println   rmb     1       ; 1 = println, 0 = print
heap_ptr        rmb     2       ; current allocation pointer in heap_buf
free_buckets    rmb     16      ; 8 free list bucket pointers (16,32,64,128,256,512,1024,2048)
param_ptr       rmb     2       ; CLI parameter pointer
param_len       rmb     2       ; CLI parameter length
path_scratch    rmb     64      ; scratch buffer for OS-9 pathnames
dp_pad          rmb     1       ; align following buffers to 16-bit word boundary

pbuf_scratch    rmb     256     ; scratch buffer for BUF_ALLOC (print/println any-array)
heap_buf        rmb     12288   ; heap buffer for dynamic allocations (12KB)
line_buf        rmb     256     ; output line buffer for PRINTLN
globals_buf     rmb     14336   ; storage buffer for global variables (14KB)
global_ptrs     rmb     256     ; pointers to each global variable (up to 128 globals)
dispatch_tbl    rmb     512     ; 256 opcode function pointers

vm_stack        rmb     512     ; 256-word evaluation stack
vm_stack_top    equ     .

* System stack room (holds VM call stack frames)
stack_space     rmb     2048
size            equ     .

name
        use     modname.asm
        fcb     edition

start
* Save data area base and OS-9 initial stack pointer
        stu     <data_base
        sts     <init_os9_sp
        stx     <param_ptr
        sty     <param_len
        ldx     <data_base
        leax    heap_buf,x
        stx     <heap_ptr

* Clear free_buckets:
        ldx     <data_base
        leax    free_buckets,x
        clra
        clrb
        std     ,x++
        std     ,x++
        std     ,x++
        std     ,x++
        std     ,x++
        std     ,x++
        std     ,x++
        std     ,x++

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
        sty     $01*2,x         ; PUSH_NIL

        leay    op_push_nil_slice,pcr
        sty     $02*2,x         ; PUSH_NIL_SLICE

        leay    op_push_1,pcr
        sty     $03*2,x         ; PUSH_TRUE

        leay    op_push_0,pcr
        sty     $04*2,x         ; PUSH_FALSE

        leay    op_push_0,pcr
        sty     $05*2,x

        leay    op_push_1,pcr
        sty     $06*2,x

        leay    op_push_neg1,pcr
        sty     $07*2,x         ; PUSH_NEG1

        leay    op_push_i8,pcr
        sty     $08*2,x

        leay    op_push_i16,pcr
        sty     $0A*2,x

        leay    op_push_str,pcr
        sty     $0B*2,x

        leay    op_pop,pcr
        sty     $0C*2,x

        leay    op_pop_slice,pcr
        sty     $0D*2,x         ; POP_SLICE

        leay    op_dup,pcr
        sty     $0E*2,x

        leay    op_swap,pcr
        sty     $0F*2,x

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

        leay    op_peek2,pcr
        sty     $31*2,x

        leay    op_poke2,pcr
        sty     $32*2,x

        leay    op_peek1,pcr
        sty     $33*2,x

        leay    op_poke1,pcr
        sty     $34*2,x

        leay    op_addr_of_local,pcr
        sty     $35*2,x

        leay    op_shl1_add,pcr
        sty     $36*2,x

        leay    op_zalloc,pcr
        sty     $37*2,x

        leay    op_add,pcr
        sty     $40*2,x

        leay    op_sub,pcr
        sty     $41*2,x

        leay    op_mul,pcr
        sty     $42*2,x

        leay    op_div,pcr
        sty     $43*2,x

        leay    op_mod,pcr
        sty     $44*2,x

        leay    op_neg,pcr
        sty     $45*2,x

        leay    op_bit_and,pcr
        sty     $46*2,x

        leay    op_bit_or,pcr
        sty     $47*2,x

        leay    op_bit_xor,pcr
        sty     $48*2,x

        leay    op_bit_not,pcr
        sty     $49*2,x

        leay    op_shl,pcr
        sty     $4A*2,x

        leay    op_shr,pcr
        sty     $4B*2,x

        leay    op_cmp_eq,pcr
        sty     $4C*2,x

        leay    op_cmp_ne,pcr
        sty     $4D*2,x

        leay    op_cmp_lt,pcr
        sty     $4E*2,x

        leay    op_cmp_le,pcr
        sty     $4F*2,x

        leay    op_cmp_gt,pcr
        sty     $50*2,x

        leay    op_cmp_ge,pcr
        sty     $51*2,x

        leay    op_not,pcr
        sty     $52*2,x

        leay    op_jump,pcr
        sty     $60*2,x

        leay    op_jump_if_true,pcr
        sty     $61*2,x

        leay    op_jump_if_false,pcr
        sty     $62*2,x

        leay    op_call,pcr
        sty     $63*2,x

        leay    op_ret,pcr
        sty     $64*2,x

        leay    op_ret,pcr
        sty     $65*2,x

        leay    op_ret_void,pcr
        sty     $66*2,x

        leay    op_nop,pcr
        sty     $70*2,x         ; SLICE_NEW

        leay    op_slice_len,pcr
        sty     $71*2,x         ; SLICE_LEN

        leay    op_slice_cap,pcr
        sty     $72*2,x         ; SLICE_CAP

        leay    op_slice_sub,pcr
        sty     $73*2,x         ; SLICE_SUB

        leay    op_str_cmp,pcr
        sty     $77*2,x         ; STR_CMP

        leay    op_str_startswith,pcr
        sty     $78*2,x         ; STR_STARTSWITH

        leay    op_str_endswith,pcr
        sty     $79*2,x         ; STR_ENDSWITH

        leay    op_str_find,pcr
        sty     $7A*2,x         ; STR_FIND

        leay    op_str_lstrip,pcr
        sty     $7B*2,x         ; STR_LSTRIP

        leay    op_str_rstrip,pcr
        sty     $7C*2,x         ; STR_RSTRIP

        leay    op_str_strip,pcr
        sty     $7D*2,x         ; STR_STRIP

        leay    op_str_replace_ident,pcr
        sty     $7F*2,x         ; STR_REPLACE_IDENT

        leay    op_list_append,pcr
        sty     $97*2,x

        leay    op_str_append,pcr
        sty     $9A*2,x

        leay    op_slice_append_str,pcr
        sty     $9B*2,x

        leay    op_file_open_read,pcr
        sty     $C0*2,x

        leay    op_file_open_write,pcr
        sty     $C1*2,x

        leay    op_file_readline,pcr
        sty     $C2*2,x

        leay    op_file_write,pcr
        sty     $C3*2,x

        leay    op_file_close,pcr
        sty     $C4*2,x

        leay    op_os_isfile,pcr
        sty     $C5*2,x

        leay    op_os_makedirs,pcr
        sty     $C6*2,x

        leay    op_sys_args,pcr
        sty     $C7*2,x

        leay    op_sys_exit,pcr
        sty     $C8*2,x

        leay    op_print,pcr
        sty     $CA*2,x

        leay    op_println,pcr
        sty     $CB*2,x

        leay    op_file_read,pcr
        sty     $CC*2,x

        leay    op_file_write_buf,pcr
        sty     $CD*2,x

* Initialize VM state by parsing NPC binary header:
        leax    npc_binary,pcr  ; X = binary start

* Clear globals_buf:
        ldu     <data_base
        leau    globals_buf,u
        ldd     #14336/2
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
        std     <frame_sz       ; save entry frame_size in DP
        coma
        comb
        addd    #1              ; D = -entry_frame_size
        leas    d,s             ; allocate entry frame on S
* S is now at start of entry function locals!
        ldd     <frame_sz       ; D = entry frame_size (sets CC!)
        beq     entry_frame_zeroed
        leax    ,s
entry_zero_loop
        clr     ,x+
        subd    #1
        bne     entry_zero_loop
entry_frame_zeroed
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

op_push_nil_slice
        clra
        clrb
        pshu    d
        pshu    d
        pshu    d
        lbra    dispatch

op_push_1
        ldd     #1
        pshu    d
        lbra    dispatch

op_push_neg1
        ldd     #$FFFF
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

op_pop_slice
        leau    6,u
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
        cmpb    #1
        beq     load_local_byte
        cmpb    #2
        beq     load_local_word
        cmpb    #6
        beq     load_local_slice
        bra     load_local_loop
load_local_byte
        ldb     ,x
        clra
        pshu    d
        lbra    dispatch
load_local_word
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
load_local_loop
        lsrb                    ; B = word count
        pshs    b               ; save count on S
ll_words
        ldd     ,x++
        pshu    d
        dec     ,s
        bne     ll_words
        leas    1,s
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
        cmpb    #1
        beq     store_local_byte
        cmpb    #2
        beq     store_local_word
        cmpb    #6
        beq     store_local_slice
        bra     store_local_loop
store_local_byte
        pulu    d
        stb     ,x
        lbra    dispatch
store_local_word
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
store_local_loop
        leax    b,x             ; X = addr + size
        lsrb                    ; B = word count
        pshs    b               ; save count on S
sl_words
        pulu    d
        std     ,--x
        dec     ,s
        bne     sl_words
        leas    1,s
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
        cmpb    #1
        beq     lg_byte
        cmpb    #2
        beq     lg_word
        cmpb    #6
        beq     lg_slice
        lsrb                    ; B = word count
        pshs    b               ; save word count on stack
lg_loop
        ldd     ,x++
        pshu    d
        dec     ,s
        bne     lg_loop
        leas    1,s             ; clean stack
        lbra    dispatch
lg_byte
        ldb     ,x
        clra
        pshu    d
        lbra    dispatch
lg_word
        ldd     ,x
        pshu    d
        lbra    dispatch
lg_slice
        ldd     ,x
        pshu    d
        ldd     2,x
        pshu    d
        ldd     4,x
        pshu    d
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
        cmpb    #1
        beq     sg_byte
        cmpb    #2
        beq     sg_word
        cmpb    #6
        beq     sg_slice
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
sg_byte
        pulu    d
        stb     ,x
        lbra    dispatch
sg_word
        pulu    d
        std     ,x
        lbra    dispatch
sg_slice
        pulu    d
        std     4,x
        pulu    d
        std     2,x
        pulu    d
        std     ,x
        lbra    dispatch

op_peek2
        ldx     ,u              ; X = addr
        ldd     ,x              ; D = word at addr
        std     ,u              ; replace addr with word
        lbra    dispatch

op_poke2
        pulu    d               ; D = val
        pulu    x               ; X = addr
        std     ,x              ; store word at addr
        lbra    dispatch

op_peek1
        ldx     ,u              ; X = addr
        ldb     ,x              ; B = byte at addr
        clra                    ; zero-extend
        std     ,u              ; replace addr with word
        lbra    dispatch

op_poke1
        pulu    d               ; B = val (low byte)
        pulu    x               ; X = addr
        stb     ,x              ; store byte at addr
        lbra    dispatch

op_shl1_add
        pulu    d               ; D = index
        aslb
        rola                    ; D = index << 1
        addd    ,u              ; D = (index << 1) + base
        std     ,u              ; replace base with result
        lbra    dispatch

* Helper: heap_alloc
* Input:  D = requested size in bytes
* Output: X = allocated memory address
heap_alloc
        pshs    y
        cmpd    #14
        bls     ha_b0
        cmpd    #30
        bls     ha_b1
        cmpd    #62
        bls     ha_b2
        cmpd    #126
        bls     ha_b3
        cmpd    #254
        bls     ha_b4
        cmpd    #510
        bls     ha_b5
        cmpd    #1022
        bls     ha_b6
        cmpd    #2046
        bls     ha_b7

* Oversized (> 2046 bytes): bump allocate directly
        addd    #3
        andb    #$FE
        ldx     <heap_ptr
        pshs    x               ; save block address
        leax    d,x             ; X = new heap_ptr
        ldd     <data_base
        addd    #heap_buf+12288 ; D = heap limit
        pshs    d
        cmpx    ,s++            ; compare new heap_ptr (X) with limit
        bhi     ha_oom_ov
        stx     <heap_ptr       ; save valid new heap_ptr
        puls    x               ; X = block address
        ldb     #$FF
        stb     ,x              ; header = $FF (oversized)
        leax    2,x             ; return user pointer
        puls    y,pc

ha_oom_ov
        puls    x               ; drop saved block address
        puls    y               ; restore Y
        ldb     #207            ; E$MemFul
        os9     F$Exit

ha_b0   ldb     #0
        ldy     #16
        bra     ha_bucket
ha_b1   ldb     #1
        ldy     #32
        bra     ha_bucket
ha_b2   ldb     #2
        ldy     #64
        bra     ha_bucket
ha_b3   ldb     #3
        ldy     #128
        bra     ha_bucket
ha_b4   ldb     #4
        ldy     #256
        bra     ha_bucket
ha_b5   ldb     #5
        ldy     #512
        bra     ha_bucket
ha_b6   ldb     #6
        ldy     #1024
        bra     ha_bucket
ha_b7   ldb     #7
        ldy     #2048

ha_bucket
* B = bucket index (0..7), Y = block size (16..2048)
        pshs    b               ; save bucket index (0..7)
        clra
        aslb
        rola                    ; D = B * 2 (0..14)
        ldx     <data_base
        leax    free_buckets,x  ; X = &free_buckets[0]
        leax    d,x             ; X = &free_buckets[B]
        ldd     ,x              ; D = free_buckets[B]
        bne     ha_reuse
        puls    b               ; restore bucket index (0..7)

ha_bump
* Allocate new block of size Y from heap_ptr
* B is untouched and holds bucket index (0..7)
        pshs    b               ; save bucket index
        ldx     <heap_ptr
        pshs    x               ; save block address
        tfr     y,d
        leax    d,x             ; X = new heap_ptr
        ldd     <data_base
        addd    #heap_buf+12288 ; D = heap limit
        pshs    d
        cmpx    ,s++            ; compare new heap_ptr (X) with limit
        bhi     ha_oom
        stx     <heap_ptr       ; save valid new heap_ptr
        puls    x               ; X = block address
        puls    b               ; restore bucket index
        stb     ,x              ; store bucket index in block header
        leax    2,x             ; return user pointer
        puls    y,pc

ha_oom
        puls    x               ; drop saved block address
        puls    b               ; drop saved B
        puls    y               ; restore Y
        ldb     #207            ; E$MemFul
        os9     F$Exit

ha_reuse
* Reuse block from free list:
* D = block address. Block layout: [byte 0: bucket_idx][byte 1: unused][bytes 2-3: next]
* X points directly to free_buckets[B]
        leas    1,s             ; drop saved B
        pshs    d               ; save block address
        tfr     d,y             ; Y = block address
        ldd     2,y             ; D = block->next
        std     ,x              ; free_buckets[B] = block->next
        puls    x               ; X = block address
        leax    2,x             ; return user pointer
        puls    y,pc

* Helper: heap_free
* Input:  D = user pointer to free
heap_free
        cmpd    #0              ; NULL check
        beq     hf_done
        tfr     d,x
        leax    -2,x            ; X = block header
* Bounds check against heap_buf:
        pshs    y
        ldd     <data_base
        addd    #heap_buf       ; D = heap_buf start
        pshs    d
        cmpx    ,s++
        blo     hf_done_y       ; before heap_buf: ignore
        addd    #12288          ; D = heap_buf end
        pshs    d
        cmpx    ,s++
        bhs     hf_done_y       ; at or after heap_buf end: ignore
        ldb     ,x              ; B = bucket index
        cmpb    #7
        bhi     hf_done_y       ; > 7 (oversized or invalid): ignore
* Valid bucket block! Link onto free_buckets[B]:
        clra
        aslb
        rola                    ; D = B * 2
        ldy     <data_base
        leay    free_buckets,y  ; Y = &free_buckets[0]
        leay    d,y             ; Y = &free_buckets[B]
        ldd     ,y              ; D = current head
        std     2,x             ; block->next = current head
        stx     ,y              ; free_buckets[B] = block
hf_done_y
        puls    y
hf_done
        rts

op_buf_alloc
        pulu    d               ; D = requested size
        lbsr    heap_alloc      ; X = allocated address
        pshu    x
        lbra    dispatch

op_zalloc
        pulu    d               ; D = requested size
        pshs    d               ; save size on S
        lbsr    heap_alloc      ; X = allocated address
        puls    d               ; D = size
        pshs    x               ; save allocated address on S
        tsta
        bne     zalloc_loop
        tstb
        beq     zalloc_done
zalloc_loop
        clr     ,x+
        subd    #1
        bne     zalloc_loop
zalloc_done
        puls    x               ; restore allocated address
        pshu    x
        lbra    dispatch

op_buf_free
        pulu    d               ; D = buffer pointer to free
        lbsr    heap_free
        lbra    dispatch

op_dup
        ldd     ,u
        pshu    d
        lbra    dispatch

op_swap
        ldd     ,u
        ldx     2,u
        std     2,u
        stx     ,u
        lbra    dispatch

op_not
        ldd     ,u
        beq     op_not_zero
        clra
        clrb
        std     ,u
        lbra    dispatch
op_not_zero
        ldd     #1
        std     ,u
        lbra    dispatch

op_jump_if_true
        ldd     ,y++            ; signed relative offset
        pulu    x               ; X = condition
        cmpx    #0              ; is condition non-zero?
        lbeq    dispatch        ; 0: do not jump
        leay    d,y             ; !=0: jump!
        lbra    dispatch

op_slice_sub
        pulu    d               ; D = end
        pulu    x               ; X = start
        pshs    d,x             ; 0,s = end, 2,s = start
        ldd     ,s              ; D = end
        subd    2,s             ; D = end - start
        std     ,u              ; update len at ,u
        ldd     2,u             ; D = cap
        subd    2,s             ; D = cap - start
        std     2,u             ; update cap at 2,u
        ldd     4,u             ; D = ptr
        addd    2,s             ; D = ptr + start
        std     4,u             ; update ptr at 4,u
        leas    4,s
        lbra    dispatch

op_str_append
        sty     <vm_pc          ; preserve VM PC
        pulu    d               ; D = char_code
        pshs    d               ; save char_code on S (low byte in 1,s)
        ldd     ,u              ; D = len
        cmpd    2,u             ; len == cap?
        blo     sa_byte_have_room
        ldd     2,u             ; D = old_cap
        aslb
        rola                    ; D = old_cap * 2
        cmpd    #8
        bhs     sa_byte_cap_ok
        ldd     #8
sa_byte_cap_ok
        std     2,u             ; update cap on U
        pshs    d               ; save new_cap
        lbsr    heap_alloc      ; X = new_buf
        puls    d               ; restore new_cap
        ldy     4,u             ; Y = old_ptr
        stx     4,u             ; update ptr on U
        ldd     ,u              ; D = len
        beq     sa_byte_copy_done
sa_byte_copy_loop
        lda     ,y+
        sta     ,x+
        subd    #1
        bne     sa_byte_copy_loop
sa_byte_copy_done
sa_byte_have_room
        ldx     4,u             ; X = ptr
        ldd     ,u              ; D = len
        leax    d,x             ; X = ptr + len
        puls    d               ; B = char_code
        stb     ,x              ; store byte
        ldd     ,u
        addd    #1
        std     ,u              ; len += 1
        ldy     <vm_pc          ; restore VM PC
        lbra    dispatch

op_slice_append_str
        sty     <vm_pc          ; preserve VM PC
        pulu    d               ; s_len
        pshs    d
        pulu    d               ; s_cap
        pshs    d
        pulu    d               ; s_ptr
        pshs    d
* S now has: 0,s = s_ptr, 2,s = s_cap, 4,s = s_len
* U now has: ,u = list_len, 2,u = list_cap, 4,u = list_base
        ldd     ,u              ; D = list_len
        cmpd    2,u             ; list_len == list_cap?
        blo     sa_str_have_room
        ldd     2,u             ; D = old_cap
        aslb
        rola
        cmpd    #8
        bhs     sa_str_cap_ok
        ldd     #8
sa_str_cap_ok
        std     2,u             ; update list_cap on U
        pshs    d               ; 0,s = new_cap
        aslb
        rola                    ; D = new_cap * 2
        addd    ,s              ; D = new_cap * 3
        aslb
        rola                    ; D = new_cap * 6
        leas    2,s             ; drop temp new_cap
        lbsr    heap_alloc      ; X = new_buf
        ldy     4,u             ; Y = old_base
        stx     4,u             ; update list_base on U
        ldd     ,u              ; D = list_len
        beq     sa_str_copy_done
sa_str_copy_loop
        pshs    d
        ldd     ,y++
        std     ,x++
        ldd     ,y++
        std     ,x++
        ldd     ,y++
        std     ,x++
        puls    d
        subd    #1
        bne     sa_str_copy_loop
sa_str_copy_done
sa_str_have_room
* Compute offset = list_len * 6
        ldd     ,u              ; D = list_len
        pshs    d
        aslb
        rola                    ; D * 2
        addd    ,s              ; D * 3
        aslb
        rola                    ; D * 6
        leas    2,s
        ldx     4,u             ; X = list_base
        leax    d,x             ; X = list_base + list_len * 6
        puls    d               ; s_ptr
        std     ,x++
        puls    d               ; s_cap
        std     ,x++
        puls    d               ; s_len
        std     ,x
        ldd     ,u
        addd    #1
        std     ,u              ; list_len += 1
        ldy     <vm_pc          ; restore VM PC
        lbra    dispatch

op_list_append
        sty     <vm_pc          ; preserve VM PC
        pulu    d               ; D = val
        pshs    d               ; save val on S
* U now has: ,u = len, 2,u = cap, 4,u = base
        ldd     ,u              ; D = len
        cmpd    2,u             ; len == cap?
        blo     la_have_room
        ldd     2,u             ; D = old_cap
        aslb
        rola
        cmpd    #8
        bhs     la_cap_ok
        ldd     #8
la_cap_ok
        std     2,u             ; update cap
        pshs    d
        aslb
        rola                    ; D = new_cap * 2
        lbsr    heap_alloc      ; X = new_buf
        puls    d
        ldy     4,u             ; Y = old_base
        stx     4,u             ; update base
        ldd     ,u              ; D = len
        beq     la_copy_done
la_copy_loop
        pshs    d
        ldd     ,y++
        std     ,x++
        puls    d
        subd    #1
        bne     la_copy_loop
la_copy_done
la_have_room
        ldd     ,u              ; D = len
        aslb
        rola                    ; D = len * 2
        ldx     4,u             ; X = base
        leax    d,x
        puls    d               ; D = val
        std     ,x
        ldd     ,u
        addd    #1
        std     ,u              ; len += 1
        ldy     <vm_pc          ; restore VM PC
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

op_neg
        ldd     #0
        subd    ,u
        std     ,u
        lbra    dispatch

op_bit_and
        pulu    d               ; D = b
        anda    ,u
        andb    1,u
        std     ,u
        lbra    dispatch

op_bit_or
        pulu    d               ; D = b
        ora     ,u
        orb     1,u
        std     ,u
        lbra    dispatch

op_bit_xor
        pulu    d               ; D = b
        eora    ,u
        eorb    1,u
        std     ,u
        lbra    dispatch

op_bit_not
        com     ,u
        com     1,u
        lbra    dispatch

op_shl
        pulu    x               ; X = count
        ldd     ,u              ; D = val
        cmpx    #0
        beq     shl_done
shl_loop
        aslb
        rola
        leax    -1,x
        bne     shl_loop
shl_done
        std     ,u
        lbra    dispatch

op_shr
        pulu    x               ; X = count
        ldd     ,u              ; D = val
        cmpx    #0
        beq     shr_done
shr_loop
        lsra
        rorb
        leax    -1,x
        bne     shr_loop
shr_done
        std     ,u
        lbra    dispatch

op_cmp_eq
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
        beq     cmp_true
        bra     cmp_false

op_cmp_ne
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
        bne     cmp_true
        bra     cmp_false

op_cmp_lt
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b (borrow -> C=1)
        leau    2,u             ; drop b
        blo     cmp_true        ; unsigned a < b
        bra     cmp_false

op_cmp_le
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b (sets C and Z)
        leau    2,u             ; drop b
        bls     cmp_true        ; unsigned a <= b
        bra     cmp_false

op_cmp_gt
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
        bhi     cmp_true        ; unsigned a > b
        bra     cmp_false

op_cmp_ge
        ldd     2,u             ; D = a
        subd    ,u              ; D = a - b
        leau    2,u             ; drop b
        bhs     cmp_true        ; unsigned a >= b

cmp_false
        clra
        clrb
        std     ,u
        lbra    dispatch

cmp_true
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
        std     <frame_sz       ; save target frame_size in DP
        ldd     4,x             ; D = target code_offset
        ldy     <code_base_ptr
        leay    d,y             ; Y = target entry PC!
        ldd     8,x             ; D = target var_table_offset
        ldx     <ltab_ptr
        leax    d,x
        stx     <callee_vtab    ; callee's var_table pointer in DP temp

* Allocate and zero frame_size on S:
        ldd     <frame_sz       ; D = target_frame_size
        coma
        comb
        addd    #1              ; D = -target_frame_size
        leas    d,s             ; S = S - target_frame_size
        ldd     <frame_sz       ; D = target_frame_size (sets CC!)
        beq     call_frame_zeroed
        leax    ,s              ; X = start of frame
call_zero_loop
        clr     ,x+
        subd    #1
        bne     call_zero_loop
call_frame_zeroed

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
        cmpb    #1
        beq     call_arg_byte
        cmpb    #2
        beq     call_arg_word
        cmpb    #6
        beq     call_arg_slice
        bra     call_arg_loop_words
call_arg_byte
        pulu    d
        stb     ,x
        bra     call_next_arg
call_arg_word
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
        bra     call_next_arg
call_arg_loop_words
        leax    b,x             ; X = addr + size
        lsrb                    ; B = word count
        pshs    b               ; save count on S
ca_words
        pulu    d
        std     ,--x
        dec     ,s
        bne     ca_words
        leas    1,s
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

op_slice_cap
        ldd     2,u             ; D = cap
        leau    6,u             ; drop slice (len, cap, ptr)
        pshu    d               ; push cap
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

********************************************************************
* cic_check: Helper to check if char in A is in chars set
* Input:  A = char to test
*         Y = chars_ptr (if 0, check default whitespace: SP, TAB, LF, CR)
*         D = chars_len
* Output: B = 1 if match, 0 if not
* Preserves: A
********************************************************************
cic_check
        cmpy    #0
        bne     cic_custom
* Default whitespace: SP, TAB, LF, CR
        cmpa    #' '
        beq     cic_yes
        cmpa    #9
        beq     cic_yes
        cmpa    #10
        beq     cic_yes
        cmpa    #13
        beq     cic_yes
        clrb
        rts

cic_custom
        subd    #0              ; test if D == 0
        beq     cic_no
        pshs    d,y
        ldx     ,s              ; X = loop count
cic_loop
        cmpa    ,y+
        beq     cic_found
        leax    -1,x
        bne     cic_loop
        puls    d,y
cic_no
        clrb
        rts

cic_found
        puls    d,y
cic_yes
        ldb     #1
        rts

********************************************************************
* op_str_startswith ($78): [str, pfx] -> [bool]
********************************************************************
op_str_startswith
        sty     <vm_pc
        pulu    d               ; D = pfx_len
        pulu    x               ; discard pfx_cap
        pulu    y               ; Y = pfx_ptr
        pshs    d,y             ; 0,s = pfx_len, 2,s = pfx_ptr

        pulu    d               ; D = str_len
        pulu    x               ; discard str_cap
        pulu    x               ; X = str_ptr

        cmpd    ,s              ; compare str_len with pfx_len
        blo     sw_false        ; str_len < pfx_len -> false

        ldd     ,s              ; D = pfx_len
        beq     sw_true         ; pfx_len == 0 -> true
        pshs    d               ; 0,s = count loop, 2,s = pfx_len, 4,s = pfx_ptr
        ldy     4,s             ; Y = pfx_ptr

sw_loop
        lda     ,x+
        cmpa    ,y+
        bne     sw_mismatch
        ldd     ,s
        subd    #1
        std     ,s
        bne     sw_loop

        leas    2,s             ; drop loop count
sw_true
        leas    4,s             ; drop pfx_len, pfx_ptr
        ldd     #1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

sw_mismatch
        leas    2,s             ; drop loop count
sw_false
        leas    4,s             ; drop pfx_len, pfx_ptr
        ldd     #0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* op_str_endswith ($79): [str, sfx] -> [bool]
********************************************************************
op_str_endswith
        sty     <vm_pc
        pulu    d               ; D = sfx_len
        pulu    x               ; discard sfx_cap
        pulu    y               ; Y = sfx_ptr
        pshs    d,y             ; 0,s = sfx_len, 2,s = sfx_ptr

        pulu    d               ; D = str_len
        pulu    x               ; discard str_cap
        pulu    x               ; X = str_ptr

        cmpd    ,s              ; compare str_len with sfx_len
        blo     ew_false        ; str_len < sfx_len -> false

        subd    ,s              ; D = str_len - sfx_len
        leax    d,x             ; X = str_ptr + offset

        ldd     ,s              ; D = sfx_len
        beq     ew_true
        pshs    d               ; 0,s = loop count
        ldy     4,s             ; Y = sfx_ptr

ew_loop
        lda     ,x+
        cmpa    ,y+
        bne     ew_mismatch
        ldd     ,s
        subd    #1
        std     ,s
        bne     ew_loop

        leas    2,s             ; drop loop count
ew_true
        leas    4,s
        ldd     #1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

ew_mismatch
        leas    2,s
ew_false
        leas    4,s
        ldd     #0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* op_str_find ($7A): [str, sub, start] -> [idx]
********************************************************************
op_str_find
        sty     <vm_pc
        pulu    d               ; D = start
        pshs    d               ; 0,s = start

        pulu    d               ; D = sub_len
        pulu    x               ; discard sub_cap
        pulu    y               ; Y = sub_ptr
        pshs    d,y             ; 0,s = sub_len, 2,s = sub_ptr, 4,s = start

        pulu    d               ; D = str_len
        pulu    x               ; discard str_cap
        pulu    x               ; X = str_ptr
        pshs    d,x             ; 0,s = str_len, 2,s = str_ptr, 4,s = sub_len, 6,s = sub_ptr, 8,s = start

        ldd     4,s             ; sub_len
        bne     sf_nonempty
* Empty needle:
        ldd     8,s             ; start
        cmpd    0,s             ; compare with str_len
        bls     sf_empty_ok
        ldd     0,s             ; min(start, str_len)
sf_empty_ok
        leas    10,s
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

sf_nonempty
        ldd     8,s             ; start
        cmpd    0,s             ; start vs str_len
        bhs     sf_not_found

        ldd     0,s             ; str_len
        subd    4,s             ; str_len - sub_len
        blo     sf_not_found    ; sub_len > str_len
        cmpd    8,s             ; max_start vs start
        blo     sf_not_found    ; start > max_start

        pshs    d               ; 0,s = max_start
* Stack:
* 0,s = max_start
* 2,s = str_len
* 4,s = str_ptr
* 6,s = sub_len
* 8,s = sub_ptr
* 10,s = start (candidate index)

sf_cand_loop
        ldd     10,s            ; D = cur_idx
        ldx     4,s             ; X = str_ptr
        leax    d,x             ; X = str_ptr + cur_idx
        ldy     8,s             ; Y = sub_ptr
        ldd     6,s             ; D = sub_len
        pshs    d               ; 0,s = byte count

sf_byte_loop
        lda     ,x+
        cmpa    ,y+
        bne     sf_cand_mismatch
        ldd     ,s
        subd    #1
        std     ,s
        bne     sf_byte_loop

* All bytes matched!
        leas    2,s             ; drop byte count
        ldd     10,s            ; D = cur_idx
        leas    12,s            ; drop max_start, str_len, str_ptr, sub_len, sub_ptr, start
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

sf_cand_mismatch
        leas    2,s             ; drop byte count
        ldd     10,s            ; cur_idx
        cmpd    0,s             ; cur_idx vs max_start
        bhs     sf_exhausted
        addd    #1
        std     10,s            ; cur_idx++
        bra     sf_cand_loop

sf_exhausted
        leas    2,s             ; drop max_start
sf_not_found
        leas    10,s            ; drop vars
        ldd     #$FFFF          ; -1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* op_str_lstrip ($7B): [str, chars] -> [view]
********************************************************************
op_str_lstrip
        sty     <vm_pc
        pulu    d               ; chars_len
        pulu    x               ; chars_cap
        pulu    y               ; chars_ptr
        pshs    d,y             ; 0,s = chars_len, 2,s = chars_ptr

        pulu    d               ; str_len
        pulu    x               ; str_cap
        pulu    y               ; str_ptr
        pshs    d,x,y           ; 0,s = str_len, 2,s = str_cap, 4,s = str_ptr, 6,s = chars_len, 8,s = chars_ptr

        ldd     #0
        pshs    d               ; 0,s = i (offset)
* Stack:
* 0,s = i
* 2,s = str_len
* 4,s = str_cap
* 6,s = str_ptr
* 8,s = chars_len
* 10,s = chars_ptr

ls_loop
        ldd     ,s              ; D = i
        cmpd    2,s             ; i vs str_len
        bhs     ls_done
        ldx     6,s             ; str_ptr
        lda     d,x             ; A = str_ptr[i]
        ldy     10,s            ; chars_ptr
        ldd     8,s             ; chars_len
        lbsr    cic_check
        tstb
        beq     ls_done         ; not in chars -> stop

        ldd     ,s
        addd    #1
        std     ,s              ; i++
        bra     ls_loop

ls_done
        ldd     ,s              ; D = i
        ldx     6,s             ; str_ptr
        leax    d,x             ; X = new_ptr
        pshu    x               ; push ptr to U

        ldd     4,s             ; str_cap
        subd    ,s              ; str_cap - i
        pshu    d               ; push cap to U

        ldd     2,s             ; str_len
        subd    ,s              ; str_len - i
        pshu    d               ; push len to U

        leas    12,s            ; drop i(2), str_len(2), str_cap(2), str_ptr(2), chars_len(2), chars_ptr(2)
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* op_str_rstrip ($7C): [str, chars] -> [view]
********************************************************************
op_str_rstrip
        sty     <vm_pc
        pulu    d               ; chars_len
        pulu    x               ; chars_cap
        pulu    y               ; chars_ptr
        pshs    d,y             ; 0,s = chars_len, 2,s = chars_ptr

        pulu    d               ; str_len
        pulu    x               ; str_cap
        pulu    y               ; str_ptr
        pshs    d,x,y           ; 0,s = str_len, 2,s = str_cap, 4,s = str_ptr, 6,s = chars_len, 8,s = chars_ptr

rs_loop
        ldd     ,s              ; str_len
        beq     rs_done
        subd    #1              ; D = str_len - 1
        ldx     4,s             ; str_ptr
        lda     d,x             ; A = str_ptr[str_len - 1]
        ldy     8,s             ; chars_ptr
        ldd     6,s             ; chars_len
        lbsr    cic_check
        tstb
        beq     rs_done

        ldd     ,s
        subd    #1
        std     ,s              ; str_len--
        bra     rs_loop

rs_done
        ldx     4,s             ; str_ptr
        ldy     2,s             ; str_cap
        ldd     ,s              ; str_len
        pshu    x
        pshu    y
        pshu    d

        leas    10,s
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* op_str_strip ($7D): [str, chars] -> [view]
********************************************************************
op_str_strip
        sty     <vm_pc
        pulu    d               ; chars_len
        pulu    x               ; chars_cap
        pulu    y               ; chars_ptr
        pshs    d,y             ; 0,s = chars_len, 2,s = chars_ptr

        pulu    d               ; str_len
        pulu    x               ; str_cap
        pulu    y               ; str_ptr
        pshs    d,x,y           ; 0,s = str_len, 2,s = str_cap, 4,s = str_ptr, 6,s = chars_len, 8,s = chars_ptr

* Find start index:
        ldd     #0
        pshs    d               ; 0,s = start_idx
* Stack:
* 0,s = start_idx
* 2,s = str_len
* 4,s = str_cap
* 6,s = str_ptr
* 8,s = chars_len
* 10,s = chars_ptr

st_l_loop
        ldd     ,s              ; start_idx
        cmpd    2,s             ; vs str_len
        bhs     st_l_done
        ldx     6,s             ; str_ptr
        lda     d,x
        ldy     10,s            ; chars_ptr
        ldd     8,s             ; chars_len
        lbsr    cic_check
        tstb
        beq     st_l_done

        ldd     ,s
        addd    #1
        std     ,s              ; start_idx++
        bra     st_l_loop

st_l_done
* Now end_idx starts at str_len
        ldd     2,s             ; str_len
        pshs    d               ; 0,s = end_idx
* Stack:
* 0,s = end_idx
* 2,s = start_idx
* 4,s = str_len
* 6,s = str_cap
* 8,s = str_ptr
* 10,s = chars_len
* 12,s = chars_ptr

st_r_loop
        ldd     ,s              ; end_idx
        cmpd    2,s             ; compare end_idx with start_idx
        bls     st_done
        subd    #1              ; end_idx - 1
        ldx     8,s             ; str_ptr
        lda     d,x
        ldy     12,s            ; chars_ptr
        ldd     10,s            ; chars_len
        lbsr    cic_check
        tstb
        beq     st_done

        ldd     ,s
        subd    #1
        std     ,s              ; end_idx--
        bra     st_r_loop

st_done
        ldd     2,s             ; start_idx
        ldx     8,s             ; str_ptr
        leax    d,x             ; X = new_ptr
        pshu    x               ; push ptr to U

        ldd     6,s             ; str_cap
        subd    2,s             ; str_cap - start_idx
        pshu    d               ; push cap to U

        ldd     ,s              ; end_idx
        subd    2,s             ; end_idx - start_idx
        pshu    d               ; push len to U

        leas    14,s
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* is_ident_char: Helper to check if char in A is in [A-Za-z0-9_.]
* Input:  A = character
* Output: B = 1 if identifier char, 0 otherwise
* Preserves: A
********************************************************************
is_ident_char
        cmpa    #'A'
        blo     iic_not_upper
        cmpa    #'Z'
        bls     iic_yes
iic_not_upper
        cmpa    #'a'
        blo     iic_not_lower
        cmpa    #'z'
        bls     iic_yes
iic_not_lower
        cmpa    #'0'
        blo     iic_not_digit
        cmpa    #'9'
        bls     iic_yes
iic_not_digit
        cmpa    #'_'
        beq     iic_yes
        cmpa    #'.
        beq     iic_yes
        clrb
        rts
iic_yes
        ldb     #1
        rts

********************************************************************
* check_ident_match_at_pos: Helper for op_str_replace_ident
* Evaluates word-boundary match at frame's pos.
* Input: frame on S (with return address at 0,s, offsets +2):
*   6,s  = pos
*   10,s = src_ptr
*   14,s = src_len
*   16,s = old_len
*   18,s = old_ptr
* Output: B = 1 if match, 0 if not
********************************************************************
check_ident_match_at_pos
* Check if pos <= src_len - old_len
        ldd     10,s            ; src_len
        subd    16,s            ; src_len - old_len
        blo     cim_no
        cmpd    6,s             ; vs pos
        blo     cim_no

* Boundary before: if pos > 0, src[pos-1] must NOT be ident char
        ldd     6,s             ; pos
        beq     cim_before_ok
        subd    #1
        ldx     14,s            ; src_ptr
        lda     d,x
        lbsr    is_ident_char
        tstb
        bne     cim_no

cim_before_ok
* Boundary after: if pos + old_len < src_len, src[pos+old_len] must NOT be ident char
        ldd     6,s             ; pos
        addd    16,s            ; pos + old_len
        cmpd    10,s            ; vs src_len
        bhs     cim_after_ok
        ldx     14,s            ; src_ptr
        lda     d,x
        lbsr    is_ident_char
        tstb
        bne     cim_no

cim_after_ok
* Compare old_len bytes at src_ptr + pos with old_ptr
        ldd     6,s             ; pos
        ldx     14,s            ; src_ptr
        leax    d,x             ; X = src_ptr + pos
        ldy     18,s            ; Y = old_ptr
        ldd     16,s            ; old_len
        pshs    d               ; 0,s = count

cim_cmp_loop
        lda     ,x+
        cmpa    ,y+
        bne     cim_mismatch
        ldd     ,s
        subd    #1
        std     ,s
        bne     cim_cmp_loop

* All bytes matched!
        leas    2,s             ; drop count
        ldb     #1
        rts

cim_mismatch
        leas    2,s             ; drop count
cim_no
        clrb
        rts

********************************************************************
* op_str_replace_ident ($7F): [src, old, new] -> [res]
********************************************************************
op_str_replace_ident
        sty     <vm_pc
        pulu    d               ; new_len
        pulu    x               ; new_cap
        pulu    y               ; new_ptr
        pshs    d,y             ; 0,s = new_len, 2,s = new_ptr

        pulu    d               ; old_len
        pulu    x               ; old_cap
        pulu    y               ; old_ptr
        pshs    d,y             ; 0,s = old_len, 2,s = old_ptr, 4,s = new_len, 6,s = new_ptr

        pulu    d               ; src_len
        pulu    x               ; src_cap
        pulu    y               ; src_ptr
        pshs    d,x,y           ; 0,s = src_len, 2,s = src_cap, 4,s = src_ptr, 6,s = old_len, 8,s = old_ptr, 10,s = new_len, 12,s = new_ptr

        leas    -8,s            ; allocate work area
* Frame layout (22 bytes total):
* 0,s  = out_ptr (2 bytes)
* 2,s  = out_idx (2 bytes)
* 4,s  = pos (2 bytes)
* 6,s  = num_matches / new_total_len (2 bytes)
* 8,s  = src_len (2 bytes)
* 10,s = src_cap (2 bytes)
* 12,s = src_ptr (2 bytes)
* 14,s = old_len (2 bytes)
* 16,s = old_ptr (2 bytes)
* 18,s = new_len (2 bytes)
* 20,s = new_ptr (2 bytes)

* Check trivial conditions:
        ldd     14,s            ; old_len
        beq     ri_return_orig
        ldd     8,s             ; src_len
        cmpd    14,s            ; src_len vs old_len
        blo     ri_return_orig

* Initialize Pass 1:
        ldd     #0
        std     4,s             ; pos = 0
        std     6,s             ; num_matches = 0

ri_p1_loop
        ldd     4,s             ; pos
        cmpd    8,s             ; pos vs src_len
        bhs     ri_p1_done
        lbsr    check_ident_match_at_pos
        tstb
        beq     ri_p1_no_match

* Match found:
        ldd     6,s
        addd    #1
        std     6,s             ; num_matches++
        ldd     4,s
        addd    14,s            ; pos += old_len
        std     4,s
        bra     ri_p1_loop

ri_p1_no_match
        ldd     4,s
        addd    #1
        std     4,s             ; pos++
        bra     ri_p1_loop

ri_p1_done
        ldd     6,s             ; num_matches
        bne     ri_do_replace
        bra     ri_return_orig

ri_return_orig
        ldx     12,s            ; src_ptr
        ldy     10,s            ; src_cap
        ldd     8,s             ; src_len
        leas    22,s            ; drop frame
        pshu    x
        pshu    y
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

ri_do_replace
* Calculate new_total_len = src_len + num_matches * (new_len - old_len)
        ldx     6,s             ; X = num_matches
        ldd     8,s             ; D = src_len
        std     6,s             ; 6,s = new_total_len accumulator
ri_len_loop
        cmpx    #0
        beq     ri_len_done
        ldd     6,s
        addd    18,s            ; + new_len
        subd    14,s            ; - old_len
        std     6,s
        leax    -1,x
        bra     ri_len_loop

ri_len_done
        ldd     6,s             ; D = new_total_len
        lbsr    heap_alloc      ; X = allocated buffer
        stx     0,s             ; out_ptr
        ldd     #0
        std     2,s             ; out_idx = 0
        std     4,s             ; pos = 0

ri_p2_loop
        ldd     4,s             ; pos
        cmpd    8,s             ; pos vs src_len
        bhs     ri_p2_done
        lbsr    check_ident_match_at_pos
        tstb
        beq     ri_p2_single

* Match in Pass 2: copy new string
        ldd     18,s            ; new_len
        beq     ri_p2_skip_copy
        pshs    d               ; 0,s = count
        ldy     22,s            ; Y = new_ptr (shifted by 2 due to pshs d)
        ldd     4,s             ; out_idx (shifted by 2)
        ldx     2,s             ; out_ptr (shifted by 2)
        leax    d,x             ; X = out_ptr + out_idx
ri_p2_cpy_new
        lda     ,y+
        sta     ,x+
        ldd     ,s
        subd    #1
        std     ,s
        bne     ri_p2_cpy_new
        leas    2,s             ; drop count

ri_p2_skip_copy
        ldd     2,s             ; out_idx
        addd    18,s            ; out_idx += new_len
        std     2,s
        ldd     4,s             ; pos
        addd    14,s            ; pos += old_len
        std     4,s
        bra     ri_p2_loop

ri_p2_single
        ldd     4,s             ; pos
        ldx     12,s            ; src_ptr
        lda     d,x             ; A = src_ptr[pos]
        ldd     2,s             ; out_idx
        ldx     0,s             ; out_ptr
        sta     d,x             ; out_ptr[out_idx] = A
        ldd     2,s
        addd    #1
        std     2,s             ; out_idx++
        ldd     4,s
        addd    #1
        std     4,s             ; pos++
        bra     ri_p2_loop

ri_p2_done
        ldx     0,s             ; out_ptr
        ldy     6,s             ; new_total_len (cap)
        ldd     6,s             ; new_total_len (len)
        leas    22,s            ; drop frame
        pshu    x
        pshu    y
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
        adda    #7
oi_d1
        adda    #'0
        sta     >$FF89          ; print high hex digit
        puls    a
        anda    #$0F
        cmpa    #9
        bls     oi_d2
        adda    #7
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
* op_print / op_println: Print Slice[any] to standard output
********************************************************************
op_print
        clr     <vm_is_println
        bra     do_print

op_println
        lda     #1
        sta     <vm_is_println
        bra     do_print

do_print
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
        tst     <vm_is_println
        beq     pr_write_raw
        lda     #$0D            ; append CR ($0D, OS-9 line terminator)
        sta     ,y+
        ldx     <data_base
        leax    line_buf,x      ; X = start of line_buf
        tfr     y,d             ; D = end of line_buf
        pshs    x
        subd    ,s++            ; D = count of bytes
        tfr     d,y             ; Y = length for I$WritLn
        lda     #1              ; path 1 = stdout
        os9     I$WritLn
        bra     pr_finish

pr_write_raw
        ldx     <data_base
        leax    line_buf,x      ; X = start of line_buf
        tfr     y,d             ; D = end of line_buf
        pshs    x
        subd    ,s++            ; D = count of bytes
        beq     pr_finish       ; 0 bytes -> nothing to write
        tfr     d,y             ; Y = length for I$Write
        lda     #1              ; path 1 = stdout
        os9     I$Write

pr_finish
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
* op_div / op_mod / udiv16: 16-bit Unsigned Division & Modulo
********************************************************************
op_div
        ldx     ,u              ; X = b (divisor)
        ldd     2,u             ; D = a (dividend)
        lbsr    udiv16          ; D = a / b, X = a % b
        leau    2,u             ; drop b
        std     ,u              ; replace a with quotient
        lbra    dispatch

op_mod
        ldx     ,u              ; X = b (divisor)
        ldd     2,u             ; D = a (dividend)
        lbsr    udiv16          ; D = a / b, X = a % b
        leau    2,u             ; drop b
        stx     ,u              ; replace a with remainder
        lbra    dispatch

udiv16
        cmpx    #0
        bne     do_udiv16
        clra
        clrb
        ldx     #0
        rts
do_udiv16
        pshs    x               ; 3,s = divisor
        pshs    d               ; 1,s = dividend
        lda     #16
        pshs    a               ; 0,s = loop counter
        clra
        clrb                    ; D = 0 (remainder)
udiv_loop
        lsl     2,s             ; shift dividend low byte
        rol     1,s             ; shift dividend high byte
        rolb                    ; shift carry into remainder D
        rola
        cmpd    3,s             ; compare remainder with divisor
        blo     udiv_skip
        subd    3,s             ; remainder -= divisor
        inc     2,s             ; quotient low bit = 1
udiv_skip
        dec     ,s
        bne     udiv_loop
        tfr     d,x             ; X = remainder
        ldd     1,s             ; D = quotient
        leas    5,s             ; drop counter (1), dividend (2), divisor (2)
        rts

********************************************************************
* copy_path: helper to copy string slice to path_scratch ($0D-terminated)
* Input:  X = string base pointer, D = string length
* Output: X = pointer to path_scratch
********************************************************************
copy_path
        pshs    u,y
        ldu     <data_base
        leau    path_scratch,u  ; U = destination
        pshs    u               ; save destination pointer
        tfr     d,y             ; Y = count
        cmpy    #62
        bls     cp_len_ok
        ldy     #62
cp_len_ok
        cmpy    #0
        beq     cp_end
cp_loop
        lda     ,x+
        sta     ,u+
        leay    -1,y
        bne     cp_loop
cp_end
        lda     #$0D            ; OS-9 path terminator
        sta     ,u
        puls    x               ; X = path_scratch
        puls    u,y,pc

********************************************************************
* File I/O Opcodes ($C0..$C4, $CC, $CD)
********************************************************************
op_file_open_read
        sty     <vm_pc
        pulu    d               ; D = len
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base
        lbsr    copy_path       ; X = path_scratch ($0D-terminated)
        lda     #1              ; READ.
        os9     I$Open
        bcs     open_r_fail
        tfr     a,b
        clra                    ; D = handle
        pshu    d
        ldd     #1              ; ok = 1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
open_r_fail
        ldd     #0
        pshu    d               ; handle = 0
        pshu    d               ; ok = 0
        ldy     <vm_pc
        lbra    dispatch

op_file_open_write
        sty     <vm_pc
        pulu    d               ; D = len
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base
        lbsr    copy_path       ; X = path_scratch ($0D-terminated)
        lda     #2              ; WRITE.
        ldb     #3              ; read/write attributes
        os9     I$Create
        bcc     open_w_ok
        cmpb    #218            ; E$CE (file exists)?
        bne     open_w_fail
        ldx     <data_base
        leax    path_scratch,x
        os9     I$Delete
        ldx     <data_base
        leax    path_scratch,x
        lda     #2
        ldb     #3
        os9     I$Create
        bcs     open_w_fail
open_w_ok
        tfr     a,b
        clra                    ; D = handle
        pshu    d
        ldd     #1              ; ok = 1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
open_w_fail
        ldd     #0
        pshu    d               ; handle = 0
        pshu    d               ; ok = 0
        ldy     <vm_pc
        lbra    dispatch

op_file_readline
        sty     <vm_pc
        pulu    d               ; D = handle
        tfr     b,a             ; A = path
        pshs    u               ; save eval stack U
        ldx     <data_base
        leax    line_buf,x      ; X = line_buf
        ldy     #255            ; max length
        os9     I$ReadLn
        bcs     rdln_eof
        cmpy    #0
        beq     rdln_eof
        tfr     y,d             ; D = length read
        pshs    d               ; save length
        lbsr    heap_alloc      ; X = allocated heap memory
        puls    d               ; D = length
        ldy     <data_base
        leay    line_buf,y      ; Y = source line_buf
        pshs    d,x             ; 0,s = buffer, 2,s = length
        tfr     d,u             ; U = count
rdln_cp
        lda     ,y+
        sta     ,x+
        leau    -1,u
        cmpu    #0
        bne     rdln_cp
        puls    d,x             ; D = length, X = buffer
        puls    u               ; restore eval stack U
        pshu    x               ; Base = X
        pshu    d               ; Cap = D
        pshu    d               ; Len = D
        ldd     #0              ; eof = 0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
rdln_eof
        puls    u               ; restore eval stack U
        ldd     #0
        pshu    d               ; Base = 0
        pshu    d               ; Cap = 0
        pshu    d               ; Len = 0
        ldd     #1              ; eof = 1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_file_write
        sty     <vm_pc
        pulu    d               ; D = len
        tfr     d,y             ; Y = length
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base
        pulu    d               ; D = handle
        tfr     b,a             ; A = path
        os9     I$Write
        bcs     wr_fail
        ldd     #1              ; ok = 1
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
wr_fail
        ldd     #0              ; ok = 0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_file_write_buf
        sty     <vm_pc
        pulu    y               ; Y = count
        pulu    x               ; X = buf_addr
        pulu    d               ; D = handle
        tfr     b,a             ; A = path
        os9     I$Write
        bcs     wr_buf_fail
        tfr     y,d             ; D = count written
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
wr_buf_fail
        ldd     #0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_file_read
        sty     <vm_pc
        pulu    y               ; Y = count
        pulu    x               ; X = buf_addr
        pulu    d               ; D = handle
        tfr     b,a             ; A = path
        os9     I$Read
        bcs     rd_fail
        tfr     y,d             ; D = actual count read
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
rd_fail
        ldd     #0
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_file_close
        sty     <vm_pc
        pulu    d               ; D = handle
        tfr     b,a             ; A = path
        os9     I$Close
        ldy     <vm_pc
        lbra    dispatch

op_os_isfile
        sty     <vm_pc
        pulu    d               ; D = len
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base
        lbsr    copy_path
        lda     #1              ; READ.
        os9     I$Open
        bcs     is_not_file
        os9     I$Close
        ldd     #1              ; true
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
is_not_file
        ldd     #0              ; false
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_os_makedirs
        sty     <vm_pc
        pulu    d               ; D = len
        pulu    x               ; X = cap (discard)
        pulu    x               ; X = base
        lbsr    copy_path
        ldb     #3              ; read/write
        os9     I$MakDir
        bcc     mak_ok
        cmpb    #218            ; E$CE (exists)?
        beq     mak_ok
        ldd     #0              ; false
        pshu    d
        ldy     <vm_pc
        lbra    dispatch
mak_ok
        ldd     #1              ; true
        pshu    d
        ldy     <vm_pc
        lbra    dispatch

op_sys_exit
        pulu    d               ; D = code
        tfr     b,a             ; A = status
        os9     F$Exit

********************************************************************
* op_sys_args: Parse CLI parameters into string slice list
********************************************************************
op_sys_args
        sty     <vm_pc
        pshs    u               ; save eval stack U
        ldx     <param_ptr
        cmpx    #0
        lbeq    sa_empty

        ; Pass 1: count number of tokens
        clra
        clrb
        pshs    d               ; 0,s = token count N
sa_scan_lead
        lda     ,x
        cmpa    #$0D
        beq     sa_count_done
        cmpa    #' '
        beq     sa_skip_space
        cmpa    #9
        beq     sa_skip_space
        ; Found token start!
        inc     1,s
sa_in_tok
        lda     ,x+
        cmpa    #$0D
        beq     sa_count_done
        cmpa    #' '
        beq     sa_scan_lead
        cmpa    #9
        beq     sa_scan_lead
        bra     sa_in_tok
sa_skip_space
        leax    1,x
        bra     sa_scan_lead

sa_count_done
        ldd     ,s              ; D = token count N
        lbeq    sa_empty_cnt
        pshs    d               ; 0,s = N (token count), 2,s = saved N
        aslb
        rola                    ; D = N * 2
        addd    ,s              ; D = N * 3
        aslb
        rola                    ; D = N * 6
        lbsr    heap_alloc      ; X = allocated slice buffer
        pshs    x               ; 0,s = list_buf (fixed base), 2,s = N, 4,s = N
        pshs    x               ; 0,s = list_buf write ptr

        ; Pass 2: populate list_buf with string slices
        ldx     <param_ptr      ; reset X to param string
sa_tok2_lead
        lda     ,x
        cmpa    #$0D
        beq     sa_tok_finish
        cmpa    #' '
        beq     sa_skip2
        cmpa    #9
        beq     sa_skip2
        ; Start of token at X
        tfr     x,y             ; Y = start of token
sa_tok2_end
        lda     ,x+
        cmpa    #$0D
        beq     sa_tok2_found
        cmpa    #' '
        beq     sa_tok2_found
        cmpa    #9
        beq     sa_tok2_found
        bra     sa_tok2_end

sa_tok2_found
        clrb                    ; B = 0, so D = delimiter word (A in high byte)
        pshs    d               ; 0,s = delimiter word
        pshs    x               ; 0,s = next char ptr in param string, 2,s = delimiter word
        tfr     x,d
        subd    #1              ; D = end of token
        pshs    y
        subd    ,s++            ; D = length of token = (X - 1) - Y
        pshs    d               ; 0,s = token length, 2,s = next char ptr, 4,s = delimiter
        lbsr    heap_alloc      ; X = allocated string buffer
        ldd     ,s              ; D = token length
        pshs    d,x             ; 0,s = allocated buffer, 2,s = token length
        tfr     d,u             ; U = loop counter (length)
sa_cp_loop
        cmpu    #0
        beq     sa_cp_done
        lda     ,y+
        sta     ,x+
        leau    -1,u
        bra     sa_cp_loop
sa_cp_done
        puls    d,x             ; D = token length, X = allocated buffer
        ldu     6,s             ; U = list_buf write ptr
        stx     ,u++            ; Base = allocated buffer
        std     ,u++            ; Cap = length
        std     ,u++            ; Len = length
        stu     6,s             ; update list_buf write ptr
        leas    2,s             ; drop token length
        puls    x               ; restore next char ptr in param string
        puls    d               ; restore delimiter word (A = delimiter)
        cmpa    #$0D
        beq     sa_tok_finish
        bra     sa_tok2_lead

sa_skip2
        leax    1,x
        bra     sa_tok2_lead

sa_tok_finish
        leas    2,s             ; drop write ptr
        puls    x               ; X = list_buf base
        puls    d               ; D = N
        leas    2,s             ; drop saved N
        puls    u               ; U = eval stack
        pshu    x               ; Base = list_buf
        pshu    d               ; Cap = N
        pshu    d               ; Len = N
        ldy     <vm_pc
        lbra    dispatch

sa_empty_cnt
        leas    2,s             ; drop count
sa_empty
        puls    u               ; U = eval stack
        ldd     #0
        pshu    d               ; Base = 0
        pshu    d               ; Cap = 0
        pshu    d               ; Len = 0
        ldy     <vm_pc
        lbra    dispatch

********************************************************************
* Baked NPC Binary Payload
********************************************************************
npc_binary
        use     payload.asm
npc_binary_end  equ     *

        emod
eom     equ     *
        end
