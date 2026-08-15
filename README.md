# AI Assistant

PicoClaw Telegram assistant using Sumopod's Responses API with `deepseek-v4-pro`, plus two local Go services: a finance API and a job-alert module that scrapes Glints and Jobstreet.

## Setup

Set `.env` from `.env.example`. For the one-time migration from SealedSecret, first push these manifest changes and sync the Argo CD application so the old SealedSecret is pruned. Then create the Kubernetes Secret:

```sh
./k8s/apply-secret.sh
```

The script applies `Secret/ai-assistant-env` directly from `.env`; neither `.env` nor a secret manifest is committed to Git. For later secret updates, run the script again and restart or sync the application.

Required secrets:

- `SUMOPOD_API_KEY`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_USER_ID` from Telegram's numeric user ID

## Google Spreadsheet finance sync

Every new finance record can be written to the matching Indonesian month tab in a Google Spreadsheet. The integration preserves the supplied workbook layout: it inserts a row before `Total` and fills `No`, `Tanggal`, `Nama Pengeluaran`, `Jumlah`, optional `Kategori`, and `Harga`.

1. Upload `Keuangan per Juli - Desember 2026.xlsx` to Google Drive and open it as a Google Spreadsheet.
2. Enable the Google Sheets API in a Google Cloud project.
3. Create a service account and download its JSON key.
4. Share the Google Spreadsheet with the service-account email as **Editor**.
5. Copy the spreadsheet ID from its URL. Run this once to encode the service-account JSON, then paste its output into `.env`:

   ```sh
   base64 < service-account.json | tr -d '[:space:]'
   ```

   Add these values to `.env`:

   ```sh
   GOOGLE_SHEETS_SPREADSHEET_ID=your-spreadsheet-id
   GOOGLE_SERVICE_ACCOUNT_JSON_BASE64=paste-the-base64-output-here
   ```

6. Apply the updated Secret, rebuild and push the image, then sync the Argo CD application:

   ```sh
   ./k8s/apply-secret.sh
   ```

The spreadsheet must contain tabs named `Januari` through `Desember` for the months you use. Leave both Google variables empty to keep local SQLite-only finance records.

The assistant only accepts the configured Telegram user. Its state, finance database, and cron jobs persist in the `ai-assistant-data` PVC. This ledger is independent from `ryuko-matoi-go`.

The paycheck reminder runs at 09:00 Asia/Jakarta every 28th. `/downloadrecap` sends an XLSX file that opens directly in Google Sheets.

## Job Search (`/loker`)

Search job listings from Glints and Jobstreet with AI-powered filtering.

### Usage

```
/loker <posisi> | <skills> | <pengalaman> | <lokasi>
```

Examples:

```
/loker react developer | react,typescript | 1-3 | jakarta
/loker golang engineer | golang,postgresql,docker | 1-3 | jakarta,bandung
/loker frontend developer
```

Fields after `|` are optional — defaults to `react,node,typescript`, `1-3` years, and `jakarta`.

### Daily cron

Runs automatically at 03:00 Asia/Jakarta via `loker-bot.sh` with default keywords:

`fullstack developer`, `fullstack engineer`, `devops engineer`, `frontend developer`, `backend developer`, `frontend engineer`, `backend engineer`, `product developer`, `product engineer`.

### Data sources

| Source | Method | Endpoint |
|---|---|---|
| Glints | GraphQL (`searchJobsV3`) | `https://glints.com/api/v2-alc/graphql` |
| Jobstreet | SSR HTML scraping | `https://id.jobstreet.com/id/jobs?keywords=…` |

Jobstreet (SEEK) renders search results server-side; `job-alert` parses `data-automation="normalJob"` job cards for title, company, location, salary, work type, and listing date. Glints exposes a public GraphQL endpoint that returns structured jobs directly.

### Filtering

1. **Location** — substring match against Jabodetabek, Bandung, Surabaya, Bali, Batam, Solo, Salatiga, Karawang, Cikampek, Cikarang.
2. **Experience** — `maxYearsExp` bound (default 3 years); jobs with `minYearsOfExperience > 3` are dropped.
3. **Tech stack** — passed to Sumopod's Responses API (`/v1/responses`); the AI returns only matching jobs. If the AI returns nothing or an unparseable response, the pre-filtered list is kept.

Max 20 results per source.

### How `/loker` works

```
Telegram /loker → loker-bot.sh (intercepts via getUpdates)
                  → /usr/local/bin/loker "<query>"
                  → job-alert --keywords … --skills … --experience … --location …
                  → fetch Glints + Jobstreet → filter → Sumopod AI filter → Telegram
```

### Architecture

| Service | Port | Role |
|---|---|---|
| picoclaw | 18790 | AI agent gateway, Telegram bot |
| finance-api | 8080 | Finance ledger + Google Sheets sync + Sumopod Responses proxy |
| loker-api | 8081 | HTTP endpoint that runs a job search on request |
| loker-bot | — | Background: intercepts `/loker` and runs the 03:00 daily alert |
| job-alert | — | Go binary: fetch, filter, and deliver job listings |

### Binaries and scripts

| Path | Purpose |
|---|---|
| `job-alert/main.go` | Fetcher + filter + Telegram delivery (one-shot binary) |
| `job-alert/loker.sh` | Wrapper that parses the `\|` query into `job-alert` flags |
| `job-alert/loker-bot.sh` | Telegram poller for `/loker` + daily cron scheduler |
| `loker-api/main.go` | HTTP service (`POST /loker`) that spawns `loker` in the background |
| `finance-api/` | Finance ledger, recap, and Sumopod Responses proxy |
