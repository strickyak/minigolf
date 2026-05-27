T=$PWD/_tmp/$(echo "$1" | tr -c A-Za-z0-9 _)
mkdir -p _tmp
rm -f $T.*.out

echo "[ IR ] _tmp/ir" >&2
(set -x; go run main.go  -I=golflib -m=ir -o=_tmp/ir  "$@" )

echo "[ CBE ] _tmp/cbe.c $T.cbe.out" >&2
(set -x; go run main.go  -I=golflib -m=cbe -o=_tmp/cbe.c "$@"  &&  ( cd _tmp ; gcc -O1 -g -o cbe cbe.c ; ./cbe > $T.cbe.out ) )

echo "[ X86_64 ] _tmp/x.s $T.x.out" >&2
(set -x; go run main.go  -I=golflib -m=x -o=_tmp/x.s "$@"  &&  ( cd _tmp ; gcc -O1 -g -o x x.s ; ./x > $T.x.out ) )

echo "[ M6809 ] _tmp/m.s $T.m.out" >&2
# go run main.go  -I=golflib -m=m -o=_tmp/m.s "$@"  &&  sh scripts/run-6809-at-4000.sh _tmp/m.s > $T.m.out
(set -x; sh run9.sh "$@" > $T.m.out )


for x in cbe x m
do
    echo ==== $T.$x.out ====
    cat -n $T.$x.out
done
echo ========

echo `md5sum $T.cbe.out` `wc < $T.cbe.out`   >&2
echo `md5sum $T.x.out`   `wc < $T.x.out`   >&2
echo `md5sum $T.m.out`   `wc < $T.m.out`   >&2
