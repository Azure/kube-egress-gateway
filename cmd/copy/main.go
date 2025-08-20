package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// copyFile copies file to destination directory
func copyFile(sourceFile, destDir string) error {
	file, err := os.Open(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Failed to close source file: %v\n", err)
		}
	}()

	sourceDir := filepath.Dir(sourceFile)
	fileName, err := filepath.Rel(sourceDir, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to calculate file name: %w", err)
	}
	destFilePath := filepath.Join(destDir, fileName)
	destFile, err := os.OpenFile(destFilePath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			fmt.Printf("Failed to close destination file: %v\n", err)
		}
	}()

	_, err = io.Copy(destFile, file)
	if err != nil {
		return fmt.Errorf("failed to copy %q to %q: %w", sourceFile, destFilePath, err)
	}
	fmt.Printf("copied %q to %q\n", sourceFile, destFilePath)
	return nil
}

func main() {
	var src, dst string
	var rootCmd = &cobra.Command{
		Use:   "copy",
		Short: "Copy file from source to destination directory",
		Long:  "Copy file from source to destination directory. NOTE that it will not create directory if destination does not exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return copyFile(src, dst)
		},
	}

	rootCmd.Flags().StringVarP(&src, "source", "s", "", "The path of file to copy")
	rootCmd.Flags().StringVarP(&dst, "destination", "d", "", "Destination directory")
	rootCmd.MarkFlagRequired("source")      //nolint:errcheck
	rootCmd.MarkFlagRequired("destination") //nolint:errcheck

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
