package accounts

import (
	"context"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// maxNameChars is the longest name a person can have.
	maxNameChars = 100
	// maxEmailChars is the longest email address there is (RFC 5321).
	maxEmailChars = 254
	// maxInitials is how many letters of a name the avatar shows when there is no image.
	maxInitials = 2
	// localZone is the name Go gives the machine's own zone. It is refused because it means a
	// different zone on each machine, and a brief must run in the one the person chose.
	localZone = "Local"
)

// Profile returns the person's profile.
func (s *Service) Profile(ctx context.Context, userID string) (protocol.Profile, error) {
	user, err := readUser(ctx, s.store.Queries(), userID)
	if err != nil {
		return protocol.Profile{}, err
	}
	return toProfile(user), nil
}

// Users returns everyone who can be put on a card, by name. Solo use lists the owner only.
func (s *Service) Users(ctx context.Context) (protocol.UserListSnapshot, error) {
	rows, err := s.store.Queries().ListUsers(ctx)
	if err != nil {
		return protocol.UserListSnapshot{}, fmt.Errorf("list the users: %w", err)
	}
	users := make([]protocol.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, toUser(row))
	}
	return protocol.UserListSnapshot{Users: users, ServerTime: protocol.NewTimestamp(s.now())}, nil
}

// UpdateProfile changes the fields the request sets and leaves the rest alone. A request that
// changes nothing answers with the profile as it is and publishes nothing.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in protocol.UpdateProfileRequest) (protocol.Profile, error) {
	in, err := checkProfileUpdate(in)
	if err != nil {
		return protocol.Profile{}, err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	var (
		row     db.User
		changed bool
	)
	err = s.store.Write(ctx, func(q *db.Queries) error {
		current, err := readUser(ctx, q, userID)
		if err != nil {
			return err
		}
		row = applyProfileUpdate(current, in)
		if row == current {
			return nil
		}
		row.UpdatedAt = store.Millis(s.now())
		changed = true
		changedRows, err := q.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
			Name: row.Name, Email: row.Email, TimeZone: row.TimeZone, UpdatedAt: row.UpdatedAt, ID: userID,
		})
		if err != nil {
			return fmt.Errorf("update user %s: %w", userID, err)
		}
		if changedRows == 0 {
			return notFoundUser(userID)
		}
		return nil
	})
	if err != nil {
		return protocol.Profile{}, err
	}
	if changed {
		s.log.Info("edited the profile", "user_id", userID)
		s.publishMe(ctx, userID)
	}
	return toProfile(row), nil
}

// applyProfileUpdate sets the fields the request names on a copy of the user. The request has
// already been checked and trimmed.
func applyProfileUpdate(user db.User, in protocol.UpdateProfileRequest) db.User {
	if in.Name != nil {
		user.Name = *in.Name
	}
	if in.Email != nil {
		user.Email = *in.Email
	}
	if in.TimeZone != nil {
		user.TimeZone = *in.TimeZone
	}
	return user
}

// checkProfileUpdate refuses a profile change that is not allowed and returns the request with its
// text trimmed, so what is stored is what was checked.
func checkProfileUpdate(in protocol.UpdateProfileRequest) (protocol.UpdateProfileRequest, error) {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if err := checkName(name); err != nil {
			return in, err
		}
		in.Name = &name
	}
	if in.Email != nil {
		email := strings.TrimSpace(*in.Email)
		if err := checkEmail(email); err != nil {
			return in, err
		}
		in.Email = &email
	}
	if in.TimeZone != nil {
		zone := strings.TrimSpace(*in.TimeZone)
		if err := checkTimeZone(zone); err != nil {
			return in, err
		}
		in.TimeZone = &zone
	}
	return in, nil
}

func checkName(name string) error {
	switch {
	case name == "":
		return protocol.InvalidArgument("Enter a name. It shows on cards you comment on.")
	case utf8.RuneCountInString(name) > maxNameChars:
		return protocol.InvalidArgument(fmt.Sprintf("A name can have at most %d characters.", maxNameChars))
	}
	return nil
}

// checkEmail accepts no address, or one plain address. A name in front of it ("Ada <ada@x.com>") is
// refused, because the address is used as it is to send briefs.
func checkEmail(email string) error {
	if email == "" {
		return nil
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > maxEmailChars {
		return protocol.InvalidArgument("That does not look like an email address. Check it and try again.")
	}
	return nil
}

// checkTimeZone accepts no zone, or the name of a zone in the time zone database, such as
// "Europe/London".
func checkTimeZone(zone string) error {
	if zone == "" {
		return nil
	}
	if _, err := time.LoadLocation(zone); err != nil || zone == localZone {
		return protocol.InvalidArgument("Marshal does not know that time zone. Choose one from the list.").With("timeZone", zone)
	}
	return nil
}

// initialsOf is up to two capital letters, the first of each of the first two words of a name. It
// is what the avatar shows when there is no image.
func initialsOf(name string) string {
	letters := make([]rune, 0, maxInitials)
	for _, word := range strings.Fields(name) {
		first, _ := utf8.DecodeRuneInString(word)
		letters = append(letters, unicode.ToUpper(first))
		if len(letters) == maxInitials {
			break
		}
	}
	return string(letters)
}

// avatarURL is where a user's avatar is served, or nil when there is none. The version is when the
// image last changed, so a new image is a new address and is never served from a stale cache.
func avatarURL(user db.User) *string {
	if user.AvatarPath == "" || user.AvatarUpdatedAt == nil {
		return nil
	}
	url := "/v1/users/" + user.ID + "/avatar?v=" + strconv.FormatInt(*user.AvatarUpdatedAt, 10)
	return &url
}

// toProfile builds the wire profile from a user row.
func toProfile(user db.User) protocol.Profile {
	return protocol.Profile{
		ID: user.ID, Name: user.Name, Email: user.Email, Initials: initialsOf(user.Name),
		TimeZone: user.TimeZone, AvatarURL: avatarURL(user), TailnetIdentity: user.TailnetIdentity,
		CreatedAt: store.Timestamp(user.CreatedAt), UpdatedAt: store.Timestamp(user.UpdatedAt),
	}
}

// toUser builds the wire user of the users list from a user row.
func toUser(user db.User) protocol.User {
	return protocol.User{ID: user.ID, Name: user.Name, Initials: initialsOf(user.Name), AvatarURL: avatarURL(user)}
}
