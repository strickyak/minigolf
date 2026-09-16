	.module gcc_support.s

	.area .text.startup
	.globl cstart
	.globl _main
cstart:
	lds #$8000
	clra
	clrb
	tfr d,x
	tfr d,y
	tfr d,u
	lbsr _main
	clra
	clrb
	stb $FF05
.stuck:
	bra .stuck

	.area .text
	.globl _abort
_abort:
	stb $FF05
.abort_stuck:
	bra .abort_stuck

	.globl _memcpy
_memcpy:
	pshs y,u
	ldy 6,s
	ldd 8,s
	beq .mc_done
	leau ,x
.mc_loop:
	lda ,y+
	sta ,x+
	subd #1
	bne .mc_loop
	tfr u,x
.mc_done:
	puls y,u,pc

	.globl _memset
_memset:
	pshs y,u
	ldb 7,s
	ldy 8,s
	beq .ms_done
	leau ,x
.ms_loop:
	stb ,x+
	leay -1,y
	bne .ms_loop
	tfr u,x
.ms_done:
	puls y,u,pc
