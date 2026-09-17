T=$PWD/_tmp/$(echo "$1" | tr -c A-Za-z0-9 _)
mkdir -p _tmp
rm -f $T.*.out

echo "[ IR ] _tmp/ir" >&2
(set -x; go run main.go  -I=c-tests -I=demos -I=demos/floating -I=golflib -m=ir -o=_tmp/ir  "$@" )

echo "[ CBE ] _tmp/cbe.c $T.cbe.out" >&2
(set -x; go run main.go  -I=c-tests -I=demos -I=demos/floating -I=golflib -m=cbe -o=_tmp/cbe.c "$@"  &&  ( cd _tmp ; gcc -O1 -g -o cbe cbe.c ; ./cbe > $T.cbe.out ) )

echo "[ AMD64 ] _tmp/amd64.s $T.amd64.out" >&2
(set -x; go run main.go  -I=c-tests -I=demos -I=demos/floating -I=golflib -m=amd64 -o=_tmp/amd64.s "$@"  &&  ( cd _tmp ; gcc -O1 -g -o amd64 amd64.s ; ./amd64 > $T.amd64.out ) )

echo "[ M6809 ] _tmp/m.s $T.m.out" >&2
# go run main.go  -I=demos -I=golflib -m=m -o=_tmp/m.s "$@"  &&  sh scripts/run-6809-at-4000.sh _tmp/m.s > $T.m.out
(set -x; sh run9.sh "$@" > $T.m.out )


for x in cbe amd64 m
do
    echo ==== $T.$x.out ====
    cat -n $T.$x.out
done
echo ========

echo `grep -v '^#' $T.cbe.out   | md5sum `   `wc $T.cbe.out`   >&2
echo `grep -v '^#' $T.amd64.out | md5sum `   `wc $T.amd64.out` >&2
echo `grep -v '^#' $T.m.out     | md5sum `   `wc $T.m.out`     >&2
