#ifndef LONG_H
#define LONG_H

typedef unsigned char byte;
typedef unsigned short word;

typedef struct Long {
    byte Bytes[4];
} Long;

Long long_zero(void);
Long long_one(void);
Long long_from_word(word w);
Long long_from_int(int v);
Long long_from_bytes(byte b0, byte b1, byte b2, byte b3);
Long long_from_words(word lo, word hi);
Long long_from_string(const char *s);

int  long_is_zero(Long a);
int  long_is_neg(Long a);
int  long_cmp(Long a, Long b);
int  long_cmp_u(Long a, Long b);
int  long_eq(Long a, Long b);
int  long_ne(Long a, Long b);
int  long_lt(Long a, Long b);
int  long_le(Long a, Long b);
int  long_gt(Long a, Long b);
int  long_ge(Long a, Long b);

Long long_neg(Long a);
Long long_abs(Long a);
Long long_add(Long a, Long b);
Long long_sub(Long a, Long b);
Long long_mul(Long a, Long b);
Long long_div(Long a, Long b);
Long long_mod(Long a, Long b);
void long_divmod(Long a, Long b, Long *q, Long *r);
void long_divmod_u(Long a, Long b, Long *q, Long *r);

Long long_and(Long a, Long b);
Long long_or(Long a, Long b);
Long long_xor(Long a, Long b);
Long long_not(Long a);
Long long_shl(Long a, word n);
Long long_shr(Long a, word n);
Long long_shru(Long a, word n);

word long_to_word(Long a);
int  long_to_int(Long a);

char* long_format(Long a, char *buf);
char* long_format_hex(Long a, char *buf);
void  long_print(Long a);
void  long_println(Long a);
void  long_print_hex(Long a);
void  long_println_hex(Long a);

#endif /* LONG_H */
