extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void print_str(char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
    }
}

void main() {
    print_str("Hello!\n");
    print_str("Bye!\n");
}
