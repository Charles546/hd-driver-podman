// Copyright 2026 Chun Huang (Charles).

package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/api/handlers"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func (d *podmanDriver) exec(msg *dipper.Message) {
	log := podman.GetLogger()
	msg = dipper.DeserializePayload(msg)
	log.Debugf("[%s] exec with payload %+v", podman.Service, msg.Payload)

	ctx, cancel := d.GetContext(msg)
	defer cancel()

	payload, _ := msg.Payload.(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}

	containerID, _ := dipper.GetMapDataStr(payload, "container_id")
	if containerID == "" {
		msg.Reply <- dipper.Message{Labels: map[string]string{"status": "error", "reason": "missing container_id"}}

		return
	}
	shell, _ := dipper.GetMapDataStr(payload, "shell")
	if shell == "" {
		shell = "sh"
	}

	// Command can be provided as `command` (array) or `script` (string)
	var cmdArgs []string
	if c, ok := payload["command"]; ok && c != nil {
		switch t := c.(type) {
		case []any:
			for _, v := range t {
				cmdArgs = append(cmdArgs, fmt.Sprintf("%v", v))
			}
		case []string:
			cmdArgs = append(cmdArgs, t...)
		case string:
			cmdArgs = []string{shell, "-c", t}
		default:
			cmdArgs = []string{shell, "-c", fmt.Sprintf("%v", t)}
		}
	} else if s, ok := dipper.GetMapDataStr(payload, "script"); ok && s != "" {
		cmdArgs = []string{shell, "-c", s}
	} else {
		msg.Reply <- dipper.Message{Labels: map[string]string{"status": "error", "reason": "missing cmd or script"}}

		return
	}

	// Use Podman bindings to create and start an exec session and attach
	createCfg := &handlers.ExecCreateConfig{
		ExecOptions: dockerContainer.ExecOptions{
			Cmd:          cmdArgs,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          false,
		},
	}

	conn := d.getConnection(ctx, msg)
	sessionID := dipper.Must(containers.ExecCreate(conn, containerID, createCfg)).(string)

	var buf bytes.Buffer
	attachOpts := new(containers.ExecStartAndAttachOptions).
		WithOutputStream(&buf).
		WithErrorStream(&buf).
		WithAttachOutput(true).
		WithAttachError(true)

	dipper.Must(containers.ExecStartAndAttach(conn, sessionID, attachOpts))

	// Inspect exec to get exit code
	var inspect *define.InspectExecSession
	running := true
	for running {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		inspect = dipper.Must(containers.ExecInspect(conn, sessionID, nil)).(*define.InspectExecSession)
		running = inspect.Running
	}
	exitCode := int(inspect.ExitCode)
	output := buf.String()

	status := "success"
	reason := ""
	if exitCode != 0 {
		status = "failure"
		reason = fmt.Sprintf("exit code %d", exitCode)
	}

	log.Debugf("[%s] exec output: %s", podman.Service, output)

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"output":    output,
			"exit_code": exitCode,
		},
		Labels: map[string]string{"status": status, "reason": reason},
	}
}
