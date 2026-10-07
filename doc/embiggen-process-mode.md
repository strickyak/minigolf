# EMBIGGEN Process Mode: Far Functions and Far Data Architecture

**Specification & Implementation Guide for MiniGolf on 64KB Small-Memory Architectures**  
*Covers Motorola 6809 / Hitachi 6309 (`gep9`), Zilog Z80 (`gepz`), and RCA CDP1802 (`gepc`)*

---

## 1. Executive Summary & Architectural Motivation

Classic 8-bit microprocessors—including the Motorola 6809, Zilog Z80, and RCA CDP1802—are fundamentally constrained by a 16-bit address bus (64 KB address space). In traditional compiled execution:
- User code and runtime libraries compete directly with application data and the process stack.
- Once a program exceeds 20–30 KB of compiled machine code, the remaining space for dynamic heap memory, btrees, string buffers, and symbol tables becomes critically cramped.
- Traditional "overlay" systems or flat banking models either suffer from severe call-overhead or force awkward manual bank management onto the programmer.

The **EMBIGGEN Process Mode** solves this limitation by pairing the Hatvan OS 8KB Paged MMU hardware specification with a segmented **Far Function** and **Far Data** compilation model in MiniGolf. 

### Key Capabilities & Invariants
1. **2 Megabyte Process Address Space**: Each process can access up to 256 physical 8KB blocks (2,048 KB total).
2. **Transparent 16-Bit Pointers**: Far Data is referenced using 16-bit reference handles (`FarRef`), maintaining a uniform 16-bit word size across registers, stack frames, and struct fields without requiring expensive 24-bit or 32-bit pointer emulation.
3. **Zero-Thrashing 3-Window Data Access**: Three independently mapped 8KB data windows (Slots 2, 3, and 4) operate in a round-robin / LRU configuration, allowing multi-operand operations (e.g. `s1 == s2`, `buf.Append(s)`, `memcpy(dst, src, n)`) to proceed at full native CPU bus speed without bank-switch thrashing.
4. **Transparent Far Subroutine Calls**: Every user-defined function (even `main()`) can reside in one of 120 Far Code blocks (up to 960 KB of code), mapped dynamically into Slot 5 via single-instruction-overhead trampolines in fixed memory.
5. **Fixed Stack & Direct Page**: The process stack, direct page, and fixed static variables reside in permanently mapped low memory (Slots 0 & 1), ensuring that function parameter passing, local variables, and return addresses are never swapped or invalidated.

---

## 2. 64KB Virtual Memory Map (The 8-Slot Layout)

The 64KB virtual address space of the CPU is divided into eight 8KB windows (Slots 0 through 7). The physical block mapped into each slot is controlled by the user-accessible hardware MMAP vector at `$FF40..$FF47` (see [Hatvan Hardware I/O Specification](file:///home/strick/github.com/strickyak/hatvan-os/doc/spec-hatvan-io.md#L161-L178)):

```
CPU Address Range   Slot      Default Mapping       Primary Function & Lifecycle
==================  ======    ===================   ================================================
$0000 .. $1FFF      Slot 0    Block 0 (Fixed)       Direct Page, Fixed Globals, BSS (Growing UP)
$2000 .. $3FFF      Slot 1    Block 1 (Fixed)       Process Stack (Growing DOWN from $3FFF)
$4000 .. $5FFF      Slot 2    Mapped Far Data 0     Active Far Data Window 0 (MMAP $FF42)
$6000 .. $7FFF      Slot 3    Mapped Far Data 1     Active Far Data Window 1 (MMAP $FF43)
$8000 .. $9FFF      Slot 4    Mapped Far Data 2     Active Far Data Window 2 (MMAP $FF44)
$A000 .. $BFFF      Slot 5    Mapped Far Function   Active Far Code Block (MMAP $FF45)
$C000 .. $DFFF      Slot 6    Block 6 (Fixed)       Fixed Core Runtime, Trampolines, Prelude
$E000 .. $FFFF      Slot 7    Block 7 (Fixed)       Fixed System Libraries, Traps, & Hardware I/O ($FFxx)
```

```mermaid
graph TD
    subgraph VirtualMemorySpace["64KB Virtual Address Space"]
        S0["Slot 0: $0000..$1FFF (8KB)<br/><b>Block 0</b>: Fixed Data / DP / Globals (Fixed)"]
        S1["Slot 1: $2000..$3FFF (8KB)<br/><b>Block 1</b>: Process Call Stack (Fixed)"]
        S2["Slot 2: $4000..$5FFF (8KB)<br/><b>Far Data Window 0</b> ($FF42)"]
        S3["Slot 3: $6000..$7FFF (8KB)<br/><b>Far Data Window 1</b> ($FF43)"]
        S4["Slot 4: $8000..$9FFF (8KB)<br/><b>Far Data Window 2</b> ($FF44)"]
        S5["Slot 5: $A000..$BFFF (8KB)<br/><b>Far Function Window</b> ($FF45)"]
        S6["Slot 6: $C000..$DFFF (8KB)<br/><b>Block 6</b>: Core Runtime & Trampolines (Fixed)"]
        S7["Slot 7: $E000..$FFFF (8KB)<br/><b>Block 7</b>: System Libs & Hardware I/O ($FFxx) (Fixed)"]
    end

    subgraph PhysicalBlocks["256 Physical 8KB Blocks (2 MB Total)"]
        P0_1["Blocks 0..1: Fixed Data & Stack"]
        P6_7["Blocks 6..7: Fixed Code & Hardware I/O"]
        PCode["Blocks 8..127: Far Function Code Pool (960 KB)"]
        PData["Blocks 128..255: Far Data Heap Arenas (1,024 KB)"]
    end

    S0 --> P0_1
    S1 --> P0_1
    S6 --> P6_7
    S7 --> P6_7
    S5 -.->|Dynamic Mapping via $FF45| PCode
    S2 -.->|Round-Robin Window 0 via $FF42| PData
    S3 -.->|Round-Robin Window 1 via $FF43| PData
    S4 -.->|Round-Robin Window 2 via $FF44| PData
```

### Rationale for Fixed vs. Floating Slots
- **Fixed Slots 0 & 1 ($0000..$3FFF)**:
  - Local stack variables and procedure call frames grow down from `$3FFF`.
  - Global scalar variables, compiler temporaries, and Direct Page variables reside in `$0000..$1FFF`.
  - Because Slots 0 and 1 are never bank-switched, CPU registers, stack pointers (`S`, `SP`, `R2`), and local addresses remain valid across every far call and every far data access.
- **Fixed Slots 6 & 7 ($C000..$FFFF)**:
  - Contains the Far Function Trampoline dispatcher, Far Data round-robin manager, `far_malloc` arena allocator, and hardware I/O interrupt vectors.
  - The hardware I/O page at `$FF00..$FFFF` is locked into high memory by Hatvan OS hardware logic.

---

## 3. Physical Block Pool Partitioning

The 256 physical 8KB blocks available to an EMBIGGEN task are partitioned into clean, non-overlapping functional zones:

| Block Range | Count | Total Capacity | Purpose | Allocation Strategy |
| :--- | :---: | :---: | :--- | :--- |
| **`0 .. 1`** | 2 | 16 KB | Fixed Process Data & Stack | Statically assigned to Slots 0 & 1 at process startup |
| **`2 .. 5`** | 4 | 32 KB | Scratch / Initial Boot Mapping | Initialized to identity mapping `{2, 3, 4, 5}` |
| **`6 .. 7`** | 2 | 16 KB | Fixed Code, Runtime, & I/O Page | Statically assigned to Slots 6 & 7 |
| **`8 .. 127`** | 120 | 960 KB | **Far Function Code Pool** | Statically linked by compiler/linker into 8KB code blocks |
| **`128 .. 255`** | 128 | 1,024 KB (1 MB) | **Far Data Heap Blocks** | Dynamically allocated on-demand by `far_malloc` |

---

## 4. Far Data Reference Architecture (`FarRef`)

Instead of representing heap objects and strings with 24-bit or 32-bit pointers (which would require costly multi-word register pairing and double stack spills across 8-bit CPUs), EMBIGGEN encodes all far heap references into a **16-bit word**:

```
Bit 15                                                  Bit 0
+----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+
|  1 |          Block ID (6 bits)       |            Chunk Index (9 bits)       |
+----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+----+
|<------------- 7 Bits ---------------->|<-------------- 9 Bits --------------->|
|    Physical Block ID: 128 .. 255      |       16-Byte Chunk: 0 .. 511        |
```

### Bitfield Decomposition
- **Block ID (Bits 15..9, 7 bits)**:
  - Selects the physical 8KB data block in range `128 .. 255`.
  - Notice that Bit 15 is **always 1** because $128 = 10000000_2$.
- **Chunk Index (Bits 8..0, 9 bits)**:
  - Each 8KB block contains exactly $8,192 / 16 = 512$ chunks of 16 bytes each.
  - 9 bits cleanly address all 512 chunks (`0 .. 511`).
  - Byte offset within the 8KB block $= \text{ChunkIndex} \times 16 = \text{ChunkIndex} \ll 4$.

### Mathematical Properties & Nil Representation
- **Unambiguous Nil**: `0x0000` (and any reference with Bit 15 = 0) represents `nil`. Because all valid Far Data blocks have Bit 15 set, checking for `nil` is a single test on the high bit:
  - 6809: `BMI is_valid` or `BEQ is_nil`
  - Z80: `BIT 7, H`
  - 1802: `GLO R_high` / `SHR`
- **Intrablock Invariant**: Every allocated object (struct, string payload, slice buffer) resides entirely within a single Far Data block. No individual object spans across an 8KB block boundary.
- **Maximum Object Size**: The maximum single `malloc` payload is $8,192 - \text{ArenaHeaderSize} \approx 8,128$ bytes.

---

## 5. Round-Robin Far Data Window Management (Slots 2, 3, 4)

Slots 2, 3, and 4 (`$4000..$9FFF`, 24KB total) serve as three active data windows into the 1 MB Far Data heap.

### Runtime State
The runtime maintains a tiny 4-byte descriptor in fixed direct page RAM:
```c
struct FarWindowManager {
    uint8_t active_blocks[3]; // Physical block mapped in Slot 2, 3, and 4
    uint8_t next_evict_slot;  // Round-robin index: 0 (Slot 2), 1 (Slot 3), 2 (Slot 4)
};
```

### Resolution Algorithm (`map_far_ref`)
When user code accesses a `FarRef`:
1. Extract `block_id = (ref >> 9)` and `chunk_idx = ref & 0x1FF`.
2. **Hit Detection**:
   - If `active_blocks[0] == block_id` $\rightarrow$ Target Slot = 2 (Base `$4000`).
   - If `active_blocks[1] == block_id` $\rightarrow$ Target Slot = 3 (Base `$6000`).
   - If `active_blocks[2] == block_id` $\rightarrow$ Target Slot = 4 (Base `$8000`).
   - If hit: **Zero MMU register writes required.** Calculate virtual address immediately.
3. **Miss / Eviction**:
   - Slot to replace: `slot = next_evict_slot`.
   - Write physical block to MMU register: `*(uint8_t*)(0xFF42 + slot) = block_id`.
   - Update cache: `active_blocks[slot] = block_id`.
   - Virtual Base $= \$4000 + (\text{slot} \times \$2000)$.
   - Advance round-robin pointer: `next_evict_slot = (next_evict_slot + 1) % 3`.
4. **Virtual Address Computation**:
   $$\text{VAddr} = \text{VirtualBase} + (\text{chunk\_idx} \ll 4) + \text{field\_offset}$$

### Multi-Operand Zero-Thrashing Proof
Consider a common string comparison or append loop:
```golf
func streq(s1 string, s2 string) bool {
    if s1.Len != s2.Len { return false }
    for i := range s1.Len {
        if s1[i] != s2[i] { return false }
    }
    return true
}
```
- Suppose `s1` payload is in Block 130 and `s2` payload is in Block 142.
- At loop start:
  - First access to `s1[0]` maps Block 130 into **Slot 2** (`$4000..$5FFF`).
  - First access to `s2[0]` maps Block 142 into **Slot 3** (`$6000..$7FFF`).
- Throughout the entire execution of the loop:
  - `s1[i]` always hits Slot 2.
  - `s2[i]` always hits Slot 3.
  - **Zero MMU writes occur during the loop.** The comparison loop runs at 100% native CPU memory speed.
- In a 3-way operation like `buf.Append(s)` where `buf` reallocates into a new block:
  - `buf_old` occupies Slot 2.
  - `s` occupies Slot 3.
  - `buf_new` occupies Slot 4.
  - All three blocks remain simultaneously visible in CPU memory.

---

## 6. Far Slices, Strings, and Indexing Semantics

### Standard 6-Byte Far Slice Layout
In EMBIGGEN mode, `string` and `slice[T]` preserve their standard 6-byte structure:

```
+--------------------+--------------------+--------------------+
|  Base: FarRef      |  Cap: uint16       |  Len: uint16       |
|  (2 bytes)         |  (2 bytes)         |  (2 bytes)         |
+--------------------+--------------------+--------------------+
```
- Because `Base` is still 16 bits, all slice passing, struct embedding, and return conventions remain binary-compatible with near compilation.
- `Len` and `Cap` measure elements/bytes up to 8,192.

### Prohibition of Raw Pointer Arithmetic
In standard MiniGolf, advancing a buffer pointer is performed via pointer arithmetic: `ptr = ptr + 1`.  
**In EMBIGGEN mode, raw pointer arithmetic on `FarRef` is strictly invalid.** Adding 1 to a `FarRef` would increment the chunk index by 1 (advancing 16 bytes!), and incrementing past 511 would overflow into the block ID.

Instead, all buffer iterations and index accesses MUST use **special indexed expressions** `s[i]`:
- `s[i]` translates into:
  $$\text{TargetOffset} = (s.\text{Base}.\text{ChunkIndex} \times 16) + (i \times \text{sizeof}(T))$$
- Bounds check: $i < s.\text{Len}$.
- The compiler lowers `s[i]` to `OpFarLoad(slice, index)` and `s[i] = val` to `OpFarStore(slice, index, val)`.

### Slicing / Reslicing (`s[low:high]`)
When creating a subslice `sub := s[start:limit]`:
- `sub.Len = limit - start`
- `sub.Cap = s.Cap - start`
- If `start` is a multiple of 16 bytes:
  - `sub.Base.ChunkIndex = s.Base.ChunkIndex + (start / 16)`
  - Subslice remains chunk-aligned.
- If `start` is not a multiple of 16 bytes:
  - EMBIGGEN supports byte-aligned `FarSlice` by using a 3-part reference or storing byte sub-offsets in slice metadata (see Section 11).

---

## 7. Malloc Arena Architecture in Far Data Blocks

Every Far Data block in active use by the heap manager possesses an **Arena Header** located in **Chunk 0** (the first 16 bytes of the block, `$0000..$000F`):

```c
struct FarArenaHeader {
    uint16_t magic;         // 0xEB69 ('EB' = EmBiggen)
    uint8_t  block_id;      // Physical block number (128..255)
    uint8_t  flags;         // Arena state flags (0x01 = Active, 0x02 = Full)
    uint16_t free_chunks;   // Total free 16-byte chunks remaining (0..508)
    uint16_t alloc_count;   // Number of active allocations in this block
    uint16_t first_free;    // Chunk index of first free chunk (or free list head)
    uint16_t rover;         // Next-fit allocation hint
    uint16_t reserved;      // Reserved for GC / arena linkage
};
```

### Intrablock Chunk Layout
- **Chunk 0** (`$0000..$000F`, 16 bytes): Arena Header.
- **Chunks 1 .. 3** (`$0010..$003F`, 48 bytes): Chunk Allocation Bitmap (512 bits = 64 bytes total; bits 0..3 marked permanently allocated).
- **Chunks 4 .. 511** (`$0040..$1FFF`, 508 chunks = 8,128 bytes): Allocatable Payload Area.

### Allocation Header
Every allocation made by `far_malloc(nbytes)` reserves an integer number of contiguous 16-byte chunks:
$$\text{chunks\_needed} = \left\lceil \frac{\text{nbytes} + 2}{16} \right\rceil$$
The first 2 bytes of the allocated region store the allocation chunk length:
- `alloc_header.chunks`: `uint8_t` (number of chunks spanned by this allocation).
- `alloc_header.reserved`: `uint8_t`.
- Payload immediately follows at offset `+2`.

### Global Block Pool Tracker (Fixed Data in Slot 0)
The global heap manager maintains a 128-bit bitmap (16 bytes in Slot 0 BSS) representing physical blocks `128 .. 255`:
- Bit $= 0$: Physical block is free (unallocated to this process).
- Bit $= 1$: Physical block is assigned as an active Far Heap Arena.

### Allocation Lifecycle
1. **`far_malloc(nbytes)`**:
   - Compute `chunks_needed`.
   - Traverse active arenas (via round-robin data slot). If an active arena has a contiguous span of `chunks_needed`, mark bitmap and return `(block_id << 9) | chunk_idx`.
   - If no active arena can satisfy the request, allocate a fresh physical block from the 128-bit global pool, map it into an available data window, initialize `FarArenaHeader`, and allocate the payload.
2. **`far_free(FarRef ref)`**:
   - Extract `block_id = ref >> 9` and `chunk_idx = ref & 0x1FF`.
   - Map `block_id` into a data window.
   - Read allocation length from chunk header.
   - Clear corresponding bits in the chunk bitmap. Decrement `alloc_count`.
   - If `alloc_count == 0`, the entire 8KB block is freed and returned to the 128-bit global pool.

---

## 8. Far Functions & Calling Convention (Slot 5)

All user-defined functions reside in Far Code blocks `8 .. 127` and execute in virtual **Slot 5 (`$A000..$BFFF`)**.

### Fixed Trampoline Dispatcher (Slot 6)
Every far function `pkg.Foo` has a lightweight 5-byte entry stub in fixed memory (Slot 6):

```asm
; In Slot 6 (Permanently Mapped):
pkg_Foo:
    ldb   #15               ; Target Physical Block ID (e.g. Block 15)
    ldx   #$A140            ; Target Virtual Entry Point in Slot 5
    jmp   __far_call_dispatcher
```

### The Far Call Dispatcher (`__far_call_dispatcher`)
```asm
; Input: B = Target Block ID, X = Target Virtual Address in Slot 5
__far_call_dispatcher:
    lda   $FF45             ; Read current block mapped in Slot 5
    pshs  a                 ; Save previous block ID on process stack (Slot 1)
    stb   $FF45             ; Map target block into Slot 5
    jsr   ,x                ; Call the target function in Slot 5
    puls  b                 ; On return: pop previous block ID
    stb   $FF45             ; Restore previous block to Slot 5
    rts                     ; Return to caller (in Slot 5 or Slot 6)
```

### Call Flow Trace

```mermaid
sequenceDiagram
    participant Caller as Caller Function (in Block 10, Slot 5)
    participant Stub as Trampoline Stub (in Slot 6)
    participant Disp as Far Call Dispatcher (in Slot 6)
    participant Callee as Callee Function (in Block 25, Slot 5)

    Caller->>Stub: JSR pkg_Bar (Call stub in Slot 6)
    Note over Stub: Load B = 25, X = $A200
    Stub->>Disp: JMP __far_call_dispatcher
    Note over Disp: PSHS CurrentBlock (10)<br/>STB $FF45 (Map Block 25 into Slot 5)
    Disp->>Callee: JSR ,X (Enter function in Slot 5)
    Note over Callee: Executes in Block 25<br/>Stack in Slot 1 remains valid
    Callee->>Disp: RTS (Return to dispatcher)
    Note over Disp: PULS B (Pop 10)<br/>STB $FF45 (Restore Block 10 to Slot 5)
    Disp->>Caller: RTS (Return to caller in Slot 5)
```

### Key Architectural Invariants
1. **Arbitrary Nesting & Recursion**: Because the caller's previous code block ID is pushed onto the CPU hardware stack `S` (located in permanently mapped Slot 1), far calls can nest to arbitrary depths and call recursively across blocks without limitation.
2. **Uniform Stack Frames**: Local variables, spilled SSA temporaries, and function parameters are indexed relative to stack frame pointers (`U` on 6809, `IX` on Z80, `R2` on 1802). Because the stack is in Slot 1, frame access has **zero bank-switch overhead**.
3. **No Code Thrashing**: Function calls change the mapping of Slot 5 only on entry and exit. During the execution of a function, code fetches never touch MMU registers.

---

## 9. Compiler Pipeline & Code Generation Integration

To support EMBIGGEN, the MiniGolf compiler adds target architecture flags and pipeline adaptations:
- Compiler Invocation: `minigolf -m=6809 -membiggen ...` (or `-m=z80 -membiggen`, `-m=1802 -membiggen`).

### Frontend & Semantic Pass Changes
1. **Far Function Identification**:
   - All functions in `package main` and user libraries are tagged with `IsFar = true`.
   - Prelude core runtime functions (`malloc_core`, `far_call_dispatcher`, `peek/poke`, math division helpers) are tagged with `IsFar = false` and pinned to Slot 6/7.
2. **Type System Adaptation**:
   - Pointers (`*T`) to heap-allocated objects are tagged as `FarPointerType`.
   - `string` and `slice[T]` types are treated as `FarSliceType` with `Base` typed as `FarRef`.
   - Pointers to stack variables (e.g. `&localVar`) remain `NearPointerType` (16-bit virtual address in Slot 1).
3. **Semantic Checking**:
   - Rejects pointer arithmetic on `FarPointerType` and `FarSliceType`.
   - Enforces index syntax `s[i]` for buffer accesses.

### Intermediate Representation (IR) Extensions
New SSA instructions in `ir/instruction.go`:
- `OpFarCall`: Calls a far function via its fixed trampoline stub.
- `OpFarLoad`: Resolves a `FarRef + offset` via the 3-window manager and loads a scalar value.
- `OpFarStore`: Resolves a `FarRef + offset` via the 3-window manager and stores a scalar value.
- `OpFarSliceGet`: Emits bounds check and loads element from a `FarSlice`.
- `OpFarSlicePut`: Emits bounds check and stores element into a `FarSlice`.

---

## 10. Target Architecture Implementations

### 10.1 Motorola 6809 / 6309 (`gep9`)

#### Hardware Register Access
The MMAP registers at `$FF40..$FF47` are accessed using standard 6809 memory instructions:
```asm
    sta   $FF45             ; Map code block into Slot 5 (4 cycles)
    sta   $FF42,x           ; Map data block into Slot 2+x (5 cycles)
```

#### Fast Inlined Far Load (Slot-Cached)
```asm
; Input: D = FarRef, Y = byte offset within object
; Output: A = loaded byte
    tfr   a,b
    lsrb                    ; B = Block ID (bits 15..9)
    cmpb  <active_slot_0   ; Check Slot 2
    beq   .L_hit_slot0
    cmpb  <active_slot_1   ; Check Slot 3
    beq   .L_hit_slot1
    cmpb  <active_slot_2   ; Check Slot 4
    beq   .L_hit_slot2
    jsr   __far_map_miss    ; Handle eviction and update active_slot
    ; Falls through with X = Virtual Base ($4000, $6000, or $8000)
.L_hit_slot0:
    ldx   #$4000
.L_access:
    anda  #$01              ; A = high bit of chunk index
    lsla
    lsla
    lsla
    lsla                    ; Shift chunk index << 4
    leax  a,x               ; Add chunk offset to virtual base
    lda   b,x               ; Load byte with offset
```

---

### 10.2 Zilog Z80 (`gepz`)

#### Hardware MMU Access
In `gepz`, `$FF40..$FF47` is mapped into CPU memory space:
```asm
    LD   (0xFF45), A        ; Map code block into Slot 5
```

#### Far Call Dispatcher
```asm
__far_call_dispatcher:
    LD   A, (0xFF45)        ; Read current Slot 5 mapping
    PUSH AF                 ; Push caller's block onto stack (Slot 1)
    LD   A, C               ; C = Target Block ID
    LD   (0xFF45), A        ; Map new block into Slot 5
    CALL __call_hl          ; Execute function at HL
    POP  AF                 ; Pop caller's block
    LD   (0xFF45), A        ; Restore caller's block
    RET
__call_hl:
    JP   (HL)
```

---

### 10.3 RCA CDP1802 (`gepc`)

#### Hardware MMU Access
In Hatvan 1802, the MMAP vector is accessed via memory-mapped I/O at `$FF40..$FF47` or `OUT 4` bank-latch instructions.
- Register `R2` is pinned as the hardware Process Stack in Slot 1.
- Register `R3` is the Program Counter, executing in Slot 5 (Far Functions) or Slot 6 (Trampolines).
- Register `R4` / `R5` implement the Standard Call and Return Technique (SCRT) modified to save and restore the previous block ID to `$FF45`.

---

## 11. Advanced Considerations & Optimization Strategies

### 1. Far String Literals
String constants can be compiled in two ways:
- **Embedded in Far Code**: Constant strings referenced only by a specific function can reside inside the function's own Far Code block in Slot 5.
- **Far Constant Pool (Blocks 128..)**: Global and shared strings are placed into dedicated read-only Far Data blocks and assigned standard `FarRef` handles.

### 2. Multi-Block Allocation Support
For allocations larger than 8KB (e.g. full-screen graphics buffers or huge arrays), the allocator can reserve contiguous physical blocks (e.g. Blocks 140..143) and map them simultaneously across Slots 2, 3, and 4 (giving a contiguous 24KB flat window).

### 3. Loop Invariant Window Hoisting (LICM Integration)
If an inner loop repeatedly accesses fields of the same Far struct `p.field1`, `p.field2`, the MiniGolf SSA LICM pass hoists the window mapping outside the loop:
- Map `p.BlockID` into Slot 2 once before loop entry.
- Inside the loop, access fields directly via fixed offset `[$4000 + chunk_offset + field_offset]` with **zero runtime mapping overhead**.

---

## 12. Summary Comparison: Near vs. EMBIGGEN Mode

| Architectural Feature | Near Mode (Standard) | EMBIGGEN Process Mode |
| :--- | :--- | :--- |
| **Max Process Code Size** | $\approx 32\text{ KB}$ | **960 KB** (120 Far Code Blocks) |
| **Max Process Data Heap** | $\approx 16\text{ KB}$ | **1,024 KB** (128 Far Data Blocks) |
| **Pointer Size** | 16-bit Flat Virtual Address | **16-bit Far Reference (`FarRef`)** |
| **Slice / String Size** | 6 Bytes (`Base`, `Cap`, `Len`) | **6 Bytes (`FarRef`, `Cap`, `Len`)** |
| **Stack Allocation** | Shared in Low/High RAM | Dedicated 8KB in **Slot 1 (`$2000..$3FFF`)** |
| **Hardware MMU Usage** | Disabled / Identity `{0..7}` | **Active 8KB Paged Vector (`$FF40..$FF47`)** |
| **Far Call Overhead** | None | 5-byte stub + $\approx 25$ CPU cycles |
| **Inner Loop Access Speed** | Native bus speed | **Native bus speed** (cached across Slots 2, 3, 4) |
