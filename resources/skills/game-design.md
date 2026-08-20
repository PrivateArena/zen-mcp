---
name: game-design
description: Build 2D tower defense games. Creates a complete project with canvas rendering (Kaplay), grid-based maps, and strict agent-executable architecture (state separation, math-based collision, waypoint interpolation).
license: Complete terms in LICENSE.txt
framework: "unspecified"
trigger: game design
---

## 1. Tech Stack & Architectural Setup

To maximize execution predictability and minimize state-syncing bugs, the game will use a **Separation of Concerns** architecture: **KAPLAY** for canvas rendering/input, and a **Vanilla JS State Machine** for game logic.

### Core Libraries

* **Game Engine:** `KAPLAY` (v3+). Chosen for its flat component entity system and native geometric placeholder rendering.
* **State Management:** Plain JavaScript Objects. The engine will read from the state, never write authoritative data to its own visual entities.
* **UI Layer:** Absolute-positioned HTML5 DOM elements overlaying the canvas.

### Folder Blueprint

```text
├── index.html          # Entry point & HTML5 UI HUD Layer
├── run-test-session.sh # The interactive lifecycle wrapper script
├── src/
│   ├── main.js         # Core game loop & KAPLAY initialization
│   ├── state.js        # Authoritative data (gold, hp, wave counters)
│   ├── map.js          # Grid matrix and waypoint calculation
│   ├── units.js        # Factory patterns for enemies, towers, projectiles
│   └── telemetry.js    # On-screen JSON data overlay for agent testing

```

---

## 2. Structural Game Design Specs

### A. Map & Grid System

The map is strictly driven by a 2D grid matrix (＄16 \times 12＄ tiles). This prevents spatial hallucinations during pathfinding.

* **Tile Size:** ＄40 \times 40＄ pixels.
* **Grid Values:** `0` = Build Zone, `1` = Enemy Path, `2` = Obstacle (Unbuildable).

```javascript
// Authoritative Map Configuration
export const MAP_CONFIG = {
    cols: 16,
    rows: 12,
    tileSize: 40,
    grid: [
        [1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        [0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        [0, 0, 0, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0],
        [0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0],
        [0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1]
    ],
    // Explicit waypoints calculated from the center of path tiles (in pixels)
    waypoints: [
        { x: 20, y: 20 },   // Tile [0,0]
        { x: 140, y: 20 },  // Tile [0,3]
        { x: 140, y: 100 }, // Tile [2,3]
        { x: 300, y: 100 }, // Tile [2,7]
        { x: 300, y: 180 }, // Tile [4,7]
        { x: 620, y: 180 }  // Tile [4,15] (Exit)
    ]
};

```

### B. Unit & Tower Definitions (Factory Archetypes)

To ensure code reuse, all units use strict constructor configurations.

```javascript
export const TOWER_TYPES = {
    DART: {
        name: "Dart Launcher", cost: 100, range: 120, damage: 15, fireRate: 1.0, 
        color: [0, 0, 255], // Fallback Blue
        upgrades: { cost: 75, damage: 25, fireRate: 1.2 }
    },
    LASER: {
        name: "Pulse Laser", cost: 250, range: 80, damage: 5, fireRate: 4.0, 
        color: [255, 0, 0], // Fallback Red
        upgrades: { cost: 150, range: 110, damage: 8 }
    }
};

export const ENEMY_TYPES = {
    SCOUT: { hp: 40, speed: 120, reward: 15, color: [0, 255, 0] },     // Fast Green
    BRUISER: { hp: 150, speed: 50, reward: 40, color: [255, 165, 0] }  // Slow Orange
};

```

### C. Upgrade and Economy Progression Math

* **Player Initial State:** ＄HP = 20＄, ＄Gold = 350＄.
* **Income Loop:** Gold is given exclusively upon enemy destruction (`ENEMY_TYPES.reward`) or as a flat ＄+50＄ bonus at the end of a successfully defended wave.
* **Linear Scale Upgrades:** Upgrading a tower mutates its core instance parameters dynamically based on its preset `upgrades` sub-object. No new entity type is instantiated.

---

## 3. Required Agent Implementation Skills

To successfully deploy this game from start to finish without breaking loops, I must apply the following core functional programming patterns:

### Skill 1: State Mutation Isolation

I will never let KAPLAY entities hold the canonical balance of player gold or enemy health. I must manage an independent `gameState` variable layout. KAPLAY entities are merely visual observers of this state.

### Skill 2: Mathematical Range Detection (No Heavy Physics)

Instead of using physical bounding colliders for tower detection ranges, I will compute standard distance formulas on every game tick. A tower acquires a target using basic Euclidean distance:

＄＄d = \sqrt{(x_2 - x_1)^2 + (y_2 - y_1)^2}＄＄

If ＄d \le \text{range}＄, the tower locks onto the enemy's index ID.

### Skill 3: Waypoint Interpolation Navigation

Enemies move using simple Vector direction vectors matching the `WAYPOINTS` index data:

1. Calculate direction vector from current position to target waypoint.
2. Normalize vector and multiply by `speed * dt` (delta time).
3. Switch target index to `index + 1` once distance to waypoint is ＄< 2＄ pixels.

### Skill 4: Robust Asset Loading Safeguards

Every single asset loader step must use a safe fallback wrapper. If a texture is missing, I will default to immediate vector shapes.

```javascript
function loadSafeSprite(name, fallbackColor) {
    loadSprite(name, `assets/＄{name}.png`)
        .catch(() => {
            console.warn(`Asset ＄{name} missing. Initializing color block.`);
            // Game logic fallback flag set here
        });
}

```

---

## 4. Telemetry and Verification Directives

When verifying code behavior using the browser test loop, I will systematically scan for the following anomalies inside the HTML telemetry box outputting data:

* **Memory Leak Detection:** If `activeEnemies` drops to zero but `activeProjectiles` keeps rising exponentially, immediately re-verify projectile lifespan deletion loops.
* **Collision De-synchronization:** If an enemy's internal HP reduces but its visual sprite doesn't flash or change coordinate positions, check if the engine-render loop lost its link to the data array index.
*   **Framerate Optimization Strategy:** If FPS drops under 60 during simulation runs, switch projectile tracking from independent entity objects to canvas-level particle system arrays.

Example reference: `browser({ action: 'screenshot', screenshot: 'full' })`