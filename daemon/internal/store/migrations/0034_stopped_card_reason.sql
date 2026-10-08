-- A card whose agent stopped unexpectedly was moved to Needs you with no reason, so the screen had
-- nothing to say and no way forward. It now gets a reason when it happens (internal/session/pump.go);
-- this gives the same reason to the cards already in that state. Forward only, like every migration.
--
-- Only a card in Needs you with no reason and a stopped session is touched. A card a person put in
-- Needs you themselves has no stopped session of its own to explain, and one with a reason keeps it.
-- +goose Up

UPDATE cards
SET needs_reason_kind = 'stuck',
    needs_reason_text = 'The agent stopped unexpectedly. Resume it to continue.'
WHERE state = 'needs'
  AND needs_reason_kind = ''
  AND EXISTS (SELECT 1 FROM sessions WHERE sessions.card_id = cards.id AND sessions.state = 'stopped');
