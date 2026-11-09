package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

// workspaceIdentity gives aliases of one physical project folder the same lock identity. Unix
// file metadata exposes the device and inode; Windows file metadata may expose a volume serial and
// file index. Other platforms fall back to the canonical path supplied by canonicalWorkspace.
func workspaceIdentity(root string) (string, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project folder: %w", err)
	}
	root, err = filepath.Abs(canonical)
	if err != nil {
		return "", fmt.Errorf("resolve project folder: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat project folder: %w", err)
	}
	value := reflect.ValueOf(info.Sys())
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if identity, ok := metadataIdentity(value); ok {
		return identity, nil
	}
	return "path:" + filepath.Clean(root), nil
}

func metadataIdentity(value reflect.Value) (string, bool) {
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return "", false
	}
	if identity, ok := unixMetadataIdentity(value); ok {
		return identity, true
	}
	return windowsMetadataIdentity(value)
}

func unixMetadataIdentity(value reflect.Value) (string, bool) {
	dev, devOK := integerField(value, "Dev")
	ino, inoOK := integerField(value, "Ino")
	if !devOK || !inoOK {
		return "", false
	}
	return "inode:" + dev + ":" + ino, true
}

func windowsMetadataIdentity(value reflect.Value) (string, bool) {
	volume, volumeOK := integerField(value, "VolumeSerialNumber")
	hi, hiOK := integerField(value, "FileIndexHigh")
	lo, loOK := integerField(value, "FileIndexLow")
	if !volumeOK || !hiOK || !loOK {
		return "", false
	}
	return "fileid:" + volume + ":" + hi + ":" + lo, true
}

func integerField(value reflect.Value, name string) (string, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() {
		return "", false
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(field.Uint(), 10), true
	default:
		return "", false
	}
}

func acquireWorkspaceLock(identity, configuredDir string) (*platform.Lock, error) {
	dir := configuredDir
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("find the user's config folder for project locks: %w", err)
		}
		dir = filepath.Join(configDir, "Marshal", "workspace-locks")
	}
	digest := sha256.Sum256([]byte(identity))
	name := strings.ToLower(hex.EncodeToString(digest[:])) + ".lock"
	return platform.AcquireLock(filepath.Join(dir, name))
}
