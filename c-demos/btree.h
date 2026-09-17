#ifndef BTREE_H
#define BTREE_H

typedef struct BTreeNode {
    int NumKeys;
    int IsLeaf;
    const char *Key0;
    const char *Key1;
    const char *Key2;
    int Val0;
    int Val1;
    int Val2;
    struct BTreeNode *Child0;
    struct BTreeNode *Child1;
    struct BTreeNode *Child2;
    struct BTreeNode *Child3;
} BTreeNode;

typedef struct BTree {
    BTreeNode *Root;
    int Len;
} BTree;

void btree_init(BTree *tree);
int btree_size(const BTree *tree);
int btree_contains(const BTree *tree, const char *key);
int* btree_get_ptr(BTree *tree, const char *key);
int btree_get(const BTree *tree, const char *key, int *out_val);
void btree_put(BTree *tree, const char *key, int val);
void btree_print(const BTree *tree);

#endif /* BTREE_H */
