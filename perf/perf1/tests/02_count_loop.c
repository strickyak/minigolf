#include "../common.h"

volatile int zero;

int main(void) {
    int i;
    int limit;
    limit = 10 + zero;
    for (i = zero; i < limit; i++) {
        putchar((char)('0' + i + zero));
    }
    putchar('\n');
    return 0;
}
