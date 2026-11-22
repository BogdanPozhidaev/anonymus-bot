package fileutil

import (
	"fmt"
	"os"
	"os/exec"
)

// StripMetadata uses ImageMagick to write an image without metadata.
// Separate input and output paths let the caller remove the original.
func StripMetadata(inputPath, outputPath string) error {
	cmd := exec.Command("convert", inputPath, "-strip", outputPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("imagemagick convert failed: %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveFile deletes a path and treats a missing file as already removed.
func RemoveFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}
	return nil
}
