# mock — Mock servers for testing

```bash
mu mock mock-server --port 8081 --size 100
mu mock file-server --port 8082 --local-dir ./uploads
mu mock oauth-server --port 8083
mu mock dynamic-server --config mock-config.json
```

## dynamic-server — Configurable multi-endpoint mock with hot-reload and admin UI

```bash
mu mock dynamic-server --config mock-config.json
```

Starts a mock server with a **web admin UI** at `http://localhost:8084/__admin/` where you can
add, edit, delete, and save endpoints in real time — no restart needed.

### Admin UI

Open `http://localhost:8084/__admin/` in your browser:

- **Table** of all endpoints (method, path, status, delay)
- **Add Endpoint** button to create new endpoints
- **Edit** / **Del** actions per endpoint
- **Save to Config** button persists all current endpoints to the config file

Endpoints created or modified via the UI take effect immediately on the next matching request.

### Config file format

```json
{
  "port": 8084,
  "endpoints": [
    {
      "id": "a1b2c3",
      "method": "POST",
      "path": "/api/users",
      "status": 201,
      "delay": "500ms",
      "headers": { "X-Request-Id": "{{header.x-request-id}}" },
      "body": "{\"created\": true, \"name\": \"{{body.name}}\"}"
    },
    {
      "id": "d4e5f6",
      "method": "GET",
      "path": "/api/users/:id",
      "status": 200,
      "body": "{\"id\": \"{{path.id}}\", \"name\": \"User {{path.id}}\", \"page\": \"{{query.page}}\"}"
    }
  ]
}
```

The `body` field is always a raw response string (JSON or plain text).

**Features:**

| Feature | Description |
|---|---|
| Admin web UI | `GET /__admin/` — browser-based endpoint management |
| Hot-reload | Add/edit/delete endpoints at runtime without restart |
| Template variables | `{{path.id}}` `{{query.page}}` `{{header.authorization}}` `{{body.name}}` |
| Custom status code | Per-endpoint `"status": 201`, `404`, `500`, etc. |
| Custom headers | `"headers": {"X-Custom": "value"}` (supports template variables) |
| Delay simulation | `"delay": "2s"` / `"500ms"` / `"1.5s"` |
| Path parameters | `/api/users/:id` matches `/api/users/42`, param available as `{{path.id}}` |
| Persistence | "Save to Config" button writes all endpoints back to the config file |
| Verbose logging | `--verbose` flag prints request/response details to stdout |

| Conditional responses | Per-endpoint `"responses"` with conditions using template expressions |
| Recursive conditions | Each condition can contain child conditions for multi-level branching |
| Default fallback | Conditionless `"default": true` response, or parent `body` as implicit fallback |

### Conditional responses

Each endpoint can specify a list of `"responses"` evaluated in order. The first matching
condition wins; if none match, the endpoint's own fields serve as the fallback.

```json
{
  "method": "GET",
  "path": "/api/hello/:id",
  "status": 200,
  "delay": "100ms",
  "body": "{\"message\": \"Hello guest!\", \"id\": \"{{path.id}}\"}",
  "responses": [
    {
      "condition": "{{path.id}} == 1",
      "delay": "500ms",
      "headers": {"X-Role": "admin"},
      "body": "{\"message\": \"Hello Admin!\", \"role\": \"admin\"}"
    },
    {
      "condition": "{{path.id}} == 2",
      "body": "{\"message\": \"Hello User!\", \"role\": \"user\"}"
    }
  ]
}
```

**Operator reference:**

| Operator | Example | Effect |
|----------|---------|--------|
| *(none)* | `{{header.auth}}` | Exists / non-empty |
| `==` | `{{path.id}} == 1` | Equal |
| `!=` | `{{path.id}} != admin` | Not equal |
| `>` / `<` / `>=` / `<=` | `{{path.id}} > 100` | Numeric comparison |
| `contains` | `{{body.email}} contains @` | Substring match |
| `matches` | `{{path.id}} matches ^\\d+$` | Regex match |

### Default demo endpoints

When the **gateway** (`mu gateway`) starts and `mock-config.json` does not exist yet,
it auto-creates two demo endpoints:

| Route | Description |
|-------|-------------|
| `GET /api/hello` | Simple greeting — quick sanity check |
| `GET /api/hello/:id` | Conditional response demo with path params, headers, delay |

Visit `http://localhost:8080/mock/api/hello` or `http://localhost:8080/mock/api/hello/1`
to try them.

**Template sources:**

| Source | Syntax | Example |
|---|---|---|
| URL path param | `{{path.xxx}}` | `/api/users/:id` → `{{path.id}}` |
| Query string | `{{query.xxx}}` | `?page=1` → `{{query.page}}` |
| Request header | `{{header.xxx}}` | `Authorization: Bearer x` → `{{header.authorization}}` |
| JSON body | `{{body.xxx}}` | `{"name":"alice"}` → `{{body.name}}` |
| Nested body | `{{body.x.y.z}}` | `{"user":{"name":"alice"}}` → `{{body.user.name}}` |

> **Note:** Conditional responses (`"responses"` array with `when`/`then`) are not yet
> supported via the admin UI but can still be added by editing the JSON config file
> manually (they will be preserved through save operations).
