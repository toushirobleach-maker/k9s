// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dao

import (
	"fmt"
	"time"

	"github.com/derailed/k9s/internal/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	headLogLimitBytes       int64 = 4 * 1024 * 1024
	initialTailFetchCap     int64 = 2_000
)

// LogOptions represents logger options.
type LogOptions struct {
	CreateDuration   time.Duration
	Path             string
	Container        string
	DefaultContainer string
	SinceTime        string
	Lines            int64
	BufferSize       int
	SinceSeconds     int64
	Head             bool
	Previous         bool
	SingleContainer  bool
	MultiPods        bool
	ShowTimestamp    bool
	AllContainers    bool
	PrettyJSON       bool
}

// Info returns the option pod and container info.
func (o *LogOptions) Info() string {
	if o.Container != "" {
		return fmt.Sprintf("%s (%s)", o.Path, o.Container)
	}

	return o.Path
}

// Clone clones options.
func (o *LogOptions) Clone() *LogOptions {
	return &LogOptions{
		Path:             o.Path,
		Container:        o.Container,
		DefaultContainer: o.DefaultContainer,
		Lines:            o.Lines,
		BufferSize:       o.BufferSize,
		Previous:         o.Previous,
		Head:             o.Head,
		SingleContainer:  o.SingleContainer,
		MultiPods:        o.MultiPods,
		ShowTimestamp:    o.ShowTimestamp,
		SinceTime:        o.SinceTime,
		SinceSeconds:     o.SinceSeconds,
		AllContainers:    o.AllContainers,
		PrettyJSON:       o.PrettyJSON,
	}
}

// HasContainer checks if a container is present.
func (o *LogOptions) HasContainer() bool {
	return o.Container != ""
}

// ToggleAllContainers toggles single or all-containers if possible.
func (o *LogOptions) ToggleAllContainers() {
	if o.SingleContainer {
		return
	}
	o.AllContainers = !o.AllContainers
	if o.AllContainers {
		o.DefaultContainer, o.Container = o.Container, ""
		return
	}

	if o.DefaultContainer != "" {
		o.Container = o.DefaultContainer
	}
}

// ToPodLogOptions returns pod log options.
func (o *LogOptions) ToPodLogOptions() *v1.PodLogOptions {
	tailLines := o.Lines
	if o.BufferSize > 0 {
		bufferTail := int64(o.BufferSize)
		if bufferTail > initialTailFetchCap {
			bufferTail = initialTailFetchCap
		}
		if bufferTail > tailLines {
			tailLines = bufferTail
		}
	}
	opts := v1.PodLogOptions{
		Follow:     true,
		Timestamps: true,
		Container:  o.Container,
		Previous:   o.Previous,
		TailLines:  &tailLines,
	}
	if o.Head {
		opts.Follow = false
		opts.TailLines, opts.SinceSeconds, opts.SinceTime = nil, nil, nil
		maxBytes := headLogLimitBytes
		opts.LimitBytes = &maxBytes
		return &opts
	}
	if o.SinceSeconds < 0 {
		return &opts
	}

	if o.SinceSeconds != 0 {
		opts.SinceSeconds, opts.SinceTime = &o.SinceSeconds, nil
		return &opts
	}

	if o.SinceTime == "" {
		return &opts
	}
	if t, err := time.Parse(time.RFC3339, o.SinceTime); err == nil {
		opts.SinceTime = &metav1.Time{Time: t.Add(time.Second)}
	}

	return &opts
}

// ToLogItem add a log header to display po/co information along with the log message.
func (o *LogOptions) ToLogItem(bytes []byte) *LogItem {
	item := NewLogItem(bytes)
	if len(bytes) == 0 {
		return item
	}
	item.SingleContainer = o.SingleContainer
	if item.SingleContainer {
		item.Container = o.Container
	}
	if o.MultiPods {
		_, pod := client.Namespaced(o.Path)
		item.Pod, item.Container = pod, o.Container
	} else {
		item.Container = o.Container
	}

	return item
}

func (*LogOptions) ToErrLogItem(err error) *LogItem {
	t := time.Now().UTC().Format(time.RFC3339Nano)
	item := NewLogItem([]byte(fmt.Sprintf("%s [orange::b]%s[::-]\n", t, err)))
	item.IsError = true
	return item
}
