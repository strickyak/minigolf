extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte i;
    for (i = 1; i <= 5; i = i + 1) {
        putchar('0' + i);
    }
    putchar('\n');
}
