package accounts_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// pngBytes and jpegBytes are real one-pixel images, and webpBytes is the start of a WebP file: the
// service never decodes an image, so its signature is all a test needs of it.
var (
	pngBytes  = encodeImage(func(b *bytes.Buffer) error { return png.Encode(b, image.NewRGBA(image.Rect(0, 0, 1, 1))) })
	jpegBytes = encodeImage(func(b *bytes.Buffer) error { return jpeg.Encode(b, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil) })
	webpBytes = "RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00"
)

func encodeImage(encode func(*bytes.Buffer) error) string {
	var buf bytes.Buffer
	if err := encode(&buf); err != nil {
		panic(err)
	}
	return buf.String()
}

// avatarFiles lists the files in the avatar folder.
func (e *env) avatarFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.dataDir, "avatars"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the avatar folder: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// An upload is kept in a folder only its owner can read, is served back byte for byte with its own
// kind, and is published with the new profile whose avatar address carries a version.
func TestSetAvatarKeepsTheImage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	profile, err := e.svc.SetAvatar(ctx, e.userID, "image/png", strings.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("SetAvatar: %v", err)
	}
	stored, err := e.svc.Avatar(ctx, e.userID)
	if err != nil {
		t.Fatalf("Avatar: %v", err)
	}
	want := "/v1/users/" + e.userID + "/avatar?v=" + strconv.FormatInt(stored.Version, 10)
	if profile.AvatarURL == nil || *profile.AvatarURL != want {
		t.Errorf("avatarUrl = %v, want %s", profile.AvatarURL, want)
	}
	if stored.ContentType != "image/png" || stored.UpdatedAt.UnixMilli() != stored.Version || !strings.HasSuffix(stored.Path, ".png") {
		t.Errorf("image = %+v", stored)
	}
	data, err := os.ReadFile(stored.Path)
	if err != nil || string(data) != pngBytes {
		t.Errorf("the stored file is not the upload: %v", err)
	}
	dir, _ := os.Stat(filepath.Dir(stored.Path))
	file, _ := os.Stat(stored.Path)
	if dir.Mode().Perm() != 0o700 || file.Mode().Perm() != 0o600 {
		t.Errorf("modes = folder %v, file %v; want 0700 and 0600", dir.Mode().Perm(), file.Mode().Perm())
	}
	if me := e.nextMe(t); me.Profile.AvatarURL == nil || *me.Profile.AvatarURL != want {
		t.Errorf("the event's avatarUrl = %v, want %s", me.Profile.AvatarURL, want)
	}
	if files := e.avatarFiles(t); len(files) != 1 {
		t.Errorf("the avatar folder holds %v, want the one image and no temporary file", files)
	}
}

// A new image replaces the old one: the address changes, the old file goes, and each kind is
// recognized from its bytes.
func TestSetAvatarReplacesTheOldImage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.svc.SetAvatar(ctx, e.userID, "image/png", strings.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("SetAvatar png: %v", err)
	}
	e.nextMe(t)
	second, err := e.svc.SetAvatar(ctx, e.userID, "image/jpeg", strings.NewReader(jpegBytes))
	if err != nil {
		t.Fatalf("SetAvatar jpeg: %v", err)
	}
	e.nextMe(t)
	if *first.AvatarURL == *second.AvatarURL {
		t.Errorf("the address did not change: %s", *second.AvatarURL)
	}
	stored, _ := e.svc.Avatar(ctx, e.userID)
	if stored.ContentType != "image/jpeg" || !strings.HasSuffix(stored.Path, ".jpg") {
		t.Errorf("image = %+v, want the jpeg", stored)
	}
	if files := e.avatarFiles(t); len(files) != 1 || files[0] != filepath.Base(stored.Path) {
		t.Errorf("the avatar folder holds %v, want only the new image", files)
	}
	third, err := e.svc.SetAvatar(ctx, e.userID, "image/webp", strings.NewReader(webpBytes))
	if err != nil {
		t.Fatalf("SetAvatar webp: %v", err)
	}
	if stored, _ := e.svc.Avatar(ctx, e.userID); stored.ContentType != "image/webp" || !strings.HasSuffix(stored.Path, ".webp") {
		t.Errorf("image = %+v, want the webp", stored)
	}
	if third.AvatarURL == nil {
		t.Error("the webp has no address")
	}
}

// Removing the avatar takes the file and the address away and publishes the profile. Removing one
// that is not there is not an error, and publishes nothing.
func TestRemoveAvatar(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetAvatar(ctx, e.userID, "image/png", strings.NewReader(pngBytes)); err != nil {
		t.Fatalf("SetAvatar: %v", err)
	}
	e.nextMe(t)

	profile, err := e.svc.RemoveAvatar(ctx, e.userID)
	if err != nil || profile.AvatarURL != nil {
		t.Fatalf("RemoveAvatar = %+v, %v; want no avatar", profile, err)
	}
	if me := e.nextMe(t); me.Profile.AvatarURL != nil {
		t.Errorf("the event still has an avatar: %v", *me.Profile.AvatarURL)
	}
	if files := e.avatarFiles(t); len(files) != 0 {
		t.Errorf("the avatar folder holds %v, want nothing", files)
	}
	wantCode(t, func() error { _, err := e.svc.Avatar(ctx, e.userID); return err }(),
		protocol.ErrorCodeNotFound, "Marshal cannot find that avatar. It may have been removed.")

	again, err := e.svc.RemoveAvatar(ctx, e.userID)
	if err != nil || again.AvatarURL != nil {
		t.Errorf("RemoveAvatar with none = %+v, %v", again, err)
	}
	e.noEvent(t)
}

// The avatar is in the file and the folder, so a daemon that starts again serves it.
func TestTheAvatarSurvivesARestart(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	saved, err := e.svc.SetAvatar(ctx, e.userID, "image/png", strings.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("SetAvatar: %v", err)
	}
	e.restart(t)
	got, err := e.svc.Profile(ctx, e.userID)
	if err != nil || got.AvatarURL == nil || *got.AvatarURL != *saved.AvatarURL {
		t.Fatalf("after a restart the avatar = %v, %v; want %s", got.AvatarURL, err, *saved.AvatarURL)
	}
	stored, err := e.svc.Avatar(ctx, e.userID)
	if err != nil {
		t.Fatalf("Avatar: %v", err)
	}
	if data, err := os.ReadFile(stored.Path); err != nil || string(data) != pngBytes {
		t.Errorf("the image after a restart: %v", err)
	}
}

// Every upload that is not allowed is refused with a plain sentence, and leaves the old avatar, the
// folder, and the events as they were.
func TestSetAvatarRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const notAnImage = "Marshal accepts PNG, JPEG, and WebP images. Choose one of those."
	const tooLarge = "That image is larger than 2 MB. Choose a smaller one."
	big := pngBytes + strings.Repeat("x", accounts.MaxAvatarBytes)
	tests := []struct {
		name     string
		declared string
		body     string
		text     string
	}{
		{"a GIF", "image/gif", "GIF89a" + strings.Repeat("x", 20), notAnImage},
		{"no kind at all", "", pngBytes, notAnImage},
		{"a kind that is not an image", "text/plain", "hello", notAnImage},
		{"an SVG, which can hold a script", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, notAnImage},
		{"a PNG that says it is a JPEG", "image/jpeg", pngBytes, notAnImage},
		{"a JPEG that says it is a PNG", "image/png", jpegBytes, notAnImage},
		{"a web page that says it is a PNG", "image/png", "<html><body>hello</body></html>", notAnImage},
		{"nothing at all", "image/png", "", "Choose an image to upload."},
		{"an image that is too large", "image/png", big, tooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.SetAvatar(ctx, e.userID, tc.declared, strings.NewReader(tc.body))
			wantCode(t, err, protocol.ErrorCodeInvalidArgument, tc.text)
		})
	}
	// A body that the server cut off at its own limit is the same refusal.
	_, err := e.svc.SetAvatar(ctx, e.userID, "image/png",
		http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(big)), accounts.MaxAvatarBytes))
	wantCode(t, err, protocol.ErrorCodeInvalidArgument, tooLarge)
	if profile, _ := e.svc.Profile(ctx, e.userID); profile.AvatarURL != nil {
		t.Errorf("a refused upload became the avatar: %v", *profile.AvatarURL)
	}
	if files := e.avatarFiles(t); len(files) != 0 {
		t.Errorf("a refused upload left %v in the avatar folder", files)
	}
	e.noEvent(t)
}

// An image of exactly the largest size is kept.
func TestSetAvatarKeepsAnImageOfTheLargestSize(t *testing.T) {
	e := newEnv(t)
	body := pngBytes + strings.Repeat("x", accounts.MaxAvatarBytes-len(pngBytes))
	if len(body) != accounts.MaxAvatarBytes {
		t.Fatalf("the test image is %d bytes, want %d", len(body), accounts.MaxAvatarBytes)
	}
	if _, err := e.svc.SetAvatar(context.Background(), e.userID, "image/png", strings.NewReader(body)); err != nil {
		t.Errorf("SetAvatar of %d bytes: %v", len(body), err)
	}
}

// A read that fails partway is the daemon's problem and not the person's, and nothing is kept.
func TestSetAvatarWhenTheUploadCannotBeRead(t *testing.T) {
	e := newEnv(t)
	broken := errors.New("the connection dropped")
	_, err := e.svc.SetAvatar(context.Background(), e.userID, "image/png", failingReader{err: broken})
	if !errors.Is(err, broken) {
		t.Errorf("SetAvatar = %v, want the read error", err)
	}
	var perr *protocol.Error
	if errors.As(err, &perr) {
		t.Errorf("a failed read was reported to the person as %q", perr.Message)
	}
	if files := e.avatarFiles(t); len(files) != 0 {
		t.Errorf("a failed upload left %v", files)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

// Only a plain file name in the folder is ever served, even if a database row says otherwise.
func TestAvatarNeverLeavesItsFolder(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	stamp := int64(1759233600000)
	for _, name := range []string{"../marshal.db", "/etc/passwd", "sub/dir.png", ".."} {
		err := e.store.Write(ctx, func(q *db.Queries) error {
			_, err := q.SetUserAvatar(ctx, db.SetUserAvatarParams{AvatarPath: name, AvatarUpdatedAt: &stamp, UpdatedAt: stamp, ID: e.userID})
			return err
		})
		if err != nil {
			t.Fatalf("plant %q: %v", name, err)
		}
		_, err = e.svc.Avatar(ctx, e.userID)
		wantCode(t, err, protocol.ErrorCodeNotFound, "Marshal cannot find that avatar. It may have been removed.")
	}
}
