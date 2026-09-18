#include "big.h"
#include "big.c"

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
    Big a;
    Big b;
    Big c;
    Big q;
    Big r;
    Big z;
    Big m;

    // 1. Basic SetSmall and queries
    big_set_small(&a, 0);
    print_str("a=0 IsZero: ");
    print_int(big_is_zero(&a));
    putchar('\n');
    print_str("a=0 EqSmall 0: ");
    print_int(big_eq_small(&a, 0));
    putchar('\n');
    print_str("a=0 IsEven: ");
    print_int(big_is_even(&a));
    putchar('\n');

    big_set_small(&a, 7);
    print_str("a=7 IsZero: ");
    print_int(big_is_zero(&a));
    putchar('\n');
    print_str("a=7 EqSmall 7: ");
    print_int(big_eq_small(&a, 7));
    putchar('\n');
    print_str("a=7 EqSmall 8: ");
    print_int(big_eq_small(&a, 8));
    putchar('\n');
    print_str("a=7 IsEven: ");
    print_int(big_is_even(&a));
    putchar('\n');

    big_set_small(&a, 8);
    print_str("a=8 IsEven: ");
    print_int(big_is_even(&a));
    putchar('\n');

    // 2. Addition
    big_set_small(&a, 12);
    big_set_small(&b, 34);
    big_add(&z, &a, &b);
    print_str("12 + 34 = ");
    big_println(&z);

    big_set_small(&a, 65535);
    big_set_small(&b, 1);
    big_add(&z, &a, &b);
    print_str("65535 + 1 = ");
    big_println(&z);
    print_str("65535 + 1 in hex = ");
    big_println_hex(&z);

    big_set_small(&b, 65535);
    big_add(&z, &a, &b);
    print_str("65535 + 65535 = ");
    big_println(&z);

    // In-place Add: a = a + a
    big_set_small(&a, 1);
    int step;
    for (step = 0; step < 10; step++) {
        big_add(&a, &a, &a);
    }
    print_str("2^10 by Add = ");
    big_println(&a);

    // 3. Subtraction
    big_set_small(&a, 46);
    big_set_small(&b, 12);
    big_sub(&z, &a, &b);
    print_str("46 - 12 = ");
    big_println(&z);

    big_set_small(&a, 65535);
    big_set_small(&b, 1);
    big_add(&c, &a, &b); // c = 65536
    big_sub(&z, &c, &b); // 65536 - 1
    print_str("65536 - 1 = ");
    big_println(&z);

    big_sub(&z, &a, &a); // 65535 - 65535 = 0
    print_str("65535 - 65535 = ");
    big_println(&z);

    // 4. Multiplication
    big_set_small(&a, 12);
    big_set_small(&b, 12);
    big_mul(&z, &a, &b);
    print_str("12 * 12 = ");
    big_println(&z);

    big_set_small(&a, 0);
    big_set_small(&b, 5);
    big_mul(&z, &a, &b);
    print_str("0 * 5 = ");
    big_println(&z);

    big_set_small(&a, 255);
    big_set_small(&b, 255);
    big_mul(&z, &a, &b);
    print_str("255 * 255 = ");
    big_println(&z);

    big_set_small(&a, 65535);
    big_set_small(&b, 65535);
    big_mul(&z, &a, &b);
    print_str("65535 * 65535 = ");
    big_println(&z);
    print_str("65535 * 65535 in hex = ");
    big_println_hex(&z);

    // 5. DivMod
    big_set_small(&a, 255);
    big_set_small(&b, 12);
    big_divmod(&q, &r, &a, &b);
    print_str("255 / 12 = ");
    big_println(&q);
    print_str("255 % 12 = ");
    big_println(&r);

    big_set_small(&a, 144);
    big_set_small(&b, 12);
    big_divmod(&q, &r, &a, &b);
    print_str("144 / 12 = ");
    big_println(&q);
    print_str("144 % 12 = ");
    big_println(&r);

    // Divide 65536 by 2 = 32768
    big_set_small(&b, 2);
    big_divmod(&q, &r, &c, &b);
    print_str("65536 / 2 = ");
    big_println(&q);
    print_str("65536 % 2 = ");
    big_println(&r);

    // Divide 4294836225 by 65535 = 65535
    big_set_small(&b, 65535);
    big_divmod(&q, &r, &z, &b);
    print_str("4294836225 / 65535 = ");
    big_println(&q);
    print_str("4294836225 % 65535 = ");
    big_println(&r);

    // 6. Div2 and Mul2
    big_set_small(&a, 255);
    big_div2(&z, &a);
    print_str("255 Div2 = ");
    big_println(&z);
    big_mul2(&z, &z);
    print_str("127 Mul2 = ");
    big_println(&z);

    // 7. Lsh and Rsh
    big_set_small(&a, 1);
    big_lsh(&z, &a, 10);
    print_str("1 << 10 = ");
    big_println(&z);

    big_rsh(&z, &z, 10);
    print_str("1024 >> 10 = ");
    big_println(&z);

    big_set_small(&a, 1);
    big_lsh(&z, &a, 64);
    print_str("1 << 64 in hex = ");
    big_println_hex(&z);
    print_str("1 << 64 = ");
    big_println(&z);

    // 8. Comparisons
    big_set_small(&a, 10);
    big_set_small(&b, 20);
    print_str("10 cmp 20: ");
    print_int(big_cmp(&a, &b));
    putchar('\n');
    print_str("20 cmp 20: ");
    print_int(big_cmp(&b, &b));
    putchar('\n');
    print_str("20 cmp 10: ");
    print_int(big_cmp(&b, &a));
    putchar('\n');
    print_str("10 lt 20: ");
    print_int(big_lt(&a, &b));
    putchar('\n');
    print_str("10 gt 20: ");
    print_int(big_gt(&a, &b));
    putchar('\n');
    print_str("10 eq 20: ");
    print_int(big_eq(&a, &b));
    putchar('\n');

    // 9. Modular Exponentiation
    big_set_small(&a, 2);
    big_set_small(&b, 10);
    big_set_small(&m, 1000);
    big_pow(&z, &a, &b, &m);
    print_str("2^10 mod 1000 = ");
    big_println(&z);

    big_set_small(&m, 10000);
    big_pow(&z, &a, &b, &m);
    print_str("2^10 mod 10000 = ");
    big_println(&z);

    // 10. PowSmall
    big_set_small(&a, 2);
    big_pow_small(&z, &a, 32);
    print_str("2^32 = ");
    big_println(&z);

    big_pow_small(&z, &a, 64);
    print_str("2^64 = ");
    big_println(&z);

    big_pow_small(&z, &a, 100);
    print_str("2^100 = ");
    big_println(&z);

    // 11. FromString and FromHex
    big_from_string(&a, "12345678901234567890");
    print_str("FromString 12345678901234567890 = ");
    big_println(&a);
    print_str("in hex = ");
    big_println_hex(&a);

    big_from_hex(&b, "0xAB54A98CEB1F0AD2");
    print_str("FromHex 0xAB54A98CEB1F0AD2 = ");
    big_println(&b);
    print_str("FromString eq FromHex: ");
    print_int(big_eq(&a, &b));
    putchar('\n');

    // 12. Bitwise operations
    big_set_small(&a, 0x1234);
    big_set_small(&b, 0x5678);
    big_and(&z, &a, &b);
    print_str("0x1234 & 0x5678 in hex = ");
    big_println_hex(&z);

    big_or(&z, &a, &b);
    print_str("0x1234 | 0x5678 in hex = ");
    big_println_hex(&z);

    big_xor(&z, &a, &b);
    print_str("0x1234 ^ 0x5678 in hex = ");
    big_println_hex(&z);

    print_str("ALL TESTS PASSED\n");
    return 0;
}
