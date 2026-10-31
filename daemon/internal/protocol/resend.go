package protocol

// SaveResendRequest is the body that saves the Resend connection. The API key is written to the
// keychain and never comes back from any route. The sender address is written to the connection's
// own row, because a screen shows it so a person can check which address mail comes from.
type SaveResendRequest struct {
	// Key is the Resend API key. Empty keeps the key already saved, so the sender can change
	// without pasting the key again.
	Key string `json:"key"`
	// From is the sender address, such as "Marshal <marshal@example.com>". It must be on a domain
	// Resend has verified, or Resend's own test sender.
	From string `json:"from"`
}
