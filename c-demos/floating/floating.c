#ifndef FLOATING_C
#define FLOATING_C

#include "floating.h"

extern void abort(void);

int floating_is_zero(FloatingPoint f) {
    return (f.Mantissa[0] == 0 && f.Mantissa[1] == 0 &&
            f.Mantissa[2] == 0 && f.Mantissa[3] == 0);
}

byte floating_sign(FloatingPoint f) {
    return (byte)((f.SignExp >> 7) & 1);
}

int floating_get_exp(FloatingPoint f) {
    int raw = (int)(f.SignExp & 0x7F);
    if ((raw & 0x40) != 0) {
        return raw - 128;
    }
    return raw;
}

void floating_set_sign_exp(FloatingPoint *f, byte sign, int exp) {
    if (exp < -64) {
        f->SignExp = 0;
        f->Mantissa[0] = 0;
        f->Mantissa[1] = 0;
        f->Mantissa[2] = 0;
        f->Mantissa[3] = 0;
        return;
    }
    if (exp > 63) {
        abort();
    }
    byte e = (byte)(exp & 0x7F);
    if (sign != 0) {
        f->SignExp = (byte)(0x80 | e);
    } else {
        f->SignExp = e;
    }
}

FloatingPoint floating_neg(FloatingPoint f) {
    if (floating_is_zero(f)) {
        return f;
    }
    f.SignExp = (byte)(f.SignExp ^ 0x80);
    return f;
}

static int mantissa_is_zero(const byte m[4]) {
    return (m[0] == 0 && m[1] == 0 && m[2] == 0 && m[3] == 0);
}

static int mantissa_cmp(const byte a[4], const byte b[4]) {
    int i;
    for (i = 0; i < 4; i++) {
        if (a[i] > b[i]) return 1;
        if (a[i] < b[i]) return -1;
    }
    return 0;
}

static byte shift_left1(byte m[4]) {
    byte b3 = m[3];
    byte c3 = (byte)((b3 >> 7) & 1);
    m[3] = (byte)(b3 << 1);

    byte b2 = m[2];
    byte c2 = (byte)((b2 >> 7) & 1);
    m[2] = (byte)((b2 << 1) | c3);

    byte b1 = m[1];
    byte c1 = (byte)((b1 >> 7) & 1);
    m[1] = (byte)((b1 << 1) | c2);

    byte b0 = m[0];
    byte c0 = (byte)((b0 >> 7) & 1);
    m[0] = (byte)((b0 << 1) | c1);

    return c0;
}

static void shift_right1(byte m[4], byte carry_in) {
    byte b0 = m[0];
    byte c0 = (byte)(b0 & 1);
    m[0] = (byte)((b0 >> 1) | (carry_in << 7));

    byte b1 = m[1];
    byte c1 = (byte)(b1 & 1);
    m[1] = (byte)((b1 >> 1) | (c0 << 7));

    byte b2 = m[2];
    byte c2 = (byte)(b2 & 1);
    m[2] = (byte)((b2 >> 1) | (c1 << 7));

    byte b3 = m[3];
    m[3] = (byte)((b3 >> 1) | (c2 << 7));
}

static void shift_right_n(byte m[4], word n) {
    if (n >= 32) {
        m[0] = 0; m[1] = 0; m[2] = 0; m[3] = 0;
        return;
    }
    while (n >= 8) {
        m[3] = m[2];
        m[2] = m[1];
        m[1] = m[0];
        m[0] = 0;
        n -= 8;
    }
    while (n > 0) {
        shift_right1(m, 0);
        n--;
    }
}

static byte add_mantissa(byte a[4], const byte b[4]) {
    word carry = 0;
    word sum3 = (word)a[3] + (word)b[3] + carry;
    a[3] = (byte)sum3;
    carry = sum3 >> 8;

    word sum2 = (word)a[2] + (word)b[2] + carry;
    a[2] = (byte)sum2;
    carry = sum2 >> 8;

    word sum1 = (word)a[1] + (word)b[1] + carry;
    a[1] = (byte)sum1;
    carry = sum1 >> 8;

    word sum0 = (word)a[0] + (word)b[0] + carry;
    a[0] = (byte)sum0;
    carry = sum0 >> 8;

    return (byte)carry;
}

static byte sub_mantissa(byte a[4], const byte b[4]) {
    word borrow = 0;

    word diff3 = ((word)a[3] + 256) - (word)b[3] - borrow;
    a[3] = (byte)diff3;
    borrow = 1 - (diff3 >> 8);

    word diff2 = ((word)a[2] + 256) - (word)b[2] - borrow;
    a[2] = (byte)diff2;
    borrow = 1 - (diff2 >> 8);

    word diff1 = ((word)a[1] + 256) - (word)b[1] - borrow;
    a[1] = (byte)diff1;
    borrow = 1 - (diff1 >> 8);

    word diff0 = ((word)a[0] + 256) - (word)b[0] - borrow;
    a[0] = (byte)diff0;
    borrow = 1 - (diff0 >> 8);

    return (byte)borrow;
}

static void normalize(FloatingPoint *f, byte carry, int exp) {
    if (carry != 0) {
        shift_right1(f->Mantissa, carry);
        exp++;
        floating_set_sign_exp(f, floating_sign(*f), exp);
        return;
    }
    if (mantissa_is_zero(f->Mantissa)) {
        f->SignExp = 0;
        return;
    }
    while ((f->Mantissa[0] & 0x80) == 0) {
        shift_left1(f->Mantissa);
        exp--;
        if (exp < -64) {
            f->SignExp = 0;
            f->Mantissa[0] = 0;
            f->Mantissa[1] = 0;
            f->Mantissa[2] = 0;
            f->Mantissa[3] = 0;
            return;
        }
    }
    floating_set_sign_exp(f, floating_sign(*f), exp);
}

FloatingPoint floating_from_word(word w) {
    FloatingPoint res;
    res.SignExp = 0;
    res.Mantissa[0] = 0; res.Mantissa[1] = 0;
    res.Mantissa[2] = 0; res.Mantissa[3] = 0;
    if (w == 0) {
        return res;
    }
    res.Mantissa[0] = (byte)(w >> 8);
    res.Mantissa[1] = (byte)w;

    int exp = 15;
    while ((res.Mantissa[0] & 0x80) == 0) {
        shift_left1(res.Mantissa);
        exp--;
    }
    floating_set_sign_exp(&res, 0, exp);
    return res;
}

word floating_to_word(FloatingPoint f) {
    if (floating_is_zero(f) || floating_sign(f) != 0) {
        return 0;
    }
    int exp = floating_get_exp(f);
    if (exp < 0) {
        return 0;
    }
    if (exp > 15) {
        return 65535;
    }
    word w = ((word)f.Mantissa[0] << 8) | (word)f.Mantissa[1];
    word shift = (word)(15 - exp);
    return w >> shift;
}

FloatingPoint floating_from_int(int i) {
    if (i == 0) {
        FloatingPoint z;
        z.SignExp = 0;
        z.Mantissa[0] = 0; z.Mantissa[1] = 0;
        z.Mantissa[2] = 0; z.Mantissa[3] = 0;
        return z;
    }
    if (i < 0) {
        word mag = (word)(0 - i);
        FloatingPoint res = floating_from_word(mag);
        res.SignExp |= 0x80;
        return res;
    }
    return floating_from_word((word)i);
}

int floating_to_int(FloatingPoint f) {
    if (floating_is_zero(f)) {
        return 0;
    }
    byte sign = floating_sign(f);
    f.SignExp &= 0x7F;
    word w = floating_to_word(f);
    if (sign != 0) {
        return 0 - (int)w;
    }
    return (int)w;
}

FloatingPoint floating_add(FloatingPoint a, FloatingPoint b) {
    if (floating_is_zero(a)) return b;
    if (floating_is_zero(b)) return a;

    int expA = floating_get_exp(a);
    int expB = floating_get_exp(b);
    byte signA = floating_sign(a);
    byte signB = floating_sign(b);

    FloatingPoint res;
    res.SignExp = 0;
    res.Mantissa[0] = 0; res.Mantissa[1] = 0;
    res.Mantissa[2] = 0; res.Mantissa[3] = 0;

    if (expA < expB) {
        FloatingPoint tmp = a;
        a = b;
        b = tmp;
        expA = floating_get_exp(a);
        expB = floating_get_exp(b);
        signA = floating_sign(a);
        signB = floating_sign(b);
    }

    word diff = (word)(expA - expB);
    shift_right_n(b.Mantissa, diff);

    int i;
    if (signA == signB) {
        for (i = 0; i < 4; i++) res.Mantissa[i] = a.Mantissa[i];
        byte carry = add_mantissa(res.Mantissa, b.Mantissa);
        floating_set_sign_exp(&res, signA, expA);
        normalize(&res, carry, expA);
    } else {
        int cmp = mantissa_cmp(a.Mantissa, b.Mantissa);
        if (cmp == 0) {
            return res;
        } else if (cmp > 0) {
            for (i = 0; i < 4; i++) res.Mantissa[i] = a.Mantissa[i];
            sub_mantissa(res.Mantissa, b.Mantissa);
            floating_set_sign_exp(&res, signA, expA);
            normalize(&res, 0, expA);
        } else {
            for (i = 0; i < 4; i++) res.Mantissa[i] = b.Mantissa[i];
            sub_mantissa(res.Mantissa, a.Mantissa);
            floating_set_sign_exp(&res, signB, expA);
            normalize(&res, 0, expA);
        }
    }
    return res;
}

FloatingPoint floating_sub(FloatingPoint a, FloatingPoint b) {
    return floating_add(a, floating_neg(b));
}

FloatingPoint floating_mul(FloatingPoint a, FloatingPoint b) {
    FloatingPoint res;
    res.SignExp = 0;
    res.Mantissa[0] = 0; res.Mantissa[1] = 0;
    res.Mantissa[2] = 0; res.Mantissa[3] = 0;
    if (floating_is_zero(a) || floating_is_zero(b)) {
        return res;
    }
    byte sign = (byte)(floating_sign(a) ^ floating_sign(b));
    int exp = floating_get_exp(a) + floating_get_exp(b);
    if (exp < -64) {
        return res;
    }

    byte acc[4];
    acc[0] = 0; acc[1] = 0; acc[2] = 0; acc[3] = 0;
    byte carry = 0;
    int i;
    for (i = 0; i < 32; i++) {
        if (i > 0) {
            shift_right1(acc, carry);
            carry = 0;
        }
        int byteIdx = 3 - (i >> 3);
        byte bitMask = (byte)(1 << (byte)(i & 7));
        if ((b.Mantissa[byteIdx] & bitMask) != 0) {
            byte c = add_mantissa(acc, a.Mantissa);
            carry = c;
        }
    }
    if (carry != 0) {
        shift_right1(acc, carry);
        exp++;
    }
    if (exp < -64) {
        return res;
    }
    if (exp > 63) {
        abort();
    }
    for (i = 0; i < 4; i++) res.Mantissa[i] = acc[i];
    floating_set_sign_exp(&res, sign, exp);
    return res;
}

FloatingPoint floating_div(FloatingPoint a, FloatingPoint b) {
    FloatingPoint res;
    res.SignExp = 0;
    res.Mantissa[0] = 0; res.Mantissa[1] = 0;
    res.Mantissa[2] = 0; res.Mantissa[3] = 0;
    if (floating_is_zero(b)) {
        abort();
    }
    if (floating_is_zero(a)) {
        return res;
    }

    int expA = floating_get_exp(a);
    int expB = floating_get_exp(b);
    byte signA = floating_sign(a);
    byte signB = floating_sign(b);
    byte resSign = (byte)(signA ^ signB);

    byte rem[4];
    byte divisor[4];
    int i;
    for (i = 0; i < 4; i++) {
        rem[i] = a.Mantissa[i];
        divisor[i] = b.Mantissa[i];
    }

    int expQ;
    int cmp = mantissa_cmp(rem, divisor);
    if (cmp >= 0) {
        expQ = expA - expB;
        res.Mantissa[0] = 0x80;
        sub_mantissa(rem, divisor);
    } else {
        expQ = expA - expB - 1;
        res.Mantissa[0] = 0x80;
        shift_left1(rem);
        sub_mantissa(rem, divisor);
    }

    int bit;
    for (bit = 1; bit < 32; bit++) {
        byte carry = shift_left1(rem);
        if (carry != 0 || mantissa_cmp(rem, divisor) >= 0) {
            sub_mantissa(rem, divisor);
            int byteIdx = bit >> 3;
            byte bitMask = (byte)(1 << (byte)(7 - (bit & 7)));
            res.Mantissa[byteIdx] = (byte)(res.Mantissa[byteIdx] | bitMask);
        }
    }

    byte carry = shift_left1(rem);
    if (carry != 0 || mantissa_cmp(rem, divisor) >= 0) {
        byte one[4];
        one[0] = 0; one[1] = 0; one[2] = 0; one[3] = 1;
        byte c = add_mantissa(res.Mantissa, one);
        if (c != 0) {
            shift_right1(res.Mantissa, c);
            expQ++;
        }
    }

    if (expQ < -64) {
        FloatingPoint z;
        z.SignExp = 0;
        z.Mantissa[0] = 0; z.Mantissa[1] = 0;
        z.Mantissa[2] = 0; z.Mantissa[3] = 0;
        return z;
    }
    if (expQ > 63) {
        abort();
    }
    floating_set_sign_exp(&res, resSign, expQ);
    return res;
}

int floating_cmp(FloatingPoint a, FloatingPoint b) {
    FloatingPoint diff = floating_sub(a, b);
    if (floating_is_zero(diff)) {
        return 0;
    }
    if (floating_sign(diff) != 0) {
        return -1;
    }
    return 1;
}

char* floating_format(FloatingPoint f, char *buf) {
    if (floating_is_zero(f)) {
        buf[0] = '0'; buf[1] = '.'; buf[2] = '0'; buf[3] = '\0';
        return buf;
    }
    int pos = 0;
    if (floating_sign(f) != 0) {
        buf[pos] = '-';
        pos = pos + 1;
        f = floating_neg(f);
    }

    word intPart = floating_to_word(f);
    if (intPart == 0) {
        buf[pos] = '0';
        pos = pos + 1;
    } else {
        char digits[10];
        int dlen = 0;
        word w = intPart;
        while (w > 0) {
            digits[dlen] = (char)('0' + (w % 10));
            dlen = dlen + 1;
            w = w / 10;
        }
        while (dlen > 0) {
            dlen = dlen - 1;
            buf[pos] = digits[dlen];
            pos = pos + 1;
        }
    }

    buf[pos] = '.';
    pos = pos + 1;

    FloatingPoint frac = floating_sub(f, floating_from_word(intPart));
    if (floating_is_zero(frac)) {
        buf[pos] = '0';
        pos = pos + 1;
        buf[pos] = '\0';
        return buf;
    }

    FloatingPoint ten = floating_from_word(10);
    int places;
    for (places = 0; places < 6; places++) {
        if (floating_is_zero(frac) || floating_get_exp(frac) < -20) {
            break;
        }
        frac = floating_mul(frac, ten);
        word d = floating_to_word(frac);
        buf[pos] = (char)('0' + (byte)d);
        pos = pos + 1;
        frac = floating_sub(frac, floating_from_word(d));
    }
    if (buf[pos - 1] == '.') {
        buf[pos] = '0';
        pos = pos + 1;
    }
    buf[pos] = '\0';
    return buf;
}

FloatingPoint floating_scan(const char *s) {
    FloatingPoint val;
    val.SignExp = 0;
    val.Mantissa[0] = 0; val.Mantissa[1] = 0;
    val.Mantissa[2] = 0; val.Mantissa[3] = 0;
    if (!s || !*s) return val;

    int i = 0;
    while (s[i] == ' ' || s[i] == '\t') i++;
    if (!s[i]) return val;

    int isNeg = 0;
    if (s[i] == '-') {
        isNeg = 1;
        i++;
    } else if (s[i] == '+') {
        i++;
    }

    FloatingPoint ten = floating_from_word(10);
    int decPlaces = 0;

    while (s[i] >= '0' && s[i] <= '9') {
        FloatingPoint digit = floating_from_word((word)(s[i] - '0'));
        val = floating_add(floating_mul(val, ten), digit);
        i++;
    }

    if (s[i] == '.') {
        i++;
        while (s[i] >= '0' && s[i] <= '9') {
            FloatingPoint digit = floating_from_word((word)(s[i] - '0'));
            val = floating_add(floating_mul(val, ten), digit);
            decPlaces++;
            i++;
        }
    }

    if (decPlaces > 0) {
        FloatingPoint div = floating_from_word(1);
        int k;
        for (k = 0; k < decPlaces; k++) {
            div = floating_mul(div, ten);
        }
        val = floating_div(val, div);
    }

    if (s[i] == 'e' || s[i] == 'E') {
        i++;
        int expNeg = 0;
        if (s[i] == '-') {
            expNeg = 1;
            i++;
        } else if (s[i] == '+') {
            i++;
        }
        int expVal = 0;
        while (s[i] >= '0' && s[i] <= '9') {
            expVal = expVal * 10 + (int)(s[i] - '0');
            i++;
        }
        int k;
        for (k = 0; k < expVal; k++) {
            if (expNeg) {
                val = floating_div(val, ten);
            } else {
                val = floating_mul(val, ten);
            }
        }
    }

    if (isNeg) {
        val = floating_neg(val);
    }
    return val;
}

#endif /* FLOATING_C */
