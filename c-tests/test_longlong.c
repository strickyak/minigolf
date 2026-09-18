#include "long.h"
#include "long.c"
#include "longlong.h"
#include "longlong.c"

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
    LongLong z = longlong_zero();
    print_str("zero: ");
    longlong_print(z);
    print_str(" is_zero=");
    print_int(longlong_is_zero(z));
    putchar('\n');

    LongLong one = longlong_one();
    print_str("one: ");
    longlong_println(one);

    LongLong w42 = longlong_from_word(42);
    print_str("w42: ");
    longlong_print(w42);
    print_str(" to_word=");
    print_int((int)longlong_to_word(w42));
    putchar('\n');

    LongLong wMax = longlong_from_word(65535);
    print_str("wMax: ");
    longlong_println(wMax);

    LongLong iNeg1 = longlong_from_int(-1);
    print_str("iNeg1: ");
    longlong_println(iNeg1);

    LongLong iNeg42 = longlong_from_int(-42);
    print_str("iNeg42: ");
    longlong_println(iNeg42);

    Long lVal = long_from_string("2000000000");
    LongLong llFromL = longlong_from_long(lVal);
    print_str("fromLong: ");
    longlong_println(llFromL);

    Long lValNeg = long_from_string("-2000000000");
    LongLong llFromLNeg = longlong_from_long(lValNeg);
    print_str("fromLongNeg: ");
    longlong_println(llFromLNeg);

    Long backToLong = longlong_to_long(llFromL);
    print_str("toLong: ");
    long_println(backToLong);

    Long backToLongNeg = longlong_to_long(llFromLNeg);
    print_str("toLongNeg: ");
    long_println(backToLongNeg);

    print_str("--- FromString and Format ---\n");
    LongLong s1 = longlong_from_string("123456789012345678");
    print_str("s1: ");
    longlong_println(s1);

    LongLong sNeg1 = longlong_from_string("-987654321098765432");
    print_str("sNeg1: ");
    longlong_println(sNeg1);

    LongLong maxPos = longlong_from_string("9223372036854775807");
    print_str("maxPos: ");
    longlong_println(maxPos);

    LongLong minNeg = longlong_from_string("-9223372036854775808");
    print_str("minNeg: ");
    longlong_println(minNeg);

    print_str("--- Addition ---\n");
    LongLong a = longlong_from_string("1000000000000");
    LongLong b = longlong_from_string("2345678000000");
    print_str("1000000000000 + 2345678000000 = ");
    longlong_println(longlong_add(a, b));

    LongLong pos5 = longlong_from_word(5);
    LongLong neg3 = longlong_from_int(-3);
    print_str("5 + (-3) = ");
    longlong_println(longlong_add(pos5, neg3));

    LongLong neg5 = longlong_from_int(-5);
    LongLong pos3 = longlong_from_word(3);
    print_str("(-5) + 3 = ");
    longlong_println(longlong_add(neg5, pos3));
    print_str("(-5) + (-3) = ");
    longlong_println(longlong_add(neg5, neg3));
    print_str("5 + (-5) = ");
    longlong_println(longlong_add(pos5, neg5));

    print_str("maxPos + 1 = ");
    longlong_println(longlong_add(maxPos, one));

    print_str("--- Subtraction ---\n");
    print_str("2345678000000 - 1000000000000 = ");
    longlong_println(longlong_sub(b, a));
    print_str("5 - 10 = ");
    longlong_println(longlong_sub(pos5, longlong_from_word(10)));
    print_str("(-5) - 3 = ");
    longlong_println(longlong_sub(neg5, pos3));
    print_str("(-5) - (-5) = ");
    longlong_println(longlong_sub(neg5, neg5));

    print_str("--- Multiplication ---\n");
    LongLong m1 = longlong_from_string("12345678");
    LongLong m2 = longlong_from_string("87654321");
    print_str("12345678 * 87654321 = ");
    longlong_println(longlong_mul(m1, m2));

    LongLong negM1 = longlong_neg(m1);
    print_str("(-12345678) * 87654321 = ");
    longlong_println(longlong_mul(negM1, m2));
    print_str("(-12345678) * (-87654321) = ");
    longlong_println(longlong_mul(negM1, longlong_neg(m2)));
    print_str("0 * 12345678 = ");
    longlong_println(longlong_mul(z, m1));
    print_str("(-1) * (-1) = ");
    longlong_println(longlong_mul(iNeg1, iNeg1));

    print_str("--- Division and Modulo ---\n");
    LongLong divA = longlong_from_string("1000000000000000");
    LongLong divB = longlong_from_word(7);
    print_str("1000000000000000 / 7 = ");
    longlong_println(longlong_div(divA, divB));
    print_str("1000000000000000 % 7 = ");
    longlong_println(longlong_mod(divA, divB));

    print_str("(-1000000000000000) / 7 = ");
    longlong_println(longlong_div(longlong_neg(divA), divB));
    print_str("(-1000000000000000) % 7 = ");
    longlong_println(longlong_mod(longlong_neg(divA), divB));

    print_str("1000000000000000 / (-7) = ");
    longlong_println(longlong_div(divA, longlong_neg(divB)));
    print_str("1000000000000000 % (-7) = ");
    longlong_println(longlong_mod(divA, longlong_neg(divB)));

    print_str("--- Comparisons ---\n");
    LongLong c10 = longlong_from_word(10);
    LongLong c20 = longlong_from_word(20);
    print_str("10 cmp 20: ");
    print_int(longlong_cmp(c10, c20));
    putchar('\n');
    print_str("20 cmp 20: ");
    print_int(longlong_cmp(c20, c20));
    putchar('\n');
    print_str("20 cmp 10: ");
    print_int(longlong_cmp(c20, c10));
    putchar('\n');
    print_str("(-10) cmp 10: ");
    print_int(longlong_cmp(longlong_neg(c10), c10));
    putchar('\n');
    print_str("10 lt 20: ");
    print_int(longlong_lt(c10, c20));
    putchar('\n');
    print_str("20 gt 10: ");
    print_int(longlong_gt(c20, c10));
    putchar('\n');
    print_str("10 eq 10: ");
    print_int(longlong_eq(c10, c10));
    putchar('\n');

    print_str("--- Bitwise and Shifts ---\n");
    LongLong h1 = longlong_from_string("1311768467294899695"); // 0x123456789abcdef
    LongLong h2 = longlong_from_string("71777214294589695");   // 0x00ff00ff00ff00ff
    print_str("h1 & h2 in hex: ");
    longlong_println_hex(longlong_and(h1, h2));
    print_str("h1 | h2 in hex: ");
    longlong_println_hex(longlong_or(h1, h2));
    print_str("h1 ^ h2 in hex: ");
    longlong_println_hex(longlong_xor(h1, h2));

    print_str("1 << 40 = ");
    longlong_println(longlong_shl(one, 40));
    print_str("(1 << 40) >> 40 = ");
    longlong_println(longlong_shru(longlong_shl(one, 40), 40));

    LongLong neg1024 = longlong_neg(longlong_shl(one, 10));
    print_str("(-1024) >> 2 = ");
    longlong_println(longlong_shr(neg1024, 2));

    print_str("ALL LONGLONG TESTS PASSED\n");
    return 0;
}
