#!/usr/bin/env node

import { copyFile, readFile, readdir, rename, stat, writeFile } from "node:fs/promises";
import { basename, dirname, join } from "node:path";

const PATCH_MARKER = "clickclack-media-attachments-v1";

function replaceExact(source, before, after, label) {
  const first = source.indexOf(before);
  const last = source.lastIndexOf(before);
  if (first === -1) {
    throw new Error(`Could not find ${label}; the OpenClaw connector may have changed`);
  }
  if (first !== last) {
    throw new Error(`Found more than one ${label}; refusing an ambiguous patch`);
  }
  return `${source.slice(0, first)}${after}${source.slice(first + before.length)}`;
}

function replaceFirst(source, before, after, label) {
  const index = source.indexOf(before);
  if (index === -1) {
    throw new Error(`Could not find ${label}; the OpenClaw connector may have changed`);
  }
  return `${source.slice(0, index)}${after}${source.slice(index + before.length)}`;
}

async function resolveBundle(input) {
  const details = await stat(input);
  if (details.isFile()) return input;

  const packageJSON = JSON.parse(await readFile(join(input, "package.json"), "utf8"));
  const entry = packageJSON.exports?.["."] ?? "./dist/index.js";
  const indexPath = join(input, typeof entry === "string" ? entry : entry.default);
  const indexSource = await readFile(indexPath, "utf8");
  const match = indexSource.match(/from\s+["']\.\/(channel-[^"']+\.js)["']/);
  if (match) return join(dirname(indexPath), match[1]);

  const candidates = (await readdir(dirname(indexPath))).filter(
    (name) =>
      name.startsWith("channel-") && name.endsWith(".js") && name !== "channel-plugin-api.js",
  );
  if (candidates.length !== 1) {
    throw new Error(
      `Expected one ClickClack channel bundle beside ${indexPath}, found ${candidates.length}`,
    );
  }
  return join(dirname(indexPath), candidates[0]);
}

const input = process.argv[2];
if (!input) {
  console.error("Usage: patch-openclaw-clickclack-media.mjs <package-directory|channel-bundle.js>");
  process.exit(2);
}

const bundlePath = await resolveBundle(input);
let source = await readFile(bundlePath, "utf8");

if (source.includes(PATCH_MARKER)) {
  console.log(`Already patched: ${bundlePath}`);
  process.exit(0);
}

source = replaceExact(
  source,
  'import { readProviderJsonResponse, readResponseTextLimited } from "openclaw/plugin-sdk/provider-http";',
  [
    'import { readProviderJsonResponse, readResponseTextLimited } from "openclaw/plugin-sdk/provider-http";',
    'import { loadOutboundMediaFromUrl } from "openclaw/plugin-sdk/outbound-media";',
  ].join("\n"),
  "provider HTTP import",
);

source = replaceFirst(
  source,
  "\t\t\t\t\tbody,\n\t\t\t\t\t...opts?.quotedMessageId ? { quoted_message_id: opts.quotedMessageId } : {},",
  "\t\t\t\t\tbody,\n\t\t\t\t\t...opts?.uploadId ? { upload_id: opts.uploadId } : {},\n\t\t\t\t\t...opts?.quotedMessageId ? { quoted_message_id: opts.quotedMessageId } : {},",
  "channel/direct message upload option",
);

source = replaceExact(
  source,
  "\t\t\t\t\tbody,\n\t\t\t\t\t...opts?.quotedMessageId ? { quoted_message_id: opts.quotedMessageId } : {}",
  "\t\t\t\t\tbody,\n\t\t\t\t\t...opts?.uploadId ? { upload_id: opts.uploadId } : {},\n\t\t\t\t\t...opts?.quotedMessageId ? { quoted_message_id: opts.quotedMessageId } : {}",
  "direct message upload option",
);

source = replaceExact(
  source,
  "\t\t\t\t\tbody,\n\t\t\t\t\t...provenanceFields(opts?.provenance)",
  "\t\t\t\t\tbody,\n\t\t\t\t\t...opts?.uploadId ? { upload_id: opts.uploadId } : {},\n\t\t\t\t\t...provenanceFields(opts?.provenance)",
  "thread reply upload option",
);

source = replaceExact(
  source,
  "\t\tevents: async (workspaceId, afterCursor) => {",
  [
    "\t\tcreateUpload: async (workspaceId, media) => {",
    "\t\t\tconst form = new FormData();",
    '\t\t\tconst contentType = media.contentType || "application/octet-stream";',
    '\t\t\tconst filename = media.fileName || "attachment.bin";',
    '\t\t\tform.set("file", new Blob([media.buffer], { type: contentType }), filename);',
    "\t\t\treturn (await request(`/api/uploads?workspace_id=${encodeURIComponent(workspaceId)}`, {",
    '\t\t\t\tmethod: "POST",',
    "\t\t\t\tbody: form",
    "\t\t\t})).upload;",
    "\t\t},",
    "\t\tattachUpload: async (messageId, uploadId) => {",
    "\t\t\tawait request(`/api/messages/${encodeURIComponent(messageId)}/attachments`, {",
    '\t\t\t\tmethod: "POST",',
    "\t\t\t\tbody: JSON.stringify({ upload_id: uploadId })",
    "\t\t\t});",
    "\t\t},",
    "\t\tevents: async (workspaceId, afterCursor) => {",
  ].join("\n"),
  "HTTP client events method",
);

const mediaHelpers = [
  `// ${PATCH_MARKER}`,
  "function uniqueClickClackMediaUrls(values) {",
  "\tconst seen = new Set();",
  "\tconst urls = [];",
  "\tfor (const value of values) {",
  '\t\tif (typeof value !== "string") continue;',
  "\t\tconst url = value.trim();",
  "\t\tif (!url || seen.has(url)) continue;",
  "\t\tseen.add(url);",
  "\t\turls.push(url);",
  "\t}",
  "\treturn urls;",
  "}",
  "async function sendClickClackMedia(params) {",
  "\tconst mediaUrls = uniqueClickClackMediaUrls(params.mediaUrls ?? []);",
  "\tif (mediaUrls.length === 0) return await sendClickClackText(params);",
  "\tconst account = resolveClickClackAccount({ cfg: params.cfg, accountId: params.accountId });",
  "\tconst client = createClickClackClient({ baseUrl: account.baseUrl, token: account.token });",
  "\tconst workspaceId = await resolveWorkspaceId(client, account.workspace);",
  "\tconst uploads = [];",
  "\tfor (const mediaUrl of mediaUrls) {",
  "\t\tconst media = await loadOutboundMediaFromUrl(mediaUrl, {",
  "\t\t\t...(params.mediaLocalRoots ? { localRoots: params.mediaLocalRoots } : {}),",
  "\t\t\t...(params.mediaReadFile ? { readFile: params.mediaReadFile, sandboxValidated: true } : {})",
  "\t\t});",
  "\t\tuploads.push(await client.createUpload(workspaceId, media));",
  "\t}",
  "\tconst parsed = parseClickClackTarget(params.to);",
  '\tconst explicitThreadId = params.threadId == null ? "" : String(params.threadId);',
  '\tconst replyToId = params.replyToId == null ? "" : String(params.replyToId);',
  "\tconst options = { uploadId: uploads[0].id, provenance: params.provenance };",
  "\tlet message;",
  '\tif (explicitThreadId || parsed.kind === "thread") {',
  '\t\tmessage = await client.createThreadReply(explicitThreadId || parsed.id, params.text ?? "", options);',
  '\t} else if (parsed.kind === "dm") {',
  "\t\tconst dm = await client.createDirectConversation(workspaceId, [parsed.id]);",
  '\t\tmessage = await client.createDirectMessage(dm.id, params.text ?? "", { ...options, quotedMessageId: replyToId || void 0 });',
  "\t} else {",
  "\t\tconst channelId = await resolveChannelId(client, workspaceId, parsed.id);",
  '\t\tmessage = await client.createChannelMessage(channelId, params.text ?? "", { ...options, quotedMessageId: replyToId || void 0 });',
  "\t}",
  "\tfor (const upload of uploads.slice(1)) await client.attachUpload(message.id, upload.id);",
  "\treturn { to: params.to, messageId: message.id };",
  "}",
].join("\n");

source = replaceExact(
  source,
  "//#endregion\n//#region extensions/clickclack/src/inbound.ts",
  `//#endregion\n${mediaHelpers}\n//#region extensions/clickclack/src/inbound.ts`,
  "outbound/inbound section boundary",
);

source = replaceExact(
  source,
  [
    "\t\t\tdeliver: async (payload) => {",
    '\t\t\t\tconst text = payload && typeof payload === "object" && "text" in payload ? payload.text ?? "" : "";',
    "\t\t\t\tif (!text.trim()) return;",
    "\t\t\t\tawait sendClickClackText({",
    "\t\t\t\t\tcfg: params.config,",
    "\t\t\t\t\taccountId: params.account.accountId,",
    "\t\t\t\t\tto: target,",
    "\t\t\t\t\ttext,",
    "\t\t\t\t\tthreadId: message.parent_message_id ? message.thread_root_id : void 0,",
    "\t\t\t\t\treplyToId: message.id,",
    "\t\t\t\t\tprovenance: turnProvenance",
    "\t\t\t\t});",
    "\t\t\t},",
  ].join("\n"),
  [
    "\t\t\tdeliver: async (payload) => {",
    '\t\t\t\tconst text = payload && typeof payload === "object" && "text" in payload ? payload.text ?? "" : "";',
    '\t\t\t\tconst mediaUrls = payload && typeof payload === "object" ? uniqueClickClackMediaUrls([payload.mediaUrl, ...(Array.isArray(payload.mediaUrls) ? payload.mediaUrls : [])]) : [];',
    "\t\t\t\tif (!text.trim() && mediaUrls.length === 0) return;",
    "\t\t\t\tawait (mediaUrls.length > 0 ? sendClickClackMedia : sendClickClackText)({",
    "\t\t\t\t\tcfg: params.config,",
    "\t\t\t\t\taccountId: params.account.accountId,",
    "\t\t\t\t\tto: target,",
    "\t\t\t\t\ttext,",
    "\t\t\t\t\tmediaUrls,",
    "\t\t\t\t\tthreadId: message.parent_message_id ? message.thread_root_id : void 0,",
    "\t\t\t\t\treplyToId: message.id,",
    "\t\t\t\t\tprovenance: turnProvenance",
    "\t\t\t\t});",
    "\t\t\t},",
  ].join("\n"),
  "inbound delivery handler",
);

source = replaceExact(
  source,
  "\t\t\tsendText: async ({ cfg, to, text, accountId, threadId, replyToId }) => await sendClickClackText({",
  [
    "\t\t\tsendMedia: async ({ cfg, to, text, mediaUrl, mediaLocalRoots, mediaReadFile, accountId, threadId, replyToId }) => await sendClickClackMedia({",
    "\t\t\t\tcfg,",
    "\t\t\t\taccountId,",
    "\t\t\t\tto,",
    "\t\t\t\ttext,",
    "\t\t\t\tmediaUrls: [mediaUrl],",
    "\t\t\t\tmediaLocalRoots,",
    "\t\t\t\tmediaReadFile,",
    "\t\t\t\tthreadId,",
    "\t\t\t\treplyToId",
    "\t\t\t}),",
    "\t\t\tsendText: async ({ cfg, to, text, accountId, threadId, replyToId }) => await sendClickClackText({",
  ].join("\n"),
  "attached outbound text sender",
);

const mode = (await stat(bundlePath)).mode;
const backupPath = `${bundlePath}.pre-${PATCH_MARKER}`;
const temporaryPath = join(dirname(bundlePath), `.${basename(bundlePath)}.${process.pid}.tmp`);
await copyFile(bundlePath, backupPath);
await writeFile(temporaryPath, source, { mode });
await rename(temporaryPath, bundlePath);

console.log(`Patched: ${bundlePath}`);
console.log(`Backup: ${backupPath}`);
