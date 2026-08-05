---
name: pdf-navigation
description: "Techniques for navigating and extracting content from PDFs using Firefox's built-in PDF viewer (PDF.js). Use this skill when the user asks to read specific pages, jump to a location, or extract text from a PDF."
framework: "zen-mcp"
trigger: pdf
---

# PDF Navigation & Extraction Skill

This skill codifies the patterns for interacting with the Firefox built-in PDF viewer (PDF.js) via the `browser` tool.

## 1. Content Extraction (Read Page)

To read the content of a specific page, use the `read` action with the `data-page-number` selector. 

> [!NOTE]
> PDF.js often uses lazy rendering. If a page is far from the viewport, it may be empty. Always **scroll** to the page before reading if you suspect it hasn't loaded.

### Example: Read Page 6
```json
{
  "action": "read",
  "selector": "[data-page-number=\"6\"]"
}
```

## 2. Navigation (Jump to Page)

There are two main ways to jump to a page:

### A. Using Scroll (Recommended)
This is the most reliable way to ensure the page is rendered for reading.
```json
{
  "action": "scroll",
  "selector": "[data-page-number=\"10\"]"
}
```

### B. Using the Toolbar Input
You can also use the native toolbar input.
```json
{
  "action": "type",
  "selector": "#pageNumber",
  "text": "10\n"
}
```

## 3. UI Element Mapping (Key Selectors)

| Element | Selector | Purpose |
| :--- | :--- | :--- |
| **Page Container** | `[data-page-number="N"]` | Target for reading or scrolling to page N. |
| **Page Number Input** | `#pageNumber` | Jump to a specific page. |
| **Zoom In/Out** | `#zoomIn`, `#zoomOut` | Adjust visibility. |
| **Text Editor** | `#editorFreeTextButton` | Activate text editing mode. |
| **Draw/Ink** | `#editorInkButton` | Activate drawing mode. |

## 4. Troubleshooting
*   **Empty Page**: If `read` returns an empty string but you know the page has text, call `scroll` on the page selector and wait a few hundred milliseconds before trying again.
*   **Selector Not Found**: Ensure the URL matches `file://*.pdf` or a PDF content type. Some sites embed PDFs in iframes; if so, you may need to switch frames or target the iframe source directly.
