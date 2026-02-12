package functions

import "path/filepath"

func GetRlativePath(rootDir, filePath string) string {
	rel, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return filePath
	}
	return filepath.ToSlash(rel)
}
