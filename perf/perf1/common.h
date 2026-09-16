#ifndef COMMON_H
#define COMMON_H

#if unix
#include <stdio.h>
#else
#ifdef _CMOC_VERSION_
#define volatile
#endif

static void putchar(char c) {
    *(volatile char *)0xFF00 = c;
}

static int getchar(void) {
    int c;
    do {
        c = (int)(*(volatile unsigned char *)0xFF01);
    } while (c == 0);
    return c;
}
#endif

#if defined(__GNUC__)
#define UNUSED __attribute__((unused))
#else
#define UNUSED
#endif

static void UNUSED put_str(const char *s) {
    while (*s) {
        putchar(*s++);
    }
}

static void UNUSED put_num(int n) {
    char buf[10];
    int i;
    unsigned int u;
    i = 0;
    if (n < 0) {
        putchar('-');
        u = (unsigned int)(-n);
    } else {
        u = (unsigned int)n;
    }
    if (u == 0) {
        putchar('0');
        return;
    }
    while (u > 0) {
        buf[i++] = (char)('0' + (u % 10));
        u /= 10;
    }
    while (i > 0) {
        putchar(buf[--i]);
    }
}

static void UNUSED put_nl(void) {
    putchar('\n');
}

#endif /* COMMON_H */
