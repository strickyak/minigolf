#include "../common.h"

static int g_arr[10] = { 10, 20, 30, 40, 50, 60, 70, 80, 90, 100 };

static int sum_indexed(int *arr, int len) {
    int i;
    int s;
    s = 0;
    for (i = 0; i < len; i++) {
        s += arr[i];
    }
    return s;
}

static int sum_pointers(int *arr, int len) {
    int *p;
    int *end;
    int s;
    p = arr;
    end = arr + len;
    s = 0;
    while (p < end) {
        s += *p++;
    }
    return s;
}

int main(void) {
    int s1;
    int s2;
    s1 = sum_indexed(g_arr, 10);
    s2 = sum_pointers(g_arr, 10);
    put_num(s1);
    putchar(' ');
    put_num(s2);
    putchar('\n');
    return 0;
}
