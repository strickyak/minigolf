#ifndef BIG_H
#define BIG_H

typedef unsigned char byte;
typedef unsigned short word;

#define BIG_MAX_WORDS 255

typedef struct Big {
    byte Size;
    word Guts[BIG_MAX_WORDS];
} Big;

void big_zero(Big *z);
void big_set_word(Big *z, word w);
void big_set_small(Big *z, word w);
void big_dup(Big *z, const Big *a);
void big_norm(Big *z);

int  big_is_zero(const Big *z);
int  big_eq_small(const Big *z, word w);
int  big_is_even(const Big *z);
word big_get(const Big *z, byte i);
word big_bit_len(const Big *a);
byte big_bit(const Big *a, word n);

int  big_cmp(const Big *a, const Big *b);
int  big_eq(const Big *a, const Big *b);
int  big_ne(const Big *a, const Big *b);
int  big_lt(const Big *a, const Big *b);
int  big_le(const Big *a, const Big *b);
int  big_gt(const Big *a, const Big *b);
int  big_ge(const Big *a, const Big *b);

void big_add(Big *z, const Big *a, const Big *b);
void big_sub(Big *z, const Big *a, const Big *b);
void big_mul(Big *z, const Big *a, const Big *b);
void big_div2(Big *z, const Big *a);
void big_mul2(Big *z, const Big *a);
void big_lsh(Big *z, const Big *a, word n);
void big_rsh(Big *z, const Big *a, word n);
void big_divmod(Big *q, Big *r, const Big *a, const Big *b);
void big_div(Big *q, const Big *a, const Big *b);
void big_mod(Big *r, const Big *a, const Big *b);
void big_pow(Big *z, const Big *a, const Big *b, const Big *m);
void big_pow_small(Big *z, const Big *a, word exp);

void big_and(Big *z, const Big *a, const Big *b);
void big_or(Big *z, const Big *a, const Big *b);
void big_xor(Big *z, const Big *a, const Big *b);

char* big_format(const Big *z, char *buf);
char* big_format_hex(const Big *z, char *buf);
void  big_print(const Big *z);
void  big_println(const Big *z);
void  big_print_hex(const Big *z);
void  big_println_hex(const Big *z);

void big_from_string(Big *z, const char *s);
void big_from_hex(Big *z, const char *s);

#endif /* BIG_H */
