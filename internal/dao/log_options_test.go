// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dao_test

import (
	"testing"

	"github.com/derailed/k9s/internal/dao"
	"github.com/stretchr/testify/assert"
)

func TestLogOptionsToggleAllContainers(t *testing.T) {
	uu := map[string]struct {
		opts dao.LogOptions
		co   string
		want bool
	}{
		"empty": {
			opts: dao.LogOptions{},
			want: true,
		},
		"container": {
			opts: dao.LogOptions{Container: "blee"},
			want: true,
		},
		"default-container": {
			opts: dao.LogOptions{AllContainers: true},
			co:   "blee",
		},
		"single-container": {
			opts: dao.LogOptions{Container: "blee", SingleContainer: true},
			co:   "blee",
		},
	}

	for k := range uu {
		u := uu[k]
		t.Run(k, func(t *testing.T) {
			u.opts.DefaultContainer = "blee"
			u.opts.ToggleAllContainers()
			assert.Equal(t, u.want, u.opts.AllContainers)
			assert.Equal(t, u.co, u.opts.Container)
		})
	}
}

func TestLogOptionsToPodLogOptionsHead(t *testing.T) {
	opts := dao.LogOptions{
		Container:  "blee",
		Lines:      100,
		BufferSize: 5000,
		Head:       true,
	}

	got := opts.ToPodLogOptions()

	assert.False(t, got.Follow)
	assert.Nil(t, got.TailLines)
	assert.Nil(t, got.SinceSeconds)
	assert.Nil(t, got.SinceTime)
	if assert.NotNil(t, got.LimitBytes) {
		assert.EqualValues(t, 4*1024*1024, *got.LimitBytes)
	}
}

func TestLogOptionsToPodLogOptionsTailUsesCappedInitialBuffer(t *testing.T) {
	opts := dao.LogOptions{
		Container:  "blee",
		Lines:      100,
		BufferSize: 5000,
	}

	got := opts.ToPodLogOptions()

	assert.True(t, got.Follow)
	if assert.NotNil(t, got.TailLines) {
		assert.EqualValues(t, 2000, *got.TailLines)
	}
}

func TestLogOptionsToPodLogOptionsTailHonorsHigherExplicitTailCount(t *testing.T) {
	opts := dao.LogOptions{
		Container:  "blee",
		Lines:      2500,
		BufferSize: 5000,
	}

	got := opts.ToPodLogOptions()

	assert.True(t, got.Follow)
	if assert.NotNil(t, got.TailLines) {
		assert.EqualValues(t, 2500, *got.TailLines)
	}
}
