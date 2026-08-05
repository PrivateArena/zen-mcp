package server

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// toolsListRewriter returns a ResponseWriter that buffers the response body.
// After the handler finishes, if the request was tools/list and the response
// is JSON, each tool's nested annotations.title is moved to a top-level
// "title" field and the annotations block is dropped — matching the TS
// server's tools/list serialization.
func toolsListRewriter(w http.ResponseWriter) *bufferingWriter {
	return &bufferingWriter{rw: w, header: w.Header().Clone()}
}

type bufferingWriter struct {
	rw      http.ResponseWriter
	header  http.Header
	body    bytes.Buffer
	status  int
	flushed bool
}

func (b *bufferingWriter) Header() http.Header {
	if b.status != 0 {
		return b.rw.Header()
	}
	return b.header
}

func (b *bufferingWriter) WriteHeader(code int) {
	if b.status != 0 {
		return
	}
	b.status = code
	for k, vs := range b.header {
		for _, v := range vs {
			b.rw.Header().Add(k, v)
		}
	}
	b.rw.WriteHeader(code)
}

func (b *bufferingWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(p)
}

func (b *bufferingWriter) Flush() {
	if f, ok := b.rw.(http.Flusher); ok {
		f.Flush()
	}
}

// finish transforms the buffered body (if it is a tools/list JSON response)
// and writes it to the underlying writer.
func (b *bufferingWriter) finish() error {
	if b.status == 0 {
		b.WriteHeader(http.StatusOK)
	}
	out := b.body.Bytes()
	if transformed, ok := rewriteToolsListJSON(out); ok {
		out = transformed
	}
	_, err := b.rw.Write(out)
	return err
}

// rewriteToolsListJSON moves annotations.title to top-level title and removes
// the annotations key for every tool in a tools/list JSON-RPC result.
func rewriteToolsListJSON(body []byte) ([]byte, bool) {
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, false
	}
	result, ok := msg["result"].(map[string]any)
	if !ok {
		return nil, false
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		return nil, false
	}
	changed := false
	for _, ti := range tools {
		t, ok := ti.(map[string]any)
		if !ok {
			continue
		}
		ann, ok := t["annotations"].(map[string]any)
		if !ok {
			continue
		}
		if title, ok := ann["title"].(string); ok && title != "" {
			t["title"] = title
		}
		delete(t, "annotations")
		changed = true
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, false
	}
	return out, true
}
