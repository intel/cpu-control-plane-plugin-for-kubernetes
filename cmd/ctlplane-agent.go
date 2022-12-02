package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"resourcemanagement.controlplane/pkg/agent"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

func runAgent(daemonPort int, nodeName string, namespacePrefix string, logger logr.Logger) {
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatal(err)
	}
	clusterClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}

	logger.Info("connecting to ctlplane daemon gRPC", "address", "localhost", "port", daemonPort)
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", daemonPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		klog.Fatal(err)
	}
	defer conn.Close()

	ctlPlaneClient = ctlplaneapi.NewControlPlaneClient(conn)
	ctx, ctxCancel := context.WithCancel(logr.NewContext(context.Background(), logger))
	defer ctxCancel()

	agent := agent.NewAgent(ctx, ctlPlaneClient, namespacePrefix)
	if err := agent.Run(clusterClient, nodeName); err != nil {
		klog.Fatal(err)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
}
