# CPU Control Plane Plugin for Kubernetes

## Requirements: 
* Ubuntu 20.04
* Docker: 20.10.14
* Containerd with systemd: 1.5.11 or newer 
* Kubernetes 1.23 or newer

The CPU Control Plane Plugin is a k8s commponent which enables 
fine granular control of CPU resources in terms of cpu and/or memory pinning.
The component consists of two parts:
* a privileged daemonset responsible for control of the cgroups for a given set of pods and containers
* a agent responsible for watching pods CRUD events

The current release supports [different allocation strategies](#policies) for guaranteed, best-effort and burstable containers. 

## Installation:  

To proceed with installation:
1. Run `make image` -- this will create a docker image
2. Push created image to preffered registry
3. Change registry path in `manifest/ctlplane-daemon.yaml`
4. Install required components and configurations by invoking `kubectl apply -f manifest/ctlplane-daemon.yaml`

> **NOTE**: The controlplane component requires admin privileges to function properly. 
> Those are installed by default for the `ctlplane` 

## CPU policies:

The `allocator` flag currently supports four policies:

* **default** this policy assings each guaranteed container to exclusive subset of cpus. Cpus are taken sequentially
(0, 1, 2, ...) from list of available cpus. Guaranteed and best-effort containers are not pinned.

* **numa** this policy assings each guaranteed container to exclusive subset of cpus with minimal topology distance.
Burstable and best-effort containers are not pinned.

* **numa-namespace:<number-of-namespaces>** this policy will isolate each namespace in separate NUMA zones.
It is required that the system supports a sufficient number of NUMA zones to assign separate zones to 
each namespace. Guaranteed container's cpus are shared with burstable and best-effort containers, but not
with other guaranteed containers.

* **numa-namespace-exclusive:<number-of-namespaces>** same as numa-namespace, except it assigns excusive cpus
to Guaranteed pods (they are not shared with burstable and best-effort containers)


## Configuration options:

### CPU policy:
The policies can be switched inside the `ctlplane-daemon.yaml` in the `ctlplane-daemonset` container my modifying `allocator` flag: 

```
name: ctlplane-daemonset
(...)
args: [(...), "-allocator", "numa-namespace=2"]
```

This configuration will use **numa-namespace** with 2 namespaces supported at a given time.


### Memory pinning:
User can enable memory pinning when using NUMA-aware allocators. This can be done by invoking ctlplane daemon with `-mem` option
```
name: ctlplane-daemonset
(...)
args: [(...), "-allocator", "numa-namespace=2", "-mem"]
```

### CGroup driver:
User can select which cgroup driver is used by the cluster. This can be done by invoking ctlplane daemon with `-cgroup-driver DRIVER` option, where `DRIVER` can be either `systemd` or `cgroupfs`. `systemd` is default option if not present.
```
name: ctlplane-daemonset
(...)
args: [(...), "-cgroup-driver", "cgroupfs"]
```

### Container runtime:
User can select which container runtime is used by the cluster. This can by done by invoking ctlplane daemon with `-runtime RUNTIME` option, where `RUNTIME`  can be either `containerd`, `docker`. Additionaly we support `kind`, as container runtime to be used when kind is used to setup cluster.
```
name: ctlplane-daemonset
(...)
args: [(...), "-runtime", "containerd"]
```


### Agent namespace filter:
The agent can be configured to listen only to CRUD events inside namespaces with given prefix. This can be configured inside `ctlplane-daemon.yaml` in the `ctlplane-agent` container.

```
name: ctlplane-agent
(...)
args: [(...) -namespace-prefix", "test-"]
```

### Other options

| Parameter | Possible values | Description | Used by |
| - | - | - | - |
| `-dport` | 0..65353 | Port used by the daemon gRPC server | daemon & agent |
| `-cpath` | string | path to cgroups main directory, usually /sys/fs/cgroup | daemon |
| `-npath` | string | path to sysfs node info, usually /sys/devices/system/node | daemon |
| `-spath` | string | path to daemon state file | daemon |
| `-agent-host` | string | hostname used by the agent, if environment variable `NODE_NAME` is set, this option is overriten | agent |

## How to invoke unit tests

1. Invoke `make utest`

## How to invoke integration tests

1. Deploy CPU control plane **daemon** with `numa-namespace-exclusive=2` allocator, and **agent** with `-namespace-prefix test-`
2. Invoke `make itest`