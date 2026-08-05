---
description: Reconstructs Hauptwerk pipe organ definition XMLs and installation packages into high-fidelity SFZ manual/division instrument suites. Supports both YAML profile-driven reconstruction and direct CLI commands.
argument-hint: |-
  i: Context or specific instructions for this skill
---
# Skill: Hauptwerk Organ Reconstruction

# 🧠 Hauptwerk Organ Reconstruction Skill (Zen-Contact)

Workflow for reconstructing Hauptwerk pipe organ definition XMLs and installation package directories into high-fidelity SFZ manual/division instrument suites.

## 🛠️ The Toolkit

| Command | Purpose |
| :--- | :--- |
| `zen-contact reconstruct -p <yaml>` | **Profile-Driven**: Auto-detects Hauptwerk XML in `patch_path` and reconstructs the organ automatically. |
| `zen-contact reconstruct-hauptwerk -packages <packages_dir> -o <output_dir> <organ_definition_xml>` | **Direct CLI**: Manual conversion from XML and package directories. |

---

## 🏃 Optimal Workflow: Profile-Driven Reconstruction

Creating a YAML reconstruction profile is the most optimal and reproducible method for reconstructing Hauptwerk organs.

### Step 1: Create the Reconstruction Profile
Create a YAML file under `reconstruct_profiles/` (e.g., `reconstruct_profiles/hauptwerk_caen_2_51_wet.yaml`).

Configure the following fields:

```yaml
name: "Caen 2.51 Wet"
patch_path: "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/OrganDefinitions/Caen 2.51 Wet.Organ_Hauptwerk_xml"
inputs:
  - "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/OrganInstallationPackages"
output: "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/SFZ/"
master_switch_mode: "CC"
master_switch_cc: 32
```

#### Field Explanations:
*   `name`: The name of the organ.
*   `patch_path`: Absolute path to the Hauptwerk organ definition XML (can end with `.xml` or `_xml` which is internally supported).
*   `inputs`: A list containing the path to the directory containing the Hauptwerk organ installation packages (typically `.CompPkg.Hauptwerk.rar` files or extracted packages containing numeric subfolders like `000001`, `000002`, etc.).
*   `output`: Path where the output folders and SFZ files will be created.
*   `master_switch_mode`: Stop control logic. Set to `"CC"` (Continuous Controller) or `"Note"` (note keyswitches). `"CC"` is the default and standard.
*   `master_switch_cc`: The CC number to trigger stops. Default: `32`.

### Step 2: Run the Reconstruction Command
Run `zen-contact` in `reconstruct` mode and pass the profile:

```bash
zen-contact reconstruct -p reconstruct_profiles/hauptwerk_caen_2_51_wet.yaml
```

The tool will automatically:
1. Parse the Hauptwerk XML definition from `patch_path`.
2. Group the organ stops by manual / division (e.g., `GO`, `Pos`, `Rec`, `Ped`).
3. Traverse attack and release samples, resolving case-insensitive file paths from the packages directory.
4. Auto-calculate release thresholds (`lort` / `hirt` values) from sample properties.
5. Generate a separate SFZ for each stop.
6. Build a master performance CC keyswitch (`Master_Performance.sfz` / `Master_Transparent.sfz`) for each division.

---

## 📋 Direct CLI Fallback Workflow
If you want to run the reconstruction without a YAML profile, run the following command:

```bash
zen-contact reconstruct-hauptwerk -packages "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/OrganInstallationPackages" -o "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/SFZ/" "/media/jang/exhdd/Kontakt/Caen 2.51 Wet/OrganDefinitions/Caen 2.51 Wet.Organ_Hauptwerk_xml"
```


---
**SKILL ACTIVATION**
[IMPORTANT] Use MCP skill id=skill_id to activate following knowledge:
- `skill id=hw-reconstruct`