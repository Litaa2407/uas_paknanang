# Platform Crowdfunding dan Donasi — Kelompok E

Repository ini berisi implementasi backend dan frontend untuk platform crowdfunding/donasi. Pengerjaan Week 12 difokuskan pada Code Freeze: integrasi service, keamanan komunikasi internal, message queue, dokumentasi API, dan deployment lokal yang konsisten menggunakan Docker Compose.

## 1. Arsitektur

| Komponen | Teknologi | Tanggung jawab |
|---|---|---|
| Frontend | React + Vite + Nginx | Antarmuka aplikasi; tidak diubah pada paket backend Week 12 |
| API Gateway | Go | Entry point HTTP, reverse proxy, CORS, service registry, health check, round-robin |
| Campaign Service (2 instance) | Go, GraphQL, PostgreSQL | Registrasi/login, JWT pengguna, campaign, persetujuan admin, payment webhook, transactional outbox |
| Notification Service | Go, WebSocket | Consumer RabbitMQ dan broadcast notifikasi real-time |
| Database | PostgreSQL 16 | Data pengguna, campaign, donasi, dan outbox event |
| Message Broker | RabbitMQ 4.4.0 | Pengiriman event asinkron dari Campaign Service ke Notification Service |

Alur request utama:

```text
Browser :5173
    |
    v
API Gateway :8000
    |-- OAuth2 Client Credentials untuk identitas service
    |-- mTLS untuk koneksi internal
    |
    +---- Round Robin ----> Campaign Service :8443 (instance 1/2)
    |                           |
    |                           +--> PostgreSQL
    |                           +--> Transactional Outbox
    |                                     |
    |                                     v
    |                                  RabbitMQ
    |                                     |
    +------------------------------> Notification Service :8443
                                          |
                                          +--> WebSocket clients
```

Port Campaign Service dan Notification Service tidak dipublikasikan ke host. Browser hanya mengakses Gateway. Komunikasi Gateway ke kedua backend menggunakan HTTPS dengan mutual TLS dan token OAuth2 internal.

## 2. Persiapan

Pastikan perangkat memiliki:

- Docker Desktop atau Docker Engine dengan Docker Compose v2.
- OpenSSL untuk membuat sertifikat development.
- Git (opsional, untuk mengelola perubahan repository).

Tidak perlu menjalankan Go, PostgreSQL, RabbitMQ, atau Node.js secara manual pada host karena seluruh service dijalankan melalui Docker Compose.

## 3. Konfigurasi environment

Salin template environment:

**PowerShell**

```powershell
Copy-Item .env.example .env -Force
```

**Linux/macOS/Git Bash**

```bash
cp .env.example .env
```

Edit `.env` dan isi semua variabel secret yang masih kosong dengan nilai acak yang berbeda. Contoh membuat nilai acak menggunakan OpenSSL:

```bash
openssl rand -hex 32
```

Variabel yang wajib diisi:

| Variabel | Fungsi |
|---|---|
| `DB_PASSWORD` | Password PostgreSQL |
| `JWT_SECRET` | Signing key JWT pengguna dan token internal; minimal 32 karakter |
| `PAYMENT_WEBHOOK_SECRET` | HMAC key untuk payment webhook; minimal 32 karakter |
| `OAUTH_CLIENT_ID` | Identitas Gateway pada OAuth2 Client Credentials |
| `OAUTH_CLIENT_SECRET` | Secret Gateway; minimal 32 karakter |
| `RABBITMQ_USER` | Username RabbitMQ |
| `RABBITMQ_PASSWORD` | Password RabbitMQ |
| `CORS_ALLOWED_ORIGINS` | Origin frontend yang diizinkan, default `http://localhost:5173` |

Untuk menghindari masalah parsing URL pada konfigurasi Docker Compose, gunakan karakter alfanumerik saja untuk `DB_PASSWORD` dan `RABBITMQ_PASSWORD`.

Jika sebelumnya kamu pernah menjalankan Docker Compose versi lama dan volume `pgdata` masih ada, perubahan `DB_PASSWORD` pada `.env` tidak otomatis mengganti password role PostgreSQL yang tersimpan di volume. Sebelum menjalankan seluruh stack, start database dan samakan password role dengan nilai baru di `.env`:

```bash
docker compose up -d db
docker compose exec db psql -U crowdfunding -d crowdfunding -c "ALTER USER crowdfunding WITH PASSWORD 'ISI_DENGAN_DB_PASSWORD_BARU_DARI_ENV';"
```

Ganti placeholder SQL tersebut dengan nilai `DB_PASSWORD` yang sama persis dari `.env`. Jika ini instalasi baru tanpa volume database lama, langkah migrasi ini tidak diperlukan.

**Jangan commit `.env`, password, private key, atau sertifikat lokal ke GitHub.** File `.env` yang pernah terunggah ke repository harus dianggap terekspos: rotasi seluruh password/secret tersebut dan hapus file `.env` dari tracking Git (`git rm --cached .env`). `.gitignore` pada paket ini mencegah file tersebut ikut ditambahkan lagi, tetapi tidak menghapus riwayat Git yang sudah ada.

## 4. Membuat sertifikat mTLS lokal

Compose mengaktifkan mTLS untuk koneksi Gateway ke backend. Buat CA dan sertifikat lokal sebelum menjalankan Docker.

**Windows PowerShell**

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\generate-dev-certs.ps1
```

**Linux/macOS/Git Bash**

```bash
bash scripts/generate-dev-certs.sh
```

Script membuat folder `certs/` berisi CA, sertifikat server Campaign Service dan Notification Service, serta sertifikat client Gateway. Folder tersebut diabaikan oleh Git.

Sertifikat ini hanya untuk demonstrasi lokal. Untuk deployment publik, gunakan CA dan sertifikat yang dikelola dengan benar, serta jangan gunakan private key development.

## 5. Menjalankan seluruh aplikasi

Dari folder utama repository, jalankan:

```bash
docker compose up --build -d
```

Periksa status container:

```bash
docker compose ps
```

Lihat log service:

```bash
docker compose logs -f gateway campaign-service campaign-service-2 notification-service rabbitmq
```

Alamat lokal:

| Komponen | URL |
|---|---|
| Frontend | http://localhost:5173 |
| API Gateway | http://localhost:8000 |
| Gateway health | http://localhost:8000/health |
| Service registry | http://localhost:8000/registry |
| RabbitMQ Management UI | http://localhost:15672 |

Login RabbitMQ Management UI menggunakan `RABBITMQ_USER` dan `RABBITMQ_PASSWORD` dari `.env`.

Untuk menghentikan container tanpa menghapus data database:

```bash
docker compose down
```

Untuk menghapus seluruh data database lokal (tindakan ini menghapus data demo):

```bash
docker compose down -v
```

## 6. Akun dan role

Registrasi publik selalu membuat akun dengan role `DONATUR`. Sesuai aturan proyek saat ini, semua pengguna yang sudah login dapat mengajukan campaign. Campaign baru berstatus `MENUNGGU` dan belum muncul pada daftar publik sampai disetujui `ADMIN`. Mutasi `approveCampaign` dan query `pendingCampaigns` hanya dapat digunakan oleh `ADMIN`.

Untuk demo lokal, role akun dapat diubah oleh administrator database. Contoh membuat akun yang sudah terdaftar menjadi ADMIN:

```bash
docker compose exec db psql -U crowdfunding -d crowdfunding -c "UPDATE users SET role='ADMIN' WHERE email='admin@example.com';"
```

Ganti email dengan email akun yang telah didaftarkan. Jangan menyediakan akses SQL admin kepada pengguna aplikasi.

## 7. Fitur Week 8–12

- **Week 8 — OAuth2/mTLS:** Gateway memperoleh access token melalui OAuth2 Client Credentials. Gateway menyertakan token service internal saat memanggil backend. Koneksi internal menggunakan mTLS dan backend memeriksa sertifikat client dengan Common Name `gateway`.
- **Week 9 — API Gateway/reverse proxy:** seluruh request browser menuju Gateway. Gateway meneruskan route autentikasi, GraphQL, webhook, dan WebSocket ke service terkait.
- **Week 10 — Service discovery/load balancing:** Gateway memuat daftar instance dari environment variable, memeriksa `/health` setiap lima detik, lalu memilih instance Campaign Service yang sehat menggunakan Round Robin.
- **Week 11 — Message queue:** perubahan campaign/donasi dan pencatatan event disimpan dalam satu transaksi PostgreSQL melalui transactional outbox. Worker mengirim event secara persisten ke RabbitMQ. Notification Service mengonsumsi queue dan menyiarkan event melalui WebSocket.
- **Week 12 — Code Freeze:** Docker Compose, konfigurasi environment, sertifikat development, dokumentasi API, dan panduan deployment disediakan dalam repository.

### Catatan reliabilitas message queue

Transactional outbox mencegah event hilang hanya karena broker sedang tidak tersedia setelah perubahan data berhasil disimpan. Advisory transaction lock PostgreSQL memastikan satu worker memublikasikan event secara berurutan berdasarkan ID, dan worker akan mencoba kembali event yang belum terkirim. Pengiriman menggunakan pola *at-least-once*: pada kondisi crash di antara publish dan update status outbox, event dapat terkirim ulang. Setiap pesan membawa `event_id` agar consumer/klien dapat mengenali duplikasi. WebSocket adalah notifikasi real-time, bukan penyimpanan riwayat notifikasi.

## 8. Pemeriksaan setelah deployment

```bash
curl http://localhost:8000/health
curl http://localhost:8000/registry
```

`/registry` menampilkan status health Campaign Service dan Notification Service. Kedua instance Campaign Service seharusnya berstatus `true` setelah health check pertama.

Contoh registrasi melalui Gateway:

```bash
curl -X POST http://localhost:8000/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Donatur Uji","email":"donatur@example.com","password":"DonaturUji123"}'
```

Contoh login:

```bash
curl -X POST http://localhost:8000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"donatur@example.com","password":"DonaturUji123"}'
```

Untuk contoh GraphQL, webhook, dan WebSocket, lihat [API.md](API.md).

## 9. Batasan dan catatan deployment

- OAuth2 yang diimplementasikan pada tahap ini adalah **Client Credentials untuk komunikasi service-to-service**, bukan Authorization Code flow untuk login pengguna. Login pengguna aplikasi tetap menggunakan email/password dan JWT.
- Sertifikat mTLS pada script hanya sertifikat development lokal.
- Payment webhook merupakan simulasi integrasi. Secret HMAC diverifikasi, tetapi format payload dan signature harus disesuaikan dengan dokumentasi payment gateway yang benar-benar digunakan.
- Service registry masih berbasis daftar URL dari environment variable, bukan registry dinamis eksternal.
- RabbitMQ Notification Service menggunakan satu queue bersama; konfigurasi ini sesuai untuk satu instance Notification Service pada demo. Jika notification service diskalakan, desain queue perlu diubah agar setiap instance menerima salinan event untuk client WebSocket miliknya. Koneksi AMQP pada Docker network saat ini memakai username/password tanpa AMQPS; aktifkan TLS broker sebelum deployment pada jaringan yang tidak tepercaya.
- GraphQL memiliki batas request size dan pemeriksaan kompleksitas dasar. Untuk produksi, tambahkan query depth/cost analyzer dan rate limiting.
- Gateway diakses browser melalui HTTP pada localhost. Untuk deployment internet, aktifkan HTTPS publik di reverse proxy/load balancer terdepan.
- Verifikasi build image dan pengujian integrasi perlu dilakukan di perangkat yang memiliki Docker dan akses untuk mengunduh dependency Go.

## 10. Dokumentasi

- [API.md](API.md) — kontrak endpoint, autentikasi, GraphQL, webhook, event, dan contoh request.
- `db/init.sql` — skema database awal. Campaign Service juga memastikan tabel outbox tersedia saat startup sehingga volume database lama dapat menjalankan versi ini.
