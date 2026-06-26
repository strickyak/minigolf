extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void check(byte n) {
    if (n < 3) {
        putchar('L');
    } else if (n == 3) {
        putchar('M');
    } else {
        putchar('H');
    }
}

void main() {
    check(1);
    check(3);
    check(7);
    putchar('\n');
}
