extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

byte counter = 0;

void inc() {
    counter = counter + 1;
}

void main() {
    inc();
    inc();
    inc();
    putchar('0' + counter);
    putchar('\n');
}
