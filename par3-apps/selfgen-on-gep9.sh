#!/usr/bin/env bash
# ==============================================================================
# selfgen-on-gep9.sh
#
# Generates a NitrOS-9 disk image (/tmp/selfgen-on-gep9.disk) containing all
# source code, runtime templates, and bootstrap tools needed to rebuild the
# deep65280 boot image (deep65280.img) and the self-hosted Par3 toolchain
# entirely within NitrOS-9 processes on gep9.
#
# Executes a 3-stage regeneration (Red -> Green -> Blue) ending with cmp
# verification between Green and Blue images and command binaries.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MINIGOLFDIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PAR3APPS="$MINIGOLFDIR/par3-apps"
NITROS9DIR="${NITROS9DIR:-/home/strick/modoc/coco-shelf/nitros9}"
DISK="${1:-/tmp/selfgen-on-gep9.disk}"
GEP9="${GEP9:-/tmp/gep9}"
BOOT_IMG="$NITROS9DIR/recipes/deep65280/deep65280.img"

if [ ! -x "$GEP9" ]; then
    if [ -x "/home/strick/github.com/strickyak/hatvan-os/build/gep9" ]; then
        GEP9="/home/strick/github.com/strickyak/hatvan-os/build/gep9"
    else
        echo "ERROR: gep9 emulator not found at $GEP9" >&2
        exit 1
    fi
fi

if [ ! -f "$BOOT_IMG" ]; then
    echo "ERROR: Boot image not found at $BOOT_IMG" >&2
    exit 1
fi

TMP_DIR="$(mktemp -d /tmp/selfgen_prep.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "=== 1. Initializing and formatting disk image: $DISK ==="
rm -f "$DISK"
os9 format -e -n'DEEP65280' -l'40000' "$DISK"
os9 makdir "$DISK,CMDS"
os9 makdir "$DISK,SRC"

# Helper to convert a file to CR line endings
to_cr() {
    local src="$1"
    local dst="$2"
    tr '\n' '\r' < "$src" > "$dst"
}

echo "=== 2. Preparing defsfile ==="
cat << 'EOF' > "$TMP_DIR/defs_raw.asm"
Level set 2
DEEP_256K set 1
deep65280 set 1
flat65280 set 0
NOS9VER set 0
NOS9MAJ set 0
NOS9MIN set 0
picothing set 0
wildbits set 0
mc09 set 0
TC9 set 0
H6309 set 0
coco3 set 0
atari set 0
corsham set 0
FLAT65280 set 0
dalpha set 0
ROM set 0
NOS9DEV set 0
coco set 0
DOHELP set 0
EOF

cat "$NITROS9DIR/defs/os9.d" >> "$TMP_DIR/defs_raw.asm"
cat "$NITROS9DIR/defs/scf.d" >> "$TMP_DIR/defs_raw.asm"
cat "$NITROS9DIR/defs/rbf.d" >> "$TMP_DIR/defs_raw.asm"
cat "$NITROS9DIR/defs/deep65280.d" >> "$TMP_DIR/defs_raw.asm"
cat "$NITROS9DIR/defs/cocovtio.d" >> "$TMP_DIR/defs_raw.asm"

python3 "$PAR3APPS/selfgen_preprocess.py" --no-collisions "$TMP_DIR/defs_raw.asm" "$TMP_DIR/defsfile_pre"
to_cr "$TMP_DIR/defsfile_pre" "$TMP_DIR/defsfile"
os9 copy -l -r "$TMP_DIR/defsfile" "$DISK,SRC/defsfile"

echo "=== 3. Inlining and preparing kernel module sources ==="
python3 - << PYEOF
import os

nitros9 = '$NITROS9DIR'
tmp_dir = '$TMP_DIR'

inc_dirs = [
    f'{nitros9}/level2/modules/kernel',
    f'{nitros9}/level1/modules/kernel',
    f'{nitros9}/level2/modules',
    f'{nitros9}/level1/modules',
    f'{nitros9}/defs'
]

def inline_asm(src_path, out_path):
    with open(src_path, 'r', encoding='latin1') as f:
        lines = f.readlines()
    with open(out_path, 'w', encoding='latin1') as out:
        for line in lines:
            s = line.strip()
            parts = s.split()
            if len(parts) >= 2 and parts[0].lower() == 'use' and parts[1].startswith('f') and parts[1].endswith('.asm'):
                fname = parts[1]
                found = None
                for d in inc_dirs:
                    cand = os.path.join(d, fname)
                    if os.path.isfile(cand):
                        found = cand
                        break
                if found:
                    out.write(f'* --- BEGIN INLINE {fname} ---\n')
                    with open(found, 'r', encoding='latin1') as inc_f:
                        out.write(inc_f.read())
                    out.write(f'\n* --- END INLINE {fname} ---\n')
                else:
                    raise Exception(f'Include not found: {fname}')
            else:
                out.write(line)

# Inline krn.asm and krnp2.asm
inline_asm(f'{nitros9}/level2/modules/kernel/krn.asm', f'{tmp_dir}/krn.asm')
inline_asm(f'{nitros9}/level2/modules/kernel/krnp2.asm', f'{tmp_dir}/krnp2.asm')

# Map standard module sources
std_sources = {
    'dd.asm': f'{nitros9}/level1/modules/emudskdesc.asm',
    'sysgo.asm': f'{nitros9}/level1/modules/sysgo.asm',
    'init.asm': f'{nitros9}/level2/deep65280/modules/init.asm',
    'ioman.asm': f'{nitros9}/level1/modules/ioman.asm',
    'rbf.asm': f'{nitros9}/level2/modules/rbf.asm',
    'scf.asm': f'{nitros9}/level1/modules/scf.asm',
    'sc6850.asm': f'{nitros9}/level1/modules/sc6850.asm',
    'emudsk.asm': f'{nitros9}/level1/modules/emudsk.asm',
    'clock.asm': f'{nitros9}/level2/deep65280/modules/clock.asm',
    'term.asm': f'{nitros9}/level1/modules/term_sc6850.asm',
    'shell_21.asm': f'{nitros9}/level1/cmds/shell_21.asm',
    'mfree.asm': f'{nitros9}/level2/cmds/mfree.asm',
    'free.asm': f'{nitros9}/level1/cmds/free.asm',
    'procs.asm': f'{nitros9}/level2/cmds/procs.asm',
    'mdir.asm': f'{nitros9}/level2/cmds/mdir.asm',
    'dir.asm': f'{nitros9}/level1/cmds/dir.asm',
    'setime.asm': f'{nitros9}/level1/cmds/setime.asm',
    'date.asm': f'{nitros9}/level1/cmds/date.asm',
    'stop.asm': f'{nitros9}/level2/deep65280/cmds/stop.asm',
}

for name, src in std_sources.items():
    with open(src, 'r', encoding='latin1') as in_f:
        with open(f'{tmp_dir}/{name}', 'w', encoding='latin1') as out_f:
            out_f.write(in_f.read())

PYEOF

MODULES=(
    krn krnp2 init ioman rbf scf sc6850 emudsk clock term dd sysgo
    shell_21 mfree free procs mdir dir setime date stop
)

for m in "${MODULES[@]}"; do
    to_cr "$TMP_DIR/$m.asm" "$TMP_DIR/$m.cr.asm"
    os9 copy -l -r "$TMP_DIR/$m.cr.asm" "$DISK,SRC/$m.asm"
done

echo "=== 4. Copying Par3 runtime and tool sources ==="
to_cr "$MINIGOLFDIR/par3-runtime/template.asm" "$TMP_DIR/template.asm"
os9 copy -l -r "$TMP_DIR/template.asm" "$DISK,SRC/template.asm"

PAR3_TOOLS=(minigolf_par3 par3asm par3pack selfgen_preprocess build_img)
for t in "${PAR3_TOOLS[@]}"; do
    to_cr "$PAR3APPS/$t.par3" "$TMP_DIR/$t.par3"
    os9 copy -l -r "$TMP_DIR/$t.par3" "$DISK,SRC/$t.par3"
done

echo "=== 5. Populating bootstrap binaries in /dd/CMDS ==="
for t in "${PAR3_TOOLS[@]}"; do
    os9 copy -o=0 -r "$PAR3APPS/$t" "$DISK,CMDS/$t"
done

STD_CMDS=(
    asm shell echo cmp copy del deldir makdir attr tmode setime date stop free mfree procs dir mdir
)
for c in "${STD_CMDS[@]}"; do
    os9 copy -o=0 -r "$NITROS9DIR/recipes/deep65280/.mods/$c" "$DISK,CMDS/$c"
done

# Set execute attributes on all bootstrap commands
attr_args=()
for t in "${PAR3_TOOLS[@]}" "${STD_CMDS[@]}"; do
    attr_args+=("$DISK,CMDS/$t")
done
os9 attr -q -pe -npw -pr -e -w -r "${attr_args[@]}"

echo "=== 6. Generating /dd/startup script ==="
cat << 'STARTUP_EOF' > "$TMP_DIR/startup.raw"
tmode </term pau=0
chx /dd/CMDS
chd /dd
x

* ====================================================================
* STAGE RED (Bootstrap -> Red)
* ====================================================================
echo === STAGE RED: Preprocessing kernel modules ===
makdir /dd/Red
makdir /dd/Red/PRE
makdir /dd/Red/MODS
makdir /dd/Red/CMDS
copy /dd/SRC/defsfile /dd/Red/PRE/defsfile
copy /dd/SRC/defsfile /dd/Red/defsfile

selfgen_preprocess /dd/SRC/krn.asm /dd/Red/PRE/krn.asm
selfgen_preprocess /dd/SRC/krnp2.asm /dd/Red/PRE/krnp2.asm
selfgen_preprocess /dd/SRC/init.asm /dd/Red/PRE/init.asm
selfgen_preprocess /dd/SRC/ioman.asm /dd/Red/PRE/ioman.asm
selfgen_preprocess /dd/SRC/rbf.asm /dd/Red/PRE/rbf.asm
selfgen_preprocess /dd/SRC/scf.asm /dd/Red/PRE/scf.asm
selfgen_preprocess /dd/SRC/sc6850.asm /dd/Red/PRE/sc6850.asm
selfgen_preprocess /dd/SRC/emudsk.asm /dd/Red/PRE/emudsk.asm
selfgen_preprocess /dd/SRC/clock.asm /dd/Red/PRE/clock.asm
selfgen_preprocess /dd/SRC/term.asm /dd/Red/PRE/term.asm
selfgen_preprocess -DDD=1 -DDNum=0 /dd/SRC/dd.asm /dd/Red/PRE/dd.asm
selfgen_preprocess -DDD=1 /dd/SRC/sysgo.asm /dd/Red/PRE/sysgo.asm
selfgen_preprocess /dd/SRC/shell_21.asm /dd/Red/PRE/shell_21.asm
selfgen_preprocess /dd/SRC/mfree.asm /dd/Red/PRE/mfree.asm
selfgen_preprocess /dd/SRC/free.asm /dd/Red/PRE/free.asm
selfgen_preprocess /dd/SRC/procs.asm /dd/Red/PRE/procs.asm
selfgen_preprocess /dd/SRC/mdir.asm /dd/Red/PRE/mdir.asm
selfgen_preprocess /dd/SRC/dir.asm /dd/Red/PRE/dir.asm
selfgen_preprocess /dd/SRC/setime.asm /dd/Red/PRE/setime.asm
selfgen_preprocess /dd/SRC/date.asm /dd/Red/PRE/date.asm
selfgen_preprocess /dd/SRC/stop.asm /dd/Red/PRE/stop.asm

echo === STAGE RED: Assembling kernel modules ===
chd /dd/Red/PRE
asm #48k krn.asm e u -o=/dd/Red/MODS/krn
asm #48k krnp2.asm e u -o=/dd/Red/MODS/krnp2
asm #48k init.asm e u -o=/dd/Red/MODS/init
asm #48k ioman.asm e u -o=/dd/Red/MODS/ioman
asm #48k rbf.asm e u -o=/dd/Red/MODS/rbf
asm #48k scf.asm e u -o=/dd/Red/MODS/scf
asm #48k sc6850.asm e u -o=/dd/Red/MODS/sc6850
asm #48k emudsk.asm e u -o=/dd/Red/MODS/emudsk
asm #48k clock.asm e u -o=/dd/Red/MODS/clock
asm #48k term.asm e u -o=/dd/Red/MODS/term
asm #48k dd.asm e u -o=/dd/Red/MODS/dd
asm #48k sysgo.asm e u -o=/dd/Red/MODS/sysgo
asm #48k shell_21.asm e u -o=/dd/Red/MODS/shell_21
asm #48k mfree.asm e u -o=/dd/Red/MODS/mfree
asm #48k free.asm e u -o=/dd/Red/MODS/free
asm #48k procs.asm e u -o=/dd/Red/MODS/procs
asm #48k mdir.asm e u -o=/dd/Red/MODS/mdir
asm #48k dir.asm e u -o=/dd/Red/MODS/dir
asm #48k setime.asm e u -o=/dd/Red/MODS/setime
asm #48k date.asm e u -o=/dd/Red/MODS/date
asm #48k stop.asm e u -o=/dd/Red/MODS/stop

echo === STAGE RED: Building Red boot image ===
chd /dd
build_img --deep /dd/Red/MODS /dd/Red/deep65280.img

echo === STAGE RED: Recompiling Par3 tools ===
selfgen_preprocess /dd/SRC/template.asm /dd/Red/template.prep.asm
minigolf_par3 /dd/SRC/par3pack.par3 /dd/Red/par3pack.p3a
par3asm /dd/Red/par3pack.p3a -o /dd/Red/par3pack.p3p
par3pack /dd/Red/par3pack.p3p par3pack /dd/Red
chd /dd/Red
asm #48k template.prep.asm u -o=/dd/Red/CMDS/par3pack
attr /dd/Red/CMDS/par3pack e pe

chd /dd
minigolf_par3 /dd/SRC/build_img.par3 /dd/Red/build_img.p3a
par3asm /dd/Red/build_img.p3a -o /dd/Red/build_img.p3p
par3pack /dd/Red/build_img.p3p build_img /dd/Red
chd /dd/Red
asm #48k template.prep.asm u -o=/dd/Red/CMDS/build_img
attr /dd/Red/CMDS/build_img e pe

chd /dd
minigolf_par3 /dd/SRC/selfgen_preprocess.par3 /dd/Red/selfgen_preprocess.p3a
par3asm /dd/Red/selfgen_preprocess.p3a -o /dd/Red/selfgen_preprocess.p3p
par3pack /dd/Red/selfgen_preprocess.p3p selfgen_preprocess /dd/Red
chd /dd/Red
asm #48k template.prep.asm u -o=/dd/Red/CMDS/selfgen_preprocess
attr /dd/Red/CMDS/selfgen_preprocess e pe

chd /dd
minigolf_par3 /dd/SRC/par3asm.par3 /dd/Red/par3asm.p3a
par3asm /dd/Red/par3asm.p3a -o /dd/Red/par3asm.p3p
par3pack /dd/Red/par3asm.p3p par3asm /dd/Red
chd /dd/Red
asm #48k template.prep.asm u -o=/dd/Red/CMDS/par3asm
attr /dd/Red/CMDS/par3asm e pe

chd /dd
minigolf_par3 /dd/SRC/minigolf_par3.par3 /dd/Red/minigolf_par3.p3a
par3asm /dd/Red/minigolf_par3.p3a -o /dd/Red/minigolf_par3.p3p
par3pack /dd/Red/minigolf_par3.p3p minigolf_par3 /dd/Red
chd /dd/Red
asm #48k template.prep.asm u -o=/dd/Red/CMDS/minigolf_par3
attr /dd/Red/CMDS/minigolf_par3 e pe

copy /dd/CMDS/asm /dd/Red/CMDS/asm
copy /dd/CMDS/shell /dd/Red/CMDS/shell
copy /dd/CMDS/echo /dd/Red/CMDS/echo
copy /dd/CMDS/cmp /dd/Red/CMDS/cmp
copy /dd/CMDS/copy /dd/Red/CMDS/copy
copy /dd/CMDS/makdir /dd/Red/CMDS/makdir
copy /dd/CMDS/attr /dd/Red/CMDS/attr
copy /dd/CMDS/stop /dd/Red/CMDS/stop

* ====================================================================
* STAGE GREEN (Red -> Green)
* ====================================================================
echo === STAGE GREEN: Running with Red toolchain ===
chx /dd/Red/CMDS
makdir /dd/Green
makdir /dd/Green/PRE
makdir /dd/Green/MODS
makdir /dd/Green/CMDS
copy /dd/SRC/defsfile /dd/Green/PRE/defsfile
copy /dd/SRC/defsfile /dd/Green/defsfile

selfgen_preprocess /dd/SRC/krn.asm /dd/Green/PRE/krn.asm
selfgen_preprocess /dd/SRC/krnp2.asm /dd/Green/PRE/krnp2.asm
selfgen_preprocess /dd/SRC/init.asm /dd/Green/PRE/init.asm
selfgen_preprocess /dd/SRC/ioman.asm /dd/Green/PRE/ioman.asm
selfgen_preprocess /dd/SRC/rbf.asm /dd/Green/PRE/rbf.asm
selfgen_preprocess /dd/SRC/scf.asm /dd/Green/PRE/scf.asm
selfgen_preprocess /dd/SRC/sc6850.asm /dd/Green/PRE/sc6850.asm
selfgen_preprocess /dd/SRC/emudsk.asm /dd/Green/PRE/emudsk.asm
selfgen_preprocess /dd/SRC/clock.asm /dd/Green/PRE/clock.asm
selfgen_preprocess /dd/SRC/term.asm /dd/Green/PRE/term.asm
selfgen_preprocess -DDD=1 -DDNum=0 /dd/SRC/dd.asm /dd/Green/PRE/dd.asm
selfgen_preprocess -DDD=1 /dd/SRC/sysgo.asm /dd/Green/PRE/sysgo.asm
selfgen_preprocess /dd/SRC/shell_21.asm /dd/Green/PRE/shell_21.asm
selfgen_preprocess /dd/SRC/mfree.asm /dd/Green/PRE/mfree.asm
selfgen_preprocess /dd/SRC/free.asm /dd/Green/PRE/free.asm
selfgen_preprocess /dd/SRC/procs.asm /dd/Green/PRE/procs.asm
selfgen_preprocess /dd/SRC/mdir.asm /dd/Green/PRE/mdir.asm
selfgen_preprocess /dd/SRC/dir.asm /dd/Green/PRE/dir.asm
selfgen_preprocess /dd/SRC/setime.asm /dd/Green/PRE/setime.asm
selfgen_preprocess /dd/SRC/date.asm /dd/Green/PRE/date.asm
selfgen_preprocess /dd/SRC/stop.asm /dd/Green/PRE/stop.asm

echo === STAGE GREEN: Assembling kernel modules ===
chd /dd/Green/PRE
asm #48k krn.asm e u -o=/dd/Green/MODS/krn
asm #48k krnp2.asm e u -o=/dd/Green/MODS/krnp2
asm #48k init.asm e u -o=/dd/Green/MODS/init
asm #48k ioman.asm e u -o=/dd/Green/MODS/ioman
asm #48k rbf.asm e u -o=/dd/Green/MODS/rbf
asm #48k scf.asm e u -o=/dd/Green/MODS/scf
asm #48k sc6850.asm e u -o=/dd/Green/MODS/sc6850
asm #48k emudsk.asm e u -o=/dd/Green/MODS/emudsk
asm #48k clock.asm e u -o=/dd/Green/MODS/clock
asm #48k term.asm e u -o=/dd/Green/MODS/term
asm #48k dd.asm e u -o=/dd/Green/MODS/dd
asm #48k sysgo.asm e u -o=/dd/Green/MODS/sysgo
asm #48k shell_21.asm e u -o=/dd/Green/MODS/shell_21
asm #48k mfree.asm e u -o=/dd/Green/MODS/mfree
asm #48k free.asm e u -o=/dd/Green/MODS/free
asm #48k procs.asm e u -o=/dd/Green/MODS/procs
asm #48k mdir.asm e u -o=/dd/Green/MODS/mdir
asm #48k dir.asm e u -o=/dd/Green/MODS/dir
asm #48k setime.asm e u -o=/dd/Green/MODS/setime
asm #48k date.asm e u -o=/dd/Green/MODS/date
asm #48k stop.asm e u -o=/dd/Green/MODS/stop

echo === STAGE GREEN: Building Green boot image ===
chd /dd
build_img --deep /dd/Green/MODS /dd/Green/deep65280.img

echo === STAGE GREEN: Recompiling Par3 tools ===
selfgen_preprocess /dd/SRC/template.asm /dd/Green/template.prep.asm
minigolf_par3 /dd/SRC/par3pack.par3 /dd/Green/par3pack.p3a
par3asm /dd/Green/par3pack.p3a -o /dd/Green/par3pack.p3p
par3pack /dd/Green/par3pack.p3p par3pack /dd/Green
chd /dd/Green
asm #48k template.prep.asm u -o=/dd/Green/CMDS/par3pack
attr /dd/Green/CMDS/par3pack e pe

chd /dd
minigolf_par3 /dd/SRC/build_img.par3 /dd/Green/build_img.p3a
par3asm /dd/Green/build_img.p3a -o /dd/Green/build_img.p3p
par3pack /dd/Green/build_img.p3p build_img /dd/Green
chd /dd/Green
asm #48k template.prep.asm u -o=/dd/Green/CMDS/build_img
attr /dd/Green/CMDS/build_img e pe

chd /dd
minigolf_par3 /dd/SRC/selfgen_preprocess.par3 /dd/Green/selfgen_preprocess.p3a
par3asm /dd/Green/selfgen_preprocess.p3a -o /dd/Green/selfgen_preprocess.p3p
par3pack /dd/Green/selfgen_preprocess.p3p selfgen_preprocess /dd/Green
chd /dd/Green
asm #48k template.prep.asm u -o=/dd/Green/CMDS/selfgen_preprocess
attr /dd/Green/CMDS/selfgen_preprocess e pe

chd /dd
minigolf_par3 /dd/SRC/par3asm.par3 /dd/Green/par3asm.p3a
par3asm /dd/Green/par3asm.p3a -o /dd/Green/par3asm.p3p
par3pack /dd/Green/par3asm.p3p par3asm /dd/Green
chd /dd/Green
asm #48k template.prep.asm u -o=/dd/Green/CMDS/par3asm
attr /dd/Green/CMDS/par3asm e pe

chd /dd
minigolf_par3 /dd/SRC/minigolf_par3.par3 /dd/Green/minigolf_par3.p3a
par3asm /dd/Green/minigolf_par3.p3a -o /dd/Green/minigolf_par3.p3p
par3pack /dd/Green/minigolf_par3.p3p minigolf_par3 /dd/Green
chd /dd/Green
asm #48k template.prep.asm u -o=/dd/Green/CMDS/minigolf_par3
attr /dd/Green/CMDS/minigolf_par3 e pe

copy /dd/Red/CMDS/asm /dd/Green/CMDS/asm
copy /dd/Red/CMDS/shell /dd/Green/CMDS/shell
copy /dd/Red/CMDS/echo /dd/Green/CMDS/echo
copy /dd/Red/CMDS/cmp /dd/Green/CMDS/cmp
copy /dd/Red/CMDS/copy /dd/Green/CMDS/copy
copy /dd/Red/CMDS/makdir /dd/Green/CMDS/makdir
copy /dd/Red/CMDS/attr /dd/Green/CMDS/attr
copy /dd/Red/CMDS/stop /dd/Green/CMDS/stop

* ====================================================================
* STAGE BLUE (Green -> Blue)
* ====================================================================
echo === STAGE BLUE: Running with Green toolchain ===
chx /dd/Green/CMDS
makdir /dd/Blue
makdir /dd/Blue/PRE
makdir /dd/Blue/MODS
makdir /dd/Blue/CMDS
copy /dd/SRC/defsfile /dd/Blue/PRE/defsfile
copy /dd/SRC/defsfile /dd/Blue/defsfile

selfgen_preprocess /dd/SRC/krn.asm /dd/Blue/PRE/krn.asm
selfgen_preprocess /dd/SRC/krnp2.asm /dd/Blue/PRE/krnp2.asm
selfgen_preprocess /dd/SRC/init.asm /dd/Blue/PRE/init.asm
selfgen_preprocess /dd/SRC/ioman.asm /dd/Blue/PRE/ioman.asm
selfgen_preprocess /dd/SRC/rbf.asm /dd/Blue/PRE/rbf.asm
selfgen_preprocess /dd/SRC/scf.asm /dd/Blue/PRE/scf.asm
selfgen_preprocess /dd/SRC/sc6850.asm /dd/Blue/PRE/sc6850.asm
selfgen_preprocess /dd/SRC/emudsk.asm /dd/Blue/PRE/emudsk.asm
selfgen_preprocess /dd/SRC/clock.asm /dd/Blue/PRE/clock.asm
selfgen_preprocess /dd/SRC/term.asm /dd/Blue/PRE/term.asm
selfgen_preprocess -DDD=1 -DDNum=0 /dd/SRC/dd.asm /dd/Blue/PRE/dd.asm
selfgen_preprocess -DDD=1 /dd/SRC/sysgo.asm /dd/Blue/PRE/sysgo.asm
selfgen_preprocess /dd/SRC/shell_21.asm /dd/Blue/PRE/shell_21.asm
selfgen_preprocess /dd/SRC/mfree.asm /dd/Blue/PRE/mfree.asm
selfgen_preprocess /dd/SRC/free.asm /dd/Blue/PRE/free.asm
selfgen_preprocess /dd/SRC/procs.asm /dd/Blue/PRE/procs.asm
selfgen_preprocess /dd/SRC/mdir.asm /dd/Blue/PRE/mdir.asm
selfgen_preprocess /dd/SRC/dir.asm /dd/Blue/PRE/dir.asm
selfgen_preprocess /dd/SRC/setime.asm /dd/Blue/PRE/setime.asm
selfgen_preprocess /dd/SRC/date.asm /dd/Blue/PRE/date.asm
selfgen_preprocess /dd/SRC/stop.asm /dd/Blue/PRE/stop.asm

echo === STAGE BLUE: Assembling kernel modules ===
chd /dd/Blue/PRE
asm #48k krn.asm e u -o=/dd/Blue/MODS/krn
asm #48k krnp2.asm e u -o=/dd/Blue/MODS/krnp2
asm #48k init.asm e u -o=/dd/Blue/MODS/init
asm #48k ioman.asm e u -o=/dd/Blue/MODS/ioman
asm #48k rbf.asm e u -o=/dd/Blue/MODS/rbf
asm #48k scf.asm e u -o=/dd/Blue/MODS/scf
asm #48k sc6850.asm e u -o=/dd/Blue/MODS/sc6850
asm #48k emudsk.asm e u -o=/dd/Blue/MODS/emudsk
asm #48k clock.asm e u -o=/dd/Blue/MODS/clock
asm #48k term.asm e u -o=/dd/Blue/MODS/term
asm #48k dd.asm e u -o=/dd/Blue/MODS/dd
asm #48k sysgo.asm e u -o=/dd/Blue/MODS/sysgo
asm #48k shell_21.asm e u -o=/dd/Blue/MODS/shell_21
asm #48k mfree.asm e u -o=/dd/Blue/MODS/mfree
asm #48k free.asm e u -o=/dd/Blue/MODS/free
asm #48k procs.asm e u -o=/dd/Blue/MODS/procs
asm #48k mdir.asm e u -o=/dd/Blue/MODS/mdir
asm #48k dir.asm e u -o=/dd/Blue/MODS/dir
asm #48k setime.asm e u -o=/dd/Blue/MODS/setime
asm #48k date.asm e u -o=/dd/Blue/MODS/date
asm #48k stop.asm e u -o=/dd/Blue/MODS/stop

echo === STAGE BLUE: Building Blue boot image ===
chd /dd
build_img --deep /dd/Blue/MODS /dd/Blue/deep65280.img

echo === STAGE BLUE: Recompiling Par3 tools ===
selfgen_preprocess /dd/SRC/template.asm /dd/Blue/template.prep.asm
minigolf_par3 /dd/SRC/par3pack.par3 /dd/Blue/par3pack.p3a
par3asm /dd/Blue/par3pack.p3a -o /dd/Blue/par3pack.p3p
par3pack /dd/Blue/par3pack.p3p par3pack /dd/Blue
chd /dd/Blue
asm #48k template.prep.asm u -o=/dd/Blue/CMDS/par3pack
attr /dd/Blue/CMDS/par3pack e pe

chd /dd
minigolf_par3 /dd/SRC/build_img.par3 /dd/Blue/build_img.p3a
par3asm /dd/Blue/build_img.p3a -o /dd/Blue/build_img.p3p
par3pack /dd/Blue/build_img.p3p build_img /dd/Blue
chd /dd/Blue
asm #48k template.prep.asm u -o=/dd/Blue/CMDS/build_img
attr /dd/Blue/CMDS/build_img e pe

chd /dd
minigolf_par3 /dd/SRC/selfgen_preprocess.par3 /dd/Blue/selfgen_preprocess.p3a
par3asm /dd/Blue/selfgen_preprocess.p3a -o /dd/Blue/selfgen_preprocess.p3p
par3pack /dd/Blue/selfgen_preprocess.p3p selfgen_preprocess /dd/Blue
chd /dd/Blue
asm #48k template.prep.asm u -o=/dd/Blue/CMDS/selfgen_preprocess
attr /dd/Blue/CMDS/selfgen_preprocess e pe

chd /dd
minigolf_par3 /dd/SRC/par3asm.par3 /dd/Blue/par3asm.p3a
par3asm /dd/Blue/par3asm.p3a -o /dd/Blue/par3asm.p3p
par3pack /dd/Blue/par3asm.p3p par3asm /dd/Blue
chd /dd/Blue
asm #48k template.prep.asm u -o=/dd/Blue/CMDS/par3asm
attr /dd/Blue/CMDS/par3asm e pe

chd /dd
minigolf_par3 /dd/SRC/minigolf_par3.par3 /dd/Blue/minigolf_par3.p3a
par3asm /dd/Blue/minigolf_par3.p3a -o /dd/Blue/minigolf_par3.p3p
par3pack /dd/Blue/minigolf_par3.p3p minigolf_par3 /dd/Blue
chd /dd/Blue
asm #48k template.prep.asm u -o=/dd/Blue/CMDS/minigolf_par3
attr /dd/Blue/CMDS/minigolf_par3 e pe

* ====================================================================
* VERIFICATION (Green vs Blue)
* ====================================================================
echo === VERIFICATION: Comparing Green vs Blue Boot Images ===
chx /dd/CMDS
chd /dd
cmp /dd/Green/deep65280.img /dd/Blue/deep65280.img

echo === VERIFICATION: Comparing Green vs Blue Preprocessed Template ===
cmp /dd/Green/template.prep.asm /dd/Blue/template.prep.asm

echo === VERIFICATION: Comparing Green vs Blue Command Binaries ===
cmp /dd/Green/CMDS/minigolf_par3 /dd/Blue/CMDS/minigolf_par3
cmp /dd/Green/CMDS/par3asm /dd/Blue/CMDS/par3asm
cmp /dd/Green/CMDS/par3pack /dd/Blue/CMDS/par3pack
cmp /dd/Green/CMDS/selfgen_preprocess /dd/Blue/CMDS/selfgen_preprocess
cmp /dd/Green/CMDS/build_img /dd/Blue/CMDS/build_img

echo === VERIFICATION: Comparing Green vs Blue Kernel Modules ===
cmp /dd/Green/MODS/krn /dd/Blue/MODS/krn
cmp /dd/Green/MODS/krnp2 /dd/Blue/MODS/krnp2
cmp /dd/Green/MODS/init /dd/Blue/MODS/init
cmp /dd/Green/MODS/ioman /dd/Blue/MODS/ioman
cmp /dd/Green/MODS/rbf /dd/Blue/MODS/rbf
cmp /dd/Green/MODS/scf /dd/Blue/MODS/scf
cmp /dd/Green/MODS/sc6850 /dd/Blue/MODS/sc6850
cmp /dd/Green/MODS/emudsk /dd/Blue/MODS/emudsk
cmp /dd/Green/MODS/clock /dd/Blue/MODS/clock
cmp /dd/Green/MODS/term /dd/Blue/MODS/term
cmp /dd/Green/MODS/dd /dd/Blue/MODS/dd
cmp /dd/Green/MODS/sysgo /dd/Blue/MODS/sysgo
cmp /dd/Green/MODS/shell_21 /dd/Blue/MODS/shell_21
cmp /dd/Green/MODS/mfree /dd/Blue/MODS/mfree
cmp /dd/Green/MODS/free /dd/Blue/MODS/free
cmp /dd/Green/MODS/procs /dd/Blue/MODS/procs
cmp /dd/Green/MODS/mdir /dd/Blue/MODS/mdir
cmp /dd/Green/MODS/dir /dd/Blue/MODS/dir
cmp /dd/Green/MODS/setime /dd/Blue/MODS/setime
cmp /dd/Green/MODS/date /dd/Blue/MODS/date
cmp /dd/Green/MODS/stop /dd/Blue/MODS/stop

echo ALL STAGES CONVERGED SUCCESSFULLY
stop 0
STARTUP_EOF

to_cr "$TMP_DIR/startup.raw" "$TMP_DIR/startup"
os9 copy -l -r "$TMP_DIR/startup" "$DISK,startup"

echo "=== Disk image generation complete: $DISK ==="
echo "=== Booting gep9 emulator to execute self-regeneration ==="
"$GEP9" -engine deep65280v2 -ram 512k -max-seconds 7200 -disk0 "$DISK" "$BOOT_IMG"
