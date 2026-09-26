package search

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A query is words. A thing matches when every word matches one of its fields, in any order, and
// its score is the sum of the best match each word found. A word matches a field when it is
// inside the field's text, ignoring case; how it sits there is its tier, and the field it sits in
// is its weight. Weight counts before tier, so a match in a title always ranks above a match in a
// description, whatever the tier of either.

// tier says how a word sits in a piece of text, worst first.
type tier int

const (
	tierNone      tier = iota // the word is not in the text
	tierWithin                // the word is somewhere inside the text
	tierWordStart             // the word starts a word of the text
	tierStart                 // the text starts with the word
	tierWhole                 // the text is the word
)

// The weight of a field. A field's score is its weight times ten plus its tier, so the ten a
// tier can add never lifts a match past the next weight up.
const (
	weightBody  = 1 // a card's description, and a project's folder
	weightKey   = 2 // the project part of a card's key, for a word that is not a number
	weightID    = 3 // a project's short id
	weightTitle = 4 // a title, or a project's name
)

// Scores for a word that names a card by its number. Most sit above every field score, because
// someone who typed "#41" or "api#41" wants that card first. Digits alone are a weaker sign, since
// a title can hold a number too, so the bare digits of a card's number sit above the titles that
// match and the digits that only start its number sit below them.
const (
	scoreKeyWhole    = 100 // the word is the card's whole key, or "#" and its number
	scoreNumberBare  = 90  // the word is only digits and is the card's number
	scoreKeyStart    = 90  // the card's key starts with the word, such as "api#4"
	scoreNumberStart = 80  // "#4" for card 41: the number starts with the digits
	scoreNumberOnly  = 30  // digits alone that start the card's number, such as "4" for card 41
	scoreKeyWithin   = 60  // the word has a "#" and is somewhere inside the key
)

// fieldScore is the score of a match of tier t in a field of weight w, or zero for no match.
func fieldScore(weight int, t tier) int {
	if t == tierNone {
		return 0
	}
	return weight*10 + int(t)
}

// matchTier says how word sits in text. Both are already lower case.
func matchTier(text, word string) tier {
	switch {
	case text == word:
		return tierWhole
	case strings.HasPrefix(text, word):
		return tierStart
	}
	at := strings.Index(text, word)
	if at < 0 {
		return tierNone
	}
	for offset := at; ; {
		if startsWord(text, offset) {
			return tierWordStart
		}
		_, width := utf8.DecodeRuneInString(text[offset:])
		next := strings.Index(text[offset+width:], word)
		if next < 0 {
			return tierWithin
		}
		offset += width + next
	}
}

// startsWord reports whether a word of text begins at byte offset: the character before it is not
// a letter or a digit.
func startsWord(text string, offset int) bool {
	before, _ := utf8.DecodeLastRuneInString(text[:offset])
	return !unicode.IsLetter(before) && !unicode.IsDigit(before)
}

// bestOf scores each word by the best match of any of its fields and adds the scores up. One word
// with no match at all means the thing does not match the query, and the score is zero.
func bestOf(words []string, score func(word string) int) int {
	total := 0
	for _, word := range words {
		best := score(word)
		if best == 0 {
			return 0
		}
		total += best
	}
	return total
}

// projectScore scores a project by its name, its id, and its folder.
func projectScore(project protocol.Project, words []string) int {
	name, id, path := strings.ToLower(project.Name), strings.ToLower(project.ID), strings.ToLower(project.Path)
	return bestOf(words, func(word string) int {
		return max(
			fieldScore(weightTitle, matchTier(name, word)),
			fieldScore(weightID, matchTier(id, word)),
			fieldScore(weightBody, matchTier(path, word)),
		)
	})
}

// cardScore scores a card by its number, its title, and its description. A word that reads as a
// card number (see numberScore) is checked against the card's key first.
func cardScore(card protocol.Card, words []string) int {
	key, title, body := strings.ToLower(card.Key), strings.ToLower(card.Title), strings.ToLower(card.Body)
	return bestOf(words, func(word string) int {
		return max(
			numberScore(card.Number, key, word),
			fieldScore(weightTitle, matchTier(title, word)),
			fieldScore(weightKey, matchTier(key, word)),
			fieldScore(weightBody, matchTier(body, word)),
		)
	})
}

// chatScore scores a chat by its title, the only thing about a chat that is searched.
func chatScore(chat protocol.Chat, words []string) int {
	title := strings.ToLower(chat.Title)
	return bestOf(words, func(word string) int {
		return fieldScore(weightTitle, matchTier(title, word))
	})
}

// numberScore scores a word that names a card by its number: "#41", the digits "41", or a key or
// the start of one such as "api#41" or "api#4". The key is lower case, like the word. Any other
// word scores zero here and is matched as text.
func numberScore(number int, key, word string) int {
	digits := strings.TrimPrefix(word, "#")
	switch {
	case word == key:
		return scoreKeyWhole
	case strings.HasPrefix(word, "#") && isDigits(digits):
		return numberMatch(number, digits, scoreKeyWhole, scoreNumberStart)
	case isDigits(word):
		return numberMatch(number, word, scoreNumberBare, scoreNumberOnly)
	case strings.Contains(word, "#") && strings.HasPrefix(key, word):
		return scoreKeyStart
	case strings.Contains(word, "#") && strings.Contains(key, word):
		return scoreKeyWithin
	}
	return 0
}

// numberMatch scores digits against a card's number: whole when they are the number, and started
// when the number starts with them, so "#4" finds card 4, card 40, and card 41 while it is typed.
func numberMatch(number int, digits string, whole, started int) int {
	text := strconv.Itoa(number)
	switch {
	case digits == text:
		return whole
	case strings.HasPrefix(text, digits):
		return started
	}
	return 0
}

// isDigits reports whether text is one or more ASCII digits.
func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}
