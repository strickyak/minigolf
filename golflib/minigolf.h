/* minigolf.h — Standard MiniGolf C preamble.
 *
 * Include at the top of any .c file compiled with:
 *   minigolf -m=<arch> -I=golflib -o out.asm program.c
 * or with the standalone tool:
 *   cc_to_golf -I=golflib program.c
 *
 * This header defines the primitive types and extern declarations that
 * correspond to MiniGolf built-ins.  Only these types are supported by the
 * cc_to_golf translator; POSIX/libc headers are not compatible.
 *
 * Type mapping:
 *   byte  → unsigned char  (8-bit, matches MiniGolf 'byte')
 *   word  → unsigned long  (pointer-width unsigned, matches MiniGolf 'word')
 *   int   → int            (signed, matches MiniGolf 'int')
 */

#ifndef MINIGOLF_H
#define MINIGOLF_H

typedef unsigned char  byte;
typedef unsigned long  word;

/* I/O primitives — implemented by the MiniGolf runtime */
extern void putchar(char ch);
extern int  getchar(void);

#endif /* MINIGOLF_H */
