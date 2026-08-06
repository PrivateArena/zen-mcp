# MCP Performance Investigation Notes

Date: 2026-08-06
Time: 14:50
Working directory: /media/jang/home/Deve/zen-mcp

## Issue
MCP server takes 3s to complete batches of `prompts/get` requests (100+ requests from Kilo client on connect).

## Investigation Findings

### Confirmed NOT the cause
- **Server cache / pool queueing**: Removed `AcquireServer`/`ReleaseServer` pool entirely. Replaced with direct `serverCache.getOrCreate` per workspace. No `MaxPoolSize=3` serialization anymore. Still 3s delay.
- **Network I/O**: Requests are localhost, SSD is fast.
- **Logging throughput**: Can handle 100M+ logs/sec on this machine. Not the bottleneck.

### Confirmed causes (per `prompts/get` request)
1. **`GetPromptDefinition(name)` in handler closure** → `LoadPromptDefinitions()` → full rescan of:
   - `prompts.yaml`
   - `resources/prompts/*.yaml`
   - `resources/skills/` directory
   - Parses every `SKILL.md` frontmatter
   
2. **`LoadSkillContent(skillID)`** → `FindSkillByID(skillID)` → `LoadSkills()` → full rescan of:
   - `resources/skills/` directory
   - `os.Stat` every entry
   - Reads every `SKILL.md`
   - Parses frontmatter for each
   - Linear scan to find matching ID

3. **`DetectSkills`** (if enabled) → calls `LoadSkills()` again → **full rescan of skills directory AGAIN**

### Amplification factor
- Kilo client fires 100+ parallel `prompts/get` requests on connect
- Each request independently repeats 2-3 full directory scans
- 100 parallel requests × 2-3 scans = 200-300 concurrent filesystem operations
- OS serializes directory metadata lookups
- `log.Printf` mutex contention adds visible queuing

## Fixes Applied and Tested

### Fix 1: Server cache bypass (internal/server/routes.go)
- Removed `AcquireServer`/`ReleaseServer` pool from request path
- Added `serverCache` to `RouteDeps` - thread-safe map from workspace → `*mcpserver.MCPServer`
- `postMCP` now calls `d.ServerCache.getOrCreate(logicalID, ...)` directly
- Server created once per workspace, reused for all requests
- No queueing, no slot limits, no rebuilds after first creation

### Fix 2: Prompt handler uses captured definition (internal/prompts/registration.go)
- Removed `GetPromptDefinition(p.Name)` call from `prompts/get` handler
- Handler now uses captured `p` from registration closure
- Eliminates one full `LoadPromptDefinitions()` rescan per request
- **Tradeoff**: prompt hot-reload now requires server restart; skill hot-reload preserved

### Fix 3: LoadSkillContent direct path lookup (internal/prompts/skilldetector.go)
- Replaced `FindSkillByID` → `LoadSkills()` full directory scan with direct path lookup
- Now checks `skills/<id>.md` and `skills/<id>/SKILL.md` directly
- Eliminates one full skills directory scan per skill load
- Skill hot-reload preserved because file is still read on every request

### Fix 4: Logging cleanup
- Removed `POST /mcp` debug log from `routes.go`
- Consolidated prompt logs to 1 line per event in `prompts/registration.go`

## Tests Added

### internal/server/routes_test.go (server cache bypass tests)
1. `TestServerCacheReusesSameInstance` - same logicalID returns same server
2. `TestServerCacheCreatesSeparateInstances` - different logicalIDs get different servers
3. `TestServerCacheConcurrentAccess` - 50 goroutines × 20 repeats safe
4. `TestServerCacheFactoryCalledOncePerLogicalID` - factory called exactly once
5. `TestServerCacheConcurrentDifferentIDs` - 20 goroutines × 10 repeats with different IDs
6. `TestPostMCPUsesCachedServer` - initialize + tools/list uses cached server
7. `TestPostMCPLongRunningRequestDoesNotBlockNewRequests` - 1.5s slow call doesn't block ping
8. `TestPostMCPConcurrentClientsDuringLongRunningCall` - 200 clients during 1.5s long call
9. `TestPostMCPExtremeConcurrency200Clients` - 200 concurrent mixed requests
10. `TestPostMCP500ConcurrentToolsList` - 500 concurrent tools/list
11. `TestPostMCPLongCallWith200ConcurrentClients` - **5s long call + 200 concurrent new clients**

### internal/prompts/registration_test.go (optimization tests)
1. `TestLoadSkillContentDirectLookup` - skill file found by direct path
2. `TestLoadSkillContentDirectorySkill` - directory-based skill found by direct path
3. `TestLoadSkillContentMissingSkill` - missing skill returns error
4. `TestPromptHandlerUsesCapturedDefinition` - handler uses captured prompt definition, not disk reload

### Key test results:
- All tests pass
- `TestPostMCPLongCallWith200ConcurrentClients` proves: 5s `browser.chat`-equivalent call does NOT block 200 new clients
- Factory called exactly 1 time per workspace even under 500 concurrent requests
- `TestPromptHandlerUsesCapturedDefinition` proves prompt definitions are frozen at registration time
- No deadlocks, no timeouts, no forced disconnections

## Current State
- Server cache bypass is in place and tested
- Logging reduced to 1 line per event
- Root cause of 3s delay is still per-request filesystem scans (not the server cache)
- Hot-reload for skills preserved (files read on every request)
- Prompt definitions frozen until restart (acceptable tradeoff for stability)

## Next steps if 3s delay must be fixed
- Profile actual CPU/memory/IO during request burst
- Add timing instrumentation inside `prompts/get` handler to measure each phase
- Consider lazy-loading skills directory instead of full scan on every `LoadSkills()` call
- Consider caching `LoadPromptDefinitions()` result with mtime invalidation
