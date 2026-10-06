; ── C Runtime Startup for Z80 on Hatvan VM (gepz) ──────────────────────────
	org     $1000

cstart:
	; Set stack pointer
	ld      sp, $8000

	; Call MiniGolf runtime initialization (if generated)
	; call  f_prelude__init_0

	; Call user main
	call    _main

	; Exit with return code in L / A
	push    hl
	call    _exit

; ── Exit System Call ─────────────────────────────────────────────────────────
_exit:
__exit:
	ld      hl, 2
	add     hl, sp
	ld      a, (hl)
	ld      ($FF0A), a
.L_exit_halt:
	halt
	jmp     .L_exit_halt

; ── Console Character I/O ───────────────────────────────────────────────────
_putchar:
putchar:
	ld      hl, 2
	add     hl, sp
	ld      a, (hl)
	ld      ($FF00), a
	ret

__putc:
	ld      ($FF00), a
	ret

_getchar:
getchar:
	ld      a, ($FF02)
	ld      l, a
	ld      h, 0
	ret

; ── Minimal _printf Implementation for Z80 ──────────────────────────────────
_printf:
	push    ix
	ld      ix, 0
	add     ix, sp
	push    bc
	push    de
	push    hl

	ld      l, (ix+4)
	ld      h, (ix+5)       ; HL = format string
	ld      de, 6           ; DE = stack offset to next arg

.L_pf_loop:
	ld      a, (hl)
	or      a
	jmp     z, .L_pf_done
	inc     hl
	cp      '%'
	jmp     nz, .L_pf_putc

	; Specifier
	ld      a, (hl)
	or      a
	jmp     z, .L_pf_done
	inc     hl
	cp      '%'
	jmp     z, .L_pf_putc
	cp      'c'
	jmp     z, .L_pf_char
	cp      's'
	jmp     z, .L_pf_str
	cp      'd'
	jmp     z, .L_pf_dec
	cp      'u'
	jmp     z, .L_pf_udec
	cp      'x'
	jmp     z, .L_pf_hex
	cp      'X'
	jmp     z, .L_pf_hex

	; Unknown specifier: emit '%' and char
	push    af
	ld      a, '%'
	call    __putc
	pop     af
.L_pf_putc:
	call    __putc
	jmp     .L_pf_loop

.L_pf_char:
	push    hl
	push    ix
	pop     hl
	add     hl, de
	ld      a, (hl)
	call    __putc
	inc     de
	inc     de
	pop     hl
	jmp     .L_pf_loop

.L_pf_str:
	push    hl
	push    ix
	pop     hl
	add     hl, de
	ld      c, (hl)
	inc     hl
	ld      b, (hl)         ; BC = string pointer
	inc     de
	inc     de
.L_pf_str_loop:
	ld      a, (bc)
	or      a
	jmp     z, .L_pf_str_done
	call    __putc
	inc     bc
	jmp     .L_pf_str_loop
.L_pf_str_done:
	pop     hl
	jmp     .L_pf_loop

.L_pf_dec:
	push    hl
	push    ix
	pop     hl
	add     hl, de
	ld      c, (hl)
	inc     hl
	ld      b, (hl)         ; BC = number
	inc     de
	inc     de
	bit     7, b
	jmp     z, .L_pf_dec_pos
	ld      a, '-'
	call    __putc
	ld      a, b
	cpl
	ld      b, a
	ld      a, c
	cpl
	ld      c, a
	inc     bc
.L_pf_dec_pos:
	push    de
	call    _print_udec
	pop     de
	pop     hl
	jmp     .L_pf_loop

.L_pf_udec:
	push    hl
	push    ix
	pop     hl
	add     hl, de
	ld      c, (hl)
	inc     hl
	ld      b, (hl)
	inc     de
	inc     de
	push    de
	call    _print_udec
	pop     de
	pop     hl
	jmp     .L_pf_loop

.L_pf_hex:
	push    hl
	push    ix
	pop     hl
	add     hl, de
	ld      c, (hl)
	inc     hl
	ld      b, (hl)
	inc     de
	inc     de
	push    de
	call    _print_hex
	pop     de
	pop     hl
	jmp     .L_pf_loop

.L_pf_done:
	pop     hl
	pop     de
	pop     bc
	pop     ix
	ret

; Print unsigned 16-bit in BC
_print_udec:
	ld      hl, 0
	push    hl              ; null terminator
.L_udec_div_loop:
	push    bc
	pop     hl
	ld      de, 10
	call    __udivmod16     ; quotient in BC, rem in HL
	ld      a, l
	add     a, '0'
	push    af
	ld      a, b
	or      c
	jmp     nz, .L_udec_div_loop
.L_udec_print_loop:
	pop     af
	or      a
	ret     z
	call    __putc
	jmp     .L_udec_print_loop

; Print 16-bit hex in BC
_print_hex:
	ld      a, b
	call    _print_hex_byte
	ld      a, c
	call    _print_hex_byte
	ret

_print_hex_byte:
	push    af
	rrca
	rrca
	rrca
	rrca
	call    _print_hex_nibble
	pop     af
	call    _print_hex_nibble
	ret

_print_hex_nibble:
	and     $0F
	cp      10
	jmp     c, .L_nib_dec
	add     a, 'A' - 10
	call    __putc
	ret
.L_nib_dec:
	add     a, '0'
	call    __putc
	ret
