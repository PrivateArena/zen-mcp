---
name: product-research
description: Conduct high-signal e-commerce product research, filtering for geographic platforms (e.g., Shopee/Lazada in Vietnam), optimizing mechanical keyboard targets (kits, keycaps, switches), and balancing affordability with quality.
license: MIT
framework: "zen-mcp"
trigger: product research
---

## 1. Dynamic Geolocation & Platform Discovery

To ensure localized search optimization without requiring manual country arguments, the agent must perform dynamic geolocation discovery at the start of the session:

### A. Geolocation Discovery Protocol
1. **Fetch Location:** Call `browser({ action: 'request', url: 'https://ipinfo.io/json' })` (or fallback to `browser({ action: 'request', url: 'http://ip-api.com/json/' })` if blocked).
2. **Extract Country:** Retrieve the `country` or `countryCode` field (e.g., `VN`, `US`, `DE`).

### B. Platform Strategy by Country
* **Vietnam (VN):**
  * **Primary Platforms:** **Shopee (shopee.vn)** and **Lazada (lazada.vn)**.
  * **Cross-Border Sourcing:** Cross-border shops on Shopee/Lazada (shipping from China) offer the lowest prices on keyboard parts (7-10 day shipping).
  * **Currency:** Vietnam Dong (VND). Use rate: 1 USD ≈ 25,000 VND.
* **Other Countries (e.g., US, CA, EU):**
  * **Primary Platforms:** Local Amazon sites, AliExpress, eBay, or specialized local distributors (e.g., Divinikey for US).
  * **Currency:** Local currency.

---

## 2. Product Profile: Mechanical Keyboard (e.g., Leobog Hi75/Hi75c)

When researching mechanical keyboards and components, organize findings into structural layers:

### A. Core Keyboard Kit / Barebone
* **Hi75 / Hi75c:** A highly popular budget aluminum 75% layout keyboard (gasket-mounted, flex-cut PCB).
* **Connection Types:** Hi75 (Wired only), Hi75c (Tri-mode: Wired, 2.4Ghz Wireless, Bluetooth).
* **Sourcing Focus:** Identify whether the listing is for a **Barebone Kit** (no switches/keycaps) or a **Pre-built Keyboard** (includes switches and keycaps). Barebones are usually much cheaper if the user plans to buy custom switches/keycaps separately.

### B. Targets & Components
* **Keycaps:**
  * **Affordable & Quality:** Double-shot PBT or thick Dye-Sublimation PBT. Avoid cheap ABS keycaps as they shine quickly.
  * **Profiles:** Cherry, OEM, or XDA profile.
* **Switches:**
  * **Brands:** Leobog (e.g., Graywood V3/V4, Reaper, Nimbus), Outemu, Gateron, Akko.
  * **Types:** Linear (smooth, quiet) vs. Tactile (tactile bump). Graywood V3/V4 linears are highly recommended for budget builds.

---

## 3. Optimization Framework: Affordability vs. Quality

Maximize the budget-to-performance ratio using the following strict filtering rules:

### A. Affordability Checklist
1. **Compare Base Prices:** Find the baseline barebone cost.
2. **Bundle Verification:** Calculate if buying a Pre-built keyboard is cheaper than buying a Barebone + separate switches + separate keycaps.
3. **Shipping & Vouchers:** Note that cross-border shipping is often cheaper or heavily subsidized via platform vouchers.

### B. Quality Safeguards
1. **Seller Ratings:** Only source from stores with a rating of **4.7/5.0 stars or higher**.
2. **Review Count:** Look for listings with at least 10+ actual user reviews (with photos/videos if possible) to confirm the item matches description.
3. **Brand Authorization:** For brand items like Leobog, prefer "Shopee Mall", "Lazada Mall", or established third-party keyboard hobbyist shops (e.g., Sói Gear, Mèo Sành Điệu).

---

## 4. Search & Verification Protocols

When using the MCP Web search tool, use targeted search patterns to bypass irrelevant listings:

```bash
# General search pattern for Vietnam Shopee/Lazada
"<Product Name> site:shopee.vn"
"<Product Name> site:lazada.vn"

# Example targeting Hi75c barebone kit
"Hi75c barebone site:shopee.vn"
"Hi75c kit site:lazada.vn"
```

---

## 5. Output Synthesis Schema

Every product research task must culminate in a structured breakdown with a comparison table and a clear final recommendation:

### Product Comparison Table
| Product / Part | Store Name | Platform | Price (VND) | Approx. USD | Rating | Pros / Cons / Quality Notes |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| *e.g., Hi75c Barebone Kit* | *Space Keyboard* | *Shopee* | *1,250,000* | *~＄50* | *4.9/5* | *Tri-mode connection, aluminum, cross-border shipping* |

### Final Recommendations
* **Lowest Cost Route:** [Direct link to cheapest listing with acceptable rating]
* **Best Quality Route:** [Direct link to Mall store or highly-rated local shop]
* **Component Recommendation:** Best switch/keycap combination that matches the target budget.
