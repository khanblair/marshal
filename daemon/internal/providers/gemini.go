package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"google.golang.org/genai"
)

// geminiClient is one Google Gemini API client, behind the Client interface. It is the native
// adapter: Gemini's own GenerateContent API, not the OpenAI-compatible door into it.
type geminiClient struct {
	sdk *genai.Client
}

// NewGemini returns a Client for Google's Gemini API. Unlike the other two adapters it can fail
// here rather than on the first call, because the SDK builds its client eagerly and refuses a
// missing key - so a provider row with no key is reported when it is set up, not later.
func NewGemini(ctx context.Context, cfg Config) (Client, error) {
	opts := genai.HTTPOptions{BaseURL: cfg.BaseURL}
	// This SDK does not retry unless it is asked to, which is already what a nil Retries means.
	if cfg.Retries != nil {
		attempts := int32(1)
		if *cfg.Retries > 0 {
			attempts = int32(*cfg.Retries) + 1
		}
		opts.RetryOptions = &genai.HTTPRetryOptions{Attempts: genai.Ptr(attempts)}
	}
	sdk, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
		// Said outright rather than left to the environment, so a stray variable cannot send a
		// provider key to Google Cloud instead of the Gemini API.
		Backend:     genai.BackendGeminiAPI,
		HTTPClient:  cfg.HTTPClient,
		HTTPOptions: opts,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuth, err)
	}
	return &geminiClient{sdk: sdk}, nil
}

// ID is Gemini's own id.
func (c *geminiClient) ID() string { return GeminiID }

// Complete makes one non-streamed call and returns the whole answer.
func (c *geminiClient) Complete(ctx context.Context, req Request) (Reply, error) {
	config, contents, err := geminiParams(req)
	if err != nil {
		return Reply{}, err
	}
	resp, err := c.sdk.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return Reply{}, mapGeminiError(err)
	}
	return geminiReply(resp)
}

// Stream makes one call and returns its events as they arrive. The SDK hands its stream back as a
// range-over function rather than something that can be read one item at a time, so one goroutine
// runs it and passes its items on, and Close stops it by cancelling the request.
func (c *geminiClient) Stream(ctx context.Context, req Request) (Stream, error) {
	config, contents, err := geminiParams(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan geminiChunk, 8)
	go func() {
		defer close(out)
		for resp, err := range c.sdk.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			chunk := geminiChunk{resp: resp, err: err}
			// Giving up on a send when the request is cancelled is what keeps this goroutine
			// from being left behind when a caller stops reading.
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return &geminiStream{ctx: ctx, cancel: cancel, out: out}, nil
}

// geminiChunk is one item the reading goroutine passed on: an answer, or the error that ended the
// stream.
type geminiChunk struct {
	resp *genai.GenerateContentResponse
	err  error
}

// geminiStream is one streamed answer, read from the channel the SDK's range is driven into.
type geminiStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	out    chan geminiChunk
	queue  []Event

	err        error
	stopReason string
	usage      Usage
	// sawToolUse remembers that a call was announced, because Gemini ends a turn that asked for a
	// tool with its ordinary stop reason and the events have already been handed to the caller by
	// the time the turn ends.
	sawToolUse bool
	done       bool
}

// Recv returns the next event, or io.EOF once the answer is over.
func (s *geminiStream) Recv() (Event, error) {
	for {
		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}
		// A stream the caller gave up on - or one whose turn ran out of time - reports that rather
		// than the answer it happened to have buffered.
		if err := s.ctx.Err(); err != nil {
			return Event{}, err
		}
		if s.err != nil {
			return Event{}, s.err
		}
		if s.done {
			return Event{}, io.EOF
		}
		select {
		case chunk, ok := <-s.out:
			if !ok {
				s.done = true
				s.finish()
				continue
			}
			if chunk.err != nil {
				s.err = mapGeminiError(chunk.err)
				continue
			}
			s.handle(chunk.resp)
		case <-s.ctx.Done():
			s.err = s.ctx.Err()
		}
	}
}

// Close abandons the stream. It is safe to call more than once.
func (s *geminiStream) Close() error {
	s.cancel()
	return nil
}

// handle turns one streamed answer into zero or more Marshal events. Gemini sends a function call
// whole, so it is announced as soon as it arrives; text and reasoning arrive in pieces.
func (s *geminiStream) handle(resp *genai.GenerateContentResponse) {
	if resp == nil {
		return
	}
	s.usage = geminiUsage(resp, s.usage)
	for _, candidate := range resp.Candidates {
		if candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			switch {
			case part.FunctionCall != nil:
				s.sawToolUse = true
				s.queue = append(s.queue, Event{
					Kind:      EventToolUse,
					ToolUseID: part.FunctionCall.ID,
					ToolName:  part.FunctionCall.Name,
					ToolInput: geminiToolInput(part.FunctionCall.Args),
				})
			case part.Thought:
				if part.Text != "" {
					s.queue = append(s.queue, Event{Kind: EventThinking, Text: part.Text, Signature: encodeSignature(part.ThoughtSignature)})
				}
			case part.Text != "":
				s.queue = append(s.queue, Event{Kind: EventText, Text: part.Text})
			}
		}
		if candidate.FinishReason != "" {
			s.stopReason = geminiStopReason(string(candidate.FinishReason))
		}
	}
}

// finish sends the last event.
func (s *geminiStream) finish() {
	if s.stopReason == "" {
		s.stopReason = StopEndTurn
	}
	if s.stopReason == StopEndTurn && s.sawToolUse {
		s.stopReason = StopToolUse
	}
	s.queue = append(s.queue, Event{Kind: EventDone, StopReason: s.stopReason, Usage: s.usage})
}

// geminiParams turns a Request into the SDK's generate-content parameters. It fails when something
// the caller passed cannot be expressed, such as a tool whose schema or a tool call whose arguments
// are not JSON.
func geminiParams(req Request) (*genai.GenerateContentConfig, []*genai.Content, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	if maxTokens > math.MaxInt32 {
		maxTokens = math.MaxInt32
	}
	contents, err := geminiContents(req.Messages)
	if err != nil {
		return nil, nil, err
	}
	config := &genai.GenerateContentConfig{MaxOutputTokens: int32(maxTokens)}
	if req.System != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: req.System}}}
	}
	tools, err := geminiTools(req.Tools)
	if err != nil {
		return nil, nil, err
	}
	config.Tools = tools
	// Thinking is asked for by level, and the thoughts are asked to come back so the app can show
	// them. A mode the model has no support for is left to Gemini to answer for.
	if level := geminiLevel(req.Thinking); level != "" {
		config.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingLevel:   genai.ThinkingLevel(level),
			IncludeThoughts: true,
		}
	}
	return config, contents, nil
}

// geminiContents turns the conversation into the SDK's content list. One Marshal message is one
// Content here, holding all of that turn's parts, which is how this API spells a turn.
func geminiContents(in []Message) ([]*genai.Content, error) {
	names := toolNames(in)
	out := make([]*genai.Content, 0, len(in))
	for _, m := range in {
		parts := make([]*genai.Part, 0, len(m.Parts))
		for _, p := range m.Parts {
			switch p.Kind {
			case PartText:
				parts = append(parts, &genai.Part{Text: p.Text, ThoughtSignature: decodeSignature(p.Signature)})
			case PartThinking:
				parts = append(parts, &genai.Part{Text: p.Text, Thought: true, ThoughtSignature: decodeSignature(p.Signature)})
			case PartToolUse:
				args, err := geminiArguments(p)
				if err != nil {
					return nil, err
				}
				parts = append(parts, &genai.Part{
					FunctionCall:     &genai.FunctionCall{Name: p.ToolName, ID: p.ToolUseID, Args: args},
					ThoughtSignature: decodeSignature(p.Signature),
				})
			case PartToolResult:
				parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
					Name:     toolResultName(p, names),
					Response: toolResultResponse(p),
				}})
			default:
				return nil, fmt.Errorf("a message part of kind %q cannot be sent to Gemini", p.Kind)
			}
		}
		out = append(out, &genai.Content{Role: geminiRole(m.Role), Parts: parts})
	}
	return out, nil
}

// geminiRole is the role this API names a turn with.
func geminiRole(role string) string {
	if role == RoleAssistant {
		return genai.RoleModel
	}
	return genai.RoleUser
}

// toolNames maps each tool call's id to the tool it named, so a tool result that does not name its
// own tool can still be answered. Gemini's function responses carry the name, not the id.
func toolNames(in []Message) map[string]string {
	names := map[string]string{}
	for _, m := range in {
		for _, p := range m.Parts {
			if p.Kind == PartToolUse && p.ToolUseID != "" {
				names[p.ToolUseID] = p.ToolName
			}
		}
	}
	return names
}

// toolResultName is the tool a result answers: the one it names, or failing that the one the call
// it answers named, or failing that the call's own id, which at least says what is missing.
func toolResultName(p Part, names map[string]string) string {
	if p.ToolName != "" {
		return p.ToolName
	}
	if name := names[p.ToolUseID]; name != "" {
		return name
	}
	return p.ToolUseID
}

// toolResultResponse is what a tool printed, in the shape this API expects: an output, or the
// failure it was.
func toolResultResponse(p Part) map[string]any {
	text := p.Text
	if text == "" {
		text = "(the tool printed nothing)"
	}
	if p.IsError {
		return map[string]any{"error": text}
	}
	return map[string]any{"output": text}
}

// geminiArguments renders a tool call's arguments as the map this API carries them in. A call the
// model made with no arguments is sent as an empty object.
func geminiArguments(p Part) (map[string]any, error) {
	if len(p.ToolInput) == 0 {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal(p.ToolInput, &args); err != nil {
		return nil, fmt.Errorf("the arguments of the call to tool %q are not a JSON object: %w", p.ToolName, err)
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// geminiToolInput is the other direction: a call the model made, as the JSON Marshal carries it.
func geminiToolInput(args map[string]any) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage("{}")
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

// geminiTools turns Marshal's tool list into the SDK's. A tool with no schema is sent with an empty
// object schema, which is how this API spells a tool that takes no arguments.
func geminiTools(in []Tool) ([]*genai.Tool, error) {
	if len(in) == 0 {
		return nil, nil
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(in))
	for _, t := range in {
		schema := any(map[string]any{"type": "object", "properties": map[string]any{}})
		if len(t.InputSchema) > 0 {
			var decoded any
			if err := json.Unmarshal(t.InputSchema, &decoded); err != nil {
				return nil, fmt.Errorf("the schema of tool %q is not JSON: %w", t.Name, err)
			}
			schema = decoded
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}, nil
}

// geminiReply turns a whole answer into a Reply. An answer with no candidates at all is the
// provider declining to answer, not a failure of Marshal's: it is reported as a refusal so the
// caller can show it rather than retry it.
func geminiReply(resp *genai.GenerateContentResponse) (Reply, error) {
	parts := make([]Part, 0)
	stopReason := StopEndTurn
	for _, candidate := range resp.Candidates {
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				parts = append(parts, geminiPart(part)...)
			}
		}
		if candidate.FinishReason != "" {
			stopReason = geminiStopReason(string(candidate.FinishReason))
		}
	}
	if len(resp.Candidates) == 0 {
		return Reply{StopReason: StopRefusal, Usage: geminiUsage(resp, Usage{})}, nil
	}
	if stopReason == StopEndTurn && hasToolUsePart(parts) {
		stopReason = StopToolUse
	}
	return Reply{Parts: parts, StopReason: stopReason, Usage: geminiUsage(resp, Usage{})}, nil
}

// geminiPart turns one of the SDK's parts into the Marshal parts it stands for. A part that holds
// nothing Marshal uses stands for none.
func geminiPart(part *genai.Part) []Part {
	switch {
	case part.FunctionCall != nil:
		return []Part{{
			Kind:      PartToolUse,
			ToolUseID: part.FunctionCall.ID,
			ToolName:  part.FunctionCall.Name,
			ToolInput: geminiToolInput(part.FunctionCall.Args),
			Signature: encodeSignature(part.ThoughtSignature),
		}}
	case part.Thought && part.Text != "":
		return []Part{{Kind: PartThinking, Text: part.Text, Signature: encodeSignature(part.ThoughtSignature)}}
	case part.Text != "":
		return []Part{{Kind: PartText, Text: part.Text, Signature: encodeSignature(part.ThoughtSignature)}}
	default:
		return nil
	}
}

// hasToolUsePart reports whether an answer named a tool.
func hasToolUsePart(parts []Part) bool {
	for _, p := range parts {
		if p.Kind == PartToolUse {
			return true
		}
	}
	return false
}

// geminiUsage reads what a call cost. Thinking tokens are part of what the owner pays for, so they
// are counted as output.
func geminiUsage(resp *genai.GenerateContentResponse, fallback Usage) Usage {
	if resp.UsageMetadata == nil {
		return fallback
	}
	return Usage{
		InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
		OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount + resp.UsageMetadata.ThoughtsTokenCount),
	}
}

// geminiStopReason turns this API's finish reason into Marshal's. Anything Marshal does not know
// reads as the end of a turn, which is what it means: the answer is complete.
func geminiStopReason(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return StopMaxTokens
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "LANGUAGE":
		return StopRefusal
	default:
		return StopEndTurn
	}
}

// mapGeminiError turns an SDK error into the provider sentinels callers act on. An error that is
// not an API answer at all (a broken connection, for example) is unavailable.
func mapGeminiError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return errorForStatus(apiErr.Code, apiErr.Message)
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

// encodeSignature keeps the provider's proof that a part is genuine as a string, which is the one
// field Marshal has for it. Gemini's signature is opaque bytes; base64 is how they travel in text.
func encodeSignature(signature []byte) string {
	if len(signature) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(signature)
}

// decodeSignature is the other direction. A signature that is not base64 is dropped rather than
// guessed at: it belongs to another provider, and sending it back as bytes would be nonsense.
func decodeSignature(signature string) []byte {
	if signature == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return nil
	}
	return decoded
}
