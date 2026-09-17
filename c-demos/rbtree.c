#ifndef RBTREE_C
#define RBTREE_C

#include "rbtree.h"

extern void* zalloc(int n);
extern void putchar(char ch);

static int rbtree_strcmp(const char *s1, const char *s2) {
    while (*s1 != '\0' && (*s1 == *s2)) {
        s1 = s1 + 1;
        s2 = s2 + 1;
    }
    return (int)((unsigned char)*s1) - (int)((unsigned char)*s2);
}

static RBTreeNode* rbtree_alloc_node(const char *key, int val, int color) {
    RBTreeNode *p = (RBTreeNode*)zalloc((int)sizeof(RBTreeNode));
    p->Key = key;
    p->Val = val;
    p->Color = color;
    p->Left = (RBTreeNode*)0;
    p->Right = (RBTreeNode*)0;
    return p;
}

static int rbtree_is_red(const RBTreeNode *h) {
    if (h == (const RBTreeNode*)0) {
        return 0;
    }
    return h->Color == RBTREE_RED;
}

static RBTreeNode* rbtree_rotate_left(RBTreeNode *h) {
    RBTreeNode *x = h->Right;
    h->Right = x->Left;
    x->Left = h;
    x->Color = h->Color;
    h->Color = RBTREE_RED;
    return x;
}

static RBTreeNode* rbtree_rotate_right(RBTreeNode *h) {
    RBTreeNode *x = h->Left;
    h->Left = x->Right;
    x->Right = h;
    x->Color = h->Color;
    h->Color = RBTREE_RED;
    return x;
}

static void rbtree_flip_colors(RBTreeNode *h) {
    h->Color = (h->Color == RBTREE_RED) ? RBTREE_BLACK : RBTREE_RED;
    if (h->Left != (RBTreeNode*)0) {
        h->Left->Color = (h->Left->Color == RBTREE_RED) ? RBTREE_BLACK : RBTREE_RED;
    }
    if (h->Right != (RBTreeNode*)0) {
        h->Right->Color = (h->Right->Color == RBTREE_RED) ? RBTREE_BLACK : RBTREE_RED;
    }
}

static RBTreeNode* rbtree_insert_node(RBTreeNode *h, const char *key, int val, int *is_new) {
    if (h == (RBTreeNode*)0) {
        *is_new = 1;
        return rbtree_alloc_node(key, val, RBTREE_RED);
    }

    int cmp = rbtree_strcmp(key, h->Key);
    if (cmp < 0) {
        h->Left = rbtree_insert_node(h->Left, key, val, is_new);
    } else if (cmp > 0) {
        h->Right = rbtree_insert_node(h->Right, key, val, is_new);
    } else {
        h->Val = val;
        return h;
    }

    if (rbtree_is_red(h->Right) && !rbtree_is_red(h->Left)) {
        h = rbtree_rotate_left(h);
    }
    if (rbtree_is_red(h->Left) && rbtree_is_red(h->Left->Left)) {
        h = rbtree_rotate_right(h);
    }
    if (rbtree_is_red(h->Left) && rbtree_is_red(h->Right)) {
        rbtree_flip_colors(h);
    }

    return h;
}

void rbtree_init(RBTree *tree) {
    tree->Root = (RBTreeNode*)0;
    tree->Len = 0;
}

int rbtree_size(const RBTree *tree) {
    return tree->Len;
}

int* rbtree_get_ptr(RBTree *tree, const char *key) {
    RBTreeNode *curr = tree->Root;
    while (curr != (RBTreeNode*)0) {
        int cmp = rbtree_strcmp(key, curr->Key);
        if (cmp < 0) {
            curr = curr->Left;
        } else if (cmp > 0) {
            curr = curr->Right;
        } else {
            return &curr->Val;
        }
    }
    return (int*)0;
}

int rbtree_contains(const RBTree *tree, const char *key) {
    return rbtree_get_ptr((RBTree*)tree, key) != (int*)0;
}

int rbtree_get(const RBTree *tree, const char *key, int *out_val) {
    int *p = rbtree_get_ptr((RBTree*)tree, key);
    if (p != (int*)0) {
        if (out_val) *out_val = *p;
        return 1;
    }
    return 0;
}

void rbtree_put(RBTree *tree, const char *key, int val) {
    int is_new = 0;
    tree->Root = rbtree_insert_node(tree->Root, key, val, &is_new);
    tree->Root->Color = RBTREE_BLACK;
    if (is_new) {
        tree->Len = tree->Len + 1;
    }
}

static void rbtree_print_str(const char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
    }
}

static void rbtree_print_int(int n) {
    if (n == 0) {
        putchar('0');
        return;
    }
    if (n < 0) {
        putchar('-');
        n = -n;
    }
    char buf[10];
    int i = 0;
    while (n > 0) {
        buf[i] = (char)('0' + (n % 10));
        i = i + 1;
        n = n / 10;
    }
    while (i > 0) {
        i = i - 1;
        putchar(buf[i]);
    }
}

static void rbtree_node_print_in_order(const RBTreeNode *n) {
    if (n == (const RBTreeNode*)0) return;
    rbtree_node_print_in_order(n->Left);
    rbtree_print_str(n->Key);
    putchar(' ');
    rbtree_print_int(n->Val);
    putchar('\n');
    rbtree_node_print_in_order(n->Right);
}

void rbtree_print(const RBTree *tree) {
    if (tree->Root != (RBTreeNode*)0) {
        rbtree_node_print_in_order(tree->Root);
    }
}

#endif /* RBTREE_C */
