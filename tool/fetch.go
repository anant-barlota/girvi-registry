package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxDownload = 300 << 20

// hashURL downloads a file and returns its SHA-256 and size.
func hashURL(ctx context.Context, c *http.Client, raw string) (sum string, size int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return "", 0, err
	}
	if n > maxDownload {
		return "", 0, fmt.Errorf("larger than %d MB", maxDownload>>20)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// VerifyDownloads checks that every listed program is where the index says and
// matches its checksum and size.
func VerifyDownloads(ctx context.Context, c *http.Client, idx *Index) []string {
	var problems []string
	for _, e := range idx.Plugins {
		for _, v := range e.Versions {
			for _, a := range v.Artifacts {
				sum, size, err := hashURL(ctx, c, a.URL)
				where := fmt.Sprintf("%s %s %s/%s", e.ID, v.Version, a.OS, a.Arch)
				switch {
				case err != nil:
					problems = append(problems, fmt.Sprintf("%s: could not download: %v", where, err))
				case sum != a.SHA256:
					problems = append(problems, fmt.Sprintf("%s: the checksum is %s, not %s", where, sum, a.SHA256))
				case size != a.Size:
					problems = append(problems, fmt.Sprintf("%s: the size is %d, not %d", where, size, a.Size))
				}
			}
		}
	}
	return problems
}
