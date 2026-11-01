## Microsoft 365 and Outlook (Graph)

Register an app in Microsoft Entra (personal or work accounts), add the `redirect_uri` shown by `POST /api/mailcal/accounts/{id}/oauth/start`, and store the client with `PUT /api/mailcal/oauth/clients/microsoft`. Build with `acton-plugin build-all --plugin mailcal-graph` and install `dist/mailcal-graph.actonpkg` through the ActonOS plugin marketplace. Scopes: `offline_access Mail.ReadWrite Mail.Send Calendars.ReadWrite`. Search returns one page (a Graph limit); mail delete moves to Deleted Items.

