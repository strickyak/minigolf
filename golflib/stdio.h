#warning TODO -- stdio.h

typedef struct stdio_FILE { int fd; } FILE;

FILE *stdin;
FILE *stdout;
FILE *stderr;

#define EOF -1
