// Package googleclient is the Google OAuth client that belongs to Marshal itself, so a person
// connects Google Calendar with one click and never pastes a key. Marshal's own client is made once,
// in Marshal's own Google Cloud project, as a Desktop app, and is put in the build; a build without
// one still works, and the person makes their own client in Settings instead.
//
// The client is read from files/client.json, which is not in git. A release build writes it before
// compiling (docs/development.md section 3.7a); a developer can instead set
// MARSHAL_GOOGLE_CLIENT_ID and MARSHAL_GOOGLE_CLIENT_SECRET. Google does not treat a Desktop app's
// secret as confidential, which is why shipping it in the app is acceptable.
package googleclient

import (
	"embed"
	"encoding/json"
	"strings"
)

//go:embed files
var files embed.FS

// Client is a Google OAuth client.
type Client struct {
	ID     string `json:"clientId"`
	Secret string `json:"clientSecret"`
}

// Valid says the client has both parts.
func (c Client) Valid() bool {
	return strings.TrimSpace(c.ID) != "" && strings.TrimSpace(c.Secret) != ""
}

// Bundled is the client built into this binary, or the zero Client when the build has none or its
// file cannot be read.
func Bundled() Client {
	raw, err := files.ReadFile("files/client.json")
	if err != nil {
		return Client{}
	}
	var client Client
	if json.Unmarshal(raw, &client) != nil || !client.Valid() {
		return Client{}
	}
	return client
}
