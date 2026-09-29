# Setup

This document describes how to run Ariel: the Slack app, Google Cloud (Cloud KMS
and Firestore), the server configuration, and local development.

## How Ariel uses Slack

- **Bot token** (`xoxb-`): installed once by a workspace administrator. Ariel
  uses it to receive mentions, reply in threads, send login prompts, and read
  the display name of a user who signs in.
- **User tokens** (`xoxp-`): obtained from each user when they sign in on the
  web page. The sign-in is a Slack OAuth v2 authorization that requests the user
  scope `search:read`, so one authorization both identifies the user and
  returns the user's token. Ariel encrypts the token with Cloud KMS and stores
  only the ciphertext in Firestore. A token is used only for requests made by
  the user who owns it.
- A user who mentions the bot without having signed in receives a message that
  only they can see, with a link to the sign-in page. Signing out on the web
  ends only that browser's session; the stored user token stays, so the bot
  keeps answering that user's mentions.

## 1. Create the Slack app

1. Copy `docs/slack-app-manifest.yaml` and replace `ariel.example.com` with the
   public URL of your server (the value of `ARIEL_BASE_URL`). Both the redirect
   URL (`/api/auth/callback`) and the event request URL (`/hooks/slack/event`)
   must use that host.
2. Open https://api.slack.com/apps, choose **Create New App** → **From an app
   manifest**, select your workspace, and paste the manifest.
3. On **Install App**, install the app to the workspace. Copy the **Bot User
   OAuth Token** (`xoxb-...`) → `ARIEL_SLACK_BOT_TOKEN`.
4. On **Basic Information** → **App Credentials**, copy:
   - **Client ID** → `ARIEL_SLACK_CLIENT_ID`
   - **Client Secret** → `ARIEL_SLACK_CLIENT_SECRET`
   - **Signing Secret** → `ARIEL_SLACK_SIGNING_SECRET`
5. Find the workspace ID (starts with `T`) → `ARIEL_SLACK_TEAM_ID`. It is shown
   in the workspace URL of the Slack web client (`https://app.slack.com/client/T.../...`).
6. The event request URL is verified by Slack only when the server is running.
   Start the server (step 4) and re-verify the URL on **Event Subscriptions** if
   Slack reported it as unverified.
7. Invite the bot to the channels where it should answer (`/invite @ariel`).

Keep **Token Rotation** disabled (the manifest sets
`token_rotation_enabled: false`). Slack does not allow turning it off once it is
enabled, and Ariel stores long-lived user tokens.

## 2. Create the Cloud KMS key

Ariel encrypts user tokens with a symmetric Cloud KMS key. The key name passed to
Ariel is the crypto key, not a key version.

```sh
gcloud kms keyrings create ariel --location=global --project=$PROJECT_ID
gcloud kms keys create slack-user-token \
  --keyring=ariel --location=global --purpose=encryption --project=$PROJECT_ID

# Allow the service account that runs Ariel to encrypt and decrypt with this key.
gcloud kms keys add-iam-policy-binding slack-user-token \
  --keyring=ariel --location=global --project=$PROJECT_ID \
  --member=serviceAccount:$SERVICE_ACCOUNT \
  --role=roles/cloudkms.cryptoKeyEncrypterDecrypter
```

`ARIEL_KMS_KEY_NAME` is then
`projects/$PROJECT_ID/locations/global/keyRings/ariel/cryptoKeys/slack-user-token`.

Each ciphertext is bound to its owner through additional authenticated data
(`ariel:slack-user-token:v1:{TeamID}:{UserID}`), so a ciphertext copied into
another user's document cannot be decrypted. Do not disable or destroy the key
versions that encrypted stored tokens: those tokens become unreadable, and the
affected users have to sign in again.

## 3. Prepare Firestore

1. Create a Firestore database in Native mode. Ariel uses the `(default)`
   database unless `ARIEL_FIRESTORE_DATABASE_ID` is set.
2. Grant the service account `roles/datastore.user`.
3. Enable TTL policies so expired documents are deleted:

   ```sh
   gcloud firestore fields ttls update ExpiresAt --collection-group=sessions --enable-ttl --project=$PROJECT_ID
   gcloud firestore fields ttls update ExpiresAt --collection-group=slackEvents --enable-ttl --project=$PROJECT_ID
   ```

   TTL deletion runs some time after the expiry; Ariel also checks the expiry
   itself, so an expired session is rejected even before it is deleted.

No composite index is needed. Documents are laid out as follows:

| Path | Content |
| --- | --- |
| `teams/{TeamID}/users/{UserID}` | User (display name, timestamps) |
| `teams/{TeamID}/users/{UserID}/credentials/slack` | Encrypted Slack user token and granted scopes |
| `sessions/{SessionID}` | Web session (hash of the session secret, owner, expiry) |
| `slackEvents/{EventID}` | Record of a processed Slack event, used to drop redelivered events (kept 24 hours) |

Everything that belongs to a user is stored under that user's document path.

## 4. Run the server

```sh
ariel serve
```

| Flag | Environment variable | Default | Required | Description |
| --- | --- | --- | --- | --- |
| `--addr` | `ARIEL_ADDR` | `:8080` | | Listen address |
| `--base-url` | `ARIEL_BASE_URL` | | yes | Public URL, `scheme://host[:port]`. Used for the OAuth callback, the link in login prompts, and the `Secure` cookie attribute (`https` only) |
| `--session-ttl` | `ARIEL_SESSION_TTL` | `168h` | | Lifetime of a web session |
| `--log-level` | `ARIEL_LOG_LEVEL` | `info` | | `debug`, `info`, `warn`, `error` |
| `--log-format` | `ARIEL_LOG_FORMAT` | `console` | | `console`, `json` |
| `--repository-backend` | `ARIEL_REPOSITORY_BACKEND` | `firestore` | | `firestore`, or `memory` for local development (single process only) |
| `--firestore-project-id` | `ARIEL_FIRESTORE_PROJECT_ID` | | with `firestore` | Google Cloud project of Firestore |
| `--firestore-database-id` | `ARIEL_FIRESTORE_DATABASE_ID` | `(default)` | | Firestore database ID |
| `--slack-client-id` | `ARIEL_SLACK_CLIENT_ID` | | yes | Client ID of the Slack app |
| `--slack-client-secret` | `ARIEL_SLACK_CLIENT_SECRET` | | yes | Client secret of the Slack app |
| `--slack-signing-secret` | `ARIEL_SLACK_SIGNING_SECRET` | | yes | Signing secret, used to verify Events API requests |
| `--slack-bot-token` | `ARIEL_SLACK_BOT_TOKEN` | | yes | Bot user OAuth token (`xoxb-`) |
| `--slack-team-id` | `ARIEL_SLACK_TEAM_ID` | | yes | The only workspace Ariel accepts sign-ins and events from |
| `--kms-key-name` | `ARIEL_KMS_KEY_NAME` | | yes | Cloud KMS key for user tokens. There is no mode without KMS |

Google Cloud credentials are read from Application Default Credentials.

### Deployment notes

- Ariel can run as several instances; state shared between requests is kept in
  Firestore.
- Slack requires a response within three seconds, so Ariel acknowledges an
  event first and handles it in the background of the same process. On Cloud
  Run, set CPU to be always allocated; with CPU allocated only during requests,
  the background work is throttled after the response and replies are delayed
  or lost.
- Put TLS in front of the server and use an `https` base URL, so the session
  cookies carry the `Secure` attribute.

## Local development

- Backend: `ariel serve --repository-backend memory ...` with the Slack and KMS
  settings. The Slack app needs a public URL for the OAuth redirect and events;
  use a tunnel and set `--base-url` to it.
- Frontend: `task dev:frontend` starts Vite on port 5173 and forwards `/api` to
  `http://localhost:8080`.
- Build the frontend before building the binary: `task build:frontend`. The
  output in `frontend/dist` is embedded into the Go binary.

### Tests

- `go test ./...` runs every Go test. The repository tests run against both the
  in-memory backend and Firestore. The Firestore side connects to an emulator on
  `127.0.0.1:28615` (start it with `task test:firestore`, which needs Docker), or
  to a real project when `TEST_FIRESTORE_PROJECT_ID` is set.
- Tests that use a real Cloud KMS key run only when `TEST_GCP_KMS` is set to a
  key name (`projects/*/locations/*/keyRings/*/cryptoKeys/*`), for example with
  `zenv go test ./...`.
- Frontend: `pnpm test`, `pnpm lint`, and `pnpm build` in `frontend/`.
