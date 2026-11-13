# Panduan Menerapkan Paket Code Freeze Week 12

Paket ZIP ini hanya berisi file yang perlu diganti atau ditambahkan. Ekstrak isi ZIP ke folder utama repository `uas_paknanang-main` dan izinkan file dengan nama yang sama untuk ditimpa.

## File yang diganti

- `README.md`
- `readme2.md`
- `docker-compose.yml`
- `db/init.sql`
- `campaign-service/Dockerfile`
- `campaign-service/go.mod`
- `campaign-service/go.sum`
- `campaign-service/main.go`
- `gateway/Dockerfile`
- `gateway/go.mod`
- `gateway/main.go`
- `notification-service/Dockerfile`
- `notification-service/go.mod`
- `notification-service/go.sum`
- `notification-service/main.go`

## File baru

- `.env.example`
- `.gitignore`
- `API.md`
- `PANDUAN_PENERAPAN.md`
- `campaign-service/outbox.go`
- `gateway/go.sum` (file kosong untuk build Docker)
- `gateway/main_test.go`
- `frontend/.dockerignore` (hanya aturan konteks Docker; source frontend tidak diubah)
- `scripts/generate-dev-certs.ps1`
- `scripts/generate-dev-certs.sh`

File `.env` sengaja tidak disertakan karena berisi kredensial lokal. Jangan menggantinya dengan file dari ZIP lama.

## Urutan setelah menyalin file

1. Pastikan file `.env` lama tidak dipakai. Salin `.env.example` menjadi `.env` dengan `-Force` pada PowerShell, lalu isi seluruh secret baru.
2. Jika pernah menjalankan Compose lama dan masih memiliki volume `pgdata`, ikuti langkah migrasi password PostgreSQL pada bagian konfigurasi environment di `README.md`.
3. Buat sertifikat development dengan `scripts/generate-dev-certs.ps1` (PowerShell) atau `scripts/generate-dev-certs.sh` (Linux/macOS/Git Bash).
4. Jalankan `docker compose up --build -d` dari folder utama repository.
5. Periksa `docker compose ps`, `http://localhost:8000/health`, dan `http://localhost:8000/registry`.
6. Baca `README.md` dan `API.md` untuk akun demo, API, serta pengujian.

Jangan menjalankan `docker compose down -v` jika ingin mempertahankan data database demo.
