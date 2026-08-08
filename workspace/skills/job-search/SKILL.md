---
name: job-search
description: "Cari lowongan kerja Glints & Jobstreet. Command: /loker /cari /cari-kerja /lowongan."
---

# /loker — Cari Lowongan Kerja (Glints + Jobstreet)

Untuk `/loker <teks>`, jalankan:

```sh
curl -fsS -X POST http://127.0.0.1:8081/loker -H 'Content-Type: application/json' -d '{"query":"<teks>"}'
```

Lalu balas: "Mencari lowongan di Glints & Jobstreet, hasil akan dikirim ke chat kamu."

JANGAN gunakan web search. JANGAN sebut LinkedIn. HANYA jalankan curl di atas.
