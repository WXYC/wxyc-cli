package cmd

import (
	"bytes"
	"encoding/json"
)

// combineRaw wraps several raw server bodies in one JSON value for --json.
// Unlike json.Marshal it leaves "&", "<" and ">" as the server sent them.
func combineRaw(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
