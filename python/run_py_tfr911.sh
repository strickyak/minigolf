#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BUILD_DIR="$DIR/build"
DECB="$BUILD_DIR/pyeval.decb"
TCL="$BUILD_DIR/mode81.tcl"

# 1. Ensure binary and mode81.tcl are built
if [ ! -f "$DECB" ] || [ ! -f "$TCL" ]; then
    echo "Building pyeval in $BUILD_DIR..." >&2
    make -C "$DIR"
fi

# 2. Locate tether binary
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

# 3. Locate serial wire
WIRE="${WIRE:-}"
if [ -z "$WIRE" ]; then
    ACM_PORTS=( /dev/ttyACM* )
    if [ -e "${ACM_PORTS[0]}" ]; then
        WIRE="${ACM_PORTS[-1]}"
    else
        WIRE="/dev/ttyACM0"
    fi
fi

# 4. Launch on TFR911H
if [ -t 0 ]; then
    # Interactive terminal: attach console directly to tether
    exec "$TETHER" -wire "$WIRE" -pc "$BUILD_DIR" -bootmode=81 -cooked "$@"
else
    # Piped/scripted input: synchronize with Phase 2 so early input isn't consumed by Phase 1 Tcl
    exec python3 -c "
import subprocess, sys, threading

cmd = ['$TETHER', '-wire', '$WIRE', '-pc', '$BUILD_DIR', '-bootmode=81', '-cooked'] + sys.argv[1:]
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
