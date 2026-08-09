---
name: visual-analyze
description: "Expert guide for analyzing images, screenshots, and visual UIs using multimodal AI providers. USE THIS SKILL whenever a user provides an image path, asks to 'look at' a screen, needs to describe a UI, or requires visual element localization for automation."
framework: "zen-mcp"
trigger: vision
---

# Visual Analyze Skill

This skill provides a standardized workflow for performing vision-based analysis using the `browser` tool's multimodal capabilities.

## 1. Core Workflow

### A. Viewport Screenshot Analysis
When you need to analyze what the user is currently looking at in the browser:
Use `browser({ action: 'chat', provider: 'gemini', upload_files: [<up to 9 relevant file paths>], message: 'Describe the layout and key elements of this page.', take_screenshot: true })`.

### B. Local Image Analysis
When the user provides a path to a local image or a specific screenshot file:
Use `browser({ action: 'chat', provider: 'gemini', upload_files: [<up to 9 relevant file paths>], message: 'Analyze the contents of this image: <your specific question>', upload_files: ['/path/to/image.png'] })`.

### C. Application UI screenshots
Use this when you need to analyze the UI of an application running on the desktop, for example zen-midi.

Use `browser({ action: 'ui-vision', path: '["/path/to/application"]', message: 'Analyze the contents of this application: <your specific question>' })`.

## 2. Provider Selection
*   **Gemini (Preferred)**: Best for detailed UI breakdown, text extraction from images, and complex reasoning about visual elements. Use for 90% of vision tasks.
*   **Google**: Use for general search-based visual identification (e.g., "What is this landmark?"). Note: May have more restrictive content filtering.

## 3. Token & Resource Efficiency
*   **Narrow Prompts**: Instead of "What do you see?", use specific inquiries like "Extract the pricing table" or "Is the 'Submit' button visible?".
*   **Avoid Redundant Screenshots**: If the page hasn't changed, reuse the previous analysis or description instead of taking a new screenshot.
*   **Coordinate Extraction**: Use vision to get relative coordinates for `trusted_click` when standard CSS selectors fail:
    *   Prompt: "Return the center coordinates of the [Element] as JSON: { 'relX': 0.5, 'relY': 0.5 }"

## 4. Troubleshooting
*   **Bridge 500 Errors**: Often caused by a stale browser session or the provider's tab being closed. Use `browser({ action: 'navigate', url: 'https://gemini.google.com' })` to refresh if needed.
*   **Empty Responses**: If the AI says it "cannot see," ensure the `screenshot: true` or `path` parameter was correctly passed and that the provider tab is active.
