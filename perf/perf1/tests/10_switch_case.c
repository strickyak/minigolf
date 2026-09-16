#include "../common.h"

struct Instr {
    int op;
    int arg;
};

static struct Instr program[] = {
    { 7, 100 },  /* SET 100 */
    { 1,  50 },  /* ADD  50 -> 150 */
    { 2,  25 },  /* SUB  25 -> 125 */
    { 3,   3 },  /* MUL   3 -> 375 */
    { 4,   5 },  /* DIV   5 ->  75 */
    { 5,  17 },  /* MOD  17 ->   7 */
    { 6,   0 },  /* NEG     ->  -7 */
    { 1,  49 },  /* ADD  49 ->  42 */
    { 0,   0 }   /* HALT */
};

static int run_vm(const struct Instr *prog) {
    int acc;
    int pc;
    acc = 0;
    pc = 0;
    while (1) {
        int op;
        int arg;
        op = prog[pc].op;
        arg = prog[pc].arg;
        pc++;
        if (op == 0) {
            break;
        }
        switch (op) {
            case 1: acc += arg; break;
            case 2: acc -= arg; break;
            case 3: acc *= arg; break;
            case 4: acc /= arg; break;
            case 5: acc %= arg; break;
            case 6: acc = -acc; break;
            case 7: acc = arg;  break;
            default: break;
        }
    }
    return acc;
}

int main(void) {
    int result;
    result = run_vm(program);
    put_num(result);
    putchar('\n');
    return 0;
}
