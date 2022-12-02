package cpudaemon

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

func getValues(path string, cpusetFileName string) ([]ctlplaneapi.CPUBucket, error) {
	return LoadCpuSet(filepath.Join(path, cpusetFileName))
}

// LoadCpuSet loads and parses cpuset from given path.
func LoadCpuSet(path string) ([]ctlplaneapi.CPUBucket, error) {
	cpus, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadCpuSetFromString(string(cpus))
}

// LoadCpuSetFromString parses cpuset from given string.
func LoadCpuSetFromString(cpuSet string) ([]ctlplaneapi.CPUBucket, error) {
	res := []ctlplaneapi.CPUBucket{}
	cStr := strings.Trim(strings.Trim(cpuSet, " "), "\n")
	if cStr == "" {
		return res, nil
	}
	s := strings.Split(cStr, ",")
	for _, v := range s {
		v = strings.TrimSpace(v)
		c := strings.Split(v, "-")
		a, err := strconv.Atoi(c[0])
		if err != nil {
			return []ctlplaneapi.CPUBucket{}, err
		}
		e := a
		if len(c) > 1 {
			e, err = strconv.Atoi(c[1])
			if err != nil {
				return []ctlplaneapi.CPUBucket{}, err
			}
		}

		b := ctlplaneapi.CPUBucket{
			StartCPU: a,
			EndCPU:   e,
		}
		res = append(res, b)
	}
	return res, nil
}

// CPUSet represents set of cpuids.
type CPUSet map[int]struct{}

func (c CPUSet) String() string {
	return c.ToCpuString()
}

// CPUSetFromBucketList creates CPUSet based on list of ctlplaneapi.CPUBucket.
func CPUSetFromBucketList(buckets []ctlplaneapi.CPUBucket) CPUSet {
	bucketSet := make(CPUSet)
	for _, bucket := range buckets {
		for cpu := bucket.StartCPU; cpu <= bucket.EndCPU; cpu++ {
			bucketSet[cpu] = struct{}{}
		}
	}
	return bucketSet
}

// CPUSetFromString creates CPUSet based on cgroup cpuset string.
func CPUSetFromString(cpuSetStr string) (CPUSet, error) {
	buckets, err := LoadCpuSetFromString(cpuSetStr)
	if err != nil {
		return CPUSet{}, err
	}
	return CPUSetFromBucketList(buckets), nil
}

// Contains checks if given cpuid exists in CPUSet.
func (c CPUSet) Contains(cpu int) bool {
	_, ok := c[cpu]
	return ok
}

// Add adds given cpuid to CPUSet. If it's already added this is noop.
func (c CPUSet) Add(cpu int) {
	c[cpu] = struct{}{}
}

// Remove removes given cpuid from CPUSet. If CPUSet does not contain given cpuid this is noop.
func (c CPUSet) Remove(cpu int) {
	delete(c, cpu)
}

// ToBucketList converts CPUSet back to CPUBucket list, sorted by cpuid.
func (c CPUSet) ToBucketList() []ctlplaneapi.CPUBucket {
	newBuckets := make([]ctlplaneapi.CPUBucket, 0, c.Count())
	for _, cpu := range c.Sorted() {
		newBuckets = append(newBuckets, ctlplaneapi.CPUBucket{StartCPU: cpu, EndCPU: cpu})
	}
	return newBuckets
}

// Merge sums all cpus from two sets.
func (c CPUSet) Merge(other CPUSet) CPUSet {
	for cpu := range other {
		c[cpu] = struct{}{}
	}
	return c
}

// RemoveAll removes all cpus that exist in other.
func (c CPUSet) RemoveAll(other CPUSet) CPUSet {
	for cpu := range other {
		delete(c, cpu)
	}
	return c
}

// Count returns count of cpus in CPUSet.
func (c CPUSet) Count() int {
	return len(c)
}

// Clone returns new CPUSet with same content.
func (c CPUSet) Clone() CPUSet {
	o := CPUSet{}
	for cpu := range c {
		o[cpu] = struct{}{}
	}
	return o
}

// Sorted returns sorted list of cpu ids.
func (c CPUSet) Sorted() []int {
	keys := make([]int, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// ToCpuString converts CPUSet to cgroup cpuset compatible string, sorted by cpuid.
func (c CPUSet) ToCpuString() string {
	if c.Count() == 0 {
		return ""
	}
	b := strings.Builder{}
	for _, cpu := range c.Sorted() {
		b.WriteString(strconv.Itoa(cpu))
		b.WriteString(",")
	}
	result := b.String()
	return result[:len(result)-1]
}
