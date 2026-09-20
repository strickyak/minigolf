▾ Thought for 4s
  Initial investigation focuses on IR-level optimizations, specifically Mem2Reg/SROA to promote local variables
  currently using memory addresses to registers. The current approach, while aided by a StoreLoadForwarding pass, still
  results in variables escaping to stack slots, representing a potential optimization target.
  
  
  Eliminating stack slots and memory operations via SSA form, coupled with preheader insertion for loop-invariant code
  motion (LICM), could drastically improve performance. Investigating loop unrolling based on existing configurations and
  analyzing function parameters for redundancy present additional optimization opportunities. Moreover, a more liberal
  stack allocation strategy, sharing slots by size instead of strict type matching, may reduce overall memory usage.
  
  
  Analyzing M6809 backend, long branches (`lbra`, `lb`) are consistently used, even when short branches (`bra`, `b`)
  would suffice, leading to increased code size and cycles. Identifying and replacing these redundant transfers (`tfr`)
  and indirect addressing sequences with more direct instructions, such as directly indexing, presents another
  optimization opportunity.
  
  Chordal graph coloring for hardware register allocation is present, although not integrated to assign hardware
  registers to improve code generation further; the existing backend primarily uses stack spilling. Exploring overlapping
  global register allocation for trunk/leaf functions along with eliminating zero-byte stack frames are promising
  strategies.
  Based on the codebase analysis, recent benchmarks, and existing architectural documents, here are the primary       
  optimization opportunities categorized by compiler stage and impact:                                                
  ──────                                                                                                              
  ### 1. M6809 Backend Codegen & Peephole (High Immediate ROI on perf1)                                               
                                                                                                                      
  Inspection of the assembly output in disasm reveals several low-hanging codegen improvements that directly close the
  gap with CMOC and GCC:                                                                                              
                                                                                                                      
  1. Short Branch Relaxation (bra / bxx vs lbra / lbxx):                                                              
      • Currently, backend.go emits long branches (lbra, lbeq, lbgt, etc.) for almost all control flow.               
      • Long conditional branches cost 4 bytes and 5–6 cycles, whereas short branches cost 2 bytes and 3 cycles.      
      • In functions like fib, sieve, and put_num, the branch targets are almost always within [-128, +127] bytes. A  
      branch-shortening pass or distance estimation will save dozens of bytes and cycles per function.                
  2. [DONE] Array Access Chaining & Direct Addressing Mode Utilization:                                                                             
      • Eliminated redundant `tfr x,d`, `std slot`, and `ldx slot` sequence for single-use `AddressOfElement`, `AddressOfField`, and `AddressOfLocal` instructions.
      • Added register X tracking (`valInX`, `clobberX()`, `setX()`), allowing effective addresses to be computed and retained directly in register X.
      • Single-use address calculations are consumed directly via `,x` by `LoadPtr`, `StorePtr`, `ExtractFieldPtr`, and `InsertFieldPtr` without allocating stack slots or spilling to memory.
      • Shaved 806 cycles on `07_sieve`, 621 cycles on `08_bubble_sort`, and 300 cycles on `05_array_sum`.                                                           
  3. [DONE] Return Sequence & Dead Store Cleanup:
      • Removed redundant `tfr d,x` from 16-bit return sequence in `m6809/backend.go`.
      • Implemented structural single-use consumer analysis (`instructionNeedsSlot`) to elide dead stores before calls, returns, and operations directly in backend code generation without relying on textual peephole matching.
      • Shaved 1,786 cycles (~10%) and 18 bytes on `04_fibonacci`; elided zero-byte frames across call sites.                    
  4. Zero-Byte Stack Frame Elision (Phase 8):                                                                         
      • For leaf functions requiring 0 local stack bytes, elide leas -0,s / leas 0,s and frame pointer setup.         
                                                                                                                      
  ──────                                                                                                              
  ### 2. IR-Level Optimizations (implementation_plan-future-ir-opt-strategies.md)                                     
                                                                                                                      
  1. Preheader Insertion for LICM (licm.go:24):                                                                       
      • licm.go is active, but currently skips any loop where loop.Preheader == nil (which occurs whenever loop       
      headers have multiple or branching outside predecessors).                                                       
      • Synthesizing dedicated loop preheader blocks will allow hoisting invariant address calculations (like         
      AddressOfGlobal base pointers) out of loops like 07_sieve and 08_bubble_sort.                                   
  2. Mem2Reg / Local SSA Promotion:                                                                                   
      • Local variables and parameters are currently introduced as memory lvalues (AddressOfLocal + LoadPtr/StorePtr).
      • While store_load.go handles intra-block forwarding, non-escaping variables still reserve stack slots. Full SSA
      promotion will turn them into pure virtual registers with Phi nodes.                                            
  3. Wiring Chordal Register Allocation (chordal.go):                                                                 
      • Simplicial Elimination Ordering (PEO) and greedy graph coloring (ColorChordalGraphWithPreferences) are already
      implemented and tested in opt/, but not yet driving hardware register assignments (D, X, Y, U) in the backend.  
  4. Liberal Stack Slot Sharing by Size (stack_alloc.go:120):                                                         
      • Currently groups by exact type equality (Type.Equals). Updating it to group by byte footprint (Type.Size())   
      allows word, int, and pointer types (all 2 bytes) to share slots across disjoint lifetimes.                     
  5. Loop Unrolling (loop.go):                                                                                        
      • Basic block dominance and natural loop induction variable analysis are in place. Unrolling small constant-    
      bound loops will eliminate loop counter overhead.                                                               
                                                                                                                      
  ──────                                                                                                              
  ### 3. Architecture & Target Roadmap                                                                                
                                                                                                                      
  • Motorola 68000 (M68K) Backend:                                                                                    
      • We added listing file support to main.go. Continuing the code generator for M68K is outlined in the           
      m68k_backend_plan.md.                                                                                           
                                                                                                                      
  ──────                                                                                                              
  ### Recommended Next Step                                                                                           
                                                                                                                      
  If the goal is to improve perf1 benchmark scores on the M6809, the two biggest and cleanest wins are:               
                                                                                                                      
  1. [DONE] Short Branch Relaxation (implemented whole-program in `cmd/asm6809`, saving 4,743 cycles and 266 bytes across `perf1`).
  2. [DONE] **Dedicated Preheader insertion in licm.go** (implemented in `opt/licm.go`, enabling invariant hoisting across critical edges and multi-entry loops).
                                                                                                                      
  Which area would you like to focus on next?
