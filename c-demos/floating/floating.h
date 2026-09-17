#ifndef FLOATING_H
#define FLOATING_H

typedef unsigned char byte;
typedef unsigned short word;

typedef struct FloatingPoint {
    byte SignExp;
    byte Mantissa[4];
} FloatingPoint;

/* Core queries and accessors */
int           floating_is_zero(FloatingPoint f);
byte          floating_sign(FloatingPoint f);
int           floating_get_exp(FloatingPoint f);
void          floating_set_sign_exp(FloatingPoint *f, byte sign, int exp);
FloatingPoint floating_neg(FloatingPoint f);

/* Integer conversions */
FloatingPoint floating_from_word(word w);
word          floating_to_word(FloatingPoint f);
FloatingPoint floating_from_int(int i);
int           floating_to_int(FloatingPoint f);

/* Arithmetic operations */
FloatingPoint floating_add(FloatingPoint a, FloatingPoint b);
FloatingPoint floating_sub(FloatingPoint a, FloatingPoint b);
FloatingPoint floating_mul(FloatingPoint a, FloatingPoint b);
FloatingPoint floating_div(FloatingPoint a, FloatingPoint b);
int           floating_cmp(FloatingPoint a, FloatingPoint b);

/* String formatting and parsing */
char*         floating_format(FloatingPoint f, char *buf);
FloatingPoint floating_scan(const char *s);

#endif /* FLOATING_H */
