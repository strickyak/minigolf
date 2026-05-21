#include <stddef.h>
#include <stdint.h>

// On 8-bit systems, standard size_t is usually 16-bit, but let's be explicit.
typedef uint16_t uint_mem_t; 

// The header structure for each memory block. 
// Packed tightly to consume exactly 4 bytes when free.
typedef struct Header {
    struct Header *next;    // Pointer to the next free block
    uint_mem_t size;        // Size of the block (in multiples of Header size)
} Header;

static Header base;          // Minimal initial empty list to kickstart allocations
static Header *freep = NULL; // Points to the start of the free list

// Initialize your heap by giving it a raw block of memory
void malloc_init(void *heap_start, size_t heap_size) {
    Header *p = (Header *)heap_start;
    
    // Calculate how many Header-sized units fit in the heap
    p->size = heap_size / sizeof(Header);
    p->next = &base;
    
    base.next = p;
    base.size = 0;
    freep = &base;
}

void *malloc(size_t nbytes) {
    Header *p, *prevp;
    uint_mem_t nunits;

    // Calculate how many Header units we need (including the 1 unit for metadata)
    nunits = (nbytes + sizeof(Header) - 1) / sizeof(Header) + 1;
    
    if ((prevp = freep) == NULL) {
        return NULL; // Heap wasn't initialized!
    }

    // Traverse the circularly linked free list
    for (p = prevp->next; ; prevp = p, p = p->next) {
        if (p->size >= nunits) { // First-fit strategy
            if (p->size == nunits) {
                // Exactly the right size; splice it out of the free list
                prevp->next = p->next;
            } else {
                // Block is bigger than needed; allocate from the tail end
                p->size -= nunits;
                p += p->size;
                p->size = nunits;
            }
            freep = prevp;
            // Return pointer to user space (skipping past the 4-byte header)
            return (void *)(p + 1);
        }
        if (p == freep) { 
            return NULL; // Wrapped around the list: Out of Memory!
        }
    }
}

void free(void *ap) {
    Header *bp, *p;

    if (!ap) return;

    // Move pointer backward to find the actual header
    bp = (Header *)ap - 1; 

    // Find the right chronological spot in the free list to insert it
    for (p = freep; !(bp > p && bp < p->next); p = p->next) {
        if (p >= p->next && (bp > p || bp < p->next)) {
            break; // Insert at the extreme beginning or end of the list
        }
    }

    // Coalesce (merge) with the next block if they are physically adjacent
    if (bp + bp->size == p->next) {
        bp->size += p->next->size;
        bp->next = p->next->next;
    } else {
        bp->next = p->next;
    }

    // Coalesce (merge) with the previous block if they are physically adjacent
    if (p + p->size == bp) {
        p->size += bp->size;
        p->next = bp->next;
    } else {
        p->next = bp;
    }
    
    freep = p;
}
