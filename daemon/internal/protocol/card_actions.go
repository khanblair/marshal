package protocol

// MaxMessageChars is the most characters one message to an agent may have. The daemon refuses a
// longer one with a plain sentence that names this limit, so the composer can show the same number.
const MaxMessageChars = 100000

// SendMessageRequest is the body of POST /v1/cards/{id}/messages: one message from the person to
// the agent that works on the card. The answer arrives on the event stream, not in the reply.
type SendMessageRequest struct {
	// Text is the message. It cannot be empty or only spaces, and it can have at most
	// MaxMessageChars characters. Its spaces and line breaks are sent as they are.
	Text string `json:"text"`
}
