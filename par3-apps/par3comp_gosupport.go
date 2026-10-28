package main

import (
	"fmt"
	"io"
	"os"
	"unsafe"
)

type word uint16

// -----------------------------------------------------------------------------
// File I/O Support
// -----------------------------------------------------------------------------

var (
	goOpenFiles   = make(map[word]*os.File)
	goNextFileIdx = word(1)
)

func file_open_read(path string) (word, word) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	h := goNextFileIdx
	goNextFileIdx++
	goOpenFiles[h] = f
	return h, 1
}

func file_open_write(path string) (word, word) {
	f, err := os.Create(path)
	if err != nil {
		return 0, 0
	}
	h := goNextFileIdx
	goNextFileIdx++
	goOpenFiles[h] = f
	return h, 1
}

func file_readline(h word) (string, word) {
	f, ok := goOpenFiles[h]
	if !ok || f == nil {
		return "", 1
	}
	var buf []byte
	var b [1]byte
	for len(buf) < 255 {
		n, err := f.Read(b[:])
		if n > 0 {
			buf = append(buf, b[0])
			if b[0] == '\n' || b[0] == '\r' {
				break
			}
		}
		if err != nil {
			if err == io.EOF && len(buf) > 0 {
				return string(buf), 0
			}
			return "", 1
		}
	}
	if len(buf) == 0 {
		return "", 1
	}
	return string(buf), 0
}

func file_write(h word, s string) word {
	f, ok := goOpenFiles[h]
	if !ok || f == nil {
		return 0
	}
	_, err := f.Write([]byte(s))
	if err != nil {
		return 0
	}
	return 1
}

func file_close(h word) {
	if f, ok := goOpenFiles[h]; ok && f != nil {
		_ = f.Close()
		delete(goOpenFiles, h)
	}
}

// -----------------------------------------------------------------------------
// Hash Map & String Pool Support
// -----------------------------------------------------------------------------

type goMap struct {
	vals map[string]word
	strs map[string]string
}

var goMaps []*goMap

func map_new(buckets word) word {
	m := &goMap{
		vals: make(map[string]word, buckets),
		strs: make(map[string]string, buckets),
	}
	idx := word(len(goMaps))
	goMaps = append(goMaps, m)
	return idx
}

func map_get(m word, key string) word {
	if int(m) >= len(goMaps) {
		return 0xFFFF
	}
	if v, ok := goMaps[m].vals[key]; ok {
		return v
	}
	return 0xFFFF
}

func map_put(m word, key string, val word) word {
	if int(m) >= len(goMaps) {
		return 0
	}
	k := string([]byte(key))
	goMaps[m].vals[k] = val
	return 1
}

func map_str(m word, key string) string {
	if int(m) >= len(goMaps) {
		return string([]byte(key))
	}
	if s, ok := goMaps[m].strs[key]; ok {
		return s
	}
	k := string([]byte(key))
	goMaps[m].strs[k] = k
	return k
}

func map_count(m word) word {
	if int(m) >= len(goMaps) {
		return 0
	}
	return word(len(goMaps[m].vals))
}

// -----------------------------------------------------------------------------
// Memory & String Support
// -----------------------------------------------------------------------------

func mk_str(base *byte, cap word, length word) string {
	if base == nil || length == 0 {
		return ""
	}
	return string(unsafe.Slice(base, length))
}

func free_str(s string) {
	// No-op in Go (garbage collected)
}

func alloc(sz word) word {
	return 0
}

func free(addr word) {
}

func peek(addr word) word {
	return 0
}

func poke(addr word, val word) {
}

func peekb(addr word) byte {
	return 0
}

func pokeb(addr word, val byte) {
}

// -----------------------------------------------------------------------------
// System & CLI Support
// -----------------------------------------------------------------------------

func sys_args() []string {
	if len(os.Args) <= 1 {
		return nil
	}
	return os.Args[1:]
}

func get_arg(args []string, idx word) string {
	if int(idx) < 0 || int(idx) >= len(args) {
		return ""
	}
	return args[idx]
}

func sys_exit(code word) {
	os.Exit(int(code))
}

func print(args ...any) {
	for _, a := range args {
		fmt.Fprint(os.Stdout, a)
	}
}

func println(args ...any) {
	for i, a := range args {
		if i > 0 {
			fmt.Fprint(os.Stdout, " ")
		}
		fmt.Fprint(os.Stdout, a)
	}
	fmt.Fprintln(os.Stdout)
}
