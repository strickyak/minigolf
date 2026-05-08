T=/tmp/$(echo "$1" | tr -c A-Za-z0-9 _)

echo "[ C ] $T.c.out" >&2
go run main.go  -m=c -o=/tmp/c.c "$@"  &&  ( cd /tmp ; gcc -o c c.c ; ./c > $T.c.out )
md5sum $T.c.out >&2

echo "[ C ] $T.cbe.out" >&2
go run main.go  -m=cbe -o=/tmp/cbe.c "$@"  &&  ( cd /tmp ; gcc -o cbe cbe.c ; ./cbe > $T.cbe.out )
md5sum $T.cbe.out >&2

echo "[ X ] $T.x.out" >&2
go run main.go  -m=x -o=/tmp/x.s "$@"  &&  ( cd /tmp ; gcc -o x x.s ; ./x > $T.x.out )
md5sum $T.x.out >&2

echo "[ M ] $T.m.out" >&2
go run main.go  -m=m -o=/tmp/m.s "$@"  &&  sh scripts/run*.sh /tmp/m.s > $T.m.out
md5sum $T.m.out >&2

