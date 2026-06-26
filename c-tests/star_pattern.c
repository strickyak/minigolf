extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte i;
    byte j;
    for (i = 1; i <= 3; i = i + 1) {
        for (j = 0; j < i; j = j + 1) {
            putchar('*');
        }
        putchar('\n');
    }
}
