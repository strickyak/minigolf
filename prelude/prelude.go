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

/////////////////////////////////////////////

func clear[T any](p *T) {
    addr := word(p)
    for i := range sizeof[T]() {
        pokeb(addr + i, 0)
    }
}
func memset(p *byte, x byte, n word) {
    addr := word(p)
    for i := range n {
        pokeb(i + addr, x)
    }
}

func memcmp(a *byte, b *byte, n word) int {
	for i := word(0); i < n; i++ {
		va := peekb(word(a) + i)
		vb := peekb(word(b) + i)
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func memeq(a *byte, b *byte, n word) byte {
	for i := word(0); i < n; i++ {
		va := peekb(word(a) + i)
		vb := peekb(word(b) + i)
		if va != vb {
			return 0
		}
	}
	return 1
}
func strcmp(a string, b string) int {
	n := a.Len
	if b.Len < n {
		n = b.Len
	}
	c := memcmp((*byte)(a.Base), (*byte)(b.Base), n)
	if c != 0 {
		return c
	}
	if a.Len < b.Len {
		return -1
	}
	if a.Len > b.Len {
		return 1
	}
	return 0
}

func streq(a string, b string) byte {
	if a.Len != b.Len {
		return 0
	}
	return memeq((*byte)(a.Base), (*byte)(b.Base), a.Len)
}

/////////////////////////////////////////////

type any struct {
	BaseAddr word
	TypeStr  word
}

func printany(x any) {
	ch := peekb(x.TypeStr)
	if ch == 'b' {
		// byte
		println("b=", peek[byte](x.BaseAddr))
	} else if ch == 'w' {
		// word
		println("w=", peek[word](x.BaseAddr))
	} else if ch == 's' {
		// string
		println("s=", peek[string](x.BaseAddr))
	} else {
		println("?", ch, "?")
	}
}

/////////////////////////////////////////////

type slice[T any] struct {
	Base word
	Cap  word
	Len  word
}

func (o *slice[T]) Append(x T) {
    if o.Base == 0 {
        o.Base = word(zalloc(mul_word(8 , sizeof[T]())))
        o.Cap = 8
        o.Len = 1
        o.Put(0, x)
        return
    }
    n := o.Len
    if n+1 < o.Cap {
        o.Len++
        o.Put(n, x)
        return
    }

    // Must re-alloc
    var z slice[T]
    z.Cap = o.Cap + o.Cap
    if z.Cap < 8 {
        z.Cap = 8
    }
    z.Base = word(zalloc(mul_word(z.Cap , sizeof[T]())))
    z.Len = n+1

    for i := range n {
        z.Put(i, o.Get(i))
    }
    z.Put(n, x)
    free((*byte)(o.Base))
    *o = z
}

func (o *slice[T]) Address(i word) word {
	if i >= o.Len {
		panic(2001)
	}
	p := mul_word(i , sizeof[T]())
	return o.Base + p
}

func (o *slice[T]) Get(i word) T {
	p := mul_word(i , sizeof[T]())
	if i >= o.Len {
		panic(2002)
	}
	return peek[T](o.Base + p)
}

func (o *slice[T]) Put(i word, x T) {
	p := mul_word(i , sizeof[T]())
	if i >= o.Len {
		panic(2003)
	}
	poke[T](o.Base+p, x)
}

func (o *slice[T]) Chop(start word, limit word) slice[T] {
	if start > limit {
		panic(2004)
	}
	if limit > o.Cap {
		panic(2005)
	}
	var z slice[T]
	z.Base = o.Base + start*sizeof[T]()
	z.Len = limit - start
	z.Cap = z.Len
	return z
}

// makeslice[T](n) is like make([]T, n) in golang.
func makeslice[T any](n word) slice[T] {
    var z slice[T]
    if n == 0 {
        return z
    }
    z.Base := zalloc( n * sizeof[T]() )
    z.Len = n
    z.Cap = n
    return z
}

// freeslice frees a malloced slice's contents, if it was made by Appending a zero slice, or by makeslice.
func freeslice[T](a slice[T]) {
	p := (*byte)(a.Base)
	free(p)
}

/////////////////////////////////////////////

func panic(w word) {
	println("\n*PANIC* why=", w)
	exit(13)
}

/////////////////////////////////////////////

func mul_byte(a byte, b byte) word

func mul_word(a word, b word) word {
    a_H := byte(a >> 8)
    a_L := byte(a)
    b_H := byte(b >> 8)
    b_L := byte(b)
    
    cross1 := mul_byte(a_H, b_L)
    cross2 := mul_byte(a_L, b_H)
    crossSum := cross1 + cross2
    crossSumShifted := crossSum << 8
    
    low := mul_byte(a_L, b_L)
    
    return crossSumShifted + low
}

func div_word(a0 word, b word) word {
    if b == 0 {
        panic(1002)
    }
    var q word
    var r word
    for i := range 16 {
        bit_idx := word(15) - i
        r = r << 1
        bit := (a0 >> bit_idx) & 1
        r = r | bit
        if r >= b {
            r = r - b
            q = q | (1 << bit_idx)
        }
    }
    return q
}

func mod_word(a0 word, b word) word {
    if b == 0 {
        panic(1004)
    }
    var r word
    for i := range 16 {
        bit_idx := word(15) - i
        r = r << 1
        bit := (a0 >> bit_idx) & 1
        r = r | bit
        if r >= b {
            r = r - b
        }
    }
    return r
}

/////////////////////////////////////////////

// strdup uses the arg's Len and allocates a copy of the string, with NUL-termination past its Len
func strdup(a string) string {
	n := a.Len
	p := word(malloc(n + 1))
	for i := range n {
		pokeb(p+i, a[i])
	}
	pokeb(p+n, 0) // NUL-terminate.

	var z string
	z.Base = p
	z.Len = n
	z.Cap = n + 1
	return z
}

func strfree(a string) {
	p := (*byte)(a.Base)
	free(p)
}

/////////////////////////////////////////////

type MallocHeader struct {
	next *MallocHeader
	size word
}

const HEAP_SIZE = 2 * 1024 * sizeof[word]() // 4K on M6809

var Heap [HEAP_SIZE]byte

var base MallocHeader
var freep *MallocHeader

func malloc_init(heap_start *byte, heap_size word) {
	var p *MallocHeader = (*MallocHeader)(word(heap_start))

	// Calculate how many MallocHeader-sized units fit in the heap
	p.size = div_word(heap_size, sizeof[MallocHeader]())
	p.next = &base

	base.next = p
	base.size = 0
	freep = &base
}

const TOO_BIG = 4000 // Assume an error, if malloc more than this big.

// zalloc allocates zeroed memory using alloc
func zalloc(nbytes word) *byte {
    p := word(malloc(nbytes))
    for i := range nbytes {
        pokeb(p+i, 0)
    }
    return (*byte)(p)
}
func malloc(nbytes word) *byte {
	var p *MallocHeader
	var prevp *MallocHeader
	var nunits word

	if nbytes > TOO_BIG {
		panic(nbytes)
	}

	// Calculate how many MallocHeader units we need (including the 1 unit for metadata)
	nunits = div_word((nbytes+sizeof[MallocHeader]()-1), sizeof[MallocHeader]()) + 1

	prevp = freep
	if word(prevp) == 0 {
		panic(4001)
		return (*byte)(0) // Heap wasn't initialized!
	}

	// Traverse the circularly linked free list
	p = prevp.next
	for {
		if p.size >= nunits { // First-fit strategy
			if p.size == nunits {
				// Exactly the right size; splice it out of the free list
				prevp.next = p.next
			} else {
				// Block is bigger than needed; allocate from the tail end
				p.size = p.size - nunits
				p = (*MallocHeader)(word(p) + mul_word(p.size, sizeof[MallocHeader]()))
				p.size = nunits
			}
			freep = prevp
			// Return pointer to user space (skipping past the MallocHeader)
			return (*byte)(word(p) + sizeof[MallocHeader]())
		}
		if word(p) == word(freep) {
			panic(4002)
			return (*byte)(0) // Wrapped around the list: Out of Memory!
		}
		prevp = p
		p = p.next
	}
	panic(4003)
	return (*byte)(0)
}

func free(ap *byte) {
	var bp *MallocHeader
	var p *MallocHeader

	if word(ap) == 0 {
		return
	}

	// Move pointer backward to find the actual header
	bp = (*MallocHeader)(word(ap) - sizeof[MallocHeader]())

	// Find the right chronological spot in the free list to insert it
	var done byte = 0
	p = freep
	for done == 0 {
		if word(bp) > word(p) && word(bp) < word(p.next) {
			done = 1
		} else if word(p) >= word(p.next) && (word(bp) > word(p) || word(bp) < word(p.next)) {
			done = 1 // Insert at the extreme beginning or end of the list
		}

		if done == 0 {
			p = p.next
		}
	}

	// Coalesce (merge) with the next block if they are physically adjacent
	if word(bp)+mul_word(bp.size, sizeof[MallocHeader]()) == word(p.next) {
		bp.size = bp.size + p.next.size
		bp.next = p.next.next
	} else {
		bp.next = p.next
	}

	// Coalesce (merge) with the previous block if they are physically adjacent
	if word(p)+mul_word(p.size, sizeof[MallocHeader]()) == word(bp) {
		p.size = p.size + bp.size
		p.next = bp.next
	} else {
		p.next = bp
	}

	freep = p
}

func init() {
	malloc_init(&Heap[0], HEAP_SIZE)
}

/////////////////////////////////////////////
`
