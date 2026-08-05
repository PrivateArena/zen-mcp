---
description: High-level orchestration for the ZenMIDI DAW. USE THIS SKILL whenever a user wants to compose music, add tracks, manage a DAW session, or automate audio parameters. This guide provides the 'clean and simple' workflow for agents to interact with the Go-based DAW backend.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: DAW Orchestration Skill

---
name: daw-orchestration
description: "High-level orchestration for the ZenMIDI DAW. USE THIS SKILL whenever a user wants to compose music, add tracks, manage a DAW session, or automate audio parameters. This guide provides the 'clean and simple' workflow for agents to interact with the Go-based DAW backend."
framework: "golang"
---

# 🎹 ZenMIDI DAW Agent Skill

This skill codifies the standard workflow for interacting with the ZenMIDI headless DAW.

## 🌐 Server Environment

- **Address**: `http://127.0.0.1:7777`
- **Header**: Always set `X-Author: <AgentName>` to track your changes in the session history.
- **Format**: All requests must be `Content-Type: application/json`.

---

## 🚀 Quick Start Workflow

### 1. Initialize Session (Optional if session is up)

Set global tempo, key, and scale.

- **POST** `/api/daw/session`

```json
{
  "library": "default",
  "tempo": 120,
  "key": "D",
  "mode": "major"
}
```

### 2. Discover & Add Tracks

Find an instrument and bind it to a track ID.

- **GET** `/api/daw/instruments` (Returns list of available instruments)
- **POST** `/api/daw/tracks`

```json
{
  "id": "t1",
  "name": "Grand Piano",
  "instrument": "Piano"
}
```

{"articulation":"cut"}
{"instrument":"Bombarde"}
{"key":"G"}
// Multiple fields can be updated together
{"mode":"mixolydian","key":"GMix"}

### 4. Agent-Friendly MIDI FX (Remix)

Use `remix_patch` for structured MIDI transforms. This is the preferred way for agents to manage looping and legato.
**PATCH** `/api/daw/tracks/{id}`

```json
{
  "remix_patch": {
    "loop": {
      "enabled": true,
      "mode": "clip",
      "mins": 5.0,
      "delay_seconds": 1.0,
      "evolution": "ambient"
    },
    "legato": "classical",
    "transpose": 12
  }
}
```

Unfriendly for agent, DO NOT use unless you need to update multiple tracks at once or above method is not working (Legacy)

```json
{
"remix": "loop:clip:5:1.0:ambient"
}
```

### 5. Audio DSP (Effects)

Update audio processing parameters directly on a track.
**PATCH** `/api/daw/tracks/{id}`

```json
{
  "reverb_preset": "Cathedral",
  "reverb_wet": 0.4,
  "saturation": 0.5,
  "hpf": 100,
  "comp_enabled": true,
  "comp_preset": "Mastering"
}
```

### 6. Arrange Clips (Polymorphic Body)

Place music on the timeline using a unified `body` and `type` schema.

- **POST** `/api/daw/arrange`
- **PATCH** `/api/daw/arrange/{id}` (Update ANY clip parameter including notation and automation)

```json
{
  "track_id": "t1",
  "type": "abc",
  "body": "D2E2 F2G2 | A2B2 c2A2 |",
  "start_beat": 0.0,
  "automation": [
    {
      "target": "cc:11",
      "template": ["start_fadein", "mid_up", "end_fadeout"]
    }
  ]
}
```

---

## 🛠️ Core Modules

### 🎼 Music Notation

- **ABC** (`type: "abc"`): Best for simple melodic lines and traditional music.
- **MIDIText** (`type: "miditext"`): Best for high-fidelity MIDI manipulation and polyphony.
  - JSON format: `{"notes": [{"pitch": 60, "start": 0, "duration": 1}]}`

### 🎛️ Semantic Automation

Instead of plotting points, use semantic segments. The system automatically scales them to the clip duration.

- **Segments**:
  - `start_`: `fadein`, `fadeout`, `low`, `high`
  - `mid_`: `up`, `down`, `rampup`, `rampdown`, `static`
  - `end_`: `fadein`, `fadeout`, `low`, `high`
  - `full_`: `fadein`, `fadeout`, `bell`, `dip`
- **Curves**: Append `_scurve`, `_exp`, or `_step` (e.g., `start_fadein_scurve`).

### 🔍 Monitoring & Maintenance

- **Status**: `GET /api/daw/status` — Get the full timeline, hydrated notes, and playback state.
- **Undo**: `POST /api/daw/undo` — Revert the last arrangement change.
- **History**: `GET /api/daw/history` — List revisions.

---

## 💡 Best Practices

1. **Author Attribution**: Use your name in `X-Author` header.
2. **Immediate Feedback**: All `POST` and `PATCH` requests (Tracks/Arrange) return the **full updated object** including generated IDs, compiled notes, and expanded automation.
3. **Dry Run**: Set `"dry_run": true` in arrangement requests to validate notation without committing.
4. **Structured Remixing**: ALWAYS prefer `remix_patch` over the raw `remix` string. It allows you to use human-friendly units like `mins` (minutes) and `delay_seconds` without calculating BPM-specific beats.
5. **DSP Persistence**: Track-level DSP settings (reverb, EQ, compression) persist even if the instrument or articulation changes.
6. **Polymorphic Switching**: You can seamlessly switch a clip from `abc` to `miditext` by PATCHing both fields; the system will recompile the MIDI data immediately.


## Referenced Files
The following Knowledge base files are part of this skill. Read them if needed:
- [MIDITEXT_NOTATION_SKILL.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/daw-api_kb/MIDITEXT_NOTATION_SKILL.md)
- [ABC_NOTATION_SKILL.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/daw-api_kb/ABC_NOTATION_SKILL.md)
- [DAW_AUTOMATION_SKILL.md](file:///media/jang/home/Deve/web-reader-mcp-master/src/resources/skills/daw-api_kb/DAW_AUTOMATION_SKILL.md)

---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=daw-api`