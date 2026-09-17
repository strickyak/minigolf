#ifndef PYSUB_C
#define PYSUB_C

#include "pysub.h"

extern void* zalloc(int n);
extern void* malloc(int n);
extern void free(void* p);
extern void putchar(char ch);

#define TOK_EOF      0
#define TOK_NEWLINE  1
#define TOK_INDENT   2
#define TOK_DEDENT   3
#define TOK_DEF      4
#define TOK_RETURN   5
#define TOK_FOR      6
#define TOK_IN       7
#define TOK_IDENT    8
#define TOK_INT      9
#define TOK_LPAREN   10
#define TOK_RPAREN   11
#define TOK_LBRACKET 12
#define TOK_RBRACKET 13
#define TOK_COLON    14
#define TOK_COMMA    15
#define TOK_ASSIGN   16
#define TOK_PLUS     17

#define EXPR_INT    1
#define EXPR_VAR    2
#define EXPR_LIST   3
#define EXPR_BINOP  4
#define EXPR_CALL   5

#define STMT_DEF    1
#define STMT_ASSIGN 2
#define STMT_FOR    3
#define STMT_RETURN 4
#define STMT_EXPR   5

#define VAL_NONE 0
#define VAL_INT  1
#define VAL_LIST 2
#define VAL_FUNC 3

typedef struct Token Token;
struct Token {
    int tok_type;
    int ival;
    char *text;
};

static int indent_stack[16];

typedef struct Lexer {
    const char *src;
    int pos;
    int at_line_start;
    int indent_depth;
    int pending_dedents;
} Lexer;

static int is_digit(char c) {
    if (c >= '0' && c <= '9') return 1;
    return 0;
}

static int is_alpha(char c) {
    if (c >= 'a' && c <= 'z') return 1;
    if (c >= 'A' && c <= 'Z') return 1;
    if (c == '_') return 1;
    return 0;
}

static int is_alnum(char c) {
    if (is_alpha(c) != 0 || is_digit(c) != 0) return 1;
    return 0;
}

static int str_equal(const char *s1, const char *s2) {
    int i = 0;
    while (s1[i] != '\0' && s2[i] != '\0') {
        if (s1[i] != s2[i]) return 0;
        i = i + 1;
    }
    if (s1[i] == '\0' && s2[i] == '\0') return 1;
    return 0;
}

static char* str_dup(const char *s) {
    int len = 0;
    while (s[len] != '\0') {
        len = len + 1;
    }
    char *d = (char*)malloc(len + 1);
    int i;
    for (i = 0; i <= len; i = i + 1) {
        d[i] = s[i];
    }
    return d;
}

static void lexer_init(Lexer *lex, const char *src) {
    lex->src = src;
    lex->pos = 0;
    lex->at_line_start = 1;
    lex->indent_depth = 0;
    indent_stack[0] = 0;
    lex->pending_dedents = 0;
}

static void lexer_next(Lexer *lex, Token *tok) {
    tok->tok_type = TOK_EOF;
    tok->ival = 0;
    tok->text = (char*)0;

    if (lex->pending_dedents > 0) {
        lex->pending_dedents = lex->pending_dedents - 1;
        tok->tok_type = TOK_DEDENT;
        return;
    }

    const char *s = lex->src;

    while (1) {
        if (lex->at_line_start != 0) {
            int col = 0;
            int start_pos = lex->pos;
            while (s[start_pos] == ' ') {
                start_pos = start_pos + 1;
                col = col + 1;
            }
            char ch = s[start_pos];
            if (ch == '\r' || ch == '\n') {
                lex->pos = start_pos;
                if (s[lex->pos] == '\r' && s[lex->pos + 1] == '\n') {
                    lex->pos = lex->pos + 2;
                } else if (s[lex->pos] == '\n') {
                    lex->pos = lex->pos + 1;
                }
                continue;
            }
            if (ch == '\0') {
                lex->pos = start_pos;
                if (lex->indent_depth > 0) {
                    lex->pending_dedents = lex->indent_depth - 1;
                    lex->indent_depth = 0;
                    tok->tok_type = TOK_DEDENT;
                    return;
                }
                tok->tok_type = TOK_EOF;
                return;
            }

            lex->pos = start_pos;
            lex->at_line_start = 0;

            int cur_indent = indent_stack[lex->indent_depth];
            if (col > cur_indent) {
                lex->indent_depth = lex->indent_depth + 1;
                indent_stack[lex->indent_depth] = col;
                tok->tok_type = TOK_INDENT;
                return;
            }
            if (col < cur_indent) {
                int count = 0;
                while (lex->indent_depth > 0 && indent_stack[lex->indent_depth] > col) {
                    lex->indent_depth = lex->indent_depth - 1;
                    count = count + 1;
                }
                if (count > 0) {
                    lex->pending_dedents = count - 1;
                    tok->tok_type = TOK_DEDENT;
                    return;
                }
            }
        }

        while (s[lex->pos] == ' ' || s[lex->pos] == '\t') {
            lex->pos = lex->pos + 1;
        }

        char c = s[lex->pos];

        if (c == '\0') {
            if (lex->indent_depth > 0) {
                lex->pending_dedents = lex->indent_depth - 1;
                lex->indent_depth = 0;
                tok->tok_type = TOK_DEDENT;
                return;
            }
            tok->tok_type = TOK_EOF;
            return;
        }

        if (c == '\r' || c == '\n') {
            if (s[lex->pos] == '\r' && s[lex->pos + 1] == '\n') {
                lex->pos = lex->pos + 2;
            } else {
                lex->pos = lex->pos + 1;
            }
            lex->at_line_start = 1;
            tok->tok_type = TOK_NEWLINE;
            return;
        }

        if (c == '(') { lex->pos = lex->pos + 1; tok->tok_type = TOK_LPAREN; return; }
        if (c == ')') { lex->pos = lex->pos + 1; tok->tok_type = TOK_RPAREN; return; }
        if (c == '[') { lex->pos = lex->pos + 1; tok->tok_type = TOK_LBRACKET; return; }
        if (c == ']') { lex->pos = lex->pos + 1; tok->tok_type = TOK_RBRACKET; return; }
        if (c == ':') { lex->pos = lex->pos + 1; tok->tok_type = TOK_COLON; return; }
        if (c == ',') { lex->pos = lex->pos + 1; tok->tok_type = TOK_COMMA; return; }
        if (c == '+') { lex->pos = lex->pos + 1; tok->tok_type = TOK_PLUS; return; }
        if (c == '=') { lex->pos = lex->pos + 1; tok->tok_type = TOK_ASSIGN; return; }

        if (is_digit(c) != 0) {
            int val = 0;
            while (is_digit(s[lex->pos]) != 0) {
                val = val * 10 + (int)(s[lex->pos] - '0');
                lex->pos = lex->pos + 1;
            }
            tok->tok_type = TOK_INT;
            tok->ival = val;
            return;
        }

        if (is_alpha(c) != 0) {
            char buf[32];
            int k = 0;
            while (is_alnum(s[lex->pos]) != 0 && k < 30) {
                buf[k] = s[lex->pos];
                lex->pos = lex->pos + 1;
                k = k + 1;
            }
            buf[k] = '\0';
            tok->text = str_dup(buf);

            if (str_equal(tok->text, "def") != 0) {
                tok->tok_type = TOK_DEF;
            } else if (str_equal(tok->text, "return") != 0) {
                tok->tok_type = TOK_RETURN;
            } else if (str_equal(tok->text, "for") != 0) {
                tok->tok_type = TOK_FOR;
            } else if (str_equal(tok->text, "in") != 0) {
                tok->tok_type = TOK_IN;
            } else {
                tok->tok_type = TOK_IDENT;
            }
            return;
        }

        lex->pos = lex->pos + 1;
    }
}

typedef struct Parser {
    Lexer lex;
    Token cur_tok;
} Parser;

static void parser_advance(Parser *p) {
    lexer_next(&p->lex, &p->cur_tok);
}

static void parser_init(Parser *p, const char *src) {
    lexer_init(&p->lex, src);
    parser_advance(p);
}

static void parser_consume(Parser *p, int expected_type) {
    if (p->cur_tok.tok_type == expected_type) {
        parser_advance(p);
    }
}

// AST
typedef struct Expr Expr;
struct Expr {
    int kind;
    int ival;
    char *name;
    int op;
    Expr *left;
    Expr *right;
    Expr *next;
    Expr *child;
};

typedef struct Stmt Stmt;
struct Stmt {
    int kind;
    char *name;
    char *param0;
    char *param1;
    char *param2;
    int num_params;
    Expr *expr;
    Stmt *body;
    Stmt *next;
};

static const char* stmt_get_param(Stmt *s, int i) {
    if (i == 0) return s->param0;
    if (i == 1) return s->param1;
    if (i == 2) return s->param2;
    return "";
}

static Expr* make_expr_int(int val) {
    Expr *e = (Expr*)zalloc((int)sizeof(Expr));
    e->kind = EXPR_INT;
    e->ival = val;
    return e;
}

static Expr* make_expr_var(const char *name) {
    Expr *e = (Expr*)zalloc((int)sizeof(Expr));
    e->kind = EXPR_VAR;
    e->name = str_dup(name);
    return e;
}

static Expr* make_expr_list(Expr *children) {
    Expr *e = (Expr*)zalloc((int)sizeof(Expr));
    e->kind = EXPR_LIST;
    e->child = children;
    return e;
}

static Expr* make_expr_binop(int op, Expr *left, Expr *right) {
    Expr *e = (Expr*)zalloc((int)sizeof(Expr));
    e->kind = EXPR_BINOP;
    e->op = op;
    e->left = left;
    e->right = right;
    return e;
}

static Expr* make_expr_call(const char *name, Expr *args) {
    Expr *e = (Expr*)zalloc((int)sizeof(Expr));
    e->kind = EXPR_CALL;
    e->name = str_dup(name);
    e->child = args;
    return e;
}

static Stmt* make_stmt_def(const char *name, int num_params,
                           const char *p0, const char *p1, const char *p2,
                           Stmt *body) {
    Stmt *s = (Stmt*)zalloc((int)sizeof(Stmt));
    s->kind = STMT_DEF;
    s->name = str_dup(name);
    s->num_params = num_params;
    if (p0 != (const char*)0) s->param0 = str_dup(p0);
    if (p1 != (const char*)0) s->param1 = str_dup(p1);
    if (p2 != (const char*)0) s->param2 = str_dup(p2);
    s->body = body;
    return s;
}

static Stmt* make_stmt_assign(const char *name, Expr *rhs) {
    Stmt *s = (Stmt*)zalloc((int)sizeof(Stmt));
    s->kind = STMT_ASSIGN;
    s->name = str_dup(name);
    s->expr = rhs;
    return s;
}

static Stmt* make_stmt_for(const char *loop_var, Expr *iter, Stmt *body) {
    Stmt *s = (Stmt*)zalloc((int)sizeof(Stmt));
    s->kind = STMT_FOR;
    s->name = str_dup(loop_var);
    s->expr = iter;
    s->body = body;
    return s;
}

static Stmt* make_stmt_return(Expr *expr) {
    Stmt *s = (Stmt*)zalloc((int)sizeof(Stmt));
    s->kind = STMT_RETURN;
    s->expr = expr;
    return s;
}

static Stmt* make_stmt_expr(Expr *expr) {
    Stmt *s = (Stmt*)zalloc((int)sizeof(Stmt));
    s->kind = STMT_EXPR;
    s->expr = expr;
    return s;
}

static Expr* parse_expr(Parser *p);

static Expr* parse_primary(Parser *p) {
    if (p->cur_tok.tok_type == TOK_INT) {
        int v = p->cur_tok.ival;
        parser_advance(p);
        return make_expr_int(v);
    }
    if (p->cur_tok.tok_type == TOK_IDENT) {
        char *name = str_dup(p->cur_tok.text);
        parser_advance(p);
        if (p->cur_tok.tok_type == TOK_LPAREN) {
            parser_advance(p);
            Expr *args = (Expr*)0;
            Expr *last = (Expr*)0;
            if (p->cur_tok.tok_type != TOK_RPAREN) {
                while (1) {
                    Expr *arg = parse_expr(p);
                    if (args == (Expr*)0) {
                        args = arg;
                        last = arg;
                    } else {
                        last->next = arg;
                        last = arg;
                    }
                    if (p->cur_tok.tok_type == TOK_COMMA) {
                        parser_advance(p);
                    } else {
                        break;
                    }
                }
            }
            parser_consume(p, TOK_RPAREN);
            Expr *call_expr = make_expr_call(name, args);
            return call_expr;
        } else {
            Expr *var_expr = make_expr_var(name);
            return var_expr;
        }
    }
    if (p->cur_tok.tok_type == TOK_LBRACKET) {
        parser_advance(p);
        Expr *elems = (Expr*)0;
        Expr *last = (Expr*)0;
        if (p->cur_tok.tok_type != TOK_RBRACKET) {
            while (1) {
                Expr *elem = parse_expr(p);
                if (elems == (Expr*)0) {
                    elems = elem;
                    last = elem;
                } else {
                    last->next = elem;
                    last = elem;
                }
                if (p->cur_tok.tok_type == TOK_COMMA) {
                    parser_advance(p);
                } else {
                    break;
                }
            }
        }
        parser_consume(p, TOK_RBRACKET);
        return make_expr_list(elems);
    }
    if (p->cur_tok.tok_type == TOK_LPAREN) {
        parser_advance(p);
        Expr *e = parse_expr(p);
        parser_consume(p, TOK_RPAREN);
        return e;
    }
    return (Expr*)0;
}

static Expr* parse_expr(Parser *p) {
    Expr *left = parse_primary(p);
    while (p->cur_tok.tok_type == TOK_PLUS) {
        parser_advance(p);
        Expr *right = parse_primary(p);
        left = make_expr_binop('+', left, right);
    }
    return left;
}

static Stmt* parse_block(Parser *p);

static Stmt* parse_stmt(Parser *p) {
    while (p->cur_tok.tok_type == TOK_NEWLINE) {
        parser_advance(p);
    }

    if (p->cur_tok.tok_type == TOK_DEF) {
        parser_advance(p);
        char *fn_name = str_dup(p->cur_tok.text);
        parser_consume(p, TOK_IDENT);
        parser_consume(p, TOK_LPAREN);

        char *p0 = (char*)0;
        char *p1 = (char*)0;
        char *p2 = (char*)0;
        int num_params = 0;

        if (p->cur_tok.tok_type == TOK_IDENT) {
            p0 = str_dup(p->cur_tok.text);
            num_params = 1;
            parser_advance(p);
            if (p->cur_tok.tok_type == TOK_COMMA) {
                parser_advance(p);
                if (p->cur_tok.tok_type == TOK_IDENT) {
                    p1 = str_dup(p->cur_tok.text);
                    num_params = 2;
                    parser_advance(p);
                    if (p->cur_tok.tok_type == TOK_COMMA) {
                        parser_advance(p);
                        if (p->cur_tok.tok_type == TOK_IDENT) {
                            p2 = str_dup(p->cur_tok.text);
                            num_params = 3;
                            parser_advance(p);
                        }
                    }
                }
            }
        }
        parser_consume(p, TOK_RPAREN);
        parser_consume(p, TOK_COLON);
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        Stmt *body = parse_block(p);
        Stmt *res = make_stmt_def(fn_name, num_params, p0, p1, p2, body);
        return res;
    }

    if (p->cur_tok.tok_type == TOK_RETURN) {
        parser_advance(p);
        Expr *e = parse_expr(p);
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        return make_stmt_return(e);
    }

    if (p->cur_tok.tok_type == TOK_FOR) {
        parser_advance(p);
        char *var_name = str_dup(p->cur_tok.text);
        parser_consume(p, TOK_IDENT);
        parser_consume(p, TOK_IN);
        Expr *iter = parse_expr(p);
        parser_consume(p, TOK_COLON);
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        Stmt *body = parse_block(p);
        Stmt *res = make_stmt_for(var_name, iter, body);
        return res;
    }

    Expr *e = parse_expr(p);
    if (p->cur_tok.tok_type == TOK_ASSIGN) {
        parser_advance(p);
        Expr *rhs = parse_expr(p);
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        return make_stmt_assign(e->name, rhs);
    } else {
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        return make_stmt_expr(e);
    }
}

static Stmt* parse_block(Parser *p) {
    parser_consume(p, TOK_INDENT);
    Stmt *head = (Stmt*)0;
    Stmt *last = (Stmt*)0;

    while (p->cur_tok.tok_type != TOK_DEDENT && p->cur_tok.tok_type != TOK_EOF) {
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        if (p->cur_tok.tok_type == TOK_DEDENT || p->cur_tok.tok_type == TOK_EOF) {
            break;
        }
        Stmt *s = parse_stmt(p);
        if (head == (Stmt*)0) {
            head = s;
            last = s;
        } else {
            last->next = s;
            last = s;
        }
    }
    if (p->cur_tok.tok_type == TOK_DEDENT) {
        parser_advance(p);
    }
    return head;
}

static Stmt* parse_program(Parser *p) {
    Stmt *head = (Stmt*)0;
    Stmt *last = (Stmt*)0;

    while (p->cur_tok.tok_type != TOK_EOF) {
        while (p->cur_tok.tok_type == TOK_NEWLINE) {
            parser_advance(p);
        }
        if (p->cur_tok.tok_type == TOK_EOF) {
            break;
        }
        Stmt *s = parse_stmt(p);
        if (head == (Stmt*)0) {
            head = s;
            last = s;
        } else {
            last->next = s;
            last = s;
        }
    }
    return head;
}

// Runtime
typedef struct Value Value;
typedef struct List List;
typedef struct Function Function;

struct List {
    int len;
    int cap;
    Value **items;
};

struct Function {
    char *name;
    int num_params;
    char *param0;
    char *param1;
    char *param2;
    Stmt *body;
};

struct Value {
    int val_type;
    int ival;
    List *lval;
    Function *fval;
};

static const char* fn_get_param(Function *fn, int i) {
    if (i == 0) return fn->param0;
    if (i == 1) return fn->param1;
    if (i == 2) return fn->param2;
    return "";
}

static Value* val_new_none(void) {
    Value *v = (Value*)zalloc((int)sizeof(Value));
    v->val_type = VAL_NONE;
    return v;
}

static Value* val_new_int(int i) {
    Value *v = (Value*)zalloc((int)sizeof(Value));
    v->val_type = VAL_INT;
    v->ival = i;
    return v;
}

static List* list_new(void) {
    List *l = (List*)zalloc((int)sizeof(List));
    l->len = 0;
    l->cap = 4;
    l->items = (Value**)zalloc(4 * (int)sizeof(Value*));
    return l;
}

static Value* val_new_list(void) {
    Value *v = (Value*)zalloc((int)sizeof(Value));
    v->val_type = VAL_LIST;
    v->lval = list_new();
    return v;
}

static Value* val_new_func(Function *f) {
    Value *v = (Value*)zalloc((int)sizeof(Value));
    v->val_type = VAL_FUNC;
    v->fval = f;
    return v;
}

static void list_append(List *l, Value *v) {
    if (l->len >= l->cap) {
        int new_cap = l->cap * 2;
        Value **new_items = (Value**)zalloc(new_cap * (int)sizeof(Value*));
        int i;
        Value **old_items = l->items;
        for (i = 0; i < l->len; i = i + 1) {
            new_items[i] = old_items[i];
        }
        l->items = new_items;
        l->cap = new_cap;
    }
    Value **items = l->items;
    items[l->len] = v;
    l->len = l->len + 1;
}

static Value* list_get(List *l, int i) {
    Value **items = l->items;
    return items[i];
}

static Value* val_add(Value *a, Value *b) {
    if (a->val_type == VAL_INT && b->val_type == VAL_INT) {
        return val_new_int(a->ival + b->ival);
    }
    if (a->val_type == VAL_LIST && b->val_type == VAL_LIST) {
        Value *res = val_new_list();
        List *la = a->lval;
        List *lb = b->lval;
        int i;
        for (i = 0; i < la->len; i = i + 1) {
            list_append(res->lval, list_get(la, i));
        }
        for (i = 0; i < lb->len; i = i + 1) {
            list_append(res->lval, list_get(lb, i));
        }
        return res;
    }
    return val_new_none();
}

static void print_str(const char *s) {
    if (s == (const char*)0) return;
    while (*s != '\0') {
        putchar(*s);
        s = s + 1;
    }
}

static void print_int(int n) {
    if (n < 0) {
        putchar('-');
        n = 0 - n;
    }
    if (n == 0) {
        putchar('0');
        return;
    }
    char buf[10];
    int count = 0;
    while (n > 0) {
        buf[count] = (char)('0' + (n % 10));
        n = n / 10;
        count = count + 1;
    }
    while (count > 0) {
        count = count - 1;
        putchar(buf[count]);
    }
}

static void val_print_repr(Value *v) {
    if (v == (Value*)0 || v->val_type == VAL_NONE) {
        print_str("None");
    } else if (v->val_type == VAL_INT) {
        print_int(v->ival);
    } else if (v->val_type == VAL_LIST) {
        putchar('[');
        List *l = v->lval;
        int i;
        for (i = 0; i < l->len; i = i + 1) {
            if (i > 0) {
                putchar(',');
                putchar(' ');
            }
            val_print_repr(list_get(l, i));
        }
        putchar(']');
    } else if (v->val_type == VAL_FUNC) {
        print_str("<function>");
    }
}

static void builtin_print(Value *v) {
    val_print_repr(v);
    putchar('\n');
}

static Value* builtin_range(int n) {
    Value *res = val_new_list();
    int i;
    for (i = 0; i < n; i = i + 1) {
        list_append(res->lval, val_new_int(i));
    }
    return res;
}

// Environment
typedef struct Binding Binding;
struct Binding {
    char *name;
    Value *val;
    Binding *next;
};

typedef struct Env Env;
struct Env {
    Env *parent;
    Binding *bindings;
};

static Env* env_new(Env *parent) {
    Env *e = (Env*)zalloc((int)sizeof(Env));
    e->parent = parent;
    e->bindings = (Binding*)0;
    return e;
}

static Value* env_get(Env *env, const char *name) {
    Env *curr = env;
    while (curr != (Env*)0) {
        Binding *b = curr->bindings;
        while (b != (Binding*)0) {
            if (str_equal(b->name, name) != 0) {
                return b->val;
            }
            b = b->next;
        }
        curr = curr->parent;
    }
    return (Value*)0;
}

static void env_set(Env *env, const char *name, Value *val) {
    Binding *b = env->bindings;
    while (b != (Binding*)0) {
        if (str_equal(b->name, name) != 0) {
            b->val = val;
            return;
        }
        b = b->next;
    }
    Binding *nb = (Binding*)zalloc((int)sizeof(Binding));
    nb->name = str_dup(name);
    nb->val = val;
    nb->next = env->bindings;
    env->bindings = nb;
}

static Value* eval_expr(Expr *e, Env *env, Env *global_env);
static int exec_stmt(Stmt *s, Env *env, Env *global_env, Value **ret_val);

static int exec_block(Stmt *head, Env *env, Env *global_env, Value **ret_val) {
    Stmt *s = head;
    while (s != (Stmt*)0) {
        int ret = exec_stmt(s, env, global_env, ret_val);
        if (ret != 0) {
            return 1;
        }
        s = s->next;
    }
    return 0;
}

static int exec_stmt(Stmt *s, Env *env, Env *global_env, Value **ret_val) {
    if (s->kind == STMT_DEF) {
        Function *fn = (Function*)zalloc((int)sizeof(Function));
        fn->name = str_dup(s->name);
        fn->num_params = s->num_params;
        if (s->param0 != (char*)0) fn->param0 = str_dup(s->param0);
        if (s->param1 != (char*)0) fn->param1 = str_dup(s->param1);
        if (s->param2 != (char*)0) fn->param2 = str_dup(s->param2);
        fn->body = s->body;
        env_set(global_env, fn->name, val_new_func(fn));
        return 0;
    }
    if (s->kind == STMT_ASSIGN) {
        Value *v = eval_expr(s->expr, env, global_env);
        env_set(env, s->name, v);
        return 0;
    }
    if (s->kind == STMT_FOR) {
        Value *iter = eval_expr(s->expr, env, global_env);
        if (iter != (Value*)0 && iter->val_type == VAL_LIST) {
            List *l = iter->lval;
            int i;
            for (i = 0; i < l->len; i = i + 1) {
                env_set(env, s->name, list_get(l, i));
                int ret = exec_block(s->body, env, global_env, ret_val);
                if (ret != 0) {
                    return 1;
                }
            }
        }
        return 0;
    }
    if (s->kind == STMT_RETURN) {
        *ret_val = eval_expr(s->expr, env, global_env);
        return 1;
    }
    if (s->kind == STMT_EXPR) {
        eval_expr(s->expr, env, global_env);
        return 0;
    }
    return 0;
}

static Value* eval_expr(Expr *e, Env *env, Env *global_env) {
    if (e == (Expr*)0) return val_new_none();

    if (e->kind == EXPR_INT) {
        return val_new_int(e->ival);
    }
    if (e->kind == EXPR_VAR) {
        Value *v = env_get(env, e->name);
        if (v == (Value*)0) {
            v = env_get(global_env, e->name);
        }
        if (v != (Value*)0) return v;
        return val_new_none();
    }
    if (e->kind == EXPR_LIST) {
        Value *res = val_new_list();
        Expr *curr = e->child;
        while (curr != (Expr*)0) {
            list_append(res->lval, eval_expr(curr, env, global_env));
            curr = curr->next;
        }
        return res;
    }
    if (e->kind == EXPR_BINOP) {
        Value *left = eval_expr(e->left, env, global_env);
        Value *right = eval_expr(e->right, env, global_env);
        if (e->op == '+') {
            return val_add(left, right);
        }
        return val_new_none();
    }
    if (e->kind == EXPR_CALL) {
        if (str_equal(e->name, "print") != 0) {
            Expr *arg = e->child;
            if (arg != (Expr*)0) {
                Value *v = eval_expr(arg, env, global_env);
                builtin_print(v);
            } else {
                putchar('\n');
            }
            return val_new_none();
        }
        if (str_equal(e->name, "range") != 0) {
            Expr *arg = e->child;
            Value *v = eval_expr(arg, env, global_env);
            int n = 0;
            if (v != (Value*)0 && v->val_type == VAL_INT) {
                n = v->ival;
            }
            return builtin_range(n);
        }

        Value *fn_val = env_get(env, e->name);
        if (fn_val == (Value*)0) {
            fn_val = env_get(global_env, e->name);
        }
        if (fn_val != (Value*)0 && fn_val->val_type == VAL_FUNC) {
            Function *fn = fn_val->fval;
            Env *call_env = env_new(global_env);
            Expr *arg = e->child;
            int idx = 0;
            while (arg != (Expr*)0 && idx < fn->num_params) {
                Value *argv = eval_expr(arg, env, global_env);
                const char *pname = fn_get_param(fn, idx);
                env_set(call_env, pname, argv);
                arg = arg->next;
                idx = idx + 1;
            }
            Value *ret = (Value*)0;
            exec_block(fn->body, call_env, global_env, &ret);
            if (ret != (Value*)0) {
                return ret;
            }
            return val_new_none();
        }
        return val_new_none();
    }
    return val_new_none();
}

int run_python(const char *source) {
    Parser p;
    parser_init(&p, source);
    Stmt *program = parse_program(&p);

    Env *global_env = env_new((Env*)0);
    Value *ret = (Value*)0;
    exec_block(program, global_env, global_env, &ret);
    return 0;
}

#ifndef NO_MAIN
int main(void) {
    run_python(PROGRAM);
    return 0;
}
#endif

#endif /* PYSUB_C */
