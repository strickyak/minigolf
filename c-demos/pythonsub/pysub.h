#ifndef PYSUB_H
#define PYSUB_H

const char* PROGRAM =
"def tri(zero, unit, n):         \n"
"    sum = zero                  \n"
"    count = zero                \n"
"    for i in range(n):          \n"
"        count = count + unit    \n"
"        sum = sum + count       \n"
"    return sum                  \n"
"                                \n"
"                                \n"
"print (tri(0, 1, 10))           \n"
"print (tri([], [[]], 10))       \n"
;

int run_python(const char *source);

#endif /* PYSUB_H */

