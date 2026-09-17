#ifndef RBTREE_H
#define RBTREE_H

#define RBTREE_RED   1
#define RBTREE_BLACK 0

typedef struct RBTreeNode {
    const char *Key;
    int Val;
    int Color;
    struct RBTreeNode *Left;
    struct RBTreeNode *Right;
} RBTreeNode;

typedef struct RBTree {
    RBTreeNode *Root;
    int Len;
} RBTree;

void rbtree_init(RBTree *tree);
int rbtree_size(const RBTree *tree);
int rbtree_contains(const RBTree *tree, const char *key);
int* rbtree_get_ptr(RBTree *tree, const char *key);
int rbtree_get(const RBTree *tree, const char *key, int *out_val);
void rbtree_put(RBTree *tree, const char *key, int val);
void rbtree_print(const RBTree *tree);

#endif /* RBTREE_H */
