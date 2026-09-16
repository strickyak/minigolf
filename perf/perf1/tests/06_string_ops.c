#include "../common.h"

static int my_strlen(const char *s) {
    const char *p;
    p = s;
    while (*p) {
        p++;
    }
    return (int)(p - s);
}

static void my_strcpy(char *dst, const char *src) {
    while (*src) {
        *dst++ = *src++;
    }
    *dst = '\0';
}

static int my_strcmp(const char *s1, const char *s2) {
    while (*s1 && (*s1 == *s2)) {
        s1++;
        s2++;
    }
    return (int)(*(const unsigned char *)s1) - (int)(*(const unsigned char *)s2);
}

int main(void) {
    char buf[32];
    const char *msg1 = "Hello, 6809 World!";
    const char *msg2 = "Hello, 6809 World!";
    const char *msg3 = "Hello, 6809!";

    put_num(my_strlen(msg1));
    putchar(' ');

    my_strcpy(buf, msg1);
    put_str(buf);
    putchar(' ');

    put_num(my_strcmp(msg1, msg2));
    putchar(' ');
    put_num(my_strcmp(msg1, msg3) > 0 ? 1 : 0);
    putchar('\n');
    return 0;
}
