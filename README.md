# speedboat

||||||| (empty tree)
=======

# Ticket Store

A local-first project management tool backed by **IndexedDB + Git + JSON + CRDT**.

## Architecture

```
React (Vite)
  └─ IndexedDB          local store — always available, works offline
       └─ SyncEngine    background sync; drains queue when backend connects
            └─ Go API   Git-backed REST — every write becomes a commit
                 └─ Git repo (JSON files)
```

All writes go to IndexedDB first. The sync engine flushes pending operations
to the backend whenever it is reachable, and CRDT-merges remote state into
local on first connect and on reconnect.

## Quick start

```bash
# Install dependencies and start both services
make dev

# Backend → http://localhost:8080
# Frontend → http://localhost:5173
```

## Configuration

### Backend flags

```
-port int      HTTP listen port (default 8080)
-host string   Bind host, empty = all interfaces (default "")
-repo string   Path to Git ticket repository (default "./ticket-data")
-cors string   Comma-separated allowed CORS origins
               (default "http://localhost:5173,http://localhost:4173,http://localhost:3000")
-verbose       Log every request
```

```bash
# Examples
make backend
make backend PORT=9090 REPO=/data/tickets VERBOSE=true
make backend HOST=0.0.0.0 CORS=https://app.example.com

# Or run the binary directly after building
make backend-build
./ticket-store -port 9090 -repo /data/tickets -cors https://app.example.com -verbose
```

### Frontend API URL

Three ways to set the backend URL (in order of precedence):

1. **Settings UI** — click Settings in the sidebar, enter the URL, hit Save.
   Persisted in IndexedDB, survives page reloads.
2. **`.env.local`** file in `frontend/`:

   ```
   VITE_API_URL=http://localhost:8080
   ```

3. **Default** — falls back to `http://localhost:8080`.

## CRDT field strategies

| Field | Strategy | Behaviour |
|-------|----------|-----------|
| `id`, `created`, `project` | immutable | Never overwritten after creation |
| `title`, `description`, `status`, `estimate_h` | lww-register | Latest `updated_at` wins |
| `priority` | max-register | Only ever escalates (`low` → `critical`) |
| `tags`, `assignees` | or-set | Union of all additions |
| `vote_count` | p-counter | Only ever increases |
| `comments` | append-log | Union by `id`, sorted by `ts` |

## Offline behaviour

- All CRUD operations write to IndexedDB immediately — the UI never blocks on the network.
- Pending operations are stored in an `sync_queue` store in IndexedDB.
- The sync badge in the topbar shows `offline` / `syncing…` / `live`.
- When the backend reconnects, the queue is flushed in insertion order,
  then remote state is pulled and CRDT-merged into local.

## Makefile targets

| Target | Description |
|--------|-------------|
| `make dev` | Backend + frontend, hot-reload |
| `make backend` | Go backend only (`go run`) |
| `make backend-build` | Compile `./ticket-store` binary |
| `make backend-run` | Build then run binary |
| `make frontend` | Vite dev server only |
| `make frontend-build` | Production build → `frontend/dist/` |
| `make install` | `go mod tidy` + `npm install` |
| `make build` | Binary + production frontend |
| `make clean` | Remove build artefacts |
| `make reset-data` | Wipe ticket data (prompts) |

## File layout

```
ticket-store/
  Makefile
  README.md
  backend/
    main.go          Go HTTP API with flag-based config
    go.mod
  frontend/
    index.html
    package.json
    vite.config.js
    .env.example
    src/
      main.jsx       React entry point
      App.jsx        Root component, state, CRUD
      index.css      CSS variables, shared styles
      db.js          IndexedDB wrapper (idb)
      api.js         Backend fetch client
      crdt.js        Client-side CRDT merge
      sync.js        SyncEngine (queue, flush, pull)
      components/
        Board.jsx    Kanban board, columns, cards
        Modals.jsx   Ticket, Project, History, Settings
        SyncBadge.jsx Connection status indicator
```

## API reference

| Method | Path | Description |
|--------|------|-------------|
| `GET`    | `/api/projects` | List projects |
| `POST`   | `/api/projects` | Create project |
| `GET`    | `/api/projects/:proj/board` | Kanban columns |
| `GET`    | `/api/projects/:proj/tickets` | List tickets |
| `POST`   | `/api/projects/:proj/tickets` | Create ticket |
| `GET`    | `/api/projects/:proj/tickets/:id` | Get ticket |
| `PUT`    | `/api/projects/:proj/tickets/:id` | Update ticket (CRDT merge) |
| `DELETE` | `/api/projects/:proj/tickets/:id` | Delete ticket |
| `GET`    | `/api/projects/:proj/history` | Git log |
