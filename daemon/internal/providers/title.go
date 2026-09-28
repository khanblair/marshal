package providers

// This file is the small call that names a project chat after its first message (docs/architecture.md
// 16.2, inventory B4.7): "New chats get a short title from their first message, written by a cheap
// model." It is the only provider call Marshal makes on its own account rather than for a session,
// which is why it lives beside the resolver: it picks a model the way Resolve does, and it is the one
// place that answers "which model is cheapest" (Cheapest).
//
// The caller is chats.Service, which hands this in as its Titler and falls back to naming a chat from
// the message's own first words when this answers an error (chats/service.go's noteMessage). Nothing
// here needs to know that: a title it cannot write is an error, and the fallback is the caller's.

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// titleMaxTokens is how much of an answer a title asks for. A chat title is a few words, so
	// asking for more would only invite the model to explain itself.
	titleMaxTokens = 24
	// titleMaxWords is how many words of a written title Marshal keeps. A model that answers with a
	// sentence rather than a name is trimmed to its first words rather than refused.
	titleMaxWords = 8
	// titleMaxChars is the longest title Marshal will take from a model. It is deliberately shorter
	// than the longest a chat may be called (chats.maxTitleChars, 100): a written title is a name,
	// and anything longer is the model ignoring what it was asked for.
	titleMaxChars = 60
)

// titleSystem is what the model is asked to do, in the words of the design. It is short so that it
// costs almost nothing, and it says the two things a person would: be brief, and answer with the name
// alone - Marshal takes the first line and throws away anything around it.
const titleSystem = "You name project chats. Answer with a title of at most six words. " +
	"Use no quotes, no punctuation at the end, and nothing else."

// Cheapest returns the model of the cheapest provider that is set up, with the client that calls it.
// It is Marshal's own choice, made once, for the work that must cost as little as it can - a chat
// title today. A model is only a candidate when its provider is set up and Marshal can name one of
// its models by price, which leaves out the providers that serve whatever a person has loaded.
//
// When no provider is set up at all the answer wraps ErrNoProvider, which is what a caller falls back
// on: Marshal says so rather than failing something the person asked for (chats names the chat from
// the message instead).
func (s *Service) Cheapest() (Resolved, error) {
	info, model, ok, err := s.cheapest()
	if err != nil {
		return Resolved{}, err
	}
	if !ok {
		return Resolved{}, fmt.Errorf(
			"%w: save a key in Settings, under Provider keys", ErrNoProvider)
	}
	secret, err := s.secret(info.ID)
	if err != nil {
		return Resolved{}, err
	}
	p, err := s.client(info, secret)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{ProviderID: info.ID, Model: model.ID, Client: p}, nil
}

// cheapest picks the provider and model with the lowest price among the providers that are set up. It
// reads no client, so the scan costs nothing but a keychain read per provider; the winner's client is
// built once by Cheapest.
//
// A model Marshal pays nothing for - a local one, which is priced at zero - is only chosen when
// nothing else is set up. That is not snobbery about free models: Marshal's own price table is the
// only thing it can compare models by, and every free model ties with every other at zero, so a tie
// gives it nothing to choose with. Preferring a price it can read is the honest reading of "cheapest"
// when the alternative is a coin toss between a small free model and a large slow one.
func (s *Service) cheapest() (Info, protocol.AgentModel, bool, error) {
	var best Info
	var bestModel protocol.AgentModel
	var bestCost int64
	paid := false
	var free Info
	var freeModel protocol.AgentModel
	haveFree := false
	for _, info := range known() {
		model, cost, priced := info.cheapestPriced()
		if !priced {
			// A provider whose models Marshal cannot price cannot be judged the cheapest, and a
			// provider with no models at all has nothing to offer; both are skipped rather than
			// guessed at.
			continue
		}
		setUp, err := s.setUp(info.ID)
		if err != nil {
			return Info{}, protocol.AgentModel{}, false, err
		}
		if !setUp {
			continue
		}
		if cost == 0 {
			if !haveFree {
				free, freeModel, haveFree = info, model, true
			}
			continue
		}
		if !paid || cost < bestCost {
			best, bestModel, bestCost, paid = info, model, cost, true
		}
	}
	if paid {
		return best, bestModel, true, nil
	}
	return free, freeModel, haveFree, nil
}

// cheapestPriced is in catalog.go, next to the table it reads.

// Title writes a short name for a chat from its first message, using the cheapest model that is set
// up. It answers chats.Service's Titler: an error means Marshal could not write one, and the caller
// names the chat from the message's own words instead, so nothing a person typed is ever lost to a
// failure here.
//
// What comes back is trimmed to something a chat can be called: the first line only, no surrounding
// quotes, one run of spaces between words, at most titleMaxChars characters and titleMaxWords words,
// and no trailing full stop. A message the model answers with nothing leaves an error rather than an
// empty name.
func (s *Service) Title(ctx context.Context, text string) (string, error) {
	resolved, err := s.Cheapest()
	if err != nil {
		return "", err
	}
	reply, err := resolved.Client.Complete(ctx, Request{
		Model:     resolved.Model,
		System:    titleSystem,
		Messages:  []Message{{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: text}}}},
		MaxTokens: titleMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("write a chat title with %s: %w", resolved.Model, err)
	}
	title := cleanTitle(textOf(reply))
	if title == "" {
		return "", fmt.Errorf("write a chat title with %s: %w", resolved.Model, ErrUnavailable)
	}
	return title, nil
}

// textOf is everything the model said, in order. A thinking part is not part of the answer.
func textOf(reply Reply) string {
	var b strings.Builder
	for _, part := range reply.Parts {
		if part.Kind == PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// cleanTitle turns whatever a model answered into something a chat can be called. It is deliberately
// forgiving: a model that wraps its answer in quotes, adds a full stop, or explains itself on a second
// line still gives a usable name, because a title is a nicety and refusing it would cost the person
// the name of their chat.
func cleanTitle(answered string) string {
	// The first line that has anything on it, so a model that answers with a blank line before its
	// title still gives one, and a model that answers with a title and then explains itself gives
	// the title.
	line := ""
	for _, candidate := range strings.Split(answered, "\n") {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			line = trimmed
			break
		}
	}
	line = strings.Trim(line, `"'`+"`")
	line = strings.TrimRight(line, ".!?:;, ")
	words := strings.Fields(line)
	if len(words) > titleMaxWords {
		words = words[:titleMaxWords]
	}
	title := strings.Join(words, " ")
	runes := []rune(title)
	if len(runes) > titleMaxChars {
		runes = runes[:titleMaxChars]
	}
	title = strings.TrimSpace(string(runes))
	if title == "" {
		return ""
	}
	// The first letter is a capital, as the app's own chat did, so a model that answers in lower
	// case still gives a name that reads like one.
	runes = []rune(title)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
