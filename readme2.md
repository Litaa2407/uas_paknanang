# Platform Crowdfunding dan Donasi
## Kelompok E – Panduan Instalasi dan Menjalankan Proyek

Aplikasi Platform Crowdfunding dan Donasi merupakan sistem yang digunakan untuk membantu pengguna mencari campaign penggalangan dana, melakukan donasi, dan melihat perkembangan campaign.

Proyek ini dikembangkan menggunakan Go, React, PostgreSQL, dan GraphQL. Backend menggunakan arsitektur microservices yang diakses melalui API Gateway.

---

## 1. Teknologi yang Digunakan

| Teknologi | Kegunaan |
|---|---|
| Go | Backend dan microservices |
| React | Antarmuka frontend |
| Vite | Development server frontend |
| PostgreSQL | Database |
| Neon | Hosting database PostgreSQL |
| GraphQL | Protokol pengambilan data |
| API Gateway | Pintu masuk request ke backend |
| Round Robin | Pembagian request ke beberapa instance Campaign Service |

## 2. Struktur Folder Proyek

Pastikan struktur folder proyek tetap seperti berikut:

```text
crowdfunding-sesi10/
│
├── campaign-service/
│   ├── main.go
│   ├── go.mod
│   └── go.sum
│
├── notification-service/
│   ├── main.go
│   ├── go.mod
│   └── go.sum
│
├── gateway/
│   ├── main.go
│   ├── go.mod
│   └── go.sum
│
├── frontend/
│   ├── package.json
│   ├── src/
│   └── ...
│
├── db/
│   └── init.sql
│
├── .env.example
├── docker-compose.yml
└── README.md
```

**Catatan:** File `docker-compose.yml` tersedia di proyek, tetapi tidak digunakan dalam panduan ini. Seluruh service dijalankan secara manual menggunakan terminal.

## 3. Persiapan Sebelum Menjalankan Proyek

Sebelum menjalankan aplikasi, pastikan laptop sudah memiliki perangkat berikut.

### 3.1 Install Go

Download Go melalui situs resmi:

https://go.dev/dl/

Pilih installer sesuai sistem operasi laptop, kemudian lakukan instalasi.

Periksa apakah Go sudah terpasang dengan membuka PowerShell:

```powershell
go version
```

Jika berhasil, akan muncul informasi versi Go.

### 3.2 Install Node.js

Download Node.js melalui:

https://nodejs.org/

Pilih versi LTS dan lakukan instalasi.

Periksa instalasi:

```powershell
node -v
npm -v
```

Kedua perintah harus menampilkan nomor versi.

### 3.3 Install Visual Studio Code

Download melalui:

https://code.visualstudio.com/

VS Code digunakan untuk membuka folder proyek, mengedit kode, dan menjalankan beberapa terminal sekaligus.

### 3.4 Siapkan Database PostgreSQL di Neon

Proyek menggunakan PostgreSQL yang di-host melalui Neon sehingga tidak perlu memasang PostgreSQL secara lokal.

1. Buka https://neon.com/
2. Buat akun atau login.
3. Buat project PostgreSQL baru.
4. Pilih region yang sesuai.
5. Buka dashboard project.
6. Salin connection string PostgreSQL.
7. Simpan connection string tersebut karena akan digunakan oleh Campaign Service.

**Penting:** Setiap anggota yang menjalankan proyek dengan database sendiri harus menggunakan connection string database miliknya. Jangan menggunakan connection string milik orang lain tanpa izin.

## 4. Persiapan Database

Setelah membuat database Neon, buka SQL Editor pada dashboard Neon.

Buka file:

```text
db/init.sql
```

Salin seluruh isi file tersebut, kemudian jalankan pada SQL Editor Neon.

File tersebut digunakan untuk membuat tabel yang diperlukan aplikasi, seperti:

- `users`
- `campaigns`
- `donations`

Setelah query berhasil dijalankan, periksa bagian Tables pada dashboard Neon untuk memastikan tabel sudah tersedia.

**Jangan membuat ulang tabel secara manual jika script `init.sql` sudah berhasil dijalankan.**

## 5. Konfigurasi Environment Variable

Backend membutuhkan beberapa konfigurasi agar dapat terhubung ke database dan service lainnya.

Buat file `.env` pada folder utama proyek:

```text
crowdfunding-sesi10/.env
```

Isi dengan konfigurasi berikut:

```env
DATABASE_URL=ISI_CONNECTION_STRING_NEON
JWT_SECRET=GANTI_DENGAN_SECRET_MINIMAL_32_KARAKTER
PAYMENT_WEBHOOK_SECRET=GANTI_DENGAN_SECRET_WEBHOOK
```

Keterangan:

| Variabel | Fungsi |
|---|---|
| `DATABASE_URL` | Connection string PostgreSQL dari Neon |
| `JWT_SECRET` | Secret untuk proses autentikasi token |
| `PAYMENT_WEBHOOK_SECRET` | Secret untuk verifikasi webhook pembayaran |

Ganti nilai contoh dengan nilai yang sesuai.

Gunakan `JWT_SECRET` dan `PAYMENT_WEBHOOK_SECRET` yang sama pada kedua instance Campaign Service.

**Perhatian keamanan:**
- Jangan mengunggah file `.env` ke GitHub.
- Jangan membagikan password database atau secret kepada orang lain.
- Gunakan `.env.example` sebagai template tanpa memasukkan kredensial asli.

### 5.1 Memuat Environment Variable di PowerShell

Kode Go dalam proyek ini tidak otomatis membaca file `.env`. Karena itu, environment variable perlu dimasukkan ke terminal sebelum menjalankan Campaign Service.

Jalankan perintah berikut pada setiap terminal Campaign Service:

```powershell
$env:DATABASE_URL="ISI_CONNECTION_STRING_NEON"
$env:JWT_SECRET="SECRET_MINIMAL_32_KARAKTER"
$env:PAYMENT_WEBHOOK_SECRET="SECRET_WEBHOOK"
```

Ganti nilai di atas sesuai konfigurasi pada file `.env`.

## 6. Mengunduh Dependency Proyek

Buka folder utama proyek menggunakan VS Code.

Buka terminal PowerShell dan jalankan perintah berikut satu per satu.

### 6.1 Campaign Service

```powershell
cd campaign-service
go mod tidy
```

### 6.2 Notification Service

Buka terminal baru, kemudian:

```powershell
cd notification-service
go mod tidy
```

### 6.3 API Gateway

Buka terminal baru, kemudian:

```powershell
cd gateway
go mod tidy
```

### 6.4 Frontend

Buka terminal baru:

```powershell
cd frontend
npm install
```

Tunggu sampai seluruh dependency berhasil diunduh.

## 7. Menjalankan Seluruh Service

Aplikasi dijalankan menggunakan beberapa terminal secara bersamaan.

**Jangan menutup terminal yang sedang menjalankan service.**

Pastikan seluruh service tetap aktif selama pengujian.

### 7.1 Terminal 1 – Campaign Service Instance 1

Buka terminal pertama di VS Code.

Masuk ke folder:

```powershell
cd campaign-service
```

Masukkan konfigurasi environment variable:

```powershell
$env:DATABASE_URL="ISI_CONNECTION_STRING_NEON"
$env:JWT_SECRET="SECRET_MINIMAL_32_KARAKTER"
$env:PAYMENT_WEBHOOK_SECRET="SECRET_WEBHOOK"
$env:PORT="8001"
$env:NOTIF_URL="http://localhost:8002"
```

Jalankan:

```powershell
go run .
```

Campaign Service Instance 1 berjalan pada:

```text
http://localhost:8001
```

### 7.2 Terminal 2 – Notification Service

Buka terminal kedua.

Masuk ke folder:

```powershell
cd notification-service
```

Jalankan:

```powershell
$env:PORT="8002"
go run .
```

Notification Service berjalan pada:

```text
http://localhost:8002
```

### 7.3 Terminal 3 – Campaign Service Instance 2

Buka terminal ketiga.

Masuk ke folder:

```powershell
cd campaign-service
```

Masukkan environment variable yang sama dengan Campaign Service Instance 1, tetapi gunakan port `8003`.

```powershell
$env:DATABASE_URL="ISI_CONNECTION_STRING_NEON"
$env:JWT_SECRET="SECRET_MINIMAL_32_KARAKTER"
$env:PAYMENT_WEBHOOK_SECRET="SECRET_WEBHOOK"
$env:PORT="8003"
$env:NOTIF_URL="http://localhost:8002"
```

Jalankan:

```powershell
go run .
```

Campaign Service Instance 2 berjalan pada:

```text
http://localhost:8003
```

Kedua Campaign Service menggunakan database Neon yang sama.

### 7.4 Terminal 4 – API Gateway

Buka terminal keempat.

Masuk ke folder:

```powershell
cd gateway
```

Masukkan konfigurasi:

```powershell
$env:PORT="8000"
$env:CAMPAIGN_SERVICES="http://localhost:8001,http://localhost:8003"
$env:NOTIFICATION_SERVICE="http://localhost:8002"
```

Jalankan:

```powershell
go run .
```

Jika berhasil, terminal akan menampilkan:

```text
API Gateway :8000
```

API Gateway dapat diakses melalui:

```text
http://localhost:8000
```

Gateway akan memeriksa kesehatan service setiap 5 detik dan memilih instance Campaign Service yang sehat menggunakan algoritma Round Robin.

### 7.5 Terminal 5 – Frontend

Buka terminal kelima.

Masuk ke folder:

```powershell
cd frontend
```

Install dependency jika belum dilakukan:

```powershell
npm install
```

Jalankan frontend:

```powershell
npm run dev
```

Jika berhasil, Vite akan menampilkan alamat development server, biasanya:

```text
http://localhost:5173
```

Buka alamat tersebut di browser.

**Catatan:** Jika konfigurasi alamat backend pada frontend masih mengarah ke port service langsung, sesuaikan konfigurasi API frontend agar menggunakan Gateway pada `http://localhost:8000`. Periksa konfigurasi API yang sudah tersedia pada source code frontend sebelum mengubah nama environment variable.

---

## 8. Pengujian Service Discovery

Setelah seluruh service berjalan, buka terminal baru untuk melakukan pengujian.

Jalankan:

```powershell
Invoke-RestMethod http://localhost:8000/registry | ConvertTo-Json -Depth 5
```

Jika seluruh service aktif, hasilnya kurang lebih:

```json
{
  "campaign": {
    "http://localhost:8001": true,
    "http://localhost:8003": true
  },
  "notification": {
    "http://localhost:8002": true
  }
}
```

Keterangan:

- `true`: Service berhasil merespons health check.
- `false`: Service tidak berhasil merespons health check.
- Jika salah satu service mati, Gateway akan memperbarui statusnya melalui pemeriksaan berkala.

## 9. Pengujian Health Check

### 9.1 API Gateway

```powershell
Invoke-RestMethod http://localhost:8000/health
```

Hasil yang diharapkan:

```json
{
  "status": "ok",
  "service": "gateway"
}
```

### 9.2 Campaign Service Instance 1

```powershell
Invoke-RestMethod http://localhost:8001/health
```

### 9.3 Campaign Service Instance 2

```powershell
Invoke-RestMethod http://localhost:8003/health
```

### 9.4 Notification Service

```powershell
Invoke-RestMethod http://localhost:8002/health
```

Setiap service harus memberikan respons health check yang sesuai.

---

## 10. Pengujian Registrasi Melalui API Gateway

Pengujian ini bertujuan memastikan request dari client dapat diteruskan melalui Gateway ke Campaign Service dan data berhasil disimpan ke PostgreSQL.

Jalankan perintah berikut:

```powershell
$body = @{
    name = "Donatur Test"
    email = "donaturtest@example.com"
    password = "DonaturTest123"
} | ConvertTo-Json

Invoke-RestMethod `
    -Uri "http://localhost:8000/auth/register" `
    -Method POST `
    -ContentType "application/json" `
    -Body $body
```

Jika berhasil, periksa tabel `users` di Neon.

Contoh query:

```sql
SELECT id, name, email, role
FROM users
ORDER BY id;
```

Data pengguna yang berhasil didaftarkan akan muncul di database.

Gunakan email berbeda untuk setiap pengujian registrasi agar tidak terjadi duplikasi.

---

## 11. Pengujian Load Balancing Round Robin

Proyek memiliki dua Campaign Service:

- Instance 1: `http://localhost:8001`
- Instance 2: `http://localhost:8003`

API Gateway menggunakan Round Robin untuk membagikan request kepada instance yang sehat.

Gateway juga menyediakan header `X-Served-By` untuk menunjukkan alamat instance yang dipilih.

Pastikan menggunakan versi `gateway/main.go` yang sudah dilengkapi header `X-Served-By`.

Jalankan pengujian berikut dari terminal baru:

```powershell
1..6 | ForEach-Object {
    $response = Invoke-WebRequest `
        -Uri "http://localhost:8000/auth/register" `
        -Method POST `
        -ContentType "application/json" `
        -Body (@{
            name = "Load Balance Test $_"
            email = "loadbalance$_@example.com"
            password = "GatewayTest123"
        } | ConvertTo-Json)

    [PSCustomObject]@{
        Request = $_
        Status = $response.StatusCode
        ServedBy = $response.Headers["X-Served-By"]
    }
}
```

Hasil yang diharapkan ketika kedua instance sehat dan tidak ada request lain yang menggunakan Registry:

| Request | Instance |
|---|---|
| 1 | localhost:8001 |
| 2 | localhost:8003 |
| 3 | localhost:8001 |
| 4 | localhost:8003 |
| 5 | localhost:8001 |
| 6 | localhost:8003 |

Status HTTP bergantung pada respons endpoint registrasi.

Selain itu, terminal API Gateway akan menampilkan log seperti:

```text
Load Balancing: POST /auth/register -> http://localhost:8001
Load Balancing: POST /auth/register -> http://localhost:8003
```

Dengan demikian, pembagian request dapat diamati melalui header respons dan log Gateway.

---

## 12. Daftar Port Aplikasi

| Komponen | Port | Alamat |
|---|---:|---|
| API Gateway | 8000 | http://localhost:8000 |
| Campaign Service 1 | 8001 | http://localhost:8001 |
| Notification Service | 8002 | http://localhost:8002 |
| Campaign Service 2 | 8003 | http://localhost:8003 |
| Frontend | 5173 | http://localhost:5173 |

Pastikan tidak ada aplikasi lain yang sedang menggunakan port tersebut.

---

## 13. Troubleshooting

### 13.1 Perintah `go` Tidak Dikenali

Jika PowerShell menampilkan:

```text
go : The term 'go' is not recognized
```

Pastikan Go sudah diinstal dan PATH sudah dikonfigurasi.

Tutup dan buka kembali terminal setelah melakukan perubahan PATH.

### 13.2 Perintah `npm` Tidak Dikenali

Pastikan Node.js sudah terinstal dengan benar.

Periksa:

```powershell
node -v
npm -v
```

Jika belum dikenali, periksa instalasi Node.js dan PATH.

### 13.3 Error `DATABASE_URL wajib diisi`

Pastikan environment variable sudah dimasukkan sebelum menjalankan Campaign Service:

```powershell
$env:DATABASE_URL="ISI_CONNECTION_STRING_NEON"
```

### 13.4 Error `JWT_SECRET wajib minimal 32 karakter`

Gunakan secret dengan panjang minimal 32 karakter.

Contoh untuk pengujian lokal:

```powershell
$env:JWT_SECRET="CrowdfundingSecretForLocalTesting2026"
```

Gunakan nilai yang sama untuk kedua Campaign Service.

### 13.5 Error Koneksi Database

Periksa hal berikut:

1. Connection string Neon sudah benar.
2. Database Neon masih aktif.
3. Connection string menggunakan konfigurasi SSL yang diperlukan Neon.
4. Environment variable `DATABASE_URL` sudah dimasukkan ke terminal Campaign Service.
5. Tabel database sudah dibuat menggunakan `db/init.sql`.

### 13.6 Error `EADDRINUSE`

Contoh:

```text
listen tcp :8001: bind: Only one usage of each socket address
```

Artinya port sedang digunakan oleh proses lain.

Periksa proses yang menggunakan port:

```powershell
netstat -ano | findstr :8001
```

Jika masih ada proses service lama, hentikan proses tersebut sebelum menjalankan service kembali.

Jangan menjalankan dua instance pada port yang sama.

### 13.7 Gateway Menampilkan Status `false`

Periksa apakah service terkait masih berjalan.

Pastikan alamat dan port pada konfigurasi Gateway sesuai:

```powershell
$env:CAMPAIGN_SERVICES="http://localhost:8001,http://localhost:8003"
$env:NOTIFICATION_SERVICE="http://localhost:8002"
```

Tunggu beberapa detik sampai health check berikutnya dilakukan.

### 13.8 Error `service tidak tersedia`

Pastikan minimal satu Campaign Service aktif dan berstatus sehat pada endpoint:

```text
http://localhost:8000/registry
```

Jika kedua instance tidak aktif, Gateway tidak dapat meneruskan request ke Campaign Service.

### 13.9 Registrasi Gagal karena Email Sudah Digunakan

Gunakan alamat email lain untuk pengujian.

Contoh:

```text
test1@example.com
test2@example.com
test3@example.com
```

### 13.10 Frontend Tidak Bisa Mengakses Backend

Pastikan:

1. Frontend berjalan pada port `5173`.
2. API Gateway berjalan pada port `8000`.
3. Alamat API frontend mengarah ke Gateway.
4. Konfigurasi CORS Gateway sesuai dengan origin frontend.

---

## 14. Urutan Menjalankan Aplikasi

Untuk memudahkan, urutan menjalankan proyek adalah:

1. Siapkan database Neon dan jalankan `db/init.sql`.
2. Jalankan Campaign Service Instance 1 pada port `8001`.
3. Jalankan Notification Service pada port `8002`.
4. Jalankan Campaign Service Instance 2 pada port `8003`.
5. Jalankan API Gateway pada port `8000`.
6. Jalankan Frontend pada port `5173`.
7. Periksa status service melalui `/registry`.
8. Buka aplikasi melalui browser.

Semua terminal service harus tetap berjalan selama aplikasi digunakan.

## 15. Arsitektur Sistem

Gambaran komunikasi antar komponen:

```text
                  USER / BROWSER
                        |
                        v
                REACT + VITE
                 Port 5173
                        |
                        v
                 API GATEWAY
                  Port 8000
                        |
              +---------+---------+
              |                   |
              v                   v
       CAMPAIGN SERVICE     NOTIFICATION SERVICE
          Registry               Port 8002
              |
       ROUND ROBIN
              |
        +-----+-----+
        |           |
        v           v
   CAMPAIGN 1   CAMPAIGN 2
    Port 8001    Port 8003
        |           |
        +-----+-----+
              |
              v
        NEON POSTGRESQL
```

API Gateway menjadi pintu masuk request dari frontend. Request diteruskan ke service yang sesuai. Dua instance Campaign Service digunakan untuk menerapkan pembagian request menggunakan algoritma Round Robin.

## 16. Batasan dan Catatan Implementasi

- Database menggunakan PostgreSQL yang di-host di Neon.
- Kedua Campaign Service menggunakan database yang sama.
- API Gateway melakukan pemeriksaan kesehatan service setiap 5 detik.
- Round Robin memilih instance yang berstatus sehat.
- Jika seluruh Campaign Service tidak tersedia, Gateway memberikan respons HTTP 503.
- Pengujian load balancing menggunakan header `X-Served-By` memerlukan versi Gateway yang sudah ditambahkan fitur tersebut.
- Konfigurasi pembayaran/webhook memerlukan secret yang sesuai. Integrasi dengan penyedia pembayaran eksternal harus menggunakan konfigurasi dan kredensial penyedia yang valid.
- Fitur yang masih berupa rancangan keamanan tidak boleh dianggap sudah diterapkan seluruhnya hanya berdasarkan keberadaan konfigurasi proyek.

---

## 17. Kesimpulan

Proyek Platform Crowdfunding dan Donasi dapat dijalankan secara lokal dengan memisahkan frontend, API Gateway, Campaign Service, dan Notification Service ke dalam beberapa terminal.

Arsitektur microservices memungkinkan Campaign Service dijalankan dalam lebih dari satu instance. API Gateway bertugas meneruskan request dan membagikan request ke instance yang sehat menggunakan Round Robin.

Database PostgreSQL melalui Neon digunakan bersama oleh kedua instance Campaign Service sehingga data tetap tersimpan pada satu basis data.

Dengan mengikuti panduan ini, anggota kelompok dapat menyiapkan lingkungan pengembangan dan menjalankan aplikasi pada laptop masing-masing tanpa harus menggunakan Docker.