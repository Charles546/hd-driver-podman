// Copyright 2026 Chun Huang (Charles).

// This Source Code Form is dual-licensed.
// By default, this file is licensed under the GNU Affero General Public License v3.0.
// If you have a separate written commercial agreement, you may use this file under those terms instead.

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

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/bindings/pods"
	"github.com/containers/podman/v5/pkg/bindings/volumes"
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
	podman.Commands["create_pod"] = podman.createPod
	podman.Commands["start_pod|interruptible"] = podman.startPod
	podman.DefaultTimeout["start_pod"] = "30m"
	podman.Commands["wait_pod|interruptible"] = podman.waitPod
	podman.DefaultTimeout["wait_pod"] = "30m"
	podman.Commands["get_pod_log"] = podman.getPodLog
	podman.RPCHandlers["get_pod_log_tail"] = podman.getPodLogTail
	podman.Commands["create_volume"] = podman.createVolume
	podman.Commands["delete_volume"] = podman.deleteVolume
	podman.Reload = func(m *dipper.Message) {}
	podman.Run()
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

func (d *podmanDriver) createPod(msg *dipper.Message) {
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
		e := json.Unmarshal(dipper.SerializeContent(pspec), &spec.PodSpecGen)
		if e != nil {
			panic(fmt.Errorf("%w: %s", e, string(dipper.SerializeContent(pspec))))
		}
	}
	spec.PodSpecGen.ExitPolicy = "stop"

	workVolumeMountPoint, _ := dipper.GetMapDataStr(msg.Payload, "work_volume_mount_point")
	if workVolumeMountPoint == "" {
		workVolumeMountPoint = "/local/honeydipper"
	}

	if useWorkVolume, _ := dipper.GetMapDataStr(msg.Payload, "use_work_volume"); useWorkVolume != "" {
		spec.PodSpecGen.Volumes = append(spec.PodSpecGen.Volumes, &specgen.NamedVolume{
			Dest: workVolumeMountPoint,
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
			c.(map[string]any)["init_container_type"] = "always"
		}

		cspec := specgen.NewSpecGenerator(image, false)
		e := json.Unmarshal(dipper.SerializeContent(c), cspec)
		if e != nil {
			panic(fmt.Errorf("%w: %s", e, string(dipper.SerializeContent(c))))
		}
		cspec.Pod = pod.Id
		if cspec.Name != "" {
			cspec.Name += suffix + "-c"
		}
		dipper.Must(containers.CreateWithSpec(conn, cspec, nil))
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"pod_id": pod.Id,
		},
	}
}

func (d *podmanDriver) startPod(msg *dipper.Message) {
	log := podman.Driver.GetLogger()
	log.Debugf("[%s] run container with payload %+v", podman.Driver.Service, msg.Payload)
	msg = dipper.DeserializePayload(msg)
	ctx, cancel := d.GetContext(msg)
	defer cancel()

	conn := d.getConnection(ctx, msg)
	podId := dipper.MustGetMapDataStr(msg.Payload, "pod_id")

	rpt := dipper.Must(pods.Start(conn, podId, nil)).(*entities.PodStartReport)

	ret := dipper.Message{}
	if len(rpt.Errs) > 0 {
		ret.Labels = map[string]string{
			"reason": fmt.Sprintf("error starting pod: %+v", rpt.Errs),
			"status": "failure",
		}
	}
	msg.Reply <- ret
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
		cinfo := dipper.Must(containers.Inspect(conn, c.ID, nil)).(*define.InspectContainerData)
		if cinfo.IsInfra {
			continue
		}
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
		succeeded    = true
		reason       string
	)
	for _, c := range inspect.Containers {
		if succeeded {
			rpt := dipper.Must(containers.Inspect(conn, c.ID, nil)).(*define.InspectContainerData)
			succeeded = rpt.State.ExitCode == 0
			if !succeeded && reason == "" {
				reason = fmt.Sprintf("container %s failed with exit code %d", c.Name, rpt.State.ExitCode)
			}
		}
		out := make(chan string)
		fin := make(chan struct{})
		go func(out chan string) {
			defer close(fin)
			for l := range out {
				// log.Warningf("podman pod log: %s", l)
				all += l
				perContainer[c.Name] += l
			}
		}(out)
		var optTrue = true
		_ = containers.Logs(conn, c.ID, &containers.LogOptions{Stderr: &optTrue, Stdout: &optTrue, Timestamps: &optTrue}, out, out)
		close(out)
		<-fin
	}

	labels := map[string]string{
		"status": "success",
	}
	if !succeeded {
		labels["status"] = "failure"
		labels["reason"] = reason
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"all":        all,
			"containers": perContainer,
		},
		Labels: labels,
	}

	if deleteOnSuccess && succeeded {
		log.Warningf("removing pod %s", pod_id)
		dipper.Must(pods.Remove(conn, pod_id, nil))
	}
}

func (d *podmanDriver) createVolume(msg *dipper.Message) {
	log := podman.Driver.GetLogger()
	log.Debugf("[%s] create volume with payload %+v", podman.Driver.Service, msg.Payload)
	msg = dipper.DeserializePayload(msg)
	ctx, cancel := d.GetContext(msg)
	defer cancel()

	conn := d.getConnection(ctx, msg)

	opts := &entities.VolumeCreateOptions{}
	if vspec, ok := dipper.GetMapData(msg.Payload, "volume_spec"); ok {
		dipper.Must(mapstructure.Decode(vspec, opts))
	}

	name, _ := dipper.GetMapDataStr(msg.Payload, "name")
	if name != "" {
		opts.Name = name
		opts.IgnoreIfExists, _ = dipper.GetMapDataBool(msg.Payload, "ignore_if_exists")
	}

	vol := dipper.Must(volumes.Create(conn, *opts, nil)).(*entities.VolumeConfigResponse)
	log.Infof("created volume %+v", vol)
	ret := dipper.Message{
		Payload: map[string]any{
			"volume_name": vol.Name,
		},
	}

	log.Warningf("message %+v", ret)

	msg.Reply <- ret
}

func (d *podmanDriver) deleteVolume(msg *dipper.Message) {
	log := podman.Driver.GetLogger()
	log.Debugf("[%s] delete volume with payload %+v", podman.Driver.Service, msg.Payload)
	msg = dipper.DeserializePayload(msg)
	ctx, cancel := d.GetContext(msg)
	defer cancel()

	conn := d.getConnection(ctx, msg)

	volumeName := dipper.MustGetMapDataStr(msg.Payload, "volume_name")
	force, _ := dipper.GetMapDataBool(msg.Payload, "force")

	opts := &volumes.RemoveOptions{}
	opts.WithForce(force)

	dipper.Must(volumes.Remove(conn, volumeName, opts))

	msg.Reply <- dipper.Message{
		Payload: map[string]any{
			"status": "success",
		},
	}
}
