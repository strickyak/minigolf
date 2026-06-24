// N is the number of binary bytes (i.e. base 256 digits).
//--------- #define N 42 // around 101 decimal digits.
#define N 3 // around 33.7 decimal digits.

/////////////////////////////////////
#include "types.h"
#include "format.h"
#include "bin.c"
#include "dec.c"

// struct bin a, b, c, d, e, f, g, h;

void PrintBinAsDec(Bin a) {
    static struct dec dec;
    BinToDec(&dec, a);
    PrintDec(&dec);
}

struct bin q; // the question
struct bin biggest;
uint max_steps;

int Traverse(Bin q, Bin one) {
    static struct bin a;
    static struct bin temp;

    printf("Start ");
    PrintBinAsDec(q);
    printf("\n");

    Dup(&a, q);

    if (Cmp(&a, &biggest) > 0) {
            Dup(&biggest, &a);
            printf("biggest  ");
    }

    uint steps = 0;
    while (! EqSmall(&a, 1)) {
        steps++;

        if (IsEven(&a)) {
            Halve(&temp, &a); // half
            Dup(&a , &temp);
        } else {
            Add(&temp, &a, &a);  // double
            Add(&a, &a, &temp); // triple
            Add(&a, &a, one); // triple+1
        }
        printf("step %d: ", steps);
        PrintBinAsDec(&a);
        printf("\n");

        if (Cmp(&a, &biggest) > 0) {
            Dup(&biggest, &a);
            printf(" biggest  ");
        }

    }
    if (false && steps > max_steps) {
        max_steps = steps;
        printf("\nOKAY (MAX steps=%u)\n", steps);
    } else {
        printf("\nOKAY (steps=%u)  ", steps);
        PrintBinAsDec(q);
        printf("\n");
    }
    return 0;
}

#if unix

#include "time.h"
#include "stdlib.h"
//#define RAND() random()
//#define SRAND() srandom((unsigned)time((time_t*)0))

#else

#define PICO_IO_BASE 0xFF00
#define LED_PORT     (PICO_IO_BASE+4)
//#define RAND_PORT    (PICO_IO_BASE+5)
//#define RAND()       (*(volatile byte*)RAND_PORT)
//#define SRAND()

#endif

byte RandSeed;
void SRAND() {}
byte RAND() {
    // Sufficint for generating an arbitrary bignum.
    RandSeed += 151;
    return RandSeed;
}

struct bin topic, one;

int main() {
    SRAND();
    Small(&one, 1);

    topic.size = N;
    for (uint i = 0; i < N; i++) {
        topic.guts[i] = (byte) RAND();
    }
    while (topic.guts[N-1] == 0) {
        topic.guts[N-1] = (byte) RAND();
    }
    return Traverse(&topic, &one);
}
