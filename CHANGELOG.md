# Changelog

## Unreleased

- Added a public product website at the web root while keeping the chat app at
  `/app` locally and on `app.clickclack.chat` when served from that host.
- Added an agent-friendly ClickClack client mode to the Go binary with
  `login`, `logout`, `whoami`, `status`, workspace/channel listing, message
  send/list, and thread open/reply commands.
- Scoped stored CLI credentials and workspace/channel defaults to the saved
  server URL, with `--user` / `CLICKCLACK_USER_ID` taking precedence over
  stored bearer tokens unless `--token` is explicitly supplied.
- Documented the `clickclack.chat` product domain, `app.clickclack.chat` app
  domain, `docs.clickclack.chat` docs domain, and recommended bearer-token auth
  flow for hosted agents.
