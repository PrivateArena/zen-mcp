package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"zen-mcp/internal/codegraph"
	"zen-mcp/internal/shared"
)

// SetupLiveGraphRoutes registers the codegraph live viewer routes on the given
// mux. It is called ONLY for the CLI mux (port 2999) in runHTTPServers — never
// through SetupRoutes — so the MCP mux stays free of non-MCP routes.
func SetupLiveGraphRoutes(mux *http.ServeMux, store *shared.Store) {
	mux.HandleFunc("GET /codegraph", serveHTML)
	mux.HandleFunc("GET /d3.v7.min.js", serveD3Asset)
	mux.HandleFunc("GET /api/codegraph/data", func(w http.ResponseWriter, r *http.Request) {
		serveGraphData(w, r, store)
	})
}

// serveHTML writes liveGraphHTML with an explicit text/html content type.
func serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, liveGraphHTML)
}

// serveD3Asset writes the embedded D3.js v7 bundle. Served same-origin from the
// CLI port so the SPA makes zero external requests.
func serveD3Asset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = io.WriteString(w, d3V7MinJS)
}

// serveGraphData resolves the active workspace from the shared store, builds the
// graph payload, and writes it as JSON. Always HTTP 200; any failure is surfaced
// through GraphPayload.Error rather than a non-2xx status.
func serveGraphData(w http.ResponseWriter, r *http.Request, store *shared.Store) {
	w.Header().Set("Content-Type", "application/json")
	payload := GraphPayload{}
	ws, ok := store.Get("workspace-root")
	if !ok || strings.TrimSpace(ws) == "" {
		payload = GraphPayload{Error: "No active workspace. Set one with: zworkspace <path>"}
	} else {
		payload = buildGraphPayload(ws)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// buildGraphPayload opens the workspace's codegraph.db read-only and assembles a
// GraphPayload. Any failure sets the Error field; it never panics and never
// writes to the database.
func buildGraphPayload(workspaceRoot string) GraphPayload {
	payload := GraphPayload{Workspace: workspaceRoot}

	dbPath := filepath.Join(workspaceRoot, ".zenmcp", "codegraph.db")
	if _, err := os.Stat(dbPath); err != nil {
		payload.Error = fmt.Sprintf("cannot open codegraph db: %v", err)
		return payload
	}

	storage, err := codegraph.NewReadOnlyStorage(dbPath)
	if err != nil {
		payload.Error = fmt.Sprintf("cannot open codegraph db: %v", err)
		return payload
	}
	defer storage.Close()

	fileRecs := storage.GetAllFiles()
	nodeRecs, nodeErr := storage.GetAllNodes()
	edgeRecs := storage.GetAllEdgeRecords("", 0)
	importRecs := storage.GetAllImports()
	if nodeErr != nil {
		payload.Error = fmt.Sprintf("cannot read graph: %v", nodeErr)
		return payload
	}

	sort.Slice(fileRecs, func(i, j int) bool { return fileRecs[i].ID < fileRecs[j].ID })

	payload.Files = make([]GraphFile, 0, len(fileRecs))
	fileByID := make(map[int64]GraphFile, len(fileRecs))
	pathToFile := make(map[string]int64, len(fileRecs))
	filesByDir := make(map[string][]int64)
	for _, fr := range fileRecs {
		gf := GraphFile{ID: fr.ID, Path: fr.Path, Language: fr.Language, IsTest: fr.IsTest}
		payload.Files = append(payload.Files, gf)
		fileByID[fr.ID] = gf
		pathToFile[fr.Path] = fr.ID
		dir := filepath.Dir(fr.Path)
		filesByDir[dir] = append(filesByDir[dir], fr.ID)
	}

	payload.Nodes = make([]GraphNode, 0, len(nodeRecs))
	nodeToFile := make(map[int64]int64, len(nodeRecs))
	for _, n := range nodeRecs {
		nodeToFile[n.ID] = n.FileID
		payload.Nodes = append(payload.Nodes, GraphNode{
			ID:        n.ID,
			FileID:    n.FileID,
			Type:      n.Type,
			Name:      n.Name,
			Signature: n.Signature,
			StartLine: n.StartLine,
			EndLine:   n.EndLine,
		})
	}

	payload.Edges = make([]GraphEdge, 0, len(edgeRecs)+len(importRecs))
	fileEdgeSet := make(map[[3]int64]struct{})
	for _, e := range edgeRecs {
		srcFile := nodeToFile[e.SourceID]
		tgtFile := nodeToFile[e.TargetID]
		if srcFile == 0 || tgtFile == 0 {
			continue
		}
		payload.Edges = append(payload.Edges, GraphEdge{
			SourceID: e.SourceID,
			TargetID: e.TargetID,
			Relation: e.Relation,
			Level:    "symbol",
		})
		if srcFile != tgtFile {
			key := [3]int64{srcFile, tgtFile, 1} // 1 = collapsed calls/references
			if _, ok := fileEdgeSet[key]; !ok {
				fileEdgeSet[key] = struct{}{}
				payload.Edges = append(payload.Edges, GraphEdge{
					SourceID: srcFile,
					TargetID: tgtFile,
					Relation: "calls",
					Level:    "file",
				})
			}
		}
	}

	for _, imp := range importRecs {
		tgt := resolveImportTarget(imp.ImportPath, pathToFile, filesByDir)
		if tgt == 0 || tgt == imp.FileID {
			continue
		}
		key := [3]int64{imp.FileID, tgt, 2} // 2 = imports
		if _, ok := fileEdgeSet[key]; ok {
			continue
		}
		fileEdgeSet[key] = struct{}{}
		payload.Edges = append(payload.Edges, GraphEdge{
			SourceID: imp.FileID,
			TargetID: tgt,
			Relation: "imports",
			Level:    "file",
		})
	}

	payload.Stats = GraphStats{
		FileCount: len(fileRecs),
		NodeCount: len(nodeRecs),
		EdgeCount: len(edgeRecs) + len(importRecs),
	}
	return payload
}

// resolveImportTarget maps an import specifier to a file ID using strict,
// deterministic matching: an exact path match first, then any file directly
// inside the imported directory. Relative "./" prefixes and surrounding quotes
// are stripped. Returns 0 when no indexed file matches (e.g. standard-library
// or third-party modules that are not part of the workspace).
func resolveImportTarget(spec string, byPath map[string]int64, filesByDir map[string][]int64) int64 {
	spec = strings.Trim(strings.TrimSpace(spec), `"'`)
	spec = strings.TrimPrefix(spec, "./")
	if spec == "" {
		return 0
	}
	if id, ok := byPath[spec]; ok {
		return id
	}
	if ids := filesByDir[spec]; len(ids) > 0 {
		return ids[0]
	}
	return 0
}

// GraphPayload is the single JSON document the SPA fetches once and renders.
type GraphPayload struct {
	Workspace string      `json:"workspace"`
	Files     []GraphFile `json:"files"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Stats     GraphStats  `json:"stats"`
	Error     string      `json:"error,omitempty"`
}

type GraphFile struct {
	ID       int64  `json:"id"`
	Path     string `json:"path"`
	Language string `json:"language"`
	IsTest   bool   `json:"is_test"`
}

type GraphNode struct {
	ID        int64  `json:"id"`
	FileID    int64  `json:"file_id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Signature string `json:"signature"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type GraphEdge struct {
	SourceID int64  `json:"source_id"`
	TargetID int64  `json:"target_id"`
	Relation string `json:"relation"`
	Level    string `json:"level"`
}

type GraphStats struct {
	FileCount int `json:"file_count"`
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
}

// liveGraphHTML is the full standalone SPA document: it loads D3.js v7 from the
// embedded asset served at /d3.v7.min.js (see embed.go), plus the viewer logic
// (see liveGraphAppJS). Same-origin only — no external requests.
const liveGraphHTML = liveGraphHTMLHead + liveGraphHTMLApp

const liveGraphHTMLHead = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Codegraph Live</title>
  <style>
    body { margin:0; background:#1a1a2e; color:#e0e0e0; font-family:monospace; display:flex; height:100vh; overflow:hidden; }
    #graph { flex:1; width:100%; height:100%; display:block; }
    #sidebar { width:320px; background:#16213e; padding:12px; overflow-y:auto; border-left:1px solid #0f3460; }
    #statusbar { position:fixed; bottom:0; left:0; width:100%; background:#0f3460; padding:4px 12px; font-size:11px; }
    .node-file circle { stroke:#fff; stroke-width:1.5; cursor:pointer; }
    .node-symbol circle { stroke:#aaa; stroke-width:0.8; cursor:pointer; }
    .link { stroke:#334; stroke-opacity:0.6; }
    .link.symbol { stroke:#446; stroke-dasharray:3,3; }
    .link.highlight { stroke:#ffd700; stroke-opacity:1; stroke-width:1.8; }
    #sidebar h3 { color:#00d4ff; margin-top:0; }
    #sidebar .field { margin:4px 0; font-size:12px; }
    #sidebar .label { color:#888; }
  </style>
</head>
<body>
  <svg id="graph"></svg>
  <div id="sidebar"><p style="color:#555">Click a node to inspect.</p></div>
  <div id="statusbar">Loading...</div>
  <script src="/d3.v7.min.js"></script>
  <script>`

const liveGraphHTMLApp = `
// === Constants ===
const LANGUAGE_COLORS = {
  "go":         "#00ADD8",
  "python":     "#3572A5",
  "javascript": "#F1E05A",
  "typescript": "#2B7489",
  "rust":       "#DEA584",
  "java":       "#B07219",
  "c":          "#555555",
  "cpp":        "#F34B7D",
  "ruby":       "#701516",
  "lua":        "#000080",
  "markdown":   "#083FA1",
  "":           "#888888"
};
const MAX_SATELLITES = 20;
const MAX_CROSS_PER_SYMBOL = 12;
const FILE_RADIUS_MIN = 6;
const FILE_RADIUS_MAX = 20;
const SYMBOL_RADIUS = 4;

// === Init ===
fetch('/api/codegraph/data')
  .then(r => r.json())
  .then(payload => {
    if (payload.error) { showError(payload.error); return; }
    updateStatus(payload.workspace + " — " + payload.stats.file_count + " files, " + payload.stats.node_count + " nodes");
    initGraph(payload);
  })
  .catch(err => showError(String(err)));

// === Graph init ===
function initGraph(payload) {
  const symbolsPerFile = {};
  for (const n of payload.nodes) {
    symbolsPerFile[n.file_id] = (symbolsPerFile[n.file_id] || 0) + 1;
  }

  const d3nodes = payload.files.map(f => ({
    id: "f_" + f.id,
    label: basename(f.path),
    language: f.language,
    _type: "file",
    _fileId: f.id,
    _data: f,
    r: clamp(FILE_RADIUS_MIN + Math.log2((symbolsPerFile[f.id] || 0) + 1) * 2, FILE_RADIUS_MIN, FILE_RADIUS_MAX),
    expanded: false,
    pinned: false,
    fx: null, fy: null
  }));

  const fileEdges = payload.edges.filter(e => e.level === "file");
  const d3links = fileEdges.map(e => ({
    source: "f_" + e.source_id,
    target: "f_" + e.target_id,
    relation: e.relation,
    level: "file"
  }));

  const svg = d3.select("#graph");
  const width = svg.node().clientWidth;
  const height = svg.node().clientHeight;

  const g = svg.append("g");
  svg.call(d3.zoom().on("zoom", ev => g.attr("transform", ev.transform)));

  const sim = d3.forceSimulation(d3nodes)
    .alphaDecay(0.06)
    .force("link", d3.forceLink(d3links).id(d => d.id).distance(300).strength(0.4))
    .force("charge", d3.forceManyBody().strength(-180))
    .force("center", d3.forceCenter(width / 2, height / 2).strength(0.02))
    .force("collide", d3.forceCollide(d => d.r + 4))
    .force("ring", ringForce);

  let linkSel = g.append("g").selectAll("line");
  let nodeSel = g.append("g").selectAll("g");

  function update(nodes, links, heat) {
    sim.nodes(nodes);
    sim.force("link").links(links);
    sim.alpha(heat).restart();

    linkSel = linkSel.data(links, d => d.source.id + "-" + d.target.id + "-" + d.relation)
      .join("line")
      .attr("class", d => "link" + (d.level === "symbol" ? " symbol" : ""));

    nodeSel = nodeSel.data(nodes, d => d.id)
      .join(enter => {
        const grp = enter.append("g")
          .attr("class", d => "node-" + d._type)
          .call(d3.drag()
            .on("start", (ev, d) => {
              if (!ev.active) sim.alphaTarget(0);
              d._ringParent = null; // a manually dragged node leaves its ring slot
              d.fx = d.x; d.fy = d.y;
              sim.alpha(0.08).restart();
              highlightConnectedLinks(d);
            })
            .on("drag", (ev, d) => {
              d.fx = ev.x; d.fy = ev.y;
              sim.alpha(0.1).restart(); // enough pull for connected nodes to follow
            })
            .on("end", (ev, d) => {
              if (!ev.active) sim.alphaTarget(0);
              if (!d.pinned) { d.fx = null; d.fy = null; }
              clearLinkHighlights();
            }));
        grp.append("circle")
          .attr("r", d => d.r)
          .attr("fill", d => d._type === "file" ? (LANGUAGE_COLORS[d.language || ""] || "#888888") : "#446");
        grp.append("text")
          .attr("dy", d => d.r + 10)
          .attr("text-anchor", "middle")
          .attr("font-size", 9)
          .attr("fill", "#ccc")
          .text(d => d.label);
        grp.on("click", (ev, d) => {
          ev.stopPropagation();
          if (d._type === "file") { handleFileClick(d, nodes, links, update); }
          else if (d._type === "symbol") { showNodeSidebar(d); }
        });
        return grp;
      });

    sim.on("tick", () => {
      linkSel.attr("x1", d => d.source.x).attr("y1", d => d.source.y)
             .attr("x2", d => d.target.x).attr("y2", d => d.target.y);
      nodeSel.attr("transform", d => "translate(" + d.x + "," + d.y + ")");
    });
  }

  update(d3nodes, d3links, 0.7);

  // Holds each symbol satellite near its assigned ring slot around the file
  // node, recomputed per tick so the whole ring travels when the file node is
  // dragged. Manual drags null out _ringParent, so a symbol stays where dropped.
  function ringForce(alpha) {
    const nodes = sim.nodes();
    for (let i = 0; i < nodes.length; i++) {
      const n = nodes[i];
      if (!n._ringParent) continue;
      const tx = n._ringParent.x + n._ringRadius * Math.cos(n._ringAngle);
      const ty = n._ringParent.y + n._ringRadius * Math.sin(n._ringAngle);
      n.vx += (tx - n.x) * 0.06 * alpha;
      n.vy += (ty - n.y) * 0.06 * alpha;
    }
  }

  function highlightConnectedLinks(d) {
    linkSel.classed("highlight", l => l.source.id === d.id || l.target.id === d.id);
  }

  function clearLinkHighlights() {
    linkSel.classed("highlight", false);
  }

  function handleFileClick(fileNode, allNodes, allLinks, updateFn) {
    if (fileNode.expanded) {
      // Collapse: drop satellites and any link touching them, but keep the file
      // node pinned where the user placed it — releasing it back into the
      // simulation would let the center force pull it away from its spot.
      fileNode.expanded = false;
      fileNode.pinned = true;
      const keepNodes = allNodes.filter(n => n._parentFileId !== fileNode._fileId);
      const satelliteIds = {};
      allNodes.forEach(n => { if (n._parentFileId === fileNode._fileId) satelliteIds[n.id] = true; });
      const keepLinks = allLinks.filter(l => !(satelliteIds[l.source.id] || satelliteIds[l.target.id]));
      updateFn(keepNodes, keepLinks, 0.08);
    } else {
      // Expand: pin the parent and reveal up to MAX_SATELLITES symbols.
      fileNode.expanded = true;
      fileNode.pinned = true;
      fileNode.fx = fileNode.x; fileNode.fy = fileNode.y;
      const myNodes = payload.nodes.filter(n => n.file_id === fileNode._fileId);
      const shown = myNodes.slice(0, MAX_SATELLITES);
      const overflow = myNodes.length - shown.length;

      // Organized layout: symbols on an evenly-spaced ring around the file
      // node (no random scatter), so the expanded view stays readable.
      const count = shown.length;
      const radius = clamp(16 + count * 2.5, 20, 90);
      const baseAngle = -Math.PI / 2;
      const step = count > 0 ? (2 * Math.PI) / count : 0;

      const satellites = shown.map((n, i) => ({
        id: "n_" + n.id,
        label: n.name,
        _type: "symbol",
        _nodeId: n.id,
        _parentFileId: fileNode._fileId,
        _data: n,
        r: SYMBOL_RADIUS,
        _ringParent: fileNode,
        _ringAngle: baseAngle + i * step,
        _ringRadius: radius,
        x: fileNode.x + radius * Math.cos(baseAngle + i * step),
        y: fileNode.y + radius * Math.sin(baseAngle + i * step)
      }));

      if (overflow > 0) {
        satellites.push({
          id: "overflow_" + fileNode._fileId,
          label: "+" + overflow + " more",
          _type: "overflow",
          _parentFileId: fileNode._fileId,
          r: SYMBOL_RADIUS,
          x: fileNode.x, y: fileNode.y + radius + 24
        });
      }

      const satLinks = satellites
        .filter(s => s._type === "symbol")
        .map(s => ({ source: fileNode.id, target: s.id, relation: "contains", level: "file" }));

      const satIds = {};
      satellites.forEach(s => { if (s._type === "symbol") satIds[s._nodeId] = true; });
      const symLinks = payload.edges
        .filter(e => e.level === "symbol" && satIds[e.source_id] && satIds[e.target_id])
        .map(e => ({ source: "n_" + e.source_id, target: "n_" + e.target_id, relation: e.relation, level: "symbol" }));

      // Cross-file usage links: for each spawned symbol, show links to the file
      // nodes that use it (or that it uses), mirroring codegraph "related". Weak
      // per-link strength so symbols stay on their ring while still hinting at
      // their consumers.
      const nodeFile = {};
      payload.nodes.forEach(n => { nodeFile[n.id] = n.file_id; });
      const crossSeen = {};
      const crossCount = {};
      const crossLinks = [];
      for (const e of payload.edges) {
        if (e.level !== "symbol") continue;
        let satNode = 0, otherNode = 0;
        if (satIds[e.source_id]) { satNode = e.source_id; otherNode = e.target_id; }
        else if (satIds[e.target_id]) { satNode = e.target_id; otherNode = e.source_id; }
        if (satNode === 0) continue;
        const otherFile = nodeFile[otherNode];
        if (!otherFile || otherFile === fileNode._fileId) continue;
        const key = "n_" + satNode + "-f_" + otherFile;
        if (crossSeen[key]) continue;
        if ((crossCount[satNode] || 0) >= MAX_CROSS_PER_SYMBOL) continue;
        crossSeen[key] = true;
        crossCount[satNode] = (crossCount[satNode] || 0) + 1;
        crossLinks.push({
          source: "n_" + satNode,
          target: "f_" + otherFile,
          relation: e.relation,
          level: "symbol",
          strength: 0.15
        });
      }

      updateFn([...allNodes, ...satellites], [...allLinks, ...satLinks, ...symLinks, ...crossLinks], 0.08);
    }
  }
}

// === Sidebar ===
function showNodeSidebar(d) {
  const data = d._data;
  if (!data) return;
  const name = data.name || data.path || "";
  const type = data.type || "file";
  const path = data.path || "";
  const sig = data.signature || "";
  const lines = (data.start_line ? data.start_line : "") + "–" + (data.end_line ? data.end_line : "");
  document.getElementById("sidebar").innerHTML =
    "<h3>" + esc(name) + "</h3>" +
    "<div class=\"field\"><span class=\"label\">Type:</span> " + esc(type) + "</div>" +
    "<div class=\"field\"><span class=\"label\">File:</span> " + esc(path) + "</div>" +
    "<div class=\"field\"><span class=\"label\">Signature:</span><br><code>" + esc(sig) + "</code></div>" +
    "<div class=\"field\"><span class=\"label\">Lines:</span> " + esc(lines) + "</div>";
}

// === Helpers ===
function basename(p) { return String(p).split("/").pop(); }
function clamp(v, min, max) { return Math.max(min, Math.min(max, v)); }
function esc(s) { return String(s).replace(/[&<>"]/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c])); }
function showError(msg) {
  document.getElementById("statusbar").textContent = "Error: " + msg;
  document.getElementById("sidebar").innerHTML = "<p style=\"color:#f66\">" + esc(msg) + "</p>";
}
function updateStatus(msg) { document.getElementById("statusbar").textContent = msg; }
</script>
</body>
</html>`
