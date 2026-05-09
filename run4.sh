T=_tmp/$(echo "$1" | tr -c A-Za-z0-9 _)
mkdir -p _tmp

echo "[ IR ] _tmp/ir" >&2
go run main.go  -m=ir            "$@" > _tmp/ir

echo "[ C ] _tmp/c.c $T.c.out" >&2
go run main.go  -m=c -o=_tmp/c.c "$@"  &&  ( cd _tmp ; gcc -o c c.c ; ./c > $T.c.out )

echo "[ CBE ] _tmp/cbe.c $T.cbe.out" >&2
go run main.go  -m=cbe -o=_tmp/cbe.c "$@"  &&  ( cd _tmp ; gcc -o cbe cbe.c ; ./cbe > $T.cbe.out )

echo "[ X86_64 ] _tmp/x.s $T.x.out" >&2
go run main.go  -m=x -o=_tmp/x.s "$@"  &&  ( cd _tmp ; gcc -o x x.s ; ./x > $T.x.out )

echo "[ M6809 ] _tmp/m.s $T.m.out" >&2
go run main.go  -m=m -o=_tmp/m.s "$@"  &&  sh scripts/run-6809-at-4000.sh _tmp/m.s > $T.m.out


echo `md5sum $T.c.out`   `wc < $T.c.out`   >&2
echo `md5sum $T.cbe.out` `wc < $T.cbe.out`   >&2
echo `md5sum $T.x.out`   `wc < $T.x.out`   >&2
echo `md5sum $T.m.out`   `wc < $T.m.out`   >&2
