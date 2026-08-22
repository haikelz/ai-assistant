---
name: job-search
description: "Cari lowongan dan atur pencarian harian dari Kitalulus dan Dealls. Command: /loker /cari /cari-kerja /lowongan /job-alert."
---

# /loker — Cari Lowongan Kerja

Untuk `/loker <teks>`, jalankan:

```sh
curl -fsS -X POST http://127.0.0.1:8081/loker -H 'Content-Type: application/json' -d '{"query":"<teks>"}'
```

Format opsional: `<posisi> | <skills> | <pengalaman> | <lokasi> | halal`. Field `halal` mengaktifkan penilaian model bisnis perusahaan oleh AI. Tanpa field tersebut, penilaian tidak dijalankan.

Lalu balas: "Mencari lowongan di Kitalulus dan Dealls. Hasil akan dikirim ke chat kamu."

JANGAN gunakan web search. HANYA jalankan curl di atas.

# /job-alert — Atur Pencarian Harian

Untuk `/job-alert <teks>`, simpan kriteria pencarian harian dengan:

```sh
curl -fsS -X POST http://127.0.0.1:8081/job-alert -H 'Content-Type: application/json' -d '{"query":"<teks>"}'
```

Formatnya sama: `<posisi> | <skills> | <pengalaman> | <lokasi>`. Posisi wajib
diisi. Job alert berjalan setiap hari pukul 03:00 WIB dan labeling halal selalu
aktif. Perintah ini hanya memperbarui konfigurasi; jangan jalankan pencarian
langsung.

Untuk `/job-alert` tanpa teks, tampilkan konfigurasi aktif dengan:

```sh
curl -fsS http://127.0.0.1:8081/job-alert
```

Balas berdasarkan field `message` dan `config` dari response. JANGAN gunakan web
search.
