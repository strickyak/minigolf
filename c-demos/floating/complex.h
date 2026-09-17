#ifndef COMPLEX_H
#define COMPLEX_H

#include "floating.h"

typedef struct Complex {
    FloatingPoint Re;
    FloatingPoint Im;
} Complex;

Complex       complex_new(FloatingPoint re, FloatingPoint im);
Complex       complex_add(Complex a, Complex b);
Complex       complex_sub(Complex a, Complex b);
Complex       complex_neg(Complex c);
Complex       complex_conj(Complex c);
Complex       complex_mul(Complex a, Complex b);
FloatingPoint complex_abs_sq(Complex c);
FloatingPoint complex_abs(Complex c);
Complex       complex_div(Complex a, Complex b);
Complex       complex_rect(FloatingPoint r, FloatingPoint theta);
void          complex_polar(Complex c, FloatingPoint *r, FloatingPoint *theta);
char*         complex_format(Complex c, char *buf);

#endif /* COMPLEX_H */
