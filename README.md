# Platform Crowdfunding dan Donasi — Kelompok E

Platform crowdfunding dan donasi berbasis layanan terpisah, API Gateway, PostgreSQL, dan RabbitMQ.

## Stack dan arsitektur
- Frontend: Vite + React, disajikan Nginx pada `:5173`; Nginx meneruskan API dan WebSocket ke Gateway
- API Gateway: Go (`:8000`), reverse proxy, service registry, health check, round-robin, OAuth2 client credentials, dan mTLS ke backend
- Campaign Service: Go + GraphQL (`:8443` internal TLS), autentikasi, campaign/media, pembayaran donasi Midtrans Snap Sandbox, webhook pembayaran, dan transactional outbox
- Notification Service: Go + WebSocket (`:8443` internal TLS), menerima event dari RabbitMQ
- Data: PostgreSQL 16 dan RabbitMQ

Gateway mendaftarkan dua instance Campaign Service (`campaign-service` dan `campaign-service-2`). Health check berjalan berkala dan round-robin hanya memilih instance yang sehat. Ini menjadi demonstrasi service discovery dan load balancing Sesi 10.

## Menjalankan dengan Docker Compose
1. Install Docker Desktop dan OpenSSL.
2. Salin `.env.example` menjadi `.env`, lalu ganti placeholder dengan secret acak. Gunakan secret minimal 32 karakter; gunakan karakter alfanumerik untuk password DB dan RabbitMQ.
3. Buat sertifikat development untuk koneksi mTLS:

```powershell
.\scripts\generate-dev-certs.ps1
```

4. Jalankan:

```bash
docker compose up --build
```

5. Buka frontend `http://localhost:5173`. Nginx meneruskan request API ke Gateway, jadi browser tidak perlu mengakses alamat internal service.
6. Health Gateway tersedia di `http://localhost:8000/health` dan registry di `http://localhost:8000/registry`.

Secara default, aplikasi memakai PostgreSQL lokal dari Docker Compose. Agar data tersimpan di Neon, salin connection string PostgreSQL Neon ke `.env` sebagai `DATABASE_URL` (gunakan `sslmode=require`), lalu buat ulang service:

```powershell
docker compose up -d --build --force-recreate campaign-service campaign-service-2
```

`.env.example` mencantumkan format URL tanpa kredensial. Jangan kirim atau commit connection string yang berisi password. Jika tabel di Neon belum tersedia, jalankan `db/init.sql` sekali melalui SQL Editor Neon; skrip memakai `IF NOT EXISTS` dan tidak menghapus data.

Untuk menguji pembayaran, buat akun Midtrans Sandbox dan isi `MIDTRANS_SERVER_KEY` pada `.env` dengan Server Key Sandbox. Key hanya digunakan Campaign Service; jangan commit atau bagikan key. Aktifkan izin **Cancel transaction: Write** agar pembatalan checkout dapat diteruskan ke Midtrans. Checkout akan mengarahkan donatur ke Snap Sandbox dan tidak memotong uang sungguhan. Jika checkout ditinggalkan saat belum selesai, pemilik donasi dapat melanjutkan atau membatalkannya dari detail campaign atau halaman donasi. Detail endpoint ada di [API.md](./API.md).

## Deploy ke Railway

Source code tetap di satu repository, tetapi Railway menjalankan komponen sebagai service terpisah dalam satu project. Buat satu service untuk masing-masing folder aplikasi di tabel berikut. Mulai dengan satu Campaign Service; replika kedua dari Docker Compose tidak wajib untuk deployment awal.

| Service Railway | Root Directory | Tipe / akses | Port aplikasi |
|---|---|---|---:|
| `frontend` | `/frontend` | Public Web Service | Railway `PORT` |
| `gateway` | `/gateway` | Private Service | `8000` |
| `campaign-service` | `/campaign-service` | Private Service | `8443` |
| `notification-service` | `/notification-service` | Private Service | `8443` |
| `Postgres` | — | Tambahkan plugin PostgreSQL Railway | internal |
| `rabbitmq` | — | Private service dari image `rabbitmq:3-management-alpine` | `5672` internal |

Semua service harus berada di project dan environment Railway yang sama. Pada service Docker dari repository, atur **Root Directory** sesuai tabel agar Dockerfile di folder tersebut digunakan. Buat domain publik hanya untuk `frontend`; gateway dan kedua backend tetap private. Railway private networking menggunakan hostname internal dan koneksi antar-service tanpa membuka port backend ke internet.

### Urutan setup

1. Buat project Railway dan hubungkan repository GitHub ini. Periksa billing dan batas penggunaan sebelum deploy; PostgreSQL, RabbitMQ, dan service aplikasi merupakan resource terpisah.
2. Tambahkan PostgreSQL Railway, lalu jalankan isi [`db/init.sql`](./db/init.sql) sekali terhadap database yang baru dibuat. Jangan jalankan skrip ini pada database berisi data tanpa backup.
3. Tambahkan `rabbitmq` dari image `rabbitmq:3-management-alpine`. Set `RABBITMQ_DEFAULT_USER` dan `RABBITMQ_DEFAULT_PASS` memakai kredensial acak alfanumerik, lalu pasang volume ke `/var/lib/rabbitmq`. Jangan buat domain publik atau TCP proxy untuk RabbitMQ.
4. Buat tiga private service dari repo dengan root directory `/campaign-service`, `/notification-service`, dan `/gateway`. Set variabel sesuai tabel di bawah.
5. Buat service `/frontend`, atur `GATEWAY_HOST` seperti tabel, lalu aktifkan public domain. Nginx membaca `PORT` dan `GATEWAY_HOST` saat container mulai, lalu meneruskan API dan WebSocket melalui jaringan private.
6. Tunggu semua deployment sehat. Buat atau aktifkan akun Admin melalui SQL Railway hanya jika diperlukan; jangan membuka SQL admin ke internet.
7. Untuk Midtrans, isi callback/notification URL Sandbox dengan `https://<domain-frontend>/webhook/midtrans`. Gunakan Server Key Sandbox terlebih dahulu; jangan aktifkan transaksi uang sungguhan sebelum konfigurasi produksi dan pengujian selesai.

### Variabel Railway

Buat variabel rahasia bersama (shared variables) `JWT_SECRET` dan `OAUTH_CLIENT_SECRET` dengan nilai acak minimal 32 karakter; gunakan nilai yang sama pada service terkait. Jangan menyalin `.env` lokal ke repository atau menaruh secret di file konfigurasi.

| Service | Variabel |
|---|---|
| `campaign-service` | `PORT=8443`, `MTLS_ENABLED=false`, `DATABASE_URL=${{Postgres.DATABASE_URL}}`, `JWT_SECRET=${{shared.JWT_SECRET}}`, `PAYMENT_WEBHOOK_SECRET=<secret acak minimal 32 karakter>`, `OAUTH_CLIENT_ID=crowdfunding-gateway`, `OAUTH_CLIENT_SECRET=${{shared.OAUTH_CLIENT_SECRET}}`, `FRONTEND_URL=https://${{frontend.RAILWAY_PUBLIC_DOMAIN}}`, `RABBITMQ_URL=amqp://${{shared.RABBITMQ_USER}}:${{shared.RABBITMQ_PASSWORD}}@${{rabbitmq.RAILWAY_PRIVATE_DOMAIN}}:5672/`, `MIDTRANS_SERVER_KEY=<Server Key Sandbox milikmu>` |
| `notification-service` | `PORT=8443`, `MTLS_ENABLED=false`, `JWT_SECRET=${{shared.JWT_SECRET}}`, `OAUTH_CLIENT_ID=crowdfunding-gateway`, `RABBITMQ_URL` sama seperti di atas, `CORS_ALLOWED_ORIGINS=https://${{frontend.RAILWAY_PUBLIC_DOMAIN}}` |
| `gateway` | `PORT=8000`, `MTLS_ENABLED=false`, `OAUTH_CLIENT_ID=crowdfunding-gateway`, `OAUTH_CLIENT_SECRET=${{shared.OAUTH_CLIENT_SECRET}}`, `CAMPAIGN_SERVICES=http://${{campaign-service.RAILWAY_PRIVATE_DOMAIN}}:8443`, `NOTIFICATION_SERVICE=http://${{notification-service.RAILWAY_PRIVATE_DOMAIN}}:8443`, `CORS_ALLOWED_ORIGINS=https://${{frontend.RAILWAY_PUBLIC_DOMAIN}}` |
| `frontend` | `GATEWAY_HOST=${{gateway.RAILWAY_PRIVATE_DOMAIN}}:8000`; Railway menyediakan `PORT` untuk HTTP service |
| `rabbitmq` | `RABBITMQ_DEFAULT_USER=${{shared.RABBITMQ_USER}}`, `RABBITMQ_DEFAULT_PASS=${{shared.RABBITMQ_PASSWORD}}` |

Tambahkan juga `RABBITMQ_USER` dan `RABBITMQ_PASSWORD` sebagai shared variables acak alfanumerik, dan referensikan nama service secara persis seperti di tabel. `MTLS_ENABLED=false` digunakan karena koneksi antar-service Railway berjalan melalui private networking; autentikasi service OAuth tetap aktif. Jangan gunakan kredensial, URL database, atau server key yang sama dengan contoh lokal.

Dokumentasi Railway: [Deploying a Monorepo](https://docs.railway.com/deployments/monorepo), [Private Networking](https://docs.railway.com/networking/private-networking), dan [Variables](https://docs.railway.com/variables/reference).

Untuk menghentikan container tanpa menghapus data database:

```bash
docker compose down
```

Data PostgreSQL tetap tersimpan di volume. Untuk menghapus seluruh data demo: `docker compose down -v`.

## Akun dan role

Registrasi publik selalu membuat akun dengan role `DONATUR`. Sesuai aturan proyek saat ini, semua pengguna yang sudah login dapat mengajukan campaign. Campaign baru berstatus `MENUNGGU` dan belum muncul pada daftar publik sampai disetujui `ADMIN`. Mutasi `approveCampaign` dan query `pendingCampaigns` hanya dapat digunakan oleh `ADMIN`.

Untuk demo lokal, role akun dapat diubah oleh administrator database. Contoh membuat akun yang sudah terdaftar menjadi ADMIN:

```bash
docker compose exec db psql -U crowdfunding -d crowdfunding -c "UPDATE users SET role='ADMIN' WHERE email='admin@example.com';"
```

Jangan membuka akses SQL admin kepada pengguna aplikasi. Mutasi `createCampaign` memerlukan login; mutasi `approveCampaign` dan daftar `pendingCampaigns` hanya tersedia untuk `ADMIN`.

## GraphQL
Endpoint: `POST /graphql` melalui frontend/Gateway.

Query campaign publik (aktif dan yang sudah mencapai target):

```graphql
query {
  campaigns { id title description target collected status }
}
```

Membuat campaign (semua pengguna yang sudah login):

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
- JWT untuk autentikasi, masa berlaku 12 jam, algoritma HS256; frontend otomatis logout saat token kedaluwarsa.
- Role-based authorization diperiksa di backend.
- Registrasi publik tidak dapat menaikkan role sendiri.
- Validasi panjang input, nominal positif, dan batas ukuran request.
- GraphQL membatasi kedalaman sederhana melalui jumlah kurung kurawal; ini proteksi awal, bukan pengganti query-cost analyzer.
- Query SQL menggunakan parameter `$1`, `$2`, dan seterusnya.
- Kredensial dan secret dibaca dari environment variable.
- Webhook Midtrans Sandbox memverifikasi signature provider dan mengonfirmasi status langsung ke API Midtrans; status pembayaran tidak dipercaya dari frontend.
- Header keamanan dasar dan pesan error yang tidak membocorkan detail database.
- Campaign yang sudah mencapai target otomatis berstatus `SELESAI`, tidak muncul di daftar campaign aktif, dan tetap tersedia untuk dibuka pada bagian campaign selesai. Data pribadi donatur tidak ditampilkan.
- Campaign baru dan donasi yang berhasil dibayar menulis event ke transactional outbox pada transaksi database yang sama.
- Berkas campaign aktif atau selesai dilayani melalui Gateway; file upload dibatasi berdasarkan jumlah, tipe, dan ukuran.
- API dan WebSocket frontend menggunakan origin frontend yang sama saat dijalankan dengan Docker Compose.

Webhook dan donasi masih berupa simulasi; belum terhubung ke payment gateway sungguhan. Sertifikat yang dibuat oleh script hanya untuk development lokal.

## Catatan pengembangan
Kontrak lengkap endpoint, validasi, serta bentuk response tersedia di [API.md](./API.md). Untuk produksi, gunakan manajemen secret, sertifikat/CA tepercaya, AMQPS, rate limiting, audit log, validasi webhook sesuai dokumentasi payment gateway, dan analisis kompleksitas GraphQL.
