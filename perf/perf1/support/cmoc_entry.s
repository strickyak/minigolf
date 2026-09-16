	SECTION start
my_start EXPORT
_main IMPORT
my_start:
	lds #$8000
	lbsr _main
	clra
	clrb
	stb $FF05
.stuck:
	bra .stuck
	ENDSECTION
