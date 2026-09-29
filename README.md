# ariel

An AI agent that works as a Slack bot and a web UI.

Current features:

- Sign in on the web page with a Slack account.
- Mention the bot in Slack; it replies in the thread. A user who has not signed
  in receives a sign-in link instead.
- At sign-in, the user's Slack user token (`search:read`) is stored encrypted
  with Cloud KMS, for features that act on the user's behalf.

See [docs/setup.md](docs/setup.md) for the Slack app, Google Cloud setup, and
configuration. The Slack app manifest is
[docs/slack-app-manifest.yaml](docs/slack-app-manifest.yaml).
