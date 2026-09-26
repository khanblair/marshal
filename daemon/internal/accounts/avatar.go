package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// bytesPerMiB is the size of a mebibyte, which the avatar limit and its message are in.
	bytesPerMiB = 1 << 20
	// MaxAvatarBytes is the largest avatar image Marshal keeps: 2 MiB.
	MaxAvatarBytes = 2 * bytesPerMiB
	// mediaPNG, mediaJPEG, and mediaWebP are the only kinds of image an avatar can be.
	mediaPNG  = "image/png"
	mediaJPEG = "image/jpeg"
	mediaWebP = "image/webp"
	// The avatar folder holds images only its owner can read, like the database beside it.
	avatarDirMode  = 0o700
	avatarFileMode = 0o600
	// sniffBytes is how much of an upload the type check reads. It is what http.DetectContentType
	// looks at.
	sniffBytes = 512
)

// AvatarImage is an avatar on disk, ready to be served.
type AvatarImage struct {
	// Path is the image file.
	Path string
	// ContentType is the kind of image it is, from the bytes it was stored as.
	ContentType string
	// UpdatedAt is when the image last changed.
	UpdatedAt time.Time
	// Version is the number in the profile's avatar address that changes with the image.
	Version int64
}

// avatarExtension gives the file extension for a kind of image, and says whether it is one an
// avatar can be.
func avatarExtension(mediaType string) (string, bool) {
	switch mediaType {
	case mediaPNG:
		return ".png", true
	case mediaJPEG:
		return ".jpg", true
	case mediaWebP:
		return ".webp", true
	}
	return "", false
}

// mediaTypeOfFile is the kind of image a stored avatar file is, from its extension. The daemon
// chose the name, so the extension is what it wrote after checking the bytes.
func mediaTypeOfFile(name string) string {
	switch filepath.Ext(name) {
	case ".png":
		return mediaPNG
	case ".jpg":
		return mediaJPEG
	case ".webp":
		return mediaWebP
	}
	return ""
}

// SetAvatar keeps an image as the person's avatar and returns their profile. The kind of image is
// found from the bytes, and the kind the client said must agree with it: only PNG, JPEG, and WebP
// are kept, at most MaxAvatarBytes of them. The image is written under a name of its own and only
// then linked, so a failed upload leaves the old avatar in place, and the old file is removed
// once the new one is linked.
func (s *Service) SetAvatar(ctx context.Context, userID, declared string, body io.Reader) (protocol.Profile, error) {
	data, mediaType, err := readAvatar(declared, body)
	if err != nil {
		return protocol.Profile{}, err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	if _, err := readUser(ctx, s.store.Queries(), userID); err != nil {
		return protocol.Profile{}, err
	}
	now := s.now()
	name, err := s.writeAvatarFile(userID, now, mediaType, data)
	if err != nil {
		return protocol.Profile{}, err
	}
	var user, previous db.User
	err = s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		if previous, err = readUser(ctx, q, userID); err != nil {
			return err
		}
		user = previous
		stamp := store.Millis(now)
		user.AvatarPath, user.AvatarUpdatedAt, user.UpdatedAt = name, &stamp, stamp
		return setAvatar(ctx, q, user)
	})
	if err != nil {
		s.removeAvatarFile(name)
		return protocol.Profile{}, err
	}
	if previous.AvatarPath != name {
		s.removeAvatarFile(previous.AvatarPath)
	}
	s.log.Info("changed the avatar", "user_id", userID, "bytes", len(data))
	s.publishMe(ctx, userID)
	return toProfile(user), nil
}

// RemoveAvatar removes the person's avatar and returns their profile. A person with no avatar is
// left as they are, and nothing is published.
func (s *Service) RemoveAvatar(ctx context.Context, userID string) (protocol.Profile, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	var user, previous db.User
	err := s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		if previous, err = readUser(ctx, q, userID); err != nil {
			return err
		}
		user = previous
		if previous.AvatarPath == "" {
			return nil
		}
		user.AvatarPath, user.AvatarUpdatedAt, user.UpdatedAt = "", nil, store.Millis(s.now())
		return setAvatar(ctx, q, user)
	})
	if err != nil {
		return protocol.Profile{}, err
	}
	if previous.AvatarPath == "" {
		return toProfile(user), nil
	}
	s.removeAvatarFile(previous.AvatarPath)
	s.log.Info("removed the avatar", "user_id", userID)
	s.publishMe(ctx, userID)
	return toProfile(user), nil
}

// Avatar finds a user's avatar image. A user with no avatar, and a user that is not there, are both
// not found.
func (s *Service) Avatar(ctx context.Context, userID string) (AvatarImage, error) {
	user, err := readUser(ctx, s.store.Queries(), userID)
	if err != nil {
		return AvatarImage{}, err
	}
	// The name was made by writeAvatarFile, but it is read from a database, so it is checked to be
	// a plain file name before it is joined to the folder.
	if user.AvatarUpdatedAt == nil || !plainFileName(user.AvatarPath) {
		return AvatarImage{}, protocol.NotFound("avatar").With("id", userID)
	}
	return AvatarImage{
		Path:        filepath.Join(s.avatars, user.AvatarPath),
		ContentType: mediaTypeOfFile(user.AvatarPath),
		UpdatedAt:   time.UnixMilli(*user.AvatarUpdatedAt).UTC(),
		Version:     *user.AvatarUpdatedAt,
	}, nil
}

// plainFileName reports whether name is the name of a file in a folder and not a path: no
// separator, and not the folder itself or its parent.
func plainFileName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name
}

// setAvatar writes a user's avatar columns.
func setAvatar(ctx context.Context, q *db.Queries, user db.User) error {
	changed, err := q.SetUserAvatar(ctx, db.SetUserAvatarParams{
		AvatarPath: user.AvatarPath, AvatarUpdatedAt: user.AvatarUpdatedAt, UpdatedAt: user.UpdatedAt, ID: user.ID,
	})
	if err != nil {
		return fmt.Errorf("set the avatar of user %s: %w", user.ID, err)
	}
	if changed == 0 {
		return notFoundUser(user.ID)
	}
	return nil
}

// readAvatar reads an upload and checks it: not empty, no larger than MaxAvatarBytes, and a PNG,
// JPEG, or WebP by its bytes as well as by what the client said. It returns the bytes and the kind.
func readAvatar(declared string, body io.Reader) ([]byte, string, error) {
	if _, ok := avatarExtension(declared); !ok {
		return nil, "", errNotAnImage()
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxAvatarBytes+1))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, "", AvatarTooLarge()
		}
		return nil, "", fmt.Errorf("read the avatar upload: %w", err)
	}
	switch {
	case len(data) == 0:
		return nil, "", protocol.InvalidArgument("Choose an image to upload.")
	case len(data) > MaxAvatarBytes:
		return nil, "", AvatarTooLarge()
	}
	sniffed := http.DetectContentType(data[:min(len(data), sniffBytes)])
	if sniffed != declared {
		return nil, "", errNotAnImage()
	}
	return data, sniffed, nil
}

// AvatarTooLarge is the refusal for an upload over MaxAvatarBytes. The API layer uses it when
// the request says it is too large before the service has read it.
func AvatarTooLarge() *protocol.Error {
	return protocol.InvalidArgument(fmt.Sprintf(
		"That image is larger than %d MB. Choose a smaller one.", MaxAvatarBytes/bytesPerMiB))
}

func errNotAnImage() *protocol.Error {
	return protocol.InvalidArgument("Marshal accepts PNG, JPEG, and WebP images. Choose one of those.")
}

// writeAvatarFile writes the image under a name of its own in the avatar folder and returns the
// name. It goes to a temporary file first and is renamed into place, so the folder never holds half
// an image under a real name.
func (s *Service) writeAvatarFile(userID string, now time.Time, mediaType string, data []byte) (string, error) {
	extension, _ := avatarExtension(mediaType)
	if err := os.MkdirAll(s.avatars, avatarDirMode); err != nil {
		return "", fmt.Errorf("make the avatar folder: %w", err)
	}
	temp, err := os.CreateTemp(s.avatars, ".upload-*")
	if err != nil {
		return "", fmt.Errorf("start the avatar file: %w", err)
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	chmodErr := os.Chmod(tempPath, avatarFileMode)
	if err := errors.Join(writeErr, closeErr, chmodErr); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("write the avatar file: %w", err)
	}
	name := fmt.Sprintf("%s-%d%s", userID, store.Millis(now), extension)
	if err := os.Rename(tempPath, filepath.Join(s.avatars, name)); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("place the avatar file: %w", err)
	}
	return name, nil
}

// removeAvatarFile deletes an avatar file by its name. A file that is already gone is not a
// problem, and one that cannot be deleted is only logged: the avatar it belonged to is already
// replaced, so it is a leftover and never shown.
func (s *Service) removeAvatarFile(name string) {
	if !plainFileName(name) {
		return
	}
	if err := os.Remove(filepath.Join(s.avatars, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warn("could not delete an old avatar file", "file", name, "error", err)
	}
}
