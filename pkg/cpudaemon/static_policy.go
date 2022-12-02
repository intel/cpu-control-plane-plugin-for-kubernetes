package cpudaemon

// Policy interface of cpu management policies.
type Policy interface {
	AssignContainer(c Container, s *DaemonState) error
	DeleteContainer(c Container, s *DaemonState) error
	ClearContainer(c Container, s *DaemonState) error
}

// StaticPolicy Static Policy type holding assigned containers.
type StaticPolicy struct {
	allocator Allocator
}

var _ Policy = &StaticPolicy{}

// NewStaticPolocy Construct a new static policy.
func NewStaticPolocy(a Allocator) *StaticPolicy {
	p := StaticPolicy{
		allocator: a,
	}
	return &p
}

// AssignContainer tries to allocate a container.
func (p *StaticPolicy) AssignContainer(c Container, s *DaemonState) error {
	return p.allocator.takeCpus(c, s)
}

// DeleteContainer delete allocated containers (without deleting cgroup config - it will be clered by k8s GC).
func (p *StaticPolicy) DeleteContainer(c Container, s *DaemonState) error {
	return p.allocator.freeCpus(c, s)
}

// ClearContainer reverts cpuset configuration to default one (use all available cpus). It does not
// remove container from the state - this should be done with DeleteContainer.
func (p *StaticPolicy) ClearContainer(c Container, s *DaemonState) error {
	return p.allocator.clearCpus(c, s)
}
