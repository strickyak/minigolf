; ── C Runtime Startup for Motorola 68000 on Hatvan VM/K ─────────────────────
	org     $00001000

cstart:
	; Set initial Supervisor Stack Pointer
	move.l  #$00080000, sp
	move.l  sp, a6

	; Call MiniGolf runtime initialization
	jsr     f_prelude__init_0

	; Call user main
	jsr     _main

	; Exit with return code in D0
	move.l  d0, -(sp)
	jsr     _exit

; ── Exit System Call ─────────────────────────────────────────────────────────
_exit:
__exit:
	; Exit code is 16-bit word at $00FF000A
	move.w  6(sp), $00FF000A
.L_exit_halt:
	stop    #$2700
	bra     .L_exit_halt

; ── Console Character I/O ───────────────────────────────────────────────────
_putchar:
putchar:
	; Character byte is at 7(sp) (lowest byte of 32-bit stack slot)
	move.b  7(sp), $00FF0000
	rts

_getchar:
getchar:
	move.b  $00FF0002, d0
	ext.w   d0
	ext.l   d0
	rts

f_prelude__mul_byte:
	move.l  4(sp), d0
	move.l  8(sp), d1
	mulu.w  d1, d0
	rts

; ── Minimal _printf Implementation ──────────────────────────────────────────
_printf:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	move.l  a2, -(sp)
	move.l  a3, -(sp)

	move.l  20(sp), a2       ; A2 = format string pointer
	lea     24(sp), a3       ; A3 = pointer to first argument

.L_pf_loop:
	move.b  (a2)+, d2
	beq     .L_pf_done
	cmp.b   #'%', d2
	bne     .L_pf_putc

	; Format specifier
	move.b  (a2)+, d2
	beq     .L_pf_done
	cmp.b   #'%', d2
	beq     .L_pf_putc
	cmp.b   #'c', d2
	beq     .L_pf_char
	cmp.b   #'s', d2
	beq     .L_pf_str
	cmp.b   #'d', d2
	beq     .L_pf_dec
	cmp.b   #'u', d2
	beq     .L_pf_udec
	cmp.b   #'x', d2
	beq     .L_pf_hex
	cmp.b   #'X', d2
	beq     .L_pf_hex

	; Unknown specifier: print '%' then char
	move.b  #'%', $00FF0000
	bra     .L_pf_putc

.L_pf_putc:
	move.b  d2, $00FF0000
	bra     .L_pf_loop

.L_pf_char:
	move.l  (a3)+, d2
	move.b  d2, $00FF0000
	bra     .L_pf_loop

.L_pf_str:
	move.l  (a3)+, a0
	move.l  a0, d0
	bne     .L_pf_str_loop
	lea     .L_null_str, a0
.L_pf_str_loop:
	move.b  (a0)+, d2
	beq     .L_pf_loop
	move.b  d2, $00FF0000
	bra     .L_pf_str_loop

.L_pf_dec:
	move.l  (a3)+, d0
	bsr     _print_signed_dec
	bra     .L_pf_loop

.L_pf_udec:
	move.l  (a3)+, d0
	bsr     _print_unsigned_dec
	bra     .L_pf_loop

.L_pf_hex:
	move.l  (a3)+, d0
	bsr     _print_hex32
	bra     .L_pf_loop

.L_pf_done:
	move.l  (sp)+, a3
	move.l  (sp)+, a2
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

_print_signed_dec:
	tst.l   d0
	bge     _print_unsigned_dec
	neg.l   d0
	move.b  #'-', $00FF0000
	; fall through to _print_unsigned_dec

_print_unsigned_dec:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	moveq   #0, d3       ; digit count on stack
.L_pud_loop:
	move.l  #10, d1
	jsr     __udivmod32  ; D0 = quotient, D1 = remainder
	add.b   #'0', d1
	move.b  d1, -(sp)
	addq.l  #1, d3
	tst.l   d0
	bne     .L_pud_loop

.L_pud_pop:
	move.b  (sp)+, $00FF0000
	subq.l  #1, d3
	bne     .L_pud_pop

	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

_print_hex32:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	move.l  d0, d2
	moveq   #7, d3
.L_hex_loop:
	rol.l   #4, d2
	move.b  d2, d0
	and.b   #$0F, d0
	cmp.b   #9, d0
	ble     .L_hex_digit
	add.b   #'A'-10, d0
	bra     .L_hex_out
.L_hex_digit:
	add.b   #'0', d0
.L_hex_out:
	move.b  d0, $00FF0000
	dbra    d3, .L_hex_loop
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

	even
.L_null_str:
	dc.b    "(null)", 0
	even
