#ifndef TYPES_H__
#define TYPES_H__

typedef unsigned char bool;
typedef unsigned char byte;
typedef unsigned int uint;

typedef unsigned char gbool;
typedef unsigned char gbyte;
typedef unsigned int gword;

#if  defined(__M6809__)

#include <stdarg.h>

#define OS9_ACIA_PORT 0xFF14
#define ACIA_CONTROL  (OS9_ACIA_PORT+0)
#define ACIA_DATA     (OS9_ACIA_PORT+1)
#define TURBO9SIMPUTCHAR     0xFF00

void printf(const char* format, ...);
void putchar(int);
void putchar_raw(int);

#define assert(B)  assert_6809(B,__FILE__,__LINE__)
void assert_6809(bool b, const char* s, uint n) {
    if (b) return;
    printf("\nASSERT FAILS at %s:%d\n", s, n);
    while (1) {}
}

#if 0
void putchar_raw(int ch) {
    *(volatile byte*)TURBO9SIMPUTCHAR = (byte)ch;
    //*(volatile byte*)ACIA_DATA = (byte)ch;
}
#endif
void putchar_raw(int ch) {
    putchar(ch);
}

#elif  defined(unix)

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>

#else
#error Unknown architecture in types.h
#endif

#define MAX_OCTETS 250 /* short of 255 */

#define false 0
#define true 1

#endif // TYPES_H__
