# Platform Crowdfunding dan Donasi — Kelompok E

Implementasi bertahap sampai **Sesi 10** berdasarkan Rancangan Keamanan Awal Kelompok E.

## Stack dan arsitektur
- Frontend: Vite + React
- API Gateway: Go (`:8000`), reverse proxy, service registry, health check, round-robin
- Campaign Service: Go + GraphQL (`:8001`), autentikasi, otorisasi, campaign, webhook pembayaran
- Notification Service: Go + WebSocket (`:8002`), broadcast aktivitas real-time
- Database: PostgreSQL

Gateway mendaftarkan dua instance Campaign Service (`campaign-service` dan `campaign-service-2`). Health check berjalan berkala dan round-robin hanya memilih instance yang sehat. Ini menjadi demonstrasi service discovery dan load balancing Sesi 10.

## Menjalankan dengan Docker Compose
1. Install Docker Desktop.
2. Salin `.env.example` menjadi `.env`.
3. Ganti semua secret di `.env` dengan nilai acak yang kuat.
4. Jalankan:

```bash
docker compose up --build
```

5. Buka frontend `http://localhost:5173` dan gateway `http://localhost:8000/health`.
6. Status registry dapat dilihat di `http://localhost:8000/registry`.

Untuk menghentikan:

```bash
docker compose down
```

Data PostgreSQL tetap tersimpan di volume. Untuk menghapus seluruh data demo: `docker compose down -v`.

## Akun dan role
Registrasi publik hanya membuat role `DONATUR`. Pengguna tidak dapat memilih role `PENGELOLA` atau `ADMIN` sendiri. Untuk demo lokal, setelah mendaftar, ubah role akun melalui PostgreSQL oleh administrator:

```sql
UPDATE users SET role = 'PENGELOLA' WHERE email = 'email-pengelola@example.com';
UPDATE users SET role = 'ADMIN' WHERE email = 'email-admin@example.com';
```

Jangan membuka akses SQL admin kepada pengguna aplikasi. Mutasi `createCampaign` hanya untuk `PENGELOLA`/`ADMIN`; `approveCampaign` hanya untuk `ADMIN`.

## GraphQL
Endpoint: `POST http://localhost:8000/graphql`

Query campaign aktif:

```graphql
query {
  campaigns { id title description target collected status }
}
```

Membuat campaign (Bearer token role PENGELOLA/ADMIN):

```graphql
mutation {
  createCampaign(title: "Bantuan Banjir", description: "Bantuan warga", target: 25000000) {
    id title status
  }
}
```

Menyetujui campaign (Bearer token role ADMIN):

```graphql
mutation { approveCampaign(id: 1) { id title status } }
```

## Keamanan yang diimplementasikan
- Password disimpan sebagai bcrypt hash.
- JWT untuk autentikasi, masa berlaku 12 jam, algoritma HS256.
- Role-based authorization diperiksa di backend.
- Registrasi publik tidak dapat menaikkan role sendiri.
- Validasi panjang input, nominal positif, dan batas ukuran request.
- GraphQL membatasi kedalaman sederhana melalui jumlah kurung kurawal; ini proteksi awal, bukan pengganti query-cost analyzer.
- Query SQL menggunakan parameter `$1`, `$2`, dan seterusnya.
- Kredensial dan secret dibaca dari environment variable.
- Webhook pembayaran memverifikasi HMAC SHA-256 melalui header `X-Signature`; bukan boolean dari frontend.
- Header keamanan dasar dan pesan error yang tidak membocorkan detail database.
- Donatur publik hanya melihat campaign aktif; data pribadi tidak ditampilkan dalam daftar campaign.

## Batas progres (sampai Sesi 10)
Belum termasuk message queue/event broker (Sesi 11), code freeze/deployment final (Sesi 12), dan rangkaian pengujian formal (Sesi 13–14). Webhook masih berupa integrasi simulasi; belum terhubung ke payment gateway sungguhan. HTTPS/TLS perlu diaktifkan saat deployment melalui reverse proxy dengan sertifikat.

## Catatan pengembangan
Konfigurasi Docker Compose disediakan sebagai lingkungan demonstrasi. Untuk produksi, tambahkan manajemen secret, TLS, rate limiting terdistribusi, audit log persisten, validasi webhook sesuai dokumentasi payment gateway yang dipakai, dan analisis kompleksitas GraphQL.
