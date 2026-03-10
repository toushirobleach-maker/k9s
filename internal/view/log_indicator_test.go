// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view_test

import (
	"testing"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/view"
	"github.com/stretchr/testify/assert"
)

func TestLogIndicatorRefresh(t *testing.T) {
	defaults := config.NewStyles()
	uu := map[string]struct {
		li *view.LogIndicator
		e  string
	}{
		"all-containers": {
			view.NewLogIndicator(config.NewConfig(nil), defaults, true), "[::b]AllContainers:[gray::d]Off[-::]     [::b]Autoscroll:[limegreen::b]On[-::]      [::b]ColumnLock:[gray::d]Off[-::]     [::b]FullScreen:[gray::d]Off[-::]     [::b]Timestamps:[limegreen::b]On[-::]      [::b]PrettyJSON:[gray::d]Off[-::]     [::b]Wrap:[gray::d]Off[-::]\n",
		},
		"plain": {
			view.NewLogIndicator(config.NewConfig(nil), defaults, false), "[::b]Autoscroll:[limegreen::b]On[-::]      [::b]ColumnLock:[gray::d]Off[-::]     [::b]FullScreen:[gray::d]Off[-::]     [::b]Timestamps:[limegreen::b]On[-::]      [::b]PrettyJSON:[gray::d]Off[-::]     [::b]Wrap:[gray::d]Off[-::]\n",
		},
		"pretty-json-hint": {
			func() *view.LogIndicator {
				li := view.NewLogIndicator(config.NewConfig(nil), defaults, false)
				li.TogglePrettyJSON()
				return li
			}(), "[::b]Autoscroll:[limegreen::b]On[-::]      [::b]ColumnLock:[gray::d]Off[-::]     [::b]FullScreen:[gray::d]Off[-::]     [::b]Timestamps:[limegreen::b]On[-::]      [::b]PrettyJSON:[limegreen::b]On[-::]      [::b]JSON Fields:[orange::b]<Shift-J>[-::]     [::b]Wrap:[gray::d]Off[-::]\n",
		},
	}

	for k := range uu {
		u := uu[k]
		t.Run(k, func(t *testing.T) {
			u.li.Refresh()
			assert.Equal(t, u.e, u.li.GetText(false))
		})
	}
}

func BenchmarkLogIndicatorRefresh(b *testing.B) {
	defaults := config.NewStyles()
	v := view.NewLogIndicator(config.NewConfig(nil), defaults, true)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		v.Refresh()
	}
}
