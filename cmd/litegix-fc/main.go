package main

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"

	fcoci "github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
)

const (
	socketPath                       = "/run/firecracker-containerd/containerd.sock"
	snapshotter                      = "devmapper"
	imageRef                         = "docker.io/library/busybox:latest"
	vmId                             = "demo-vm"
	containerID                      = "demo-container"
	firecrackerVMIDAnnotation        = "firecracker.vm.id"
	firecrackerMemoryAnnotation      = "firecracker.vm.memory"
	firecrackerCPUCountAnnotation    = "firecracker.vm.cpu_count"
	firecrackerSnapshotterAnnotation = "firecracker.vm.snapshotter"
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
        // highlight-start
        oci.WithProcessArgs("/bin/sh", "-c", "echo '--- CPU Count ---'; nproc; echo; echo '--- Memory (MiB) ---'; free -m"),
        // highlight-end
        fcoci.WithVMID(vmId),
		//oci.WithCPUs("2"),
		oci.WithMemoryLimit(1024),
        oci.WithAnnotations(map[string]string{
            firecrackerVMIDAnnotation:     vmId,
            firecrackerMemoryAnnotation:   "1024",
            firecrackerCPUCountAnnotation: "2",
        }),
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

    // The message "VM running" is printed after the VM has already exited in this script.
    // It's kept here to match the original code's structure.
    fmt.Println("VM finished execution")

}