extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

void main() {
    byte arr[4];
    arr[0] = 'H';
    arr[1] = 'i';
    arr[2] = '!';
    arr[3] = '\n';
    byte i;
    for (i = 0; i < 4; i = i + 1) {
        putchar(arr[i]);
    }
}
