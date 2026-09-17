#ifndef FFT_H
#define FFT_H

#include "floating.h"
#include "complex.h"

void fft_forward(const Complex *in, int n, Complex *out);
void fft_reverse(const Complex *in, int n, Complex *out);

#endif /* FFT_H */
