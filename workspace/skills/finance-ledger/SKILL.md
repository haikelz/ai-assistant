---
name: finance-ledger
description: "Record and report the user's personal finance ledger through the local AI Assistant finance API. Use for /expense, /income, /modal, /totalexpenses, /totalincomes, /totalmoney, /financerecap, and /downloadrecap."
---

# Finance Ledger

The finance API is local to this AI Assistant deployment: `http://127.0.0.1:8080`.

Only accept these categories:

- `/expense`: `Investasi`, `Sumbangan`, `Makan/Minum`, or `Lain - Lain`.
- `/income`: always use `Pendapatan`.
- `/modal`: always use `Modal`. Use this after the monthly paycheck reminder or when the user explicitly records capital.

For `/expense <amount> <category> <description>`, `/income <amount> <description>`, or `/modal <amount> <description>`, parse an amount in rupiah and call:

```sh
curl -fsS -X POST http://127.0.0.1:8080/records \
  -H 'Content-Type: application/json' \
  --data '{"phone":"'$TELEGRAM_USER_ID'","type":"TYPE","amount":AMOUNT,"category":"CATEGORY","description":"DESCRIPTION"}'
```

Use `$TELEGRAM_USER_ID` as `phone`. Ask one short clarification if the amount, allowed category, or description is missing. Do not call the API until the user clarifies.

For `/totalexpenses`, `/totalincomes`, or `/totalmoney`, call:

```sh
curl -fsS "http://127.0.0.1:8080/totals?phone=$TELEGRAM_USER_ID"
```

Report only the requested total. `money` is `(modal + income) - expense`.

For `/financerecap`, call the same totals endpoint and present modal, income, expense, and money in a compact Indonesian recap.

For `/downloadrecap`, download the file into the workspace, then call `send_file` for that exact path:

```sh
curl -fsS "http://127.0.0.1:8080/recap.xlsx?phone=$TELEGRAM_USER_ID" -o /root/.picoclaw/workspace/finance-recap.xlsx
```

On the 28th monthly cron event, ask the user for their payday amount. When they reply with a clear amount, record it as `/modal` using the description `Gaji <month> <year>` unless they provide a different description.
