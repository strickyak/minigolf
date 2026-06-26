extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte x = 7;
    byte *p = &x;
    *p = *p + 1;
    putchar('0' + x);
    putchar('\n');
}
