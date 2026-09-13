package fileutil

import (
	"fmt"
	"os"
	"os/exec"
)

// StripMetadata запускает ImageMagick для удаления всех метаданных
// (EXIF, IPTC, XMP) из изображения. inputPath и outputPath должны
// быть разными файлами — ImageMagick корректно работает in-place,
// но раздельные пути упрощают последующую гарантированную очистку
// оригинала с метаданными.
func StripMetadata(inputPath, outputPath string) error {
	cmd := exec.Command("convert", inputPath, "-strip", outputPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("imagemagick convert failed: %w, output: %s", err, string(output))
	}

	return nil
}

// RemoveFile удаляет файл с диска, игнорируя ошибку "файл не найден"
// (файл мог быть уже удалён ранее в цепочке обработки).
func RemoveFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}
	return nil
}
