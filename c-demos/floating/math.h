#ifndef MATH_H
#define MATH_H

#include "floating.h"

FloatingPoint math_pi(void);
FloatingPoint math_pi_over_two(void);
FloatingPoint math_tau(void);

FloatingPoint math_abs(FloatingPoint x);
FloatingPoint math_sqrt(FloatingPoint x);
FloatingPoint math_hypot(FloatingPoint p, FloatingPoint q);

void          math_sincos(FloatingPoint x, FloatingPoint *sin_out, FloatingPoint *cos_out);
FloatingPoint math_sin(FloatingPoint x);
FloatingPoint math_cos(FloatingPoint x);

FloatingPoint math_atan(FloatingPoint x);
FloatingPoint math_atan2(FloatingPoint y, FloatingPoint x);

#endif /* MATH_H */
