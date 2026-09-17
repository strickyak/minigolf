	pragma cescapes
	org $8000
cstart:
	lds #$8000
	clra
	clrb
	tfr d,x
	tfr d,y
	tfr d,u
	lbsr _main
__exit0:
	clra
	clrb
__exit:
	stb $FF05
stuck:
	bra stuck
putchar:
	stb $FF00
	rts
_printf:
	rts

