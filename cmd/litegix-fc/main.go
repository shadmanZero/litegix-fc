package main

import (
	"context"
	"fmt"
	"log"
	"time"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"

	fcoci "github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
)
const (
	socketPath  = "/run/firecracker-containerd/containerd.sock"
	snapshotter = "devmapper"
	imageRef    = "docker.io/library/busybox:latest"
	vmId        = "demo-vm"
	containerID = "demo-container"
)

func main() {

	client, err := containerd.New(socketPath)
	if err != nil {
		log.Fatalf("containerd connect: %v", err)
	}
	defer client.Close()

	ctx := namespaces.WithNamespace(context.Background(), "demoNS")
	
	fmt.Println("Pulling image …")
	image, err := client.Pull(ctx, imageRef,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter(snapshotter),
	)
	if err != nil {
		log.Fatalf("pull: %v", err)
	}
	fmt.Println("Image ready:", image.Name())
	
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithProcessArgs("/bin/sh", "-c", "echo hello-vm"),
		fcoci.WithVMID(vmId),                 
	}
	container, err := client.NewContainer(ctx, containerID,
		containerd.WithSnapshotter(snapshotter),
		containerd.WithNewSnapshot(containerID+"-snap", image),
		containerd.WithNewSpec(specOpts...),
		containerd.WithRuntime("aws.firecracker", nil), 
	)
	if err != nil {
		log.Fatalf("new container: %v", err)
	}
	defer container.Delete(ctx, containerd.WithSnapshotCleanup)

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		log.Fatalf("new task: %v", err)
	}
	defer task.Delete(ctx)

	if err := task.Start(ctx); err != nil {
		log.Fatalf("start task: %v", err)
	}

	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		log.Fatalf("wait: %v", err)
	}

	select {
	case es := <-exitStatusC:
		code, _, _ := es.Result()
		fmt.Println("VM exited with code", code)
	case <-time.After(30 * time.Second):
		fmt.Println("timeout – killing VM")
		task.Kill(ctx, syscall.SIGKILL)
	}

	fmt.Println("VM running")
	

	

}
