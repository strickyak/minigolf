#ifndef LONG_C
#define LONG_C

#include "long.h"

extern void putchar(char ch);
extern void abort(void);

Long long_zero(void) {
    Long l;
    l.Bytes[0] = 0;
    l.Bytes[1] = 0;
    l.Bytes[2] = 0;
    l.Bytes[3] = 0;
    return l;
}

Long long_one(void) {
    Long l;
    l.Bytes[0] = 1;
    l.Bytes[1] = 0;
    l.Bytes[2] = 0;
    l.Bytes[3] = 0;
    return l;
}

Long long_from_word(word w) {
    Long l;
    l.Bytes[0] = (byte)(w & 0xFF);
    l.Bytes[1] = (byte)((w >> 8) & 0xFF);
    l.Bytes[2] = 0;
    l.Bytes[3] = 0;
    return l;
}

Long long_from_int(int v) {
    Long l;
    l.Bytes[0] = (byte)(v & 0xFF);
    l.Bytes[1] = (byte)((v >> 8) & 0xFF);
    if (v < 0) {
        l.Bytes[2] = 0xFF;
        l.Bytes[3] = 0xFF;
    } else if (v > 32767) {
        l.Bytes[2] = (byte)((v >> 16) & 0xFF);
        l.Bytes[3] = (byte)((v >> 24) & 0xFF);
    } else {
        l.Bytes[2] = 0;
        l.Bytes[3] = 0;
    }
    return l;
}

Long long_from_bytes(byte b0, byte b1, byte b2, byte b3) {
    Long l;
    l.Bytes[0] = b0;
    l.Bytes[1] = b1;
    l.Bytes[2] = b2;
    l.Bytes[3] = b3;
    return l;
}

Long long_from_words(word lo, word hi) {
    Long l;
    l.Bytes[0] = (byte)(lo & 0xFF);
    l.Bytes[1] = (byte)((lo >> 8) & 0xFF);
    l.Bytes[2] = (byte)(hi & 0xFF);
    l.Bytes[3] = (byte)((hi >> 8) & 0xFF);
    return l;
}

int long_is_zero(Long a) {
    return (a.Bytes[0] == 0 && a.Bytes[1] == 0 && a.Bytes[2] == 0 && a.Bytes[3] == 0);
}

int long_is_neg(Long a) {
    return (int)((a.Bytes[3] >> 7) & 1);
}

int long_cmp_u(Long a, Long b) {
    int step;
    for (step = 0; step < 4; step++) {
        int i = 3 - step;
        if (a.Bytes[i] > b.Bytes[i]) return 1;
        if (a.Bytes[i] < b.Bytes[i]) return -1;
    }
    return 0;
}

int long_cmp(Long a, Long b) {
    int signA = (int)((a.Bytes[3] >> 7) & 1);
    int signB = (int)((b.Bytes[3] >> 7) & 1);
    if (signA != signB) {
        if (signA == 1) return -1;
        return 1;
    }
    return long_cmp_u(a, b);
}

int long_eq(Long a, Long b) {
    return (a.Bytes[0] == b.Bytes[0] &&
            a.Bytes[1] == b.Bytes[1] &&
            a.Bytes[2] == b.Bytes[2] &&
            a.Bytes[3] == b.Bytes[3]);
}

int long_ne(Long a, Long b) {
    return !long_eq(a, b);
}

int long_lt(Long a, Long b) {
    return (long_cmp(a, b) < 0);
}

int long_le(Long a, Long b) {
    return (long_cmp(a, b) <= 0);
}

int long_gt(Long a, Long b) {
    return (long_cmp(a, b) > 0);
}

int long_ge(Long a, Long b) {
    return (long_cmp(a, b) >= 0);
}

Long long_add(Long a, Long b) {
    Long res;
    word carry = 0;
    int i;
    for (i = 0; i < 4; i++) {
        word sum = (word)a.Bytes[i] + (word)b.Bytes[i] + carry;
        res.Bytes[i] = (byte)(sum & 0xFF);
        carry = sum >> 8;
    }
    return res;
}

Long long_sub(Long a, Long b) {
    Long res;
    word carry = 1;
    int i;
    for (i = 0; i < 4; i++) {
        word sum = (word)a.Bytes[i] + (word)(b.Bytes[i] ^ 0xFF) + carry;
        res.Bytes[i] = (byte)(sum & 0xFF);
        carry = sum >> 8;
    }
    return res;
}

Long long_neg(Long a) {
    return long_sub(long_zero(), a);
}

Long long_abs(Long a) {
    if (long_is_neg(a)) {
        return long_neg(a);
    }
    return a;
}

Long long_mul(Long a, Long b) {
    Long res;
    res = long_zero();
    int i;
    int j;
    for (i = 0; i < 4; i++) {
        word c = 0;
        for (j = 0; j < 4 - i; j++) {
            word prod = (word)a.Bytes[i] * (word)b.Bytes[j] + (word)res.Bytes[i+j] + c;
            res.Bytes[i+j] = (byte)(prod & 0xFF);
            c = prod >> 8;
        }
    }
    return res;
}

static byte shift_left_1(Long *l) {
    byte carry = 0;
    int i;
    for (i = 0; i < 4; i++) {
        byte b = l->Bytes[i];
        byte next_carry = (byte)((b >> 7) & 1);
        l->Bytes[i] = (byte)(((b << 1) | carry) & 0xFF);
        carry = next_carry;
    }
    return carry;
}

static byte shift_right_1(Long *l, byte carry_in) {
    byte c = carry_in;
    int step;
    for (step = 0; step < 4; step++) {
        int i = 3 - step;
        byte b = l->Bytes[i];
        byte next_carry = (byte)(b & 1);
        l->Bytes[i] = (byte)((b >> 1) | (c << 7));
        c = next_carry;
    }
    return c;
}

void long_divmod_u(Long a, Long b, Long *quo, Long *rem) {
    *rem = long_zero();
    *quo = long_zero();
    int step;
    for (step = 0; step < 32; step++) {
        int i = 31 - step;
        byte carry_out = shift_left_1(rem);
        int byte_idx = i >> 3;
        byte bit_idx = (byte)(i & 7);
        byte bit_val = (byte)((a.Bytes[byte_idx] >> bit_idx) & 1);
        rem->Bytes[0] = (byte)(rem->Bytes[0] | bit_val);
        if (carry_out != 0 || long_cmp_u(*rem, b) >= 0) {
            *rem = long_sub(*rem, b);
            quo->Bytes[byte_idx] = (byte)(quo->Bytes[byte_idx] | (1 << bit_idx));
        }
    }
}

void long_divmod(Long a, Long b, Long *q, Long *r) {
    if (long_is_zero(b)) {
        abort();
    }
    int negA = long_is_neg(a);
    int negB = long_is_neg(b);

    Long uA = a;
    if (negA) {
        uA = long_neg(a);
    }
    Long uB = b;
    if (negB) {
        uB = long_neg(b);
    }

    long_divmod_u(uA, uB, q, r);

    if (negA != negB) {
        *q = long_neg(*q);
    }
    if (negA) {
        *r = long_neg(*r);
    }
}

Long long_div(Long a, Long b) {
    Long q;
    Long r;
    long_divmod(a, b, &q, &r);
    return q;
}

Long long_mod(Long a, Long b) {
    Long q;
    Long r;
    long_divmod(a, b, &q, &r);
    return r;
}

Long long_and(Long a, Long b) {
    Long res;
    int i;
    for (i = 0; i < 4; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] & b.Bytes[i]);
    }
    return res;
}

Long long_or(Long a, Long b) {
    Long res;
    int i;
    for (i = 0; i < 4; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] | b.Bytes[i]);
    }
    return res;
}

Long long_xor(Long a, Long b) {
    Long res;
    int i;
    for (i = 0; i < 4; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] ^ b.Bytes[i]);
    }
    return res;
}

Long long_not(Long a) {
    Long res;
    int i;
    for (i = 0; i < 4; i++) {
        res.Bytes[i] = (byte)(a.Bytes[i] ^ 0xFF);
    }
    return res;
}

Long long_shl(Long a, word n) {
    Long res = a;
    while (n >= 8) {
        res.Bytes[3] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[0];
        res.Bytes[0] = 0;
        n = n - 8;
    }
    while (n > 0) {
        shift_left_1(&res);
        n = n - 1;
    }
    return res;
}

Long long_shru(Long a, word n) {
    Long res = a;
    while (n >= 8) {
        res.Bytes[0] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[3];
        res.Bytes[3] = 0;
        n = n - 8;
    }
    while (n > 0) {
        shift_right_1(&res, 0);
        n = n - 1;
    }
    return res;
}

Long long_shr(Long a, word n) {
    Long res = a;
    byte sign_bit = (byte)((a.Bytes[3] >> 7) & 1);
    byte fill = 0;
    if (sign_bit) fill = 0xFF;
    while (n >= 8) {
        res.Bytes[0] = res.Bytes[1];
        res.Bytes[1] = res.Bytes[2];
        res.Bytes[2] = res.Bytes[3];
        res.Bytes[3] = fill;
        n = n - 8;
    }
    while (n > 0) {
        shift_right_1(&res, sign_bit);
        n = n - 1;
    }
    return res;
}

word long_to_word(Long a) {
    return (word)a.Bytes[0] | ((word)a.Bytes[1] << 8);
}

int long_to_int(Long a) {
    return (int)long_to_word(a);
}

char* long_format(Long a, char *buf) {
    if (long_is_zero(a)) {
        buf[0] = '0';
        buf[1] = 0;
        return buf;
    }
    int is_neg = long_is_neg(a);
    Long u = a;
    if (is_neg) {
        u = long_neg(a);
    }
    char digits[12];
    int dlen = 0;
    Long ten = long_from_word(10);
    while (!long_is_zero(u)) {
        Long q;
        Long r;
        long_divmod_u(u, ten, &q, &r);
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

static char hex_nybble(byte b) {
    if (b < 10) return (char)('0' + b);
    return (char)('a' + (b - 10));
}

char* long_format_hex(Long a, char *buf) {
    int out_idx = 0;
    int started = 0;
    int step;
    for (step = 0; step < 4; step++) {
        int i = 3 - step;
        byte hi = (byte)((a.Bytes[i] >> 4) & 0x0F);
        byte lo = (byte)(a.Bytes[i] & 0x0F);
        if (started || hi > 0) {
            buf[out_idx] = hex_nybble(hi);
            out_idx = out_idx + 1;
            started = 1;
        }
        if (started || lo > 0) {
            buf[out_idx] = hex_nybble(lo);
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

void long_print(Long a) {
    char buf[16];
    long_format(a, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void long_println(Long a) {
    long_print(a);
    putchar('\n');
}

void long_print_hex(Long a) {
    char buf[16];
    long_format_hex(a, buf);
    char *p = buf;
    while (*p) {
        putchar(*p);
        p = p + 1;
    }
}

void long_println_hex(Long a) {
    long_print_hex(a);
    putchar('\n');
}

Long long_from_string(const char *s) {
    while (*s && (*s == ' ' || *s == '\t' || *s == '\n' || *s == '\r')) {
        s = s + 1;
    }
    if (!*s) {
        return long_zero();
    }
    int is_neg = 0;
    if (*s == '-') {
        is_neg = 1;
        s = s + 1;
    } else if (*s == '+') {
        s = s + 1;
    }
    Long val = long_zero();
    Long ten = long_from_word(10);
    while (*s >= '0' && *s <= '9') {
        Long d = long_from_word((word)(*s - '0'));
        val = long_add(long_mul(val, ten), d);
        s = s + 1;
    }
    if (is_neg) {
        val = long_neg(val);
    }
    return val;
}

#endif /* LONG_C */
