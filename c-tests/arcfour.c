// Test ssh-arcfour.c

#include "ctlib/ssh-arcfour.h"
#include "ctlib/ssh-arcfour.c"

ArcfourContext C;

#define N 5

char cypher[N];
char plain[N];

int main() {
    arcfour_init(&C, "key", 3);
    arcfour_encrypt(&C, cypher, "plain", N);

    arcfour_init(&C, "key", 3);
    arcfour_decrypt(&C, plain, cypher, N);

    for (int i=0; i<N; i++) putchar(plain[i]);
    return 0;
}
