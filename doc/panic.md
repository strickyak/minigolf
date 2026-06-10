MiniGolf will use a builtin `panic` function with this prototype:

```
    func panic(*byte) panicked
```

`panicked` is a builtin type that indicates that under all
situations, the function will never return except with an
"active panic".   Like the builtin `void` type, this type does not
have any values.  User-defined functions can return panicked, like this:
`func log.Panicf(format string, args ...any) panicked`.

An builtin global variable `var _panic_ *byte` will be nonzero
if there is an "active panic".

The panic() function may be called explicity from user code
in MiniGolf, or intrensically to indicate a violation was
detected.  The MiniGolf compiler does not check for these things yet,
but examples might be using a nil pointer, calling a function
via a nil reference, invoking a method on nil, and slice indices
out-of-bounds.

Intrensic calls to panic() will always pass the address of an
ASCII NUL-terminated C string, such as "BS\0" for Bad Subscript,
or "NP\0" for null pointer.  But this detail is not required for
actually handling panics.

## Panics are optional.

There should be a compiler flags to enable intrensic panics:

```
    --bounds_checks
    --nil_checks
```

If none of these flags are specified, and there are no calls
to panic() in the trimmed IR code, then the entire panic
infrastructure is NOT emitted.

## SETJMP / LONGJUMP are used.

Assume two C functions will be available at link-time:

```
       typedef jmp_buf char[N];

       int setjmp(jmp_buf env);

       [[noreturn]] void longjmp(jmp_buf env, int val);
```

On X86_64, N is 200.   On M6809, N is 16.
In C, `#include <setjmp.h>`

## PANIC INFRASTRUCTURE.

* Global C type `struct jmp_struct { jmp_buf jmpbuf; struct jmp_struct *prev; };`
* Global C variable `char* _panic_;`
* Global C variable `struct jmp_struct *_jmp_chain_;`
* Global C function `void abort() { /* core dump or exit non-zero */ }`
* Global C function `void aborts(const char* s) { printf("\n*** ABORT\n"); printf("\n*** %s\n", s); abort(); }`

On panic(w word),

* if `_panic_` is nonzero, `aborts("DOUBLE_PANIC");`
* w is stored in `_panic_`
* if `_jmp_chain_` is nonzero, `longjmp(_jmp_chain_->jmpbuf, 1);`
* else `abort("EMPTY_CHAIN");`

In any function compiled with one or more DeferredActions,
emit this pseudo-c code at the TOP of the function.

```
    struct jmp_struct jumper;
    { int val = setjmp(jumper.jmpbuf);
      if (val) {
        // Panicking got us here.
        goto DEFERRED_ACTION;
      } else {
        // Normal function execution got us here.
        // Link the jmp_struct.
        if (_jmp_chain) {
          buf_i.prev = _jmp_chain_;
          _jmp_chain_ = &jumper;
        } else {
          abort();
        }
      }
    }
```

Remember our current limitation, that deferred actions cannot call panic.
If a deferred action calls a function that calls panic, then panic will see that the `_panic_`
variable is already set, and it will `aborts("DOUBLE_PANIC")`.

So we can trust that all deferred actions will succeed, or else we abort().

In any function compiled with one or more DeferredActions,
emit this pseudo-c code at the BOTTOM of the function,
before the actual "C-style return" of the return value.

```
   if (_panic_) {
        if (_jmp_chain_) {
            longjmpm(_jump_chain_->jmpbuf, 1);
        } else {
            abort("EMPTY_RE_CHAIN");
        }
   }
   // Existing return statement:
   return ret_val;
```

Finally, if Panic Infrastructure is being emitted,
the initial C `main()` (which calls MiniGolf main `f_main()`)
should begin by calling `setjmp(&_main_jumper_.jmpbuf)`.
`_main_jumper_` is allocated as a global.
If that setjmp returns nonzero, call aborts(_panic_).



