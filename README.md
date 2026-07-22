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

The assistant only accepts the configured Telegram user. Its state, finance database, and cron jobs persist in the `ai-assistant-data` PVC. This ledger is independent from `ryuko-matoi-go`.

The paycheck reminder runs at 09:00 Asia/Jakarta every 28th. `/downloadrecap` sends an XLSX file that opens directly in Google Sheets.
