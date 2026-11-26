package gallerydl

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type PhotoItem struct {
	Index    int    `json:"index"`
	URL      string `json:"url"`
	Ext      string `json:"ext"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Filesize int64  `json:"filesize,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type GalleryInfo struct {
	Title     string      `json:"title"`
	Author    string      `json:"author"`
	Thumbnail string      `json:"thumbnail"`
	URL       string      `json:"url"`
	Photos    []PhotoItem `json:"photos"`
}

func proxyArgs() []string {
	if p := strings.TrimSpace(os.Getenv("DATAIMPULSE_PROXY_URL")); p != "" {
		return []string{"--proxy", p}
	}
	if p := strings.TrimSpace(os.Getenv("YTDLP_PROXY_URL")); p != "" {
		return []string{"--proxy", p}
	}
	return nil
}

func cookieArgs() []string {
	if path := strings.TrimSpace(os.Getenv("COOKIES_FILE")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return []string{"--cookies", path}
		}
	}
	defaultPath := filepath.Join(os.TempDir(), "saveit_cookies.txt")
	if _, err := os.Stat(defaultPath); err == nil {
		return []string{"--cookies", defaultPath}
	}
	return nil
}

func commonArgs() []string {
	args := []string{"--no-skip", "--no-part"}
	args = append(args, proxyArgs()...)
	args = append(args, cookieArgs()...)
	return args
}

// FetchPhotos queries gallery-dl for image items in a post.
func FetchPhotos(ctx context.Context, mediaURL string) (*GalleryInfo, error) {
	cmdArgs := append([]string{"-j"}, commonArgs()...)
	cmdArgs = append(cmdArgs, "--", mediaURL)

	cmd := exec.CommandContext(ctx, "gallery-dl", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("timed out fetching photo info")
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = "no photos found at this link"
		}
		return nil, errors.New(cleanGalleryError(errMsg))
	}

	return parseJSON(stdout.Bytes(), mediaURL)
}

func parseJSON(data []byte, mediaURL string) (*GalleryInfo, error) {
	var raw [][]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("could not parse photo gallery information")
	}

	info := &GalleryInfo{URL: mediaURL}
	idx := 1

	for _, item := range raw {
		if len(item) < 2 {
			continue
		}
		code, ok := item[0].(float64)
		if !ok {
			continue
		}

		if int(code) == 2 && len(item) >= 2 {
			if meta, ok := item[1].(map[string]any); ok {
				if info.Title == "" {
					if t, ok := meta["title"].(string); ok && t != "" {
						info.Title = t
					}
				}
				if info.Author == "" {
					if u, ok := meta["user"].(string); ok && u != "" {
						info.Author = u
					} else if a, ok := meta["author"].(string); ok && a != "" {
						info.Author = a
					}
				}
			}
			continue
		}

		// Message type 3 in gallery-dl represents a resolved media item
		if int(code) == 3 && len(item) >= 3 {
			imgURL, _ := item[1].(string)
			meta, _ := item[2].(map[string]any)
			if imgURL == "" {
				continue
			}

			p := PhotoItem{
				Index: idx,
				URL:   imgURL,
				Ext:   "jpg",
			}

			if meta != nil {
				if ext, ok := meta["extension"].(string); ok && ext != "" {
					p.Ext = strings.ToLower(ext)
				}
				if fn, ok := meta["filename"].(string); ok {
					p.Filename = fn
				}
				if w, ok := meta["width"].(float64); ok {
					p.Width = int(w)
				}
				if h, ok := meta["height"].(float64); ok {
					p.Height = int(h)
				}
				if sz, ok := meta["filesize"].(float64); ok {
					p.Filesize = int64(sz)
				}
				if info.Title == "" {
					if t, ok := meta["title"].(string); ok && t != "" {
						info.Title = t
					} else if desc, ok := meta["description"].(string); ok && desc != "" {
						info.Title = desc
					}
				}
				if info.Author == "" {
					if u, ok := meta["user"].(string); ok {
						info.Author = u
					} else if a, ok := meta["author"].(string); ok {
						info.Author = a
					} else if a, ok := meta["artist"].(string); ok {
						info.Author = a
					}
				}
			}

			if info.Thumbnail == "" {
				info.Thumbnail = imgURL
			}

			info.Photos = append(info.Photos, p)
			idx++
		}
	}

	if len(info.Photos) == 0 {
		return nil, errors.New("no photos found at this link")
	}

	if info.Title == "" {
		info.Title = fmt.Sprintf("Photo post (%d images)", len(info.Photos))
	}

	return info, nil
}

// DownloadAll fetches all gallery photos to destDir.
func DownloadAll(ctx context.Context, mediaURL, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}

	cmdArgs := append([]string{"--directory", destDir, "--no-mtime"}, commonArgs()...)
	cmdArgs = append(cmdArgs, "--", mediaURL)

	cmd := exec.CommandContext(ctx, "gallery-dl", cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[gallery-dl] download failed: %s", stderr.String())
		return nil, errors.New(cleanGalleryError(stderr.String()))
	}

	var files []string
	_ = filepath.Walk(destDir, func(path string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && fi.Size() > 0 {
			files = append(files, path)
		}
		return nil
	})

	if len(files) == 0 {
		return nil, errors.New("failed to save photos")
	}
	return files, nil
}

// DownloadSingle fetches a specific photo by index (1-based) to destDir.
func DownloadSingle(ctx context.Context, mediaURL string, index int, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	cmdArgs := append([]string{"--directory", destDir, "--no-mtime", "--range", fmt.Sprintf("%d", index)}, commonArgs()...)
	cmdArgs = append(cmdArgs, "--", mediaURL)

	cmd := exec.CommandContext(ctx, "gallery-dl", cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[gallery-dl] single download failed: %s", stderr.String())
		return "", errors.New(cleanGalleryError(stderr.String()))
	}

	var found string
	_ = filepath.Walk(destDir, func(path string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && fi.Size() > 0 && found == "" {
			found = path
		}
		return nil
	})

	if found == "" {
		return "", errors.New("failed to save photo")
	}
	return found, nil
}

// CreateZip compresses files into a target zip archive.
func CreateZip(zipPath string, files []string) error {
	zFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zFile.Close()

	zw := zip.NewWriter(zFile)
	defer zw.Close()

	for _, fPath := range files {
		f, err := os.Open(fPath)
		if err != nil {
			continue
		}
		name := filepath.Base(fPath)
		w, err := zw.Create(name)
		if err == nil {
			_, _ = io.Copy(w, f)
		}
		f.Close()
	}
	return nil
}

func cleanGalleryError(errStr string) string {
	lower := strings.ToLower(errStr)
	switch {
	case strings.Contains(lower, "404") || strings.Contains(lower, "not found"):
		return "This post or photo was not found or has been deleted."
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "login"):
		return "This post is private or requires an account login."
	case strings.Contains(lower, "no suitable extractor"):
		return "This site is not currently supported for photo downloads."
	}
	return "Could not download photos from this link."
}
