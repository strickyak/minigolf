; ── C Runtime Startup for MOS 6502 on Hatvan VM (gep5) ──────────────────────

ZP_SP = $00     ; $00..$01: 16-bit Software C Stack Pointer
ZP_FP = $02     ; $02..$03: 16-bit Software Frame Pointer
ZP_R0 = $04     ; $04..$05: Primary 16-bit accumulator / return value
ZP_R1 = $06     ; $06..$07: Secondary 16-bit operand
ZP_RA = $08     ; $08..$09: Source address pointer
ZP_RC = $0A     ; $0A..$0B: Destination / general address pointer
ZP_RD = $0C     ; $0C..$0D: Scratch address / counter pointer
ZP_RE = $0E     ; $0E..$0F: Internal helper scratch word
ZP_RF = $10     ; $10..$11: Internal helper scratch word

	ORG     $0200

cstart:
	SEI
	CLD
	LDX     #$FF
	TXS                 ; Initialize 6502 hardware stack ($0100..$01FF)

	; Initialize software stack pointer ZP_SP = $FEF0 and frame pointer ZP_FP = $FEF0
	LDA     #<$FEF0
	STA     ZP_SP
	STA     ZP_FP
	LDA     #>$FEF0
	STA     ZP_SP+1
	STA     ZP_FP+1

	; Call MiniGolf user main
	JSR     _main

	; Exit with return code in ZP_R0 (low byte)
	LDA     ZP_R0
	STA     $FF05
.L_halt:
	JMP     .L_halt

; ── Stack & Frame Management Helpers ─────────────────────────────────────────

; Allocate Y bytes (2..255) on ZP_SP, save old ZP_FP at (ZP_SP)+0..1, set ZP_FP = ZP_SP
__enter_frame_y:
	STY     ZP_RE
	LDA     #0
	STA     ZP_RE+1
__enter_frame_re:
	SEC
	LDA     ZP_SP
	SBC     ZP_RE
	STA     ZP_SP
	LDA     ZP_SP+1
	SBC     ZP_RE+1
	STA     ZP_SP+1
	LDY     #0
	LDA     ZP_FP
	STA     (ZP_SP),Y
	INY
	LDA     ZP_FP+1
	STA     (ZP_SP),Y
	LDA     ZP_SP
	STA     ZP_FP
	LDA     ZP_SP+1
	STA     ZP_FP+1
	RTS

; Restore ZP_SP = ZP_FP + Y (or ZP_RE), restore ZP_FP from (ZP_FP)+0..1, and RTS
; Preserves ZP_R0 (return value)
__leave_frame_y:
	STY     ZP_RE
	LDA     #0
	STA     ZP_RE+1
__leave_frame_re:
	CLC
	LDA     ZP_FP
	ADC     ZP_RE
	STA     ZP_SP
	LDA     ZP_FP+1
	ADC     ZP_RE+1
	STA     ZP_SP+1
	LDY     #0
	LDA     (ZP_FP),Y
	TAX
	INY
	LDA     (ZP_FP),Y
	STA     ZP_FP+1
	STX     ZP_FP
	RTS

; Push 16-bit ZP_R0 onto ZP_SP
__push_r0:
	SEC
	LDA     ZP_SP
	SBC     #2
	STA     ZP_SP
	BCS     .L_push_r0_nb
	DEC     ZP_SP+1
.L_push_r0_nb:
	LDY     #0
	LDA     ZP_R0
	STA     (ZP_SP),Y
	INY
	LDA     ZP_R0+1
	STA     (ZP_SP),Y
	RTS

; Push 16-bit ZP_RC onto ZP_SP
__push_rc:
	SEC
	LDA     ZP_SP
	SBC     #2
	STA     ZP_SP
	BCS     .L_push_rc_nb
	DEC     ZP_SP+1
.L_push_rc_nb:
	LDY     #0
	LDA     ZP_RC
	STA     (ZP_SP),Y
	INY
	LDA     ZP_RC+1
	STA     (ZP_SP),Y
	RTS

; Subtract Y bytes (1..255) from ZP_SP and copy new ZP_SP into ZP_RC
__sub_sp_y:
	STY     ZP_RE
	SEC
	LDA     ZP_SP
	SBC     ZP_RE
	STA     ZP_SP
	STA     ZP_RC
	BCS     .L_sub_sp_nb
	DEC     ZP_SP+1
.L_sub_sp_nb:
	LDA     ZP_SP+1
	STA     ZP_RC+1
	RTS

; Add Y bytes (1..255) to ZP_SP (caller argument cleanup)
__add_sp_y:
	TYA
	CLC
	ADC     ZP_SP
	STA     ZP_SP
	BCC     .L_add_sp_nc
	INC     ZP_SP+1
.L_add_sp_nc:
	RTS

; Indirect function call via ZP_RC: JSR __call_rc
__call_rc:
	JMP     (ZP_RC)

; Load 2 bytes at (ZP_FP),Y into ZP_R0 / ZP_R1 / ZP_RA / ZP_RC
__load_r0_y:
	LDA     (ZP_FP),Y
	STA     ZP_R0
	INY
	LDA     (ZP_FP),Y
	STA     ZP_R0+1
	RTS

__load_r0_b_y:
	LDA     (ZP_FP),Y
	STA     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	RTS

__load_r1_y:
	LDA     (ZP_FP),Y
	STA     ZP_R1
	INY
	LDA     (ZP_FP),Y
	STA     ZP_R1+1
	RTS

__load_r1_b_y:
	LDA     (ZP_FP),Y
	STA     ZP_R1
	LDA     #0
	STA     ZP_R1+1
	RTS

__load_ra_y:
	LDA     (ZP_FP),Y
	STA     ZP_RA
	INY
	LDA     (ZP_FP),Y
	STA     ZP_RA+1
	RTS

__load_rc_y:
	LDA     (ZP_FP),Y
	STA     ZP_RC
	INY
	LDA     (ZP_FP),Y
	STA     ZP_RC+1
	RTS

; Store 2 bytes from ZP_R0 into (ZP_FP),Y and copy ZP_R0 to ZP_RC
__store_r0_y:
	LDA     ZP_R0
	STA     (ZP_FP),Y
	STA     ZP_RC
	INY
	LDA     ZP_R0+1
	STA     (ZP_FP),Y
	STA     ZP_RC+1
	RTS

; Store 1 byte from ZP_R0 into (ZP_FP),Y and zero-extend ZP_R0
__store_r0_b_y:
	LDA     ZP_R0
	STA     (ZP_FP),Y
	LDA     #0
	STA     ZP_R0+1
	RTS

; Load/Store 2 bytes via (ZP_RA) or (ZP_RC) or (ZP_RD)
__load_r0_rc:
	LDY     #0
	LDA     (ZP_RC),Y
	STA     ZP_R0
	INY
	LDA     (ZP_RC),Y
	STA     ZP_R0+1
	RTS

__load_r0_ra:
	LDY     #0
	LDA     (ZP_RA),Y
	STA     ZP_R0
	INY
	LDA     (ZP_RA),Y
	STA     ZP_R0+1
	RTS

__store_r0_rc:
	LDY     #0
	LDA     ZP_R0
	STA     (ZP_RC),Y
	INY
	LDA     ZP_R0+1
	STA     (ZP_RC),Y
	RTS

__store_r0_rd:
	LDY     #0
	LDA     ZP_R0
	STA     (ZP_RD),Y
	INY
	LDA     ZP_R0+1
	STA     (ZP_RD),Y
	RTS

; Load small unsigned constant Y (0..255) into ZP_R0 / ZP_R1
__zero_r0:
	LDA     #0
	STA     ZP_R0
	STA     ZP_R0+1
	RTS

__const_r0_y:
	STY     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	RTS

__const_r1_y:
	STY     ZP_R1
	LDA     #0
	STA     ZP_R1+1
	RTS

; Add Y (1..255) to 16-bit ZP_R0 / ZP_RA / ZP_RC
__add_r0_y:
	TYA
	CLC
	ADC     ZP_R0
	STA     ZP_R0
	BCC     .L_add_r0_nc
	INC     ZP_R0+1
.L_add_r0_nc:
	RTS

__add_ra_y:
	TYA
	CLC
	ADC     ZP_RA
	STA     ZP_RA
	BCC     .L_add_ra_nc
	INC     ZP_RA+1
.L_add_ra_nc:
	RTS

__add_rc_y:
	TYA
	CLC
	ADC     ZP_RC
	STA     ZP_RC
	BCC     .L_add_rc_nc
	INC     ZP_RC+1
.L_add_rc_nc:
	RTS

; Compute ZP_Rx = ZP_FP + Y (0..255) or ZP_FP + (A:Y)
__addr_r0_y:
	LDA     #0
__addr_r0_ay:
	STA     ZP_R0+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_R0
	LDA     ZP_FP+1
	ADC     ZP_R0+1
	STA     ZP_R0+1
	RTS

__addr_r1_y:
	LDA     #0
__addr_r1_ay:
	STA     ZP_R1+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_R1
	LDA     ZP_FP+1
	ADC     ZP_R1+1
	STA     ZP_R1+1
	RTS

__addr_ra_y:
	LDA     #0
__addr_ra_ay:
	STA     ZP_RA+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_RA
	LDA     ZP_FP+1
	ADC     ZP_RA+1
	STA     ZP_RA+1
	RTS

__addr_rc_y:
	LDA     #0
__addr_rc_ay:
	STA     ZP_RC+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_RC
	LDA     ZP_FP+1
	ADC     ZP_RC+1
	STA     ZP_RC+1
	RTS

__addr_rd_y:
	LDA     #0
__addr_rd_ay:
	STA     ZP_RD+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_RD
	LDA     ZP_FP+1
	ADC     ZP_RD+1
	STA     ZP_RD+1
	RTS

__addr_re_ay:
	STA     ZP_RE+1
	TYA
	CLC
	ADC     ZP_FP
	STA     ZP_RE
	LDA     ZP_FP+1
	ADC     ZP_RE+1
	STA     ZP_RE+1
	LDY     #0
	RTS

__load_r0_ay:
	JSR     __addr_re_ay
	LDA     (ZP_RE),Y
	STA     ZP_R0
	INY
	LDA     (ZP_RE),Y
	STA     ZP_R0+1
	RTS

__load_r0_b_ay:
	JSR     __addr_re_ay
	LDA     (ZP_RE),Y
	STA     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	RTS

__load_r1_ay:
	JSR     __addr_re_ay
	LDA     (ZP_RE),Y
	STA     ZP_R1
	INY
	LDA     (ZP_RE),Y
	STA     ZP_R1+1
	RTS

__load_ra_ay:
	JSR     __addr_re_ay
	LDA     (ZP_RE),Y
	STA     ZP_RA
	INY
	LDA     (ZP_RE),Y
	STA     ZP_RA+1
	RTS

__load_rc_ay:
	JSR     __addr_re_ay
	LDA     (ZP_RE),Y
	STA     ZP_RC
	INY
	LDA     (ZP_RE),Y
	STA     ZP_RC+1
	RTS

__store_r0_ay:
	JSR     __addr_re_ay
	LDA     ZP_R0
	STA     (ZP_RE),Y
	STA     ZP_RC
	INY
	LDA     ZP_R0+1
	STA     (ZP_RE),Y
	STA     ZP_RC+1
	RTS

__store_r0_b_ay:
	JSR     __addr_re_ay
	LDA     ZP_R0
	STA     (ZP_RE),Y
	LDA     #0
	STA     ZP_R0+1
	RTS

; ── Memory Copy & Clear Helpers ──────────────────────────────────────────────

; Copy Y bytes (1..255) from (ZP_RA) to (ZP_RC)
__memcpy_y:
	DEY
.L_mcpy_y_loop:
	LDA     (ZP_RA),Y
	STA     (ZP_RC),Y
	DEY
	CPY     #$FF
	BNE     .L_mcpy_y_loop
	RTS

; Copy ZP_RD bytes (16-bit count) from (ZP_RA) to (ZP_RC)
__memcpy_rd:
	LDY     #0
.L_mcpy_rd_loop:
	LDA     ZP_RD
	ORA     ZP_RD+1
	BEQ     .L_mcpy_rd_done
	LDA     (ZP_RA),Y
	STA     (ZP_RC),Y
	INC     ZP_RA
	BNE     .L_mcpy_rd_1
	INC     ZP_RA+1
.L_mcpy_rd_1:
	INC     ZP_RC
	BNE     .L_mcpy_rd_2
	INC     ZP_RC+1
.L_mcpy_rd_2:
	LDA     ZP_RD
	BNE     .L_mcpy_rd_3
	DEC     ZP_RD+1
.L_mcpy_rd_3:
	DEC     ZP_RD
	JMP     .L_mcpy_rd_loop
.L_mcpy_rd_done:
	RTS

; Clear Y bytes (1..255) at (ZP_RC) to 0
__memclear_y:
	LDA     #0
	DEY
.L_mclr_y_loop:
	STA     (ZP_RC),Y
	DEY
	CPY     #$FF
	BNE     .L_mclr_y_loop
	RTS

; Clear ZP_RD bytes (16-bit count) at (ZP_RC) to 0
__memclear_rd:
	LDY     #0
.L_mclr_rd_loop:
	LDA     ZP_RD
	ORA     ZP_RD+1
	BEQ     .L_mclr_rd_done
	LDA     #0
	STA     (ZP_RC),Y
	INC     ZP_RC
	BNE     .L_mclr_rd_1
	INC     ZP_RC+1
.L_mclr_rd_1:
	LDA     ZP_RD
	BNE     .L_mclr_rd_2
	DEC     ZP_RD+1
.L_mclr_rd_2:
	DEC     ZP_RD
	JMP     .L_mclr_rd_loop
.L_mclr_rd_done:
	RTS

; ── Exit System Call ─────────────────────────────────────────────────────────
_exit:
__exit:
	LDY     #0
	LDA     (ZP_SP),Y
	STA     $FF05
.L_exit_halt:
	JMP     .L_exit_halt

; ── Console Character I/O ───────────────────────────────────────────────────
_putchar:
putchar:
	LDY     #0
	LDA     (ZP_SP),Y
	STA     $FF00
	RTS

__putc:
	STA     $FF00
	RTS

_getchar:
getchar:
	LDA     $FF01
	STA     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	RTS

f_prelude__mul_byte:
	LDY     #0
	LDA     (ZP_SP),Y
	STA     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	LDY     #2
	LDA     (ZP_SP),Y
	STA     ZP_R1
	LDA     #0
	STA     ZP_R1+1
	JMP     __mul16

; ── Minimal _printf Implementation for 6502 ──────────────────────────────────
_printf:
	; Load format string pointer from (ZP_SP)+0..1 into ZP_RA
	LDY     #0
	LDA     (ZP_SP),Y
	STA     ZP_RA
	INY
	LDA     (ZP_SP),Y
	STA     ZP_RA+1

	; Set ZP_RD = ZP_SP + 2 (points to first vararg)
	CLC
	LDA     ZP_SP
	ADC     #2
	STA     ZP_RD
	LDA     ZP_SP+1
	ADC     #0
	STA     ZP_RD+1

.L_pf_loop:
	LDY     #0
	LDA     (ZP_RA),Y
	BEQ     .L_pf_done
	INC     ZP_RA
	BNE     .L_pf_adv1
	INC     ZP_RA+1
.L_pf_adv1:
	CMP     #'%'
	BNE     .L_pf_putc

	; Fetch specifier char
	LDA     (ZP_RA),Y
	BEQ     .L_pf_done
	INC     ZP_RA
	BNE     .L_pf_adv2
	INC     ZP_RA+1
.L_pf_adv2:
	CMP     #'%'
	BEQ     .L_pf_putc
	CMP     #'c'
	BEQ     .L_pf_char
	CMP     #'s'
	BEQ     .L_pf_str
	CMP     #'d'
	BEQ     .L_pf_dec
	CMP     #'u'
	BEQ     .L_pf_udec
	CMP     #'x'
	BEQ     .L_pf_hex
	CMP     #'X'
	BEQ     .L_pf_hex

	; Unknown specifier: emit '%' and char
	PHA
	LDA     #'%'
	STA     $FF00
	PLA
.L_pf_putc:
	STA     $FF00
	JMP     .L_pf_loop

.L_pf_done:
	RTS

; Helper: fetch next 16-bit vararg from (ZP_RD) into ZP_R0 and advance ZP_RD by 2
__pf_next_arg:
	LDY     #0
	LDA     (ZP_RD),Y
	STA     ZP_R0
	INY
	LDA     (ZP_RD),Y
	STA     ZP_R0+1
	CLC
	LDA     ZP_RD
	ADC     #2
	STA     ZP_RD
	BCC     .L_pf_na_done
	INC     ZP_RD+1
.L_pf_na_done:
	RTS

.L_pf_char:
	JSR     __pf_next_arg
	LDA     ZP_R0
	STA     $FF00
	JMP     .L_pf_loop

.L_pf_str:
	JSR     __pf_next_arg
	LDY     #0
.L_pf_str_loop:
	LDA     (ZP_R0),Y
	BEQ     .L_pf_loop
	STA     $FF00
	INC     ZP_R0
	BNE     .L_pf_str_loop
	INC     ZP_R0+1
	JMP     .L_pf_str_loop

.L_pf_dec:
	JSR     __pf_next_arg
	LDA     ZP_R0+1
	BPL     .L_pf_dec_pos
	LDA     #'-'
	STA     $FF00
	JSR     __neg_r0
.L_pf_dec_pos:
	JSR     _print_udec
	JMP     .L_pf_loop

.L_pf_udec:
	JSR     __pf_next_arg
	JSR     _print_udec
	JMP     .L_pf_loop

.L_pf_hex:
	JSR     __pf_next_arg
	LDA     ZP_R0+1
	JSR     _print_hex_byte
	LDA     ZP_R0
	JSR     _print_hex_byte
	JMP     .L_pf_loop

; Print unsigned 16-bit decimal in ZP_R0 (preserves ZP_RA, ZP_RD)
_print_udec:
	LDA     #0
	PHA
.L_pud_div_loop:
	LDA     #10
	STA     ZP_R1
	LDA     #0
	STA     ZP_R1+1
	JSR     __udivmod16
	LDA     ZP_R1
	CLC
	ADC     #'0'
	PHA
	LDA     ZP_R0
	ORA     ZP_R0+1
	BNE     .L_pud_div_loop
.L_pud_print_loop:
	PLA
	BEQ     .L_pud_done
	STA     $FF00
	JMP     .L_pud_print_loop
.L_pud_done:
	RTS

_print_hex_byte:
	PHA
	LSR     A
	LSR     A
	LSR     A
	LSR     A
	JSR     _print_hex_nibble
	PLA
	AND     #$0F
	JMP     _print_hex_nibble

_print_hex_nibble:
	AND     #$0F
	CMP     #10
	BCC     .L_phn_dec
	CLC
	ADC     #'A' - 10
	STA     $FF00
	RTS
.L_phn_dec:
	CLC
	ADC     #'0'
	STA     $FF00
	RTS

; ── 16-Bit Math Helpers ──────────────────────────────────────────────────────

; __neg_r0: ZP_R0 = -ZP_R0
__neg_r0:
	SEC
	LDA     #0
	SBC     ZP_R0
	STA     ZP_R0
	LDA     #0
	SBC     ZP_R0+1
	STA     ZP_R0+1
	RTS

; __neg_r1: ZP_R1 = -ZP_R1
__neg_r1:
	SEC
	LDA     #0
	SBC     ZP_R1
	STA     ZP_R1
	LDA     #0
	SBC     ZP_R1+1
	STA     ZP_R1+1
	RTS

; __add16: ZP_R0 = ZP_R0 + ZP_R1
__add16:
	CLC
	LDA     ZP_R0
	ADC     ZP_R1
	STA     ZP_R0
	LDA     ZP_R0+1
	ADC     ZP_R1+1
	STA     ZP_R0+1
	RTS

; __sub16: ZP_R0 = ZP_R0 - ZP_R1
__sub16:
	SEC
	LDA     ZP_R0
	SBC     ZP_R1
	STA     ZP_R0
	LDA     ZP_R0+1
	SBC     ZP_R1+1
	STA     ZP_R0+1
	RTS

; __and16: ZP_R0 = ZP_R0 & ZP_R1
__and16:
	LDA     ZP_R0
	AND     ZP_R1
	STA     ZP_R0
	LDA     ZP_R0+1
	AND     ZP_R1+1
	STA     ZP_R0+1
	RTS

; __or16: ZP_R0 = ZP_R0 | ZP_R1
__or16:
	LDA     ZP_R0
	ORA     ZP_R1
	STA     ZP_R0
	LDA     ZP_R0+1
	ORA     ZP_R1+1
	STA     ZP_R0+1
	RTS

; __xor16: ZP_R0 = ZP_R0 ^ ZP_R1
__xor16:
	LDA     ZP_R0
	EOR     ZP_R1
	STA     ZP_R0
	LDA     ZP_R0+1
	EOR     ZP_R1+1
	STA     ZP_R0+1
	RTS

; __shl16: ZP_R0 = ZP_R0 << ZP_R1.0
__shl16:
	LDX     ZP_R1
	BEQ     .L_shl_done
.L_shl_loop:
	ASL     ZP_R0
	ROL     ZP_R0+1
	DEX
	BNE     .L_shl_loop
.L_shl_done:
	RTS

; __shr16: ZP_R0 = ZP_R0 >> ZP_R1.0 (unsigned)
__shr16:
	LDX     ZP_R1
	BEQ     .L_shr_done
.L_shr_loop:
	LSR     ZP_R0+1
	ROR     ZP_R0
	DEX
	BNE     .L_shr_loop
.L_shr_done:
	RTS

; __sar16: ZP_R0 = ZP_R0 >> ZP_R1.0 (signed arithmetic)
__sar16:
	LDX     ZP_R1
	BEQ     .L_sar_done
.L_sar_loop:
	LDA     ZP_R0+1
	ASL     A
	ROR     ZP_R0+1
	ROR     ZP_R0
	DEX
	BNE     .L_sar_loop
.L_sar_done:
	RTS

; __mul16: ZP_R0 = ZP_R0 * ZP_R1
__mul16:
	LDA     #0
	STA     ZP_RE
	STA     ZP_RE+1
	LDX     #16
.L_mul_loop:
	LSR     ZP_R1+1
	ROR     ZP_R1
	BCC     .L_mul_noadd
	CLC
	LDA     ZP_RE
	ADC     ZP_R0
	STA     ZP_RE
	LDA     ZP_RE+1
	ADC     ZP_R0+1
	STA     ZP_RE+1
.L_mul_noadd:
	ASL     ZP_R0
	ROL     ZP_R0+1
	DEX
	BNE     .L_mul_loop
	LDA     ZP_RE
	STA     ZP_R0
	LDA     ZP_RE+1
	STA     ZP_R0+1
	RTS

; __udivmod16: ZP_R0 = ZP_R0 / ZP_R1 (quotient), ZP_R1 = ZP_R0 % ZP_R1 (remainder)
__udivmod16:
	LDA     ZP_R1
	ORA     ZP_R1+1
	BNE     .L_udiv_ok
	LDA     #<.L_div_zero_str
	STA     ZP_R0
	LDA     #>.L_div_zero_str
	STA     ZP_R0+1
	JSR     __push_r0
	JMP     _panic
.L_udiv_ok:
	LDA     #0
	STA     ZP_RE
	STA     ZP_RE+1
	LDX     #16
.L_udiv_loop:
	ASL     ZP_R0
	ROL     ZP_R0+1
	ROL     ZP_RE
	ROL     ZP_RE+1
	SEC
	LDA     ZP_RE
	SBC     ZP_R1
	TAY
	LDA     ZP_RE+1
	SBC     ZP_R1+1
	BCC     .L_udiv_nosub
	STA     ZP_RE+1
	STY     ZP_RE
	INC     ZP_R0
.L_udiv_nosub:
	DEX
	BNE     .L_udiv_loop
	LDA     ZP_RE
	STA     ZP_R1
	LDA     ZP_RE+1
	STA     ZP_R1+1
	RTS

__udiv16:
	JMP     __udivmod16

__umod16:
	JSR     __udivmod16
	LDA     ZP_R1
	STA     ZP_R0
	LDA     ZP_R1+1
	STA     ZP_R0+1
	RTS

__sdiv16:
	JMP     __sdivmod16

__smod16:
	JSR     __sdivmod16
	LDA     ZP_R1
	STA     ZP_R0
	LDA     ZP_R1+1
	STA     ZP_R0+1
	RTS

__sdivmod16:
	LDA     #0
	STA     ZP_RF       ; quotient sign toggle
	STA     ZP_RF+1     ; remainder sign
	LDA     ZP_R0+1
	BPL     .L_sdiv_chk_d
	JSR     __neg_r0
	LDA     #1
	STA     ZP_RF
	STA     ZP_RF+1
.L_sdiv_chk_d:
	LDA     ZP_R1+1
	BPL     .L_sdiv_do
	JSR     __neg_r1
	LDA     ZP_RF
	EOR     #1
	STA     ZP_RF
.L_sdiv_do:
	JSR     __udivmod16
	LDA     ZP_RF
	BEQ     .L_sdiv_chk_rem
	JSR     __neg_r0
.L_sdiv_chk_rem:
	LDA     ZP_RF+1
	BEQ     .L_sdiv_done
	JSR     __neg_r1
.L_sdiv_done:
	RTS

; ── 16-Bit Comparison Helpers ────────────────────────────────────────────────

__cmpeq16:
	LDA     ZP_R0
	CMP     ZP_R1
	BNE     .L_cmp_false
	LDA     ZP_R0+1
	CMP     ZP_R1+1
	BNE     .L_cmp_false
.L_cmp_true:
	LDA     #1
	STA     ZP_R0
	LDA     #0
	STA     ZP_R0+1
	LDA     #1
	RTS
.L_cmp_false:
	LDA     #0
	STA     ZP_R0
	STA     ZP_R0+1
	RTS

__cmpne16:
	LDA     ZP_R0
	CMP     ZP_R1
	BNE     .L_cmp_true
	LDA     ZP_R0+1
	CMP     ZP_R1+1
	BNE     .L_cmp_true
	JMP     .L_cmp_false

__cmplt16_u:
	LDA     ZP_R0+1
	CMP     ZP_R1+1
	BCC     .L_cmp_true
	BNE     .L_cmp_false
	LDA     ZP_R0
	CMP     ZP_R1
	BCC     .L_cmp_true
	JMP     .L_cmp_false

__cmpgte16_u:
	LDA     ZP_R0+1
	CMP     ZP_R1+1
	BCC     .L_cmp_false
	BNE     .L_cmp_true
	LDA     ZP_R0
	CMP     ZP_R1
	BCS     .L_cmp_true
	JMP     .L_cmp_false

__cmpgt16_u:
	LDA     ZP_R1+1
	CMP     ZP_R0+1
	BCC     .L_cmp_true
	BNE     .L_cmp_false
	LDA     ZP_R1
	CMP     ZP_R0
	BCC     .L_cmp_true
	JMP     .L_cmp_false

__cmplte16_u:
	LDA     ZP_R1+1
	CMP     ZP_R0+1
	BCC     .L_cmp_false
	BNE     .L_cmp_true
	LDA     ZP_R1
	CMP     ZP_R0
	BCS     .L_cmp_true
	JMP     .L_cmp_false

__cmplt16_s:
	LDA     ZP_R0+1
	EOR     ZP_R1+1
	BMI     .L_cmplt_s_diff
	JMP     __cmplt16_u
.L_cmplt_s_diff:
	LDA     ZP_R0+1
	BMI     .L_cmp_true
	JMP     .L_cmp_false

__cmpgte16_s:
	LDA     ZP_R0+1
	EOR     ZP_R1+1
	BMI     .L_cmpgte_s_diff
	JMP     __cmpgte16_u
.L_cmpgte_s_diff:
	LDA     ZP_R0+1
	BPL     .L_cmp_true
	JMP     .L_cmp_false

__cmpgt16_s:
	LDA     ZP_R1+1
	EOR     ZP_R0+1
	BMI     .L_cmpgt_s_diff
	JMP     __cmpgt16_u
.L_cmpgt_s_diff:
	LDA     ZP_R1+1
	BMI     .L_cmp_true
	JMP     .L_cmp_false

__cmplte16_s:
	LDA     ZP_R1+1
	EOR     ZP_R0+1
	BMI     .L_cmplte_s_diff
	JMP     __cmplte16_u
.L_cmplte_s_diff:
	LDA     ZP_R1+1
	BPL     .L_cmp_true
	JMP     .L_cmp_false

; ── Panic & LongJmp Runtime ──────────────────────────────────────────────────

_panic:
__panic:
	; Fetch panic message pointer from (ZP_SP)+0..1 into ZP_R0
	LDY     #0
	LDA     (ZP_SP),Y
	STA     ZP_R0
	STA     v_prelude._panic_
	INY
	LDA     (ZP_SP),Y
	STA     ZP_R0+1
	STA     v_prelude._panic_+1

	; Push panic message pointer
	JSR     __push_r0

	; Push format string .L_panic_fmt
	LDA     #<.L_panic_fmt
	STA     ZP_R0
	LDA     #>.L_panic_fmt
	STA     ZP_R0+1
	JSR     __push_r0

	JSR     _printf
	LDY     #4
	JSR     __add_sp_y

__longjmp_chain:
	LDA     v_prelude._jmp_chain_
	STA     ZP_RC
	LDA     v_prelude._jmp_chain_+1
	STA     ZP_RC+1
	ORA     ZP_RC
	BNE     __longjmp_rc

	; Empty recover chain -> abort with message and exit(1)
	LDA     #<.L_empty_chain_msg
	STA     ZP_R0
	LDA     #>.L_empty_chain_msg
	STA     ZP_R0+1
	JSR     __push_r0
	JSR     _printf
	LDA     #1
	STA     $FF05
.L_panic_halt:
	JMP     .L_panic_halt

__longjmp_rc:
	LDY     #2
	LDA     (ZP_RC),Y
	STA     ZP_RD
	INY
	LDA     (ZP_RC),Y
	STA     ZP_RD+1
	INY
	LDA     (ZP_RC),Y
	STA     ZP_SP
	INY
	LDA     (ZP_RC),Y
	STA     ZP_SP+1
	INY
	LDA     (ZP_RC),Y
	STA     ZP_FP
	INY
	LDA     (ZP_RC),Y
	STA     ZP_FP+1
	INY
	LDA     (ZP_RC),Y
	TAX
	TXS
	JMP     (ZP_RD)

.L_div_zero_str:
	ASCIZ   "division by zero"
.L_panic_fmt:
	ASCIZ   "\n*PANIC* %s\n"
.L_empty_chain_msg:
	ASCIZ   "\n*** ABORT\n\n*** EMPTY_RE_CHAIN\n"
