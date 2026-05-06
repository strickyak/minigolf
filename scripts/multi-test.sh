set -x

nicename() {
    echo -n $* | tr -c 'A-Za-z0-9' _
}

for src
do
    for mode in C CBE
    do
        go run main.go  -o /tmp/c.c -m $mode "$src"
        n=$(nicename $src $mode)
        ( cd /tmp && gcc c.c && ./a.out ) > /tmp/$n.out 2> /tmp/$n.err
        echo ==================================================
        wc /tmp/$n.err
        cat -n /tmp/$n.err
        echo ..................................................
        wc /tmp/$n.out
        cat -n /tmp/$n.out
        echo ================================================== YAK
    done
done
