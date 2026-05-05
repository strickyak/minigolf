set -ex

nicename() {
    echo -n $* | tr -c 'A-Za-z0-9' _
}

for src
do
    for mode in C CBE
    do
        go run main.go  -o /tmp/c.c -m $mode "$src"
        ( cd /tmp && gcc c.c && ./a.out ) > /tmp/$(nicename $src-$mode).out 2> /tmp/$(nicename $src-$mode).err
        echo ==================================================
        wc /tmp/$(nicename $src-$mode).err
        cat -n /tmp/$(nicename $src-$mode).err
        echo ..................................................
        wc /tmp/$(nicename $src-$mode).out
        cat -n /tmp/$(nicename $src-$mode).out
        echo ================================================== YAK
    done
done
