---
name: ascii-image
description: Convert UI screenshots, app interfaces, and layout images into clean ASCII wireframes and text diagrams. Use when asked to turn an image into ASCII, draw a UI layout, or represent a screenshot in text style.
framework: "zen-mcp"
trigger: ascii image
---

A visual interface converts into a fixed-width **Ascii Wireframe**. The target is structural fidelity: capturing visual hierarchy, container boundaries, and component alignment within a monospaced grid.

## Formatting Rules

* **Grid Bounds**: Limit output width to **70–80 characters** to prevent text wrapping.
* **Containers & Panels**: Enclose windows and structural regions with explicit box boundaries:
  * Horizontal borders: `+---+` or `+===+`
  * Vertical borders: `|`
* **UI Components**:
  * **Window Controls**: Right-aligned `[-][o][X]` (Minimize, Maximize, Close).
  * **Buttons**: `[LABEL]`
  * **Active / Selected Items**: `[ Active Item ]` or highlighted text within panels.
  * **Text / Path Truncation**: Collapse long path strings using `...` (e.g., `/media/.../zcode`) to preserve panel alignment.

## Conversion Steps

1. **Map the Scaffold**: Identify the primary layout splits (Header bar, Navigation sidebar, Main content panel, Footer).
2. **Draw Bounding Boxes**: Draft the outer window frame and interior dividing walls (`|`, `+---`).
3. **Populate Widgets**: Place text labels, button blocks `[LABEL]`, metadata panels, and list rows inside their respective visual cells.
4. **Align and Balance**: Verify all right-side grid borders `|` align vertically across all rows.

**Completion Criterion**: Every top-level region, navigation item, actionable button, and text group from the image is accounted for inside aligned box boundaries without line wrapping.

---

## Example

### Input Image Concept
A desktop application interface ("Zen-Cycle Account Manager") with a left sidebar, a top action header, a metadata panel, and a stacked list of source rows.

### Generated ASCII Output

```text
+-------------------------------------------------------------------+
| Zen-Cycle Account Manager                                 [-][o][X]|
+-------------------------------------------------------------------+
|               |                                                   |
|  ZEN-CYCLE    |  ZCode                    [SCAN]  [EDIT]  [REMOVE]|
|               |  +----------------------------------------------+ |
|  +---------+  |  | Location:      /media/.../zcode/home         | |
|  |  HOME   |  |  | Symlink Point: /media/.../zcode/home/.zcode  | |
|  +---------+  |  | Process Locks: zcode                         | |
|               |  +----------------------------------------------+ |
|  [ ZCode ]    |                                                   |
|               |  AVAILABLE CYCLE SOURCES (.zen-cycle)             |
|    AG         |  +------------------------------------+---------+ |
|               |  | .zcode_mrjangdonggun               | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_namidanoriyuu4              | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_pachoigame                  | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_seamleondev                 | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_support                     | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_takedaryoma4                | [SWITCH]| |
|               |  +------------------------------------+---------+ |
|               |  | .zcode_tapat                       | [SWITCH]| |
|               |  +------------------------------------+---------+ |
+---------------+---------------------------------------------------+