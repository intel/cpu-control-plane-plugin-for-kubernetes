// Package ctlplaneapi creates a control plane api grpc server
package ctlplaneapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CPUBucket A cpu bucket describes a bucket of cpus by a given start CPU ID and
// end CPU ID. The bucket includes all cpus in the range
// [start CPU ID - end CPU ID].
type CPUBucket struct {
	StartCPU int
	EndCPU   int
}

// AllocatedContainerResource represents single container allocation.
type AllocatedContainerResource struct {
	ContainerID string
	CPUSet      []CPUBucket
}

// AllocatedPodResources repesents pod allocation, together with container sub-allocation.
type AllocatedPodResources struct {
	CPUSet             []CPUBucket
	ContainerResources []AllocatedContainerResource
}

// CtlPlane is a interface to be implmented by the Daemon.
type CtlPlane interface {
	// Creates a pod with given resource allocation for the parent pod and all
	CreatePod(req *CreatePodRequest) (*AllocatedPodResources, error)
	// Deletes pod and children containers allocations
	DeletePod(req *DeletePodRequest) error
	// Creates a pod with given resource allocation for the parent pod and all
	UpdatePod(req *UpdatePodRequest) (*AllocatedPodResources, error)
}

// Server implements CtlPlane GRPC Server protocol.
type Server struct {
	UnimplementedControlPlaneServer
	ctl CtlPlane
}

// NewServer initializes new ctlplaneapi.Server.
func NewServer(c CtlPlane) *Server {
	return &Server{
		ctl: c,
	}
}

// DeletePod deletes pod from allocator.
func (d *Server) DeletePod(ctx context.Context, cP *DeletePodRequest) (*PodAllocationReply, error) {
	if err := d.ctl.DeletePod(cP); err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	reply := PodAllocationReply{
		PodId:      cP.PodId,
		AllocState: AllocationState_DELETED,
	}
	return &reply, nil
}

// CreatePod creates pod inside allocator.
func (d *Server) CreatePod(ctx context.Context, cP *CreatePodRequest) (*PodAllocationReply, error) {
	podResources, err := d.ctl.CreatePod(cP)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	reply := PodAllocationReply{
		PodId:      cP.PodId,
		CpuSet:     toGRPCHelper4CPUSet(podResources.CPUSet),
		AllocState: AllocationState_CREATED,
	}
	return &reply, nil
}

// UpdatePod reallocates all changed containers of a pod.
func (d *Server) UpdatePod(ctx context.Context, cP *UpdatePodRequest) (*PodAllocationReply, error) {
	podResources, err := d.ctl.UpdatePod(cP)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	reply := PodAllocationReply{
		PodId:      cP.PodId,
		CpuSet:     toGRPCHelper4CPUSet(podResources.CPUSet),
		AllocState: AllocationState_UPDATED,
	}
	return &reply, nil
}

func toGRPCHelper4CPUSet(b []CPUBucket) []*CPUSet {
	res := []*CPUSet{}
	for _, it := range b {
		res = append(res,
			&CPUSet{
				StartCPU: int32(it.StartCPU),
				EndCPU:   int32(it.EndCPU),
			})
	}
	return res
}
