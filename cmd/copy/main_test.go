package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyGoodCase(t *testing.T) {
	srcDir, err := os.MkdirTemp(os.TempDir(), "source-*")
	assert.NoError(t, err)
	defer os.RemoveAll(srcDir)
	tempFile, err := os.CreateTemp(srcDir, "file-*")
	assert.NoError(t, err)
	destDir, err := os.MkdirTemp(os.TempDir(), "dest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(destDir)

	actualErr := copyFile(tempFile.Name(), destDir)
	assert.NoError(t, actualErr)
	tempFileName, err := filepath.Rel(srcDir, tempFile.Name())
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(destDir, tempFileName))
	assert.NoError(t, err)
}

func TestCopyNonExistingFile(t *testing.T) {
	srcDir, err := os.MkdirTemp(os.TempDir(), "source-*")
	assert.NoError(t, err)
	defer os.RemoveAll(srcDir)
	tempFilePath := filepath.Join(srcDir, "non-existing-file")
	destDir, err := os.MkdirTemp(os.TempDir(), "dest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(destDir)

	actualErr := copyFile(tempFilePath, destDir)
	assert.NotNil(t, actualErr)
	assert.ErrorContains(t, actualErr, "failed to open source file")
	assert.ErrorContains(t, actualErr, "no such file or directory")
}

func TestCopyToNonExistingDest(t *testing.T) {
	srcDir, err := os.MkdirTemp(os.TempDir(), "source-*")
	assert.NoError(t, err)
	defer os.RemoveAll(srcDir)
	tempFile, err := os.CreateTemp(srcDir, "file-*")
	assert.NoError(t, err)
	destDir := filepath.Join(os.TempDir(), "non-existing-dir")

	actualErr := copyFile(tempFile.Name(), destDir)
	assert.NotNil(t, actualErr)
	assert.ErrorContains(t, actualErr, "failed to open destination file")
	assert.ErrorContains(t, actualErr, "no such file or directory")
}
