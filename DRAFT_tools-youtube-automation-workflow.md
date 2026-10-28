# Architecture Plan: Youtube Automation Workflow

## Summary

This document outlines an **architecture for a browser-based YouTube automation workflow**, built upon an existing internal/bridge to expose quick abilities to interact with Youtube.

Goal:
- Upload
- Read view comments (yt-dlp)
- Interact with comments by replying

## System Boundaries

### Upload

Tool youtube -a upload -f <filepath> -t <title> -d <description> -c <category> -y <visibility>

Upload works by navigating using bridge's navigate API to https://www.youtube.com/upload
and then fill and submit the form.

Steps:
1. Navigate to https://www.youtube.com/upload with container set to `pachoigame`
2. Wait for the page to load
3. Upload using bridge's upload API
3. Fill the form
    - Title
    - Description
    - Category
    - Visibility
    - Thumbnail
4. Submit the form

Note: For testing and sandboxing, use container `pachoigame`.

### Read comments

This is simpler, use yt-dlp to fetch comments

### Reply comments

Basically we will navigate to the video URL, focus on the comment, reply and submit.

## config.json expansion

In config.json, we expand a small section for youtube tool with container setting. 
For examp

```json
{
    "youtube": {
        "container": "pachoigame"
    }
}
```

## Skill

At the end of the task, after API is stable, we create a skill to use this tool.