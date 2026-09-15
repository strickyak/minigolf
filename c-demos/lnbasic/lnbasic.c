/*
 * Line Number BASIC (lnbasic.c)
 * An ANSI C (C89) Line Number BASIC Interpreter.
 *
 * Designed for freestanding environments (e.g. Motorola 6809)
 * and standard Unix/Linux systems.
 *
 * Supported statements:
 *   LET, PRINT, FOR/NEXT, REM, IF/THEN, GOTO
 *
 * Supported commands:
 *   NEW, LIST, RUN, BYE
 *
 * Lines starting with line numbers insert, replace, or delete lines.
 */

#if unix
#include <stdio.h>
#else
static int putchar(int c) {
    *(volatile unsigned char *)0xFF00 = (unsigned char)c;
    return c;
}

static int getchar(void) {
    int c;
    do {
        c = (int)(*(volatile unsigned char *)0xFF01);
    } while (c == 0);
    return c;
}
#endif

#define MAX_LINES 128
#define MAX_LINE_LEN 80
#define MAX_FOR_STACK 8

struct Line {
    int num;
    char text[MAX_LINE_LEN];
};

static struct Line g_prog[MAX_LINES];
static int g_num_lines = 0;
static int g_vars[26];

struct ForFrame {
    int var_idx;
    int end_val;
    int step_val;
    int line_idx;
    int text_pos;
};

static struct ForFrame g_for_stack[MAX_FOR_STACK];
static int g_for_top = 0;

static int g_cur_line_idx = 0;
static int g_cur_offset = 0;
static int g_running = 0;
static int g_jumped = 0;
static int g_stopped = 0;
static int g_exit = 0;

/* Character and string utilities */

static int is_space(char c) {
    return c == ' ' || c == '\t';
}

static int is_digit(char c) {
    return c >= '0' && c <= '9';
}

static int is_alpha(char c) {
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z');
}

static char to_upper(char c) {
    if (c >= 'a' && c <= 'z') {
        return (char)(c - 'a' + 'A');
    }
    return c;
}

static void my_strcpy(char *dst, const char *src, int max_len) {
    int i;
    i = 0;
    while (i < max_len - 1 && src[i] != '\0') {
        dst[i] = src[i];
        i++;
    }
    dst[i] = '\0';
}

static void put_str(const char *s) {
    while (*s) {
        putchar(*s++);
    }
}

static void put_num(int n) {
    char buf[12];
    int i;
    unsigned int u;
    i = 0;
    if (n < 0) {
        putchar('-');
        u = (unsigned int)(-n);
    } else {
        u = (unsigned int)n;
    }
    if (u == 0) {
        putchar('0');
        return;
    }
    while (u > 0) {
        buf[i++] = (char)('0' + (u % 10));
        u /= 10;
    }
    while (i > 0) {
        putchar(buf[--i]);
    }
}

static void put_nl(void) {
    putchar('\n');
}

static int read_line(char *buf, int max_len) {
    int idx;
    int c;
    idx = 0;
    while (1) {
        c = getchar();
#if unix
        if (c < 0) {
            if (idx == 0) {
                return -1;
            }
            break;
        }
#endif
        if (c == '\r' || c == '\n') {
            break;
        }
        if (c == '\b' || c == 127) {
            if (idx > 0) {
                idx--;
            }
            continue;
        }
        if (idx < max_len - 1) {
            buf[idx++] = (char)c;
        }
    }
    buf[idx] = '\0';
    return idx;
}

static void skip_spaces(const char **p) {
    while (is_space(**p)) {
        (*p)++;
    }
}

static int match_kw(const char **p, const char *kw) {
    const char *s;
    s = *p;
    skip_spaces(&s);
    while (*kw) {
        char c1;
        char c2;
        c1 = to_upper(*s);
        c2 = to_upper(*kw);
        if (c1 != c2) {
            return 0;
        }
        s++;
        kw++;
    }
    if (is_alpha(*s) || is_digit(*s)) {
        return 0;
    }
    *p = s;
    return 1;
}

static int match_char(const char **p, char ch) {
    skip_spaces(p);
    if (**p == ch) {
        (*p)++;
        return 1;
    }
    return 0;
}

/* Program line management */

static int find_line(int num) {
    int i;
    for (i = 0; i < g_num_lines; i++) {
        if (g_prog[i].num == num) {
            return i;
        }
    }
    return -1;
}

static void insert_or_replace_line(int num, const char *text) {
    int i;
    int idx;
    
    skip_spaces(&text);
    idx = find_line(num);
    
    if (*text == '\0') {
        /* Deleting line */
        if (idx >= 0) {
            for (i = idx; i < g_num_lines - 1; i++) {
                g_prog[i] = g_prog[i + 1];
            }
            g_num_lines--;
        }
        return;
    }
    
    if (idx >= 0) {
        /* Replacing line */
        my_strcpy(g_prog[idx].text, text, MAX_LINE_LEN);
        return;
    }
    
    /* Inserting in sorted line number order */
    if (g_num_lines >= MAX_LINES) {
        put_str("?PROGRAM FULL\n");
        return;
    }
    
    idx = g_num_lines;
    for (i = 0; i < g_num_lines; i++) {
        if (g_prog[i].num > num) {
            idx = i;
            break;
        }
    }
    
    for (i = g_num_lines; i > idx; i--) {
        g_prog[i] = g_prog[i - 1];
    }
    
    g_prog[idx].num = num;
    my_strcpy(g_prog[idx].text, text, MAX_LINE_LEN);
    g_num_lines++;
}

static void cmd_new(void) {
    int i;
    g_num_lines = 0;
    for (i = 0; i < 26; i++) {
        g_vars[i] = 0;
    }
    g_for_top = 0;
    put_str("OK\n");
}

static void cmd_list(void) {
    int i;
    for (i = 0; i < g_num_lines; i++) {
        put_num(g_prog[i].num);
        putchar(' ');
        put_str(g_prog[i].text);
        put_nl();
    }
    put_str("OK\n");
}

/* Recursive-descent expression evaluator */

static int eval_expr(const char **p);
static int eval_term(const char **p);
static int eval_factor(const char **p);

static int eval_factor(const char **p) {
    int val;
    skip_spaces(p);
    if (**p == '+') {
        (*p)++;
        return eval_factor(p);
    }
    if (**p == '-') {
        (*p)++;
        return -eval_factor(p);
    }
    if (**p == '(') {
        (*p)++;
        val = eval_expr(p);
        match_char(p, ')');
        return val;
    }
    if (is_digit(**p)) {
        val = 0;
        while (is_digit(**p)) {
            val = val * 10 + (**p - '0');
            (*p)++;
        }
        return val;
    }
    if (is_alpha(**p)) {
        char c;
        c = to_upper(**p);
        (*p)++;
        return g_vars[c - 'A'];
    }
    return 0;
}

static int eval_term(const char **p) {
    int val;
    val = eval_factor(p);
    while (1) {
        skip_spaces(p);
        if (**p == '*') {
            (*p)++;
            val = val * eval_factor(p);
        } else if (**p == '/') {
            int denom;
            (*p)++;
            denom = eval_factor(p);
            if (denom == 0) {
                put_str("?DIVISION BY ZERO\n");
                g_stopped = 1;
                return 0;
            }
            val = val / denom;
        } else if (**p == '%') {
            int denom;
            (*p)++;
            denom = eval_factor(p);
            if (denom == 0) {
                put_str("?DIVISION BY ZERO\n");
                g_stopped = 1;
                return 0;
            }
            val = val % denom;
        } else {
            break;
        }
    }
    return val;
}

static int eval_expr(const char **p) {
    int val;
    val = eval_term(p);
    while (1) {
        skip_spaces(p);
        if (**p == '+') {
            (*p)++;
            val = val + eval_term(p);
        } else if (**p == '-') {
            (*p)++;
            val = val - eval_term(p);
        } else {
            break;
        }
    }
    return val;
}

static int eval_cond(const char **p) {
    int left;
    int right;
    int op; /* 1: ==, 2: !=, 3: <, 4: <=, 5: >, 6: >= */
    
    left = eval_expr(p);
    skip_spaces(p);
    
    if (**p == '=' && *(*p + 1) == '=') {
        op = 1; *p += 2;
    } else if (**p == '=') {
        op = 1; (*p)++;
    } else if (**p == '<' && *(*p + 1) == '>') {
        op = 2; *p += 2;
    } else if (**p == '!' && *(*p + 1) == '=') {
        op = 2; *p += 2;
    } else if (**p == '<' && *(*p + 1) == '=') {
        op = 4; *p += 2;
    } else if (**p == '<') {
        op = 3; (*p)++;
    } else if (**p == '>' && *(*p + 1) == '=') {
        op = 6; *p += 2;
    } else if (**p == '>') {
        op = 5; (*p)++;
    } else {
        return left != 0;
    }
    
    right = eval_expr(p);
    
    switch (op) {
        case 1: return left == right;
        case 2: return left != right;
        case 3: return left < right;
        case 4: return left <= right;
        case 5: return left > right;
        case 6: return left >= right;
        default: return 0;
    }
}

/* Statement execution */

static void exec_stmt(const char **p) {
    skip_spaces(p);
    if (**p == '\0' || **p == ':') {
        return;
    }
    
    /* REM */
    if (match_kw(p, "REM")) {
        while (**p) {
            (*p)++;
        }
        return;
    }
    
    /* PRINT */
    if (match_kw(p, "PRINT")) {
        int trailing;
        trailing = 0;
        skip_spaces(p);
        if (**p == '\0' || **p == ':') {
            put_nl();
            return;
        }
        while (**p && **p != ':') {
            skip_spaces(p);
            if (**p == '\0' || **p == ':') {
                break;
            }
            trailing = 0;
            if (**p == '"') {
                (*p)++;
                while (**p && **p != '"') {
                    putchar(**p);
                    (*p)++;
                }
                if (**p == '"') {
                    (*p)++;
                }
            } else {
                int val;
                val = eval_expr(p);
                put_num(val);
            }
            skip_spaces(p);
            if (**p == ';') {
                (*p)++;
                trailing = 1;
            } else if (**p == ',') {
                (*p)++;
                putchar('\t');
                trailing = 1;
            } else if (**p != '\0' && **p != ':') {
                break;
            }
        }
        if (!trailing) {
            put_nl();
        }
        return;
    }
    
    /* LET */
    if (match_kw(p, "LET")) {
        skip_spaces(p);
        if (is_alpha(**p)) {
            char v;
            v = to_upper(**p);
            (*p)++;
            if (match_char(p, '=')) {
                int val;
                val = eval_expr(p);
                g_vars[v - 'A'] = val;
            } else {
                put_str("?EXPECTED = IN LET\n");
                g_stopped = 1;
            }
        } else {
            put_str("?EXPECTED VARIABLE IN LET\n");
            g_stopped = 1;
        }
        return;
    }
    
    /* GOTO */
    if (match_kw(p, "GOTO")) {
        int target;
        int idx;
        target = eval_expr(p);
        idx = find_line(target);
        if (idx < 0) {
            put_str("?LINE NOT FOUND: ");
            put_num(target);
            put_nl();
            g_stopped = 1;
        } else {
            g_cur_line_idx = idx;
            g_cur_offset = 0;
            g_jumped = 1;
        }
        return;
    }
    
    /* IF ... THEN ... */
    if (match_kw(p, "IF")) {
        int cond;
        cond = eval_cond(p);
        if (match_kw(p, "THEN")) {
            if (cond) {
                skip_spaces(p);
                if (is_digit(**p)) {
                    int target;
                    int idx;
                    target = eval_expr(p);
                    idx = find_line(target);
                    if (idx < 0) {
                        put_str("?LINE NOT FOUND: ");
                        put_num(target);
                        put_nl();
                        g_stopped = 1;
                    } else {
                        g_cur_line_idx = idx;
                        g_cur_offset = 0;
                        g_jumped = 1;
                    }
                } else {
                    exec_stmt(p);
                }
            } else {
                while (**p) {
                    (*p)++;
                }
            }
        } else {
            put_str("?EXPECTED THEN IN IF\n");
            g_stopped = 1;
        }
        return;
    }
    
    /* FOR <var> = <start> TO <end> [STEP <step>] */
    if (match_kw(p, "FOR")) {
        char v;
        int var_idx;
        int start_val;
        int end_val;
        int step_val;
        struct ForFrame *f;
        
        skip_spaces(p);
        if (!is_alpha(**p)) {
            put_str("?SYNTAX ERROR IN FOR\n");
            g_stopped = 1;
            return;
        }
        v = to_upper(**p);
        (*p)++;
        var_idx = v - 'A';
        
        if (!match_char(p, '=')) {
            put_str("?EXPECTED = IN FOR\n");
            g_stopped = 1;
            return;
        }
        
        start_val = eval_expr(p);
        if (!match_kw(p, "TO")) {
            put_str("?EXPECTED TO IN FOR\n");
            g_stopped = 1;
            return;
        }
        
        end_val = eval_expr(p);
        step_val = 1;
        if (match_kw(p, "STEP")) {
            step_val = eval_expr(p);
        }
        
        g_vars[var_idx] = start_val;
        
        if (g_for_top >= MAX_FOR_STACK) {
            put_str("?FOR STACK OVERFLOW\n");
            g_stopped = 1;
            return;
        }
        
        f = &g_for_stack[g_for_top++];
        f->var_idx = var_idx;
        f->end_val = end_val;
        f->step_val = step_val;
        f->line_idx = g_cur_line_idx;
        if (g_running && g_cur_line_idx < g_num_lines) {
            f->text_pos = (int)(*p - g_prog[g_cur_line_idx].text);
        } else {
            f->text_pos = 0;
        }
        return;
    }
    
    /* NEXT [<var>] */
    if (match_kw(p, "NEXT")) {
        int target_var;
        int match_idx;
        int i;
        struct ForFrame *f;
        int done;
        
        skip_spaces(p);
        target_var = -1;
        if (is_alpha(**p)) {
            target_var = to_upper(**p) - 'A';
            (*p)++;
        }
        
        match_idx = -1;
        if (target_var >= 0) {
            for (i = g_for_top - 1; i >= 0; i--) {
                if (g_for_stack[i].var_idx == target_var) {
                    match_idx = i;
                    break;
                }
            }
        } else {
            if (g_for_top > 0) {
                match_idx = g_for_top - 1;
            }
        }
        
        if (match_idx < 0) {
            put_str("?NEXT WITHOUT FOR\n");
            g_stopped = 1;
            return;
        }
        
        f = &g_for_stack[match_idx];
        g_vars[f->var_idx] += f->step_val;
        
        done = 0;
        if (f->step_val > 0 && g_vars[f->var_idx] > f->end_val) done = 1;
        else if (f->step_val < 0 && g_vars[f->var_idx] < f->end_val) done = 1;
        
        if (!done) {
            g_cur_line_idx = f->line_idx;
            g_cur_offset = f->text_pos;
            g_jumped = 1;
            g_for_top = match_idx + 1;
        } else {
            g_for_top = match_idx;
        }
        return;
    }
    
    /* Implicit LET: <var> = <expr> */
    skip_spaces(p);
    if (is_alpha(**p)) {
        const char *peek;
        peek = *p + 1;
        skip_spaces(&peek);
        if (*peek == '=') {
            char v;
            v = to_upper(**p);
            *p = peek + 1;
            g_vars[v - 'A'] = eval_expr(p);
            return;
        }
    }
    
    /* Unknown statement */
    put_str("?SYNTAX ERROR\n");
    g_stopped = 1;
}

/* Command execution */

static void cmd_run(void) {
    int i;
    for (i = 0; i < 26; i++) {
        g_vars[i] = 0;
    }
    g_for_top = 0;
    g_stopped = 0;
    g_running = 1;
    g_cur_line_idx = 0;
    g_cur_offset = 0;

    while (g_cur_line_idx < g_num_lines && !g_stopped) {
        const char *p;
        g_jumped = 0;
        p = g_prog[g_cur_line_idx].text + g_cur_offset;
        g_cur_offset = 0;
        
        while (*p && !g_stopped && !g_jumped) {
            skip_spaces(&p);
            if (*p == '\0') break;
            exec_stmt(&p);
            if (g_jumped) break;
            skip_spaces(&p);
            if (*p == ':') {
                p++;
            } else if (*p != '\0') {
                break;
            }
        }
        
        if (!g_jumped) {
            g_cur_line_idx++;
            g_cur_offset = 0;
        }
    }
    g_running = 0;
    if (!g_stopped) {
        put_str("OK\n");
    }
}

static void process_input_line(const char *line) {
    const char *p;
    p = line;
    skip_spaces(&p);
    if (*p == '\0') {
        return;
    }
    
    /* If line starts with a number, insert, replace, or delete program line */
    if (is_digit(*p)) {
        int num;
        num = 0;
        while (is_digit(*p)) {
            num = num * 10 + (*p - '0');
            p++;
        }
        insert_or_replace_line(num, p);
        return;
    }
    
    /* Commands */
    if (match_kw(&p, "NEW")) {
        cmd_new();
        return;
    }
    if (match_kw(&p, "LIST")) {
        cmd_list();
        return;
    }
    if (match_kw(&p, "RUN")) {
        cmd_run();
        return;
    }
    if (match_kw(&p, "BYE")) {
        g_exit = 1;
        return;
    }
    
    /* Immediate statement */
    g_stopped = 0;
    g_jumped = 0;
    while (*p && !g_stopped && !g_jumped) {
        skip_spaces(&p);
        if (*p == '\0') break;
        exec_stmt(&p);
        if (g_jumped) break;
        skip_spaces(&p);
        if (*p == ':') {
            p++;
        } else if (*p != '\0') {
            break;
        }
    }
    if (!g_stopped) {
        put_str("OK\n");
    }
}

int main(void) {
    char buf[MAX_LINE_LEN];
    put_str("Line Number BASIC\nOK\n");
    while (!g_exit) {
        int len;
        len = read_line(buf, sizeof(buf));
        if (len < 0) {
            break;
        }
        process_input_line(buf);
    }
    return 0;
}
