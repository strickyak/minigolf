extern void putchar(char ch);
typedef unsigned char byte;
typedef unsigned long word;

typedef struct { byte x; byte y; } Point;

Point make_point(byte x, byte y) {
    Point p;
    p.x = x;
    p.y = y;
    return p;
}

void main() {
    Point p = make_point(3, 7);
    putchar('0' + p.x);
    putchar('0' + p.y);
    putchar('\n');
}
