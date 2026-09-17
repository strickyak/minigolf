#include "btree.h"
#include "btree.c"
#include "rbtree.h"
#include "rbtree.c"

extern void* malloc(int n);
extern void free(void* p);
extern void putchar(char ch);

static const char hamlet[] =
    "O that this too too solid flesh would melt,\n"
    "Thaw, and resolve itself into a dew!\n"
    "Or that the Everlasting had not fix'd\n"
    "His canon 'gainst self-slaughter! O God! God!\n"
    "How weary, stale, flat, and unprofitable\n"
    "Seem to me all the uses of this world!\n"
    "Fie on't! ah, fie! 'Tis an unweeded garden\n"
    "That grows to seed; things rank and gross in nature\n"
    "Possess it merely. That it should come to this!\n"
    "But two months dead! Nay, not so much, not two.\n"
    "So excellent a king, that was to this\n"
    "Hyperion to a satyr; so loving to my mother\n"
    "That he might not beteem the winds of heaven\n"
    "Visit her face too roughly. Heaven and earth!\n"
    "Must I remember? Why, she would hang on him\n"
    "As if increase of appetite had grown\n"
    "By what it fed on; and yet, within a month-\n"
    "Let me not think on't! Frailty, thy name is woman!-\n"
    "A little month, or ere those shoes were old\n"
    "With which she followed my poor father's body\n"
    "Like Niobe, all tears- why she, even she\n"
    "(O God! a beast that wants discourse of reason\n"
    "Would have mourn'd longer) married with my uncle;\n"
    "My father's brother, but no more like my father\n"
    "Than I to Hercules. Within a month,\n"
    "Ere yet the salt of most unrighteous tears\n"
    "Had left the flushing in her galled eyes,\n"
    "She married. O, most wicked speed, to post\n"
    "With such dexterity to incestuous sheets!\n"
    "It is not, nor it cannot come to good.\n"
    "But break my heart, for I must hold my tongue!\n";

static int is_word_char(char c) {
    if (c >= 'a' && c <= 'z') return 1;
    if (c >= 'A' && c <= 'Z') return 1;
    if (c == 39) return 1;
    return 0;
}

static char to_lower(char c) {
    if (c >= 'A' && c <= 'Z') return (char)(c + 32);
    return c;
}

static void print_str(const char *s) {
    while (*s) {
        putchar(*s);
        s = s + 1;
    }
}

static void test_btree(void) {
    print_str("--- B-Tree ---\n");
    BTree tree;
    btree_init(&tree);

    int i = 0;
    while (hamlet[i] != '\0') {
        while (hamlet[i] != '\0' && !is_word_char(hamlet[i])) {
            i = i + 1;
        }
        if (hamlet[i] == '\0') break;
        int start = i;
        while (hamlet[i] != '\0' && is_word_char(hamlet[i])) {
            i = i + 1;
        }
        int wlen = i - start;
        char *w = (char*)malloc(wlen + 1);
        int j;
        for (j = 0; j < wlen; j = j + 1) {
            w[j] = to_lower(hamlet[start + j]);
        }
        w[wlen] = '\0';

        int *ptr = btree_get_ptr(&tree, w);
        if (ptr != (int*)0) {
            *ptr = *ptr + 1;
            free(w);
        } else {
            btree_put(&tree, w, 1);
        }
    }

    btree_print(&tree);
}

static void test_rbtree(void) {
    print_str("--- Red-Black Tree ---\n");
    RBTree tree;
    rbtree_init(&tree);

    int i = 0;
    while (hamlet[i] != '\0') {
        while (hamlet[i] != '\0' && !is_word_char(hamlet[i])) {
            i = i + 1;
        }
        if (hamlet[i] == '\0') break;
        int start = i;
        while (hamlet[i] != '\0' && is_word_char(hamlet[i])) {
            i = i + 1;
        }
        int wlen = i - start;
        char *w = (char*)malloc(wlen + 1);
        int j;
        for (j = 0; j < wlen; j = j + 1) {
            w[j] = to_lower(hamlet[start + j]);
        }
        w[wlen] = '\0';

        int *ptr = rbtree_get_ptr(&tree, w);
        if (ptr != (int*)0) {
            *ptr = *ptr + 1;
            free(w);
        } else {
            rbtree_put(&tree, w, 1);
        }
    }

    rbtree_print(&tree);
}

int main() {
    test_btree();
    test_rbtree();
    return 0;
}
