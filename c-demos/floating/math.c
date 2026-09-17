#ifndef MATH_C
#define MATH_C

#include "math.h"
#include "floating.c"

extern void abort(void);

FloatingPoint math_pi(void) {
    FloatingPoint p;
    p.SignExp = 0;
    p.Mantissa[0] = 0xC9;
    p.Mantissa[1] = 0x0F;
    p.Mantissa[2] = 0xDA;
    p.Mantissa[3] = 0xA2;
    floating_set_sign_exp(&p, 0, 1);
    return p;
}

FloatingPoint math_pi_over_two(void) {
    FloatingPoint p;
    p.SignExp = 0;
    p.Mantissa[0] = 0xC9;
    p.Mantissa[1] = 0x0F;
    p.Mantissa[2] = 0xDA;
    p.Mantissa[3] = 0xA2;
    floating_set_sign_exp(&p, 0, 0);
    return p;
}

FloatingPoint math_tau(void) {
    FloatingPoint p;
    p.SignExp = 0;
    p.Mantissa[0] = 0xC9;
    p.Mantissa[1] = 0x0F;
    p.Mantissa[2] = 0xDA;
    p.Mantissa[3] = 0xA2;
    floating_set_sign_exp(&p, 0, 2);
    return p;
}

FloatingPoint math_abs(FloatingPoint x) {
    if (floating_sign(x) != 0) {
        return floating_neg(x);
    }
    return x;
}

FloatingPoint math_sqrt(FloatingPoint x) {
    if (floating_sign(x) != 0 && !floating_is_zero(x)) {
        abort();
    }
    if (floating_is_zero(x)) {
        return x;
    }

    int exp = floating_get_exp(x);
    int resExp;
    FloatingPoint y;
    y.SignExp = 0;
    y.Mantissa[1] = 0;
    y.Mantissa[2] = 0;
    y.Mantissa[3] = 0;

    if ((exp & 1) == 0) {
        if (exp < 0) {
            resExp = 0 - (int)((word)(0 - exp) / 2);
        } else {
            resExp = exp / 2;
        }
        y.Mantissa[0] = 0x98;
    } else {
        if (exp < 0) {
            resExp = 0 - (int)((word)(0 - (exp - 1)) / 2);
        } else {
            resExp = exp / 2;
        }
        y.Mantissa[0] = 0xD8;
    }
    floating_set_sign_exp(&y, 0, resExp);

    int iter;
    for (iter = 0; iter < 6; iter++) {
        FloatingPoint div = floating_div(x, y);
        FloatingPoint sum = floating_add(y, div);
        FloatingPoint next = sum;
        floating_set_sign_exp(&next, 0, floating_get_exp(next) - 1);
        if (next.SignExp == y.SignExp &&
            next.Mantissa[0] == y.Mantissa[0] &&
            next.Mantissa[1] == y.Mantissa[1] &&
            next.Mantissa[2] == y.Mantissa[2] &&
            next.Mantissa[3] == y.Mantissa[3]) {
            break;
        }
        y = next;
    }
    return y;
}

FloatingPoint math_hypot(FloatingPoint p, FloatingPoint q) {
    FloatingPoint p2 = floating_mul(p, p);
    FloatingPoint q2 = floating_mul(q, q);
    return math_sqrt(floating_add(p2, q2));
}

static FloatingPoint sin_core(FloatingPoint z) {
    FloatingPoint one = floating_from_word(1);
    FloatingPoint c6 = floating_div(one, floating_from_word(6));
    FloatingPoint c120 = floating_div(one, floating_from_word(120));
    FloatingPoint c5040 = floating_div(one, floating_from_word(5040));
    FloatingPoint fact9 = floating_mul(floating_from_word(5040), floating_from_word(72));
    FloatingPoint c362880 = floating_div(one, fact9);

    FloatingPoint z2 = floating_mul(z, z);
    FloatingPoint t = floating_sub(c5040, floating_mul(z2, c362880));
    t = floating_sub(c120, floating_mul(z2, t));
    t = floating_sub(c6, floating_mul(z2, t));
    FloatingPoint poly = floating_sub(one, floating_mul(z2, t));
    return floating_mul(z, poly);
}

static FloatingPoint cos_core(FloatingPoint z) {
    FloatingPoint one = floating_from_word(1);
    FloatingPoint c2;
    c2.SignExp = 0;
    c2.Mantissa[0] = 0x80;
    c2.Mantissa[1] = 0; c2.Mantissa[2] = 0; c2.Mantissa[3] = 0;
    floating_set_sign_exp(&c2, 0, -1);

    FloatingPoint c24 = floating_div(one, floating_from_word(24));
    FloatingPoint c720 = floating_div(one, floating_from_word(720));
    FloatingPoint c40320 = floating_div(one, floating_from_word(40320));

    FloatingPoint z2 = floating_mul(z, z);
    FloatingPoint t = floating_sub(c720, floating_mul(z2, c40320));
    t = floating_sub(c24, floating_mul(z2, t));
    t = floating_sub(c2, floating_mul(z2, t));
    return floating_sub(one, floating_mul(z2, t));
}

void math_sincos(FloatingPoint x, FloatingPoint *sin_out, FloatingPoint *cos_out) {
    if (floating_is_zero(x)) {
        *sin_out = floating_from_word(0);
        *cos_out = floating_from_word(1);
        return;
    }

    int negSin = 0;
    if (floating_sign(x) != 0) {
        negSin = 1;
        x = floating_neg(x);
    }

    FloatingPoint tau = math_tau();
    FloatingPoint pi2 = math_pi_over_two();
    FloatingPoint pi4 = math_pi_over_two();
    floating_set_sign_exp(&pi4, 0, -1);

    if (floating_cmp(x, tau) >= 0) {
        word k = floating_to_word(floating_div(x, tau));
        x = floating_sub(x, floating_mul(floating_from_word(k), tau));
        if (floating_cmp(x, tau) >= 0) {
            x = floating_sub(x, tau);
        }
    }

    int q = 0;
    if (floating_cmp(x, pi2) >= 0) {
        q = (int)floating_to_word(floating_div(x, pi2));
        if (q > 3) {
            q = 3;
        }
        x = floating_sub(x, floating_mul(floating_from_word((word)q), pi2));
    }

    FloatingPoint s;
    FloatingPoint c;
    if (floating_cmp(x, pi4) > 0) {
        FloatingPoint d = floating_sub(pi2, x);
        s = cos_core(d);
        c = sin_core(d);
    } else {
        s = sin_core(x);
        c = cos_core(x);
    }

    if (q == 0) {
        *sin_out = s;
        *cos_out = c;
    } else if (q == 1) {
        *sin_out = c;
        *cos_out = floating_neg(s);
    } else if (q == 2) {
        *sin_out = floating_neg(s);
        *cos_out = floating_neg(c);
    } else {
        *sin_out = floating_neg(c);
        *cos_out = s;
    }

    if (negSin) {
        *sin_out = floating_neg(*sin_out);
    }
}

FloatingPoint math_sin(FloatingPoint x) {
    FloatingPoint s, c;
    math_sincos(x, &s, &c);
    return s;
}

FloatingPoint math_cos(FloatingPoint x) {
    FloatingPoint s, c;
    math_sincos(x, &s, &c);
    return c;
}

static FloatingPoint atan_core(FloatingPoint z) {
    FloatingPoint one = floating_from_word(1);
    FloatingPoint u = floating_mul(z, z);

    FloatingPoint c15 = floating_div(one, floating_from_word(15));
    FloatingPoint c13 = floating_div(one, floating_from_word(13));
    FloatingPoint c11 = floating_div(one, floating_from_word(11));
    FloatingPoint c9 = floating_div(one, floating_from_word(9));
    FloatingPoint c7 = floating_div(one, floating_from_word(7));
    FloatingPoint c5 = floating_div(one, floating_from_word(5));
    FloatingPoint c3 = floating_div(one, floating_from_word(3));

    FloatingPoint t = floating_sub(c13, floating_mul(u, c15));
    t = floating_sub(c11, floating_mul(u, t));
    t = floating_sub(c9, floating_mul(u, t));
    t = floating_sub(c7, floating_mul(u, t));
    t = floating_sub(c5, floating_mul(u, t));
    t = floating_sub(c3, floating_mul(u, t));
    FloatingPoint poly = floating_sub(one, floating_mul(u, t));
    return floating_mul(z, poly);
}

FloatingPoint math_atan(FloatingPoint x) {
    if (floating_is_zero(x)) {
        return x;
    }
    int isNeg = 0;
    if (floating_sign(x) != 0) {
        isNeg = 1;
        x = floating_neg(x);
    }

    FloatingPoint one = floating_from_word(1);
    FloatingPoint pi2 = math_pi_over_two();
    FloatingPoint pi4 = math_pi_over_two();
    floating_set_sign_exp(&pi4, 0, -1);

    FloatingPoint two = floating_from_word(2);
    FloatingPoint tanPi8 = floating_sub(math_sqrt(two), one);

    FloatingPoint res;
    if (floating_cmp(x, one) > 0) {
        FloatingPoint invX = floating_div(one, x);
        if (floating_cmp(invX, tanPi8) > 0) {
            FloatingPoint num = floating_sub(invX, one);
            FloatingPoint den = floating_add(invX, one);
            FloatingPoint red = floating_div(num, den);
            FloatingPoint negRed = floating_neg(red);
            FloatingPoint subAtan = floating_neg(atan_core(negRed));
            res = floating_sub(pi2, floating_add(pi4, subAtan));
        } else {
            res = floating_sub(pi2, atan_core(invX));
        }
    } else if (floating_cmp(x, tanPi8) > 0) {
        FloatingPoint num = floating_sub(x, one);
        FloatingPoint den = floating_add(x, one);
        FloatingPoint red = floating_div(num, den);
        FloatingPoint negRed = floating_neg(red);
        FloatingPoint subAtan = floating_neg(atan_core(negRed));
        res = floating_add(pi4, subAtan);
    } else {
        res = atan_core(x);
    }

    if (isNeg) {
        res = floating_neg(res);
    }
    return res;
}

FloatingPoint math_atan2(FloatingPoint y, FloatingPoint x) {
    if (floating_is_zero(x)) {
        if (floating_is_zero(y)) {
            return floating_from_word(0);
        }
        if (floating_sign(y) != 0) {
            return floating_neg(math_pi_over_two());
        }
        return math_pi_over_two();
    }

    if (floating_sign(x) == 0) {
        return math_atan(floating_div(y, x));
    }

    FloatingPoint atanYX = math_atan(floating_div(y, x));
    if (floating_sign(y) == 0) {
        return floating_add(atanYX, math_pi());
    }
    return floating_sub(atanYX, math_pi());
}

#endif /* MATH_C */
