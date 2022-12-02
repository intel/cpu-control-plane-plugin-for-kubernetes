package cpudaemon

import (
	"testing"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"

	"github.com/stretchr/testify/assert"
)

func TestGetCPUSetThrowOnMissingGroup(t *testing.T) {
	p := "testdata/"
	b, e := getValues(p, "cpuset.cpus")
	assert.Nil(t, b)
	assert.NotNil(t, e)
}

func TestGetCPUSet(t *testing.T) {
	p := "testdata/no_state"
	b, e := getValues(p, "cpuset.cpus")
	assert.Nil(t, e)
	assert.Equal(t, []ctlplaneapi.CPUBucket{
		{
			StartCPU: 0,
			EndCPU:   127,
		},
	}, b, "Missmatch to expected get cpu value")
}

func TestCPUSetFromBuckets(t *testing.T) {
	buckets := []ctlplaneapi.CPUBucket{
		{StartCPU: 1, EndCPU: 1},
		{StartCPU: 8, EndCPU: 8},
		{StartCPU: 5, EndCPU: 5},
	}
	expectedSet := []int{1, 5, 8}

	assert.Equal(t, expectedSet, CPUSetFromBucketList(buckets).Sorted())
}

func TestCPUSetFromString(t *testing.T) {
	cpuSet, err := CPUSetFromString("1,2-5,7")
	assert.Nil(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 7}, cpuSet.Sorted())
}

func TestCPUSetContains(t *testing.T) {
	cpuSet, err := CPUSetFromString("1,3,6")
	assert.Nil(t, err)

	assert.True(t, cpuSet.Contains(1))
	assert.False(t, cpuSet.Contains(2))
}

func TestCPUSetAdd(t *testing.T) {
	cpuSet := CPUSet{}
	cpuSet.Add(1)

	assert.Equal(t, []int{1}, cpuSet.Sorted())
}

func TestCPUSetRemove(t *testing.T) {
	cpuSet := CPUSet{}
	cpuSet.Add(1)
	cpuSet.Add(2)
	cpuSet.Remove(1)

	assert.Equal(t, []int{2}, cpuSet.Sorted())
}

func TestCPUSetToBucketList(t *testing.T) {
	cpuSet := CPUSet{}
	cpuSet.Add(1)
	cpuSet.Add(3)

	assert.Equal(t, []ctlplaneapi.CPUBucket{{StartCPU: 1, EndCPU: 1}, {StartCPU: 3, EndCPU: 3}}, cpuSet.ToBucketList())
}

func TestCPUSetMerge(t *testing.T) {
	fst, err := CPUSetFromString("1-5")
	assert.Nil(t, err)
	snd, err := CPUSetFromString("4-8")
	assert.Nil(t, err)

	merged := fst.Merge(snd)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8}, merged.Sorted())
	assert.Equal(t, fst, merged) // merge is in-place
}

func TestCPUSetRemoveAll(t *testing.T) {
	fst, err := CPUSetFromString("1-5")
	assert.Nil(t, err)
	snd, err := CPUSetFromString("4-8")
	assert.Nil(t, err)

	removed := fst.RemoveAll(snd)
	assert.Equal(t, []int{1, 2, 3}, removed.Sorted())
	assert.Equal(t, fst, removed) // remove is in-place
}

func TestCPUSetCount(t *testing.T) {
	c := CPUSet{}
	assert.Equal(t, 0, c.Count())
	c.Add(5)
	assert.Equal(t, 1, c.Count())
}

func TestCPUSetClone(t *testing.T) {
	c := CPUSet{}
	c2 := c.Clone()
	c2.Add(5)

	assert.Equal(t, 0, c.Count())
	assert.Equal(t, 1, c2.Count())
}

func TestCPUSetSorted(t *testing.T) {
	c, err := CPUSetFromString("7,4,124,8,1,0")
	assert.Nil(t, err)

	assert.Equal(t, []int{0, 1, 4, 7, 8, 124}, c.Sorted())
}

func TestCPUSetSortedEmpty(t *testing.T) {
	assert.Equal(t, []int{}, CPUSet{}.Sorted())
}

func TestCPUSetToCpuString(t *testing.T) {
	c, err := CPUSetFromString("7,4,124,8,1,0")
	assert.Nil(t, err)

	assert.Equal(t, "0,1,4,7,8,124", c.ToCpuString())
}

func TestCPUSetToCpuStringEmpty(t *testing.T) {
	assert.Equal(t, "", CPUSet{}.ToCpuString())
}

func TestCPUSetFromStringWithNewline(t *testing.T) {
	fst, err := CPUSetFromString("\n")
	assert.Nil(t, err)

	assert.Equal(t, []int{}, fst.Sorted())
}
