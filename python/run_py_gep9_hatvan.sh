#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BUILD_DIR="$DIR/build"
DECB="$BUILD_DIR/pyeval-hv.decb"

# Ensure Hatvan OS repo is located
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

# Ensure binary is built
if [ ! -f "$DECB" ]; then
    echo "Building pyeval-hv in $BUILD_DIR..." >&2
    make -C "$DIR" "$DECB"
fi

# Locate os9 tool
OS9="${OS9:-os9}"
if ! which "$OS9" >/dev/null 2>&1; then
    echo "Error: os9 tool not found in PATH" >&2
    exit 1
fi

# Locate disk image and kernel binaries
DISK="${DISK:-$HATVAN_DIR/build/disk0.dsk}"
RBF="$HATVAN_DIR/build/rbf_6809.decb"
PROCFS="$HATVAN_DIR/build/procfs_6809.decb"
KERNEL="$HATVAN_DIR/build/kernel_6809.decb"

if [ ! -f "$DISK" ] || [ ! -f "$RBF" ] || [ ! -f "$PROCFS" ] || [ ! -f "$KERNEL" ]; then
    echo "Building missing Hatvan OS targets in $HATVAN_DIR..." >&2
    make -C "$HATVAN_DIR" kernels disk
fi

# Install pyeval-hv.decb into /dd/cmds9/pyeval (Cmds9/pyeval on disk0)
echo "Installing pyeval to $DISK,Cmds9/pyeval..." >&2
"$OS9" copy -r "$DECB" "$DISK,Cmds9/pyeval"
"$OS9" attr -r -w -e -pr -pe "$DISK,Cmds9/pyeval"

# Locate gep9 emulator
GEP9="${GEP9:-}"
if [ -z "$GEP9" ]; then
    for cand in "$HATVAN_DIR/build/gep9" "gep9"; do
        if [ -x "$cand" ]; then
            GEP9="$cand"
            break
        elif which "$cand" >/dev/null 2>&1; then
            GEP9="$(which "$cand")"
            break
        fi
    done
fi

if [ -z "$GEP9" ] || [ ! -x "$GEP9" ]; then
    echo "Error: gep9 emulator not found" >&2
    exit 1
fi

echo "Launching Hatvan OS on gep9 (big6809 mode)..." >&2
exec "$GEP9" -embiggen -disk0="$DISK" -task1="$RBF" -task2="$PROCFS" "$@" "$KERNEL"
