package authoring

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Upload is one file part handed to the domain by the HTTP layer. The domain
// never trusts the client filename or Content-Type; only Data is used
// (OPS-123, AUT-028).
type Upload struct {
	Filename    string
	ContentType string
	Data        []byte
	ReadErr     error
}

// imageBlob is a validated image ready to be staged or compared.
type imageBlob struct {
	MIME string
	Ext  string
	Data []byte
	Size int
	SHA  [32]byte
}

// safeImageName is the generated-name grammar from AUT-015/AUT-028.
var safeImageName = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|webp)$`)

// extensionForMIME maps the accepted sniffed types to their stored extension.
func extensionForMIME(mime string) (string, bool) {
	switch mime {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}

// mimeForExtension is the inverse mapping used by the publish gate.
func mimeForExtension(ext string) (string, bool) {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".webp":
		return "image/webp", true
	default:
		return "", false
	}
}

// validateUpload enforces the business image limits and sniffs the bytes.
func validateUpload(u Upload) (*imageBlob, string) {
	if u.ReadErr != nil {
		return nil, MsgImageUnreadable
	}
	if len(u.Data) == 0 {
		return nil, MsgImageEmpty
	}
	if len(u.Data) > MaxImageBytes {
		return nil, MsgImageTooLarge
	}
	sniff := u.Data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	mime := http.DetectContentType(sniff)
	ext, ok := extensionForMIME(mime)
	if !ok {
		return nil, MsgImageType
	}
	blob := &imageBlob{
		MIME: mime,
		Ext:  ext,
		Data: u.Data,
		Size: len(u.Data),
		SHA:  sha256.Sum256(u.Data),
	}
	return blob, ""
}

// newImageName mints the unguessable capability name: 128 random bits as 32
// lowercase hex characters plus the extension derived from the sniffed MIME.
func newImageName(ext string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]) + ext, nil
}

// stageImage writes the bytes to a temp file in the uploads root, fsyncs it,
// and atomically renames it to its final unguessable name (AUT-028). The file
// is created before the transaction that references it.
func stageImage(uploadsDir string, blob *imageBlob) (string, error) {
	if blob == nil {
		return "", nil
	}
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		return "", err
	}
	name, err := newImageName(blob.Ext)
	if err != nil {
		return "", err
	}
	final := filepath.Join(uploadsDir, name)
	tmp, err := os.CreateTemp(uploadsDir, ".upload-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(blob.Data); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Chmod(final, 0o644); err != nil {
		_ = os.Remove(final)
		return "", err
	}
	return name, nil
}

// removeImage unlinks one generated image name, tolerating a missing file. It
// refuses anything outside the generated-name grammar, so a corrupt row can
// never turn a cleanup into a path traversal (AUT-030, OPS-061).
func removeImage(uploadsDir, name string) error {
	if !safeImageName.MatchString(name) {
		return errors.New("authoring: refusing to remove a non-generated image name")
	}
	err := os.Remove(filepath.Join(uploadsDir, name))
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// checkImageFile verifies the stored row's file on disk. It returns the flash
// reason for a user-fixable miss, or an error for impossible metadata.
func checkImageFile(uploadsDir, name, mime string, size int64) (string, error) {
	if !safeImageName.MatchString(name) {
		return "", fmt.Errorf("authoring: image path %q is not a generated name", name)
	}
	extMIME, ok := mimeForExtension(filepath.Ext(name))
	if !ok || mime != extMIME {
		return FlashErrImageMissing, nil
	}
	info, err := os.Stat(filepath.Join(uploadsDir, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FlashErrImageMissing, nil
		}
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		return FlashErrImageMissing, nil
	}
	return "", nil
}
