#include "math.h"
#include "math.c"

extern void putchar(char ch);

static void print_str(const char *s) {
    while (*s) {
        putchar(*s++);
    }
}

static void print_nl(void) {
    putchar('\n');
}

int main() {
    char buf[32];
    char buf2[32];

    print_str("--- Constants ---\n");
    print_str("Pi: "); print_str(floating_format(math_pi(), buf)); print_nl();
    print_str("PiOverTwo: "); print_str(floating_format(math_pi_over_two(), buf)); print_nl();
    print_str("Tau: "); print_str(floating_format(math_tau(), buf)); print_nl();

    print_str("--- Sqrt Zero and One ---\n");
    FloatingPoint zero = floating_from_word(0);
    print_str("sqrt(0) = "); print_str(floating_format(math_sqrt(zero), buf)); print_nl();

    FloatingPoint one = floating_from_word(1);
    print_str("sqrt(1) = "); print_str(floating_format(math_sqrt(one), buf)); print_nl();

    print_str("--- Sqrt Exact Squares ---\n");
    FloatingPoint four = floating_from_word(4);
    print_str("sqrt(4) = "); print_str(floating_format(math_sqrt(four), buf)); print_nl();

    FloatingPoint nine = floating_from_word(9);
    print_str("sqrt(9) = "); print_str(floating_format(math_sqrt(nine), buf)); print_nl();

    FloatingPoint sixteen = floating_from_word(16);
    print_str("sqrt(16) = "); print_str(floating_format(math_sqrt(sixteen), buf)); print_nl();

    FloatingPoint twentyfive = floating_from_word(25);
    print_str("sqrt(25) = "); print_str(floating_format(math_sqrt(twentyfive), buf)); print_nl();

    FloatingPoint hundred = floating_from_word(100);
    print_str("sqrt(100) = "); print_str(floating_format(math_sqrt(hundred), buf)); print_nl();

    print_str("--- Sqrt Fractions ---\n");
    FloatingPoint pt25 = floating_scan("0.25");
    print_str("sqrt(0.25) = "); print_str(floating_format(math_sqrt(pt25), buf)); print_nl();

    FloatingPoint pt0625 = floating_scan("0.0625");
    print_str("sqrt(0.0625) = "); print_str(floating_format(math_sqrt(pt0625), buf)); print_nl();

    print_str("--- Sqrt Irrational ---\n");
    FloatingPoint two = floating_from_word(2);
    print_str("sqrt(2) = "); print_str(floating_format(math_sqrt(two), buf)); print_nl();

    print_str("--- Hypot ---\n");
    FloatingPoint three = floating_from_word(3);
    print_str("hypot(3, 4) = "); print_str(floating_format(math_hypot(three, four), buf)); print_nl();

    print_str("--- Trigonometry ---\n");
    FloatingPoint s0, c0;
    math_sincos(zero, &s0, &c0);
    print_str("sin(0) = "); print_str(floating_format(s0, buf));
    print_str(" cos(0) = "); print_str(floating_format(c0, buf2)); print_nl();

    FloatingPoint pi2 = math_pi_over_two();
    FloatingPoint sPi2, cPi2;
    math_sincos(pi2, &sPi2, &cPi2);
    print_str("sin(pi/2) = "); print_str(floating_format(sPi2, buf));
    print_str(" cos(pi/2) = "); print_str(floating_format(cPi2, buf2)); print_nl();

    FloatingPoint pi = math_pi();
    FloatingPoint sPi, cPi;
    math_sincos(pi, &sPi, &cPi);
    print_str("sin(pi) = "); print_str(floating_format(sPi, buf));
    print_str(" cos(pi) = "); print_str(floating_format(cPi, buf2)); print_nl();

    print_str("--- Atan and Atan2 ---\n");
    print_str("atan(0) = "); print_str(floating_format(math_atan(zero), buf)); print_nl();
    print_str("atan(1) = "); print_str(floating_format(math_atan(one), buf)); print_nl();
    print_str("atan2(0, 1) = "); print_str(floating_format(math_atan2(zero, one), buf)); print_nl();
    print_str("atan2(1, 0) = "); print_str(floating_format(math_atan2(one, zero), buf)); print_nl();
    FloatingPoint negOne = floating_from_int(-1);
    print_str("atan2(0, -1) = "); print_str(floating_format(math_atan2(zero, negOne), buf)); print_nl();
    print_str("atan2(-1, 0) = "); print_str(floating_format(math_atan2(negOne, zero), buf)); print_nl();

    return 0;
}
