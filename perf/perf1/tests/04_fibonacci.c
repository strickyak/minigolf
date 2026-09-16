#include "../common.h"

static int fib(int n) {
    if (n <= 1) {
        return n;
    }
    return fib(n - 1) + fib(n - 2);
}

int main(void) {
    int val;
    val = fib(10);
    put_num(val);
    putchar('\n');
    return 0;
}
