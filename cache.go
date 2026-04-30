package main

import (
	"compress/gzip"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
)

var (
	useCache     bool
	cacheDirPath string
)

func getCacheFilePath(url string) string {
	h := fnv.New128a()
	h.Write([]byte(url))
	filename := fmt.Sprintf("%x.json.gz", h.Sum(nil))
	return filepath.Join(cacheDirPath, filename)
}

func loadFromCache(url string) ([]byte, bool) {
	if !useCache {
		return nil, false
	}

	cachePath := getCacheFilePath(url)
	file, err := os.Open(cachePath)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, false
	}
	defer gzReader.Close()

	data, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, false
	}
	return data, true
}

func saveToCache(url string, data []byte) error {
	if !useCache {
		return nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDirPath, 0755); err != nil {
		return err
	}

	cachePath := getCacheFilePath(url)
	file, err := os.Create(cachePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	_, err = gzWriter.Write(data)
	return err
}
