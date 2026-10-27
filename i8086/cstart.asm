; ── C Runtime Startup for Intel 8086 on Hatvan VM/0 (gep0) ──────────────────
	SEGMENT CODE
	ORG     $0200

cstart:
	CLI
	CLD
	XOR     AX, AX
	MOV     DS, AX
	MOV     SS, AX
	MOV     ES, AX
	MOV     SP, $FE00
	MOV     BP, SP

	; Call MiniGolf user main
	CALL    _main

	; Exit with return code in AX (low byte)
	PUSH    AX
	CALL    _exit

; ── Exit System Call ─────────────────────────────────────────────────────────
_exit:
__exit:
	MOV     BP, SP
	MOV     AL, [BP+2]
	MOV     [$FF05], AL
.L_exit_halt:
	HLT
	JMP     .L_exit_halt

; ── Console Character I/O ───────────────────────────────────────────────────
_putchar:
putchar:
	MOV     BX, SP
	MOV     AL, [BX+2]
	MOV     [$FF00], AL
	RET

_getchar:
getchar:
	MOV     AL, [$FF01]
	MOV     AH, 0
	RET

; ── Minimal _printf Implementation for Intel 8086 ───────────────────────────
_printf:
	PUSH    BP
	MOV     BP, SP
	PUSH    SI
	PUSH    DI
	PUSH    BX

	MOV     SI, [BP+4]       ; SI = format string pointer
	LEA     DI, [BP+6]       ; DI = pointer to first variadic argument

.L_pf_loop:
	LODSB
	TEST    AL, AL
	JZ      .L_pf_done
	CMP     AL, '%'
	JNE     .L_pf_putc

	; Format specifier
	LODSB
	TEST    AL, AL
	JZ      .L_pf_done
	CMP     AL, '%'
	JE      .L_pf_putc
	CMP     AL, 'c'
	JE      .L_pf_char
	CMP     AL, 's'
	JE      .L_pf_str
	CMP     AL, 'd'
	JE      .L_pf_dec
	CMP     AL, 'u'
	JE      .L_pf_udec
	CMP     AL, 'x'
	JE      .L_pf_hex
	CMP     AL, 'X'
	JE      .L_pf_hex

	; Unknown specifier: emit '%' then char
	MOV     BYTE PTR [$FF00], '%'
.L_pf_putc:
	MOV     [$FF00], AL
	JMP     .L_pf_loop

.L_pf_char:
	MOV     AL, [DI]
	ADD     DI, 2
	MOV     [$FF00], AL
	JMP     .L_pf_loop

.L_pf_str:
	MOV     BX, [DI]
	ADD     DI, 2
	TEST    BX, BX
	JZ      .L_pf_loop
.L_pf_str_loop:
	MOV     AL, [BX]
	TEST    AL, AL
	JZ      .L_pf_loop
	MOV     [$FF00], AL
	INC     BX
	JMP     .L_pf_str_loop

.L_pf_dec:
	MOV     AX, [DI]
	ADD     DI, 2
	TEST    AX, AX
	JGE     .L_pf_udec_ax
	NEG     AX
	MOV     BYTE PTR [$FF00], '-'
	JMP     .L_pf_udec_ax

.L_pf_udec:
	MOV     AX, [DI]
	ADD     DI, 2
.L_pf_udec_ax:
	CALL    _print_udec
	JMP     .L_pf_loop

.L_pf_hex:
	MOV     AX, [DI]
	ADD     DI, 2
	CALL    _print_hex16
	JMP     .L_pf_loop

.L_pf_done:
	POP     BX
	POP     DI
	POP     SI
	POP     BP
	RET

; Print unsigned 16-bit integer in AX
_print_udec:
	XOR     CX, CX
	MOV     BX, 10
.L_pud_loop:
	XOR     DX, DX
	DIV     BX
	ADD     DL, '0'
	PUSH    DX
	INC     CX
	TEST    AX, AX
	JNZ     .L_pud_loop
.L_pud_pop:
	POP     AX
	MOV     [$FF00], AL
	LOOP    .L_pud_pop
	RET

; Print 16-bit hex in AX (4 uppercase hex digits)
_print_hex16:
	MOV     BX, AX
	MOV     AL, BH
	CALL    _print_hex_byte
	MOV     AL, BL
	CALL    _print_hex_byte
	RET

_print_hex_byte:
	PUSH    AX
	MOV     CL, 4
	SHR     AL, CL
	CALL    _print_hex_nibble
	POP     AX
	CALL    _print_hex_nibble
	RET

_print_hex_nibble:
	AND     AL, $0F
	CMP     AL, 10
	JB      .L_nib_dec
	ADD     AL, 'A' - 10
	MOV     [$FF00], AL
	RET
.L_nib_dec:
	ADD     AL, '0'
	MOV     [$FF00], AL
	RET
