#include "../common.h"

struct Point {
    int x;
    int y;
};

struct Rect {
    struct Point top_left;
    struct Point bottom_right;
};

static int rect_area(const struct Rect *r) {
    int width;
    int height;
    width = r->bottom_right.x - r->top_left.x;
    height = r->bottom_right.y - r->top_left.y;
    return width * height;
}

static int point_in_rect(const struct Rect *r, const struct Point *p) {
    if (p->x >= r->top_left.x && p->x <= r->bottom_right.x &&
        p->y >= r->top_left.y && p->y <= r->bottom_right.y) {
        return 1;
    }
    return 0;
}

int main(void) {
    struct Rect box;
    struct Point p1;
    struct Point p2;
    int area;
    int in1;
    int in2;

    box.top_left.x = 10;
    box.top_left.y = 20;
    box.bottom_right.x = 35;
    box.bottom_right.y = 50;

    p1.x = 20;
    p1.y = 30;

    p2.x = 5;
    p2.y = 25;

    area = rect_area(&box);
    in1 = point_in_rect(&box, &p1);
    in2 = point_in_rect(&box, &p2);

    put_num(area);
    putchar(' ');
    put_num(in1);
    putchar(' ');
    put_num(in2);
    putchar('\n');
    return 0;
}
