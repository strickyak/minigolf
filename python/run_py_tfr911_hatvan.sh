#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCEd[0]:-${BASH_SOURCE[0]}}")" >/dev/null 2>&1 && pwd)"
BUILD_DIR="$DIR/build"
DECB="$BUILD_DIR/pyeval-hv.decb"

# 1. Locate Hatvan OS repo
HATVAN_DIR="${HATVAN_DIR:-}"
if [ -z "$HATVAN_DIR" ]; then
    for cand in "$DIR/../../hatvan-os" "$DIR/../hatvan-os"; do
        if [ -d "$cand" ]; then
            HATVAN_DIR="$(cd "$cand" && pwd)"
            break
        fi
    done
fi

if [ -z "$HATVAN_DIR" ] || [ ! -d "$HATVAN_DIR" ]; then
    echo "Error: hatvan-os repository directory not found" >&2
    exit 1
fi

# 2. Ensure binary is built
if [ ! -f "$DECB" ]; then
    echo "Building pyeval-hv in $BUILD_DIR..." >&2
    make -C "$DIR" "$DECB"
fi

# 3. Locate os9 tool
OS9="${OS9:-os9}"
if ! which "$OS9" >/dev/null 2>&1; then
    echo "Error: os9 tool not found in PATH" >&2
    exit 1
fi

# 4. Locate disk image and kernel binaries
DISK="${DISK:-$HATVAN_DIR/build/disk0.dsk}"
RBF="$HATVAN_DIR/build/rbf_6809.decb"
PROCFS="$HATVAN_DIR/build/procfs_6809.decb"
KERNEL="$HATVAN_DIR/build/kernel_6809.decb"

if [ ! -f "$DISK" ] || [ ! -f "$RBF" ] || [ ! -f "$PROCFS" ] || [ ! -f "$KERNEL" ]; then
    echo "Building missing Hatvan OS targets in $HATVAN_DIR..." >&2
    make -C "$HATVAN_DIR" kernels disk
fi

# 5. Install pyeval-hv.decb into /dd/cmds9/pyeval (Cmds9/pyeval on disk0)
echo "Installing pyeval to $DISK,Cmds9/pyeval..." >&2
"$OS9" copy -r "$DECB" "$DISK,Cmds9/pyeval"
"$OS9" attr -r -w -e -pr -pe "$DISK,Cmds9/pyeval"

# 6. Locate tether binary
TETHER="${TETHER:-}"
if [ -z "$TETHER" ]; then
    for cand in \
        "/home/strick/modoc/coco-shelf/tfr9/v4/build/tether.linux-amd64.exe" \
        "$DIR/../../modoc/coco-shelf/tfr9/v4/build/tether.linux-amd64.exe" \
        "tether"; do
        if [ -x "$cand" ]; then
            TETHER="$cand"
            break
        elif which "$cand" >/dev/null 2>&1; then
            TETHER="$(which "$cand")"
            break
        fi
    done
fi

if [ -z "$TETHER" ] || [ ! -x "$TETHER" ]; then
    echo "Error: tether binary not found" >&2
    exit 1
fi

# 7. Locate serial wire
WIRE="${WIRE:-}"
if [ -z "$WIRE" ]; then
    ACM_PORTS=( /dev/ttyACM* )
    if [ -e "${ACM_PORTS[0]}" ]; then
        WIRE="${ACM_PORTS[-1]}"
    else
        WIRE="/dev/ttyACM0"
    fi
fi

# 8. Prepare virtual /pc directory for tether
PC_DIR="$BUILD_DIR/pc_hatvan"
mkdir -p "$PC_DIR"
ln -sf "$KERNEL" "$PC_DIR/kernel_6809.decb"
ln -sf "$RBF" "$PC_DIR/rbf_6809.decb"
ln -sf "$PROCFS" "$PC_DIR/procfs_6809.decb"
ln -sf "$DISK" "$PC_DIR/disk0.dsk"

cat << 'EOF' > "$PC_DIR/mode81.tcl"
set Config(engine) "gep9big"
set Config(flat_img) "/pc/kernel_6809.decb"
set Config(task1) "/pc/rbf_6809.decb"
set Config(task2) "/pc/procfs_6809.decb"
set Config(drive0) "/pc/disk0.dsk"
set Config(drive1) ""
set Config(drive2) ""
set Config(drive3) ""
menu store Config
bye
EOF

echo "Launching Hatvan OS on TFR911H via tether (wire=$WIRE)..." >&2

# 9. Launch on TFR911H
if [ -t 0 ]; then
    # Interactive terminal: attach console directly to tether
    exec "$TETHER" -wire "$WIRE" -pc "$PC_DIR" -bootmode=81 -cooked "$@"
else
    # Piped/scripted input: synchronize with Phase 2 so early input isn't consumed by Phase 1 Tcl
    exec python3 -c "
import subprocess, sys, threading

cmd = ['$TETHER', '-wire', '$WIRE', '-pc', '$PC_DIR', '-bootmode=81', '-cooked'] + sys.argv[1:]
proc = subprocess.Popen(
    cmd,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.STDOUT,
    text=True,
    bufsize=1
)

def feed():
    for line in iter(proc.stdout.readline, ''):
        sys.stdout.write(line)
        sys.stdout.flush()
        if 'Phase 2 started' in line:
            try:
                data = sys.stdin.read()
                proc.stdin.write(data)
                proc.stdin.flush()
            except (BrokenPipeError, OSError):
                pass
            break
    for line in iter(proc.stdout.readline, ''):
        sys.stdout.write(line)
        sys.stdout.flush()

t = threading.Thread(target=feed)
t.start()
proc.wait()
t.join()
sys.exit(proc.returncode)
" "$@"
fi
