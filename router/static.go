package router

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	immutableCacheControl  = "public,max-age=31536000,immutable"
	revalidateCacheControl = "no-cache"
)

var contentHashPattern = regexp.MustCompile(`(?i)(?:^|[._-])[0-9a-f]{8,}(?:[._-]|$)`)

type staticFile struct {
	content      []byte
	contentType  string
	cacheControl string
	etag         string
	name         string
}

type staticServer struct {
	files map[string]staticFile
	index staticFile
}

func newStaticServer(source fs.FS) (*staticServer, error) {
	if source == nil {
		return nil, nil
	}
	server := &staticServer{files: make(map[string]staticFile)}
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if !safeAssetName(name) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		content, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("read %q: %w", name, err)
		}
		server.files[name] = makeStaticFile(name, content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load web assets: %w", err)
	}
	index, exists := server.files["index.html"]
	if !exists || len(index.content) == 0 {
		return nil, fmt.Errorf("load web assets: non-empty index.html is required")
	}
	server.index = index
	return server, nil
}

func makeStaticFile(name string, content []byte) staticFile {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	cacheControl := revalidateCacheControl
	if name != "index.html" && contentHashPattern.MatchString(path.Base(name)) {
		cacheControl = immutableCacheControl
	}
	digest := sha256.Sum256(content)
	return staticFile{
		content:      content,
		contentType:  contentType,
		cacheControl: cacheControl,
		etag:         fmt.Sprintf(`"%x"`, digest),
		name:         name,
	}
}

func (server *staticServer) serve(c *gin.Context, name string) {
	file, exists := server.files[name]
	if !exists {
		file = server.index
	}
	c.Header("Cache-Control", file.cacheControl)
	c.Header("Content-Type", file.contentType)
	c.Header("ETag", file.etag)
	http.ServeContent(c.Writer, c.Request, file.name, time.Time{}, bytes.NewReader(file.content))
}

func staticAssetName(requestPath string) (string, bool) {
	if requestPath == "" || !strings.HasPrefix(requestPath, "/") || strings.ContainsAny(requestPath, "\\\x00") {
		return "", false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(requestPath, "/"), "/") {
		if segment == "." || segment == ".." || strings.HasPrefix(segment, ".") {
			return "", false
		}
	}
	name := strings.TrimPrefix(path.Clean(requestPath), "/")
	if name == "" || name == "." {
		return "index.html", true
	}
	if !safeAssetName(name) {
		return "", false
	}
	return name, true
}

func safeAssetName(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return true
}

func isOperationalPath(requestPath string) bool {
	for _, endpoint := range []string{"/healthz", "/readyz", "/metrics"} {
		if requestPath == endpoint || strings.HasPrefix(requestPath, endpoint+"/") {
			return true
		}
	}
	return false
}
