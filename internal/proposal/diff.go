package proposal

import "github.com/Wuerike/git-push-memories/internal/unifieddiff"

func unifiedDiff(filePath string, before, after []byte) string {
	return unifieddiff.Between(filePath, before, after)
}

func createPageDiff(filePath string, after []byte) string {
	return unifieddiff.Create(filePath, after)
}
