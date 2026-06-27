#include "minigolf.h"

#include "ctlib/string-match.c"

void putstring(const char* s) {
    while (*s) {
        putchar(*s);
        s++;
    }
}

void trial(const char* string, const char* pattern) {
    putstring(string);
    putchar(' ');
    putstring(pattern);
    putchar(' ');
    putchar('0' + Tcl_StringMatch(string, pattern));
    putchar('\n');
}

int main() {
    trial("abracadabra", "ajax*");
    trial("abracadabra", "*ajax*");
    trial("abracadabra", "*dab*");
    trial("abracadabra", "ab*");
    trial("abracadabra", "*bra");
    trial("abracadabra", "ab*bra");
    trial("abracadabra", "a??a?a??a");
    trial("abracadabra", "a??a?a?a??a");
    trial("abracadabra", "[A-Z]*");
    trial("abracadabra", "[a-z]*");
    trial("abracadabra", "*[A-Z]*");
    trial("abracadabra", "*[a-z]*");
    return 0;
}
