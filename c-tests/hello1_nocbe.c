extern void putchar(char ch);

int main() {
    const char* s = "Hello\nWorld\n";
    while (*s) {
        putchar(*s);
        s++;
    }
}
