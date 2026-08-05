---
description: Reconstructs Native Instruments Kontakt instruments (.nki) from samples (.wav) and mappings (.nkr/.nks). Supports both high-fidelity NKI parsing and heuristic filename-based reconstruction.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Kontakt Instrument Reconstruction

# 🧠 Kontakt Reconstruction Skill (Zen-Contact)

High-speed protocol for translating proprietary Kontakt libraries into high-fidelity SFZ suites.

## 🛠️ The Toolkit

| Format | Command | Purpose |
| :--- | :--- | :--- |
| **Analyze Samples (Dir)** | `zen-contact analyze-samples -i <in> -o <out>` | **Analyze** sample directory tree. |
| **Reconstruct Samples (Dir)** | `zen-contact reconstruct -p <yaml>` | **Heuristic**: Reconstruct via filename patterns. |

`zen-contact` is available globally (PATH). Compile with `go build .`.

---

## 🏃 Workflow: Zero-to-Fidelity

### **PRE-FLIGHT CHECKLIST — Execute in order. Do not skip steps.**

1. **Run `analyze-samples` FIRST** — **HALT until complete**..
2. **Identify instrument type** (Plucked String, Woodwind, Vocal, Percussion, etc.) → determines Best Practices section to follow.
3. **Study a similar completed profile** in `reconstruct_profiles/` before authoring.
4. **Verify output path is NOT inside the input path** — always set output outside the input directory tree.

From the output:
- **Note/Pitch schema** → select `key_regex` (note names like `C#3`) or `midi_regex` (raw numbers).
  - **[IMPORTANT] Negative Octaves**: Some libraries use `A-1` or `C0` for the lowest piano keys. Ensure your `key_regex` includes `-?` before the digit (e.g., `_([A-G]#?-?\d+)_`) and adjust `octave_offset` (usually `+2`) to align `A-1` to MIDI 21.
- **Round Robin** → identify explicit RR sequences (`_rr1`, `_rr2`, suffix indices) → configure `rr_regex`.
- **Velocity layers** → identify range indicators (`_1_25`, `_26_50`) → configure `vel_regex`.
- **Mic channels** → plan `mic_positions` or use `exclusions` to drop unwanted mono channels.

Run reconstruct command with the created profile:
```bash
zen-contact reconstruct -p "reconstruct_profiles/my_profile.yaml"
```

> [!TIP]
> **Diagnostics**: The engine now prints a `[WARNING]` if a sample fails note detection. If you see this for non-pedal samples, your `key_regex` or `midi_regex` is likely too restrictive (missing negative signs, multi-digits, or specific prefixes).

---

## 🏗️ Root Level: `ReconstructionProfile`

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | Display name of the library. |
| `input` | `string` | Path to a single input samples directory or NKI file. (Legacy) |
| `inputs` | `list` | List of input sample directories to aggregate into a single pass. Preferred for surgical fusion. |
| `output` | `string` | Path to the output SFZ directory. |
| `flat_walk` | `bool` | If `true`, performs a recursive directory walk inside every input folder. Default: `false`. |
| `sample_tuner` | `string` | Tuner command (e.g., `DADGAD`). Primarily for Guitar/Bass. |
| `octave_offset` | `int` | Global octave shift applied to all detected MIDI notes. |
| `key_regex` | `string` | Regex to extract note names (`C#3`, `Bb2`). Uses first capture group. |
| `midi_regex` | `string` | Regex to extract raw MIDI numbers. Uses first capture group. |
| `vel_regex` | `string` | Regex to extract velocity layer values from filenames. |
| `vel_anchor_regex` | `string` | Secondary regex to anchor velocity detection. |
| `rr_regex` | `string` | Regex to extract Round Robin indices. |
| `group_regex` | `string` | Regex to determine the Articulation/Group name from the filename. |
| `global_opcodes` | `map` | SFZ opcodes applied to every `<group>` in generated files. |
| `merge_articulations` | `bool` | If `true`, merges all articulations into one SFZ file. Default: `false`. **[IMPORTANT] ONLY USE FOR PIANO, KEEP IT FALSE (DEFAULT) AT ALL COST FOR OTHER INSTRUMENTS TO PRESERVE CC KEY SWITCHES**. |
| `enable_gap_filling` | `bool` | Stretch samples to cover empty keys. Default: `false`. |
| `spread_direction` | `string` | Target direction for stretching gaps: `"both"` (default), `"up"`, or `"down"`. |
| `max_stretch_up` | `int` | Maximum semitones a sample can be stretched upward. Default: `0` (unlimited). |
| `max_stretch_down` | `int` | Maximum semitones a sample can be stretched downward. Default: `0` (unlimited). |
| `spread_bias` | `float` | Split bias ratio between adjacent notes. Default: `0.5` (midpoint). |
| `spread_compensation` | `map` | Volume and filter cutoff compensation for stretched notes. |
| `spread_blend` | `bool` | Enable overlapping crossfades/blending between adjacent stretched zones. Default: `false`. |
| `blend_width` | `int` | Overlap width in semitones when `spread_blend` is enabled. Default: `1`. |
| `prefer_rr_first` | `bool` | Extract Round Robin before MIDI notes (strips RR to avoid digit confusion). |

| `treat_subdirs_as_instruments` | `bool` | If `true`, treats each subdirectory as a standalone instrument. |
| `master_switch_mode` | `string` | Keyswitch logic: `"CC"` (default) or `"Note"`. |
| `master_switch_cc` | `int` | CC number for switching in CC mode. Default: `32`. |
| `base_switch_note` | `int` | Starting MIDI note for Note-mode keyswitches. Default: `12` (C0). |
| `amp_veltrack` | `float` | Global `amp_veltrack` (0–100). Default: `100.0`. |
| `cutoff_veltrack` | `int` | Global `cutoff_veltrack` (cents). Enables LPF. |
| `release_decay` | `float` | Global `rt_decay` scaling for release samples (dB/s). |
| `velocity_crossfade` | `int` | Global velocity crossfade width (0–127). |
| `sub_instruments` | `list` | Distinct sub-instruments multiplexed from a single directory. |
| `mic_positions` | `list` | Multi-mic configurations (see Mic Positions). |
| `sanitizers` | `map` | Configuration map containing `raw_name_cleanup`, `group_name_cleanup`, and `display_name_cleanup` pipelines. |
| `profiles` | `list` | Instrument sub-profiles (e.g., "Standard", "Muted"). |
| `fusion_rules` | `list` | Logic for merging articulations into performance patches. |
| `trigger_rules` | `list` | Map filenames to SFZ triggers (e.g., `release`). |
| `exclusions` | `list` | Substrings — any matching sample path is skipped. |
| `spectral_config` | `map` | Tune spectral spread randomization (see Spectral Spread Config). |
| `midi_remap` | `map` | Manual MIDI number → absolute MIDI note overrides. |
| `midi_auto_remap_start` | `int` | Remap all detected pitches contiguously from this MIDI note. |
| `remap_rules` | `list` | Smart Pitch Anchoring rules to dynamically align unpitched samples to a reference articulation's detected range. |

> [!IMPORTANT]
> **Default keyswitch mode is CC on CC 32.** Omit `master_switch_mode` and `master_switch_cc` from your YAML unless explicitly overriding. CC mode is always preferred over Note mode — it never occupies keyboard octaves, allows voice overlap, and release trails decay naturally.

---

## 🎼 Sanitizer Pipelines (`sanitizers`)

Three distinct pipelines can be configured inside the `sanitizers:` block, corresponding to different stages of reconstruction. All share the same action vocabulary.

| Pipeline Key | When it Runs | Mutates |
| :--- | :--- | :--- |
| `raw_name_cleanup` | Before all extraction | Working filename (original preserved for mic filters) |
| `group_name_cleanup` | After group detection | Group/articulation label |
| `display_name_cleanup` | After group detection, in label output | Final display label |

> [!IMPORTANT]
> **`raw_name_cleanup` mutates only the working filename.** The original raw filename is preserved and used for `mic_positions` filter matching and `exclusions` checks.

### Actions

| Action | Arguments | Description |
| :--- | :--- | :--- |
| `replace_all` | `find`, `replace` | Simple string replacement. |
| `regex_replace` | `find`, `replace` | Regex-based replacement. |
| `apply_mapping` | `mapping` (map) | Case-insensitive word replacement using a dictionary. |
| `regex_mapping` | `regex`, `replace` | Mass replacement using a regex. |
| `strip_prefix` | `find` | Removes a specific prefix. |
| `strip_suffix` | `find` | Removes a specific suffix. |
| `deduplicate` | — | Removes repeated words ("Violin Violin" → "Violin"). |
| `title_case` | — | Converts label to Title Case. |
| `strip_note_suffix` | — | Removes trailing note names ("Flute C3" → "Flute"). |
| `strip_digit_suffix` | — | Removes trailing numbers. |
| `unify_ornaments` | — | Maps any label containing "ornamentation" → "Ornamentations". |
| `fallback` | `replace`, `allowed` | Coerces any name **not in** `allowed` to `replace`. |

**Standard display name cleanup pattern:**
```yaml
sanitizers:
  raw_name_cleanup:
    - action: "regex_replace"
      find: '(?i)^(.*)_([a-g][#b]?\d).*'
      replace: 'Art_${1}_sep_${0}'

  display_name_cleanup:
    - action: "replace_all"
      find: "_"
      replace: " "
    - action: "deduplicate"
    - action: "title_case"
```

---

## 🎸 Instrument Profiles (`profiles`)

The engine automatically injects a `Standard` profile if `profiles:` is empty.

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | Profile name (e.g., "Sustain", "Staccato"). |
| `name_pattern` | `string` | Regex to match filenames belonging to this profile. |
| `path_pattern` | `string` | Regex to match folder paths belonging to this profile. |
| `volume` | `float` | Volume adjustment in dB. |
| `lpf` | `int` | Low-pass filter cutoff (Hz). |
| `vel_track` | `int` | `amp_veltrack` value (0–100). |
| `is_monophonic` | `bool` | Enables `polyphony=2` and `off_by=1` for solo instruments. |
| `polyphony` | `int` | Override for `polyphony` when `is_monophonic: true`. Default: `2`. |
| `has_release` | `bool` | Hint to search for release samples. |
| `merge_groups` | `bool` | If `true`, ignores folder names and merges all samples into one "Main" group. Use this to fix Master SFZ bloat (repeated `#include` lines). |
| `enable_gap_filling` | `bool` | Profile override for gap filling. |
| `spread_direction` | `string` | Profile override for spread direction. |
| `max_stretch_up` | `int` | Profile override for maximum stretch up. |
| `max_stretch_down` | `int` | Profile override for maximum stretch down. |
| `spread_bias` | `float` | Profile override for spread bias. |
| `spread_compensation` | `map` | Profile override for spread compensation. |
| `spread_blend` | `bool` | Profile override for spread blend. |
| `blend_width` | `int` | Profile override for blend width. |
| `prefer_rr_first` | `bool` | Profile override for RR-first detection. |

| `opcodes` | `map` | Profile-specific SFZ opcode overrides. |
| `filter_rules` | `list` | `match` + `action: "exclude"` to skip specific samples. |
| `trigger_rules` | `list` | Map samples to `attack`, `release`, etc. |
| `velocity_crossfade` | `float` | Profile override for velocity crossfade. |
| `release_decay` | `float` | Profile override for release decay. |

**Path-level filtering example:**
```yaml
profiles:
  - name: "Natural"
    lpf: 0
    has_release: true
    merge_groups: true
  - name: "Presence"
    lpf: 0
    has_release: true
    merge_groups: true
  - name: "Warm"
    lpf: 2000
    has_release: true
    merge_groups: true
```

---

## 🧪 Fusion Rules (`fusion_rules`)

Creates "Performance" patches by merging two articulations (e.g., Sustain + Legato).

> [!IMPORTANT]
> `articulation` and `technique` strings must match cleaned names **after** `display_name_cleanup` runs, not raw group names.

| Field | Type | Description |
| :--- | :--- | :--- |
| `articulation` | `string` | Base articulation name. |
| `technique` | `string` | Target technique to merge. |
| `control_cc` | `int` | CC number for crossfading or switching. |
| `type` | `string` | `"Crossfade"`, `"Switch"`, `"VelocitySplit"`, `"Legato"`, `"Release"`, or `"Collection"`. |
| `base_match` | `list` | Filename glob patterns for the base articulation (or all elements in `Collection`). |
| `tech_match` | `list` | Filename glob patterns for the technique articulation (Ignored in `Collection`). |
| `split_point` | `int` | Override CC/velocity split boundary. Default: `64`. |
| `crossfade_curve` | `string` | Override `xf_cccurve`. Default: `"gain"`. |
| `offset` | `int` | **[Legato]** Sample start offset in samples (bypass silent attack). |
| `delay` | `float` | **[Legato]** Trigger delay for target sustain (seconds). |
| `fade_in` | `float` | **[Legato]** Fade-in time for target sustain (seconds). |
| `fade_out` | `float` | **[Legato]** Fade-out time for transition sample (seconds). |
| `inline` | `bool` | If `true`, embeds regions directly into the merged SFZ instead of using `#include`. Best practice for DAW piano roll detection. |
| `direction` | `string` | **[Legato]** Interval direction: `"up"`, `"down"`, or `"auto"` (default). `"auto"` sniffs filename for the word `"up"`. |
| `strip_regex` | `string` | **[Legato]** Regex to strip per-RR opcodes from transition regions. |
| `interval_regex` | `string` | **[Legato]** Fallback regex for interval extraction (capture group 1 = semitones). Default: `_(\d+)_+\d+\.flac`. |
| `legato_group_opcodes` | `map` | **[Legato]** Per-group SFZ opcode overrides. Keys: `"first"`, `"legato"`, `"target"`. |
| `interval_map` | `map` | **[Legato]** Map interval strings/direction codes to integer offsets. |

---

## 🔊 Spectral Spread Config (`spectral_config`)

Tunes randomization when `spread_mode: spectral` generates synthetic chromatic neighbours. All fields optional; zero-value falls back to hardcoded default.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `pan_range` | `int` | `15` | Stereo pan half-width (±N). |
| `base_cutoff` | `int` | `2000` | Base LPF cutoff (Hz) for synthetic neighbours. Overridden by `global_opcodes.cutoff` if set. |
| `cutoff_variance` | `[int, int]` | `[-400, 600]` | Random deviation range `[min, max]` added to `base_cutoff`. |
| `resonance_max` | `int` | `7` | Upper bound for randomized `resonance` on synthetic neighbours. |

---

## 🗂️ Sub-Instruments & Mic Positions

`sub_instruments` is the **outer loop**. `mic_positions` is the **inner loop** that runs for every sub-instrument. Together they reconstruct monolithic "all-in-one" libraries into a clean `[Category]/[Instrument]/[Mic Rig]/` hierarchy.

### `sub_instruments`

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | Display name suffix (e.g., "Erhu"). |
| `filter` | `string` | Regex to include only samples for this sub-instrument. |
| `folder` | `string` | Optional relative sub-directory path (e.g., `Stringed/Erhu`). |

### `mic_positions`

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | Suffix for the generated SFZ (e.g., "Close", "Room"). |
| `filter` | `string` | Regex matched against the **original raw filename** (not sanitized). |
| `folder` | `string` | Optional relative sub-directory path for generated SFZ files. |

> [!IMPORTANT]
> **`mic_positions` filter matches the ORIGINAL raw filename, not the sanitized working filename.** Use `raw_name_cleanup` with `regex_mapping` to strip mic prefixes from the working filename so `group_regex` generates identical articulation keys across all mic rigs.

**Multiplexing example:**
```yaml
sub_instruments:
  - name: "Single Notes"
    folder: "SingleNotes"
    filter: "Abyss_(Sustain|Staccato|Harmonics|HammerOn|PullOff|Pop|Slide|FX_Slides)"
  - name: "Noises"
    folder: "Noises"
    filter: "Abyss_(PitchedRelease|DI_Noise)"

mic_positions:
  - name: "Down"
    filter: "Down"
  - name: "Up"
    filter: "Up"
  - name: "Neutral"
    # Explicitly list techniques that don't have Down/Up in Shreddage 3.5 Abyss
    filter: "Sustain|Staccato|Harmonics|PitchedRelease|Pop|HammerOn|PullOff|Noise|FX_Slides|DI_Noise"
```
*Result: Automatically builds `Stringed/Erhu/Close/` + `Stringed/Erhu/Room/` with dedicated Master Keyswitch SFZs per mic.*

**Multi-mic rig anti-double-naming rule:**
When both `mic_positions` and `mic_labels` are defined, the engine may produce `Close - Close.sfz`. To prevent this, set `merge_articulations: true` at root + `merge_groups: true` inside profiles.

---

## 🎏 Best Practices

### Velocity Strings (Orange Tree Samples)

For libraries using text instead of numbers for velocity (e.g., `loud`, `med`, `soft`), use `vel_map` to assign them to numeric MIDI zones, avoiding fragmented articulation groups.

```yaml
vel_regex: '(?i)_(loud|med|soft)_'
vel_map:
  soft: 42
  med: 85
  loud: 127
```

### Plucked String Instruments (Guzheng, Koto, Harp, Guitar)

Plucked strings ring out fully once plucked, regardless of key hold duration.

1. **Use selective `one_shot` via `trigger_rules`** — never global `one_shot` (breaks Tremolo/looped sustains):
   ```yaml
   trigger_rules:
     # Hold-based — must ring continuously
     - match: "(?i)Tremolo|Trem|TR"
       opcodes:
         loop_mode: "wait_until_release"
     # Transient — one pluck, done
     - match: "(?i)Pluck|Pick|Finger|Pizz|Hammer|Pull|Slide|Bounce|Spic|Stacc|Stroke|Down|Up"
       opcodes:
         loop_mode: "one_shot"
   ```

2. **Release trails**: When strings use `one_shot`, also map release samples via `trigger: release` + `loop_mode: one_shot` to simulate damper/body noise.

3. **Damping**: Use `polyphony: 1` or group-level muting to cut a string's vibration when it is struck again.

---

### True Legato Reconstruction

1. **Sanitize typos and extract legato style first**:
   ```yaml
   sanitizers:
     raw_name_cleanup:
       # Fix typos in interval note pairs
       - action: "regex_replace"
         find: "C3-D#-"
         replace: "C3-D#3-"
       # Promote speed suffix into articulation name before note parsing
       - action: "regex_replace"
         find: "Ocarina_(LegDn|LegUp)_(.*)-Slow(\\d+)"
         replace: "Ocarina_${1}Slow_${2}-$3"
       # Strip source note, keep only target note (C4-A#3 → A#3)
       - action: "regex_replace"
         find: "(?i)([a-g][#b]?\\d)-([a-g][#b]?\\d)"
         replace: "$2"
   ```

2. **Engine legato `sw_previous` assignment**:
   - If source+target notes are in the filename, the engine extracts `sw_previous` directly.
   - For dynamic signed offsets (e.g., `Int_-2`), define `legato_interval_regex`:
     ```yaml
     legato_interval_regex: '(?i)Int_(-?\d+)'
     ```
   - For unsigned offsets with a direction folder, the engine computes `previousNote = targetNote + (interval × direction)`.

3. **Define `fusion_rules` for every legato direction/speed**:
   ```yaml
   fusion_rules:
     - articulation: "Sustain"
       technique: "Legato Down"
       type: "Legato"
       base_match: ["* - Sustain.sfz"]
       tech_match: ["* - Legato Down.sfz"]
       offset: 1500
       delay: 0.120
       fade_in: 0.080
       fade_out: 0.050
   ```

---

### Smart Pitch Anchoring (Unpitched Phrases & Textures)

For libraries where some articulations (like `Sustain`) have explicit pitches in their filenames (e.g., `_C3_`), but other articulations (like `Improvisation`, `Chords`, or `Slap`) are unpitched phrase loops, use `remap_rules` to automatically anchor the unpitched samples to the starting note of the pitched articulation.

1. **Define Articulation Order**: Ensure the reference articulation (the "Anchor") is processed *before* the unpitched ones.
   ```yaml
   articulation_order: ["Sustain", "Sus"]
   ```

2. **Configure Remap Rules**: Map the unpitched articulation to the `Anchor`. The engine will inject the anchor's lowest detected MIDI note into the unpitched samples, allowing them to stack perfectly with velocity layers and round robins on a single key.
   ```yaml
   remap_rules:
     - articulation: "Improvisation"
       anchor: "Sustain"
       strategy: "anchor_start"
     - articulation: "Slap"
       anchor: "Sustain"
       strategy: "anchor_start"
   ```

---

### Multi-Input Surgical Fusion (High Performance)

For complex libraries where transition samples and holds are stored in sibling directories (e.g. `/Samples/Legato` vs `/Samples/Sustain`), use the **Multi-Input** architecture to avoid scanning the entire sample root.

1. **Configure Multiple Inputs**: List only the specific folders required for the instrument.
2. **Enable Flat-Walk**: Use `flat_walk: true` to ensure the engine recurses into the Syllable subdirectories within those inputs.
3. **Surgical Fusion**: The engine aggregates all files into one pass, enabling perfect fusion without root-path "spaghetti".

```yaml
# Surgical Legato Profile
inputs:
  - "/path/to/Samples/Legato"
  - "/path/to/Samples/Sustain"
output: "/path/to/SFZ/Legato"
flat_walk: true

fusion_rules:
  - articulation: "Sustain A"
    technique: "Legato A"
    type: "Legato"
    base_match: ["*Sustain*A*.sfz"]
    tech_match: ["*Legato*A*.sfz"]

---

### Mechanical Pedal Noises (Piano/Harp)

For physical pedal-up and pedal-down sounds, use CC-based triggers instead of note triggers to simulate mechanical action.

1. **Use CC64 state triggers**: Map samples using `on_locc64` and `on_hicc64` to fire exactly once when the pedal crosses the threshold.
2. **One-shot playback**: Use `loop_mode: one_shot` so mechanical clicks play to completion regardless of pedal release speed.
3. **Self-muting**: Use `group` and `off_by` with the same ID to cut previous pedal sounds during rapid "pumping", preventing unnatural noise build-up.

```yaml
trigger_rules:
  - match: "Pedal.*Dn"
    opcodes:
      on_locc64: 64      # Trigger when pedal enters Down state
      on_hicc64: 127
      loop_mode: "one_shot"
      group: 100
      off_by: 100        # Mute previous pedal sound
      pitch_keycenter: 60
  - match: "Pedal.*Up"
    opcodes:
      on_locc64: 0       # Trigger when pedal enters Up state
      on_hicc64: 63
      loop_mode: "one_shot"
      group: 100
      off_by: 100
      pitch_keycenter: 60
```

---

### Chromatic Keyboard Mapping & Gap Filling (Pentatonic/Whole-Tone)

For instruments with non-chromatic scales (e.g., pentatonic or whole-tone instruments like the Guzheng), we can reconstruct both a standard authentic layout and a full performance chromatic layout using a dual-profile configuration.

#### 1. Dual-Profile Configuration
By defining both `Standard` and `Performance` profiles, the engine separates the generation into Transparent master files (containing unmodified mappings) and Performance master files (containing chromatic mappings):

```yaml
profiles:
  - name: "Standard"
  - name: "Performance"
    enable_gap_filling: true
    spread_mode: "stretch" # Options: "stretch", "spectral"
```

#### 2. Key Mapping & Pitch Tracking
* **Gap Filling**: Enabling `enable_gap_filling: true` automatically expands the key range bounds (`lokey` and `hikey`) of raw samples to overlap and cover empty neighboring notes.
* **Dynamic Transposition**: When the SFZ file plays a note different from `pitch_keycenter`, standard SFZ keyboard tracking (`pitch_keytrack=100`) pitch-shifts the playback automatically. For example, triggering key 40 with `pitch_keycenter=39` will shift the pitch up by 1 semitone. No manual `transpose` opcode is required.
* **Spread Modes**:
  * `spread_mode: "stretch"`: Generates continuous key ranges per region block (e.g., `lokey=39 hikey=40 pitch_keycenter=39`), which is clean and efficient.
  * `spread_mode: "spectral"`: Automatically expands and splits intervals into discrete single-key regions (`lokey=k hikey=k`) with randomized pan, filter cutoff, and resonance to simulate acoustic/positional variation.

#### 3. Advanced Keyboard Spread & Gap-Filling Control
To achieve realistic, high-fidelity chromatic mappings on non-chromatic acoustic instruments, you can tune how samples stretch across gaps:

* **Directional Stretch Control (`spread_direction`)**:
  * `"both"` (default): Symmetrical stretch extending up and down to meet the neighbor notes at a split point.
  * `"up"`: Stretches only upward. Lower notes are not allowed to be played by shifting higher samples down.
  * `"down"`: Stretches only downward. Higher notes are not allowed to be played by shifting lower samples up.

* **Transposition Limits (`max_stretch_up` / `max_stretch_down`)**:
  * Define the maximum transposition interval (in semitones) in either direction to prevent artifacts from over-stretching. If a gap exceeds the limits, a dead zone is created.

* **Split Bias (`spread_bias`)**:
  * Adjusts the transition point between neighboring samples. A bias of `0.5` is the exact midpoint. A bias $< 0.5$ shifts the boundary lower (favoring the higher sample's range), while a bias $> 0.5$ shifts it higher (favoring the lower sample's range).

* **Distance-Based Compensation (`spread_compensation`)**:
  * Compensates for timbre changes (brightness loss or volume inflation) that occur when stretching a sample away from its original key:
    ```yaml
    spread_compensation:
      volume_db_per_semitone: -0.5  # Reduces volume by 0.5 dB per semitone stretched away from the root pitch
      cutoff_hz_per_semitone: -150  # Attenuates the LPF cutoff by 150 Hz per semitone stretched to simulate high-end decay
    ```
  * *Note: When `spread_compensation` is configured, regions are automatically split into single-key region lines to apply individual offset calculations.*

* **Asymmetric Crossfade Blend (`spread_blend` / `blend_width`)**:
  * Setting `spread_blend: true` creates an overlap of `blend_width` semitones between adjacent zones. Stretches overlap on intermediate keys to blend the acoustic timbres smoothly, while protecting the original root key centers from being triggered simultaneously.



## Referenced Files
The following Knowledge base files are part of this skill. Read them if needed:
- [SFZ_PEDAL_CC_CONCEPT.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/SFZ_PEDAL_CC_CONCEPT.md)
- [SFZ_OPCODE.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/SFZ_OPCODE.md)
- [KONTAKT_FILE_EXT.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/KONTAKT_FILE_EXT.md)
- [SFZ_PIANO_BEST_PRACTICE.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/SFZ_PIANO_BEST_PRACTICE.md)
- [KONTAKT_RECONSTRUCT_OPCODES.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/KONTAKT_RECONSTRUCT_OPCODES.md)
- [KONTAKT_RECONSTRUCT_YAML.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/KONTAKT_RECONSTRUCT_YAML.md)
- [KONTAKT_EXTRACTOR_API.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/kontakt-reconstruct_kb/KONTAKT_EXTRACTOR_API.md)

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=kontakt-reconstruct`