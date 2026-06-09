# FYP Data

Utilities and a small REST service for reconstructing FYP theme data from JSON files, building a local vector database, and browsing/searching the data.

## Requirements

- Go 1.26+
- Node.js and npm for the frontend
- JSON source files under `data/`
- An OpenAI-compatible embeddings API for vector search

SQLite is implemented with `modernc.org/sqlite`, so no CGO SQLite library is required. Vector storage uses `github.com/philippgille/chromem-go`.

## Build The SQLite Database

Create the full database from `data/p1.json` through `data/p4.json` and dictionary JSON files:

```bash
go run ./cmd/reconstruct-db -data data -out fyp-data.sqlite
```

Create a small test database from `data/small.json` only:

```bash
go run ./cmd/reconstruct-db -data data -out small.sqlite -small-only
```

Options:

```text
-data          Source JSON directory. Default: data
-out           SQLite database path. Default: fyp-data.sqlite
-include-small Also import small.json with p1-p4
-small-only    Import only small.json for theme rows
-incremental   Preserve the existing database and update/add imported rows
-theme-file    Theme page JSON file to import; repeat for multiple files
```

Incremental imports preserve existing row IDs when possible. Theme rows are updated by `source_file + themeId`; dictionary rows are updated by `source_file + dictType + dictValue`. This makes it possible to add a new page without rebuilding the whole database:

```bash
go run ./cmd/reconstruct-db -data data -out fyp-data.sqlite -incremental -theme-file p5.json
```

In incremental mode, the imported theme page files are treated as the latest complete theme dataset. Imported theme rows are marked `missing = false`; existing theme rows not seen in the latest import are retained and marked `missing = true`.

The generated database contains:

- `themes`: reconstructed theme records with `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `dictionaries`: dictionary rows with `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `raw_json`: original source object for each row

## Build The Vector Database

Copy the example config and fill in the embeddings API fields:

```bash
cp config/vector-db.example.json config/vector-db.json
```

Example config:

```json
{
  "embedding_api": {
    "base_url": "https://api.example.com/v1",
    "model": "text-embedding-model-name",
    "api_key": "replace-with-your-api-key",
    "requests_per_minute": 60
  },
  "sqlite_path": "fyp-data.sqlite",
  "vector_db_path": "theme-vectors",
  "collection_name": "themes",
  "request_timeout_seconds": 120,
  "batch_size": 16,
  "concurrency": 1
}
```

Build vectors:

```bash
go run ./cmd/build-vector-db -config config/vector-db.json
```

Options:

```text
-config      Vector DB config path. Default: config/vector-db.example.json
-sqlite      Override sqlite_path from config
-out         Override vector_db_path from config
-incremental Preserve the existing vector DB and embed only absent vector documents
```

Use incremental mode after adding new SQLite rows:

```bash
go run ./cmd/build-vector-db -config config/vector-db.json -incremental
```

Each vector document embeds:

- `themeTitle`
- `themeProjectDescription`

Metadata includes SQLite ID, theme ID, teacher, department, subject area, source file, and embedded field names.

Vector builds read only themes where `missing = false`.

The existing vector database in this repository was generated with `Qwen3-Embedding-8B`. If you use the existing `theme-vectors` database, configure the REST API with the same embedding model. Query embeddings must be produced by the same model as the stored document embeddings.

## Run The REST API

Copy and edit the API config:

```bash
cp config/api.example.json config/api.json
```

Example config:

```json
{
  "listen_addr": "127.0.0.1:8080",
  "sqlite_path": "fyp-data.sqlite",
  "vector_db_path": "theme-vectors",
  "collection_name": "themes",
  "embedding_api": {
    "base_url": "",
    "model": "",
    "api_key": "",
    "requests_per_minute": 60,
    "request_timeout_seconds": 120,
    "cache_size": 256
  }
}
```

Start the service:

```bash
go run . -config config/api.json
```

If `embedding_api` is left blank, all SQLite browsing endpoints work and semantic search returns `503`. Fill in `embedding_api` to enable `/semantic-search`.

`embedding_api.cache_size` controls an in-memory LRU cache for query embeddings. The default is `256` entries; set it to `-1` to disable caching.

Theme responses keep raw coded fields and also include a `labels` object for resolved dictionary labels where available. For example, `themeSubjectArea: "3"` is accompanied by `labels.themeSubjectArea`.

## Run The Frontend

The frontend is a separate Vite React project under `frontend/`. It provides a friendly theme search interface with filters, semantic search, an optional negative prompt, and a theme detail view.

Install frontend dependencies:

```bash
cd frontend
npm install
```

During development, keep the REST API running on `127.0.0.1:8080` and start the Vite dev server:

```bash
npm run dev
```

Vite proxies API calls to the Go service, so the development frontend is available at the local URL printed by Vite, usually `http://127.0.0.1:5173/`.

To serve the frontend from the Go REST service, build the frontend first:

```bash
cd frontend
npm run build
cd ..
go run . -config config/api.json
```

The Go service serves `frontend/dist` for non-API routes. After the build, open `http://127.0.0.1:8080/`.

## API Documentation

All endpoints return JSON.

### `GET /health`

Returns service status, row counts, and vector DB availability.

Example:

```bash
curl http://127.0.0.1:8080/health
```

Response fields:

```json
{
  "ok": true,
  "sqlite_path": "fyp-data.sqlite",
  "theme_count": 392,
  "dictionary_count": 95,
  "vector_db_path": "theme-vectors",
  "vector_collection": "themes",
  "vector_loaded": true,
  "vector_load_error": "",
  "semantic_available": true
}
```

### `GET /themes`

Lists themes with pagination and optional filters.

Query parameters:

```text
limit         Page size, 1-200. Default: 50
offset        Row offset. Default: 0
state         Filter by themeState
subject_area  Filter by themeSubjectArea
programme     Filter by one value in themeProgramme
teacher_id    Filter by themeTeacherId
project_type  Filter by themeProjectType
theme_type    Filter by themeType
missing       Filter by latest-import marker: true/false or 1/0
```

Example:

```bash
curl 'http://127.0.0.1:8080/themes?limit=10&subject_area=3&programme=1'
```

Response:

```json
{
  "total": 123,
  "limit": 10,
  "offset": 0,
  "rows": [
    {
      "id": 1,
      "missing": false,
      "themeSubjectArea": "3",
      "themeTitle": "...",
      "themeProjectDescription": "...",
      "labels": {
        "themeSubjectArea": {
          "label": "Artificial Intelligence Technology",
          "label_en": "Artificial Intelligence Technology"
        }
      }
    }
  ]
}
```

### `GET /themes/{id}`

Returns one theme by SQLite row ID. The database stores `raw_json`, but API responses omit it to keep payloads small.

Example:

```bash
curl http://127.0.0.1:8080/themes/1
```

### `GET /dictionaries`

Lists dictionary rows.

Query parameters:

```text
limit   Page size, 1-500. Default: 100
offset  Row offset. Default: 0
type    Optional dictType filter
```

Example:

```bash
curl 'http://127.0.0.1:8080/dictionaries?type=theme_subject_area'
```

### `GET /dictionary-types`

Lists available dictionary types and row counts.

Example:

```bash
curl http://127.0.0.1:8080/dictionary-types
```

Response:

```json
[
  {
    "dictType": "theme_subject_area",
    "count": 7
  }
]
```

### `GET /search`

Performs exact SQLite text search over theme title, project description, teacher name, and department name.

Query parameters:

```text
q             Required search text
limit         Page size, 1-100. Default: 25
offset        Row offset. Default: 0
state         Filter by themeState
subject_area  Filter by themeSubjectArea
programme     Filter by one value in themeProgramme
teacher_id    Filter by themeTeacherId
project_type  Filter by themeProjectType
theme_type    Filter by themeType
```

Example:

```bash
curl 'http://127.0.0.1:8080/search?q=tomato&programme=1&limit=5'
```

### `GET /semantic-search`

Performs vector search against the `chromem-go` collection. Requires vector DB files and embedding API config.

Query parameters:

```text
q         Required search prompt
negative  Optional negative prompt, applied with chromem NEGATIVE_MODE_SUBTRACT
limit     Number of results, 1-50. Default: 10
```

Examples:

```bash
curl 'http://127.0.0.1:8080/semantic-search?q=computer%20vision&limit=5'
curl 'http://127.0.0.1:8080/semantic-search?q=computer%20vision&negative=hardware&limit=5'
```

Response:

```json
{
  "query": "computer vision",
  "negative": "hardware",
  "negative_mode": "subtract",
  "rows": [
    {
      "similarity": 0.82,
      "theme": {
        "id": 42,
        "missing": false,
        "themeTitle": "...",
        "labels": {
          "themeSubjectArea": {
            "label": "Artificial Intelligence Technology",
            "label_en": "Artificial Intelligence Technology"
          }
        }
      }
    }
  ]
}
```

## Development Checks

```bash
go test ./...
cd frontend && npm run lint && npm run build
```

The working embedding config may contain secrets. Keep `config/vector-db.json` and `config/api.json` private if they contain API keys.
