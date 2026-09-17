#include "complex.h"
#include "complex.c"

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

static void print_nl(void) {
    putchar('\n');
}

int main() {
    char buf[32];
    char buf2[32];

    print_str("--- Sizeof ---\n");
    print_str("sizeof Complex: ");
    print_word((word)sizeof(Complex));
    print_nl();

    print_str("--- New and Format ---\n");
    FloatingPoint re1 = floating_from_word(1);
    FloatingPoint im2 = floating_from_word(2);
    Complex z1 = complex_new(re1, im2);
    print_str("z1: "); print_str(complex_format(z1, buf)); print_nl();

    FloatingPoint re3 = floating_from_int(3);
    FloatingPoint imNeg4 = floating_from_int(-4);
    Complex z2 = complex_new(re3, imNeg4);
    print_str("z2: "); print_str(complex_format(z2, buf)); print_nl();

    print_str("--- Add and Sub ---\n");
    Complex zAdd = complex_add(z1, z2);
    print_str("z1 + z2 = "); print_str(complex_format(zAdd, buf)); print_nl();

    Complex zSub = complex_sub(z1, z2);
    print_str("z1 - z2 = "); print_str(complex_format(zSub, buf)); print_nl();

    print_str("--- Neg and Conj ---\n");
    print_str("-z1 = "); print_str(complex_format(complex_neg(z1), buf)); print_nl();
    print_str("conj(z2) = "); print_str(complex_format(complex_conj(z2), buf)); print_nl();

    print_str("--- Mul ---\n");
    Complex zMul = complex_mul(z1, z2);
    print_str("z1 * z2 = "); print_str(complex_format(zMul, buf)); print_nl();

    FloatingPoint im4 = floating_from_word(4);
    Complex z3 = complex_new(re3, im4);
    Complex zMul2 = complex_mul(z1, z3);
    print_str("z1 * z3 = "); print_str(complex_format(zMul2, buf)); print_nl();

    print_str("--- Div ---\n");
    Complex zDiv = complex_div(zMul2, z3);
    print_str("zMul2 / z3 = "); print_str(complex_format(zDiv, buf)); print_nl();

    print_str("--- AbsSq and Abs ---\n");
    FloatingPoint absSq = complex_abs_sq(z3);
    print_str("|z3|^2 = "); print_str(floating_format(absSq, buf)); print_nl();
    print_str("|z3| = "); print_str(floating_format(complex_abs(z3), buf)); print_nl();

    print_str("--- Rect and Polar ---\n");
    FloatingPoint r2 = floating_from_word(2);
    FloatingPoint zero = floating_from_word(0);
    Complex zRect1 = complex_rect(r2, zero);
    print_str("rect(2, 0) = "); print_str(complex_format(zRect1, buf)); print_nl();

    FloatingPoint pi2 = math_pi_over_two();
    Complex zRect2 = complex_rect(r2, pi2);
    print_str("rect(2, pi/2) = "); print_str(complex_format(zRect2, buf)); print_nl();

    FloatingPoint rPol, thPol;
    complex_polar(zRect2, &rPol, &thPol);
    print_str("polar(zRect2): r = "); print_str(floating_format(rPol, buf));
    print_str(" theta = "); print_str(floating_format(thPol, buf2)); print_nl();

    return 0;
}
