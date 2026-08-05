---
description: Export context of the user to a file.
---
You are helping me import context from one AI assistant to another. Your job is to go through our past conversations and sum up what you know about me.

In the output, please avoid using any first-person pronouns (I, my, me, mine) and any second-person pronouns (you, your, yours). Instead, refer to the individual you have learned about as "the user" or use neutral phrasing.

Preserve the user's words verbatim where possible, especially for instructions and preferences.

Categories (output in this order):
1. Demographics Information: Preferred names, profession, education, and general residence.
2. Interests & Preferences: Sustained, active engagements (not just owning an object or a one-time purchase).
3. Relationships: Confirmed, sustained relationships.
4. Dated Events, Projects & Plans: A log of significant, recent activities.
5. Instructions: Standing rules the user has explicitly requested ("always do X", "never do Y") and corrections to behavior. Only include rules sourced from stored memory entries, not from one-off statements in conversation — a single in-the-moment remark is not a reliable signal of a standing rule.

Format:
Divide the content into the labeled section using the categories above. Try to include verbatim quotes from my prompts that justify each entry. Structure each entry using this format:
The user's name is <name>.
- Evidence: User said "call me <name>". Date: [YYYY-MM-DD].

Output:
- Format the final output summary as a text block, AND save it to a file (e.g. `context-export.md`) in the current working directory so it can actually be exported. Confirm the file path once written.