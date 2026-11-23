---
name: godot-re
description: "Godot game reverse engineering with the portable `re` environment (rea-agents MCP + GDRE Tools + pinned Godot editor). Use for any Godot .pck/APK/desktop build: inventory, decompile GDScript, recover a buildable project, patch .gdc back into a pack, and deciding whether native libraries must be reversed at all."
framework: "zen-mcp"
trigger: godot reverse engineer pck gdre decompile gdscript godot modding rea-agents
---

# Godot Reverse Engineering (portable `re` env)

Everything below is **measured**, not assumed. The environment is a single wipe-proof tree:

```
RE_HOME = /media/jang/home/PortableApp/re
  bin/{rea,gdre_tools,godot}   pinned node v24.22.3 · rea-agents 6.9.0 · GDRE 2.7.4 · Godot 4.7.2
  install.sh  activate.sh  verify.sh  PROJECT_OVERVIEW.md
  profile/                     the fake $HOME/XDG/TMP every tool writes into
```

## 0. Boot (30 s, idempotent)

```sh
. /media/jang/home/PortableApp/re/activate.sh          # PATH += re/bin, RE_HOME, RE_WORK
/media/jang/home/PortableApp/re/verify.sh --fast       # 8 checks, exits non-zero on failure
# or rebuild everything from scratch on a new machine:
env_re            # shell function: preflight -> install --tier=1 -> --register -> activate -> verify --fast
env_re 2          # adds Ghidra + JDK 21 (only if native code is really on the critical path)
```

`verify.sh` (full) also drives a real MCP handshake (`rea: connected, N tools`) and `pi mcp list`.
If a tool is missing from a login shell, the wiring lives in
`/media/jang/home/Deve/zen-bashrc/src/01_env_suite.sh` (`env_active()` + `env_re`); regenerate with
`cd .../zen-bashrc && ./stitch.sh && . ./bashrc && env_register -n && env_register`.

**Wrapper rules.** `bin/gdre_tools` injects `--headless` — never pass it twice. `bin/godot` is a
plain pass-through. Each wrapper sets `HOME=$RE_HOME/profile/home` + `XDG_*` + `TMPDIR`, so tool
state never touches the real `$HOME`; the tier-2 exports (`JAVA_HOME`, `GHIDRA_INSTALL_DIR`) are
`[ -x ]`-guarded no-ops until tier 2 is installed.

## 1. New target: work tree + triage (read-only)

```
<project>/work/
 10_evidence   rea inspect-artifact bundles        20_extracted  raw --extract (usually redundant)
 25_scripts    gdre --scripts-only                 30_recovered  full project (git baseline lives in 40_mod)
 40_mod        git repo = the only place you edit  50_roundtrip  write-path sandbox
 logs          pck-files.txt, verify.baseline.json, per-run tool logs
```

Triage before decompiling — 5 cheap probes answer most of the plan:

| Probe | Command | What it decides |
| --- | --- | --- |
| pack header | `od -A d -t x1 -N 40 <pck>` | `GDPC` magic, format int @4 (2/3/4), engine 8/12/16, `pack_flags` @20 |
| encrypted? | `pack_flags & 1`, or REA `"encrypted":false` per member | need `--key=<64-hex>` or not |
| embedded? | REA root `"embedded":true/false` | script-pack vs packed exe (`.tpz`/APK differ) |
| engine version | `strings <exe> \| grep -m1 'Godot Engine'` | which Godot editor to use — must match, or imports break |
| bytecode rev | first `.gdc`: `od -N 8` → `GDSC` + LE int32 | which `--bytecode=` token compiles a loadable `.gdc` |
| native surface | `readelf -d <exe>` (NEEDED) · `nm -D --defined-only <lib.so> \| grep ' T '` | is there real logic in native code, or just SDK glue (§5) |

Then `rea inspect-artifact <pck> --detail summary --evidence_output …/10_evidence/pck-$(LC_ALL=C date -u +%Y%m%dT%H%M%S).json --json`.

## 2. Recovery pipeline (verified: 14,383 members, 131 scripts, 0 failures)

```sh
gdre_tools --list-files=<PCK> > work/logs/pck-files.txt          # "Total files: N"
gdre_tools --recover=<PCK> --scripts-only --output=work/25_scripts   # ~2 s, .gd only
gdre_tools --recover=<PCK> --output=work/30_recovered                # ~84 s, ~3.9x the PCK size
cp -a work/30_recovered/. work/40_mod/ && cd work/40_mod && git init -qb main   # baseline before editing
```

Read the verdict from **`<output>/gdre_export.log`** (not from `~/.local/share`):
`Decompiled scripts: N | Scripts not decompiled: 0 | Failed conversions: 0`.
Assert the decompiled count **equals** `grep -c '\.gdc$' pck-files.txt` — `≥1` is not a gate.
`.md5`/`.ctex` under `.godot/imported/` are the editor cache: gitignore them, they regenerate.

## 3. Write-back (the part no doc states correctly)

```sh
# .gd -> .gdc (bytecode token, not the int!)
gdre_tools --compile=src/game_data.gd --bytecode=4.5.0-stable --output=build   # --output is a DIR
# check it
od -A d -t x1 -N 8 build/game_data.gdc                       # GDSC + LE int32 revision
gdre_tools --decompile=build/game_data.gdc --bytecode=4.5.0-stable --output=verify/decompiled
diff src/game_data.gd verify/decompiled/game_data.gd         # GDRE's own fixpoint: must be empty
# patch: reads the read-only original, writes a NEW pack — no 557 MB copy needed
gdre_tools --pck-patch=<PCK> \
  --patch-file="$PWD/build/game_data.gdc=res://scripts/autoload/game_data.gdc" \
  --output=work/50_roundtrip/pck/patched/MidgardSurvivors.pck
# prove it landed
gdre_tools --list-files=work/50_roundtrip/pck/patched/MidgardSurvivors.pck | tail -1
gdre_tools --extract=<patched> --include='res://scripts/autoload/game_data.gdc' --output=verify/member
sha256sum build/game_data.gdc verify/member/scripts/autoload/game_data.gdc     # must match
```

Hard-won gotchas, all reproduced on 2.7.4:

- `--decompile` **requires** `--bytecode=…`; without it: `exit 255` + a help dump.
- `--output` = **directory** for `--compile/--decompile/--extract`, **file** for `--pck-patch/--pck-create`.
- `--patch-file=SRC=DEST` wants the `res://` prefix even when the archive stores relative names.
- **`--pck-version` only produces formats 0/1/2** → `--pck-create` CANNOT build a format-4 pack.
  `--pck-patch` is therefore the only verified write path into a Godot 4 pack, and it preserves
  format 4 + `pack_flags` itself (index rewritten, member count intact).
- Consequence: **project-setting edits (the `project.binary`/`ECFG` blob) have no GDRE route.**
  `--setting=` writes GDRE's own cfg, not the pack. Use the Godot editor
  (`godot --headless --export-pack`, needs an `export_presets.cfg` — absent in exported builds)
  or keep edits inside `.gd`/`.tscn`/`.json` members.
- `.gdc` bodies are serializer-obfuscated: no plaintext constants, so verify by hash/decompile, never `grep`.
- `gdre_tools --gdre-version` prints fine but **exits 1** by upstream design; gate on `--help`'s first line.

## 4. REA (inventory/evidence/MCP lane — read-only)

- `rea inspect-artifact` / `compare-artifacts` / `extract-artifact` / `search` / `decompile`. It has
  **no** pack/write/patch command — GDRE owns writes.
- `extract-artifact` takes `--occurrence-id` (IDs from the evidence bundle, content-derived, so they
  change when the pack changes) + `--output-root` which must be absolute and **not exist**.
- `--evidence_output` never overwrites and refuses only *after* the ~10 s analysis; each bundle is
  ~45 MB. Always timestamp the name; `LC_ALL=C date -u` (else Buddhist-era years on `th_TH` locales).
- REA cannot open a bare `.gdc` (`unsupported_target`) — extract from the pack, or use GDRE.
- For single members prefer `gdre_tools --extract --include=` over REA (no 45 MB side effect).
- As an MCP server: `pi mcp list` → `rea: connected, 255 tools`. A piped `printf … | rea mcp` loses
  `tools/list` to a race — drive `initialize → notifications/initialized → tools/list` with stdin held
  open (see `re/verify.sh handshake()`).

## 5. Do I need to reverse the native libraries?

Usually **no**. Prove it before downloading 1.6 GB of Ghidra:

1. Is the exe a **stock export template**? `readelf -S` shows only standard sections, `nm -D` exports
   nothing, and `strings` matches the Godot version banner → the binary contains no game logic.
2. Does it verify anything? grep the binary for `pack_error|Corrupt PCK|signature_hash|MD5 mismatch`
   and the scripts for `sha256|verify|integrity|anticheat|tamper`. Absent → the pack is loaded, not checked.
3. Are the `.so` files SDK glue? One `T` entry point (`*_library_init`) + Epic/Steam symbol names =
   a GDExtension wrapper; then check the *guard* in GDScript
   (`ClassDB.class_exists("<ExtPeer>")`) — a guarded extension is optional by design.
4. Is the data server-side? grep scripts for `https?://`/`HTTPRequest` and read what each endpoint is
   (auth/telemetry/manifest vs live config). Balance living in `.gd` const tables ⇒ fully client-side.

If all four come back negative: mod scripts/scenes/resources and repack — **no disassembler**.
Native RE is only on the critical path to fake the SDK's surface, bypass a genuine anti-cheat, or
recover logic that was shipped in C# / a custom native extension.

## 6. Prohibited (each one silently breaks the env)

| Never | Because |
| --- | --- |
| `rea setup` / `--client <x>` without `--dry-run` | writes `~/.claude.json`, `~/.cursor/mcp.json`… and replaces `re/bin/rea` with a bare `node rea.mjs`, dropping every HOME/XDG redirect |
| `pi mcp add` | turns the `~/.pi/agent/mcp.json` symlink into a regular file on the OS partition |
| `rea update` | mutates the digest-pinned tree; upgrade = edit the `install.sh` registry pin + `install.sh --force --only=<name>` |
| bare `npm install` / `npm config …` | `NPM_CONFIG_PREFIX` points at the shared `PortableApp/node` tree; use `install.sh --only=rea` |
| writing into the shipped game dir | it is the only unmodified copy; `verify.sh check_target_integrity` compares its sha256 |
| running the *only* copy of the game while patching | an in-game updater can overwrite your work — in Midgard, any extra `--` arg disables updater+telemetry (`updater.gd:132-152`) |
| testing a rebuilt pack with `--main-pack`/`--path` | official templates are built `disable_path_overrides=yes` and abort; name-match a copy instead (`Foo.x86_64` + `Foo.pck`) |

## 7. Handoff

Per-project: `AGENTS.md` (state + next step) and `work/README.md` (dir map). Per-env:
`$RE_HOME/PROJECT_OVERVIEW.md`. Plan with as-built deltas:
`/media/jang/free/MidgardSurvivors/zen-rea/ARCHITECTURE_PLAN_portable-rea-re-env.md` §11.
Machine-readable ground truth: `work/logs/verify.baseline.json` (versions, tool count, member count,
`.gdc` count, PCK sha256) — recapture with `verify.sh --capture` only after a deliberate change.
