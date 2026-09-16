package main

import (
	"compress/gzip"
	"encoding/json"
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

type cacheEntry struct {
	ETag string `json:"etag"`
	Data []byte `json:"data"`
}

func getCacheFilePath(url string) string {
	h := fnv.New128a()
	h.Write([]byte(url))
	filename := fmt.Sprintf("%x.json.gz", h.Sum(nil))
	return filepath.Join(cacheDirPath, filename)
}

func loadFromCache(url string) (data []byte, etag string, found bool) {
	if !useCache {
		return nil, "", false
	}

	cachePath := getCacheFilePath(url)
	file, err := os.Open(cachePath)
	if err != nil {
		return nil, "", false
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, "", false
	}
	defer gzReader.Close()

	compressed, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, "", false
	}

	var entry cacheEntry
	if err := json.Unmarshal(compressed, &entry); err != nil {
		return nil, "", false
	}

	return entry.Data, entry.ETag, true
}

func saveToCache(url string, data []byte, etag string) error {
	if !useCache {
		return nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDirPath, 0755); err != nil {
		return err
	}

	entry := cacheEntry{
		ETag: etag,
		Data: data,
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
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

	_, err = gzWriter.Write(entryData)
	return err
}
