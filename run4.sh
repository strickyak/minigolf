T=/tmp/$(echo "$1" | tr -c A-Za-z0-9 _)

go run main.go  -m=ir            "$@" > /tmp/ir

echo "[ C ] /tmp/c.c $T.c.out" >&2
go run main.go  -m=c -o=/tmp/c.c "$@"  &&  ( cd /tmp ; gcc -o c c.c ; ./c > $T.c.out )

echo "[ CBE ] /tmp/cbe.c $T.cbe.out" >&2
go run main.go  -m=cbe -o=/tmp/cbe.c "$@"  &&  ( cd /tmp ; gcc -o cbe cbe.c ; ./cbe > $T.cbe.out )

echo "[ X86_64 ] /tmp/x.s $T.x.out" >&2
go run main.go  -m=x -o=/tmp/x.s "$@"  &&  ( cd /tmp ; gcc -o x x.s ; ./x > $T.x.out )

echo "[ M6809 ] /tmp/m.s $T.m.out" >&2
go run main.go  -m=m -o=/tmp/m.s "$@"  &&  sh scripts/run-6809-at-4000.sh /tmp/m.s > $T.m.out


md5sum $T.c.out >&2
md5sum $T.cbe.out >&2
md5sum $T.x.out >&2
md5sum $T.m.out >&2

