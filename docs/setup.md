# Setup

This document describes how to run Ariel: the Slack app, Google Cloud (Cloud KMS
and Firestore), the optional Google Workspace integration, the server
configuration, and local development.

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

## How Ariel uses Google Workspace

The Google Workspace integration is optional and is enabled only when the
server has a Google OAuth client (step 4). It is meant for one Google Workspace
organization: the OAuth client belongs to that organization and admits only
its accounts.

- Each user connects their own Google account on the settings page. Ariel asks
  Google for these scopes:

  | Scope | Used for |
  | --- | --- |
  | `openid`, `email` | Identify the connected Google account and show its address on the settings page |
  | `https://www.googleapis.com/auth/calendar.readonly` | Read the user's calendars and events |
  | `https://www.googleapis.com/auth/drive.readonly` | Search Drive and read files, including Google Docs, Sheets, and Slides |
  | `https://www.googleapis.com/auth/gmail.readonly` | Search and read the user's mail |

  No scope allows creating, changing, deleting, or sending anything. Ariel
  currently obtains and stores this access only; reading Calendar, Drive, or
  Gmail will be added with the features that need it.
- Google lets a user uncheck individual permissions on the consent screen. If
  the Calendar, Drive, or Gmail permission is not granted, Ariel revokes the
  grant at Google, stores nothing, and tells the user to connect again.
- Ariel stores the connected account as authorized in that sign-in; it does not
  compare it with the Slack account.
- One Google account can be connected to only one Ariel user. Google revokes a
  grant per Google account and Cloud project, not per token, so if two users
  shared one Google account, one user's disconnection would end the other's
  access. A user who authorizes an account that is already connected to someone
  else is told so; Ariel stores nothing and leaves that account's grant as it
  is.
- A user who is already connected cannot connect a second account: the
  connection is ignored and the settings page stays as it is. Disconnect first
  to switch accounts.
- Ariel stores only the refresh token, encrypted with Cloud KMS, together with
  the granted scopes and the account's address. Access tokens are not stored.
- **Disconnect Google Workspace** on the settings page revokes the grant at
  Google and deletes the stored token. Signing out of Ariel does not disconnect
  Google Workspace. A user can also remove Ariel from the third-party apps
  list of their Google account.
- Google stops accepting a refresh token when the user revokes access, when it
  has not been used for six months, when the user changes their password (the
  Gmail scope is included), or when an administrator restricts one of the
  services. The settings page does not ask Google whether the stored token is
  still accepted, so it keeps showing **Connected** in these cases; disconnect
  and connect again to obtain a new token.

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
   Start the server (step 5) and re-verify the URL on **Event Subscriptions** if
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

The same key encrypts the Google refresh tokens of the Google Workspace
integration.

Each ciphertext is bound to its owner through additional authenticated data
(`ariel:slack-user-token:v1:{TeamID}:{UserID}` for Slack,
`ariel:google-refresh-token:v1:{TeamID}:{UserID}` for Google), so a ciphertext
copied into another user's document cannot be decrypted. Do not disable or
destroy the key versions that encrypted stored tokens: those tokens become
unreadable, the affected users have to sign in again, and they cannot
disconnect Google Workspace until the key version is restored.

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
| `teams/{TeamID}/users/{UserID}/credentials/google_workspace` | Encrypted Google refresh token, granted scopes, and the connected Google account (ID and address) |
| `googleWorkspaceAccounts/{GoogleAccountID}` | The only Ariel user a Google account is connected to. Created and deleted together with the credential above |
| `sessions/{SessionID}` | Web session (hash of the session secret, owner, expiry) |
| `slackEvents/{EventID}` | Record of a processed Slack event, used to drop redelivered events (kept 24 hours) |

Everything that belongs to a user is stored under that user's document path.
`googleWorkspaceAccounts` is the exception: it is looked up by the Google
account to keep one Google account from being connected to two users, and it
holds only the owner's Slack IDs.

## 4. Set up the Google Workspace integration (optional)

Skip this step to run Ariel without Google Workspace; the settings page then
shows the integration as not available. Use a Google Cloud project that belongs
to your Google Workspace organization, signed in as a user of that
organization.

1. Enable the APIs. In the Google Cloud console, open **APIs & Services** →
   **Library** and enable **Google Calendar API**, **Google Drive API**, and
   **Gmail API**.
2. Configure the consent screen. Open **Google Auth platform** → **Branding**
   and enter the app name (for example `Ariel`) and a user support email.
   On **Audience**, set the user type to **Internal**. This is required: with
   Internal, Google admits only accounts of your organization, and Ariel does
   not check the account's domain itself. Internal apps do not need Google's
   verification for the sensitive and restricted scopes above.
3. Register the scopes (optional for Internal apps, but it documents what the
   app asks for). On **Data Access** → **Add or Remove Scopes**, add `openid`,
   `.../auth/userinfo.email`, `.../auth/calendar.readonly`,
   `.../auth/drive.readonly`, and `.../auth/gmail.readonly`.
4. Create the OAuth client. Open **Google Auth platform** → **Clients** →
   **Create Client**, choose the application type **Web application**, and
   under **Authorized redirect URIs** add
   `https://ariel.example.com/api/integrations/google-workspace/callback`,
   with your `ARIEL_BASE_URL` in place of `https://ariel.example.com`. Click
   **Create** and copy:
   - **Client ID** → `ARIEL_GOOGLE_CLIENT_ID`
   - **Client secret** → `ARIEL_GOOGLE_CLIENT_SECRET` (store it right away; the
     console may not show it again)
5. Allow the app in the Google Admin console. If Gmail, Drive, or Calendar is
   set to **Restricted** under **Security** → **API controls**, Google refuses
   the grant until the app is trusted. Either select **Trust internal apps**
   under **API controls** → **Settings** → **Internal apps**, or open
   **Manage Third-Party App Access** → **Add app** → **OAuth App Name or
   Client ID**, search for the client ID, select it, and choose **Trusted**.
6. Start Ariel with both values set (step 5). Users then connect their account
   with **Connect Google Workspace** on the settings page.

## 5. Run the server

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
| `--slack-client-id` | `ARIEL_SLACK_CLIENT_ID` | | yes (not with `--no-auth`) | Client ID of the Slack app |
| `--slack-client-secret` | `ARIEL_SLACK_CLIENT_SECRET` | | yes (not with `--no-auth`) | Client secret of the Slack app |
| `--slack-signing-secret` | `ARIEL_SLACK_SIGNING_SECRET` | | yes (with `--no-auth`: together with the bot token, or neither) | Signing secret, used to verify Events API requests |
| `--slack-bot-token` | `ARIEL_SLACK_BOT_TOKEN` | | yes (with `--no-auth`: together with the signing secret, or neither) | Bot user OAuth token (`xoxb-`) |
| `--slack-team-id` | `ARIEL_SLACK_TEAM_ID` | | yes | The only workspace Ariel accepts sign-ins and events from |
| `--kms-key-name` | `ARIEL_KMS_KEY_NAME` | | yes (not with `--no-auth`) | Cloud KMS key for the Slack and Google user tokens |
| `--google-client-id` | `ARIEL_GOOGLE_CLIENT_ID` | | with `--google-client-secret` | Client ID of the Google OAuth client (step 4). Setting both Google values enables the Google Workspace integration |
| `--google-client-secret` | `ARIEL_GOOGLE_CLIENT_SECRET` | | with `--google-client-id` | Client secret of the same OAuth client |
| `--no-auth` | `ARIEL_NO_AUTH` | | | Development and E2E only. A Slack user ID (`U...`) of `--slack-team-id`: every web sign-in becomes this user without asking Slack, and no user token is stored. Accepted only with `--repository-backend memory` |

With `--no-auth`, the Slack event endpoint (`/hooks/slack/event`) exists only
when both the bot token and the signing secret are set.

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

- Backend with Slack: `ariel serve --repository-backend memory ...` with the
  Slack and KMS settings. The Slack app needs a public URL for the OAuth
  redirect and events; use a tunnel and set `--base-url` to it.
- Backend without Slack: `ariel serve --base-url http://localhost:8080
  --repository-backend memory --slack-team-id T0123ABCD --no-auth U0123ABCD`.
  "Sign in with Slack" signs you in as `U0123ABCD` directly.
- Google Workspace in local development: `--no-auth` does not skip Google. With
  the Google flags set, **Connect Google Workspace** goes to the real Google
  authorization, and storing the token needs `--kms-key-name`; without it the
  connection fails after Google returns. The redirect URI registered in the
  OAuth client must match `--base-url`.
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
- E2E: `task e2e` builds the binary and runs the Playwright tests in
  `frontend/e2e/tests/` against it, started with `--no-auth` and the in-memory
  repository. Install the browser once with
  `pnpm exec playwright install chromium` in `frontend/`.
- Screenshots for pull requests: `task screenshots` captures every screen state
  into `frontend/screenshots/`. Attach them to the PR description with
  `gh pr edit <number> --body-file <body.md> --attach '<file>#<alt text>' ...`;
  a body reference to the same path, such as
  `![Login: failed](./frontend/screenshots/login-failed.png)`, is rewritten to
  the uploaded image. Do not commit them or push them to any branch.
