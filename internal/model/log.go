// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package model

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/derailed/k9s/internal"
	"github.com/derailed/k9s/internal/client"
	"github.com/derailed/k9s/internal/color"
	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/dao"
	"github.com/derailed/k9s/internal/slogs"
)

// LogsListener represents a log model listener.
type LogsListener interface {
	// LogChanged notifies the model changed.
	LogChanged([][]byte)

	// LogCleared indicates logs are cleared.
	LogCleared()

	// LogFailed indicates a log failure.
	LogFailed(error)

	// LogStop indicates logging was canceled.
	LogStop()

	// LogResume indicates logging has resumed.
	LogResume()

	// LogCanceled indicates no more logs will come.
	LogCanceled()
}

// Log represents a resource logger.
type Log struct {
	factory      dao.Factory
	lines        *dao.LogItems
	listeners    []LogsListener
	gvr          *client.GVR
	logOptions   *dao.LogOptions
	cancelFn     context.CancelFunc
	mx           sync.RWMutex
	filter       string
	lastSent     int
	flushTimeout time.Duration
	prettyAll    bool
	prettyFields map[string]struct{}
	prettyCache  map[string]struct{}
}

// NewLog returns a new model.
func NewLog(gvr *client.GVR, opts *dao.LogOptions, flushTimeout time.Duration) *Log {
	return &Log{
		gvr:          gvr,
		logOptions:   opts,
		lines:        dao.NewLogItems(),
		flushTimeout: flushTimeout,
		prettyAll:    true,
		prettyFields: make(map[string]struct{}),
		prettyCache:  make(map[string]struct{}),
	}
}

func (l *Log) GVR() *client.GVR {
	return l.gvr
}

func (l *Log) LogOptions() *dao.LogOptions {
	return l.logOptions
}

// SinceSeconds returns since seconds option.
func (l *Log) SinceSeconds() int64 {
	l.mx.RLock()
	defer l.mx.RUnlock()

	return l.logOptions.SinceSeconds
}

// IsHead returns log head option.
func (l *Log) IsHead() bool {
	l.mx.RLock()
	defer l.mx.RUnlock()

	return l.logOptions.Head
}

// ToggleShowTimestamp toggles to logs timestamps.
func (l *Log) ToggleShowTimestamp(b bool) {
	l.logOptions.ShowTimestamp = b
	l.Refresh()
}

// TogglePrettyJSON toggles pretty JSON rendering.
func (l *Log) TogglePrettyJSON(b bool) {
	l.logOptions.PrettyJSON = b
	if b {
		l.buildPrettyCache(500)
	} else {
		l.mx.Lock()
		l.prettyCache = make(map[string]struct{})
		l.mx.Unlock()
	}
	l.Refresh()
}

// SetPrettyFields sets the pretty JSON field selection.
func (l *Log) SetPrettyFields(all bool, fields map[string]struct{}) {
	l.mx.Lock()
	l.prettyAll = all
	l.prettyFields = make(map[string]struct{}, len(fields))
	for k := range fields {
		l.prettyFields[k] = struct{}{}
	}
	l.mx.Unlock()
	l.Refresh()
}

// PrettyFieldsAll reports whether all fields are selected.
func (l *Log) PrettyFieldsAll() bool {
	l.mx.RLock()
	defer l.mx.RUnlock()
	return l.prettyAll
}

// PrettyFields returns the selected pretty JSON fields.
func (l *Log) PrettyFields() []string {
	l.mx.RLock()
	defer l.mx.RUnlock()
	if l.prettyAll || len(l.prettyFields) == 0 {
		return nil
	}
	return mapKeys(l.prettyFields)
}

// PrettyFieldCache returns all known JSON fields.
func (l *Log) PrettyFieldCache() []string {
	l.mx.RLock()
	defer l.mx.RUnlock()
	return mapKeys(l.prettyCache)
}

func (l *Log) Head(ctx context.Context) {
	l.mx.Lock()
	l.logOptions.Head = true
	l.mx.Unlock()
	l.Restart(ctx)
}

// SetSinceSeconds sets the logs retrieval time.
func (l *Log) SetSinceSeconds(ctx context.Context, i int64) {
	l.logOptions.SinceSeconds, l.logOptions.Head = i, false
	l.Restart(ctx)
}

// Configure sets logger configuration.
func (l *Log) Configure(opts config.Logger) {
	l.logOptions.Lines = opts.TailCount
	l.logOptions.BufferSize = opts.BufferSize
	l.logOptions.SinceSeconds = opts.SinceSeconds
	l.logOptions.PrettyJSON = opts.PrettyJSON
}

// GetPath returns resource path.
func (l *Log) GetPath() string {
	return l.logOptions.Path
}

// GetContainer returns the resource container if any or "" otherwise.
func (l *Log) GetContainer() string {
	return l.logOptions.Container
}

// HasDefaultContainer returns true if the pod has a default container, false otherwise.
func (l *Log) HasDefaultContainer() bool {
	return l.logOptions.DefaultContainer != ""
}

// Init initializes the model.
func (l *Log) Init(f dao.Factory) {
	l.factory = f
}

// Clear the logs.
func (l *Log) Clear() {
	l.mx.Lock()
	l.lines.Clear()
	l.lastSent = 0
	l.mx.Unlock()

	l.fireLogCleared()
}

// Refresh refreshes the logs.
func (l *Log) Refresh() {
	l.fireLogCleared()
	ll := make([][]byte, l.lines.Len())
	l.lines.Render(0, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
	l.fireLogChanged(ll)
}

// Restart restarts the logger.
func (l *Log) Restart(ctx context.Context) {
	l.Stop()
	l.Clear()
	l.fireLogResume()
	l.Start(ctx)
}

// Start starts logging.
func (l *Log) Start(ctx context.Context) {
	if err := l.load(ctx); err != nil {
		slog.Error("Tail logs failed!", slogs.Error, err)
		l.fireLogError(err)
	}
}

// Stop terminates logging.
func (l *Log) Stop() {
	l.cancel()
	l.mx.Lock()
	l.prettyCache = make(map[string]struct{})
	l.mx.Unlock()
}

// Set sets the log lines (for testing only!)
func (l *Log) Set(lines *dao.LogItems) {
	l.mx.Lock()
	l.lines.Merge(lines)
	l.mx.Unlock()

	l.fireLogCleared()
	ll := make([][]byte, l.lines.Len())
	l.lines.Render(0, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
	l.fireLogChanged(ll)
}

// ClearFilter resets the log filter if any.
func (l *Log) ClearFilter() {
	l.mx.Lock()
	l.filter = ""
	l.mx.Unlock()

	l.fireLogCleared()
	ll := make([][]byte, l.lines.Len())
	l.lines.Render(0, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
	l.fireLogChanged(ll)
}

// Filter filters the model using either fuzzy or regexp.
func (l *Log) Filter(q string) {
	l.mx.Lock()
	l.filter = q
	l.mx.Unlock()

	l.fireLogCleared()
	l.fireLogBuffChanged(0)
}

func (l *Log) cancel() {
	l.mx.Lock()
	defer l.mx.Unlock()
	if l.cancelFn != nil {
		l.cancelFn()
		l.cancelFn = nil
	}
}

func (l *Log) load(ctx context.Context) error {
	accessor, err := dao.AccessorFor(l.factory, l.gvr)
	if err != nil {
		return err
	}
	loggable, ok := accessor.(dao.Loggable)
	if !ok {
		return fmt.Errorf("resource %s is not Loggable", l.gvr)
	}

	l.cancel()
	ctx = context.WithValue(ctx, internal.KeyFactory, l.factory)
	ctx, l.cancelFn = context.WithCancel(ctx)

	cc, err := loggable.TailLogs(ctx, l.logOptions)
	if err != nil {
		slog.Error("Tail logs failed", slogs.Error, err)
		l.cancel()
		l.fireLogError(err)
	}
	for _, c := range cc {
		go l.updateLogs(ctx, c)
	}

	return nil
}

// Append adds a log line.
func (l *Log) Append(line *dao.LogItem) {
	if line == nil || line.IsEmpty() {
		return
	}
	l.mx.Lock()
	defer l.mx.Unlock()
	l.logOptions.SinceTime = line.GetTimestamp()
	maxLines := l.maxBufferedLinesLocked()
	if l.lines.Len() < maxLines {
		l.lines.Add(line)
		return
	}
	l.lines.Shift(line)
	l.lastSent--
	if l.lastSent < 0 {
		l.lastSent = 0
	}
}

func (l *Log) maxBufferedLinesLocked() int {
	if l.logOptions.BufferSize > 0 {
		return l.logOptions.BufferSize
	}
	if l.logOptions.Lines > 0 {
		return int(l.logOptions.Lines)
	}
	if l.logOptions.BufferSize > 0 {
		return l.logOptions.BufferSize
	}
	return config.MaxLogThreshold
}

// Notify fires of notifications to the listeners.
func (l *Log) Notify() {
	l.mx.Lock()
	defer l.mx.Unlock()

	if l.lastSent < l.lines.Len() {
		l.fireLogBuffChanged(l.lastSent)
		l.lastSent = l.lines.Len()
	}
}

// ToggleAllContainers toggles to show all containers logs.
func (l *Log) ToggleAllContainers(ctx context.Context) {
	l.logOptions.ToggleAllContainers()
	l.Restart(ctx)
}

func (l *Log) updateLogs(ctx context.Context, c dao.LogChan) {
	for {
		select {
		case item, ok := <-c:
			if !ok {
				l.Append(item)
				l.Notify()
				return
			}
			if item == dao.ItemEOF {
				l.fireCanceled()
				return
			}
			l.collectPrettyFields(item)
			l.Append(item)
			var overflow bool
			l.mx.RLock()
			overflow = l.lines.Len()-l.lastSent > l.maxBufferedLinesLocked()
			l.mx.RUnlock()
			if overflow {
				l.Notify()
			}
		case <-time.After(l.flushTimeout):
			l.Notify()
		case <-ctx.Done():
			return
		}
	}
}

func (l *Log) collectPrettyFields(item *dao.LogItem) {
	if !l.logOptions.PrettyJSON || item == nil || len(item.Bytes) == 0 {
		return
	}
	payload := item.Bytes
	if index := bytes.IndexByte(payload, ' '); index > 0 {
		payload = payload[index+1:]
	}
	keys := dao.ExtractJSONKeys(payload)
	if len(keys) == 0 {
		return
	}
	l.mx.Lock()
	for _, k := range keys {
		l.prettyCache[k] = struct{}{}
	}
	l.mx.Unlock()
}

func (l *Log) buildPrettyCache(n int) {
	items := l.lines.Last(n)
	if len(items) == 0 {
		return
	}
	cache := make(map[string]struct{})
	for _, item := range items {
		if item == nil || len(item.Bytes) == 0 {
			continue
		}
		payload := item.Bytes
		if index := bytes.IndexByte(payload, ' '); index > 0 {
			payload = payload[index+1:]
		}
		keys := dao.ExtractJSONKeys(payload)
		for _, k := range keys {
			cache[k] = struct{}{}
		}
	}
	l.mx.Lock()
	l.prettyCache = cache
	l.mx.Unlock()
}

func mapKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AddListener adds a new model listener.
func (l *Log) AddListener(listener LogsListener) {
	l.mx.Lock()
	defer l.mx.Unlock()

	l.listeners = append(l.listeners, listener)
}

// RemoveListener delete a listener from the list.
func (l *Log) RemoveListener(listener LogsListener) {
	l.mx.Lock()
	defer l.mx.Unlock()

	victim := -1
	for i, lis := range l.listeners {
		if lis == listener {
			victim = i
			break
		}
	}

	if victim >= 0 {
		l.listeners = append(l.listeners[:victim], l.listeners[victim+1:]...)
	}
}

func (l *Log) applyFilter(index int, q string) ([][]byte, error) {
	if q == "" {
		return nil, nil
	}
	matches, indices, err := l.lines.Filter(index, q, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields)
	if err != nil {
		return nil, err
	}

	// No filter!
	if matches == nil {
		ll := make([][]byte, l.lines.Len())
		l.lines.Render(index, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
		return ll, nil
	}
	// Blank filter
	if len(matches) == 0 {
		return nil, nil
	}
	filtered := make([][]byte, 0, len(matches))
	ll := make([][]byte, l.lines.Len())
	l.lines.Lines(index, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
	for i, idx := range matches {
		filtered = append(filtered, color.Highlight(ll[idx], indices[i], 209))
	}

	return filtered, nil
}

func (l *Log) fireLogBuffChanged(index int) {
	ll := make([][]byte, l.lines.Len()-index)
	if l.filter == "" {
		l.lines.Render(index, l.logOptions.ShowTimestamp, l.logOptions.PrettyJSON, l.prettyAll, l.prettyFields, ll)
	} else {
		ff, err := l.applyFilter(index, l.filter)
		if err != nil {
			l.fireLogError(err)
			return
		}
		ll = ff
	}

	if len(ll) > 0 {
		l.fireLogChanged(ll)
	}
}

func (l *Log) fireLogResume() {
	for _, lis := range l.listeners {
		lis.LogResume()
	}
}

func (l *Log) fireCanceled() {
	for _, lis := range l.listeners {
		lis.LogCanceled()
	}
}

func (l *Log) fireLogError(err error) {
	for _, lis := range l.listeners {
		lis.LogFailed(err)
	}
}

func (l *Log) fireLogChanged(lines [][]byte) {
	for _, lis := range l.listeners {
		lis.LogChanged(lines)
	}
}

func (l *Log) fireLogCleared() {
	var ll []LogsListener
	l.mx.RLock()
	ll = l.listeners
	l.mx.RUnlock()
	for _, lis := range ll {
		lis.LogCleared()
	}
}
