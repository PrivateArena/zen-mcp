---
description: How to extract and read Firefox internal API schemas from omni.ja so agents never have to guess userChrome APIs. USE THIS SKILL whenever users ask for help automating Firefox, writing userChrome tweaks, or when debugging the MCP browser bridge.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Firefox API Research Using omni.ja

---
name: firefox-api-research
description: "How to read Firefox internal API schemas. USE THIS SKILL whenever users ask for help automating Firefox, writing userChrome tweaks, or when debugging the MCP browser bridge."
framework: "ucjs"
---

# Firefox API Research Protocol

Firefox's internal API changes frequently without public documentation. Instead of guessing or assuming stale data, agents MUST dynamically inspect the live browser memory.

## 1. The Primary Method: Live Memory Dumping (Zero Extraction)
If a `userChrome.js` MCP bridge is running (e.g., `/media/jang/home/PortableApp/firefox/profile/chrome/JS/mcp_bridge.uc.js`), you can query the exact Firefox API currently running in memory by evaluating JavaScript directly in the Chrome Process using the `chrome_eval` action.

**Workflow:**
1. Call the `browser` tool with `action: 'chrome_eval'`.
2. Query the live classes instead of hallucinating functionality:
   ```javascript
   // Get all available Firefox Services
   Object.keys(Services)

   // See all properties/methods on the Browser UI controller
   Object.keys(gBrowser)

   // Find core components available in the binary
   Object.getOwnPropertyNames(Components.classes)

   // Check if a specific UI function exists
   typeof gBrowser.removeTab === "function"
   ```
3. **Live Decompiling (Reading Source Code)**: If you need to know exactly how a Firefox internal function works or what parameter arguments it accepts, you can decompile it live by calling `.toString()`:
   ```javascript
   // Read the internal XUL/Gecko source code of addTab!
   gBrowser.addTab.toString()
   ```
4. **Reading Entire Chrome Files**: You can read the raw contents of any `chrome://` or `resource://` (or `jar:file://`) URI directly without extraction by evaluating an asynchronous `fetch` call:
   ```javascript
   (async function() {
       let req = await fetch("chrome://browser/content/browser.js");
       return await req.text();
   })()
   ```

5. **Directory Searching (Finding Source Files)**: Due to Firefox mapping internal files through `omni.ja` archives, directory enumeration on `chrome://` isn't possible. However! You can query the archives natively in memory via `nsIZipReader` to quickly list any file matching a search pattern:
   ```javascript
   (function() {
       let results = [];
       let Z = Cc["@mozilla.org/libjar/zip-reader;1"];
       function scanOmni(dirProp) {
           try {
               let file = Services.dirsvc.get(dirProp, Ci.nsIFile).clone();
               file.append("omni.ja");
               if (!file.exists()) return;
               let zr = Z.createInstance(Ci.nsIZipReader);
               zr.open(file);
               let entries = zr.findEntries("*browser.js*"); // Put search keyword here
               while (entries.hasMore()) {
                   // Returns fetchable jar URIs
                   results.push(`jar:file://${file.path}!/${entries.getNext()}`);
               }
               zr.close();
           } catch(e) {}
       }
       scanOmni("GreD");       // Core backend (XPCOM, platform)
       scanOmni("XCurProcD");  // Frontend (Browser UI)
       return results;
   })()
   ```
   **Output:** `["jar:file:///path/to/firefox/browser/omni.ja!/chrome/browser/content/browser/browser.js", ...]`
   *You can take any output path from this list and pass it straight to `fetch` (as shown in Step 4) to instantly dump the native C++/XUL/JS codebase source strings matching your result!* 

This workflow instantly returns the exact LIVE objects Firefox is using right now. No CPU-heavy tasks or extraction required!

---

## 2. DEPRECATED: Offline Extraction (omni.ja) & Indexing
*(🚨 **CRITICAL WARNING**: Indexing the Firefox API takes a MASSIVE amount of CPU time and is officially deprecated. Do not use this unless the live memory bridge is utterly broken or inaccessible.)*

Firefox relies on two main `omni.ja` archives located at `/media/jang/home/PortableApp/firefox/omni.ja` and `/media/jang/home/PortableApp/firefox/browser/omni.ja`. 

If you absolutely MUST read the raw offline code:
1. **Pathfinding**: Use the ultra-fast pathfinder to check for file existence without extracting:
   ```bash
   unzip -l /media/jang/home/PortableApp/firefox/omni.ja | grep -i "RemoteAgent"
   ```
2. **Persistent Indexed Graph**: A permanent, fully extracted parsing sandbox with a pre-indexed `codegraph` database already exists. Do **NOT** recreate it. Keep your queries pointed to:
   `/media/jang/home/knowledge_bases/firefox_omni`
   
   To use it, change the `workspace` to that folder, and use `codegraph.search` or `codegraph.skeleton`. Do NOT call `codegraph.index` again.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=firefox-api-research`