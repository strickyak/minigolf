#include "../common.h"

volatile int zero;

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
    int limit;
    sum = 0;
    limit = 5 + zero;
    for (i = 1 + zero; i <= limit; i++) {
        sum += compute(i * 3 + zero, i + 1 + zero, i * 5 + zero, i + 2 + zero);
    }
    put_num(sum);
    putchar('\n');
    return 0;
}
