# HTTP API v1

[Back to README](../README.md) · [WebSocket events](websocket.md)

Base path: `/api/v1`. Examples describe the current handlers and use cases,
not a separate generated OpenAPI specification. UUIDs are JSON strings;
timestamps use RFC 3339 with optional fractional seconds.

## Authentication and response format

Protected endpoints expect `Authorization: Bearer <access_token>`.
Request bodies are JSON (`Content-Type: application/json`).

Successful responses use `{"data": ...}`. `204 No Content` has no body.
Errors use:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid request",
    "fields": {"chat_id": "invalid uuid"}
  }
}
```

`fields` is optional. Branch on status/code rather than parsing human-readable
messages. Codes depend on the feature mapper; unexpected failures map to
`500 internal_error`, without exposing internal error details.

Refresh tokens use the `refresh_token` cookie: `HttpOnly`, `SameSite=Lax`, path
`/api/v1/auth`; `Secure` is enabled in production mode. Registration, login, and
refresh set this cookie. Refresh/logout do not take a refresh token in JSON.
For browser requests that need cookies, keep credentials enabled.

Revoking sessions invalidates future refresh operations. It does not immediately
invalidate an already-issued access JWT or terminate an existing WebSocket.

## Accounts

| Method and route | Request body | Success data / status |
| --- | --- | --- |
| `POST /auth/register` | `username`, `first_name`, `password`; optional `last_name`, `bio` | `201`: `user`, `access_token` |
| `POST /auth/login` | `username`, `password` | `200`: `access_token` |
| `POST /auth/refresh` | None; refresh cookie | `200`: `access_token`, rotated cookie |
| `POST /auth/logout` | None; refresh cookie | `204` |
| `PUT /auth/password` | `current_password`, `new_password` | `204`; sessions revoked |
| `GET /users/me` | None | `200`: user profile |
| `GET /users/{id}` | None | `200`: active user's profile |
| `GET /users/search` | Query: `q`, optional `limit` (default 10, max 20) | `200`: `users` |
| `PATCH /users/me` | Any of `username`, `first_name`, `last_name`, `bio` | `200`: updated profile |
| `DELETE /users/me` | None | `204`; anonymization and session revocation |

The password endpoint and all user endpoints require an access token. User
profile responses contain `id`, `username`, `first_name`, `last_name`, and `bio`.
The registration response's user object also includes `created_at`.

User search matches the beginning of username, without case sensitivity. `q`
accepts whitespace around the value and an optional leading `@`; only ASCII
letters, digits and underscores are accepted, at most 32 characters after
normalization. Queries shorter than two characters return an empty `users`
array. Invalid search values return `400 invalid_user_search`; a nonnumeric
`limit` returns `400 invalid_request`.

Each search result has `id`, `username` (original display casing), `first_name`
and nullable `last_name`. The requester and deleted accounts are excluded.
An expired/deleted requester cannot search. Results are ordered by exact match
first, then lowercase username and ID, with a fixed maximum of 20. Responses
use `Cache-Control: no-store`. The search SQL reads only those four public
columns; passwords, bio and token data are absent from results.

PATCH semantics:

- Omitted fields are unchanged; an empty object is permitted.
- `username` and `first_name`: strings replace values; `null` behaves as omission.
- `last_name` and `bio`: strings replace values; explicit `null` clears them.
- Username comparison/uniqueness is case-insensitive, but display casing is kept.

## Chats and membership

All endpoints below require an access token. `chat_id` identifies the common chat,
including when it appears in a group-specific route.

| Method and route | Request | Success |
| --- | --- | --- |
| `GET /chats/` | Query: `cursor`, `limit` | `200`: `chats`, `next_cursor` |
| `POST /chats/directs` | `{"peer_username":"@daniel_k"}` | `201` new / `200` existing chat |
| `POST /chats/groups` | `{"title":"Team","participant_usernames":["anya_1"]}` | `201`: group |
| `PUT /chats/groups/{chat_id}` | `{"title":"New title"}` | `200`: updated group |
| `GET /chats/groups/{chat_id}/participants` | Query: `cursor`, `limit` | `200`: `participants`, `next_cursor` |
| `POST /chats/groups/{chat_id}/participants` | `{"participant_usernames":["anya_1"]}` | `200`: per-input results |
| `DELETE /chats/groups/{chat_id}/participants` | JSON body: `{"target_username":"anya_1"}` | `204` |

Usernames are trimmed, accept one optional leading `@`, and are matched without
case sensitivity. After removing `@`, names must contain 5–32 ASCII letters,
digits or underscores. Group creation and addition accept at most 100 input
names, before deduplication. These four endpoints reject unknown JSON fields,
including obsolete `peer_id`, `participant_ids` and `target_id`, with `400
invalid_request`. Only one JSON value is accepted per request.

The service resolves usernames to stable user UUIDs using the existing users
repository, then calls the existing chat operations and permission checks.
Each distinct normalized name is looked up once per request; no persistent
username cache is used. Chat IDs, membership keys and profile URLs remain UUIDs,
so renaming an account does not change existing conversations.

A chat response contains `id`, `type`, `last_message_id`, `last_activity_at`, and
`created_at`. Group creation/update responses also contain `title`.

Chat-list items contain:

- `chat`: the common chat object;
- either `direct.peer` (`id`, `username`, `first_name`, `last_name`, `deleted_at`)
  or `group.title`;
- nullable `last_message` (`id`, `sender_id`, `sender_first_name`, `content`,
  `created_at`, `updated_at`);
- nullable `last_read_message_id` and numeric `unread_count`.

Direct creation uses the authenticated user and `peer_username`; it does not create a
self-chat. Repeating creation for the pair returns the existing chat, not its
message history. Unknown or deleted peers return `404 not_found`.

Group creation automatically adds the creator as `owner`. Including the creator's
username is harmless; repeated usernames resolve to one member. Initial members
must be available accounts; an unknown/deleted name returns `404 not_found`
with username-keyed details and no group is created. An empty initial list is allowed.

Any group participant may list participants. Each item contains `user_id`,
`username`, `first_name`, `last_name`, `role`, and `joined_at`. Anonymized accounts
remain visible through their stored profile values, including their generated
`deleted_…` username.

Owners/admins can change the title and add members. Batch addition accepts up to
100 usernames and returns one result per input, in input order. Result usernames
are normalized to lowercase without `@`:

```json
{
  "data": [
    {"username":"daniel_k","status":"added"},
    {"username":"daniel_k","status":"already_member"},
    {"username":"unknown_user","status":"unavailable"}
  ]
}
```

An empty addition list is permitted. New roles are always `member`. Missing or
deleted accounts produce `unavailable`; existing members/repeated successful
usernames produce `already_member`. Permissions are checked even if all supplied
names are unavailable. This is partial acceptance of business outcomes,
not a promise to hide infrastructure failures.

Non-owners may leave using their own `target_username`. An owner may remove other
participants; an admin may remove members. The owner cannot leave through this
endpoint. Deleted accounts can still be removed using their generated username
from the participant list. Role changes, ownership transfer, and group deletion
have no endpoints.

## Messages

All routes require an access token. New sends validate the sender's access and
account state; directs also reject unavailable peers.

| Method and route | Request | Success |
| --- | --- | --- |
| `POST /chats/{chat_id}/messages/` | `client_message_id`, `content` | `201` new / `200` repeated message |
| `GET /chats/{chat_id}/messages/` | Query: `cursor`, `limit`, `after` | `200`: `messages`, `next_cursor` |
| `PATCH /chats/{chat_id}/messages/{message_id}` | `{"content":"Updated text"}` | `200`: message |
| `DELETE /chats/{chat_id}/messages/{message_id}` | None | `204` |
| `PUT /chats/{chat_id}/messages/read` | `{"message_id":"<uuid>"}` | `204` |

HTTP message objects contain `id`, `client_message_id`, `chat_id`, `sender_id`,
`content`, `created_at`, and nullable `updated_at`.

Generate a `client_message_id` once for a logical send, then reuse it if the HTTP
response is lost. The key is unique per sender across all chats. A matching
stored message is returned with `200`; using the key with a different chat or
different normalized content returns `409 message_conflict`. Comparison is with
the currently stored content, so this is not an immutable historical response
cache. Physical deletion also removes the key; retrying after deletion can
create another message.

Only the author can edit/delete a message, while still having access to its chat.
Sending advances the sender's read marker in the same transaction. Mark-as-read
advances the marker without moving it backwards. Deleting a referenced message
repairs the last-message/read pointers; chat activity time is preserved.

Successful creation/editing/deletion can also produce [WebSocket events](websocket.md).
A successful HTTP response means persistence succeeded, **not** that every
recipient received a notification. Editing to the same normalized content and
returning an existing send do not emit another event.

## Cursor pagination

All three lists default to `limit=50`; `0` also selects the default. Explicit
limits from 1 to 100 are supported. Cursors are URL-safe base64-encoded JSON
values with `=` padding when needed, not offsets or authorization credentials.
A cursor does not grant access. Preserve the padding when passing a cursor back.

Chat and participant lists accept `cursor` and return the next page in descending
order: chats by `(last_activity_at, id)`, participants by `(joined_at, user_id)`.
Their `next_cursor` is `null` when no further page was found. Chat activity can
change while paging; this is not a fixed snapshot of the entire chat list.

Message direction is explicit:

| Query | Meaning and response order |
| --- | --- |
| No cursor | Latest N messages, newest first |
| `cursor=...` (`after=false`) | Messages strictly older than the cursor, newest first |
| `cursor=...&after=true` | Messages strictly newer than the cursor, oldest first |

`after=true` requires a cursor. The message ordering key is `(created_at, id)`;
timestamps alone are insufficient when two messages share a timestamp.

For older history, pass `next_cursor` unchanged until it is `null`.
For forward polling, `next_cursor` is a continuation position, **not a has-more
flag**: a nonempty page returns its last message's position; an empty page echoes
the input cursor. Keep advancing through nonempty pages; pause polling on an
empty page rather than looping immediately forever.

To start forward polling from the newest message already displayed, encode its
position (not the last/oldest item's history cursor):

```js
function messageCursor(message) {
  const json = JSON.stringify({
    message_id: message.id,
    created_at: message.created_at,
  });
  return btoa(json).replaceAll("+", "-").replaceAll("/", "_");
}
```

Preserve the returned timestamp string, including fractional precision. With no
messages available there is no position for `after=true`; fetch the latest page
again or wait for a new-message event. Forward history queries do not replay edits
or deletions of older messages and are not a durable synchronization protocol.
