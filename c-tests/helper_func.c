extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void print_char(byte c) {
    putchar(c);
}

void say_hi() {
    print_char('H');
    print_char('i');
    print_char('!');
    print_char('\n');
}

void main() {
    say_hi();
}
