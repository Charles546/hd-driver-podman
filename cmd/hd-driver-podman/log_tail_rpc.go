// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/pods"
	"github.com/containers/podman/v5/pkg/domain/entities"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const (
	defaultTailWaitSeconds = 3
	defaultTailMaxLines    = 200
	defaultDoneMaxLines    = 5000
	hardTailMaxLines       = 10000
	tailPollInterval       = 300 * time.Millisecond
)

type podContainerState struct {
	ID       string
	Name     string
	ExitCode int32
	State    string
	Infra    bool
}

type tailLine struct {
	Container string `json:"container"`
	Line      string `json:"line"`
	Index     int    `json:"index"`
}

type containerCursor struct {
	TS   string `json:"ts"`
	Skip int    `json:"skip"`
}

func toInt(v interface{}, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint8:
		return int(t)
	case uint16:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		i, err := strconv.Atoi(t)
		if err == nil {
			return i
		}
	}

	return fallback
}

func parseContainerCursor(payload map[string]interface{}) map[string]containerCursor {
	ret := map[string]containerCursor{}
	raw, ok := payload["cursor"]
	if !ok || raw == nil {
		return ret
	}

	cursorMap, ok := raw.(map[string]interface{})
	if !ok {
		return ret
	}

	if nested, found := cursorMap["containers"]; found && nested != nil {
		if nestedMap, isMap := nested.(map[string]interface{}); isMap {
			cursorMap = nestedMap
		}
	}

	for name, item := range cursorMap {
		state := containerCursor{}
		switch t := item.(type) {
		case map[string]interface{}:
			if ts, ok := t["ts"].(string); ok {
				state.TS = strings.TrimSpace(ts)
			}
			state.Skip = toInt(t["skip"], 0)
		default:
			// Backward compatibility for legacy index cursors.
			state.Skip = toInt(t, 0)
		}
		if state.Skip < 0 {
			state.Skip = 0
		}
		ret[name] = state
	}

	return ret
}

func parseIncludeContainers(payload map[string]interface{}) map[string]bool {
	ret := map[string]bool{}
	raw, ok := payload["include_containers"]
	if !ok || raw == nil {
		return ret
	}

	switch t := raw.(type) {
	case []interface{}:
		for _, v := range t {
			name := strings.TrimSpace(fmt.Sprintf("%v", v))
			if name != "" {
				ret[name] = true
			}
		}
	case []string:
		for _, v := range t {
			name := strings.TrimSpace(v)
			if name != "" {
				ret[name] = true
			}
		}
	}

	return ret
}

func collectLinesFromChunks(ch <-chan string) []string {
	lines := []string{}
	carry := ""

	for chunk := range ch {
		if chunk == "" {
			continue
		}

		carry += chunk
		for {
			idx := strings.IndexByte(carry, '\n')
			if idx < 0 {
				break
			}

			line := carry[:idx]
			if line != "" {
				lines = append(lines, line)
			}
			carry = carry[idx+1:]
		}
	}

	if carry != "" {
		lines = append(lines, carry)
	}

	return lines
}

func extractLineTimestamp(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	space := strings.IndexByte(line, ' ')
	if space <= 0 {
		return ""
	}

	ts := line[:space]
	if _, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return ts
	}
	if _, err := time.Parse(time.RFC3339, ts); err == nil {
		return ts
	}

	return ""
}

func (d *podmanDriver) getPodContainers(conn context.Context, podID string, include map[string]bool) ([]podContainerState, bool, string) {
	inspect := dipper.Must(pods.Inspect(conn, podID, nil)).(*entities.PodInspectReport)
	ret := []podContainerState{}

	done := true
	status := "success"
	reason := ""

	for _, c := range inspect.Containers {
		rpt := dipper.Must(containers.Inspect(conn, c.ID, nil)).(*define.InspectContainerData)
		item := podContainerState{
			ID:       c.ID,
			Name:     c.Name,
			ExitCode: rpt.State.ExitCode,
			State:    strings.ToLower(strings.TrimSpace(rpt.State.Status)),
			Infra:    rpt.IsInfra,
		}
		if item.Infra {
			continue
		}
		if len(include) > 0 && !include[item.Name] {
			continue
		}

		if item.State == "running" || item.State == "paused" || item.State == "created" || item.State == "configured" {
			done = false
		}

		if done && item.ExitCode != 0 {
			status = "failure"
			if reason == "" {
				reason = fmt.Sprintf("container %s failed with exit code %d", item.Name, item.ExitCode)
			}
		}

		ret = append(ret, item)
	}

	return ret, done, mapReason(status, reason)
}

func mapReason(status, reason string) string {
	if status == "failure" {
		return reason
	}

	return ""
}

func (d *podmanDriver) fetchContainerLines(conn context.Context, containerID string, state containerCursor, tailLimit int) []string {
	out := make(chan string)
	linesOut := make(chan []string, 1)
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		linesOut <- collectLinesFromChunks(out)
	}()

	logOpts := new(containers.LogOptions).
		WithStderr(true).
		WithStdout(true).
		WithTimestamps(true)

	if state.TS != "" {
		logOpts.WithSince(state.TS)
	} else if tailLimit > 0 {
		logOpts.WithTail(strconv.Itoa(tailLimit))
	}

	dipper.Must(containers.Logs(conn, containerID, logOpts, out, out))
	close(out)
	<-fin
	lines := <-linesOut

	return lines
}

func clampLimit(limit int) int {
	if limit <= 0 {
		limit = defaultTailMaxLines
	}
	if limit > hardTailMaxLines {
		limit = hardTailMaxLines
	}

	return limit
}

func updateCursorWithLine(state containerCursor, line string) containerCursor {
	ts := extractLineTimestamp(line)
	if ts == "" {
		state.Skip++

		return state
	}

	if state.TS == ts {
		state.Skip++

		return state
	}

	state.TS = ts
	state.Skip = 1

	return state
}

func filterByCursor(lines []string, state containerCursor) []string {
	if state.TS == "" || state.Skip <= 0 {
		return lines
	}

	ret := []string{}
	boundarySeen := 0
	for _, line := range lines {
		ts := extractLineTimestamp(line)
		if ts == state.TS && boundarySeen < state.Skip {
			boundarySeen++
			continue
		}
		ret = append(ret, line)
	}

	return ret
}

func (d *podmanDriver) buildTailChunk(conn context.Context, podID string, cursor map[string]containerCursor, include map[string]bool, limit int, tailForEmptyCursor int) ([]tailLine, map[string]containerCursor, bool, bool, string, string) {
	containers, done, reason := d.getPodContainers(conn, podID, include)
	status := "success"
	if reason != "" {
		status = "failure"
	}

	lines := []tailLine{}
	nextCursor := map[string]containerCursor{}
	hasMore := false

	for _, c := range containers {
		state := cursor[c.Name]
		rawLines := d.fetchContainerLines(conn, c.ID, state, tailForEmptyCursor)
		candidateLines := filterByCursor(rawLines, state)

		remaining := limit - len(lines)
		emitCount := len(candidateLines)
		if remaining < emitCount {
			emitCount = remaining
		}

		for i := 0; i < emitCount; i++ {
			line := candidateLines[i]
			lines = append(lines, tailLine{
				Container: c.Name,
				Line:      line,
				Index:     state.Skip,
			})
			state = updateCursorWithLine(state, line)
		}

		nextCursor[c.Name] = state
		if emitCount < len(candidateLines) {
			hasMore = true
		}

		if len(lines) >= limit {
			for _, cc := range containers {
				if _, ok := nextCursor[cc.Name]; ok {
					continue
				}
				nextCursor[cc.Name] = cursor[cc.Name]
			}
			hasMore = true

			break
		}
	}

	return lines, nextCursor, hasMore, done, status, reason
}

func (d *podmanDriver) getPodLogTail(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	ctx, cancel := d.GetContext(msg)
	defer cancel()

	podID := dipper.MustGetMapDataStr(msg.Payload, "pod_id")
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		payload = map[string]interface{}{}
	}

	waitSeconds := toInt(payload["wait_seconds"], defaultTailWaitSeconds)
	if waitSeconds < 0 {
		waitSeconds = 0
	}
	maxLines := clampLimit(toInt(payload["max_lines"], defaultTailMaxLines))
	doneMaxLines := clampLimit(toInt(payload["done_max_lines"], defaultDoneMaxLines))
	cursor := parseContainerCursor(payload)
	include := parseIncludeContainers(payload)

	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		conn := d.getConnection(ctx, msg)
		lines, nextCursor, hasMore, done, status, reason := d.buildTailChunk(conn, podID, cursor, include, maxLines, maxLines)
		limitUsed := maxLines
		if done {
			limitUsed = doneMaxLines
			if limitUsed != maxLines {
				lines, nextCursor, hasMore, done, status, reason = d.buildTailChunk(conn, podID, cursor, include, limitUsed, doneMaxLines)
			}
		}

		if len(lines) > 0 || done || waitSeconds == 0 || time.Now().After(deadline) {
			msg.Reply <- dipper.Message{
				Payload: map[string]interface{}{
					"lines":       lines,
					"next_cursor": map[string]interface{}{"containers": nextCursor},
					"has_more":    hasMore,
					"done":        done,
					"truncated":   hasMore && len(lines) >= limitUsed,
				},
				Labels: map[string]string{
					"status": status,
					"reason": reason,
				},
			}

			return
		}

		time.Sleep(tailPollInterval)
	}
}
