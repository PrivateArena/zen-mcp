package markdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// rawKind classifies a JSON raw value by its leading byte.
type rawKind int

const (
	kindScalar rawKind = iota
	kindString
	kindObject
	kindArray
)

// kindOf is a helper function
func kindOf(raw json.RawMessage) rawKind {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 {
		return kindScalar
	}
	switch s[0] {
	case '"':
		return kindString
	case '{':
		return kindObject
	case '[':
		return kindArray
	default:
		return kindScalar
	}
}

// scalarText is a helper function
func scalarText(raw json.RawMessage) string {
	return string(bytes.TrimSpace(raw))
}

// tokToText is a helper function
func tokToText(tok any) string {
	switch v := tok.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// JSONToMarkdown ports converter.ts jsonToMarkdown: renders a JSON document
// as markdown while preserving key insertion order.
func JSONToMarkdown(raw string) string {
	var lines []string

	var walk func(dec *json.Decoder, depth int) error
	walk = func(dec *json.Decoder, depth int) error {
		pad := strings.Repeat("  ", depth)
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			lines = append(lines, pad+tokToText(tok))
			return nil
		}
		switch delim {
		case '{':
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key := keyTok.(string)
				var val json.RawMessage
				if err := dec.Decode(&val); err != nil {
					return err
				}
				switch kindOf(val) {
				case kindString:
					var s string
					_ = json.Unmarshal(val, &s)
					lines = append(lines, pad+"**"+key+"**: "+s)
				case kindObject, kindArray:
					lines = append(lines, pad+"## "+key)
					if err := walk(json.NewDecoder(bytes.NewReader(val)), depth+1); err != nil {
						return err
					}
				default:
					lines = append(lines, pad+"**"+key+"**: "+scalarText(val))
				}
			}
		case '[':
			for dec.More() {
				var item json.RawMessage
				if err := dec.Decode(&item); err != nil {
					return err
				}
				switch kindOf(item) {
				case kindString:
					var s string
					_ = json.Unmarshal(item, &s)
					lines = append(lines, pad+"- "+s)
				case kindObject, kindArray:
					lines = append(lines, pad+"-")
					if err := walk(json.NewDecoder(bytes.NewReader(item)), depth+1); err != nil {
						return err
					}
				default:
					lines = append(lines, pad+"- "+scalarText(item))
				}
			}
		}
		return nil
	}

	if err := walk(json.NewDecoder(strings.NewReader(raw)), 0); err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
