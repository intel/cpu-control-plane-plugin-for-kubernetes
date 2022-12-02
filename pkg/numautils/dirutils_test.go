package numautils

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNameWithPrefixAndNumber(t *testing.T) {
	testCases := []struct {
		name              string
		prefix            string
		expectedIsPrefix  bool
		expectedSuffixNum int
	}{
		{"test12", "test", true, 12},
		{"123", "", true, 123},
		{"xx1t", "xx", false, 0},
		{"test123", "zest", false, 0},
		{"test", "test", false, 0},
	}

	for _, testCase := range testCases {
		name := testCase.name + "_with_prefix_" + testCase.prefix
		t.Run(
			name, func(t *testing.T) {
				isPrefix, suffixNum := getNameWithPrefixAndNumber(testCase.name, testCase.prefix)
				assert.Equal(
					t,
					testCase.expectedIsPrefix,
					isPrefix,
				)
				assert.Equal(
					t,
					testCase.expectedSuffixNum,
					suffixNum,
				)
			},
		)
	}
}

func TestGetEntriesWithPrefixAndNumber(t *testing.T) {
	fileNames := []string{"test1", "xtest", "test5", "test3x", "54"}
	expectedNumbers := []int{1, 5}

	dir, err := os.MkdirTemp("", "dirutils_test")
	assert.Nil(t, err)

	defer os.RemoveAll(dir)

	for _, fileName := range fileNames {
		err = os.Mkdir(path.Join(dir, fileName), 0750)
		assert.Nil(t, err)
	}

	result, err := getEntriesWithPrefixAndNumber(dir, "test")

	assert.Nil(t, err)
	assert.ElementsMatch(t, expectedNumbers, result)
}
