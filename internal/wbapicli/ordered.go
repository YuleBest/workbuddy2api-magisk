package wbapicli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type orderedPair struct {
	key   string
	value any
}

type orderedMap struct {
	pairs []orderedPair
}

func decodeOrdered(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	if token, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value %v", token)
		}
		return nil, err
	}
	return value, nil
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		obj := orderedMap{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			obj.pairs = append(obj.pairs, orderedPair{key: key, value: value})
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return obj, nil
	case '[':
		var values []any
		for dec.More() {
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func (o orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCompactOrdered(&buf, o); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCompactOrdered(buf *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case orderedMap:
		buf.WriteByte('{')
		for i, pair := range typed.pairs {
			key, err := marshalJSONScalar(pair.key)
			if err != nil {
				return err
			}
			buf.Write(key)
			buf.WriteByte(':')
			if err := writeCompactOrdered(buf, pair.value); err != nil {
				return err
			}
			if i+1 < len(typed.pairs) {
				buf.WriteByte(',')
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range typed {
			if err := writeCompactOrdered(buf, item); err != nil {
				return err
			}
			if i+1 < len(typed) {
				buf.WriteByte(',')
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		raw, err := marshalJSONScalar(value)
		if err != nil {
			return err
		}
		buf.Write(raw)
		return nil
	}
}

func redactOrderedLocal(value any, path ...string) any {
	switch typed := value.(type) {
	case orderedMap:
		out := orderedMap{pairs: make([]orderedPair, 0, len(typed.pairs))}
		for _, pair := range typed.pairs {
			childPath := append(append([]string(nil), path...), pair.key)
			if shouldRedactPath(childPath) && orderedTruthy(pair.value) {
				out.pairs = append(out.pairs, orderedPair{key: pair.key, value: "***"})
				continue
			}
			out.pairs = append(out.pairs, orderedPair{
				key:   pair.key,
				value: redactOrderedLocal(pair.value, childPath...),
			})
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactOrderedLocal(item, path...)
		}
		return out
	default:
		return value
	}
}

func shouldRedactPath(path []string) bool {
	if len(path) == 1 && path[0] == "api_key" {
		return true
	}
	if len(path) != 2 {
		return false
	}
	switch path[0] {
	case "upstash":
		return path[1] == "url" || path[1] == "token"
	case "admin":
		return path[1] == "token"
	case "upstream":
		return path[1] == "device_token"
	default:
		return false
	}
}

func orderedTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case json.Number:
		n, _ := typed.Float64()
		return n != 0
	case []any:
		return len(typed) > 0
	case orderedMap:
		return len(typed.pairs) > 0
	default:
		return true
	}
}

func writeIndentedJSON(w io.Writer, raw []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}
