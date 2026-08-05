---
name: musictheory
description: Analyze MIDI files and perform music theory operations.
license: Complete terms in LICENSE.txt
framework: "musictheory"
trigger: musictheory
---

# 🎵 Music Theory & MIDI Analysis Skill

High-performance music analysis, transformation, and structural auditing using the `musictheory` engine. This tool is optimized to be token-efficient by using a single flat entry point for all operations.

## 🛠️ Actions & Capabilities

| Action | Description | Key Parameters |
| :--- | :--- | :--- |
| `summarize_metadata` | Rapid metadata and track overview. | `path` |
| `analyze_theory` | Deep theory (Robotics/Groove scores). | `path`, `json: true` |
| `inspect_structure` | Audits MIDI health and protocol compliance. | `path` |
| `remix_transform` | Applies transformation chains (transposition). | `path`, `remix`, `out` |
| `validate_integrity` | Strict protocol validation. | `path` |
| `compare_pedals` | Compares sustain pedal usage patterns. | `path`, `compare` |
| `diff_notes` | Note-by-note comparison (Grep Hell killer). | `path`, `compare` |
| `generate_lilypond` | Transcribes MIDI to LilyPond notation. | `path`, `out` |
| `lilypond_to_midi` | Compiles LilyPond source to MIDI. | `path`, `out` |
| `midi_batch_replace` | Target and replace specific MIDI events. | `path`, `out`, `options` |

## 🚦 Best Practice Usage (Zero-Error Protocol)

To ensure smooth operations and avoid tool-call failures, follow these mandates:

### 1. Workspace Synchronization
**CRITICAL:** Always run `workspace` to align with the project root before calling any music tool.

### 2. The JSON Restriction
Only the `analyze_theory` action supports the `json: true` parameter.
- **DO NOT** use `json: true` with `inspect_structure` or `diff_notes`; it will result in a "flag provided but not defined" error from the underlying engine.

### 3. Smart Flag Mapping
The tool maps flat parameters directly to CLI flags. 
- Use boolean `true` for flags without values: `{"verbose": true}` -> `-verbose`.
- Use strings for flags with values: `{"transpose": "5"}` -> `-transpose "5"`.
- **Validation:** Double-check that the flag exists for the specific action before calling.

### 4. Direct Parameter Pathing
Always provide absolute paths for the `path` and `compare` parameters. This bypasses potential resolution issues during the path-safety validation phase.

### 5. Managing Large Outputs
Actions like `diff_notes` often generate output exceeding 50KB.
- **Strategic Filtering:** Use `start` and `end` parameters (representing seconds) to target only problematic ranges (e.g., `start: 0, end: 3`) to quickly diagnose issues at the beginning of a file.
- **File Fallback:** If the output is still too large, the tool will automatically save it to an `.output.txt` file. Use `view_file` to inspect results without re-running the diagnostic.

## 🚨 Common Error Resolution
- **"DANGEROUS ROOT DETECTED"**: Your workspace root is too broad or not set. Use `workspace` to point to a specific project directory.
- **"flag provided but not defined"**: You passed a parameter (like `json` or `verbose`) to an action that doesn't support it. Refer to the table above.
- **"Command failed"**: Verify the input MIDI file actually exists at the provided path.
