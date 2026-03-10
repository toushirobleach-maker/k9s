// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dao

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
)

const logHeaderTimestampWidth = 28
const jsonFieldColorTag = "aqua"
const prettyLogIndent = 2

// LogChan represents a channel for logs.
type LogChan chan *LogItem

var ItemEOF = new(LogItem)

// LogItem represents a container log line.
type LogItem struct {
	Pod, Container  string
	SingleContainer bool
	Bytes           []byte
	IsError         bool
}

// NewLogItem returns a new item.
func NewLogItem(bb []byte) *LogItem {
	return &LogItem{
		Bytes: bb,
	}
}

// NewLogItemFromString returns a new item.
func NewLogItemFromString(s string) *LogItem {
	return &LogItem{
		Bytes: []byte(s),
	}
}

// ID returns pod and or container based id.
func (l *LogItem) ID() string {
	if l.Pod != "" {
		return l.Pod
	}
	return l.Container
}

// GetTimestamp fetch log lime timestamp
func (l *LogItem) GetTimestamp() string {
	index := bytes.Index(l.Bytes, []byte{' '})
	if index < 0 {
		return ""
	}
	return string(l.Bytes[:index])
}

// Info returns pod and container information.
func (l *LogItem) Info() string {
	return l.Pod + "::" + l.Container
}

// IsEmpty checks if the entry is empty.
func (l *LogItem) IsEmpty() bool {
	return len(l.Bytes) == 0
}

// Size returns the size of the item.
func (l *LogItem) Size() int {
	return 100 + len(l.Bytes) + len(l.Pod) + len(l.Container)
}

// Render returns a log line as string.
func (l *LogItem) Render(paint string, showTime bool, prettyJSON bool, prettyAll bool, prettyFields map[string]struct{}, bb *bytes.Buffer) {
	index := bytes.Index(l.Bytes, []byte{' '})
	var ts []byte
	if index > 0 {
		ts = l.Bytes[:index]
	}
	if prettyJSON {
		l.renderPrettyJSON(paint, ts, index, showTime, prettyAll, prettyFields, bb)
		return
	}

	l.renderPlain(paint, ts, index, showTime, bb)
}

func (l *LogItem) renderPlain(paint string, ts []byte, index int, showTime bool, bb *bytes.Buffer) {
	showTimeEffective := showTime
	if showTimeEffective && index > 0 {
		bb.WriteString("[gray::b]")
		bb.Write(ts)
		bb.WriteString(" ")
		if l := logHeaderTimestampWidth - len(ts); l > 0 {
			bb.Write(bytes.Repeat([]byte{' '}, l))
		}
		bb.WriteString("[-::-]")
	}

	if l.Pod != "" {
		bb.WriteString("[" + paint + "::]" + l.Pod)
	}

	if !l.SingleContainer && l.Container != "" {
		if l.Pod != "" {
			bb.WriteString(" ")
		}
		bb.WriteString("[" + paint + "::b]" + l.Container + "[-::-] ")
	} else if l.Pod != "" {
		bb.WriteString("[-::] ")
	}

	payload := l.Bytes
	if index > 0 {
		payload = l.Bytes[index+1:]
	}
	bb.Write(payload)
}

func (l *LogItem) renderPrettyJSON(paint string, ts []byte, index int, showTime bool, prettyAll bool, prettyFields map[string]struct{}, bb *bytes.Buffer) {
	hasHeader := false
	if showTime && len(ts) > 0 {
		bb.WriteString("[gray::b]")
		bb.Write(ts)
		bb.WriteString("[-::-]")
		hasHeader = true
	}

	if l.Pod != "" {
		if hasHeader {
			bb.WriteByte(' ')
		}
		bb.WriteString("[" + paint + "::]" + l.Pod + "[-::]")
		hasHeader = true
	}

	if !l.SingleContainer && l.Container != "" {
		if hasHeader {
			bb.WriteByte(' ')
		}
		bb.WriteString("[" + paint + "::b]" + l.Container + "[-::-]")
		hasHeader = true
	}

	payload := l.Bytes
	if index > 0 {
		payload = l.Bytes[index+1:]
	}
	if pretty, ok := prettyJSONBytes(payload, prettyAll, prettyFields); ok {
		if len(pretty) == 0 {
			if hasHeader {
				bb.WriteByte('\n')
			}
			return
		}
		if hasHeader {
			bb.WriteByte('\n')
		}
		bb.Write(indentAllLines(pretty, prettyLogIndent))
		return
	}

	if hasHeader {
		if bytes.Equal(payload, []byte{'\n'}) {
			bb.Write(payload)
			return
		}
		if len(payload) > 0 {
			bb.Write(bytes.Repeat([]byte{' '}, prettyLogIndent))
		}
	}
	bb.Write(payload)
}

var jsonKeyRx = regexp.MustCompile(`(?m)^(\s*)\"([^\"]+)\":`)

func prettyJSONBytes(in []byte, all bool, fields map[string]struct{}) ([]byte, bool) {
	if len(in) == 0 {
		return nil, false
	}
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return nil, false
	}
	if len(trimmed) > 64*1024 {
		return nil, false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return nil, false
	}
	if !all && len(fields) == 0 {
		all = true
	}
	var pretty []byte
	if all {
		var out bytes.Buffer
		if err := json.Indent(&out, trimmed, "", "  "); err != nil {
			return nil, false
		}
		pretty = out.Bytes()
	} else {
		var v any
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		var found bool
		v, found = filterJSON(v, fields)
		if !found {
			return []byte{}, true
		} else {
			out, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return nil, false
			}
			pretty = out
		}
	}
	if trimmed[0] == '{' {
		pretty = stripOuterObjectBraces(pretty)
	}
	pretty = jsonKeyRx.ReplaceAll(pretty, []byte(`${1}[`+jsonFieldColorTag+`::b]"$2":[-::]`))
	if in[len(in)-1] == '\n' {
		pretty = append(pretty, '\n')
	}
	return pretty, true
}

func filterJSON(v any, fields map[string]struct{}) (any, bool) {
	if len(fields) == 0 {
		return v, true
	}
	switch t := v.(type) {
	case map[string]any:
		res := make(map[string]any, len(fields))
		found := false
		for k, v := range t {
			if _, ok := fields[k]; ok {
				res[k] = v
				found = true
			}
		}
		return res, found
	case []any:
		res := make([]any, 0, len(t))
		found := false
		for _, e := range t {
			switch et := e.(type) {
			case map[string]any:
				filtered, ok := filterJSON(et, fields)
				if ok {
					found = true
				}
				res = append(res, filtered)
			default:
				res = append(res, e)
			}
		}
		return res, found
	default:
		return v, false
	}
}

// ExtractJSONKeys returns sorted top-level JSON keys from a log payload.
func ExtractJSONKeys(in []byte) []string {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return nil
	}
	if len(trimmed) > 64*1024 {
		return nil
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return nil
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return nil
	}
	keys := make(map[string]struct{})
	switch t := v.(type) {
	case map[string]any:
		for k := range t {
			keys[k] = struct{}{}
		}
	case []any:
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				for k := range m {
					keys[k] = struct{}{}
				}
			}
		}
	default:
		return nil
	}
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func indentAllLines(in []byte, indent int) []byte {
	if indent <= 0 || len(in) == 0 {
		return in
	}
	pad := bytes.Repeat([]byte{' '}, indent)
	lines := bytes.Split(in, []byte{'\n'})
	var out bytes.Buffer
	out.Grow(len(in) + indent*len(lines))
	for i := 0; i < len(lines); i++ {
		if i > 0 {
			out.WriteByte('\n')
		}
		// Preserve a trailing newline without carrying indentation into the next log item.
		if i == len(lines)-1 && len(lines[i]) == 0 {
			continue
		}
		out.Write(pad)
		out.Write(lines[i])
	}
	return out.Bytes()
}

func stripOuterObjectBraces(in []byte) []byte {
	lines := bytes.Split(in, []byte{'\n'})
	if len(lines) == 0 {
		return in
	}
	start := 0
	end := len(lines)
	for start < end && len(bytes.TrimSpace(lines[start])) == 0 {
		start++
	}
	for end > start && len(bytes.TrimSpace(lines[end-1])) == 0 {
		end--
	}
	if end-start < 2 {
		return in
	}
	if !bytes.Equal(bytes.TrimSpace(lines[start]), []byte{'{'}) {
		return in
	}
	if !bytes.Equal(bytes.TrimSpace(lines[end-1]), []byte{'}'}) {
		return in
	}
	lines = lines[start+1 : end-1]
	for i := range lines {
		if bytes.HasPrefix(lines[i], []byte("  ")) {
			lines[i] = lines[i][2:]
		}
	}
	if len(lines) == 0 {
		return []byte{}
	}
	return bytes.Join(lines, []byte{'\n'})
}
