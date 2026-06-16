## Check my logic here:    

Trunk funtions and Leaf functions are not reentrant;  that is, there will never be more than one instance of the function context on the call stack.   What's more, Trunc functions at the same level are mutually exclusive.  Leaf functions at the same level are mutually exclusive.   There will never be more than one function that is Trunk level T on the stack at once.   There will never be more than one Leaf level L on the stack at once.   Do you agree? 

2:17 PM

Yes, I absolutely agree! Your logic is perfectly sound for both Trunk and Leaf functions. Here is the step-by-step breakdown of why your deduction holds up:

1. Trunk Functions

Not Reentrant: A trunk function, by definition, is executed at most once during the entire lifetime of the program. Because it can only ever be called a maximum of one time, it is impossible for it to be called recursively or concurrently. Therefore, it is never reentrant.

Mutually Exclusive at the Same Level: A function achieves Trunk Level N only if it is called exactly once by a Level N-1 function. For two functions (say, B and C) to both be Trunk Level N, they must both be called directly by Level N-1 functions.

If they are called by the same Level N-1 function (say, A), the program executes them sequentially. A calls B, B returns and is popped off the stack, and then A calls C. Even if they are arguments to the same function foo(B(), C()), the language evaluates them sequentially. They are never on the stack at the same time.

If B were to call C, C's caller would be Level N, pushing C to Trunk Level N+1.

Because Trunk levels strictly increase as you go down the call chain (Level 1 -> Level 2 -> Level 3...), any Trunk functions currently on the stack must have strictly different levels. Therefore, there will never be more than one Trunk Level T on the stack at once.

2. Leaf Functions

Not Reentrant & Mutually Exclusive at the Same Level: A Leaf Level 1 function makes no function calls. A Leaf Level 2 function only makes calls to Level 1 functions. A Leaf Level L function only calls functions with a maximum Leaf Level of L-1.

If you are currently executing inside a Level 1 function, its caller must be at least Level 2, its caller's caller must be at least Level 3, and so on.

Because Leaf levels strictly decrease as you go deeper into the call stack (Level L -> Level L-1 -> Level L-2...), it is impossible for a Level L function to call another Level L function (otherwise the caller would instantly become Level L+1).

Since the stack trace of Leaf functions must be strictly monotonically decreasing, no two functions on the stack can ever share the same Leaf Level.

Because a Leaf function can never appear on the stack twice at the same time, it is never reentrant.

Caveats Accounted For

Dynamic / Indirect Calls: Closures, function pointers, and indirect calls instantly disqualify a function from being a Trunk function (it gets Level 0) and disqualify a caller from being a Leaf function (it also gets Level 0).

Mutual Recursion: Mutually recursive functions will fail the "called exactly once" rule (disqualifying them as Trunk) and their maxLevel will never resolve (disqualifying them as Leaf).

Your logic maps perfectly to how the call stack models these trees! This has massive implications for the backend—you can safely allocate global, statically-addressed memory for the local variables and frames of all Trunk functions and all Leaf functions without worrying about recursion, stack overflow, or memory overlapping!

