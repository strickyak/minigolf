#include <stdarg.h>

typedef unsigned char byte;

extern void putchar(byte b);
extern void exit(int status);

#define NULL ((void*)0)

/* --------------- freestanding / MiniGolf memory shims ---------------
 * On MiniGolf the prelude provides malloc, zalloc (zeroing malloc), and free.
 * calloc(nmemb, size) maps to zalloc(nmemb * size) — same zeroing semantics.
 * Declare them here so the compiler knows their signatures under -ffreestanding.
 */
extern void* malloc(unsigned int nbytes);
extern void* zalloc(unsigned int nbytes);
extern void  free(void* ptr);

/* calloc: allocate nmemb*size zeroed bytes, just like zalloc on MiniGolf. */
static void* calloc(unsigned int nmemb, unsigned int size) {
    return zalloc(nmemb * size);
}

/* --------------- freestanding stdio stubs ----------------------------
 * stdin is only used as a stream token passed to fgets(); our fake fgets()
 * ignores the stream argument entirely, so NULL is a safe stand-in.
 * FILE / fopen / fclose are only used in loadAndRunFile() which is excluded
 * from freestanding / M6809 builds via HAVE_FILE_IO guard below.
 */
#define stdin NULL

//#include <stdio.h>
//#include <stdlib.h>
//#include <string.h>
//#include <ctype.h>
//#include <stdint.h>


#define PROGRAM_SIZE 200
#define VAR_COUNT 26
#define MAX_LINE_LEN 256
#define FOR_STACK_SIZE 10
#define MAX_STR_LEN 64

// Direct raw string print
void print_str(const char* s) {
    while (*s) {
        putchar((byte)*s++);
    }
}

// Direct 16-bit integer print
void print_int(int val) {
    if (val < 0) {
        putchar('-');
        val = -val;
    }
    char buf[6]; // Max 5 digits for 16-bit int + null
    int idx = 0;
    if (val == 0) buf[idx++] = '0';
    while (val > 0) {
        buf[idx++] = '0' + (val % 10);
        val /= 10;
    }
    while (idx > 0) {
        putchar((byte)buf[--idx]);
    }
}

// Fixed-argument emulator for the interpreter error blocks
void print_error_msg(const char* msg, const char* arg) {
    print_str("? ");
    print_str(msg);
    if (arg) {
        print_str(": ");
        print_str(arg);
    }
    print_str("\n");
}

// Explicit string formatting for the GOTO engine (replaces sprintf)
void int_to_str(char* dest, int val) {
    char buf[6];
    int idx = 0;
    int is_neg = 0;
    if (val < 0) {
        is_neg = 1;
        val = -val;
    }
    if (val == 0) buf[idx++] = '0';
    while (val > 0) {
        buf[idx++] = '0' + (val % 10);
        val /= 10;
    }
    char* p = dest;
    if (is_neg) *p++ = '-';
    while (idx > 0) {
        *p++ = buf[--idx];
    }
    *p = '\0';
}

//////////////////////////////////

// A shared core engine to format 16-bit integers and strings into a callback
void core_vsprintf(void (*out_cb)(char, void*), void* ctx, const char* format, va_list args) {
    while (*format) {
        if (*format == '%' && *(format + 1) != '\0') {
            format++;
            if (*format == 'd') {
                int val = (int)va_arg(args, int);
                if (val < 0) {
                    out_cb('-', ctx);
                    val = -val;
                }
                char buf[6];
                int idx = 0;
                if (val == 0) buf[idx++] = '0';
                while (val > 0) {
                    buf[idx++] = '0' + (val % 10);
                    val /= 10;
                }
                while (idx > 0) {
                    out_cb(buf[--idx], ctx);
                }
            } else if (*format == 's') {
                char* s = va_arg(args, char*);
                while (*s) {
                    out_cb(*s++, ctx);
                }
            }
        } else {
            out_cb(*format, ctx);
        }
        format++;
    }
}

// sprintf context and wrapper
static void sprintf_cb(char c, void* ctx) {
    char** p = (char**)ctx;
    **p = c;
    (*p)++;
}

int sprintf(char* str, const char* format, ...) {
    va_list args;
    char* ptr = str;
    va_start(args, format);
    core_vsprintf(sprintf_cb, &ptr, format, args);
    *ptr = '\0';
    va_end(args);
    return (int)(ptr - str);
}

// printf context and wrapper
static void printf_cb(char c, void* ctx) {
    (void)ctx;
    putchar((byte)c);
}

int printf(const char* format, ...) {
    va_list args;
    va_start(args, format);
    core_vsprintf(printf_cb, NULL, format, args);
    va_end(args);
    return 0;
}

/////////////////////////////////////////

// ROM table acting as your hardware keyboard/file reader input buffer
const char* input_lines[] = {
    "5 LET N$ = \"THE TOTAL SUM IS: \"",
    "10 DIM A(5)",
    "20 FOR I = 0 TO 4",
    "30 LET A(I) = I * 10",
    "40 NEXT I",
    "50 LET S = A(1) + A(2) + A(3)",
    "60 PRINT N$; S",
    "RUN",
    "QUIT"
};

#define INPUT_LINE_COUNT (sizeof(input_lines) / sizeof(input_lines[0]))
static unsigned int virtual_input_index = 0;

char* fgets(char* str, int num, void* stream) {
    (void)stream;
    if (virtual_input_index >= INPUT_LINE_COUNT) return NULL;

    const char* line = input_lines[virtual_input_index++];
    int i = 0;
    while (i < num - 1 && line[i] != '\0') {
        str[i] = line[i];
        i++;
    }
    str[i] = '\0';
    return str;
}

// Safe string copy layer replacing strncpy
void mini_strncpy(char* dest, const char* src, int max_len) {
    int i = 0;
    while (i < max_len - 1 && src[i] != '\0') {
        dest[i] = src[i];
        i++;
    }
    dest[i] = '\0';
}

int atoi(const char* str) {
    int res = 0;
    int sign = 1;
    while (*str == ' ') str++;
    if (*str == '-') { sign = -1; str++; }
    else if (*str == '+') str++;
    while (*str >= '0' && *str <= '9') {
        res = res * 10 + (*str - '0');
        str++;
    }
    return sign * res;
}

int strncmp(const char* s1, const char* s2, unsigned int n) {
    if (n == 0) return 0;
    while (n-- > 0) {
        if (*s1 != *s2++) return *(unsigned char*)s1 - *(unsigned char*)--s2;
        if (*s1++ == '\0') break;
    }
    return 0;
}

unsigned int strlen(const char* str) {
    unsigned int len = 0;
    while (str[len]) len++;
    return len;
}

unsigned int strcspn(const char* s, const char* reject) {
    unsigned int count = 0;
    while (s[count]) {
        for (int i = 0; reject[i]; i++) {
            if (s[count] == reject[i]) return count;
        }
        count++;
    }
    return count;
}

int toupper(int c) { return (c >= 'a' && c <= 'z') ? c - 32 : c; }
int isdigit(int c) { return (c >= '0' && c <= '9'); }
int isupper(int c) { return (c >= 'A' && c <= 'Z'); }

void* memset(void* s, int c, unsigned int n) {
    unsigned char* p = s;
    while (n--) *p++ = (unsigned char)c;
    return s;
}

/////////////////

typedef struct {
    int lineNumber;
    char text[MAX_LINE_LEN];
} ProgramLine;

typedef struct {
    int varIdx;
    int targetValue;
    int stepValue;
    int loopLineNumber; 
} ForLoopState;

// Storage arrays for new features
ProgramLine program[PROGRAM_SIZE];
int programCount = 0;
int variables[VAR_COUNT];
char stringVariables[VAR_COUNT][MAX_STR_LEN]; 
int* arrays[VAR_COUNT] = {NULL};           
int arraySizes[VAR_COUNT] = {0};           

char* currentLinePtr = NULL;
int isRunning = 0;
ForLoopState forStack[FOR_STACK_SIZE];
int forStackPtr = 0;
int currentExecutingLineNum = -1;

void executeLine(char* p);
int evaluateExpression(char** s);

void printError(const char* msg) {
    printf("? %s\n", msg);
    isRunning = 0;
}

void cleanArrays(void) {
    for (int i = 0; i < VAR_COUNT; i++) {
        if (arrays[i] != NULL) {
            free(arrays[i]);
            arrays[i] = NULL;
        }
        arraySizes[i] = 0;
    }
}

int findLineIndex(int num) {
    for (int i = 0; i < programCount; i++) {
        if (program[i].lineNumber == num) return i;
    }
    return -1;
}

int findInsertionIndex(int num) {
    for (int i = 0; i < programCount; i++) {
        if (program[i].lineNumber >= num) return i;
    }
    return programCount;
}

void deleteLine(int num) {
    int idx = findLineIndex(num);
    if (idx != -1) {
        for (int i = idx; i < programCount - 1; i++) {
            program[i] = program[i+1];
        }
        programCount--;
    }
}

void insertLine(int num, char* text) {
    int idx = findLineIndex(num);
    if (idx != -1) {
        mini_strncpy(program[idx].text, text, MAX_LINE_LEN - 1);
        program[idx].text[MAX_LINE_LEN - 1] = '\0';
    } else {
        if (programCount >= PROGRAM_SIZE) {
            printError("Program memory full");
            return;
        }
        int insIdx = findInsertionIndex(num);
        for (int i = programCount; i > insIdx; i--) {
            program[i] = program[i-1];
        }
        program[insIdx].lineNumber = num;
        mini_strncpy(program[insIdx].text, text, MAX_LINE_LEN - 1);
        program[insIdx].text[MAX_LINE_LEN - 1] = '\0';
        programCount++;
    }
}

void doList(void) {
    for (int i = 0; i < programCount; i++) {
        printf("%d %s\n", program[i].lineNumber, program[i].text);
    }
}

void doRun(void) {
    isRunning = 1;
    forStackPtr = 0;
    int i = 0;
    while (i < programCount && isRunning) {
        currentLinePtr = program[i].text;
        currentExecutingLineNum = program[i].lineNumber;
        executeLine(currentLinePtr);
        if (!isRunning) break;
        if (currentLinePtr == program[i].text) {
            i++;
        } else {
            int nextLineNum = (int)atoi(currentLinePtr);
            i = findLineIndex(nextLineNum);
            if (i == -1) { printError("Undefined Line Number"); break; }
        }
    }
    isRunning = 0;
}

int parseFactor(char** s) {
    while (**s == ' ') (*s)++;
    if (isdigit((unsigned char)**s)) {
        int val = 0;
        while (isdigit((unsigned char)**s)) { val = val * 10 + (**s - '0'); (*s)++; }
        return val;
    } else if (isupper((unsigned char)**s)) {
        int varIdx = **s - 'A';
        (*s)++;
        while (**s == ' ') (*s)++;
        if (**s == '(') { // Array element lookup element syntax: A(X)
            (*s)++;
            int idx = evaluateExpression(s);
            if (**s == ')') (*s)++;
            if (arrays[varIdx] == NULL || idx < 0 || idx >= arraySizes[varIdx]) {
                printError("Array Index Out of Bounds");
                return 0;
            }
            return arrays[varIdx][idx];
        }
        return variables[varIdx];
    } else if (**s == '(') {
        (*s)++;
        int val = evaluateExpression(s);
        if (**s == ')') (*s)++;
        return val;
    }
    return 0;
}

int parseTerm(char** s) {
    int val = parseFactor(s);
    while (**s == ' ' || **s == '*' || **s == '/') {
        char op = **s;
        if (op == ' ') { (*s)++; continue; }
        (*s)++;
        int nextVal = parseFactor(s);
        if (op == '*') val *= nextVal;
        else if (op == '/') {
            if (nextVal == 0) { printError("Division by Zero"); return 0; }
            val /= nextVal;
        }
    }
    return val;
}

int evaluateExpression(char** s) {
    int val = parseTerm(s);
    while (**s == ' ' || **s == '+' || **s == '-') {
        char op = **s;
        if (op == ' ') { (*s)++; continue; }
        (*s)++;
        int nextVal = parseTerm(s);
        if (op == '+') val += nextVal;
        else if (op == '-') val -= nextVal;
    }
    return val;
}

int evaluateCondition(char** s) {
    int val1 = evaluateExpression(s);
    while (**s == ' ') (*s)++;
    char op = **s;
    (*s)++;
    if (**s == '=') { (*s)++; }
    while (**s == ' ') (*s)++;
    int val2 = evaluateExpression(s);
    if (op == '=') return val1 == val2;
    if (op == '<') return val1 < val2;
    if (op == '>') return val1 > val2;
    return 0;
}
void executeLine(char* p) {
    while (*p == ' ') p++;
    if (!*p) return;

    if (strncmp(p, "DIM", 3) == 0) {
        p += 3;
        while (*p == ' ') p++;
        if (isupper((unsigned char)*p)) {
            int varIdx = *p - 'A';
            p++;
            while (*p == ' ') p++;
            if (*p == '(') {
                p++;
                int size = evaluateExpression(&p);
                if (*p == ')') p++;
                if (size <= 0) { printError("Invalid Dimension Size"); return; }
                if (arrays[varIdx] != NULL) free(arrays[varIdx]);
                arrays[varIdx] = (int*)calloc(size, sizeof(int));
                arraySizes[varIdx] = size;
            }
        }
    }
    else if (strncmp(p, "PRINT", 5) == 0) {
        p += 5;
        int omitNewline = 0;
        while (*p == ' ') p++;
        while (*p != '\0' && *p != ';') {
            if (*p == '"') {
                p++;
                while (*p && *p != '"') { putchar(*p); p++; }
                if (*p == '"') p++;
            } else if (isupper((unsigned char)*p) && *(p+1) == '$') {
                int varIdx = *p - 'A';
                printf("%s", stringVariables[varIdx]);
                p += 2;
            } else {
                int val = evaluateExpression(&p);
                printf("%d", val);
            }
            while (*p == ' ') p++;
        }
        if (*p == ';') { omitNewline = 1; p++; }
        if (!omitNewline) printf("\n");
    }
    else if (strncmp(p, "LET", 3) == 0) {
        p += 3;
        while (*p == ' ') p++;
        if (isupper((unsigned char)*p)) {
            int varIdx = *p - 'A';
            if (*(p+1) == '$') { // String allocation handler
                p += 2;
                while (*p == ' ') p++;
                if (*p == '=') {
                    p++;
                    while (*p == ' ') p++;
                    if (*p == '"') {
                        p++;
                        int i = 0;
                        while (*p && *p != '"' && i < MAX_STR_LEN - 1) {
                            stringVariables[varIdx][i++] = *p;
                            p++;
                        }
                        stringVariables[varIdx][i] = '\0';
                        if (*p == '"') p++;
                    }
                }
            } else { // Int or array location assignment
                p++;
                int isArrayAssign = 0;
                int arrayIdx = 0;
                while (*p == ' ') p++;
                if (*p == '(') {
                    p++;
                    arrayIdx = evaluateExpression(&p);
                    if (*p == ')') p++;
                    isArrayAssign = 1;
                }
                while (*p == ' ') p++;
                if (*p == '=') {
                    p++;
                    int rhs = evaluateExpression(&p);
                    if (isArrayAssign) {
                        if (arrays[varIdx] == NULL || arrayIdx < 0 || arrayIdx >= arraySizes[varIdx]) {
                            printError("Array Index Out of Bounds");
                        } else { arrays[varIdx][arrayIdx] = rhs; }
                    } else { variables[varIdx] = rhs; }
                }
            }
        }
    }
    else if (strncmp(p, "FOR", 3) == 0) {
        p += 3;
        while (*p == ' ') p++;
        if (isupper((unsigned char)*p)) {
            int varIdx = *p - 'A';
            p++;
            while (*p == ' ') p++;
            if (*p == '=') {
                p++;
                int startVal = evaluateExpression(&p);
                while (*p == ' ') p++;
                if (strncmp(p, "TO", 2) == 0) {
                    p += 2;
                    int target = evaluateExpression(&p);
                    int step = 1;
                    while (*p == ' ') p++;
                    if (strncmp(p, "STEP", 4) == 0) { p += 4; step = evaluateExpression(&p); }

                    int existingIdx = -1;
                    if (forStackPtr > 0 && forStack[forStackPtr - 1].varIdx == varIdx &&
                        forStack[forStackPtr - 1].loopLineNumber == currentExecutingLineNum) {
                        existingIdx = forStackPtr - 1;
                    }
                    if (existingIdx != -1) {
                        forStack[existingIdx].targetValue = target;
                        forStack[existingIdx].stepValue = step;
                    } else {
                        if (forStackPtr < FOR_STACK_SIZE) {
                            variables[varIdx] = startVal;
                            forStack[forStackPtr].varIdx = varIdx;
                            forStack[forStackPtr].targetValue = target;
                            forStack[forStackPtr].stepValue = step;
                            forStack[forStackPtr].loopLineNumber = currentExecutingLineNum;
                            forStackPtr++;
                        } else { printError("Too many nested FOR loops"); }
                    }
                }
            }
        }
    }
    else if (strncmp(p, "NEXT", 4) == 0) {
        p += 4;
        while (*p == ' ') p++;
        if (isupper((unsigned char)*p)) {
            int varIdx = *p - 'A';
            int foundIdx = -1;
            for (int i = forStackPtr - 1; i >= 0; i--) {
                if (forStack[i].varIdx == varIdx) { foundIdx = i; break; }
            }
            if (foundIdx != -1) {
                variables[varIdx] += forStack[foundIdx].stepValue;
                int step = forStack[foundIdx].stepValue;
                int keepLooping = (step >= 0) ? (variables[varIdx] <= forStack[foundIdx].targetValue)
                                              : (variables[varIdx] >= forStack[foundIdx].targetValue);
                if (keepLooping) {
                    static char targetLineStr[16];
                    sprintf(targetLineStr, "%d", forStack[foundIdx].loopLineNumber);
                    currentLinePtr = targetLineStr;
                    forStackPtr = foundIdx + 1;
                } else { forStackPtr = foundIdx; }
            } else { printError("NEXT without FOR"); }
        }
    }
    else if (strncmp(p, "INPUT", 5) == 0) {
        p += 5;
        while (*p == ' ') p++;
        if (isupper((unsigned char)*p)) {
            int varIdx = *p - 'A';
            printf("? ");
            char inBuf[MAX_LINE_LEN];
            if (fgets(inBuf, sizeof(inBuf), stdin)) {
                if (*(p+1) == '$') {
                    inBuf[strcspn(inBuf, "\r\n")] = 0;
                    mini_strncpy(stringVariables[varIdx], inBuf, MAX_STR_LEN - 1);
                    stringVariables[varIdx][MAX_STR_LEN - 1] = '\0';
                } else { variables[varIdx] = (int)atoi(inBuf); }
            }
        }
    }
    else if (strncmp(p, "GOTO", 4) == 0) {
        p += 4;
        while (*p == ' ') p++;
        static char targetLineStr[16];
        sprintf(targetLineStr, "%d", evaluateExpression(&p));
        currentLinePtr = targetLineStr;
    }
    else if (strncmp(p, "IF", 2) == 0) {
        p += 2;
        while (*p == ' ') p++;
        if (evaluateCondition(&p)) {
            while (*p == ' ') p++;
            if (strncmp(p, "THEN", 4) == 0) {
                p += 4;
                while (*p == ' ') p++;
                executeLine(p);
            }
        }
    }
}
void parseCommand(char* buffer) {
    char* p = buffer;
    while (*p == ' ') p++;
    if (isdigit((unsigned char)*p)) {
        int lineNum = 0;
        while (isdigit((unsigned char)*p)) { lineNum = lineNum * 10 + (*p - '0'); p++; }
        while (*p == ' ') p++;
        if (*p == 0) { deleteLine(lineNum); }
        else { insertLine(lineNum, p); }
    } else {
        if (strncmp(buffer, "LIST", 4) == 0) { doList(); }
        else if (strncmp(buffer, "RUN", 3) == 0) { doRun(); }
        else if (strncmp(buffer, "NEW", 3) == 0) {
            programCount = 0;
            memset(variables, 0, sizeof(variables));
            memset(stringVariables, 0, sizeof(stringVariables));
            cleanArrays();
        }
        else if (strncmp(buffer, "QUIT", 4) == 0) { cleanArrays(); exit(0); }
        else {
            char temp[MAX_LINE_LEN];
            mini_strncpy(temp, buffer, MAX_LINE_LEN - 1);
            temp[MAX_LINE_LEN - 1] = '\0';
            executeLine(temp);
        }
    }
}

/* loadAndRunFile requires FILE I/O which is not available on M6809 / freestanding.
 * Guard it so it can still be used when compiled with a hosted toolchain.
 */
#ifdef HAVE_FILE_IO
typedef void FILE;
extern FILE* fopen(const char* path, const char* mode);
extern int   fclose(FILE* fp);
extern char* fgets_file(char* s, int n, FILE* fp); /* renamed to avoid clash */

void loadAndRunFile(const char* filename) {
    FILE* fp = fopen(filename, "r");
    if (!fp) { printf("? Could not open file: %s\n", filename); return; }
    char fileBuf[MAX_LINE_LEN];
    while (fgets_file(fileBuf, sizeof(fileBuf), fp)) {
        fileBuf[strcspn(fileBuf, "\r\n")] = 0;
        if (strlen(fileBuf) == 0) continue;
        int inQuote = 0;
        for (int i = 0; fileBuf[i]; i++) {
            if (fileBuf[i] == '"') inQuote = !inQuote;
            if (!inQuote) fileBuf[i] = toupper((unsigned char)fileBuf[i]);
        }
        parseCommand(fileBuf);
    }
    fclose(fp);
    doRun();
}
#endif /* HAVE_FILE_IO */

int main(void) {
    printf("16-Bit Integer Tiny Basic (C99)\n");
    char buffer[MAX_LINE_LEN];
    while (1) {
        printf("Ready\n");
        if (!fgets(buffer, sizeof(buffer), stdin)) break;
        buffer[strcspn(buffer, "\r\n")] = 0;
        int inQuote = 0;
        for (int i = 0; buffer[i]; i++) {
            if (buffer[i] == '"') inQuote = !inQuote;
            if (!inQuote) buffer[i] = toupper((unsigned char)buffer[i]);
        }
        if (strlen(buffer) == 0) continue;
        parseCommand(buffer);
    }
    cleanArrays();
    return 0;
}

