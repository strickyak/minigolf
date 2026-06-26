extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte c;
    for (c = 'A'; c <= 'E'; c = c + 1) {
        putchar(c);
    }
    putchar('\n');
}
