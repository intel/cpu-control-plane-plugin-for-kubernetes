// Package utils holds varius utilities functions used across other packages.
package utils

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrPathNotInBase = errors.New("final path goes outside base directory")
	ErrFileIsSymlink = errors.New("file cannot be a symlink")
)

// EvaluateRealPath returns absolute path with symlinks evaluated.
func EvaluateRealPath(path string) (string, error) {
	pathEval, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, err
	}
	pathAbs, err := filepath.Abs(pathEval)
	if err != nil {
		return path, err
	}
	return pathAbs, nil
}

// ValidatePathInsideBase checks if given path, after evaluating all symbolic links does not go outside baseDir.
func ValidatePathInsideBase(filePath string, baseDir string) error {
	absRealPath, err := EvaluateRealPath(filePath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absRealPath, baseDir) {
		return ErrPathNotInBase
	}
	return nil
}

// ReadFileAt reads file contents only if target file is inside baseDir.
func ReadFileAt(baseDir string, fileName string) ([]byte, error) {
	filePath := path.Join(baseDir, fileName)
	if err := ValidatePathInsideBase(filePath, baseDir); err != nil {
		return []byte{}, err
	}
	return os.ReadFile(filePath)
}

// ErrorIfSymlink returns an error if path is symlink or doesn't exist.
func ErrorIfSymlink(path string) error {
	finfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if finfo.Mode()&fs.ModeSymlink != 0 {
		return ErrFileIsSymlink
	}
	return nil
}
