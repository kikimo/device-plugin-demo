package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type BarDevicePlugin struct {
	// pluginapi.DevicePluginServer
}

func (p *BarDevicePlugin) GetDevicePluginOptions(ctx context.Context, empty *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (p *BarDevicePlugin) ListAndWatch(empty *pluginapi.Empty, server pluginapi.DevicePlugin_ListAndWatchServer) error {
	devices := []*pluginapi.Device{}
	for i := 0; i < 3; i++ {
		dev := &pluginapi.Device{
			ID:     strconv.Itoa(i + 1),
			Health: pluginapi.Healthy,
		}
		devices = append(devices, dev)
	}

	resp := pluginapi.ListAndWatchResponse{Devices: devices}
	log.Printf("sending devices: %+v", devices)
	server.Send(&resp)
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			// log.Printf("updating devices: %+v", devices)
			server.Send(&resp)
		}
	}

	return nil
}

func (p *BarDevicePlugin) GetPreferredAllocation(ctx context.Context, req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return nil, nil
}

func (p *BarDevicePlugin) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	log.Printf("alloc req: %+v", req)
	// return nil, nil
	resp := pluginapi.AllocateResponse{}
	for _, req := range req.ContainerRequests {
		containerResp := &pluginapi.ContainerAllocateResponse{}
		for _, devID := range req.DevicesIDs {
			log.Printf("device %s allocated", devID)
			// devSpec := &pluginapi.DeviceSpec{HostPath: path.Join("/root/dev", devID)}
			devSpec := &pluginapi.DeviceSpec{
				HostPath:    "/dev/zero",
				Permissions: "r",
			}
			containerResp.Devices = append(containerResp.Devices, devSpec)
		}

		resp.ContainerResponses = append(resp.ContainerResponses, containerResp)
	}

	return &resp, nil
}

func (p *BarDevicePlugin) PreStartContainer(ctx context.Context, req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return nil, nil
}

func runServer(endpoint string) {
}

func main() {
	log.Printf("starting plugin server...")
	log.Printf("starting device plugin server...")
	// 1. start device plugin grpc server
	endpoint := "bar.sock"
	lis, err := net.Listen("unix", path.Join(pluginapi.DevicePluginPath, endpoint))
	if err != nil {
		log.Panicf("error creating unix socket %s: %+v", endpoint, err)
	}
	// defer func() {
	// 	if err := lis.Close(); err != nil {
	// 		log.Printf("error closing device plugin server: %+v", err)
	// 	}
	// }()

	server := grpc.NewServer()
	plugin := &BarDevicePlugin{}
	pluginapi.RegisterDevicePluginServer(server, plugin)
	go func() {
		if err := server.Serve(lis); err != nil {
			log.Fatalf("error starting device plugin server: %+v", err)
		}
	}()

	// 2. register device plugin
	target := "unix:///var/lib/kubelet/device-plugins/kubelet.sock"
	conn, err := grpc.Dial(target, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("error connecting to %s: %+v", target, err)
	}
	// defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	ctx := context.TODO()
	regReq := pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     "bar.sock",
		ResourceName: "foo.org/bar",
	}
	_, err = client.Register(ctx, &regReq)
	if err != nil {
		log.Panicf("error registring device plugin: %+v", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigs
		log.Printf("receive signal: %+v", sig)
		server.Stop()
		lis.Close()
		conn.Close()
		os.Exit(0)
	}()
	time.Sleep(3600 * time.Second)
}
