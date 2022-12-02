package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
	"resourcemanagement.controlplane/pkg/numautils"
	"resourcemanagement.controlplane/pkg/utils"

	"resourcemanagement.controlplane/pkg/cpudaemon"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const defaultDaemonPort = 31000

var (
	ctlPlaneClient ctlplaneapi.ControlPlaneClient
)

type ctlParameters struct {
	daemonPort      int         // ctlplane daemon port
	memoryPinning   bool        // also do memory pinning
	runtime         string      // container runtime
	cgroupPath      string      // path to the system cgroup fs
	nodeName        string      // agent node name
	numaPath        string      // path to the sysfs node info
	statePath       string      // path to the state file
	allocator       string      // allocator to use
	namespacePrefix string      // required namespace prefix
	cgroupDriver    string      // either cgroupfs or systemd
	logger          logr.Logger // logger
}

func readNumberFromCommandOrPanic(cmd, prefix string) int {
	numNamespaces, err := strconv.Atoi(cmd[len(prefix)+1:])
	if err != nil {
		klog.Fatalf("cannot read number of namespaces %s. format is %s=[0-9]+", cmd, prefix)
	}
	if numNamespaces <= 0 {
		klog.Fatalf("number of namespaces must be greater than 0. it is %d", numNamespaces)
	}
	return numNamespaces
}

func getAllocator(args ctlParameters) cpudaemon.Allocator {
	cR := parseRuntime(args.runtime)
	driver := parseCGroupDriver(args.cgroupDriver)

	cgroupController := cpudaemon.NewCgroupController(cR, driver, args.logger)

	if args.allocator == "default" {
		if args.memoryPinning {
			klog.Fatal("option 'use memory pinning' is available only for numa-aware allocators")
		}
		return cpudaemon.NewDefaultAllocator(cgroupController)
	}
	if args.allocator == "numa" {
		return cpudaemon.NewNumaAwareAllocator(cgroupController, args.memoryPinning)
	}
	if strings.HasPrefix(args.allocator, "numa-namespace=") {
		numNamespaces := readNumberFromCommandOrPanic(args.allocator, "numa-namespace")
		return cpudaemon.NewNumaPerNamespaceAllocator(
			numNamespaces,
			cgroupController,
			false,
			args.memoryPinning,
			args.logger,
		)
	}
	if strings.HasPrefix(args.allocator, "numa-namespace-exclusive=") {
		numNamespaces := readNumberFromCommandOrPanic(args.allocator, "numa-namespace-exclusive")
		return cpudaemon.NewNumaPerNamespaceAllocator(
			numNamespaces,
			cgroupController,
			true,
			args.memoryPinning,
			args.logger,
		)
	}
	klog.Fatalf("unknown allocator %s", args.allocator)
	return nil
}

func parseRuntime(runtime string) cpudaemon.ContainerRuntime {
	val, ok := map[string]cpudaemon.ContainerRuntime{
		"containerd": cpudaemon.ContainerdRunc,
		"kind":       cpudaemon.Kind,
		"docker":     cpudaemon.Docker,
	}[runtime]
	if !ok {
		klog.Fatalf("unknown runtime %s", runtime)
	}
	return val
}

func parseCGroupDriver(driver string) cpudaemon.CGroupDriver {
	val, ok := map[string]cpudaemon.CGroupDriver{
		"systemd":  cpudaemon.DriverSystemd,
		"cgroupfs": cpudaemon.DriverCgroupfs,
	}[driver]
	if !ok {
		klog.Fatalf("unknown cgroup driver %s", driver)
	}
	return val
}

func runDaemon(args ctlParameters) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", args.daemonPort))
	if err != nil {
		klog.Fatal(err.Error())
	}

	srv := grpc.NewServer()
	allocator := getAllocator(args)
	policy := cpudaemon.NewStaticPolocy(allocator)

	args.logger.Info(
		"starting control plane server",
		"nodeName",
		args.nodeName,
		"allocator",
		args.allocator,
		"policy",
		"static",
	)

	daemon, err := cpudaemon.New(args.cgroupPath, args.numaPath, args.statePath, policy, args.logger)
	if err != nil {
		klog.Fatal(err)
	}

	svc := ctlplaneapi.NewServer(daemon)
	healthSvc := health.NewServer()

	ctlplaneapi.RegisterControlPlaneServer(srv, svc)
	grpc_health_v1.RegisterHealthServer(srv, healthSvc) //nolint: nosnakecase

	err = srv.Serve(l)
	if err != nil {
		klog.Fatal(err)
	}
}

func runAgentMode(args ctlParameters) {
	if os.Getenv("NODE_NAME") != "" {
		args.nodeName = os.Getenv("NODE_NAME")
	} else if args.nodeName == "" {
		klog.Fatal("Running in agent mode with unknown agent node name!")
	}
	runAgent(args.daemonPort, args.nodeName, args.namespacePrefix, args.logger)
}

func createLogger() logr.Logger {
	flags := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Parse([]string{"-v", "3"})
	return klogr.NewWithOptions(klogr.WithFormat(klogr.FormatKlog))
}

// normalizePath returns absolute path with symlinks evaluated.
func normalizePath(path string, notExistOk bool) string {
	realPath, err := utils.EvaluateRealPath(path)
	if err != nil {
		if notExistOk && errors.Is(err, os.ErrNotExist) { // file does not exist,
			return path
		}
		klog.Fatal(err)
	}
	return realPath
}

func main() {
	args := ctlParameters{}
	agentMode := false

	flag.BoolVar(&agentMode, "a", false, "Run Controlplane agent")
	flag.BoolVar(
		&args.memoryPinning,
		"mem",
		false,
		"Pin memory togeter with cpu (valid only for numa-aware allocators)",
	)
	flag.IntVar(&args.daemonPort, "dport", defaultDaemonPort, "Specify Control Plane Daemon port")
	flag.StringVar(
		&args.allocator,
		"allocator",
		"default",
		"Allocator to use. Available are: default, numa, numa-namespace=NUM_NAMESPACES",
	)
	flag.StringVar(&args.cgroupPath, "cpath", "/sys/fs/cgroup/", "Specify Path to cgroupds")
	flag.StringVar(&args.numaPath, "npath", numautils.LinuxTopologyPath, "Specify Path to sysfs node info")
	flag.StringVar(&args.statePath, "spath", "daemon.state", "Specify path to state file")
	flag.StringVar(&args.nodeName, "agent-host", "", "Agent node name")
	flag.StringVar(&args.namespacePrefix, "namespace-prefix", "", "If set, serves only namespaces with given prefix")
	flag.StringVar(
		&args.runtime,
		"runtime",
		"containerd",
		"Container Runtime (Default: containerd, Possible values: containerd, docker, kind)",
	)
	flag.StringVar(&args.cgroupDriver, "cgroup-driver", "systemd", "Set cgroup driver used by kubelet. Values: systemd, cgroupfs")

	flag.Parse() // after declaring flags we need to call it
	args.logger = createLogger()

	defer func() {
		err := recover()
		if err != nil {
			args.logger.Info("Fatal error", "value", err)
		}
	}()

	args.cgroupPath = normalizePath(args.cgroupPath, false)
	args.numaPath = normalizePath(args.numaPath, false)
	args.statePath = normalizePath(args.statePath, true)

	switch {
	case agentMode:
		runAgentMode(args)
	default:
		runDaemon(args)
	}
}
