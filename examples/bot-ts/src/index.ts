import { ClickClackClient } from "@clickclack/sdk-ts";

const baseUrl = requiredEnv("CLICKCLACK_URL");
const channelId = requiredEnv("CLICKCLACK_CHANNEL_ID");
const text = process.env.CLICKCLACK_TEXT ?? "clack from bot";

const client = new ClickClackClient({
  baseUrl,
  token: process.env.CLICKCLACK_TOKEN,
  userId: process.env.CLICKCLACK_TOKEN ? undefined : process.env.CLICKCLACK_USER_ID,
});

const message = await client.channels.sendMessage(channelId, { body: text });
console.log(JSON.stringify({ message_id: message.id, channel_seq: message.channel_seq }));

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`missing ${name}`);
  return value;
}
