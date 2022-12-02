package cpudaemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type AllocatorMock struct {
	mock.Mock
}

var _ Allocator = &AllocatorMock{}

func (m *AllocatorMock) takeCpus(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func (m *AllocatorMock) freeCpus(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func (m *AllocatorMock) clearCpus(c Container, s *DaemonState) error {
	args := m.Called(c, s)
	return args.Error(0)
}

func TestNewStaticPolicy(t *testing.T) {
	s := NewStaticPolocy(nil)
	assert.NotNil(t, s)
}

func TestAssignContainerMocked(t *testing.T) {
	a := AllocatorMock{}
	s := NewStaticPolocy(&a)

	// check a new container
	c := Container{
		CID:  "test-contaier",
		PID:  "test-pod",
		Cpus: 42,
		QS:   Guaranteed,
	}
	st := DaemonState{}
	a.On("takeCpus", c, &st).Return(nil)
	err := s.AssignContainer(c, &st)
	assert.Nil(t, err)
	c.QS = BestEffort
	a.On("takeCpus", c, &st).Return(nil)
	err = s.AssignContainer(c, &st)
	assert.Nil(t, err)
	a.AssertNumberOfCalls(t, "takeCpus", 2)
}

func TestDeleteContainerMocked(t *testing.T) {
	a := AllocatorMock{}
	s := NewStaticPolocy(&a)

	// check a new container
	c := Container{
		CID:  "test-contaier",
		PID:  "test-pod",
		Cpus: 42,
		QS:   Guaranteed,
	}
	st := DaemonState{}
	a.On("freeCpus", c, &st).Return(nil)
	assert.Nil(t, s.DeleteContainer(c, &st))
	c.QS = BestEffort
	a.On("freeCpus", c, &st).Return(nil)
	assert.Nil(t, s.DeleteContainer(c, &st))
	a.AssertNumberOfCalls(t, "freeCpus", 2)
}
