## Gmail and Google Calendar

You bring your own OAuth client, so ActonOS never goes through Google's app review.

1. In the Google Cloud Console create a project, enable the **Gmail API** and the **Google Calendar API**, and create an OAuth client of type **Web application**. While the app is in "Testing", add your own Google account as a test user. (Refresh tokens of apps in Testing expire after 7 days; reconnect when the account shows "sign-in expired".)
2. Build and install the plugin:
   ```bash
   acton-plugin build-all --plugin mailcal-gmail
   install dist/mailcal-gmail.actonpkg through the ActonOS plugin marketplace (or copy it into <data dir>/plugins/)
   ```
   Unsigned plugins need `ACTONOS_ALLOW_UNSIGNED_PLUGINS=1` unless you sign the package.
3. Store your client: `PUT /api/mailcal/oauth/clients/google` with `{client_id, client_secret}`.
4. Create an account with `plugin_id: "mailcal-gmail"`, `kind: "both"`, `auth: {"type": "oauth"}`, then `POST /api/mailcal/accounts/{id}/oauth/start`. Add the returned `redirect_uri` to the client's authorized redirect URIs and open `auth_url`.

The plugin asks only for the Gmail "modify" scope (read, draft, send, label, trash; not permanent delete) and the Calendar events and free/busy scopes.

