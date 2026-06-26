extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte sum = 0;
    byte i = 1;
    while (i <= 5) {
        sum = sum + i;
        i = i + 1;
    }
    putchar('0' + sum / 10);
    putchar('0' + sum % 10);
    putchar('\n');
}
