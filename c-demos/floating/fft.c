#ifndef FFT_C
#define FFT_C

#include "fft.h"
#include "complex.c"
#include "math.c"
#include "floating.c"

extern void abort(void);

static word fft_bit_reverse(word x, int bits) {
    word res = 0;
    int k;
    for (k = 0; k < bits; k = k + 1) {
        res = (res << 1) | (x & 1);
        x = x >> 1;
    }
    return res;
}

static void fft_transform(const Complex *in, int n, Complex *out, int is_reverse) {
    if (n <= 0 || (n & (n - 1)) != 0) {
        abort();
    }

    int bits = 0;
    while ((1 << bits) < n) {
        bits = bits + 1;
    }

    int i;
    for (i = 0; i < n; i = i + 1) {
        word rev = fft_bit_reverse((word)i, bits);
        out[i] = in[rev];
    }

    FloatingPoint tau = math_tau();
    FloatingPoint one = floating_from_word(1);

    int length;
    for (length = 2; length <= n; length = length << 1) {
        int halfLen = length >> 1;
        FloatingPoint angle = floating_div(tau, floating_from_word((word)length));
        if (!is_reverse) {
            angle = floating_neg(angle);
        }
        Complex wm = complex_rect(one, angle);

        for (i = 0; i < n; i = i + length) {
            Complex w = complex_new(one, floating_from_word(0));
            int j;
            for (j = 0; j < halfLen; j = j + 1) {
                Complex u = out[i + j];
                Complex v = complex_mul(out[i + j + halfLen], w);
                out[i + j] = complex_add(u, v);
                out[i + j + halfLen] = complex_sub(u, v);
                w = complex_mul(w, wm);
            }
        }
    }

    if (is_reverse) {
        FloatingPoint scale = floating_div(one, floating_from_word((word)n));
        for (i = 0; i < n; i = i + 1) {
            Complex c = out[i];
            c.Re = floating_mul(c.Re, scale);
            c.Im = floating_mul(c.Im, scale);
            out[i] = c;
        }
    }
}

void fft_forward(const Complex *in, int n, Complex *out) {
    fft_transform(in, n, out, 0);
}

void fft_reverse(const Complex *in, int n, Complex *out) {
    fft_transform(in, n, out, 1);
}

#endif /* FFT_C */
