# AI Assistant Chat API

This document describes the chat-related APIs of the Nightingale AI assistant.

Every successful response uses the standard Nightingale JSON envelope:

```json
{
  "dat": {},
  "err": "",
  "request_id": "..."
}
```

## Rename a chat

```text
POST /api/n9e/assistant/chat/rename
```

Requires login. A caller may only rename chats they created themselves.

### Request body

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `chat_id` | string | Yes | ID of the chat to rename |
| `title` | string | Yes | The new title; it must not be empty or consist only of whitespace |

```json
{
  "chat_id": "ad4075b8-ea4e-43d5-954c-71bb951874ca",
  "title": "Production alert troubleshooting"
}
```

### Response

On success, `dat` holds the updated chat object:

```json
{
  "dat": {
    "chat_id": "ad4075b8-ea4e-43d5-954c-71bb951874ca",
    "title": "Production alert troubleshooting",
    "last_update": 1730000000,
    "page_from": {
      "page": "dashboards"
    },
    "user_id": 1,
    "is_new": false,
    "is_renamed": true
  },
  "err": "",
  "request_id": "..."
}
```

`is_renamed` indicates that the title was set manually by the user. If the chat is renamed before the first message is sent, the server will no longer overwrite that title automatically with the content of the first message. Renaming itself does not change `last_update`, so it does not affect the ordering of the chat history list.

### Errors

| HTTP status | Scenario |
| --- | --- |
| 400 | `chat_id` is missing, or `title` is empty / whitespace only |
| 200 | The chat does not exist or does not belong to the current user; the error message is returned in `err` using the standard n9e error format |
