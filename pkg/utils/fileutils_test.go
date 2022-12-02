package utils

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateRealPathOfFile(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	createFile(t, file)

	f, err := EvaluateRealPath(file)
	assert.Nil(t, err)
	assert.Equal(t, file, f)
}

func TestEvaluateRealPathOfSymlink(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	symlink := path.Join(dir, "test-symlink.txt")
	createFile(t, file)
	require.Nil(t, os.Symlink(file, symlink))

	f, err := EvaluateRealPath(symlink)
	assert.Nil(t, err)
	assert.Equal(t, file, f)
}

func TestValidatePathPasses(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	createFile(t, file)
	assert.Nil(t, ValidatePathInsideBase(file, dir))
}

func TestValidatePathSymlinkOutsideBase(t *testing.T) {
	dir := t.TempDir()
	outsideFile := path.Join(dir, "test_outside.txt")
	dir1 := path.Join(dir, "dir1")
	insideSymlink := path.Join(dir1, "test_inside.txt")

	require.Nil(t, os.Mkdir(dir1, 0700))
	createFile(t, outsideFile)
	require.Nil(t, os.Symlink(outsideFile, insideSymlink))

	assert.ErrorIs(t, ValidatePathInsideBase(insideSymlink, dir1), ErrPathNotInBase)
}

func createFile(t *testing.T, path string) {
	f, err := os.Create(path)
	require.Nil(t, err)
	f.Close()
}

func TestReadFileAt(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	createFile(t, file)
	content, err := ReadFileAt(dir, "test.txt")
	assert.Nil(t, err)
	assert.Empty(t, content)
}

func TestValidateReadFileAtFails(t *testing.T) {
	dir := t.TempDir()
	outsideFile := path.Join(dir, "test_outside.txt")
	dir1 := path.Join(dir, "dir1")
	insideSymlink := path.Join(dir1, "test_inside.txt")

	require.Nil(t, os.Mkdir(dir1, 0700))
	createFile(t, outsideFile)
	require.Nil(t, os.Symlink(outsideFile, insideSymlink))

	_, err := ReadFileAt(dir1, "test_inside.txt")
	assert.ErrorIs(t, err, ErrPathNotInBase)
}

func TestNoErrorIfSymlinkNormalFile(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	createFile(t, file)

	assert.Nil(t, ErrorIfSymlink(file))
}

func TestErrorIfSymlinkDoesntExist(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")

	assert.ErrorIs(t, ErrorIfSymlink(file), os.ErrNotExist)
}

func TestErrorIfSymlink(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "test.txt")
	symlink := path.Join(dir, "sym.txt")
	createFile(t, file)
	require.Nil(t, os.Symlink(file, symlink))

	assert.ErrorIs(t, ErrorIfSymlink(symlink), ErrFileIsSymlink)
}
