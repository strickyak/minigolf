#include "../common.h"

static int compute(int a, int b, int c, int d) {
    int x;
    int y;
    x = a * b + c;
    y = x / d - (a % b);
    return y;
}

int main(void) {
    int i;
    int sum;
    sum = 0;
    for (i = 1; i <= 5; i++) {
        sum += compute(i * 3, i + 1, i * 5, i + 2);
    }
    put_num(sum);
    putchar('\n');
    return 0;
}
