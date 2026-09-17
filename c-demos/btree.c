#ifndef BTREE_C
#define BTREE_C

#include "btree.h"

extern void* zalloc(int n);
extern void putchar(char ch);

static int btree_strcmp(const char *s1, const char *s2) {
    while (*s1 != '\0' && (*s1 == *s2)) {
        s1 = s1 + 1;
        s2 = s2 + 1;
    }
    return (int)((unsigned char)*s1) - (int)((unsigned char)*s2);
}

static BTreeNode* btree_alloc_node(int is_leaf) {
    BTreeNode *p = (BTreeNode*)zalloc((int)sizeof(BTreeNode));
    p->IsLeaf = is_leaf;
    p->NumKeys = 0;
    return p;
}

static const char* btree_node_get_key(const BTreeNode *n, int i) {
    if (i == 0) return n->Key0;
    if (i == 1) return n->Key1;
    return n->Key2;
}

static void btree_node_set_key(BTreeNode *n, int i, const char *k) {
    if (i == 0) { n->Key0 = k; return; }
    if (i == 1) { n->Key1 = k; return; }
    n->Key2 = k;
}

static int btree_node_get_val(const BTreeNode *n, int i) {
    if (i == 0) return n->Val0;
    if (i == 1) return n->Val1;
    return n->Val2;
}

static int* btree_node_get_val_ptr(BTreeNode *n, int i) {
    if (i == 0) return &n->Val0;
    if (i == 1) return &n->Val1;
    return &n->Val2;
}

static void btree_node_set_val(BTreeNode *n, int i, int v) {
    if (i == 0) { n->Val0 = v; return; }
    if (i == 1) { n->Val1 = v; return; }
    n->Val2 = v;
}

static BTreeNode* btree_node_get_child(const BTreeNode *n, int i) {
    if (i == 0) return n->Child0;
    if (i == 1) return n->Child1;
    if (i == 2) return n->Child2;
    return n->Child3;
}

static void btree_node_set_child(BTreeNode *n, int i, BTreeNode *c) {
    if (i == 0) { n->Child0 = c; return; }
    if (i == 1) { n->Child1 = c; return; }
    if (i == 2) { n->Child2 = c; return; }
    n->Child3 = c;
}

void btree_init(BTree *tree) {
    tree->Root = (BTreeNode*)0;
    tree->Len = 0;
}

int btree_size(const BTree *tree) {
    return tree->Len;
}

int* btree_get_ptr(BTree *tree, const char *key) {
    BTreeNode *node = tree->Root;
    while (node != (BTreeNode*)0) {
        int i = 0;
        while (i < node->NumKeys && btree_strcmp(key, btree_node_get_key(node, i)) > 0) {
            i = i + 1;
        }
        if (i < node->NumKeys && btree_strcmp(key, btree_node_get_key(node, i)) == 0) {
            return btree_node_get_val_ptr(node, i);
        }
        if (node->IsLeaf) {
            return (int*)0;
        }
        node = btree_node_get_child(node, i);
    }
    return (int*)0;
}

int btree_contains(const BTree *tree, const char *key) {
    return btree_get_ptr((BTree*)tree, key) != (int*)0;
}

int btree_get(const BTree *tree, const char *key, int *out_val) {
    int *p = btree_get_ptr((BTree*)tree, key);
    if (p != (int*)0) {
        if (out_val) *out_val = *p;
        return 1;
    }
    return 0;
}

static void btree_split_child(BTreeNode *parent, int i) {
    BTreeNode *y = btree_node_get_child(parent, i);
    BTreeNode *z = btree_alloc_node(y->IsLeaf);
    z->NumKeys = 1;
    btree_node_set_key(z, 0, btree_node_get_key(y, 2));
    btree_node_set_val(z, 0, btree_node_get_val(y, 2));

    if (!y->IsLeaf) {
        btree_node_set_child(z, 0, btree_node_get_child(y, 2));
        btree_node_set_child(z, 1, btree_node_get_child(y, 3));
        btree_node_set_child(y, 2, (BTreeNode*)0);
        btree_node_set_child(y, 3, (BTreeNode*)0);
    }

    const char *med_key = btree_node_get_key(y, 1);
    int med_val = btree_node_get_val(y, 1);

    y->NumKeys = 1;

    int j = parent->NumKeys;
    while (j > i) {
        btree_node_set_child(parent, j + 1, btree_node_get_child(parent, j));
        j = j - 1;
    }
    btree_node_set_child(parent, i + 1, z);

    j = parent->NumKeys;
    while (j > i) {
        btree_node_set_key(parent, j, btree_node_get_key(parent, j - 1));
        btree_node_set_val(parent, j, btree_node_get_val(parent, j - 1));
        j = j - 1;
    }
    btree_node_set_key(parent, i, med_key);
    btree_node_set_val(parent, i, med_val);
    parent->NumKeys = parent->NumKeys + 1;
}

static void btree_insert_non_full(BTreeNode *node, const char *key, int val) {
    if (node->IsLeaf) {
        int i = node->NumKeys;
        while (i > 0 && btree_strcmp(key, btree_node_get_key(node, i - 1)) < 0) {
            btree_node_set_key(node, i, btree_node_get_key(node, i - 1));
            btree_node_set_val(node, i, btree_node_get_val(node, i - 1));
            i = i - 1;
        }
        btree_node_set_key(node, i, key);
        btree_node_set_val(node, i, val);
        node->NumKeys = node->NumKeys + 1;
        return;
    }

    int i = node->NumKeys;
    while (i > 0 && btree_strcmp(key, btree_node_get_key(node, i - 1)) < 0) {
        i = i - 1;
    }
    BTreeNode *child = btree_node_get_child(node, i);
    if (child->NumKeys == 3) {
        btree_split_child(node, i);
        if (btree_strcmp(key, btree_node_get_key(node, i)) > 0) {
            i = i + 1;
        }
    }
    btree_insert_non_full(btree_node_get_child(node, i), key, val);
}

void btree_put(BTree *tree, const char *key, int val) {
    int *p = btree_get_ptr(tree, key);
    if (p != (int*)0) {
        *p = val;
        return;
    }

    tree->Len = tree->Len + 1;
    if (tree->Root == (BTreeNode*)0) {
        tree->Root = btree_alloc_node(1);
        tree->Root->NumKeys = 1;
        btree_node_set_key(tree->Root, 0, key);
        btree_node_set_val(tree->Root, 0, val);
        return;
    }

    if (tree->Root->NumKeys == 3) {
        BTreeNode *s = btree_alloc_node(0);
        btree_node_set_child(s, 0, tree->Root);
        btree_split_child(s, 0);
        tree->Root = s;
    }
    btree_insert_non_full(tree->Root, key, val);
}

static void btree_print_str(const char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
    }
}

static void btree_print_int(int n) {
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

static void btree_node_print_in_order(const BTreeNode *n) {
    if (n == (const BTreeNode*)0) return;
    int i;
    for (i = 0; i < n->NumKeys; i = i + 1) {
        if (!n->IsLeaf) {
            btree_node_print_in_order(btree_node_get_child(n, i));
        }
        btree_print_str(btree_node_get_key(n, i));
        putchar(' ');
        btree_print_int(btree_node_get_val(n, i));
        putchar('\n');
    }
    if (!n->IsLeaf) {
        btree_node_print_in_order(btree_node_get_child(n, n->NumKeys));
    }
}

void btree_print(const BTree *tree) {
    if (tree->Root != (BTreeNode*)0) {
        btree_node_print_in_order(tree->Root);
    }
}

#endif /* BTREE_C */
