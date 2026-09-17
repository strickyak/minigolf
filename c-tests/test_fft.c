#include "fft.h"
#include "fft.c"

extern void putchar(char ch);

static void print_str(const char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
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
        buf[i] = (char)('0' + (w % 10));
        i = i + 1;
        w = w / 10;
    }
    while (i > 0) {
        i = i - 1;
        putchar(buf[i]);
    }
}

static void print_nl(void) {
    putchar('\n');
}

static Complex make_real(word re) {
    return complex_new(floating_from_word(re), floating_from_word(0));
}

static void print_vector(const char *label, const Complex *v, int n) {
    char buf[32];
    print_str(label);
    print_nl();
    int i;
    for (i = 0; i < n; i = i + 1) {
        print_word((word)i);
        putchar(' ');
        print_str(complex_format(v[i], buf));
        print_nl();
    }
}

int main() {
    print_str("--- Test 1: Impulse (N=4) ---\n");
    Complex impulse[4];
    Complex fImpulse[4];
    Complex rImpulse[4];
    impulse[0] = make_real(1);
    impulse[1] = make_real(0);
    impulse[2] = make_real(0);
    impulse[3] = make_real(0);

    fft_forward(impulse, 4, fImpulse);
    print_vector("forward(impulse):", fImpulse, 4);

    fft_reverse(fImpulse, 4, rImpulse);
    print_vector("reverse(fImpulse):", rImpulse, 4);

    print_str("--- Test 2: Sequence [1, 2, 3, 4] ---\n");
    Complex seq[4];
    Complex fSeq[4];
    Complex rSeq[4];
    seq[0] = make_real(1);
    seq[1] = make_real(2);
    seq[2] = make_real(3);
    seq[3] = make_real(4);

    fft_forward(seq, 4, fSeq);
    print_vector("forward(seq):", fSeq, 4);

    fft_reverse(fSeq, 4, rSeq);
    print_vector("reverse(fSeq):", rSeq, 4);

    print_str("--- Test 3: Constant (N=8) ---\n");
    Complex constSig[8];
    Complex fConst[8];
    Complex rConst[8];
    int i;
    for (i = 0; i < 8; i = i + 1) {
        constSig[i] = make_real(1);
    }
    fft_forward(constSig, 8, fConst);
    print_vector("forward(const):", fConst, 8);

    fft_reverse(fConst, 8, rConst);
    print_vector("reverse(fConst):", rConst, 8);

    return 0;
}
