# AI Assistant

PicoClaw Telegram assistant using Sumopod's Responses API with `deepseek-v4-pro` and its own local finance API.

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
