#include "../common.h"

static int g_data[10] = { 64, 34, 25, 12, 22, 11, 90, 88, 76, 42 };

static void bubble_sort(int *arr, int n) {
    int i;
    int j;
    int temp;
    for (i = 0; i < n - 1; i++) {
        for (j = 0; j < n - i - 1; j++) {
            if (arr[j] > arr[j + 1]) {
                temp = arr[j];
                arr[j] = arr[j + 1];
                arr[j + 1] = temp;
            }
        }
    }
}

static void print_array(int *arr, int n) {
    int i;
    for (i = 0; i < n; i++) {
        put_num(arr[i]);
        if (i < n - 1) {
            putchar(' ');
        }
    }
    putchar('\n');
}

int main(void) {
    bubble_sort(g_data, 10);
    print_array(g_data, 10);
    return 0;
}
