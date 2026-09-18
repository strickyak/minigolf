#ifndef LONGLONG_H
#define LONGLONG_H

#include "long.h"

typedef struct LongLong {
    byte Bytes[8];
} LongLong;

LongLong longlong_zero(void);
LongLong longlong_one(void);
LongLong longlong_from_word(word w);
LongLong longlong_from_int(int v);
LongLong longlong_from_long(Long a);
Long     longlong_to_long(LongLong a);
LongLong longlong_from_string(const char *s);

int  longlong_is_zero(LongLong a);
int  longlong_is_neg(LongLong a);
int  longlong_cmp(LongLong a, LongLong b);
int  longlong_cmp_u(LongLong a, LongLong b);
int  longlong_eq(LongLong a, LongLong b);
int  longlong_ne(LongLong a, LongLong b);
int  longlong_lt(LongLong a, LongLong b);
int  longlong_le(LongLong a, LongLong b);
int  longlong_gt(LongLong a, LongLong b);
int  longlong_ge(LongLong a, LongLong b);

LongLong longlong_neg(LongLong a);
LongLong longlong_abs(LongLong a);
LongLong longlong_add(LongLong a, LongLong b);
LongLong longlong_sub(LongLong a, LongLong b);
LongLong longlong_mul(LongLong a, LongLong b);
LongLong longlong_div(LongLong a, LongLong b);
LongLong longlong_mod(LongLong a, LongLong b);
void     longlong_divmod(LongLong a, LongLong b, LongLong *q, LongLong *r);
void     longlong_divmod_u(LongLong a, LongLong b, LongLong *q, LongLong *r);

LongLong longlong_and(LongLong a, LongLong b);
LongLong longlong_or(LongLong a, LongLong b);
LongLong longlong_xor(LongLong a, LongLong b);
LongLong longlong_not(LongLong a);
LongLong longlong_shl(LongLong a, word n);
LongLong longlong_shr(LongLong a, word n);
LongLong longlong_shru(LongLong a, word n);

word longlong_to_word(LongLong a);
int  longlong_to_int(LongLong a);

char* longlong_format(LongLong a, char *buf);
char* longlong_format_hex(LongLong a, char *buf);
void  longlong_print(LongLong a);
void  longlong_println(LongLong a);
void  longlong_print_hex(LongLong a);
void  longlong_println_hex(LongLong a);

#endif /* LONGLONG_H */
