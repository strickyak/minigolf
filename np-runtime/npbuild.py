#!/usr/bin/env python3
"""
npbuild.py - MiniGolf-NP Compiler & OS-9 Module Builder for NitrOS-9

Automates the complete pipeline:
  1. Compile MiniGolf source (.golf) -> NPCode assembly (.npasm) via minigolf -m=np
  2. Assemble NPCode assembly (.npasm) -> NPCode bytecode (.npc) via npasm.py
  3. Combine bytecode payload with M6809 OS-9 interpreter engine (template.asm)
  4. Assemble into standalone OS-9 binary module via lwasm
  5. Optionally copy into NitrOS-9 disk image (deep0.dsk), run on gep9 emulator,
     and verify output against npvm.py reference interpreter.

Usage:
  npbuild.py <input.golf|input.npasm|input.npc> [options]

Options:
  -o, --output <file>       Output OS-9 module path (default: ./<name>)
  -m, --module-name <name>  OS-9 module name (default: derived from input filename)
  --run                     Install to disk image and run on gep9 emulator
  --verify                  Run on both npvm.py and gep9, comparing output
  --disk <path>             NitrOS-9 disk image path
  --system-img <path>       NitrOS-9 boot system image path
  --engine <name>           gep9 engine: deep65280v2, flat65280v2, or hatvan
  --gep9 <path>             Path to gep9 emulator executable
  -v, --verbose             Print detailed execution logs
  -h, --help                Show this help message
"""

import sys
import os
import argparse
import subprocess
import tempfile
import shutil
import re

# Default paths
SCRIPT_DIR = os.path.dirname(os.path.realpath(__file__))
MINIGOLF_DIR = os.path.abspath(os.path.join(SCRIPT_DIR, ".."))
NITROS9_DIR = os.environ.get("NITROS9DIR", "/home/strick/modoc/coco-shelf/nitros9")
NPCODE_DIR = os.path.join(NITROS9_DIR, "npcode")
NITROS9_DEFS = os.path.join(NITROS9_DIR, "defs")
RECIPES_DIR = os.path.join(NITROS9_DIR, "recipes", "deep65280")
DEFAULT_DISK = os.path.join(RECIPES_DIR, "deep0.dsk")
DEFAULT_IMG = os.path.join(RECIPES_DIR, "deep65280.img")
DEFAULT_GEP9 = "/home/strick/github.com/strickyak/hatvan-os/build/gep9"
TEMPLATE_ASM = os.path.join(SCRIPT_DIR, "template.asm")


def log(msg: str, verbose: bool = False, is_verbose_msg: bool = False):
    if not is_verbose_msg or verbose:
        print(f"[*] {msg}", flush=True)


def error(msg: str):
    print(f"[ERROR] {msg}", file=sys.stderr, flush=True)
    sys.exit(1)


def run_cmd(cmd, verbose: bool = False, cwd: str = None, input_data: str = None) -> subprocess.CompletedProcess:
    if verbose:
        cmd_str = " ".join(cmd) if isinstance(cmd, list) else cmd
        log(f"Running: {cmd_str}", verbose, is_verbose_msg=True)
    result = subprocess.run(
        cmd,
        cwd=cwd,
        input=input_data,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
        shell=isinstance(cmd, str)
    )
    if result.returncode != 0:
        error(f"Command failed (code {result.returncode}):\n{result.stderr}\n{result.stdout}")
    return result


def extract_emulator_output(raw_output: str, cmd_name: str) -> str:
    """Extracts output produced by cmd_name from raw gep9 console output."""
    # Find start marker: OS9:cmd_name
    pattern = rf"OS9:\s*{re.escape(cmd_name)}"
    match = re.search(pattern, raw_output, re.IGNORECASE)
    if not match:
        return raw_output.strip()

    after_cmd = raw_output[match.end():]
    # Find end marker: next OS9: prompt or exit message
    end_pattern = r"(OS9:|\*\*\* Exiting)"
    end_match = re.search(end_pattern, after_cmd)
    if end_match:
        program_output = after_cmd[:end_match.start()]
    else:
        program_output = after_cmd

    # Clean up CR/LF and empty lines
    lines = [line.strip() for line in program_output.splitlines() if line.strip()]
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(
        description="MiniGolf-NP Compiler & OS-9 Module Builder for NitrOS-9"
    )
    parser.add_argument("input", help="Source file (.golf, .npasm, or .npc)")
    parser.add_argument("-o", "--output", help="Output OS-9 module path")
    parser.add_argument("-m", "--module-name", help="OS-9 module name (8 chars max recommended)")
    parser.add_argument("--run", action="store_true", help="Install to disk image and run on gep9")
    parser.add_argument("--verify", action="store_true", help="Run on both npvm.py and gep9, verifying output")
    parser.add_argument("--disk", default=DEFAULT_DISK, help="NitrOS-9 disk image path")
    parser.add_argument("--system-img", default=DEFAULT_IMG, help="NitrOS-9 system boot image path")
    parser.add_argument("--engine", default="deep65280v2", help="gep9 engine")
    parser.add_argument("--gep9", default=DEFAULT_GEP9, help="Path to gep9 executable")
    parser.add_argument("-I", "--include", action="append", default=[], help="Directory to search for imports")
    parser.add_argument("-v", "--verbose", action="store_true", help="Verbose logging")

    args = parser.parse_args()

    input_path = os.path.abspath(args.input)
    if not os.path.exists(input_path):
        error(f"Input file not found: {input_path}")

    base_name = os.path.splitext(os.path.basename(input_path))[0]
    ext = os.path.splitext(input_path)[1].lower()

    module_name = args.module_name or base_name
    # Keep module name alphanumeric + underscore, max 28 chars
    module_name = re.sub(r"[^A-Za-z0-9_]", "_", module_name)[:28]

    output_path = os.path.abspath(args.output or os.path.join(os.getcwd(), base_name))

    build_dir = tempfile.mkdtemp(prefix="npbuild_")
    try:
        cur_file = input_path

        # Step 1: .golf -> .npasm
        if ext == ".golf":
            log(f"Compiling MiniGolf source '{cur_file}' -> NPCode assembly...", args.verbose)
            npasm_path = os.path.join(build_dir, f"{base_name}.npasm")
            minigolf_bin = os.path.join(MINIGOLF_DIR, "minigolf")
            if os.path.exists(minigolf_bin) and os.access(minigolf_bin, os.X_OK):
                cmd = [minigolf_bin, "-m=np"]
            else:
                cmd = ["go", "run", "main.go", "-m=np"]
            for inc in args.include:
                cmd.extend(["-I", inc])
            cmd.extend(["-o", npasm_path, cur_file])
            run_cmd(cmd, verbose=args.verbose, cwd=MINIGOLF_DIR)
            cur_file = npasm_path
            ext = ".npasm"

        # Step 2: .npasm -> .npc
        if ext == ".npasm":
            log(f"Assembling NPCode assembly '{cur_file}' -> bytecode...", args.verbose)
            npc_path = os.path.join(build_dir, f"{base_name}.npc")
            npasm_py = os.path.join(NPCODE_DIR, "npasm.py")
            if not os.path.exists(npasm_py):
                error(f"npasm.py not found at: {npasm_py}")
            cmd = [sys.executable, npasm_py, cur_file, "-o", npc_path]
            run_cmd(cmd, verbose=args.verbose)
            cur_file = npc_path
            ext = ".npc"

        if ext != ".npc":
            error(f"Unsupported file format: {ext}")

        npc_file = cur_file
        with open(npc_file, "rb") as f:
            npc_bytes = f.read()
        log(f"Loaded NPC bytecode ({len(npc_bytes)} bytes).", args.verbose)

        # Run reference execution on npvm.py if needed
        ref_output = None
        if args.verify or args.run:
            npvm_py = os.path.join(NPCODE_DIR, "npvm.py")
            if os.path.exists(npvm_py):
                log("Running reference execution on npvm.py...", args.verbose)
                res = run_cmd([sys.executable, npvm_py, npc_file], verbose=args.verbose)
                ref_output = res.stdout.strip()
                if args.verbose:
                    log(f"npvm.py output:\n{ref_output}", args.verbose, is_verbose_msg=True)

        # Step 3: Generate payload.asm and modname.asm in build_dir
        payload_asm = os.path.join(build_dir, "payload.asm")
        modname_asm = os.path.join(build_dir, "modname.asm")

        with open(modname_asm, "w") as f:
            f.write(f"        fcs     /{module_name}/\n")

        lines = []
        for i in range(0, len(npc_bytes), 16):
            chunk = npc_bytes[i:i + 16]
            lines.append("        fcb     " + ",".join(f"${b:02X}" for b in chunk))
        with open(payload_asm, "w") as f:
            f.write("\n".join(lines) + "\n")

        # Step 4: Assemble OS-9 module with lwasm
        log(f"Assembling OS-9 module '{module_name}' with lwasm...", args.verbose)
        lwasm_bin = shutil.which("lwasm.orig") or shutil.which("lwasm")
        if not lwasm_bin:
            error("lwasm / lwasm.orig not found on PATH!")

        lwasm_cmd = [
            lwasm_bin,
            "--no-warn=ifp1",
            "--6809",
            "--format=os9",
            "--pragma=pcaspcr,nosymbolcase,condundefzero,undefextern,dollarnotlocal,noforwardrefmax",
            f"--includedir={build_dir}",
            f"--includedir={SCRIPT_DIR}",
            f"--includedir={NITROS9_DEFS}",
            "--list=/tmp/test_for3.list", "-o", output_path,
            TEMPLATE_ASM
        ]
        run_cmd(lwasm_cmd, verbose=args.verbose)
        module_sz = os.path.getsize(output_path)
        log(f"Built OS-9 module '{output_path}' ({module_sz} bytes).")

        # Step 5: Install and Run on gep9 if requested
        if args.run or args.verify:
            if not os.path.exists(args.disk):
                error(f"Disk image not found: {args.disk}")
            if not os.path.exists(args.system_img):
                error(f"System image not found: {args.system_img}")
            if not os.path.exists(args.gep9):
                error(f"gep9 emulator not found: {args.gep9}")

            os9_bin = shutil.which("os9") or "/home/strick/modoc/coco-shelf/bin/os9"
            if not os.path.exists(os9_bin):
                error("os9 tool not found!")

            target_os9 = f"{args.disk},cmds/{module_name}"
            log(f"Installing module into '{target_os9}'...", args.verbose)
            run_cmd([os9_bin, "copy", "-r", output_path, target_os9], verbose=args.verbose)
            run_cmd([os9_bin, "attr", "-e", "-pe", "-pr", target_os9], verbose=args.verbose)

            log(f"Running '{module_name}' on gep9 ({args.engine})...", args.verbose)
            gep9_cmd = [
                args.gep9,
                f"--engine={args.engine}",
                "-disk0", args.disk,
                args.system_img
            ]
            sim_input = f"{module_name}\rstop 0\r"
            gep9_res = run_cmd(gep9_cmd, verbose=args.verbose, input_data=sim_input)

            clean_out = extract_emulator_output(gep9_res.stdout, module_name)

            print("=" * 60)
            print(f"NitrOS-9 Output ({module_name}):")
            print("=" * 60)
            if clean_out:
                print(clean_out)
            else:
                print("(no output)")
            print("=" * 60)

            if args.verify:
                log("Verifying output equivalence with npvm.py...", args.verbose)
                if ref_output is None:
                    error("Reference output unavailable for verification.")

                # Compare line-by-line
                emu_lines = [l.strip() for l in clean_out.splitlines() if l.strip()]
                ref_lines = [l.strip() for l in ref_output.splitlines() if l.strip()]

                if emu_lines == ref_lines:
                    print(f"\n[SUCCESS] Output matches npvm.py reference ({len(emu_lines)} lines verified)!\n")
                else:
                    print(f"\n[FAILURE] Output mismatch between npvm.py and gep9!\n")
                    print(f"npvm.py lines ({len(ref_lines)}):\n{ref_lines}\n")
                    print(f"gep9 lines ({len(emu_lines)}):\n{emu_lines}\n")
                    sys.exit(1)

    finally:
        shutil.rmtree(build_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
