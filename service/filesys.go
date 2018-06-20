package service

import (
	"github.com/lectio/lectiod/graph"
	"github.com/peterbourgon/diskv"
	// github.com/rcrowley/go-metrics
)

// FileStorage stores Lectio content in the file system in a pseudo key-value pattern style
type FileStorage struct {
	config graph.FileStorageConfiguration
	diskv  *diskv.Diskv
}

// NewFileStorage that can persist content
func NewFileStorage(basePath string) *FileStorage {
	result := new(FileStorage)
	result.config.BasePath = basePath

	// Simplest transform function: put all the data files into the base dir.
	flatTransform := func(s string) []string { return []string{} }

	// Initialize a new diskv store, rooted at "my-data-dir", with a 1MB cache.
	result.diskv = diskv.New(diskv.Options{
		BasePath:     basePath,
		Transform:    flatTransform,
		CacheSizeMax: 1024 * 1024,
	})

	return result
}
