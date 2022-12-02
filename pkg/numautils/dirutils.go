package numautils

import (
	"os"
	"strconv"
	"strings"
)

// For strings in form prefixX returns X if X is a number.
func getNameWithPrefixAndNumber(name, prefix string) (bool, int) {
	if strings.HasPrefix(name, prefix) {
		nodeID, err := strconv.Atoi(name[len(prefix):])
		if err != nil {
			return false, 0
		}
		return true, nodeID
	}
	return false, 0
}

// Returns list of files/directories with name prefix[0-9]+.
// For example, (file, [file1, file2, filetest, nofile3]) returns [1, 2].
func getEntriesWithPrefixAndNumber(path, prefix string) ([]int, error) {
	dir, err := os.Open(path)
	if err != nil {
		return []int{}, err
	}
	defer dir.Close()

	dirContents, err := dir.Readdirnames(0)
	if err != nil {
		return []int{}, err
	}

	entries := []int{}
	for _, directory := range dirContents {
		if isValid, nodeID := getNameWithPrefixAndNumber(directory, prefix); isValid {
			entries = append(entries, nodeID)
		}
	}
	return entries, nil
}
