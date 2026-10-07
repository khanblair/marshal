/** Where a person makes the Discord bot. */
export const DISCORD_PORTAL_URL = "https://discord.com/developers/applications";

/*
 * What a person does in Discord, as the connect dialog and the onboarding form both say it. The
 * labels are the ones the developer portal and the app use, so a step can be matched to the screen.
 */

export const DISCORD_BOT_STEPS = [
  "In the Discord developer portal, choose Create App and name it.",
  "Open the Bot page. Under Token, choose Reset Token and copy the new token now: Discord shows it once. Paste it below.",
  "On the same Bot page, under Privileged Gateway Intents, switch on Message Content Intent. Marshal needs it to read replies you type. Without it the buttons still work, and Test connection says so.",
  "Open the Installation page. Under Default Install Settings, add Guild Install with the bot scope, then allow View Channels and Send Messages.",
  "Copy the Install Link, open it, and choose Add to server. For a private channel, also let the bot see and send in it.",
] as const;

export const DISCORD_CHANNEL_STEPS = [
  "In Discord, open User Settings, then Advanced, and switch on Developer Mode.",
  "In the channel list, find the channel notices should go to (for example #general).",
  "Right-click the channel (long-press it on a phone) and choose Copy Channel ID. Not Copy Server ID: that is the server's number, and it will not work.",
  "Paste it below. The channel's link works too.",
] as const;

export const DISCORD_TOKEN_HINT = "Marshal keeps it in the OS keychain and never shows it again.";

export const DISCORD_CHANNEL_HINT =
  "A long number like 1234567890123456789: the channel's id, not the server's.";

/** What a saved token looks like in its field: dots, because the token itself never comes back. */
export const DISCORD_TOKEN_DOTS = "••••••••••••••••••••••••";

export const DISCORD_TOKEN_SAVED_HINT =
  "Saved in the OS keychain. Leave it as it is to keep it, or paste a new one to replace it.";
