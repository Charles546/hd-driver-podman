// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package hd-driver-podman enables Honeydipper to run containers and
// pods with podman API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/bindings/pods"
	"github.com/containers/podman/v5/pkg/domain/entities"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

type podmanDriver struct {
	*dipper.Driver
}

var podman = &podmanDriver{}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all operator service only.\n")
		fmt.Printf("  This program provides honeydipper with capability of running pod and containers using podman.\n")
	}
}

func main() {
	initFlags()
	flag.Parse()
	podman.Driver = dipper.NewDriver(flag.Arg(0), "podman")
	podman.Driver.Commands["start_pod"] = podman.startPod
	podman.Driver.Commands["wait_pod|interruptible"] = podman.waitPod
	podman.Driver.DefaultTimeout["wait_pod"] = "30m"
	podman.Driver.Commands["get_pod_log"] = podman.getPodLog
	podman.Driver.Reload = func(m *dipper.Message) {}
	podman.Driver.Run()
}

func (d *podmanDriver) getConnection(ctx context.Context, m *dipper.Message) context.Context {
	var opts bindings.Options
	if uri, ok := dipper.GetMapDataStr(m.Payload, "uri"); ok {
		opts.URI = uri
	}
	if identity, ok := dipper.GetMapDataStr(m.Payload, "identity"); ok {
		opts.Identity = identity
	}

	return dipper.Must(bindings.NewConnectionWithOptions(ctx, opts)).(context.Context)
}

func (d *podmanDriver) startPod(msg *dipper.Message) {
	log := podman.Driver.GetLogger()
	log.Debugf("[%s] run container with payload %+v", podman.Driver.Service, msg.Payload)
	msg = dipper.DeserializePayload(msg)
	ctx, cancel := d.GetContext(msg)
	defer cancel()

	spec := &entities.PodSpec{
		PodSpecGen: *specgen.NewPodSpecGenerator(),
	}
	pspec, _ := dipper.GetMapData(msg.Payload, "pod_spec")
	if pspec != nil {
		dipper.Must(mapstructure.Decode(pspec, &spec.PodSpecGen))
	}
	spec.PodSpecGen.ExitPolicy = "stop"

	workVolumeMountPoint, _ := dipper.GetMapDataStr(msg.Payload, "work_volume_mount_point")
	if workVolumeMountPoint == "" {
		workVolumeMountPoint = "/opt/honeydipper"
	}

	if useWorkVolume, _ := dipper.GetMapDataStr(msg.Payload, "use_work_volume"); useWorkVolume != "" {
		spec.PodSpecGen.Volumes = append(spec.PodSpecGen.Volumes, &specgen.NamedVolume{
			Dest: "/opt/honeydipper",
			Name: useWorkVolume,
		})
	} else if withWorkVolume, _ := dipper.GetMapDataBool(msg.Payload, "with_work_volume"); withWorkVolume {
		spec.PodSpecGen.Volumes = append(spec.PodSpecGen.Volumes, &specgen.NamedVolume{
			Dest: workVolumeMountPoint,
		})
	}

	dipper.Must(spec.PodSpecGen.Validate())

	cursor := "0"
	if c, ok := msg.Labels["cursor"]; ok {
		cursor = c
	}
	suffix := fmt.Sprintf("-hd-%s-%s", msg.Labels["sessionID"], cursor)
	if spec.PodSpecGen.Name != "" {
		spec.PodSpecGen.Name += suffix
	}

	conn := d.getConnection(ctx, msg)
	pod := dipper.Must(pods.CreatePodFromSpec(conn, spec)).(*entities.PodCreateReport)

	cspecs := dipper.MustGetMapData(msg.Payload, "containers").([]any)
	numContainers := len(cspecs)

	for i, c := range cspecs {
		image := dipper.MustGetMapDataStr(c, "image")
		exists := dipper.Must(images.Exists(conn, image, nil)).(bool)
		policy, _ := dipper.GetMapDataStr(c, "imagePullPolicy")
		delete(c.(map[string]any), "imagePullPolicy")

		if !exists && !strings.EqualFold(policy, "Never") || strings.EqualFold(policy, "Always") {
			dipper.Must(images.Pull(conn, image, nil))
		}

		initContainerType, _ := dipper.GetMapDataStr(c, "init_container_type")
		if initContainerType == "" && i < numContainers-1 {
			c.(map[string]any)["init_container_type"] = "once"
		}

		cspec := specgen.NewSpecGenerator(image, false)
		dipper.Must(json.Unmarshal(dipper.SerializeContent(c), cspec))
		cspec.Pod = pod.Id
		if cspec.Name != "" {
			cspec.Name += suffix + "-c"
		}
		dipper.Must(containers.CreateWithSpec(conn, cspec, nil))
	}
	rpt := dipper.Must(pods.Start(conn, pod.Id, nil)).(*entities.PodStartReport)

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"pod_id": rpt.Id,
			"errors": rpt.Errs,
		},
	}
}

func (d *podmanDriver) waitPod(msg *dipper.Message) {
	log := podman.GetLogger()
	msg = dipper.DeserializePayload(msg)
	log.Debugf("[%s] wait pod with payload %+v", podman.Service, msg.Payload)

	pod_id := dipper.MustGetMapDataStr(msg.Payload, "pod_id")
	cmdCtx, cancel := d.GetContext(msg)
	defer cancel()
	conn := d.getConnection(cmdCtx, msg)

	inspect := dipper.Must(pods.Inspect(conn, pod_id, nil)).(*entities.PodInspectReport)
	status := "success"
	reason := ""
	for _, c := range inspect.Containers {
		extcode := dipper.Must(containers.Wait(conn, c.ID, nil)).(int32)
		if extcode != 0 {
			status = "failure"
			reason = fmt.Sprintf("container %s failed with exit code %d", c.Name, extcode)
		}
	}

	ret := dipper.Message{
		Labels: map[string]string{
			"status": status,
		},
	}
	if status == "failure" {
		ret.Labels["reason"] = reason
	}

	msg.Reply <- ret
}

func (d *podmanDriver) getPodLog(msg *dipper.Message) {
	log := podman.GetLogger()
	msg = dipper.DeserializePayload(msg)
	log.Debugf("[%s] wait pod with payload %+v", podman.Service, msg.Payload)

	deleteOnSuccess := dipper.MustGetMapDataBool(msg.Payload, "delete_on_success")

	pod_id := dipper.MustGetMapDataStr(msg.Payload, "pod_id")
	cmdCtx, cancel := d.GetContext(msg)
	defer cancel()
	conn := d.getConnection(cmdCtx, msg)

	inspect := dipper.Must(pods.Inspect(conn, pod_id, nil)).(*entities.PodInspectReport)
	var (
		all          string
		perContainer = map[string]string{}
		receivers    sync.WaitGroup
		succeeded    bool = true
	)
	for _, c := range inspect.Containers {
		if succeeded {
			rpt := dipper.Must(containers.Inspect(conn, c.ID, nil)).(*define.InspectContainerData)
			succeeded = rpt.State.ExitCode == 0
		}
		out := make(chan string)
		receivers.Add(1)
		go func(out chan string) {
			defer receivers.Done()
			for l := range out {
				log.Warningf("podman pod log: %s", l)
				all += l
				perContainer[c.Name] += l
			}
		}(out)
		containers.Logs(conn, c.ID, nil, out, out)
		close(out)
	}

	receivers.Wait()
	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"all":        all,
			"containers": perContainer,
		},
	}

	if deleteOnSuccess && succeeded {
		log.Warningf("removing pod %s", pod_id)
		dipper.Must(pods.Remove(conn, pod_id, nil))
	}
}
