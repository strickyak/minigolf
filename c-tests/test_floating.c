#include "floating.h"
#include "floating.c"

extern void putchar(char ch);

static void print_str(const char *s) {
    while (*s) {
        putchar(*s++);
    }
}

static void print_word(word w) {
    if (w == 0) {
        putchar('0');
        return;
    }
    char buf[10];
    int i = 0;
    while (w > 0) {
        buf[i++] = (char)('0' + (w % 10));
        w /= 10;
    }
    while (i > 0) {
        putchar(buf[--i]);
    }
}

static void print_int(int n) {
    if (n < 0) {
        putchar('-');
        n = 0 - n;
    }
    print_word((word)n);
}

static void print_nl(void) {
    putchar('\n');
}

int main() {
    char buf[32];

    print_str("--- Sizeof ---\n");
    print_str("sizeof: ");
    print_word((word)sizeof(FloatingPoint));
    print_nl();

    print_str("--- FromWord and ToWord ---\n");
    FloatingPoint w0 = floating_from_word(0);
    print_str("w0: "); print_str(floating_format(w0, buf)); putchar(' '); print_word(floating_to_word(w0)); print_nl();

    FloatingPoint w1 = floating_from_word(1);
    print_str("w1: "); print_str(floating_format(w1, buf)); putchar(' '); print_word(floating_to_word(w1)); print_nl();

    FloatingPoint w10 = floating_from_word(10);
    print_str("w10: "); print_str(floating_format(w10, buf)); putchar(' '); print_word(floating_to_word(w10)); print_nl();

    FloatingPoint w42 = floating_from_word(42);
    print_str("w42: "); print_str(floating_format(w42, buf)); putchar(' '); print_word(floating_to_word(w42)); print_nl();

    FloatingPoint w1000 = floating_from_word(1000);
    print_str("w1000: "); print_str(floating_format(w1000, buf)); putchar(' '); print_word(floating_to_word(w1000)); print_nl();

    FloatingPoint wMax = floating_from_word(65535);
    print_str("wMax: "); print_str(floating_format(wMax, buf)); putchar(' '); print_word(floating_to_word(wMax)); print_nl();

    print_str("--- FromInt and ToInt ---\n");
    FloatingPoint i0 = floating_from_int(0);
    print_str("i0: "); print_str(floating_format(i0, buf)); putchar(' '); print_int(floating_to_int(i0)); print_nl();

    FloatingPoint i1 = floating_from_int(1);
    print_str("i1: "); print_str(floating_format(i1, buf)); putchar(' '); print_int(floating_to_int(i1)); print_nl();

    FloatingPoint iNeg1 = floating_from_int(-1);
    print_str("iNeg1: "); print_str(floating_format(iNeg1, buf)); putchar(' '); print_int(floating_to_int(iNeg1)); print_nl();

    FloatingPoint i42 = floating_from_int(42);
    print_str("i42: "); print_str(floating_format(i42, buf)); putchar(' '); print_int(floating_to_int(i42)); print_nl();

    FloatingPoint iNeg42 = floating_from_int(-42);
    print_str("iNeg42: "); print_str(floating_format(iNeg42, buf)); putchar(' '); print_int(floating_to_int(iNeg42)); print_nl();

    FloatingPoint iPosMax = floating_from_int(32767);
    print_str("iPosMax: "); print_str(floating_format(iPosMax, buf)); putchar(' '); print_int(floating_to_int(iPosMax)); print_nl();

    FloatingPoint iNegMax = floating_from_int(-32767);
    print_str("iNegMax: "); print_str(floating_format(iNegMax, buf)); putchar(' '); print_int(floating_to_int(iNegMax)); print_nl();

    print_str("--- Addition ---\n");
    FloatingPoint a1 = floating_from_word(1);
    FloatingPoint a2 = floating_from_word(2);
    FloatingPoint a3 = floating_add(a1, a2);
    print_str("1 + 2 = "); print_str(floating_format(a3, buf)); print_nl();

    FloatingPoint a10 = floating_from_word(10);
    FloatingPoint a20 = floating_from_word(20);
    print_str("10 + 20 = "); print_str(floating_format(floating_add(a10, a20), buf)); print_nl();

    FloatingPoint pos5 = floating_from_word(5);
    FloatingPoint neg3 = floating_from_int(-3);
    print_str("5 + (-3) = "); print_str(floating_format(floating_add(pos5, neg3), buf)); print_nl();

    FloatingPoint neg5 = floating_from_int(-5);
    FloatingPoint pos3 = floating_from_word(3);
    print_str("(-5) + 3 = "); print_str(floating_format(floating_add(neg5, pos3), buf)); print_nl();

    print_str("(-5) + (-3) = "); print_str(floating_format(floating_add(neg5, neg3), buf)); print_nl();

    print_str("5 + (-5) = "); print_str(floating_format(floating_add(pos5, neg5), buf)); print_nl();

    print_str("--- Subtraction ---\n");
    print_str("5 - 3 = "); print_str(floating_format(floating_sub(pos5, pos3), buf)); print_nl();
    print_str("3 - 5 = "); print_str(floating_format(floating_sub(pos3, pos5), buf)); print_nl();
    print_str("5 - (-3) = "); print_str(floating_format(floating_sub(pos5, neg3), buf)); print_nl();
    print_str("(-5) - 3 = "); print_str(floating_format(floating_sub(neg5, pos3), buf)); print_nl();
    print_str("5 - 5 = "); print_str(floating_format(floating_sub(pos5, pos5), buf)); print_nl();

    print_str("--- Multiplication and Division ---\n");
    FloatingPoint m2 = floating_from_word(2);
    FloatingPoint m3 = floating_from_word(3);
    print_str("2 * 3 = "); print_str(floating_format(floating_mul(m2, m3), buf)); print_nl();

    FloatingPoint m10 = floating_from_word(10);
    FloatingPoint m4 = floating_from_word(4);
    print_str("10 / 4 = "); print_str(floating_format(floating_div(m10, m4), buf)); print_nl();

    FloatingPoint m1 = floating_from_word(1);
    FloatingPoint m8 = floating_from_word(8);
    print_str("1 / 8 = "); print_str(floating_format(floating_div(m1, m8), buf)); print_nl();

    print_str("--- Scanner and Formatter ---\n");
    FloatingPoint s0 = floating_scan("0.0");
    print_str("scan 0.0 = "); print_str(floating_format(s0, buf)); print_nl();

    FloatingPoint s1 = floating_scan("1.0");
    print_str("scan 1.0 = "); print_str(floating_format(s1, buf)); print_nl();

    FloatingPoint s12_5 = floating_scan("12.5");
    print_str("scan 12.5 = "); print_str(floating_format(s12_5, buf)); print_nl();

    FloatingPoint sNeg3_75 = floating_scan("-3.75");
    print_str("scan -3.75 = "); print_str(floating_format(sNeg3_75, buf)); print_nl();

    FloatingPoint s0_125 = floating_scan("0.125");
    print_str("scan 0.125 = "); print_str(floating_format(s0_125, buf)); print_nl();

    FloatingPoint s42 = floating_scan("42");
    print_str("scan 42 = "); print_str(floating_format(s42, buf)); print_nl();

    FloatingPoint sSci1 = floating_scan("1.5e2");
    print_str("scan 1.5e2 = "); print_str(floating_format(sSci1, buf)); print_nl();

    FloatingPoint sSci2 = floating_scan("1.5e-1");
    print_str("scan 1.5e-1 = "); print_str(floating_format(sSci2, buf)); print_nl();

    print_str("--- Operations with Scanned Numbers ---\n");
    FloatingPoint x = floating_scan("1.25");
    FloatingPoint y = floating_scan("2.5");
    print_str("1.25 + 2.5 = "); print_str(floating_format(floating_add(x, y), buf)); print_nl();
    print_str("2.5 - 1.25 = "); print_str(floating_format(floating_sub(y, x), buf)); print_nl();
    print_str("1.25 * 2.5 = "); print_str(floating_format(floating_mul(x, y), buf)); print_nl();
    print_str("2.5 / 1.25 = "); print_str(floating_format(floating_div(y, x), buf)); print_nl();

    return 0;
}
