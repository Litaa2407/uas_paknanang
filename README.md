# Platform Crowdfunding dan Donasi — Kelompok E

Implementasi bertahap sampai **Sesi 10** berdasarkan Rancangan Keamanan Awal Kelompok E.

## Stack dan arsitektur
- Frontend: Vite + React
- API Gateway: Go (`:8000`), reverse proxy, service registry, health check, round-robin
- Campaign Service: Go + GraphQL (`:8001`), autentikasi, otorisasi, campaign, webhook pembayaran
- Notification Service: Go + WebSocket (`:8002`), broadcast aktivitas real-time
- Database: PostgreSQL

Ini adalah aplikasi berbasis beberapa service, bukan satu backend monolitik. Gateway mendaftarkan dua instance Campaign Service (`campaign-service` dan `campaign-service-2`). Health check berjalan berkala dan round-robin hanya memilih instance yang sehat. Ini menjadi demonstrasi service discovery dan load balancing Sesi 10.

## Menjalankan dengan Docker Compose
1. Install Docker Desktop.
2. Salin `.env.example` menjadi `.env`.
3. Dari dashboard Neon, ambil pooled connection string untuk database yang akan digunakan, lalu isi `DATABASE_URL` pada `.env`. Pastikan URL memakai `sslmode=require`. Ganti juga `JWT_SECRET` dan `PAYMENT_WEBHOOK_SECRET` dengan secret acak milikmu; `JWT_SECRET` harus sekurangnya 32 karakter.

```env
DATABASE_URL=postgresql://USER:PASSWORD@YOUR-NEON-HOST/DBNAME?sslmode=require
JWT_SECRET=secret-acak-minimal-32-karakter
PAYMENT_WEBHOOK_SECRET=secret-webhook-acak
```

Jika password Neon memuat karakter khusus URL (misalnya `@`, `:`, `/`, atau `#`), URL-encode karakter tersebut sebelum memasukkannya ke connection string. Jangan bagikan atau commit file `.env`; file tersebut sudah dikecualikan Git.
4. Buka **SQL Editor** pada project Neon dan jalankan isi [`db/init.sql`](./db/init.sql) untuk membuat tabel aplikasi. Jika project Neon sudah berisi tabel users/campaigns/donations, jalankan migration yang belum diterapkan secara berurutan: [`db/migrations/002_campaign_media.sql`](./db/migrations/002_campaign_media.sql), lalu [`db/migrations/003_donation_contact.sql`](./db/migrations/003_donation_contact.sql). Migration 003 menambahkan email dan telepon donatur ke tabel donasi; jangan menjalankan ulang script setup atau drop table.
5. Jalankan:

```bash
docker compose up --build
```

6. Buka frontend `http://localhost:5173` dan gateway `http://localhost:8000/health`.
7. Status registry dapat dilihat di `http://localhost:8000/registry`.

Kedua instance Campaign Service memakai `DATABASE_URL` yang sama, sehingga campaign dan akun disimpan ke Neon. Data yang sebelumnya tersimpan pada PostgreSQL lokal Docker **tidak otomatis dipindahkan**; campaign lokal harus dimasukkan kembali atau dimigrasikan secara terpisah. Menghentikan Compose dengan `docker compose down` tidak menghapus data Neon.

Untuk menghentikan:

```bash
docker compose down
```

Data tetap tersimpan pada Neon. Untuk menghapus data, gunakan prosedur yang disengaja pada Neon; jangan menjalankan reset/drop terhadap database yang berisi data penting.

## Akun dan role
Registrasi publik membuat role `DONATUR`. Semua pengguna yang sudah login dapat mengajukan campaign. Campaign berstatus `MENUNGGU` dan tidak tampil dalam daftar publik sampai disetujui oleh `ADMIN`. `ADMIN` dapat meninjau dan menyetujui pengajuan dari bagian **Persetujuan Campaign** di aplikasi. Pengguna tidak dapat memilih role `PENGELOLA` atau `ADMIN` sendiri. Untuk demo lokal, role dapat diubah melalui PostgreSQL oleh administrator:

```sql
UPDATE users SET role = 'PENGELOLA' WHERE email = 'email-pengelola@example.com';
UPDATE users SET role = 'ADMIN' WHERE email = 'email-admin@example.com';
```

Jangan membuka akses SQL admin kepada pengguna aplikasi. Mutasi `createCampaign` memerlukan login; mutasi `approveCampaign` dan daftar `pendingCampaigns` hanya tersedia untuk `ADMIN`.

## GraphQL
Endpoint: `POST http://localhost:8000/graphql`

Query campaign aktif:

```graphql
query {
  campaigns { id title description target collected status }
}
```

Membuat campaign (memerlukan login; semua role terautentikasi saat ini dapat mengajukan):

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
- Request GraphQL ditolak bila memiliki lebih dari 12 kurung kurawal pembuka; ini pembatasan sederhana, bukan analisis kedalaman atau biaya query.
- Query SQL menggunakan parameter `$1`, `$2`, dan seterusnya.
- Kredensial dan secret dibaca dari environment variable.
- Webhook simulasi memverifikasi HMAC SHA-256 melalui header `X-Signature`; bukan status yang dikirim dari frontend.
- Header keamanan dasar dipasang pada Campaign Service.
- Donatur hanya dapat mengirim data donasinya melalui endpoint terautentikasi. Nama, email, dan nomor telepon disimpan pada tabel `donations`, tetapi tidak disertakan dalam aktivitas publik.

## Status implementasi sebagai acuan Test Plan

Bagian ini merangkum apa yang benar-benar tersedia di source code saat ini. Gunakan status berikut untuk memperbarui jawaban di dokumen Test Plan; kebutuhan yang belum diimplementasikan tetap boleh dicatat sebagai rencana atau pengujian masa depan, bukan sebagai fitur yang sudah tersedia.

| Area | Sudah tersedia | Belum tersedia / batasan saat ini |
|---|---|---|
| Akun dan autentikasi | Registrasi dan login melalui Gateway. Password memakai bcrypt; token memakai JWT HS256 dengan masa berlaku 12 jam. Registrasi publik selalu mendapat role `DONATUR`. | Belum ada lupa/reset password atau pembatasan percobaan login. |
| Campaign | Daftar campaign aktif melalui GraphQL; kartu campaign dapat dibuka ke halaman detail berisi cerita, media, target, dan progres dana. Pengguna login dapat mengajukan campaign berisi judul, deskripsi, target, minimal 1 dan maksimal 5 foto, serta opsional 1 video MP4. Donatur memilih nominal dan mengisi nama, telepon, serta email pada halaman checkout simulasi terpisah. File media disimpan sebagai `BYTEA` di tabel `campaign_media` pada Neon dalam transaksi yang sama dengan campaign. | Belum ada edit/delete atau pembayaran sungguhan. Upload maksimum 5 MB per foto dan 25 MB untuk video; total request maksimum sekitar 50 MB. Foto JPG/PNG/WebP dan video MP4. |
| Peran | Query `pendingCampaigns` dan mutasi `approveCampaign` memerlukan role `ADMIN`. `createCampaign` memerlukan login, tetapi saat ini tidak membatasi pengaju ke role `PENGELOLA`/`ADMIN`. | Role istimewa tidak dapat dipilih saat registrasi publik; pengaturan role untuk demo dilakukan langsung oleh administrator database. |
| Pembayaran | Donatur yang login dapat membuka halaman checkout dari detail campaign, memilih nominal, dan mengisi nama, email, serta nomor telepon. `POST /donations/simulate` hanya menerima role `DONATUR`, memvalidasi informasi tersebut dan campaign aktif, lalu menyimpan donasi serta memperbarui progres secara transaksional. Nama, email, dan telepon disimpan pada tabel `donations`; identitas tidak dikirim ke aktivitas publik. Tersedia juga `POST /webhook/payment` untuk payload ber-signature HMAC-SHA256; hanya `SETTLEMENT` yang menambah nilai `collected`. | Semua donasi dari UI adalah simulasi dan bukan uang sungguhan. Belum ada checkout/provider pembayaran, sandbox provider, maupun transaksi uang sungguhan. |
| Notifikasi real-time | WebSocket tersedia di `/ws/donasi`. Campaign Service mengirim event `CAMPAIGN_CREATED`, `CAMPAIGN_APPROVED`, `NEW_DONATION`, dan `SIMULATED_DONATION`; frontend menampilkan aktivitas yang diterima. Identitas donatur checkout tidak ikut dibroadcast. | Broadcast dikirim ke semua koneksi, tanpa autentikasi, filter campaign, penyimpanan/replay event, atau reconnect otomatis di frontend. Event tidak dengan sendirinya memperbarui daftar campaign yang sedang terbuka. |
| Gateway dan service | Gateway merutekan `/auth/`, `/graphql`, `/webhook/`, dan `/ws/`; menyediakan `/health` dan `/registry`; dan memeriksa kesehatan dua instance Campaign Service. | Belum ada broker/message queue, HTTPS/TLS bawaan, atau pengujian formal yang terdokumentasi di repository. |

### Alur aplikasi yang dapat diuji sekarang

1. Pengguna mendaftar atau login, lalu menerima JWT.
2. Pengguna yang login mengirim pengajuan campaign. Campaign tersimpan sebagai `MENUNGGU` dan belum muncul pada daftar campaign publik.
3. Admin meninjau dan menyetujui pengajuan. Campaign berubah menjadi `AKTIF` dan dapat muncul pada daftar publik.
4. Campaign Service mengirim event perubahan ke Notification Service untuk dibroadcast melalui WebSocket.
5. Donatur membuka halaman checkout dari detail campaign, mengisi nama, telepon, email, dan nominal untuk mencatat dukungan simulasi. Progres bertambah, tetapi nominal tersebut bukan transaksi uang sungguhan. Untuk database lama, terapkan migration `003_donation_contact.sql` sebelum memakai checkout. Untuk menguji webhook provider secara terpisah, penguji mengirim payload dengan signature yang valid; status `SETTLEMENT` menambah total terkumpul, sedangkan `PENDING` dan `FAILED` tidak.

### Penyesuaian cakupan dokumen Test Plan

- **Test items yang sudah ada:** autentikasi/role, query dan mutasi campaign, approval admin, validasi webhook simulasi, serta broadcast event WebSocket.
- **Skenario webhook yang sesuai:** signature tidak valid ditolak; payload invalid ditolak; referensi duplikat ditolak; `PENDING`/`FAILED` tidak menambah total; `SETTLEMENT` yang valid menambah total dan mengirim event. Uji memakai secret dan data lokal/sandbox milik tim.
- **Media campaign:** jalankan migration `db/migrations/002_campaign_media.sql` pada Neon yang sudah berisi schema lama sebelum mencoba upload. Uji unggah beberapa foto/video, format yang tidak didukung, dan batas ukuran. Media campaign tertunda hanya dapat dilihat pemilik/Admin; media campaign aktif dapat ditampilkan publik.
- **Skenario WebSocket yang sesuai:** koneksi ke `/ws/donasi`, penerimaan event campaign/donasi yang dihasilkan oleh layanan, dan perilaku broadcast kepada koneksi yang tersambung. Jangan menyatakan reconnect otomatis, pembaruan ulang daftar campaign, atau pembatasan event per campaign sebagai perilaku yang sudah didukung.
- **Belum dapat diuji sebagai fitur berjalan:** alur donasi uang sungguhan dari UI sampai GoPay/payment provider, status pembayaran provider seperti kedaluwarsa, callback resmi provider, sinkronisasi transaksi provider, serta pemulihan/replay event setelah koneksi terputus. Ini memerlukan implementasi tambahan.
- **Di luar pengujian lokal:** transaksi uang sungguhan, pengujian production, dan load/penetration test terhadap sistem atau provider tanpa persetujuan. Pengujian beban atau payload berbahaya hanya dilakukan pada lingkungan terisolasi dengan izin tim.

## Batas progres
Implementasi repository saat ini mencakup alur campaign, autentikasi, webhook pembayaran simulasi, dan broadcast notifikasi. Integrasi payment provider, message queue/event broker (Sesi 11), code freeze/deployment final (Sesi 12), serta pelaksanaan dan bukti pengujian formal (Sesi 13–14) belum tersedia di sini. HTTPS/TLS perlu diaktifkan saat deployment melalui reverse proxy dengan sertifikat.

## Catatan pengembangan
Konfigurasi Docker Compose disediakan sebagai lingkungan demonstrasi. Untuk produksi, tambahkan manajemen secret, TLS, rate limiting terdistribusi, audit log persisten, validasi webhook sesuai dokumentasi payment gateway yang dipakai, dan analisis kompleksitas GraphQL.
