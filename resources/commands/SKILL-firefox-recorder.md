---
description: Recorder API to study/parrot API requests, use together with firefox-bridge
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Firefox Recorder API

# 🎙️ Zen Recorder & Cache API

**Version**: `2.2.0` (Advanced BadgerDB Engine)  
**Endpoint**: `http://127.0.0.1:31313/api/recorder` (or via `Host: recorder.local`)  
**Content-Type**: `application/json`

High-performance web recording and caching engine. Supports deduplicated storage, Zstd compression, and database-like querying.

---

## 🛠 CORE ACTIONS

### `status`
Retrieve current recorder configuration and active session.
```bash
curl -s -H "Host: recorder.local" http://127.0.0.1:31313/api/recorder/status | jq .
```

### `stats`
Retrieve database statistics (item counts and disk usage).
```bash
curl -s -H "Host: recorder.local" http://127.0.0.1:31313/api/recorder/stats | jq .
```

### `toggle` (Start Recording)
Enable recording for a specific session.
- **`enabled`**: `true` to start recording new traffic.
- **`cache_enabled`**: `true` to serve matching URLs from the database.
- **`session_id`**: The database name.
- **`domains`**: Optional array to filter hosts.
- **`timeout`**: Safety valve in seconds (default: `600`).


```bash
curl -X POST http://127.0.0.1:31313/api/recorder/toggle \
     -H "Host: recorder.local" \
     -d '{
       "enabled": true,
       "session_id": "api_flight_01",
       "domains": ["www.jmarshall.com"],
       "timeout": 300
     }'

curl -X POST http://127.0.0.1:31313/api/recorder/toggle \
     -H "Host: recorder.local" \
     -d '{
       "cache_enabled": true,
       "session_id": "api_flight_01"
     }'
```

### `close`
Explicitly close the session and release file locks.
```bash
curl -X POST http://127.0.0.1:31313/api/recorder/close -H "Host: recorder.local"
```

---

## 🔍 QUERY & INSPECTION

### `list`
List recorded entries with support for pagination and filtering.
- **`domain`**: Efficient site-level filter.
- **`prefix`**: URL substring filter.
- **`limit`**: Max items to return.
- **`offset`**: Items to skip.

```bash
# List all resources for a specific domain and path prefix
curl -X POST http://127.0.0.1:31313/api/recorder/list \
     -H "Host: recorder.local" \
     -d '{
       "domain": "www.jmarshall.com",
       "limit": 10,
       "offset": 0
     }'
# List all resources for a specific domain and path prefix
curl -X POST http://127.0.0.1:31313/api/recorder/list \
     -H "Host: recorder.local" \
     -d '{"domain": "www.jmarshall.com", "prefix": "html"}' | jq .
# List all resources for a specific domain
curl -X POST http://127.0.0.1:31313/api/recorder/list \
     -H "Host: recorder.local" \
     -d '{"domain": "www.jmarshall.com"}' | jq .
```

### `inspect`
Retrieve full metadata and bodies for a specific URL.
```bash
curl -X POST http://127.0.0.1:31313/api/recorder/inspect \
     -H "Host: recorder.local" \
     -d '{
       "url": "http://www.jmarshall.com/easy/html/testbed.html",
       "host": "www.jmarshall.com"
     }'
```

### `delete`
Remove a specific entry from the database.
```bash
curl -X POST http://127.0.0.1:31313/api/recorder/delete \
     -H "Host: recorder.local" \
     -d '{
       "url": "http://www.jmarshall.com/junk",
       "host": "www.jmarshall.com"
     }'
```

---

## 🔑 ARCHITECTURAL NOTES

1. **Storage**: Initial footprint ~32MB. Files auto-rotate every 16MB.
2. **Deduplication**: Working for both request and response bodies.
3. **Safety Valve**: 10-minute auto-stop prevents disk fill from accidental browsing.
4. **Consistency**: Protocol forced to `HTTP/1.1` and hop-by-hop headers removed during cache hit to ensure browser compatibility.


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=firefox-recorder`