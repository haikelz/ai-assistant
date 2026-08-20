---
name: job-search
description: "Cari lowongan kerja dari Kitalulus dan Dealls. Command: /loker /cari /cari-kerja /lowongan."
---

# /loker — Cari Lowongan Kerja

Untuk `/loker <teks>`, jalankan:

```sh
curl -fsS -X POST http://127.0.0.1:8081/loker -H 'Content-Type: application/json' -d '{"query":"<teks>"}'
```

Format opsional: `<posisi> | <skills> | <pengalaman> | <lokasi> | halal`. Field `halal` mengaktifkan penilaian model bisnis perusahaan oleh AI. Tanpa field tersebut, penilaian tidak dijalankan.

Lalu balas: "Mencari lowongan di Kitalulus dan Dealls. Hasil akan dikirim ke chat kamu."

JANGAN gunakan web search. HANYA jalankan curl di atas.
