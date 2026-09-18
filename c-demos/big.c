#ifndef BIG_C
#define BIG_C

#include "big.h"

extern void putchar(char ch);
extern void abort(void);

static word add_carry(word a, word b, word *cin) {
    word s0 = (a & 0xFF) + (b & 0xFF) + *cin;
    word c = s0 >> 8;
    word s1 = ((a >> 8) & 0xFF) + ((b >> 8) & 0xFF) + c;
    *cin = s1 >> 8;
    return (word)((s0 & 0xFF) | (((s1 & 0xFF) << 8) & 0xFFFF));
}

static word sub_borrow(word a, word b, word *bin) {
    word a0 = a & 0xFF;
    word b0 = (b & 0xFF) + *bin;
    word borrow0 = 0;
    if (a0 < b0) {
        a0 = a0 + 0x100;
        borrow0 = 1;
    }
    word d0 = a0 - b0;

    word a1 = (a >> 8) & 0xFF;
    word b1 = ((b >> 8) & 0xFF) + borrow0;
    word borrow1 = 0;
    if (a1 < b1) {
        a1 = a1 + 0x100;
        borrow1 = 1;
    }
    word d1 = a1 - b1;

    *bin = borrow1;
    return (word)((d0 & 0xFF) | (((d1 & 0xFF) << 8) & 0xFFFF));
}

static void mul_word(word u, word v, word *hi, word *lo) {
    word u0 = u & 0xFF;
    word u1 = (u >> 8) & 0xFF;
    word v0 = v & 0xFF;
    word v1 = (v >> 8) & 0xFF;

    word p00 = u0 * v0;
    word p01 = u0 * v1;
    word p10 = u1 * v0;
    word p11 = u1 * v1;

    word mid = (p00 >> 8) + (p01 & 0xFF) + (p10 & 0xFF);
    *lo = (word)((p00 & 0xFF) | (((mid & 0xFF) << 8) & 0xFFFF));
    *hi = (word)(((mid >> 8) + (p01 >> 8) + (p10 >> 8) + p11) & 0xFFFF);
}

void big_zero(Big *z) {
    z->Size = 0;
}

void big_norm(Big *z) {
    while (z->Size > 0 && z->Guts[z->Size - 1] == 0) {
        z->Size = (byte)(z->Size - 1);
    }
}

static byte div10(Big *q, const Big *a) {
    if (a->Size == 0) {
        q->Size = 0;
        return 0;
    }
    Big t;
    t.Size = a->Size;
    word rem = 0;
    int step;
    for (step = 0; step < (int)a->Size; step++) {
        int i = (int)a->Size - 1 - step;
        word w = a->Guts[i];
        word hi = (w >> 8) & 0xFF;
        word lo = w & 0xFF;

        word d1 = (word)(((rem & 0xFF) << 8) | hi);
        word q1 = d1 / 10;
        rem = d1 % 10;

        word d2 = (word)(((rem & 0xFF) << 8) | lo);
        word q2 = d2 / 10;
        rem = d2 % 10;

        t.Guts[i] = (word)(((q1 & 0xFF) << 8) | (q2 & 0xFF));
    }
    big_norm(&t);
    big_dup(q, &t);
    return (byte)rem;
}

void big_set_word(Big *z, word w) {
    w = w & 0xFFFF;
    if (w == 0) {
        z->Size = 0;
    } else {
        z->Size = 1;
        z->Guts[0] = w;
    }
}

void big_set_small(Big *z, word w) {
    big_set_word(z, w);
}

void big_dup(Big *z, const Big *a) {
    byte sz = a->Size;
    z->Size = sz;
    byte i;
    for (i = 0; i < sz; i++) {
        z->Guts[i] = a->Guts[i];
    }
}

int big_is_zero(const Big *z) {
    return (z->Size == 0);
}

int big_eq_small(const Big *z, word w) {
    w = w & 0xFFFF;
    if (w == 0) {
        return (z->Size == 0);
    }
    if (z->Size != 1) {
        return 0;
    }
    return (z->Guts[0] == w);
}

int big_is_even(const Big *z) {
    if (z->Size == 0) {
        return 1;
    }
    return ((z->Guts[0] & 1) == 0);
}

word big_get(const Big *z, byte i) {
    if (i < z->Size) {
        return z->Guts[i];
    }
    return 0;
}

word big_bit_len(const Big *a) {
    if (a->Size == 0) return 0;
    word top = a->Guts[a->Size - 1];
    word bits = 0;
    while (top > 0) {
        bits = bits + 1;
        top = top >> 1;
    }
    return (word)(((word)(a->Size - 1) << 4) + bits);
}

byte big_bit(const Big *a, word n) {
    byte widx = (byte)(n >> 4);
    if (widx >= a->Size) return 0;
    word bidx = n & 15;
    return (byte)((a->Guts[widx] >> bidx) & 1);
}

int big_cmp(const Big *a, const Big *b) {
    if (a->Size > b->Size) return 1;
    if (a->Size < b->Size) return -1;
    if (a->Size == 0) return 0;
    int step;
    for (step = 0; step < (int)a->Size; step++) {
        int i = (int)a->Size - 1 - step;
        word ai = a->Guts[i];
        word bi = b->Guts[i];
        if (ai > bi) return 1;
        if (ai < bi) return -1;
    }
    return 0;
}

int big_eq(const Big *a, const Big *b) { if (big_cmp(a, b) == 0) return 1; return 0; }
int big_ne(const Big *a, const Big *b) { if (big_cmp(a, b) != 0) return 1; return 0; }
int big_lt(const Big *a, const Big *b) { if (big_cmp(a, b) < 0) return 1; return 0; }
int big_le(const Big *a, const Big *b) { if (big_cmp(a, b) <= 0) return 1; return 0; }
int big_gt(const Big *a, const Big *b) { if (big_cmp(a, b) > 0) return 1; return 0; }
int big_ge(const Big *a, const Big *b) { if (big_cmp(a, b) >= 0) return 1; return 0; }

void big_add(Big *z, const Big *a, const Big *b) {
    byte sz = a->Size;
    if (b->Size > sz) sz = b->Size;
    Big t;
    word carry = 0;
    byte i;
    for (i = 0; i < sz; i++) {
        word wa = big_get(a, i);
        word wb = big_get(b, i);
        t.Guts[i] = add_carry(wa, wb, &carry);
    }
    t.Size = sz;
    if (carry > 0) {
        if (sz >= BIG_MAX_WORDS) abort();
        t.Guts[sz] = 1;
        t.Size = (byte)(sz + 1);
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_sub(Big *z, const Big *a, const Big *b) {
    if (big_lt(a, b)) abort();
    Big t;
    word borrow = 0;
    byte i;
    for (i = 0; i < a->Size; i++) {
        word wa = a->Guts[i];
        word wb = big_get(b, i);
        t.Guts[i] = sub_borrow(wa, wb, &borrow);
    }
    if (borrow > 0) abort();
    t.Size = a->Size;
    big_norm(&t);
    big_dup(z, &t);
}

void big_mul(Big *z, const Big *a, const Big *b) {
    if (a->Size == 0 || b->Size == 0) {
        z->Size = 0;
        return;
    }
    Big t;
    word sz = (word)a->Size + (word)b->Size;
    if (sz > BIG_MAX_WORDS) abort();
    word w_idx;
    for (w_idx = 0; w_idx < sz; w_idx++) {
        t.Guts[w_idx] = 0;
    }
    t.Size = (byte)sz;

    byte i;
    byte j;
    for (i = 0; i < a->Size; i++) {
        word ai = a->Guts[i];
        if (ai != 0) {
            word carry = 0;
            for (j = 0; j < b->Size; j++) {
                word k = (word)i + (word)j;
                word p_hi;
                word p_lo;
                mul_word(ai, b->Guts[j], &p_hi, &p_lo);

                word c1 = 0;
                word sum1 = add_carry(p_lo, t.Guts[k], &c1);
                word c2 = 0;
                word sum2 = add_carry(sum1, carry, &c2);
                t.Guts[k] = sum2;

                carry = p_hi + c1 + c2;
            }
            word idx = (word)i + (word)b->Size;
            while (carry > 0) {
                word c3 = 0;
                t.Guts[idx] = add_carry(t.Guts[idx], carry, &c3);
                carry = c3;
                idx = idx + 1;
            }
        }
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_div2(Big *z, const Big *a) {
    if (a->Size == 0) {
        z->Size = 0;
        return;
    }
    Big t;
    t.Size = a->Size;
    word carry = 0;
    int step;
    for (step = 0; step < (int)a->Size; step++) {
        int i = (int)a->Size - 1 - step;
        word w = a->Guts[i];
        t.Guts[i] = (word)((w >> 1) | ((carry << 15) & 0xFFFF));
        carry = w & 1;
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_mul2(Big *z, const Big *a) {
    if (a->Size == 0) {
        z->Size = 0;
        return;
    }
    Big t;
    word carry = 0;
    byte i;
    for (i = 0; i < a->Size; i++) {
        word w = a->Guts[i];
        t.Guts[i] = (word)(((w << 1) & 0xFFFF) | carry);
        carry = (w >> 15) & 1;
    }
    t.Size = a->Size;
    if (carry > 0) {
        if (t.Size >= BIG_MAX_WORDS) abort();
        t.Guts[t.Size] = 1;
        t.Size = (byte)(t.Size + 1);
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_lsh(Big *z, const Big *a, word n) {
    if (a->Size == 0 || n == 0) {
        big_dup(z, a);
        return;
    }
    byte wshift = (byte)(n >> 4);
    word bshift = n & 15;
    Big t;
    word sz = (word)a->Size + (word)wshift + 1;
    if (sz > BIG_MAX_WORDS) abort();
    word k;
    for (k = 0; k < sz; k++) {
        t.Guts[k] = 0;
    }
    if (bshift == 0) {
        byte i;
        for (i = 0; i < a->Size; i++) {
            t.Guts[i + wshift] = a->Guts[i];
        }
        t.Size = (byte)(a->Size + wshift);
    } else {
        word carry = 0;
        word rshift = 16 - bshift;
        byte i;
        for (i = 0; i < a->Size; i++) {
            word w = a->Guts[i];
            t.Guts[i + wshift] = (word)(((w << bshift) & 0xFFFF) | carry);
            carry = w >> rshift;
        }
        t.Guts[a->Size + wshift] = carry;
        t.Size = (byte)(a->Size + wshift + 1);
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_rsh(Big *z, const Big *a, word n) {
    if (a->Size == 0) {
        z->Size = 0;
        return;
    }
    byte wshift = (byte)(n >> 4);
    if (wshift >= a->Size) {
        z->Size = 0;
        return;
    }
    word bshift = n & 15;
    Big t;
    byte rem_words = (byte)(a->Size - wshift);
    if (bshift == 0) {
        byte i;
        for (i = 0; i < rem_words; i++) {
            t.Guts[i] = a->Guts[i + wshift];
        }
        t.Size = rem_words;
    } else {
        word lshift = 16 - bshift;
        byte i;
        for (i = 0; i < rem_words; i++) {
            word w = a->Guts[i + wshift];
            word next = 0;
            if ((byte)(i + 1) < rem_words) {
                next = a->Guts[i + 1 + wshift];
            }
            t.Guts[i] = (word)(((w >> bshift) | ((next << lshift) & 0xFFFF)) & 0xFFFF);
        }
        t.Size = rem_words;
    }
    big_norm(&t);
    big_dup(z, &t);
}

void big_divmod(Big *q, Big *r, const Big *a, const Big *b) {
    if (b->Size == 0) abort();
    if (big_lt(a, b)) {
        q->Size = 0;
        big_dup(r, a);
        return;
    }
    Big rem;
    big_dup(&rem, a);

    Big quot;
    quot.Size = 0;

    Big b_shift;
    word k = big_bit_len(a) - big_bit_len(b);
    big_lsh(&b_shift, b, k);

    word step;
    for (step = 0; step <= k; step++) {
        word shift = k - step;
        if (big_cmp(&rem, &b_shift) >= 0) {
            big_sub(&rem, &rem, &b_shift);
            byte widx = (byte)(shift >> 4);
            word bidx = shift & 15;
            if (widx >= quot.Size) {
                byte i;
                for (i = quot.Size; i <= widx; i++) {
                    quot.Guts[i] = 0;
                }
                quot.Size = (byte)(widx + 1);
            }
            quot.Guts[widx] = (word)(quot.Guts[widx] | (word)(1 << bidx));
        }
        big_div2(&b_shift, &b_shift);
    }
    big_norm(&quot);
    big_norm(&rem);
    big_dup(q, &quot);
    big_dup(r, &rem);
}

void big_div(Big *q, const Big *a, const Big *b) {
    Big r;
    big_divmod(q, &r, a, b);
}

void big_mod(Big *r, const Big *a, const Big *b) {
    Big q;
    big_divmod(&q, r, a, b);
}

void big_pow(Big *z, const Big *a, const Big *b, const Big *m) {
    Big res;
    big_set_small(&res, 1);

    Big bval;
    Big q;
    big_divmod(&q, &bval, a, m);

    Big exp;
    big_dup(&exp, b);

    Big t;
    while (exp.Size > 0) {
        if ((exp.Guts[0] & 1) != 0) {
            big_mul(&t, &res, &bval);
            big_divmod(&q, &res, &t, m);
        }
        big_div2(&exp, &exp);
        if (exp.Size == 0) break;
        big_mul(&t, &bval, &bval);
        big_divmod(&q, &bval, &t, m);
    }
    big_dup(z, &res);
}

void big_pow_small(Big *z, const Big *a, word exp) {
    Big res;
    big_set_small(&res, 1);
    Big bval;
    big_dup(&bval, a);
    Big t;
    word e = exp;
    while (e > 0) {
        if ((e & 1) != 0) {
            big_mul(&t, &res, &bval);
            big_dup(&res, &t);
        }
        e = e >> 1;
        if (e == 0) break;
        big_mul(&t, &bval, &bval);
        big_dup(&bval, &t);
    }
    big_dup(z, &res);
}

void big_and(Big *z, const Big *a, const Big *b) {
    byte sz = a->Size;
    if (b->Size < sz) sz = b->Size;
    Big t;
    byte i;
    for (i = 0; i < sz; i++) {
        t.Guts[i] = (word)(a->Guts[i] & b->Guts[i]);
    }
    t.Size = sz;
    big_norm(&t);
    big_dup(z, &t);
}

void big_or(Big *z, const Big *a, const Big *b) {
    byte sz = a->Size;
    if (b->Size > sz) sz = b->Size;
    Big t;
    byte i;
    for (i = 0; i < sz; i++) {
        t.Guts[i] = (word)(big_get(a, i) | big_get(b, i));
    }
    t.Size = sz;
    big_norm(&t);
    big_dup(z, &t);
}

void big_xor(Big *z, const Big *a, const Big *b) {
    byte sz = a->Size;
    if (b->Size > sz) sz = b->Size;
    Big t;
    byte i;
    for (i = 0; i < sz; i++) {
        t.Guts[i] = (word)(big_get(a, i) ^ big_get(b, i));
    }
    t.Size = sz;
    big_norm(&t);
    big_dup(z, &t);
}

char* big_format(const Big *z, char *buf) {
    if (z->Size == 0) {
        buf[0] = '0';
        buf[1] = 0;
        return buf;
    }
    Big u;
    big_dup(&u, z);
    char digits[600];
    int dlen = 0;
    while (!big_is_zero(&u)) {
        byte rem = div10(&u, &u);
        digits[dlen] = (char)('0' + rem);
        dlen = dlen + 1;
    }
    int out_idx = 0;
    while (dlen > 0) {
        dlen = dlen - 1;
        buf[out_idx] = digits[dlen];
        out_idx = out_idx + 1;
    }
    buf[out_idx] = 0;
    return buf;
}

static char hex_digit(byte b) {
    if (b < 10) return (char)('0' + b);
    return (char)('a' + (b - 10));
}

char* big_format_hex(const Big *z, char *buf) {
    if (z->Size == 0) {
        buf[0] = '0';
        buf[1] = 0;
        return buf;
    }
    int out_idx = 0;
    word top = z->Guts[z->Size - 1];
    byte nyb3 = (byte)((top >> 12) & 0x0F);
    byte nyb2 = (byte)((top >> 8) & 0x0F);
    byte nyb1 = (byte)((top >> 4) & 0x0F);
    byte nyb0 = (byte)(top & 0x0F);

    int started = 0;
    if (nyb3 > 0) {
        buf[out_idx] = hex_digit(nyb3);
        out_idx = out_idx + 1;
        started = 1;
    }
    if (started || nyb2 > 0) {
        buf[out_idx] = hex_digit(nyb2);
        out_idx = out_idx + 1;
        started = 1;
    }
    if (started || nyb1 > 0) {
        buf[out_idx] = hex_digit(nyb1);
        out_idx = out_idx + 1;
        started = 1;
    }
    buf[out_idx] = hex_digit(nyb0);
    out_idx = out_idx + 1;

    if (z->Size > 1) {
        int step;
        for (step = 0; step < (int)z->Size - 1; step++) {
            int i = (int)z->Size - 2 - step;
            word w = z->Guts[i];
            buf[out_idx] = hex_digit((byte)((w >> 12) & 0x0F));
            out_idx = out_idx + 1;
            buf[out_idx] = hex_digit((byte)((w >> 8) & 0x0F));
            out_idx = out_idx + 1;
            buf[out_idx] = hex_digit((byte)((w >> 4) & 0x0F));
            out_idx = out_idx + 1;
            buf[out_idx] = hex_digit((byte)(w & 0x0F));
            out_idx = out_idx + 1;
        }
    }
    buf[out_idx] = 0;
    return buf;
}

void big_print(const Big *z) {
    char buf[600];
    big_format(z, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void big_println(const Big *z) {
    big_print(z);
    putchar('\n');
}

void big_print_hex(const Big *z) {
    char buf[600];
    big_format_hex(z, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void big_println_hex(const Big *z) {
    big_print_hex(z);
    putchar('\n');
}

static int hex_val(char c, word *out) {
    if (c >= '0' && c <= '9') {
        *out = (word)(c - '0');
        return 1;
    }
    if (c >= 'a' && c <= 'f') {
        *out = (word)(c - 'a' + 10);
        return 1;
    }
    if (c >= 'A' && c <= 'F') {
        *out = (word)(c - 'A' + 10);
        return 1;
    }
    return 0;
}

void big_from_hex(Big *z, const char *s) {
    big_zero(z);
    if (s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
        s = s + 2;
    }
    while (*s) {
        word v;
        if (hex_val(*s, &v)) {
            big_lsh(z, z, 4);
            Big digit;
            big_set_small(&digit, v);
            big_add(z, z, &digit);
        }
        s = s + 1;
    }
}

void big_from_string(Big *z, const char *s) {
    big_zero(z);
    if (s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
        big_from_hex(z, s);
        return;
    }
    Big ten;
    big_set_small(&ten, 10);
    Big digit;
    while (*s) {
        if (*s >= '0' && *s <= '9') {
            big_mul(z, z, &ten);
            big_set_small(&digit, (word)(*s - '0'));
            big_add(z, z, &digit);
        }
        s = s + 1;
    }
}

#endif /* BIG_C */
