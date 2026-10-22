# The `twostep09` Architecture & Instruction Set Design

> *"One byte to lead, one byte to follow — every instruction does the Two-Step."*

## 1. Philosophy & Overview

The **twostep09** is an experimental, regularised re-imagining of the Motorola 6809 architecture built around a single ironclad law:

> **Every machine instruction is exactly 2 bytes (16 bits) long: `[Opcode Byte] [Postbyte]`.**

In the classic 6809, instructions varied from 1 to 5 bytes, and `$10`/`$11` prefix bytes created multi-table decoding. **twostep09** eliminates all variable-length instruction decoding while keeping the primary 6809 opcode map and register model intact via a simple, clever mechanism: **the `HIBYTE #N` instruction (`$10 N`)**.

### The Core Rules of `twostep09`

1. **Every Machine Instruction is 2 Bytes (Even-Aligned)**:
   - **Byte 0 (even address)**: Opcode and addressing mode (`$00..$FF`).
   - **Byte 1 (odd address)**: Postbyte (8-bit immediate, 8-bit direct-page offset, 8-bit branch offset, 8-bit indexed postbyte, 8-bit register-pair/mask, 8-bit low byte of a 16-bit numeric operand, or ignored for inherent ops).
   - **Low bit of `PC` is hardwired to `0`**: `PC` always increments by `+2`, and any operation that loads or modifies `PC` (`BRA`/`Bcc`/`BSR`, `LBRA`/`LBSR`, `JMP`, `JSR`, `RTS`, `PULS PC`/`PULU PC`, `TFR r,PC`, `EXG r,PC`) clamps bit 0 to `0` (`PC <- target & $FFFE`). It is **impossible** to execute the middle (postbyte) of an instruction.
2. **No `$10` or `$11` Multi-Byte Opcode Tables**:
   - All opcodes live in a single flat 256-entry table (`$00..$FF`).
   - Opcode **`$10`**—formerly the Page 2 prefix byte—is now **`HIBYTE #N`**.
3. **`HIBYTE #N` & 16-Bit Numeric Operands**:
   - Any primary 6809 instruction that originally took a **2-byte numeric value** (`#imm16` immediate, `$xxxx` extended address, or `rel16` long branch offset) takes **one postbyte (`lo`)** as its **low byte**.
   - **`HIBYTE #N` (`$10 N`)** is paired with the immediately following instruction to provide the **high byte (`N`)** of that 16-bit numeric value:
     $$\text{Word16} = (N \ll 8) \mid \text{lo}$$
   - **Default High Byte is `$00`**: If an instruction that needs a high byte was **not** immediately preceded by `HIBYTE #N`, its high byte is **`$00`** (`Word16 = $00lo`).
   - **No Interrupt After `HIBYTE`**: When interrupts are enabled, no interrupt (not even `NMI`) can take effect immediately after `HIBYTE #N`; hardware always completes the paired instruction first.
   - **No Repeated `HIBYTE`**: A `HIBYTE` should never be followed by another `HIBYTE`. If it is, the **second `HIBYTE` becomes a `UOP` (`NOP`)** (consuming the `HIBYTE` effect and allowing interrupts again).
4. **Zero-Extra-Byte Indexed Addressing Only**:
   - The `HIBYTE #N` mechanism applies **only** when the original 2-byte suffix was a pure numeric value (`#imm16`, `ext16`, `rel16`)—**never** to multi-byte indexed suffixes that had flag bits in a postbyte followed by offset bytes.
   - Indexed addressing uses a single postbyte (`n5,R` with 5-bit signed offset `-16..+15`, `,R+`, `,R++`, `,-R`, `,--R`, `,R`, `A,R`, `B,R`, `D,R`, and indirect `[...]` forms). Fixed 8-bit (`n8,R`) and 16-bit (`n16,R`) indexed offsets do not exist.
5. **Reset State (`PC = $0100`, All Other Registers `$00`)**:
   - On reset, `PC = $0100`, and **all other registers (`A`, `B`, `DP`, `X`, `Y`, `U`, `S`, `CC`) are cleared to `0`**.
   - In the base configuration, there are no interrupts (`IRQ`, `FIRQ`, `NMI`, `SWI`, `SWI2`, `SWI3`, `RTI`, `CWAI`, `SYNC`) and no vectors at `$FFF0..$FFFF`.
6. **`DP` Register & Direct Addressing**:
   - The 8-bit `DP` (Direct Page) register exists, resets to `$00`, participates normally in `TFR`/`EXG` (nibble `$B`) and `PSHS`/`PULS`/`PSHU`/`PULU` (bit 3, `$08`), and forms the high byte of all Direct Page (`<dir8`) addresses:
     $$\text{EA}_{\text{DIR}} = (\text{DP} \ll 8) \mid \text{dir8}$$
7. **Undefined Opcodes (`UOP`) & Undefined Postbytes**:
   - All undefined opcodes in `$00..$FF` (and any instruction given an unallocated/forbidden indexed postbyte) are **`UOP`**, which acts like a 2-byte `NOP` (advances `PC` by `+2` and modifies no registers, memory, or flags).

---

## 2. Programmer's Model & Registers

The architectural register set is identical to the Motorola 6809:

```
 15            8 7             0
+---------------+---------------+
|       A       |       B       |  D (16-bit concatenated A:B, reset = $0000)
+---------------+---------------+
|               X               |  X (16-bit Index Register, reset = $0000)
+---------------+---------------+
|               Y               |  Y (16-bit Index Register, reset = $0000)
+---------------+---------------+
|               U               |  U (16-bit User Stack / Index, reset = $0000)
+---------------+---------------+
|               S               |  S (16-bit System Stack / Index, reset = $0000)
+---------------+---------------+
|            PC            | 0 |  PC (16-bit Program Counter, bit 0 = 0, reset = $0100)
+---------------+---------------+
                +---------------+
                |      DP       |  DP (8-bit Direct Page Register, reset = $00)
                +---------------+
                | 0 0 H 0 N Z V C|  CC (8-bit Condition Code Register, reset = $00)
                +---------------+
```

### Register Encoding (`TFR` / `EXG` Postbyte)

`TFR r1,r2` (`$1F postbyte`) and `EXG r1,r2` (`$1E postbyte`) encode source in the high nibble (`bits 7..4`) and destination in the low nibble (`bits 3..0`):

- **Nonexistent Registers (`$6..$7`, `$C..$F`)**:
  - `TFR` from a nonexistent register copies zeros (`$00` or `$0000`).
  - `TFR` to a nonexistent register is a `NOP` (discarded).
  - `EXG` with a nonexistent register sets the existing register to `0` and discards its old value.
- **Mixed-Size Transfers (`8-bit <-> 16-bit`)**:
  - Always assigns the low byte to the low byte and clears the high byte (if any):
    - **8-bit $\rightarrow$ 16-bit (`TFR r8, r16`)**: `r16 <- ($00 << 8) | r8` *(clamping bit 0 to `0` if destination is `PC`)*.
    - **16-bit $\rightarrow$ 8-bit (`TFR r16, r8`)**: `r8 <- r16 & $00FF`.
    - **`EXG r8, r16` / `EXG r16, r8`**: `r8_new <- r16_old & $00FF`, `r16_new <- $00 : r8_old`.

| Nibble | Register | Width | Notes |
| :---: | :---: | :---: | :--- |
| `$0` | `D` (`A:B`) | 16-bit | Concatenated `A:B` |
| `$1` | `X` | 16-bit | Index X |
| `$2` | `Y` | 16-bit | Index Y (loaded/stored via `TFR D,Y` / `TFR Y,D` or `PSH`/`PUL`) |
| `$3` | `U` | 16-bit | User Stack Pointer |
| `$4` | `S` | 16-bit | Hardware/System Stack Pointer |
| `$5` | `PC` | 16-bit | Program Counter (bit 0 hardwired to `0`; `TFR D,PC` sets `PC <- D & $FFFE`) |
| `$6..$7` | *(none)* | 16-bit | Nonexistent: reads as `$0000`, writes are `NOP` |
| `$8` | `A` | 8-bit | High byte of `D` |
| `$9` | `B` | 8-bit | Low byte of `D` |
| `$A` | `CC` | 8-bit | Condition Codes (`N`, `Z`, `V`, `C`, `H`) |
| `$B` | `DP` | 8-bit | Direct Page Register (reset = `$00`) |
| `$C..$F` | *(none)* | 8-bit | Nonexistent: reads as `$00`, writes are `NOP` |

### Push/Pull Postbyte Mask (`PSHS`, `PULS`, `PSHU`, `PULU`)

Identical to the Motorola 6809 (`bits 7..0`: `PC`, `U/S`, `Y`, `X`, `DP`, `B`, `A`, `CC`). Pulling `PC` clamps bit 0 to `0` (`PC <- pulled_word & $FFFE`).

### 16-Bit Memory Access Wrap-Around

All 16-bit memory reads and writes access the high byte at `EA` and the low byte at `(EA + 1) & $FFFF`, wrapping around at the 64 KB boundary (`$FFFF` $\rightarrow$ `$0000`) exactly like the Motorola 6809. Direct-page 16-bit accesses at offset `<$FF` access `(DP << 8) | $FF` and `(((DP << 8) | $FF) + 1) & $FFFF`.

---

## 3. Addressing Modes (All 2 Bytes per Machine Instruction)

In the table below, `hi8` is `$N` if the instruction was immediately preceded by `HIBYTE #N` (`$10 N`), or `$00` otherwise:

| Mode | Byte 0 | Byte 1 (Postbyte) | Effective Operand / Address |
| :--- | :---: | :---: | :--- |
| **Inherent (`INH`)** | Opcode | Ignored (`$00`) | Register / implied operation (`PC += 2`) |
| **Immediate 8-bit (`IMM8`)** | Opcode | `imm8` | 8-bit constant `#imm8` |
| **Immediate 16-bit (`IMM16`)** | Opcode | `lo8` | 16-bit constant `#((hi8 << 8) \| lo8)` |
| **Direct Page (`DIR`)** | Opcode | `dir8` | $\text{EA} = (\text{DP} \ll 8) \mid \text{dir8}$ |
| **Extended (`EXT`)** | Opcode | `lo8` | $\text{EA} = (\text{hi8} \ll 8) \mid \text{lo8}$ |
| **Relative 8-bit (`REL8`)** | Opcode | `rel8` | $\text{PC}_{\text{target}} = ((\text{PC} + 2) + \text{sext8}(\text{rel8})) \ \& \ \text{\$FFFE}$ |
| **Relative 16-bit (`REL16`)** | Opcode | `lo8` | $\text{PC}_{\text{target}} = ((\text{PC} + 2) + \text{int16}((\text{hi8} \ll 8) \mid \text{lo8})) \ \& \ \text{\$FFFE}$ |
| **Indexed (`IDX`)** | Opcode | `idx8` | Zero-extra-byte 6809 indexed postbyte (see table below) |

### Allowed Indexed Postbytes (`IDX`)

Because multi-byte indexed suffixes (`n8,R`, `n16,R`, `n8,PC`, `n16,PC`, `[n16]`) are forbidden by the 2-byte rule, only single-postbyte indexed forms are valid. Any instruction with an unallocated or forbidden indexed postbyte pattern executes as a **`UOP` (`NOP`)**:

```
Bit:   7   6   5   4   3   2   1   0
     +---+---+---+---+---+---+---+---+
     | 0 |  RR   |     offset5       |   5-bit signed offset (-16 .. +15), direct
     +---+---+---+---+---+---+---+---+
     | 1 |  RR   | I |     mmmm      |   Auto-inc/dec, zero-offset, or Acc-offset
     +---+---+---+---+---+---+---+---+
```

| Postbyte Pattern | Assembler Syntax | Effective Address ($\text{EA}$) | Indirect (`I=1`) Allowed? |
| :---: | :--- | :--- | :---: |
| `0RRnnnnn` | `n5,R` | $R + \text{sext5}(nnnnn)$ (`-16..+15`) | No (`bit 4` is offset sign) |
| `1RR00000` | `,R+` | $R$, then $R \leftarrow R + 1$ | No (`1RR10000` is `UOP`) |
| `1RRi0001` | `,R++` / `[,R++]` | $R$ (or $\text{Mem16}[R]$), then $R \leftarrow R + 2$ | Yes (`[,R++]`) |
| `1RR00010` | `,-R` | $R \leftarrow R - 1$, then $R$ | No (`1RR10010` is `UOP`) |
| `1RRi0011` | `,--R` / `[,--R]` | $R \leftarrow R - 2$, then $R$ (or $\text{Mem16}[R]$) | Yes (`[,--R]`) |
| `1RRi0100` | `,R` / `[,R]` | $R$ (or $\text{Mem16}[R]$) | Yes (`[,R]`) |
| `1RRi0101` | `B,R` / `[B,R]` | $R + \text{sext8}(B)$ (or indirect) | Yes (`[B,R]`) |
| `1RRi0110` | `A,R` / `[A,R]` | $R + \text{sext8}(A)$ (or indirect) | Yes (`[A,R]`) |
| `1RRi1011` | `D,R` / `[D,R]` | $R + D$ (or indirect) | Yes (`[D,R]`) |

*(Where `RR` = `00` for `X`, `01` for `Y`, `10` for `U`, `11` for `S`. All other `mmmm` values when bit 7 is `1` are `UOP`.)*

---

## 4. How `HIBYTE #N` Pairs with 16-Bit Numeric-Operand Instructions

Whenever an instruction takes a 16-bit numeric value (`#imm16`, `ext16`, or `rel16`), its postbyte supplies the low byte `lo`, and the preceding `HIBYTE #N` instruction (if present) supplies the high byte `N` (defaulting to `$00` if omitted).

### 4.1 16-Bit Immediate Instructions (`#imm16`)

| Opcode | Mnemonic | Standalone 2-Byte Meaning (`hi = $00`) | Paired with `HIBYTE #N` (4 Bytes Total) |
| :---: | :--- | :--- | :--- |
| **`$83 lo`** | `SUBD #lo` | `D <- D - ($00:lo)` | `HIBYTE #$12` (`10 12`) + `SUBD #$34` (`83 34`) $\rightarrow$ `D -= $1234` |
| **`$8C lo`** | `CMPX #lo` | `X - ($00:lo)` | `HIBYTE #$12` (`10 12`) + `CMPX #$34` (`8C 34`) $\rightarrow$ `X - $1234` |
| **`$8E lo`** | `LDX #lo` | `X <- ($00:lo)` | `HIBYTE #$12` (`10 12`) + `LDX #$34` (`8E 34`) $\rightarrow$ `X <- $1234` |
| **`$C3 lo`** | `ADDD #lo` | `D <- D + ($00:lo)` | `HIBYTE #$12` (`10 12`) + `ADDD #$34` (`C3 34`) $\rightarrow$ `D += $1234` |
| **`$CC lo`** | `LDD #lo` | `D <- ($00:lo)` (`A <- 0, B <- lo`) | `HIBYTE #$12` (`10 12`) + `LDD #$34` (`CC 34`) $\rightarrow$ `D <- $1234` |
| **`$CE lo`** | `LDU #lo` | `U <- ($00:lo)` | `HIBYTE #$12` (`10 12`) + `LDU #$34` (`CE 34`) $\rightarrow$ `U <- $1234` |

> **Note on `LDD` and `LDY`/`LDS`**:
> - `LDD #$1234` can be written either as `HIBYTE #$12` + `LDD #$34` (`10 12 CC 34`) or `LDA #$12` + `LDB #$34` (`86 12 C6 34`).
> - Because `Y` and `S` do not have dedicated `LDY`/`LDS` opcodes in the unprefixed table, loading `Y` or `S` is done via `LDA`/`LDB` (or `LDD`/`LDX`) followed by `TFR D,Y` / `TFR D,S`.

### 4.2 Extended Addressing (`$7x`, `$Bx`, `$Fx`)

Every instruction in the `$7x` (Extended RMW + `JMP`), `$Bx` (Extended `A`/`X`/`SUBD`/`CMPX`/`JSR`), and `$Fx` (Extended `B`/`D`/`U`/`ADDD`) columns uses `EA = (hi8 << 8) | lo`:

```nasm
; --- Extended JMP $2400 (4 bytes: HIBYTE + JMP) ---
    HIBYTE  #$24        ; 10 24   (high byte = $24)
    JMP     $00         ; 7E 00   (PC <- $2400)

; --- Extended JSR $2400 (4 bytes: HIBYTE + JSR) ---
    HIBYTE  #$24        ; 10 24   (high byte = $24)
    JSR     $00         ; BD 00   (Push PC; PC <- $2400)

; --- Extended LDA $4050 (4 bytes: HIBYTE + LDA) ---
    HIBYTE  #$40        ; 10 40   (high byte = $40)
    LDA     $50         ; B6 50   (A <- Mem[$4050])
```

### 4.3 Long Branches (`LBRA` `$16`, `LBSR` `$17`)

In the `$1x` column, `LBRA` (`$16`) and `LBSR` (`$17`) form a signed 16-bit PC-relative offset from `(hi8 << 8) | lo`:
$$\text{PC}_{\text{target}} = ((\text{PC} + 2) + \text{int16}((\text{hi8} \ll 8) \mid \text{lo})) \ \& \ \text{\$FFFE}$$

```nasm
; --- Assembler expansion of "LBRA target" (4 bytes) ---
; Let delta = target - (PC_of_HIBYTE + 4)
    HIBYTE  #>delta     ; 10 hh   (high byte of delta)
    LBRA    #<delta     ; 16 ll   (PC <- (PC + 2 + delta) & $FFFE)

; --- Assembler expansion of "LBSR target" (4 bytes) ---
; Let delta = target - (PC_of_HIBYTE + 4)
    HIBYTE  #>delta     ; 10 hh   (high byte of delta)
    LBSR    #<delta     ; 17 ll   (Push PC + 2; PC <- (PC + 2 + delta) & $FFFE)
```

---

## 5. Complete `twostep09` Opcode Matrix (`$00 .. $FF`)

Every single primary 6809 instruction (except removed interrupt instructions `SWI`/`RTI`/`CWAI`/`SYNC` and the `$11` prefix) sits at its **exact original 6809 opcode byte**!

- `<d` = Direct Page `(DP << 8) | dd` (1 postbyte `dd`)
- `idx` = Single-byte Indexed postbyte (5-bit offset, auto-inc/dec, `,R`, `A/B/D,R`, or indirect)
- `#i` = 8-bit Immediate postbyte
- `#hi:lo` = 16-bit Immediate `(hi8 << 8) | lo` (`hi8` from preceding `HIBYTE #N`, or `$00` if none)
- `@hi:lo` = 16-bit Extended Effective Address `(hi8 << 8) | lo` (`hi8` from preceding `HIBYTE #N`, or `$00` if none)
- `r8` = 8-bit signed PC-relative offset
- `hi:r8` = 16-bit signed PC-relative offset `(hi8 << 8) | r8` (`hi8` from preceding `HIBYTE #N`, or `$00` if none)
- `UOP` = **Undefined Opcode**: acts identically to a 2-byte `NOP` (consumes the postbyte, advances `PC` by `+2`, and modifies no registers, memory, or flags)

| Low \ High | `$0x` (DIR RMW) | `$1x` (MISC) | `$2x` (REL8) | `$3x` (LEA/STK) | `$4x` (ACC A) | `$5x` (ACC B) | `$6x` (IDX RMW) | `$7x` (EXT RMW) |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`x0`** | `NEG <d` | **`HIBYTE #i`** | `BRA r8` | `LEAX idx` | `NEGA` | `NEGB` | `NEG idx` | `NEG @hi:lo` |
| **`x1`** | `UOP` | `UOP` | `BRN r8` | `LEAY idx` | `UOP` | `UOP` | `UOP` | `UOP` |
| **`x2`** | `UOP` | `NOP` | `BHI r8` | `LEAS idx` | `UOP` | `UOP` | `UOP` | `UOP` |
| **`x3`** | `COM <d` | `UOP` | `BLS r8` | `LEAU idx` | `COMA` | `COMB` | `COM idx` | `COM @hi:lo` |
| **`x4`** | `LSR <d` | `UOP` | `BCC r8` | `PSHS m8` | `LSRA` | `LSRB` | `LSR idx` | `LSR @hi:lo` |
| **`x5`** | `UOP` | `UOP` | `BCS r8` | `PULS m8` | `UOP` | `UOP` | `UOP` | `UOP` |
| **`x6`** | `ROR <d` | `LBRA hi:r8` | `BNE r8` | `PSHU m8` | `RORA` | `RORB` | `ROR idx` | `ROR @hi:lo` |
| **`x7`** | `ASR <d` | `LBSR hi:r8` | `BEQ r8` | `PULU m8` | `ASRA` | `ASRB` | `ASR idx` | `ASR @hi:lo` |
| **`x8`** | `ASL <d` | `UOP` | `BVC r8` | `UOP` | `ASLA` | `ASLB` | `ASL idx` | `ASL @hi:lo` |
| **`x9`** | `ROL <d` | `DAA` | `BVS r8` | `RTS` | `ROLA` | `ROLB` | `ROL idx` | `ROL @hi:lo` |
| **`xA`** | `DEC <d` | `ORCC #i` | `BPL r8` | `ABX` | `DECA` | `DECB` | `DEC idx` | `DEC @hi:lo` |
| **`xB`** | `UOP` | `UOP` | `BMI r8` | `UOP` | `UOP` | `UOP` | `UOP` | `UOP` |
| **`xC`** | `INC <d` | `ANDCC #i` | `BGE r8` | `UOP` | `INCA` | `INCB` | `INC idx` | `INC @hi:lo` |
| **`xD`** | `TST <d` | `SEX` | `BLT r8` | `MUL` | `TSTA` | `TSTB` | `TST idx` | `TST @hi:lo` |
| **`xE`** | `JMP <d` | `EXG r,r` | `BGT r8` | `UOP` | `UOP` | `UOP` | `JMP idx` | `JMP @hi:lo` |
| **`xF`** | `CLR <d` | `TFR r,r` | `BLE r8` | `UOP` | `CLRA` | `CLRB` | `CLR idx` | `CLR @hi:lo` |

| Low \ High | `$8x` (A/X IMM) | `$9x` (A/X DIR) | `$Ax` (A/X IDX) | `$Bx` (A/X EXT) | `$Cx` (B/D/U IMM) | `$Dx` (B/D/U DIR) | `$Ex` (B/D/U IDX) | `$Fx` (B/D/U EXT) |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **`x0`** | `SUBA #i` | `SUBA <d` | `SUBA idx` | `SUBA @hi:lo` | `SUBB #i` | `SUBB <d` | `SUBB idx` | `SUBB @hi:lo` |
| **`x1`** | `CMPA #i` | `CMPA <d` | `CMPA idx` | `CMPA @hi:lo` | `CMPB #i` | `CMPB <d` | `CMPB idx` | `CMPB @hi:lo` |
| **`x2`** | `SBCA #i` | `SBCA <d` | `SBCA idx` | `SBCA @hi:lo` | `SBCB #i` | `SBCB <d` | `SBCB idx` | `SBCB @hi:lo` |
| **`x3`** | `SUBD #hi:lo` | `SUBD <d` | `SUBD idx` | `SUBD @hi:lo` | `ADDD #hi:lo` | `ADDD <d` | `ADDD idx` | `ADDD @hi:lo` |
| **`x4`** | `ANDA #i` | `ANDA <d` | `ANDA idx` | `ANDA @hi:lo` | `ANDB #i` | `ANDB <d` | `ANDB idx` | `ANDB @hi:lo` |
| **`x5`** | `BITA #i` | `BITA <d` | `BITA idx` | `BITA @hi:lo` | `BITB #i` | `BITB <d` | `BITB idx` | `BITB @hi:lo` |
| **`x6`** | `LDA #i` | `LDA <d` | `LDA idx` | `LDA @hi:lo` | `LDB #i` | `LDB <d` | `LDB idx` | `LDB @hi:lo` |
| **`x7`** | `UOP` | `STA <d` | `STA idx` | `STA @hi:lo` | `UOP` | `STB <d` | `STB idx` | `STB @hi:lo` |
| **`x8`** | `EORA #i` | `EORA <d` | `EORA idx` | `EORA @hi:lo` | `EORB #i` | `EORB <d` | `EORB idx` | `EORB @hi:lo` |
| **`x9`** | `ADCA #i` | `ADCA <d` | `ADCA idx` | `ADCA @hi:lo` | `ADCB #i` | `ADCB <d` | `ADCB idx` | `ADCB @hi:lo` |
| **`xA`** | `ORA #i` | `ORA <d` | `ORA idx` | `ORA @hi:lo` | `ORB #i` | `ORB <d` | `ORB idx` | `ORB @hi:lo` |
| **`xB`** | `ADDA #i` | `ADDA <d` | `ADDA idx` | `ADDA @hi:lo` | `ADDB #i` | `ADDB <d` | `ADDB idx` | `ADDB @hi:lo` |
| **`xC`** | `CMPX #hi:lo` | `CMPX <d` | `CMPX idx` | `CMPX @hi:lo` | `LDD #hi:lo` | `LDD <d` | `LDD idx` | `LDD @hi:lo` |
| **`xD`** | `BSR r8` | `JSR <d` | `JSR idx` | `JSR @hi:lo` | `UOP` | `STD <d` | `STD idx` | `STD @hi:lo` |
| **`xE`** | `LDX #hi:lo` | `LDX <d` | `LDX idx` | `LDX @hi:lo` | `LDU #hi:lo` | `LDU <d` | `LDU idx` | `LDU @hi:lo` |
| **`xF`** | `UOP` | `STX <d` | `STX idx` | `STX @hi:lo` | `UOP` | `STU <d` | `STU idx` | `STU @hi:lo` |

---

## 6. Example Program: `twostep09` in Action

```nasm
; ============================================================================
; twostep09 Demo: Copy string from $0200 to $0300 and store length at $0080
; Every machine instruction is strictly 2 bytes!
; ============================================================================
        ORG     $0100           ; Reset entry point (PC=$0100, all other regs=0)

start:
        ; Set System Stack S = $8000 (via A, B, TFR)
        LDA     #$80            ; 86 80
        LDB     #$00            ; C6 00
        TFR     D,S             ; 1F 04

        ; Load X = $0200 using HIBYTE + LDX# (4 bytes = two 2-byte instructions)
        HIBYTE  #$02            ; 10 02
        LDX     #$00            ; 8E 00   (X <- $0200)

        ; Load U = $0300 using HIBYTE + LDU# (4 bytes)
        HIBYTE  #$03            ; 10 03
        LDU     #$00            ; CE 00   (U <- $0300)

        ; Call strcpy via Extended JSR (4 bytes: HIBYTE + JSR)
        HIBYTE  #>strcpy        ; 10 01
        JSR     <strcpy         ; BD 18   (JSR $0118)

        ; Store 16-bit length D into Direct Page $0080..$0081 (2 bytes; DP is $00!)
        STD     <$80            ; DD 80

halt:
        BRA     halt            ; 20 FE   (spin forever)

; ----------------------------------------------------------------------------
; strcpy: copy NUL-terminated string from ,X+ to ,U+, return length in D
; ----------------------------------------------------------------------------
strcpy:
        CLRA                    ; 4F 00   (2-byte inherent; $00 postbyte ignored)
        CLRB                    ; 5F 00   (D = 0 length counter)
.loop:
        LDA     ,X+             ; A6 80   (load char from X++)
        STA     ,U+             ; A7 C0   (store char to U++)
        BEQ     .done           ; 27 04   (if NUL, exit loop)
        ADDD    #$01            ; C3 01   (no HIBYTE needed -> D <- D + $0001!)
        BRA     .loop           ; 20 F6   (loop back)
.done:
        RTS                     ; 39 00   (2-byte RTS; $00 postbyte ignored)
```
