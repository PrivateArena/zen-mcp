package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jang/zen-mcp/internal/mcpcfg"
)

// ErrBridgeFailure is returned when the bridge responds with HTTP >= 400.
type ErrBridgeFailure struct {
	Status     int
	StatusText string
	Body       string
}

func (e *ErrBridgeFailure) Error() string {
	return fmt.Sprintf("Bridge failure: %d %s - %s", e.Status, e.StatusText, e.Body)
}

// CallBridge POSTs to the Firefox bridge and returns the parsed JSON response.
func CallBridge(ctx context.Context, action string, params map[string]any) (map[string]any, error) {
	base := mcpcfg.FirefoxBridgeURL()
	if base == "" {
		return nil, fmt.Errorf("bridge URL is empty")
	}
	url := base
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	timeoutMs := mcpcfg.GetToolConfig("browser").Timeout
	if timeoutMs <= 0 {
		timeoutMs = 2_400_000
	}
	if t, ok := params["timeout"].(int); ok && t > 0 {
		timeoutMs = t
	}

	sanitized := sanitizeBridgeParams(params)

	reqBody := map[string]any{"action": action}
	for k, v := range sanitized {
		reqBody[k] = v
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("bridge marshal: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	respText := string(respBody)

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &ErrBridgeFailure{
			Status:     resp.StatusCode,
			StatusText: resp.Status,
			Body:       respText,
		}
	}

	var data any
	if err := json.Unmarshal(respBody, &data); err != nil {
		data = respText
	}
	result := sanitizeBridgeResponse(data)
	if m, ok := result.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{"raw": result}, nil
}

func sanitizeBridgeParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		if k == "action" {
			continue
		}
		switch k {
		case "message", "prompt":
			out[k] = sanitizeChatMessage(v)
		default:
			out[k] = v
		}
	}
	return out
}

func sanitizeChatMessage(value any) any {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		return DecodeHTMLEntities(FixMojibake(s))
	}
	if arr, ok := value.([]any); ok {
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, sanitizeChatMessage(item))
		}
		return out
	}
	return value
}

// DecodeHTMLEntities decodes common HTML entities.
func DecodeHTMLEntities(text string) string {
	result := text
	entityMap := map[string]string{
		"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": "\"",
		"&#x27;": "'", "&39;": "'", "&apos;": "'", "&nbsp;": " ",
		"&copy;": "©", "&trade;": "™", "&reg;": "®", "&hellip;": "…",
		"&mdash;": "—", "&ndash;": "–", "&rsquo;": "\u2019",
		"&lsquo;": "\u2018", "&rdquo;": "\u201D", "&ldquo;": "\u201C",
		"&bull;": "•", "&middot;": "·", "&euro;": "€", "&pound;": "£",
		"&yen;": "¥", "&sect;": "§", "&para;": "¶", "&deg;": "°",
		"&plusmn;": "±", "&divide;": "÷", "&times;": "×",
		"&frac14;": "¼", "&frac12;": "½", "&frac34;": "¾",
	}
	for entity, char := range entityMap {
		result = strings.ReplaceAll(result, entity, char)
	}
	re := regexp.MustCompile(`&#(\d+);`)
	result = re.ReplaceAllStringFunc(result, func(m string) string {
		n, _ := strconv.ParseInt(m[2:len(m)-1], 10, 32)
		return string(rune(n))
	})
	re2 := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	result = re2.ReplaceAllStringFunc(result, func(m string) string {
		n, _ := strconv.ParseInt(m[3:len(m)-1], 16, 32)
		return string(rune(n))
	})
	return result
}

// FixMojibake fixes latin1-encoded UTF-8 strings.
func FixMojibake(text string) string {
	if strings.Contains(text, "\u00e2") {
		result, err := decodeLatin1ToUTF8(text)
		if err == nil {
			return result
		}
	}
	return text
}

func decodeLatin1ToUTF8(s string) (string, error) {
	decoded := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		decoded = append(decoded, s[i])
	}
	return string(decoded), nil
}

func sanitizeBridgeResponse(value any) any {
	switch v := value.(type) {
	case string:
		return DecodeHTMLEntities(FixMojibake(v))
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeBridgeResponse(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for kk, vv := range v {
			out[kk] = sanitizeBridgeResponse(vv)
		}
		return out
	default:
		return value
	}
}
