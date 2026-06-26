#include <assert.h>

typedef unsigned char bool;
typedef unsigned char byte;
typedef unsigned int uint;

typedef unsigned char gbool;
typedef unsigned char gbyte;
typedef unsigned int gword;



typedef __builtin_va_list __gnuc_va_list;
typedef __gnuc_va_list va_list;


void Jprintf(const char* format, ...);
void putchar(int);
void exit(int);


void assert_6809(bool b, const char* s, uint n) {
    if (b) return;
    Jprintf("\nASSERT FAILS at %s:%d\n", s, n);
    assert(b);
}

char* PPutStr(char* p, const char* s) {
  int max = 80;
  for (; *s; s++) {
    do { *p++ = (*s); } while (0);
    if (max-- <= 0) {
      do { *p++ = ('\\'); } while (0);
      return p;
    }
  }
  return p;
}

const char PHexAlphabet[] = "0123456789ABCDEF";

char* PPutHex(char* p, gword x) {
  if (x > 15u) {
    p = PPutHex(p, x >> 4u);
  }
  do { *p++ = (PHexAlphabet[15u & x]); } while (0);
  return p;
}

gbyte PDivMod10(gword x, gword* out_div) {
  gword div = 0;
  while (x >= 10000) x -= 10000, div += 1000;
  while (x >= 1000) x -= 1000, div += 100;
  while (x >= 100) x -= 100, div += 10;
  while (x >= 10) x -= 10, div++;
  *out_div = div;
  return (gbyte)x;
}
char* PPutDec(char* p, gword x) {
  gword div;
  if (x > 9u) {

    PDivMod10(x, &div);
    p = PPutDec(p, div);
  }

  do { *p++ = ('0' + PDivMod10(x, &div)); } while (0);
  return p;
}

char* PPutSigned(char* p, int x) {
  if (x < 0) {
    x = -x;
    do { *p++ = ('-'); } while (0);
  }
  return PPutDec(p, x);
}

char* Vprintf(char* p, const char* format, va_list ap) {
  int max = 80;

  for (const char* s = format; *s; s++) {
    if (max-- <= 0) {
      do { *p++ = ('\\'); } while (0);
      break;
    }

    if (*s < ' ') {
      do { *p++ = ('\n'); } while (0);
    } else if (*s != '%') {
      do { *p++ = (*s); } while (0);
    } else {
      s++;
      switch (*s) {
        case 'd': {
          int x = __builtin_va_arg(ap,int);
          p = PPutSigned(p, x);
        } break;
        case 'u': {
          gword x = __builtin_va_arg(ap,gword);
          p = PPutDec(p, x);
        } break;
        case 'x': {
          gword x = __builtin_va_arg(ap,gword);
          p = PPutHex(p, x);
        } break;
        case 's': {
          char* x = __builtin_va_arg(ap,char*);
          p = PPutStr(p, x);
        } break;
        default:
          do { *p++ = (*s); } while (0);
      };
    }
  }
  *p = '\0';
  return p;
}
char* Sprintf(char* p, const char* format, ...) {
  va_list ap;
  __builtin_va_start(ap,format);
  return Vprintf(p, format, ap);
}

void Jprintf(const char* format, ...) {
  static char buffer[256];
  va_list ap;
  __builtin_va_start(ap,format);
  Vprintf(buffer, format, ap);
  for (const char* p = buffer; *p; p++) {
      putchar(*p);
  }
}

typedef struct bin {
    byte size;
    byte guts[250];
} *Bin;

void Small(Bin z, uint a) {

    assert_6809(a < 256,"bin.c",9);
    z->size = (a > 0);
    z->guts[0] = (byte)a;
}

bool EqSmall(Bin z, uint a) {
    if (z->size == 0) return (a==0);





    if (z->guts[0] != 1) return 0;
    for (byte i = 1; i < z->size; i++) {
        if (z->guts[i] != 0) return 0;
    }

    return 1;
}

bool IsEven(Bin a) {

    if (a->size == 0) return 1;
    return (a->guts[0] & 1) == 0;
}

void Dup(Bin z, Bin a) {

    byte sz = a->size;
    z->size = sz;
    for (byte i = 0; i < sz; i++) {
        z->guts[i] = a->guts[i];
    }
}





void Add(Bin z, Bin a, Bin b) {

    byte max = ((a->size) > (b->size) ? (a->size) : (b->size));
    byte carry = 0;
    for (byte i = 0; i < max; i++) {
        uint t = ((i)<(a)->size ? (a)->guts[i] : 0u) + ((i)<(b)->size ? (b)->guts[i] : 0u) + carry;
        carry = (t>255);
        z->guts[i] = (byte)t;
    }
    z->size = max;
    if (carry) {
        z->guts[max] = 1;
        z->size++;
    }
    assert_6809(z->size <= 250,"bin.c",62);
}

int Cmp(Bin a, Bin b) {

    byte max = ((a->size) > (b->size) ? (a->size) : (b->size));
    if (max==0) return 0;

    byte i = max-1;
    while(1) {
        if (((i)<(a)->size ? (a)->guts[i] : 0u) < ((i)<(b)->size ? (b)->guts[i] : 0u)) return -1;
        if (((i)<(a)->size ? (a)->guts[i] : 0u) > ((i)<(b)->size ? (b)->guts[i] : 0u)) return +1;

        if (i==0) break;
        i--;
    }
    return 0;
}

void Halve(Bin z, Bin a) {

    z->size = a->size;
    if (a->size==0) return;

    byte i = a->size-1;
    byte carry = 0;
    while(1) {
        z->guts[i] = a->guts[i] >> 1;
        if (carry) z->guts[i] |= 0x80u;
        carry = 0x01u & a->guts[i];

        if (i==0) break;
        i--;
    }
    if (z->guts[z->size-1]==0) z->size--;
}

void Print(Bin a) {
    Jprintf("$");
    if (!a->size) {
        Jprintf("0 ");
        return;
    }
    byte i = a->size-1;
    while(1) {
        Jprintf("%02x", a->guts[i]);

        if (i==0) break;
        i--;
    }
    Jprintf(" ");
}

typedef struct dec {
    uint size;
    byte guts[3 * 250];
} *Dec;

void SmallDec(Dec z, uint a) {
    assert_6809(a < 10,"dec.c",8);
    z->size = (a > 0);
    z->guts[0] = (byte)a;
}

void AddDec(Dec z, Dec a, Dec b) {
    byte max = ((a->size) > (b->size) ? (a->size) : (b->size));
    byte carry = 0;
    for (byte i = 0; i < max; i++) {
        byte t = ((i)<(a)->size ? (a)->guts[i] : 0u) + ((i)<(b)->size ? (b)->guts[i] : 0u) + carry;
        carry = (t>9);
        z->guts[i] = (t > 9) ? t-10 : t;
    }
    z->size = max;
    if (carry) {
        z->guts[max] = 1;
        z->size++;
    }
    assert_6809(z->size <= 250,"dec.c",26);
}

void PrintDec(Dec a) {

    if (!a->size) {
        Jprintf("0 ");
        return;
    }
    byte i = a->size-1;
    while(1) {
        putchar( '0' + a->guts[i]);

        if (i==0) break;
        i--;
    }
    putchar(' ');
}

struct dec Power2;
void BinToDec(Dec z, Bin a) {
    SmallDec(&Power2, 1);
    SmallDec(z, 0);

    for (byte i = 0; i < a->size; i++) {
        for (byte bit = 1; bit; bit<<=1) {
            if (a->guts[i] & bit) {
                AddDec(z, z, &Power2);
            }
            AddDec(&Power2, &Power2, &Power2);
        }
    }
}



void PrintBinAsDec(Bin a) {
    static struct dec dec;
    BinToDec(&dec, a);
    PrintDec(&dec);
}

struct bin q;
struct bin biggest;
uint max_steps;

int Traverse(Bin q, Bin one) {
    static struct bin a;
    static struct bin temp;

    Jprintf("Start ");
    PrintBinAsDec(q);
    Jprintf("\n");

    Dup(&a, q);

    if (Cmp(&a, &biggest) > 0) {
            Dup(&biggest, &a);
            Jprintf("biggest  ");
    }

    uint steps = 0;
    while (! EqSmall(&a, 1)) {
        steps++;

        if (IsEven(&a)) {
            Halve(&temp, &a);
            Dup(&a , &temp);
        } else {
            Add(&temp, &a, &a);
            Add(&a, &a, &temp);
            Add(&a, &a, one);
        }
        Jprintf("step %d: ", steps);
        PrintBinAsDec(&a);
        Jprintf("\n");

        if (Cmp(&a, &biggest) > 0) {
            Dup(&biggest, &a);
            Jprintf(" biggest  ");
        }

    }
    if (0 && steps > max_steps) {
        max_steps = steps;
        Jprintf("\nOKAY (MAX steps=%u)\n", steps);
    } else {
        Jprintf("\nOKAY (steps=%u)  ", steps);
        PrintBinAsDec(q);
        Jprintf("\n");
    }
    return 0;
}
byte RandSeed;
void SRAND() {}
byte RAND() {

    RandSeed += 151;
    return RandSeed;
}

struct bin topic, one;

int main() {
    SRAND();
    Small(&one, 1);

    topic.size = 3;
    for (uint i = 0; i < 3; i++) {
        topic.guts[i] = (byte) RAND();
    }
    while (topic.guts[3 -1] == 0) {
        topic.guts[3 -1] = (byte) RAND();
    }
    return Traverse(&topic, &one);
}
