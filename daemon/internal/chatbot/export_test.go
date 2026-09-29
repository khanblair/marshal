package chatbot

import (
	"context"
	"encoding/json"

	"github.com/bwmarrin/discordgo"
	tgbotmodels "github.com/go-telegram/bot/models"
)

// HandleUpdateForTest feeds one Telegram update, as JSON, to the bot's update handler with a
// receive-loop handler already set, so a test drives the handling without starting the loop (which
// would call Telegram).
func (t *Telegram) HandleUpdateForTest(ctx context.Context, handle Handler, updateJSON string) {
	t.mu.Lock()
	t.handle = handle
	t.mu.Unlock()
	var update tgbotmodels.Update
	if err := json.Unmarshal([]byte(updateJSON), &update); err != nil {
		panic(err)
	}
	t.onUpdate(ctx, nil, &update)
}

// InteractionForTest feeds a pressed button to the bot's interaction handler.
func (d *Discord) InteractionForTest(ctx context.Context, channelID, customID string, handle Handler) {
	d.onInteraction(ctx, &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Type:      discordgo.InteractionMessageComponent,
		ChannelID: channelID,
		Data:      discordgo.MessageComponentInteractionData{CustomID: customID},
	}}, handle)
}
