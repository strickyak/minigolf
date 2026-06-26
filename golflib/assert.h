#ifndef _MINIGOLF_CTLIB_ASSERT_H_
#define _MINIGOLF_CTLIB_ASSERT_H_

void abort(void);

void assert(int pred) {
    if (!pred) {
        const char* s = "\n*** golflib: ASSERT FAILED\n";
        while (*s) {
            putchar(*s);
            s++;
        }
        abort();
    }
}

#endif // _MINIGOLF_CTLIB_ASSERT_H_
