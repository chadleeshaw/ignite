package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"ignite/osimage"
)

const bootFileAllow = "GET, HEAD"

// NewBootFileHandler serves boot files to unauthenticated iPXE clients.
//
// Guarantees:
//   - Only GET and HEAD succeed. Every other method is 405 with Allow: GET, HEAD.
//   - The response is a regular file under <tftpDir>/os-images. The request
//     path is percent-decoded, including repeated encoding, then cleaned.
//     filepath.EvalSymlinks must keep the result inside that directory.
//     Traversal, backslashes, absolute paths, and symlinks that point
//     outside are rejected with 400 or 404 and no file bytes.
//   - A directory, including /os-images and /os-images/<os>/, is 404.
//     There is no index.
//   - A successful response sets Content-Type and Content-Length.
//     HEAD sends those headers and an empty body.
func NewBootFileHandler(tftpDir string) http.Handler {
	return bootFileHandler{root: filepath.Join(tftpDir, osimage.DirName)}
}

type bootFileHandler struct {
	root string
}

// BootFileRequest reports whether r belongs on the public OS image route.
// That includes /os-images, paths under it, and absolute or encoded forms
// of those paths. The router dispatches these with the original path.
func BootFileRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	for _, candidate := range bootRequestTargets(r) {
		cur := candidate
		for i := 0; i < 8; i++ {
			if bootTreeTarget(cur) {
				return true
			}
			next, err := url.PathUnescape(cur)
			if err != nil || next == cur {
				break
			}
			cur = next
		}
	}
	return false
}

func bootRequestTargets(r *http.Request) []string {
	out := bootPathCandidates(r.URL)
	uri := r.RequestURI
	if i := strings.IndexByte(uri, '?'); i >= 0 {
		uri = uri[:i]
	}
	if uri != "" {
		out = append(out, uri)
	}
	return out
}

// bootTreeTarget reports a path that names the OS image tree or an
// absolute URL path such as "//etc/passwd".
func bootTreeTarget(p string) bool {
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "//") {
		return true
	}
	base := "/" + osimage.DirName
	return p == base || strings.HasPrefix(p, base+"/") || strings.HasPrefix(p, base+`\`)
}

func (h bootFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", bootFileAllow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	rel, status := bootFileRel(r)
	if status != 0 {
		rejectBootPath(w, r, status)
		return
	}

	file, info, err := h.openRegular(rel)
	if err != nil {
		rejectBootPath(w, r, bootOpenStatus(err))
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func rejectBootPath(w http.ResponseWriter, r *http.Request, status int) {
	if status == http.StatusBadRequest {
		http.Error(w, http.StatusText(status), status)
		return
	}
	http.NotFound(w, r)
}

var errBootOutside = errors.New("boot file escapes image directory")

func bootOpenStatus(err error) int {
	if errors.Is(err, errBootOutside) {
		return http.StatusBadRequest
	}
	return http.StatusNotFound
}

// openRegular returns a regular file whose resolved path stays under root.
func (h bootFileHandler) openRegular(rel string) (*os.File, os.FileInfo, error) {
	root, err := filepath.Abs(h.root)
	if err != nil {
		return nil, nil, err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}

	full := filepath.Join(root, filepath.FromSlash(rel))
	if !pathInside(root, full) {
		return nil, nil, errBootOutside
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, nil, err
	}
	if !pathInside(root, resolved) {
		return nil, nil, errBootOutside
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, os.ErrNotExist
	}
	return file, info, nil
}

// pathInside reports whether target is a path strictly below root.
// root and target must already be absolute, symlink-resolved paths.
func pathInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// bootFileRel returns the slash-separated path under os-images.
// A non-zero status means the request must be rejected.
func bootFileRel(r *http.Request) (string, int) {
	if r == nil || r.URL == nil {
		return "", http.StatusBadRequest
	}
	for _, candidate := range bootPathCandidates(r.URL) {
		if bootPathForbidden(candidate) {
			return "", http.StatusBadRequest
		}
	}

	decoded, err := decodeBootPath(r.URL.Path)
	if err != nil || bootPathForbidden(decoded) {
		return "", http.StatusBadRequest
	}
	cleaned := path.Clean("/" + strings.TrimLeft(decoded, "/"))
	prefix := "/" + osimage.DirName
	if cleaned == prefix {
		return "", http.StatusNotFound
	}
	if !strings.HasPrefix(cleaned, prefix+"/") {
		return "", http.StatusBadRequest
	}
	rel := strings.TrimPrefix(cleaned, prefix+"/")
	if rel == "" || rel == "." || filepath.IsAbs(rel) {
		return "", http.StatusBadRequest
	}
	return rel, 0
}

func bootPathCandidates(u *url.URL) []string {
	if u == nil {
		return nil
	}
	out := []string{u.Path, u.EscapedPath()}
	if u.RawPath != "" {
		out = append(out, u.RawPath)
	}
	return out
}

// bootPathForbidden reports a path that could escape the image directory
// before or after percent-decoding.
func bootPathForbidden(p string) bool {
	if strings.ContainsRune(p, 0) || strings.Contains(p, "\\") || strings.Contains(p, "..") {
		return true
	}
	if strings.HasPrefix(p, "//") {
		return true
	}
	decoded, err := decodeBootPath(p)
	if err != nil {
		return true
	}
	if decoded != p {
		return bootPathForbidden(decoded)
	}
	return false
}

// decodeBootPath percent-decodes p, repeating while the value still changes,
// so a double-encoded ".." cannot survive as a file name.
func decodeBootPath(p string) (string, error) {
	cur := p
	for i := 0; i < 8; i++ {
		next, err := url.PathUnescape(cur)
		if err != nil {
			return "", err
		}
		if next == cur {
			return cur, nil
		}
		cur = next
	}
	return "", errors.New("boot path encoding is nested too deeply")
}
