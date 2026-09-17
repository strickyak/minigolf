#include "fft.h"
#include "fft.c"

extern void putchar(char ch);
extern void abort(void);

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

static FloatingPoint table[64];
static Complex in[64];
static Complex fwd[64];
static Complex inv[64];

int main() {
    char buf[32];
    FloatingPoint tau = math_tau();
    FloatingPoint n64 = floating_from_word(64);
    FloatingPoint zero = floating_from_word(0);

    int i;
    for (i = 0; i < 64; i = i + 1) {
        FloatingPoint fi = floating_from_word((word)i);
        FloatingPoint theta = floating_div(floating_mul(fi, tau), n64);
        FloatingPoint s = math_sin(theta);
        table[i] = s;
        in[i] = complex_new(s, zero);
    }

    print_str("--- Wave Table ---\n");
    for (i = 0; i < 64; i = i + 1) {
        print_word((word)i);
        putchar(' ');
        print_str(floating_format(table[i], buf));
        print_nl();
    }

    fft_forward(in, 64, fwd);
    print_str("--- Forward FFT (64 points) ---\n");
    for (i = 0; i < 64; i = i + 1) {
        print_word((word)i);
        putchar(' ');
        print_str(complex_format(fwd[i], buf));
        print_nl();
    }

    fft_reverse(fwd, 64, inv);
    print_str("--- Inverse FFT (64 points) ---\n");
    for (i = 0; i < 64; i = i + 1) {
        print_word((word)i);
        putchar(' ');
        print_str(complex_format(inv[i], buf));
        print_nl();
    }

    print_str("--- Checking difference ---\n");
    FloatingPoint maxDiff = zero;
    for (i = 0; i < 64; i = i + 1) {
        Complex diff = complex_sub(inv[i], in[i]);
        FloatingPoint err = complex_abs(diff);
        if (floating_cmp(err, maxDiff) > 0) {
            maxDiff = err;
        }
    }

    print_str("max error: ");
    print_str(floating_format(maxDiff, buf));
    print_nl();

    FloatingPoint thresh = floating_scan("0.001");
    if (floating_cmp(maxDiff, thresh) > 0) {
        abort();
    }
    print_str("PASS: Inverse is very close to original input\n");

    return 0;
}
