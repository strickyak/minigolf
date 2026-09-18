#include "long.h"
#include "long.c"

extern void putchar(char ch);

static void print_str(const char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
    }
}

static void print_int(int v) {
    if (v < 0) {
        putchar('-');
        v = 0 - v;
    }
    if (v == 0) {
        putchar('0');
        return;
    }
    char buf[8];
    int n = 0;
    while (v > 0) {
        buf[n] = (char)('0' + (v % 10));
        n = n + 1;
        v = v / 10;
    }
    while (n > 0) {
        n = n - 1;
        putchar(buf[n]);
    }
}

int main(void) {
    print_str("--- Constants and Conversions ---\n");
    Long z = long_zero();
    print_str("zero: ");
    long_print(z);
    print_str(" is_zero=");
    print_int(long_is_zero(z));
    putchar('\n');

    Long one = long_one();
    print_str("one: ");
    long_println(one);

    Long w42 = long_from_word(42);
    print_str("w42: ");
    long_print(w42);
    print_str(" to_word=");
    print_int((int)long_to_word(w42));
    putchar('\n');

    Long wMax = long_from_word(65535);
    print_str("wMax: ");
    long_println(wMax);

    Long iNeg1 = long_from_int(-1);
    print_str("iNeg1: ");
    long_println(iNeg1);

    Long iNeg42 = long_from_int(-42);
    print_str("iNeg42: ");
    long_println(iNeg42);

    print_str("--- FromString and Format ---\n");
    Long s1 = long_from_string("123456789");
    print_str("s1: ");
    long_println(s1);

    Long sNeg1 = long_from_string("-987654321");
    print_str("sNeg1: ");
    long_println(sNeg1);

    Long maxPos = long_from_string("2147483647");
    print_str("maxPos: ");
    long_println(maxPos);

    Long minNeg = long_from_string("-2147483648");
    print_str("minNeg: ");
    long_println(minNeg);

    print_str("--- Addition ---\n");
    Long a = long_from_string("1000000");
    Long b = long_from_string("2345678");
    print_str("1000000 + 2345678 = ");
    long_println(long_add(a, b));

    Long pos5 = long_from_word(5);
    Long neg3 = long_from_int(-3);
    print_str("5 + (-3) = ");
    long_println(long_add(pos5, neg3));

    Long neg5 = long_from_int(-5);
    Long pos3 = long_from_word(3);
    print_str("(-5) + 3 = ");
    long_println(long_add(neg5, pos3));
    print_str("(-5) + (-3) = ");
    long_println(long_add(neg5, neg3));
    print_str("5 + (-5) = ");
    long_println(long_add(pos5, neg5));

    print_str("maxPos + 1 = ");
    long_println(long_add(maxPos, one));

    print_str("--- Subtraction ---\n");
    print_str("2345678 - 1000000 = ");
    long_println(long_sub(b, a));
    print_str("5 - 10 = ");
    long_println(long_sub(pos5, long_from_word(10)));
    print_str("(-5) - 3 = ");
    long_println(long_sub(neg5, pos3));
    print_str("(-5) - (-5) = ");
    long_println(long_sub(neg5, neg5));

    print_str("--- Multiplication ---\n");
    Long m1 = long_from_word(12345);
    Long m2 = long_from_word(6789);
    print_str("12345 * 6789 = ");
    long_println(long_mul(m1, m2));

    Long negM1 = long_neg(m1);
    print_str("(-12345) * 6789 = ");
    long_println(long_mul(negM1, m2));
    print_str("(-12345) * (-6789) = ");
    long_println(long_mul(negM1, long_neg(m2)));
    print_str("0 * 12345 = ");
    long_println(long_mul(z, m1));
    print_str("(-1) * (-1) = ");
    long_println(long_mul(iNeg1, iNeg1));

    print_str("--- Division and Modulo ---\n");
    Long divA = long_from_string("1000000");
    Long divB = long_from_word(3);
    print_str("1000000 / 3 = ");
    long_println(long_div(divA, divB));
    print_str("1000000 % 3 = ");
    long_println(long_mod(divA, divB));

    print_str("(-1000000) / 3 = ");
    long_println(long_div(long_neg(divA), divB));
    print_str("(-1000000) % 3 = ");
    long_println(long_mod(long_neg(divA), divB));

    print_str("1000000 / (-3) = ");
    long_println(long_div(divA, long_neg(divB)));
    print_str("1000000 % (-3) = ");
    long_println(long_mod(divA, long_neg(divB)));

    print_str("--- Comparisons ---\n");
    Long c10 = long_from_word(10);
    Long c20 = long_from_word(20);
    print_str("10 cmp 20: ");
    print_int(long_cmp(c10, c20));
    putchar('\n');
    print_str("20 cmp 20: ");
    print_int(long_cmp(c20, c20));
    putchar('\n');
    print_str("20 cmp 10: ");
    print_int(long_cmp(c20, c10));
    putchar('\n');
    print_str("(-10) cmp 10: ");
    print_int(long_cmp(long_neg(c10), c10));
    putchar('\n');
    print_str("10 lt 20: ");
    print_int(long_lt(c10, c20));
    putchar('\n');
    print_str("20 gt 10: ");
    print_int(long_gt(c20, c10));
    putchar('\n');
    print_str("10 eq 10: ");
    print_int(long_eq(c10, c10));
    putchar('\n');

    print_str("--- Bitwise and Shifts ---\n");
    Long h1 = long_from_words(0x5678, 0x1234);
    Long h2 = long_from_words(0x00FF, 0x00FF);
    print_str("h1 & h2 in hex: ");
    long_println_hex(long_and(h1, h2));
    print_str("h1 | h2 in hex: ");
    long_println_hex(long_or(h1, h2));
    print_str("h1 ^ h2 in hex: ");
    long_println_hex(long_xor(h1, h2));

    print_str("1 << 10 = ");
    long_println(long_shl(one, 10));
    print_str("1024 >> 10 = ");
    long_println(long_shru(long_shl(one, 10), 10));

    Long neg1024 = long_neg(long_shl(one, 10));
    print_str("(-1024) >> 2 = ");
    long_println(long_shr(neg1024, 2));

    print_str("ALL LONG TESTS PASSED\n");
    return 0;
}
