#include "../common.h"

#define LIMIT 100

static char flags[LIMIT + 1];

static int sieve(void) {
    int count;
    int i;
    int k;

    for (i = 0; i <= LIMIT; i++) {
        flags[i] = 1;
    }
    flags[0] = 0;
    flags[1] = 0;

    count = 0;
    for (i = 2; i <= LIMIT; i++) {
        if (flags[i]) {
            count++;
            for (k = i + i; k <= LIMIT; k += i) {
                flags[k] = 0;
            }
        }
    }
    return count;
}

int main(void) {
    int primes;
    primes = sieve();
    put_num(primes);
    putchar('\n');
    return 0;
}
