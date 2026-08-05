---
description: Standardized workflow for generating four-language (Chinese, Pinyin, English, Vietnamese) synchronized lyrics in LRC format. USE THIS SKILL whenever users request 'quad' translations or multi-language song lyrics.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Quad Lyric Creator

---
name: quad-lyric-creator
description: "Comprehensive guide for creating multi-language (Quad) lyrics in LRC format. USE THIS SKILL whenever the user requests a 'quad' translation, 'four-language' lyrics, or needs to format lyrics with Chinese, Pinyin, English, and Vietnamese for ZenMIDI or similar visualizers."
framework: "zen-mcp"
---

# Quad Lyric Creator Skill

This skill provides a standardized workflow for generating high-quality, four-language (Quad) synchronised lyrics (.lrc).

## 1. Input Analysis
Before generation, identify the source components:
- **Primary**: Usually Chinese (Simplified/Traditional).
- **Secondary**: Existing translations (Vietnamese or English).
- **Core**: Timing markers `[mm:ss.xx]`.

## 2. Generation Workflow

### Step A: Chinese Extraction & Cleanup
Ensure the Chinese text is clean, punctuation matches the musical phrasing, and timing markers are correctly associated.

### Step B: Pinyin Generation
Generate standard Pinyin with tone marks.
- **Rule**: Pinyin should match the Chinese characters 1:1 in whitespace where possible.
- **Rule**: Capitalize start of phrases.

### Step C: Vietnamese Translation
Focus on "Hán-Việt" resonance or modern poetic Vietnamese.
- Use "Yêu chàng" or "Yêu thiếp" appropriately for gendered addresses (if known).

### Step D: English Translation
Create a concise, poetic English version that captures the meaning without being overly literal.

## 3. Formatting Standards (The "Quad" Layout)
Every timing marker must have FOUR lines in this exact order:
1.  **Chinese** (Original)
2.  **Pinyin** (Reading)
3.  **English** (Meaning)
4.  **Vietnamese** (Meaning)

### Format Example
```lrc
[00:20.22]突來的消息 那个人是你
[00:20.22]tū lái de xiāoxī nà gè rén shì nǐ
[00:20.22]Sudden news, and that person is you
[00:20.22]Tin tức đến bất ngờ, người đó chính là em
```

## 4. Metadata Mapping
Always include and translate metadata headers:
- `[00:00.00]Artist - Title` (Translated into a compound or multi-line header)
- `[00:01.00]词:Name` (Lyrisist)
- `[00:02.00]曲:Name` (Composer)

## 5. Tool Usage
- Use `browser.search` or `browser.solve_challenge` to find high-quality professional translations if a "super good version" is requested.
- For Pinyin, use internal knowledge or specialized libraries.

## 6. Verification
- **Count Equality**: Check that every timestamp has exactly 4 lines.
- **Timing Integrity**: Ensure timestamps are copied exactly from the source without alteration.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=quad-lyric-creator`