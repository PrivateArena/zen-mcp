package whiteboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	httpTimeoutMs    = 5000
	cardContentLimit = 40 * 1024
)

// CardData mirrors whiteboard card shape.
type CardData struct {
	CardSlug    string
	Title       string
	Content     string
	Description string
	Group       string
	CreatedAt   string
}

// BoardState mirrors whiteboard board state.
type BoardState struct {
	Slug        string
	Title       string
	Cards       []CardData
	Connections []struct {
		ID       string
		From     string
		FromPort string
		To       string
		ToPort   string
	}
	Sections []struct {
		ID         string
		Label      string
		X, Y, W, H int
		Color      string
	}
	Viewport struct {
		X, Y  float64
		Scale float64
	}
}

// BoardSummary is a minimal board descriptor.
type BoardSummary struct {
	Slug  string
	Title string
}

// Client is the whiteboard HTTP client.
type Client struct {
	BaseURL string
	Slug    string
	Title   string
	Owner   string
	HTTP    *http.Client
}

// NewClient creates a whiteboard client.
func NewClient(baseURL, slug, title, owner string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		Slug:    slug,
		Title:   title,
		Owner:   owner,
		HTTP:    &http.Client{Timeout: time.Duration(httpTimeoutMs) * time.Millisecond},
	}
}

func (c *Client) apiURL(path string) string {
	return fmt.Sprintf("%s/_zen/api%s", c.BaseURL, path)
}

func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL(path), nil)
	if err != nil {
		return nil, err
	}
	return c.HTTP.Do(req)
}

func (c *Client) doPost(ctx context.Context, path string, body any) (*http.Response, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.HTTP.Do(req)
}

// EnsureBoard creates the board if it does not exist.
func (c *Client) EnsureBoard(ctx context.Context) error {
	listRes, err := c.doGet(ctx, "/whiteboard/list")
	if err != nil {
		return err
	}
	defer listRes.Body.Close()
	if listRes.StatusCode == http.StatusOK {
		var listData struct {
			Whiteboards []BoardSummary `json:"whiteboards"`
		}
		if err := json.NewDecoder(listRes.Body).Decode(&listData); err == nil {
			for _, wb := range listData.Whiteboards {
				if wb.Slug == c.Slug {
					return nil
				}
			}
		}
	}
	createRes, err := c.doPost(ctx, "/whiteboard/create", map[string]any{
		"slug":  c.Slug,
		"title": c.Title,
	})
	if err != nil {
		return err
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK && createRes.StatusCode != http.StatusCreated {
		txt, _ := io.ReadAll(createRes.Body)
		return fmt.Errorf("whiteboard create failed: %s", string(txt))
	}
	return nil
}

// UpsertCard adds or updates a card.
func (c *Client) UpsertCard(ctx context.Context, slug, title, content, group string) error {
	if len(content) > cardContentLimit {
		content = content[:cardContentLimit] + "\n\n…[truncated]"
	}
	body := map[string]any{
		"whiteboard_slug": c.Slug,
		"card_slug":       slug,
		"title":           title,
		"content":         content,
		"auto_layout":     true,
		"row_wrap":        4,
	}
	if group != "" {
		body["group"] = group
	}
	res, err := c.doPost(ctx, "/whiteboard/add_card", body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		txt, _ := io.ReadAll(res.Body)
		return fmt.Errorf("whiteboard add_card failed: %s", string(txt))
	}
	return nil
}

// LoadBoardState returns the full board state.
func (c *Client) LoadBoardState(ctx context.Context) (BoardState, error) {
	state := BoardState{Slug: c.Slug, Title: c.Title}
	qPath := fmt.Sprintf("/whiteboard/state?slug=%s&include_content=true", url.QueryEscape(c.Slug))
	res, err := c.doGet(ctx, qPath)
	if err != nil {
		return state, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return state, nil
	}
	if res.StatusCode != http.StatusOK {
		txt, _ := io.ReadAll(res.Body)
		return state, fmt.Errorf("whiteboard state failed: %s", string(txt))
	}
	var raw struct {
		Cards       []CardData `json:"cards"`
		Connections []struct {
			ID       string
			From     string
			FromPort string
			To       string
			ToPort   string
		} `json:"connections"`
		Sections []struct {
			ID         string
			Label      string
			X, Y, W, H int
			Color      string
		} `json:"sections"`
		Viewport map[string]any `json:"viewport"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return state, err
	}
	state.Cards = raw.Cards
	state.Connections = raw.Connections
	state.Sections = raw.Sections
	if raw.Viewport != nil {
		if x, ok := raw.Viewport["x"].(float64); ok {
			state.Viewport.X = x
		}
		if y, ok := raw.Viewport["y"].(float64); ok {
			state.Viewport.Y = y
		}
		if s, ok := raw.Viewport["scale"].(float64); ok {
			state.Viewport.Scale = s
		}
	}
	return state, nil
}

// Save persists board state (alias for UpsertCard + ensure).
func (c *Client) Save(ctx context.Context) error {
	if err := c.EnsureBoard(ctx); err != nil {
		return err
	}
	return nil
}

// LinkCards creates a link between two cards.
func (c *Client) LinkCards(ctx context.Context, fromCard, toCard string) error {
	res, err := c.doPost(ctx, "/whiteboard/link_cards", map[string]any{
		"whiteboard_slug": c.Slug,
		"from_card":       fromCard,
		"to_card":         toCard,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		txt, _ := io.ReadAll(res.Body)
		return fmt.Errorf("link_cards failed: %s", string(txt))
	}
	return nil
}

// Scope is a placeholder for scoped queries (returns board state).
func (c *Client) Scope(ctx context.Context, scope string) (BoardState, error) {
	return c.LoadBoardState(ctx)
}
