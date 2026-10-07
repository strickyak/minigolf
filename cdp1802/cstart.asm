; ── C Runtime Startup for RCA COSMAC 1802 on Hatvan VM (gepc) ───────────────
	org     $0100

cstart:
	; P=0 on hardware reset. Switch P to R3 for main program execution:
	LOAD    R3, _start3
	SEP     R3

_start3:
	; Now running with P = 3 (R3 is PC).
	; Set stack pointer R2 to top of user RAM (below $FF00 page)
	LOAD    R2, $EFF0
	SEX     R2

	; Initialize console output port pointer RF = $FF00
	LOAD    RF, $FF00

	; Initialize Frame Pointer RB = R2
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	; Initialize SCRT Call engine R4
	LOAD    R4, _CALL

	; Initialize SCRT Return engine R5
	LOAD    R5, _RET

	; Initialize Hatvan Trap pointer RE ($0004)
	LOAD    RE, $0004

	; Call MiniGolf user main via SCRT
	SEP     R4
	DW      _main

	; Exit with return code in R7.0
	LOAD    RC, $FF05
	GLO     R7
	STR     RC
	SEP     RE
	DB      $06
.L_halt:
	IDL
	BR      .L_halt

; ── SCRT Call and Return Engines ─────────────────────────────────────────────
_CALL:
	BR      _CALL_LOOP

_CALL_LOOP:
	; Push caller's return address (R6) onto stack R2
	GLO     R6
	STXD
	GHI     R6
	STXD

	; Fetch inline target function address from caller PC (R3)
	LDA     R3
	PHI     RA          ; Target high byte
	LDA     R3
	PLO     RA          ; Target low byte

	; R3 now points past the .DW target to caller's return address -> copy to R6
	GHI     R3
	PHI     R6
	GLO     R3
	PLO     R6

	; Copy target function address (RA) to R3
	GHI     RA
	PHI     R3
	GLO     RA
	PLO     R3

	; Transfer control to target function with P = 3
	SEP     R3
	BR      _CALL_LOOP

_RET:
	BR      _RET_LOOP

_RET_LOOP:
	; Copy return address (R6) into R3
	GHI     R6
	PHI     R3
	GLO     R6
	PLO     R3

	; Pop caller's old R6 from stack R2
	INC     R2
	LDA     R2
	PHI     R6
	LDN     R2
	PLO     R6

	; Transfer control back to caller with P = 3
	SEP     R3
	BR      _RET_LOOP

; Indirect function call helper: transfers to address in RC
__call_rc:
	LOAD    RA, .L_call_rc_trampoline
	SEP     RA
.L_call_rc_trampoline:
	GHI     RC
	PHI     R3
	GLO     RC
	PLO     R3
	SEP     R3

; ── Exit System Call ─────────────────────────────────────────────────────────
_exit:
__exit:
	; Set up frame to access exit code at RB+6
	GLO     RB
	STXD
	GHI     RB
	STXD
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	GLO     RB
	ADI     6
	PLO     RA
	GHI     RB
	ADCI    0
	PHI     RA

	; Memory-mapped exit code port ($FF05) halts gepc / Hatvan VM
	LOAD    RC, $FF05
	LDN     RA          ; D = exit code byte
	STR     RC

	; Also trigger Hatvan OS kernel trap if running under OS
	SEP     RE
	DB      $06
.L_exit_halt:
	IDL
	BR      .L_exit_halt

; ── Console Character I/O ───────────────────────────────────────────────────
_putchar:
putchar:
	GLO     RB
	STXD
	GHI     RB
	STXD
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	GLO     RB
	ADI     6
	PLO     RC
	GHI     RB
	ADCI    0
	PHI     RC
	LDN     RC          ; D = char byte
	STR     RF          ; Output to $FF00 (console output port)

	GHI     RB
	PHI     R2
	GLO     RB
	PLO     R2
	INC     R2
	LDA     R2
	PHI     RB
	LDN     R2
	PLO     RB
	SEP     R5

__putc:
	STR     RF          ; Output char in D to $FF00
	SEP     R5

_getchar:
getchar:
	; Read char from console into D and M(R2)
	DEC     R2
	INP     1
	LDN     R2
	PLO     R7
	LDI     0
	PHI     R7
	INC     R2
	SEP     R5

f_prelude__mul_byte:
	GLO     RB
	STXD
	GHI     RB
	STXD
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	; Load a from RB+5..6 into R7
	GLO     RB
	ADI     5
	PLO     RC
	GHI     RB
	ADCI    0
	PHI     RC
	LDA     RC
	PHI     R7
	LDN     RC
	PLO     R7

	; Load b from RB+7..8 into R8
	GLO     RB
	ADI     7
	PLO     RC
	GHI     RB
	ADCI    0
	PHI     RC
	LDA     RC
	PHI     R8
	LDN     RC
	PLO     R8

	SEP     R4
	DW      __mul16

	GHI     RB
	PHI     R2
	GLO     RB
	PLO     R2
	INC     R2
	LDA     R2
	PHI     RB
	LDN     R2
	PLO     RB
	SEP     R5

; ── Minimal _printf Implementation for 1802 ──────────────────────────────────
_printf:
	; Function prologue: save RB, set RB = R2
	GLO     RB
	STXD
	GHI     RB
	STXD
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	; RD points directly to the first variable argument at RB + 7
	GLO     RB
	ADI     7
	PLO     RD
	GHI     RB
	ADCI    0
	PHI     RD

	; Load format string pointer from RB+5 into RA
	GLO     RB
	ADI     5
	PLO     RC
	GHI     RB
	ADCI    0
	PHI     RC
	LDA     RC
	PHI     RA
	LDN     RC
	PLO     RA

.L_pf_loop:
	LDA     RA          ; D = *format++
	BZ      .L_pf_done
	SMI     '%'
	BNZ     .L_pf_not_spec

	; Specifier character
	LDA     RA          ; D = specifier
	BZ      .L_pf_done
	SMI     '%'
	BZ      .L_pf_put_pct
	SMI     'c' - '%'
	BZ      .L_pf_char
	SMI     's' - 'c'
	BZ      .L_pf_str
	SMI     'd' - 's'
	BZ      .L_pf_dec
	SMI     'u' - 'd'
	BZ      .L_pf_udec
	SMI     'x' - 'u'
	BZ      .L_pf_hex
	SMI     'X' - 'x'
	BZ      .L_pf_hex

	; Unknown specifier: emit '%' and char
	LDI     '%'
	STR     RF
	DEC     RA
	LDA     RA
	STR     RF
	BR      .L_pf_loop

.L_pf_put_pct:
	LDI     '%'
	STR     RF
	BR      .L_pf_loop

.L_pf_not_spec:
	ADI     '%'         ; restore character
	STR     RF
	BR      .L_pf_loop

.L_pf_char:
	LDA     RD          ; skip high byte
	LDA     RD          ; D = char byte
	STR     RF
	BR      .L_pf_loop

.L_pf_str:
	LDA     RD
	PHI     R7
	LDA     RD
	PLO     R7
.L_pf_str_loop:
	LDA     R7
	BZ      .L_pf_loop
	STR     RF
	BR      .L_pf_str_loop

.L_pf_dec:
	LDA     RD
	PHI     R7
	LDA     RD
	PLO     R7
	; Check sign bit of R7.1
	GHI     R7
	SHL
	BNF     .L_pf_dec_pos
	; Negative: print '-' and negate R7
	LDI     '-'
	STR     RF
	GLO     R7
	SDI     0
	PLO     R7
	GHI     R7
	SDBI    0
	PHI     R7
.L_pf_dec_pos:
	; Push RA and RD
	GLO     RA
	STXD
	GHI     RA
	STXD
	GLO     RD
	STXD
	GHI     RD
	STXD
	SEP     R4
	DW      _print_udec
	INC     R2
	LDA     R2
	PHI     RD
	LDN     R2
	PLO     RD
	INC     R2
	LDA     R2
	PHI     RA
	LDN     R2
	PLO     RA
	BR      .L_pf_loop

.L_pf_udec:
	LDA     RD
	PHI     R7
	LDA     RD
	PLO     R7
	; Push RA and RD
	GLO     RA
	STXD
	GHI     RA
	STXD
	GLO     RD
	STXD
	GHI     RD
	STXD
	SEP     R4
	DW      _print_udec
	INC     R2
	LDA     R2
	PHI     RD
	LDN     R2
	PLO     RD
	INC     R2
	LDA     R2
	PHI     RA
	LDN     R2
	PLO     RA
	BR      .L_pf_loop

.L_pf_hex:
	LDA     RD
	PHI     R7
	LDA     RD
	PLO     R7
	GLO     RA
	STXD
	GHI     RA
	STXD
	GLO     RD
	STXD
	GHI     RD
	STXD
	GHI     R7
	SEP     R4
	DW      _print_hex_byte
	GLO     R7
	SEP     R4
	DW      _print_hex_byte
	INC     R2
	LDA     R2
	PHI     RD
	LDN     R2
	PLO     RD
	INC     R2
	LDA     R2
	PHI     RA
	LDN     R2
	PLO     RA
	BR      .L_pf_loop

.L_pf_done:
	GHI     RB
	PHI     R2
	GLO     RB
	PLO     R2
	INC     R2
	LDA     R2
	PHI     RB
	LDN     R2
	PLO     RB
	SEP     R5

; ── Print Unsigned Decimal in R7 ─────────────────────────────────────────────
_print_udec:
	; Push 0 marker onto stack
	LDI     0
	STXD
.L_pud_div_loop:
	LOAD    R8, 10
	SEP     R4
	DW      __udivmod16     ; Quotient in R7, remainder in R8
	GLO     R8
	ADI     '0'
	STXD                    ; Push digit character
	GLO     R7
	BNZ     .L_pud_div_loop
	GHI     R7
	BNZ     .L_pud_div_loop
.L_pud_print_loop:
	INC     R2
	LDN     R2
	BZ      .L_pud_done
	STR     RF
	BR      .L_pud_print_loop
.L_pud_done:
	SEP     R5

; ── Print Hex Byte in D ──────────────────────────────────────────────────────
_print_hex_byte:
	PLO     RC
	; High nibble
	GLO     RC
	SHR
	SHR
	SHR
	SHR
	SEP     R4
	DW      _print_hex_nibble
	; Low nibble
	GLO     RC
	ANI     $0F
	SEP     R4
	DW      _print_hex_nibble
	SEP     R5

_print_hex_nibble:
	ANI     $0F
	SMI     10
	BNF     .L_phn_dec
	ADI     'A'
	STR     RF
	SEP     R5
.L_phn_dec:
	ADI     10 + '0'
	STR     RF
	SEP     R5

; ── 16-bit Math Helpers ──────────────────────────────────────────────────────
; __add16: R7 = R7 + R8
__add16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	ADD
	PLO     R7
	DEC     R2
	GHI     R7
	ADC
	PHI     R7
	INC     R2
	SEP     R5

; __sub16: R7 = R7 - R8
__sub16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	SM
	PLO     R7
	DEC     R2
	GHI     R7
	SMB
	PHI     R7
	INC     R2
	SEP     R5

; __and16: R7 = R7 & R8
__and16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	AND
	PLO     R7
	DEC     R2
	GHI     R7
	AND
	PHI     R7
	INC     R2
	SEP     R5

; __or16: R7 = R7 | R8
__or16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	OR
	PLO     R7
	DEC     R2
	GHI     R7
	OR
	PHI     R7
	INC     R2
	SEP     R5

; __xor16: R7 = R7 ^ R8
__xor16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	XOR
	PLO     R7
	DEC     R2
	GHI     R7
	XOR
	PHI     R7
	INC     R2
	SEP     R5

; __shl16: R7 = R7 << R8.0
__shl16:
	GLO     R8
	BZ      .L_shl_done
.L_shl_loop:
	GLO     R7
	SHL
	PLO     R7
	GHI     R7
	SHLC
	PHI     R7
	DEC     R8
	GLO     R8
	BNZ     .L_shl_loop
.L_shl_done:
	SEP     R5

; __shr16: R7 = R7 >> R8.0 (unsigned)
__shr16:
	GLO     R8
	BZ      .L_shr_done
.L_shr_loop:
	GHI     R7
	SHR
	PHI     R7
	GLO     R7
	SHRC
	PLO     R7
	DEC     R8
	GLO     R8
	BNZ     .L_shr_loop
.L_shr_done:
	SEP     R5

; __sar16: R7 = R7 >> R8.0 (arithmetic / signed)
__sar16:
	GLO     R8
	BZ      .L_sar_done
.L_sar_loop:
	; Keep sign bit in DF
	GHI     R7
	SHL
	GHI     R7
	SHRC
	PHI     R7
	GLO     R7
	SHRC
	PLO     R7
	DEC     R8
	GLO     R8
	BNZ     .L_sar_loop
.L_sar_done:
	SEP     R5

; __mul16: R7 = R7 * R8
__mul16:
	; Push constant multiplicand R8 onto stack once
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2          ; R2 points to GLO R8, R2-1 points to GHI R8

	LOAD    RD, 0       ; High word accumulator
	LDI     16
	PLO     RA          ; Bit counter = 16
.L_mul_loop:
	GLO     R7
	SHR
	BNF     .L_mul_no_add

	; Add R8 to RD (carry into DF)
	GLO     RD
	ADD                 ; + M(R2) = GLO R8
	PLO     RD
	DEC     R2          ; points to GHI R8
	GHI     RD
	ADC                 ; + M(R2) = GHI R8 + DF
	PHI     RD
	INC     R2          ; points back to GLO R8

	; Shift RD:R7 right with carry in DF from ADC
	GHI     RD
	SHRC
	PHI     RD
	GLO     RD
	SHRC
	PLO     RD
	GHI     R7
	SHRC
	PHI     R7
	GLO     R7
	SHRC
	PLO     R7
	BR      .L_mul_next

.L_mul_no_add:
	; Shift RD:R7 right with 0 in high bit (DF=0 from BNF)
	GHI     RD
	SHR
	PHI     RD
	GLO     RD
	SHRC
	PLO     RD
	GHI     R7
	SHRC
	PHI     R7
	GLO     R7
	SHRC
	PLO     R7

.L_mul_next:
	DEC     RA
	GLO     RA
	BNZ     .L_mul_loop

	; Result low 16 bits is already in R7
	SEP     R5

; __udivmod16: R7 = R7 / R8 (quotient), R8 = R7 % R8 (remainder)
__udivmod16:
	; Push divisor R8 onto stack once
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2          ; R2 points to GLO R8, R2-1 points to GHI R8

	LOAD    RD, 0       ; Remainder accumulator
	LDI     16
	PLO     RA          ; Bit counter = 16
.L_udiv_loop:
	; Shift R7 left, MSB goes into DF
	GLO     R7
	SHL
	PLO     R7
	GHI     R7
	SHLC
	PHI     R7
	; Shift RD left with carry from R7
	GLO     RD
	SHLC
	PLO     RD
	GHI     RD
	SHLC
	PHI     RD
	; Trial subtraction: RD - R8
	GLO     RD
	SM                  ; D = GLO RD - M(R2) [GLO R8]
	PLO     RC          ; Temporary low diff
	DEC     R2          ; R2 points to GHI R8
	GHI     RD
	SMB                 ; DF = 1 if no borrow (RD >= R8)
	INC     R2          ; R2 points back to GLO R8
	BNF     .L_udiv_no_sub
	; RD >= R8: accept subtraction, set bit 0 of R7
	PHI     RD
	GLO     RC
	PLO     RD
	GLO     R7
	ORI     1
	PLO     R7
.L_udiv_no_sub:
	DEC     RA
	GLO     RA
	BNZ     .L_udiv_loop

	; Remainder into R8
	GHI     RD
	PHI     R8
	GLO     RD
	PLO     R8
	SEP     R5

; __sdiv16: signed division R7 / R8
__sdiv16:
	SEP     R4
	DW      __sdivmod16
	SEP     R5

; __smod16: signed modulo R7 % R8
__smod16:
	SEP     R4
	DW      __sdivmod16
	GHI     R8
	PHI     R7
	GLO     R8
	PLO     R7
	SEP     R5

; __udiv16: unsigned division R7 / R8
__udiv16:
	SEP     R4
	DW      __udivmod16
	SEP     R5

; __umod16: unsigned modulo R7 % R8
__umod16:
	SEP     R4
	DW      __udivmod16
	GHI     R8
	PHI     R7
	GLO     R8
	PLO     R7
	SEP     R5

; __sdivmod16: signed division and modulo
__sdivmod16:
	; Track quotient sign in RA.1, remainder sign in RA.0
	LDI     0
	PHI     RA
	PLO     RA
	; Check sign of dividend R7
	GHI     R7
	SHL
	BNF     .L_sdiv_check_d
	; Dividend negative: negate R7, set remainder sign and toggle quotient sign
	GLO     R7
	SDI     0
	PLO     R7
	GHI     R7
	SDBI    0
	PHI     R7
	LDI     1
	PLO     RA          ; Remainder is negative
	LDI     1
	PHI     RA          ; Quotient sign toggle
.L_sdiv_check_d:
	; Check sign of divisor R8
	GHI     R8
	SHL
	BNF     .L_sdiv_do_div
	; Divisor negative: negate R8, toggle quotient sign
	GLO     R8
	SDI     0
	PLO     R8
	GHI     R8
	SDBI    0
	PHI     R8
	GHI     RA
	XRI     1
	PHI     RA
.L_sdiv_do_div:
	; Save RA (signs)
	GLO     RA
	STXD
	GHI     RA
	STXD
	SEP     R4
	DW      __udivmod16
	; Restore RA
	INC     R2
	LDA     R2
	PHI     RA
	LDN     R2
	PLO     RA
	; Check quotient sign
	GHI     RA
	BZ      .L_sdiv_check_rem
	GLO     R7
	SDI     0
	PLO     R7
	GHI     R7
	SDBI    0
	PHI     R7
.L_sdiv_check_rem:
	GLO     RA
	BZ      .L_sdiv_done
	GLO     R8
	SDI     0
	PLO     R8
	GHI     R8
	SDBI    0
	PHI     R8
.L_sdiv_done:
	SEP     R5

; ── Comparison Helpers ───────────────────────────────────────────────────────
; All helpers compare R7 with R8 and return 1 in D if condition met, 0 if not.
; Also loads D into R7 (0 or 1).

; __cmpeq16: R7 == R8
__cmpeq16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	XOR
	STXD
	GHI     R7
	XOR
	INC     R2
	OR
	BZ      .L_cmp_true
	LBR     .L_cmp_false

; __cmpne16: R7 != R8
__cmpne16:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	XOR
	STXD
	GHI     R7
	XOR
	INC     R2
	OR
	BNZ     .L_cmp_true
	LBR     .L_cmp_false

; __cmplt16_u: R7 < R8 (unsigned)
__cmplt16_u:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	SM
	DEC     R2
	GHI     R7
	SMB
	INC     R2
	; DF = 0 if borrow (R7 < R8)
	BNF     .L_cmp_true
	LBR     .L_cmp_false

; __cmpgte16_u: R7 >= R8 (unsigned)
__cmpgte16_u:
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	SM
	DEC     R2
	GHI     R7
	SMB
	INC     R2
	; DF = 1 if no borrow (R7 >= R8)
	BDF     .L_cmp_true
	LBR     .L_cmp_false

; __cmplte16_u: R7 <= R8 (unsigned)
__cmplte16_u:
	; R7 <= R8 is !(R8 < R7): swap R7/R8 and test __cmpgte16_u
	GLO     R7
	PLO     RD
	GLO     R8
	PLO     R7
	GLO     RD
	PLO     R8
	GHI     R7
	PHI     RD
	GHI     R8
	PHI     R7
	GHI     RD
	PHI     R8
	LBR     __cmpgte16_u

; __cmpgt16_u: R7 > R8 (unsigned)
__cmpgt16_u:
	; R7 > R8 is R8 < R7: swap R7/R8 and test __cmplt16_u
	GLO     R7
	PLO     RD
	GLO     R8
	PLO     R7
	GLO     RD
	PLO     R8
	GHI     R7
	PHI     RD
	GHI     R8
	PHI     R7
	GHI     RD
	PHI     R8
	LBR     __cmplt16_u

; __cmplt16_s: R7 < R8 (signed)
__cmplt16_s:
	; Check sign bits
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	GHI     R7
	XOR                 ; D = R7.1 ^ R8.1
	SHL                 ; DF = 1 if sign bits differ
	INC     R2
	BDF     .L_cmplt_s_diff
	; Same signs: subtraction borrows iff R7 < R8
	GLO     R8
	STXD
	GHI     R8
	STXD
	INC     R2
	INC     R2
	GLO     R7
	SM
	DEC     R2
	GHI     R7
	SMB
	INC     R2
	BNF     .L_cmp_true
	LBR     .L_cmp_false
.L_cmplt_s_diff:
	; Signs differ: R7 < R8 iff R7 is negative (bit 7 set)
	GHI     R7
	SHL
	BDF     .L_cmp_true
	LBR     .L_cmp_false

; __cmpgte16_s: R7 >= R8 (signed)
__cmpgte16_s:
	SEP     R4
	DW      __cmplt16_s
	GLO     R7
	XRI     1
	PLO     R7
	LDI     0
	PHI     R7
	GLO     R7
	SEP     R5

; __cmpgt16_s: R7 > R8 (signed)
__cmpgt16_s:
	; Swap R7/R8 and test R8 < R7
	GLO     R7
	PLO     RD
	GLO     R8
	PLO     R7
	GLO     RD
	PLO     R8
	GHI     R7
	PHI     RD
	GHI     R8
	PHI     R7
	GHI     RD
	PHI     R8
	LBR     __cmplt16_s

; __cmplte16_s: R7 <= R8 (signed)
__cmplte16_s:
	SEP     R4
	DW      __cmpgt16_s
	GLO     R7
	XRI     1
	PLO     R7
	LDI     0
	PHI     R7
	GLO     R7
	SEP     R5

.L_cmp_true:
	LDI     1
	PLO     R7
	LDI     0
	PHI     R7
	LDI     1
	SEP     R5

.L_cmp_false:
	LDI     0
	PLO     R7
	PHI     R7
	LDI     0
	SEP     R5

; ── Panic Runtime ────────────────────────────────────────────────────────────
_panic:
__panic:
	GLO     RB
	STXD
	GHI     RB
	STXD
	GHI     R2
	PHI     RB
	GLO     R2
	PLO     RB

	; Fetch panic message string from caller argument at RB+5..6
	GLO     RB
	ADI     5
	PLO     RC
	GHI     RB
	ADCI    0
	PHI     RC
	LDA     RC
	PHI     R7
	LDN     RC
	PLO     R7

	; Push string argument
	GLO     R7
	STXD
	GHI     R7
	STXD

	; Push format string
	LOAD    RC, .L_panic_fmt
	GLO     RC
	STXD
	GHI     RC
	STXD

	SEP     R4
	DW      _printf
	INC     R2
	INC     R2
	INC     R2
	INC     R2

	LOAD    R7, 1
	GLO     R7
	STXD
	INC     R2
	SEP     R4
	DW      _exit

.L_panic_fmt:
	ASCIZ   "\n*PANIC*: %s\n"
