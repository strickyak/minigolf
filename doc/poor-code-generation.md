Look at the IR for peekb in  `_tmp/ir` line 34 `func byte @prelude.peekb(word p0_addr)` .   Compare it to the M6809 assembly language in  `_tmp/moto.asm` line 9 `EMIT FUNC "prelude.peekb"`.   In an optimal backend, that code would be two instructions:   `    ldb ,x    \n    rts  `  but our backend is making 17 instructions.     What steps will get us closer to an optimal backend?  
11:51 PM
The reason the M6809 backend generates 17 instructions instead of the optimal 2 for 

prelude.peekb
 is due to a combination of IR-generation patterns and backend code-generation defaults.

In GOLF, the compiler always treats parameters and local variable accesses as memory-backed LValues. As a result, the IR builder generates an 

AddressOfLocal
 instruction followed by a 

LoadPtr
 for every variable read. This marks the variables as "escaping" in 

opt/stack_alloc.go
, which forces the backend to allocate stack slots and spill values to memory continuously.

Steps to Achieve an Optimal Backend
1. Direct SSA RValue Reads (Frontend Optimization)
Currently, in 

ir/builder.go
 (around b.eval for ast.Identifier), the compiler always takes the address of local variables.

Action: Change the IR builder to only use AddressOfLocal when the variable’s address is explicitly taken in the code (e.g., via the & operator). Otherwise, reads should directly use the SSA value returned by b.readVariable(...) rather than wrapping it in an AddressOfLocal + LoadPtr.
2. Introduce a Memory-to-Register (mem2reg) Optimization Pass
If the frontend continues to generate load/store operations on locals, we need an optimization pass to clean them up.

Action: Implement a standard register promotion pass (like mem2reg or SROA—Scalar Replacement of Aggregates) in the opt package. This pass detects non-escaping variables, eliminates their AddressOfLocal allocations, and transforms pointer loads/stores into direct SSA value definitions.
3. No-Op Cast Elision
The cast v4:*byte = word_to_ptr v3 is a machine no-op on the M6809 since both words and pointers are 16-bit.

Action: Make the backend recognize that casts like word_to_ptr are zero-cost and avoid allocating stack slots or executing move instructions for them.
4. Hardware Register Allocation
In 

m6809/backend.go
, the backend assigns every instruction ID a memory slot on the stack and emits memory-write/read instructions.

Action: Implement a local or global register allocator (e.g., linear scan or graph coloring using the existing InterferenceGraph) to keep values in M6809 registers (X, D, Y, U) across their lifetimes, only spilling them to stack slots when registers are exhausted.
5. Omit Stack Frames for Simple Leaf Functions
Since 

prelude.peekb
 is a leaf function, once variables are kept in registers, it needs 0 stack slots.

Action: Detect when a function does not call other functions (is a leaf) and uses 0 local stack bytes. If so, omit the stack adjustment prelude (leas -9,s) and postlude (leas 9,s) entirely.
