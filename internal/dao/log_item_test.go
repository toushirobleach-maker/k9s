// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dao_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/derailed/k9s/internal/client"
	"github.com/derailed/k9s/internal/dao"
	"github.com/derailed/tview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogItemEmpty(t *testing.T) {
	uu := map[string]struct {
		s string
		e bool
	}{
		"empty": {s: "", e: true},
		"full":  {s: "Testing 1,2,3..."},
	}

	for k := range uu {
		u := uu[k]
		t.Run(k, func(t *testing.T) {
			i := dao.NewLogItemFromString(u.s)
			assert.Equal(t, u.e, i.IsEmpty())
		})
	}
}

func TestLogItemRender(t *testing.T) {
	uu := map[string]struct {
		opts dao.LogOptions
		log  string
		e    string
	}{
		"empty": {
			opts: dao.LogOptions{},
			log:  fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."),
			e:    "Testing 1,2,3...\n",
		},
		"container": {
			opts: dao.LogOptions{
				Container: "fred",
			},
			log: fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."),
			e:   "[yellow::b]fred[-::-] Testing 1,2,3...\n",
		},
		"pod": {
			opts: dao.LogOptions{
				Path:            "blee/fred",
				Container:       "blee",
				SingleContainer: true,
			},
			log: fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."),
			e:   "[yellow::]fred [yellow::b]blee[-::-] Testing 1,2,3...\n",
		},
		"full": {
			opts: dao.LogOptions{
				Path:            "blee/fred",
				Container:       "blee",
				SingleContainer: true,
				ShowTimestamp:   true,
			},
			log: fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."),
			e:   "[gray::b]2018-12-14T10:36:43.326972-07:00 [-::-][yellow::]fred [yellow::b]blee[-::-] Testing 1,2,3...\n",
		},
		"log-level": {
			opts: dao.LogOptions{
				Path:            "blee/fred",
				Container:       "",
				SingleContainer: false,
				ShowTimestamp:   false,
			},
			log: fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "2021-10-28T13:06:37Z [INFO] [blah-blah] Testing 1,2,3..."),
			e:   "[yellow::]fred[-::] 2021-10-28T13:06:37Z [INFO[] [blah-blah[] Testing 1,2,3...\n",
		},
		"escape": {
			opts: dao.LogOptions{
				Path:            "blee/fred",
				Container:       "",
				SingleContainer: false,
				ShowTimestamp:   false,
			},
			log: fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", `{"foo":["bar"]} Server listening on: [::]:5000`),
			e:   `[yellow::]fred[-::] {"foo":["bar"[]} Server listening on: [::[]:5000` + "\n",
		},
	}

	for k := range uu {
		u := uu[k]
		t.Run(k, func(t *testing.T) {
			i := dao.NewLogItem([]byte(tview.Escape(u.log)))
			_, n := client.Namespaced(u.opts.Path)
			i.Pod, i.Container = n, u.opts.Container

			bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
			i.Render("yellow", u.opts.ShowTimestamp, u.opts.PrettyJSON, true, nil, bb)
			assert.Equal(t, u.e, bb.String())
		})
	}
}

func BenchmarkLogItemRenderTS(b *testing.B) {
	s := []byte(fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."))
	i := dao.NewLogItem(s)
	i.Pod, i.Container = "fred", "blee"

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
		i.Render("yellow", true, false, true, nil, bb)
	}
}

func BenchmarkLogItemRenderNoTS(b *testing.B) {
	s := []byte(fmt.Sprintf("%s %s\n", "2018-12-14T10:36:43.326972-07:00", "Testing 1,2,3..."))
	i := dao.NewLogItem(s)
	i.Pod, i.Container = "fred", "blee"

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
		i.Render("yellow", false, false, true, nil, bb)
	}
}

func TestLogItemPrettyJSON(t *testing.T) {
	ts := "2024-01-01T00:00:00Z"
	log := fmt.Sprintf("%s %s\n", ts, `{"foo":1,"bar":{"baz":"x"},"list":[1,2]}`)
	uu := map[string]bool{
		"timestamp-enabled":  true,
		"timestamp-disabled": false,
	}

	for name, showTime := range uu {
		t.Run(name, func(t *testing.T) {
			i := dao.NewLogItem([]byte(log))
			i.Pod, i.Container = "pod", "c1"

			bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
			i.Render("yellow", showTime, true, true, nil, bb)
			out := bb.String()
			lines := strings.Split(out, "\n")

			if showTime {
				assert.Equal(t, 1, strings.Count(out, ts))
				assert.Contains(t, lines[0], ts)
			} else {
				assert.Equal(t, 0, strings.Count(out, ts))
				assert.NotContains(t, lines[0], ts)
			}
			require.GreaterOrEqual(t, len(lines), 3)
			assert.Contains(t, lines[0], "pod")
			assert.Contains(t, lines[0], "c1")
			bodyIndent := strings.Repeat(" ", 2)
			assert.True(t, strings.HasPrefix(lines[1], bodyIndent+`[aqua::b]"foo":[-::] 1`))
			assert.Contains(t, out, bodyIndent+`[aqua::b]"foo":[-::] 1`)
			assert.Contains(t, out, bodyIndent+`  [aqua::b]"baz":[-::] "x"`)
			assert.Contains(t, out, bodyIndent+`[aqua::b]"list":[-::] [`)
			first := ""
			last := ""
			for _, line := range lines[1:] {
				tl := strings.TrimSpace(line)
				if tl == "" {
					continue
				}
				if first == "" {
					first = tl
				}
				last = tl
			}
			assert.NotEqual(t, "{", first)
			assert.NotEqual(t, "}", last)
		})
	}
}

func TestLogItemPrettyJSONFieldsFilter(t *testing.T) {
	log := fmt.Sprintf("%s %s\n", "2024-01-01T00:00:00Z", `{"foo":1,"bar":2}`)
	i := dao.NewLogItem([]byte(log))
	i.Pod, i.Container = "pod", "c1"

	bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
	fields := map[string]struct{}{"foo": {}}
	i.Render("yellow", true, true, false, fields, bb)
	out := bb.String()
	bodyIndent := strings.Repeat(" ", 2)

	assert.Contains(t, out, bodyIndent+`[aqua::b]"foo":[-::] 1`)
	assert.NotContains(t, out, `"bar"`)
}

func TestLogItemPrettyJSONEmptyFieldSelectionFallsBackToAll(t *testing.T) {
	log := fmt.Sprintf("%s %s\n", "2024-01-01T00:00:00Z", `{"foo":1,"bar":2}`)
	i := dao.NewLogItem([]byte(log))
	i.Pod, i.Container = "pod", "c1"

	bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
	i.Render("yellow", true, true, false, map[string]struct{}{}, bb)
	out := bb.String()
	bodyIndent := strings.Repeat(" ", 2)

	assert.Contains(t, out, bodyIndent+`[aqua::b]"foo":[-::] 1`)
	assert.Contains(t, out, bodyIndent+`[aqua::b]"bar":[-::] 2`)
}

func TestLogItemPrettyJSONMissingSelectedFieldsShowsHeaderOnly(t *testing.T) {
	log := fmt.Sprintf("%s %s\n", "2024-01-01T00:00:00Z", `{"bar":2}`)
	i := dao.NewLogItem([]byte(log))
	i.Pod, i.Container = "pod", "c1"

	bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
	i.Render("yellow", true, true, false, map[string]struct{}{"foo": {}}, bb)
	out := bb.String()

	assert.Contains(t, out, "2024-01-01T00:00:00Z")
	assert.Contains(t, out, "pod")
	assert.Contains(t, out, "c1")
	assert.NotContains(t, out, `[aqua::b]"bar":[-::] 2`)
	assert.NotContains(t, out, `[aqua::b]"foo":[-::]`)
	assert.Equal(t, 1, strings.Count(out, "\n"))
}

func TestLogItemPrettyJSONKeepsNonJSONLinesInline(t *testing.T) {
	log := "2026-03-10T18:01:45.380422837+03:00 :: Spring Boot ::               (v3.2.12)\n"
	i := dao.NewLogItem([]byte(log))

	bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
	i.Render("yellow", false, true, false, map[string]struct{}{"message": {}}, bb)

	assert.Equal(t, ":: Spring Boot ::               (v3.2.12)\n", bb.String())
}

func TestLogItemPrettyJSONKeepsEmptyLinesAsHeaderOnly(t *testing.T) {
	log := "2026-03-10T18:01:44.780598821+03:00 \n"
	i := dao.NewLogItem([]byte(log))

	bb := bytes.NewBuffer(make([]byte, 0, i.Size()))
	i.Render("yellow", false, true, false, map[string]struct{}{"message": {}}, bb)

	assert.Equal(t, "\n", bb.String())
}

func TestLogItemPrettyJSONDoesNotIndentTrailingNewline(t *testing.T) {
	first := dao.NewLogItem([]byte("2026-03-11T20:02:12.235852992Z {\"message\":\"json tick 38\"}\n"))
	second := dao.NewLogItem([]byte("2026-03-11T20:02:12.235906752Z plain tick 38\n"))

	bb := bytes.NewBuffer(nil)
	first.Render("yellow", false, true, true, nil, bb)
	second.Render("yellow", false, true, true, nil, bb)

	out := bb.String()
	assert.Contains(t, out, "\nplain tick 38\n")
	assert.NotContains(t, out, "\n  plain tick 38\n")
}
