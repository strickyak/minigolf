package prelude

const Source = `
package prelude

type uint = word
type string = slice[byte]

func peek[T any](addr word) T {
	return *((*T)(addr))
}

func poke[T any](addr word, value T) {
	*((*T)(addr)) = value
}

func peekb(addr word) byte { return peek[byte](addr) }
func peekw(addr word) word { return peek[word](addr) }

func pokeb(addr word, value byte) { poke[byte](addr, value) }
func pokew(addr word, value word) { poke[word](addr, value) }

type memslice struct {
	Base word
	Cap  word
	Len  word
}

func (o *memslice) Address(i word) word {
	if i >= o.Len {
		panic()
	}
	return o.Base + i
}

func (o *memslice) Get(i word) byte {
	if i >= o.Len {
		panic()
	}
	return peekb(o.Base + i)
}

func (o *memslice) Put(i word, x byte) {
	if i >= o.Len {
		panic()
	}
	pokeb(o.Base+i, x)
}

func (o *memslice) Chop(start word, limit word) memslice {
	if start > limit {
		panic()
	}
	if limit > o.Cap {
		panic()
	}
	var z memslice
	z.Base = o.Base
	z.Cap = z.Cap - start
	z.Len = limit - start
    return z
}

/////////////////////////////////////////////

type slice[T any] struct {
	guts memslice
}

func (o *slice[T]) Address(i word) word {
	p := i * sizeof[T]()
    return o.guts.Address(p)
}

func (o *slice[T]) Get(i word) T {
	p := i * sizeof[T]()
	if i >= o.guts.Len {
		panic()
	}
	return peek[T](o.guts.Base + p)
}

func (o *slice[T]) Put(i word, x T) {
	p := i * sizeof[T]()
	if i >= o.guts.Len {
		panic()
	}
	poke[T](o.guts.Base+p, x)
}

func (o *slice[T]) Chop(start word, limit word) slice[T] {
	p1 := start * sizeof[T]()
	p2 := limit * sizeof[T]()
	if p1 > p2 {
		panic()
	}
	if p2 > o.guts.Cap {
		panic()
	}
	var z slice[T]
	z.guts.Base = o.Base + p1
	z.guts.Cap = o.guts.Cap - p1
	z.guts.Len = p2 - p1
	return z
}

func panic() {
    println("\nPANIC\n")
    for {}
}
`
