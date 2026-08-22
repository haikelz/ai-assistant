# AI Assistant

PicoClaw Telegram assistant with configurable Sumopod, Google Gemini, or OpenAI models. The local backend is a Go Fiber modular monolith for finance, job search, and AI-provider compatibility.

## Setup

Copy `.env.example` to `.env`, configure the AI provider and model, then create the Kubernetes Secret:

```sh
./k8s/apply-secret.sh
```

The script applies `Secret/ai-assistant-env` directly from `.env`; neither `.env` nor a generated secret manifest is committed to Git. For later configuration updates, run the script again and restart the deployment.

Required variables:

- `AI_PROVIDER`: `sumopod`, `google`, or `openai`
- `AI_MODEL`: the provider's model ID
- The matching API key: `SUMOPOD_API_KEY`, `GOOGLE_API_KEY`, or `OPENAI_API_KEY`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_USER_ID` from Telegram's numeric user ID
- Optional `WHATSAPP_RECIPIENT` to send job results to one WhatsApp number

Examples:

```dotenv
AI_PROVIDER=sumopod
AI_MODEL=your-sumopod-model
SUMOPOD_API_KEY=...

# Or Google Gemini:
# AI_PROVIDER=google
# AI_MODEL=gemini-2.5-flash
# GOOGLE_API_KEY=...

# Or OpenAI:
# AI_PROVIDER=openai
# AI_MODEL=gpt-5-mini
# OPENAI_API_KEY=...
```

`AI_MODEL` configures both PicoClaw and halal company assessment. At startup, the container regenerates PicoClaw's model config from these variables, replacing stale provider/model settings from the PVC. The existing Sumopod Responses API proxy path remains unchanged.

For `AI_PROVIDER=sumopod`, models containing `gpt` continue through Sumopod's Responses API, but the local proxy removes PicoClaw's unsupported `temperature` field. If Sumopod returns Responses events as an SSE stream, the proxy converts its final `response.completed` event into the JSON response PicoClaw expects. DeepSeek and other Sumopod models retain the original request payload.

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

6. Apply the updated Secret, rebuild and push the image, then restart the Kubernetes deployment:

   ```sh
   ./k8s/apply-secret.sh
   kubectl apply -k k8s
   kubectl rollout restart deployment/ai-assistant -n default
   kubectl rollout status deployment/ai-assistant -n default
   ```

The spreadsheet must contain tabs named `Januari` through `Desember` for the months you use. Leave both Google variables empty to keep local SQLite-only finance records.

The assistant only accepts the configured Telegram user. Its state, finance database, and cron jobs persist in the `ai-assistant-data` PVC. This ledger is independent from `ryuko-matoi-go`.

The paycheck reminder runs at 09:00 Asia/Jakarta every 28th. `/downloadrecap` sends an XLSX file that opens directly in Google Sheets.

## Job Search (`/loker`)

Search job listings from Kitalulus and Dealls with deterministic filtering.

### Usage

```
/loker <posisi> | <skills> | <pengalaman> | <lokasi> | [halal]
```

Examples:

```
/loker react developer | react,typescript | 1-3 | jakarta
/loker golang engineer | golang,postgresql,docker | 1-3 | jakarta,bandung
/loker software engineer | go,typescript | 1-3 | jakarta | halal
/loker frontend developer
```

The fifth field is optional. When set to `halal`, AI assesses each company's primary business against these criteria:

1. It does not primarily produce or sell prohibited goods or services.
2. Its primary business model is not connected to interest-based financing.
3. It is not a bank, insurance company, online lender, or interest-based financing business.

Every result remains visible and receives one label: `Halal`, `Tidak Halal — <reason>`, or `Perlu Riset`. The conservative `Perlu Riset` label is used when evidence is insufficient, the AI request fails, or no API key is configured. Without the `halal` field, no company assessment is requested and no label is shown.

### Daily cron

Runs automatically at 03:00 Asia/Jakarta via `job-alert-scheduler` with halal labeling enabled. The daily run retrieves current listings from both active sources and limits output to 20 matching results per source.

Configure its search criteria from Telegram with the same fields as `/loker`:

```
/job-alert Software Engineer | go,typescript | 1-3 | Jakarta
```

Position is required. Skills, experience, and location remain optional. Sending
`/job-alert` without arguments shows the active configuration. The setting is
stored at `/root/.picoclaw/job-alert.json` on the existing persistent volume, so
it survives pod restarts. Halal labeling is always enabled for scheduled runs,
regardless of whether `halal` is included in the command.

### WhatsApp Web delivery

Set `WHATSAPP_RECIPIENT` to an Indonesian local number such as
`081234567890`, or an international number such as `6281234567890`. When this
variable is empty, the application remains Telegram-only and does not open a
WhatsApp session.

After the first deployment with a recipient configured, follow the application
logs:

```sh
kubectl logs -n default -f deploy/ai-assistant -c picoclaw
```

Scan the rendered QR in WhatsApp under **Perangkat tertaut → Tautkan
perangkat**. Treat the QR as a temporary credential and do not share the logs
while it is visible. The linked-device session is stored at
`/root/.picoclaw/whatsapp.db` on the existing PVC, so ordinary pod restarts do
not require another scan. If pairing times out, restart the pod to print a new
QR. To unlink or re-pair, remove the linked device in WhatsApp, remove the
session database from the PVC, and restart the pod.

Both `/loker` and the 03:00 scheduled alert send independently to Telegram and
WhatsApp. A failure in one channel does not prevent the other channel from
being attempted. WhatsApp Web automation is unofficial and may be disconnected
or restricted by WhatsApp; Telegram remains the primary channel. Remove
`WHATSAPP_RECIPIENT` to disable WhatsApp delivery.

### Scheduled email delivery

The 03:00 scheduled job alert can also send the same result by SMTP. Interactive
`/loker` searches do not send email. Configure these values in `.env`:

```dotenv
MAIL_MAILER=smtp
MAIL_USERNAME=your_email@gmail.com
MAIL_PASSWORD=your_gmail_app_password
MAIL_HOST=smtp.gmail.com
MAIL_PORT=465
MAIL_ENCRYPTION=ssl
MAIL_FROM=your_email@gmail.com
MAIL_TO=recipient@example.com
```

For Gmail, enable two-step verification and use an App Password rather than the
account password. Port 465 uses implicit TLS, and the deployment network policy
allows that port without permitting unencrypted SMTP fallback. Email is enabled
only when `MAIL_TO` is set; remove it to roll back to Telegram and optional
WhatsApp delivery. Each channel is attempted independently, and failed SMTP
sends are not retried automatically to avoid duplicate email.

### Data sources

| Source    | Method            | Endpoint                         |
| --------- | ----------------- | -------------------------------- |
| Kitalulus | SSR HTML scraping | `https://kitalulus.com/lowongan` |
| Dealls    | Next.js SSR data  | `https://dealls.com/loker`       |

Dealls exposes structured job data in its server-rendered Next.js pages. Kitalulus listings are parsed from server-rendered job cards.

### Filtering

1. **Location** — substring match against Jabodetabek, Bandung, Surabaya, Bali, Batam, Solo, Salatiga, Karawang, Cikampek, Cikarang.
2. **Experience** — `maxYearsExp` bound (default 3 years); jobs with `minYearsOfExperience > 3` are dropped.
3. **Position and tech stack** — deterministic text matching against titles and available skill metadata. Listings without skill metadata are retained when their title matches.
4. **Optional company assessment** — when `halal` is specified, the configured AI provider classifies unique companies after job filtering. The daily cron enables this automatically. This is an informational AI assessment, not a religious ruling.

Max 20 results per source.

### How `/loker` works

```
Telegram /loker → PicoClaw job-search skill → Fiber POST :8081/loker
                  → jobsearch application service
                  → fetch Kitalulus + Dealls → filter → optional halal assessment
                  → Telegram + optional WhatsApp Web
```

### Architecture

| Runtime | Port | Role |
| --- | --- | --- |
| picoclaw | 18790 | AI agent gateway and Telegram bot |
| `cmd/app` | 8080 | Finance ledger, Google Sheets sync, and Sumopod Responses proxy |
| `cmd/app` | 8081 | Compatible asynchronous `POST /loker` endpoint |
| `job-alert-scheduler` | — | Background scheduler for the 03:00 daily alert |
| `cmd/job-alert` | — | One-shot fetch, filter, halal assessment, and scheduled multi-channel delivery |

### Binaries and scripts

| Path | Purpose |
| --- | --- |
| `cmd/app` | Fiber composition root and process lifecycle |
| `cmd/job-alert` | One-shot job-alert CLI |
| `internal/domains/finance` | Finance domain, use cases, SQLite/Sheets adapters, HTTP delivery |
| `internal/domains/jobsearch` | Job-search domain, orchestration, provider adapters, HTTP delivery |
| `internal/domains/ai` | Sumopod Responses compatibility adapter and HTTP delivery |
| `scripts/runtime` | Container wrappers and daily scheduler |

Repository engineering workflow, traceability, and validation are documented in [`docs/HARNESS.md`](docs/HARNESS.md).
