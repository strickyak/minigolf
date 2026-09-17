#ifndef COMPLEX_C
#define COMPLEX_C

#include "complex.h"
#include "floating.c"
#include "math.c"

extern void abort(void);

Complex complex_new(FloatingPoint re, FloatingPoint im) {
    Complex c;
    c.Re = re;
    c.Im = im;
    return c;
}

Complex complex_add(Complex a, Complex b) {
    Complex c;
    c.Re = floating_add(a.Re, b.Re);
    c.Im = floating_add(a.Im, b.Im);
    return c;
}

Complex complex_sub(Complex a, Complex b) {
    Complex c;
    c.Re = floating_sub(a.Re, b.Re);
    c.Im = floating_sub(a.Im, b.Im);
    return c;
}

Complex complex_neg(Complex c) {
    Complex r;
    r.Re = floating_neg(c.Re);
    r.Im = floating_neg(c.Im);
    return r;
}

Complex complex_conj(Complex c) {
    Complex r;
    r.Re = c.Re;
    r.Im = floating_neg(c.Im);
    return r;
}

Complex complex_mul(Complex a, Complex b) {
    FloatingPoint ac = floating_mul(a.Re, b.Re);
    FloatingPoint bd = floating_mul(a.Im, b.Im);
    FloatingPoint ad = floating_mul(a.Re, b.Im);
    FloatingPoint bc = floating_mul(a.Im, b.Re);
    Complex r;
    r.Re = floating_sub(ac, bd);
    r.Im = floating_add(ad, bc);
    return r;
}

FloatingPoint complex_abs_sq(Complex c) {
    FloatingPoint re2 = floating_mul(c.Re, c.Re);
    FloatingPoint im2 = floating_mul(c.Im, c.Im);
    return floating_add(re2, im2);
}

FloatingPoint complex_abs(Complex c) {
    return math_hypot(c.Re, c.Im);
}

Complex complex_div(Complex a, Complex b) {
    FloatingPoint denom = complex_abs_sq(b);
    if (floating_is_zero(denom)) {
        abort();
    }
    FloatingPoint ac = floating_mul(a.Re, b.Re);
    FloatingPoint bd = floating_mul(a.Im, b.Im);
    FloatingPoint bc = floating_mul(a.Im, b.Re);
    FloatingPoint ad = floating_mul(a.Re, b.Im);
    FloatingPoint numRe = floating_add(ac, bd);
    FloatingPoint numIm = floating_sub(bc, ad);
    Complex res;
    res.Re = floating_div(numRe, denom);
    res.Im = floating_div(numIm, denom);
    return res;
}

Complex complex_rect(FloatingPoint r, FloatingPoint theta) {
    Complex res;
    FloatingPoint s, c;
    math_sincos(theta, &s, &c);
    res.Re = floating_mul(r, c);
    res.Im = floating_mul(r, s);
    return res;
}

void complex_polar(Complex c, FloatingPoint *r, FloatingPoint *theta) {
    *r = complex_abs(c);
    *theta = math_atan2(c.Im, c.Re);
}

char* complex_format(Complex c, char *buf) {
    char temp[32];
    int pos = 0;
    buf[pos] = '(';
    pos = pos + 1;

    floating_format(c.Re, temp);
    int i = 0;
    while (temp[i] != 0) {
        buf[pos] = temp[i];
        pos = pos + 1;
        i = i + 1;
    }

    if (floating_sign(c.Im) == 0) {
        buf[pos] = '+';
        pos = pos + 1;
    }

    floating_format(c.Im, temp);
    i = 0;
    while (temp[i] != 0) {
        buf[pos] = temp[i];
        pos = pos + 1;
        i = i + 1;
    }

    buf[pos] = 'i';
    pos = pos + 1;
    buf[pos] = ')';
    pos = pos + 1;
    buf[pos] = '\0';
    return buf;
}

#endif /* COMPLEX_C */
