#ifndef LONGLONG_C
#define LONGLONG_C

#include "longlong.h"

extern void putchar(char ch);
extern void abort(void);

LongLong longlong_zero(void) {
    LongLong l;
    int i;
    for (i = 0; i < 8; i++) {
        l.Bytes[i] = 0;
    }
    return l;
}

LongLong longlong_one(void) {
    LongLong l = longlong_zero();
    l.Bytes[0] = 1;
    return l;
}

LongLong longlong_from_word(word w) {
    LongLong l = longlong_zero();
    l.Bytes[0] = (byte)(w & 0xFF);
    l.Bytes[1] = (byte)((w >> 8) & 0xFF);
    return l;
}

LongLong longlong_from_int(int v) {
    LongLong l = longlong_zero();
    l.Bytes[0] = (byte)(v & 0xFF);
    l.Bytes[1] = (byte)((v >> 8) & 0xFF);
    if (v < 0) {
        int i;
        for (i = 2; i < 8; i++) {
            l.Bytes[i] = 0xFF;
        }
    } else if (v > 32767) {
        l.Bytes[2] = (byte)((v >> 16) & 0xFF);
        l.Bytes[3] = (byte)((v >> 24) & 0xFF);
    }
    return l;
}

LongLong longlong_from_long(Long a) {
    LongLong l;
    l.Bytes[0] = a.Bytes[0];
    l.Bytes[1] = a.Bytes[1];
    l.Bytes[2] = a.Bytes[2];
    l.Bytes[3] = a.Bytes[3];
    byte fill = 0;
    if ((a.Bytes[3] >> 7) & 1) {
        fill = 0xFF;
    }
    int i;
    for (i = 4; i < 8; i++) {
        l.Bytes[i] = fill;
    }
    return l;
}

Long longlong_to_long(LongLong a) {
    Long l;
    l.Bytes[0] = a.Bytes[0];
    l.Bytes[1] = a.Bytes[1];
    l.Bytes[2] = a.Bytes[2];
    l.Bytes[3] = a.Bytes[3];
    return l;
}

int longlong_is_zero(LongLong a) {
    int i;
    for (i = 0; i < 8; i++) {
        if (a.Bytes[i] != 0) return 0;
    }
    return 1;
}

int longlong_is_neg(LongLong a) {
    return (int)((a.Bytes[7] >> 7) & 1);
}

int longlong_cmp_u(LongLong a, LongLong b) {
    int step;
    for (step = 0; step < 8; step++) {
        int i = 7 - step;
        if (a.Bytes[i] > b.Bytes[i]) return 1;
        if (a.Bytes[i] < b.Bytes[i]) return -1;
    }
    return 0;
}

int longlong_cmp(LongLong a, LongLong b) {
    int signA = (int)((a.Bytes[7] >> 7) & 1);
    int signB = (int)((b.Bytes[7] >> 7) & 1);
    if (signA != signB) {
        if (signA == 1) return -1;
        return 1;
    }
    return longlong_cmp_u(a, b);
}

int longlong_eq(LongLong a, LongLong b) {
    int i;
    for (i = 0; i < 8; i++) {
        if (a.Bytes[i] != b.Bytes[i]) return 0;
    }
    return 1;
}

int longlong_ne(LongLong a, LongLong b) {
    return !longlong_eq(a, b);
}

int longlong_lt(LongLong a, LongLong b) {
    return (longlong_cmp(a, b) < 0);
}

int longlong_le(LongLong a, LongLong b) {
    return (longlong_cmp(a, b) <= 0);
}

int longlong_gt(LongLong a, LongLong b) {
    return (longlong_cmp(a, b) > 0);
}

int longlong_ge(LongLong a, LongLong b) {
    return (longlong_cmp(a, b) >= 0);
}

LongLong longlong_add(LongLong a, LongLong b) {
    LongLong res;
    word carry = 0;
    int i;
    for (i = 0; i < 8; i++) {
        word sum = (word)a.Bytes[i] + (word)b.Bytes[i] + carry;
        res.Bytes[i] = (byte)(sum & 0xFF);
        carry = sum >> 8;
    }
    return res;
}

LongLong longlong_sub(LongLong a, LongLong b) {
    LongLong res;
    word carry = 1;
    int i;
    for (i = 0; i < 8; i++) {
        word sum = (word)a.Bytes[i] + (word)(b.Bytes[i] ^ 0xFF) + carry;
        res.Bytes[i] = (byte)(sum & 0xFF);
        carry = sum >> 8;
    }
    return res;
}

LongLong longlong_neg(LongLong a) {
    return longlong_sub(longlong_zero(), a);
}

LongLong longlong_abs(LongLong a) {
    if (longlong_is_neg(a)) {
        return longlong_neg(a);
    }
    return a;
}

LongLong longlong_mul(LongLong a, LongLong b) {
    LongLong res = longlong_zero();
    int i;
    int j;
    for (i = 0; i < 8; i++) {
        word c = 0;
        for (j = 0; j < 8 - i; j++) {
            word prod = (word)a.Bytes[i] * (word)b.Bytes[j] + (word)res.Bytes[i+j] + c;
            res.Bytes[i+j] = (byte)(prod & 0xFF);
            c = prod >> 8;
        }
    }
    return res;
}

static byte shift_left_1_ll(LongLong *l) {
    byte carry = 0;
    int i;
    for (i = 0; i < 8; i++) {
        byte b = l->Bytes[i];
        byte next_carry = (byte)((b >> 7) & 1);
        l->Bytes[i] = (byte)(((b << 1) | carry) & 0xFF);
        carry = next_carry;
    }
    return carry;
}

static byte shift_right_1_ll(LongLong *l, byte carry_in) {
    byte c = carry_in;
    int step;
    for (step = 0; step < 8; step++) {
        int i = 7 - step;
        byte b = l->Bytes[i];
        byte next_carry = (byte)(b & 1);
        l->Bytes[i] = (byte)((b >> 1) | (c << 7));
        c = next_carry;
    }
    return c;
}

void longlong_divmod_u(LongLong a, LongLong b, LongLong *quo, LongLong *rem) {
    *rem = longlong_zero();
    *quo = longlong_zero();
    int step;
    for (step = 0; step < 64; step++) {
        int i = 63 - step;
        byte carry_out = shift_left_1_ll(rem);
        int byte_idx = i >> 3;
        byte bit_idx = (byte)(i & 7);
        byte bit_val = (byte)((a.Bytes[byte_idx] >> bit_idx) & 1);
        rem->Bytes[0] = (byte)(rem->Bytes[0] | bit_val);
        if (carry_out != 0 || longlong_cmp_u(*rem, b) >= 0) {
            *rem = longlong_sub(*rem, b);
            quo->Bytes[byte_idx] = (byte)(quo->Bytes[byte_idx] | (1 << bit_idx));
        }
    }
}

void longlong_divmod(LongLong a, LongLong b, LongLong *q, LongLong *r) {
    if (longlong_is_zero(b)) {
        abort();
    }
    int negA = longlong_is_neg(a);
    int negB = longlong_is_neg(b);

    LongLong uA = a;
    if (negA) {
        uA = longlong_neg(a);
    }
    LongLong uB = b;
    if (negB) {
        uB = longlong_neg(b);
    }

    longlong_divmod_u(uA, uB, q, r);

    if (negA != negB) {
        *q = longlong_neg(*q);
    }
    if (negA) {
        *r = longlong_neg(*r);
    }
}

LongLong longlong_div(LongLong a, LongLong b) {
    LongLong q;
    LongLong r;
    longlong_divmod(a, b, &q, &r);
    return q;
}

LongLong longlong_mod(LongLong a, LongLong b) {
    LongLong q;
    LongLong r;
    longlong_divmod(a, b, &q, &r);
    return r;
}

LongLong longlong_and(LongLong a, LongLong b) {
    LongLong res;
    int i;
    for (i = 0; i < 8; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] & b.Bytes[i]);
    }
    return res;
}

LongLong longlong_or(LongLong a, LongLong b) {
    LongLong res;
    int i;
    for (i = 0; i < 8; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] | b.Bytes[i]);
    }
    return res;
}

LongLong longlong_xor(LongLong a, LongLong b) {
    LongLong res;
    int i;
    for (i = 0; i < 8; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] ^ b.Bytes[i]);
    }
    return res;
}

LongLong longlong_not(LongLong a) {
    LongLong res;
    int i;
    for (i = 0; i < 8; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] ^ 0xFF);
    }
    return res;
}

LongLong longlong_shl(LongLong a, word n) {
    LongLong res = a;
    while (n >= 8) {
        res.Bytes[7] = res.Bytes[6];
        res.Bytes[6] = res.Bytes[5];
        res.Bytes[5] = res.Bytes[4];
        res.Bytes[4] = res.Bytes[3];
        res.Bytes[3] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[0];
        res.Bytes[0] = 0;
        n = n - 8;
    }
    while (n > 0) {
        shift_left_1_ll(&res);
        n = n - 1;
    }
    return res;
}

LongLong longlong_shru(LongLong a, word n) {
    LongLong res = a;
    while (n >= 8) {
        res.Bytes[0] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[3];
        res.Bytes[3] = res.Bytes[4];
        res.Bytes[4] = res.Bytes[5];
        res.Bytes[5] = res.Bytes[6];
        res.Bytes[6] = res.Bytes[7];
        res.Bytes[7] = 0;
        n = n - 8;
    }
    while (n > 0) {
        shift_right_1_ll(&res, 0);
        n = n - 1;
    }
    return res;
}

LongLong longlong_shr(LongLong a, word n) {
    LongLong res = a;
    byte sign_bit = (byte)((a.Bytes[7] >> 7) & 1);
    byte fill = 0;
    if (sign_bit) fill = 0xFF;
    while (n >= 8) {
        res.Bytes[0] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[3];
        res.Bytes[3] = res.Bytes[4];
        res.Bytes[4] = res.Bytes[5];
        res.Bytes[5] = res.Bytes[6];
        res.Bytes[6] = res.Bytes[7];
        res.Bytes[7] = fill;
        n = n - 8;
    }
    while (n > 0) {
        shift_right_1_ll(&res, sign_bit);
        n = n - 1;
    }
    return res;
}

word longlong_to_word(LongLong a) {
    return (word)a.Bytes[0] | ((word)a.Bytes[1] << 8);
}

int longlong_to_int(LongLong a) {
    return (int)longlong_to_word(a);
}

char* longlong_format(LongLong a, char *buf) {
    if (longlong_is_zero(a)) {
        buf[0] = '0';
        buf[1] = 0;
        return buf;
    }
    int is_neg = longlong_is_neg(a);
    LongLong u = a;
    if (is_neg) {
        u = longlong_neg(a);
    }
    char digits[24];
    int dlen = 0;
    LongLong ten = longlong_from_word(10);
    while (!longlong_is_zero(u)) {
        LongLong q;
        LongLong r;
        longlong_divmod_u(u, ten, &q, &r);
        digits[dlen] = (char)('0' + r.Bytes[0]);
        dlen = dlen + 1;
        u = q;
    }
    int out_idx = 0;
    if (is_neg) {
        buf[out_idx] = '-';
        out_idx = out_idx + 1;
    }
    while (dlen > 0) {
        dlen = dlen - 1;
        buf[out_idx] = digits[dlen];
        out_idx = out_idx + 1;
    }
    buf[out_idx] = 0;
    return buf;
}

static char hex_nybble_ll(byte b) {
    if (b < 10) return (char)('0' + b);
    return (char)('a' + (b - 10));
}

char* longlong_format_hex(LongLong a, char *buf) {
    int out_idx = 0;
    int started = 0;
    int step;
    for (step = 0; step < 8; step++) {
        int i = 7 - step;
        byte hi = (byte)((a.Bytes[i] >> 4) & 0x0F);
        byte lo = (byte)(a.Bytes[i] & 0x0F);
        if (started || hi > 0) {
            buf[out_idx] = hex_nybble_ll(hi);
            out_idx = out_idx + 1;
            started = 1;
        }
        if (started || lo > 0) {
            buf[out_idx] = hex_nybble_ll(lo);
            out_idx = out_idx + 1;
            started = 1;
        }
    }
    if (!started) {
        buf[out_idx] = '0';
        out_idx = out_idx + 1;
    }
    buf[out_idx] = 0;
    return buf;
}

void longlong_print(LongLong a) {
    char buf[28];
    longlong_format(a, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void longlong_println(LongLong a) {
    longlong_print(a);
    putchar('\n');
}

void longlong_print_hex(LongLong a) {
    char buf[28];
    longlong_format_hex(a, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void longlong_println_hex(LongLong a) {
    longlong_print_hex(a);
    putchar('\n');
}

LongLong longlong_from_string(const char *s) {
    while (*s && (*s == ' ' || *s == '\t' || *s == '\n' || *s == '\r')) {
        s = s + 1;
    }
    if (!*s) {
        return longlong_zero();
    }
    int is_neg = 0;
    if (*s == '-') {
        is_neg = 1;
        s = s + 1;
    } else if (*s == '+') {
        s = s + 1;
    }
    LongLong val = longlong_zero();
    LongLong ten = longlong_from_word(10);
    while (*s >= '0' && *s <= '9') {
        LongLong d = longlong_from_word((word)(*s - '0'));
        val = longlong_add(longlong_mul(val, ten), d);
        s = s + 1;
    }
    if (is_neg) {
        val = longlong_neg(val);
    }
    return val;
}

#endif /* LONGLONG_C */
