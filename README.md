# Donasi Bersama — Platform Crowdfunding dan Donasi

**Kelompok pembangun:** E (Kelompok 5)
**Jenis aplikasi:** Web crowdfunding dan donasi
**Versi dokumentasi:** Revisi serah-terima Code Freeze
**Stack:** React, Go, GraphQL, REST, PostgreSQL, RabbitMQ, Docker, Nginx, Railway

Dokumen ini merupakan panduan utama proyek. Informasi instalasi, konfigurasi, endpoint REST, GraphQL, OpenAPI, Google OAuth 2.0, deployment Railway, pengujian, dan keamanan digabungkan di sini agar tidak tersebar di banyak file Markdown.

---

## 1. URL aplikasi dan dokumentasi

### Deployment Railway

| Kegunaan | URL |
|---|---|
| Aplikasi frontend | <https://frontend-production-666ac5dd.up.railway.app> |
| Base URL API | <https://frontend-production-666ac5dd.up.railway.app> |
| GraphQL | <https://frontend-production-666ac5dd.up.railway.app/graphql> |
| Swagger UI | <https://frontend-production-666ac5dd.up.railway.app/docs/> |
| Spesifikasi OpenAPI 3.1 | <https://frontend-production-666ac5dd.up.railway.app/openapi.yaml> |
| Gateway health check | <https://frontend-production-666ac5dd.up.railway.app/health> |
| Service registry | <https://frontend-production-666ac5dd.up.railway.app/registry> |
| WebSocket notifikasi | `wss://frontend-production-666ac5dd.up.railway.app/ws/donasi` |

Frontend, API Gateway, dan backend menggunakan satu origin publik. Request `/auth/`, `/graphql`, `/campaigns`, `/donations`, `/webhook/`, `/health`, `/registry`, dan `/ws/` diteruskan oleh Nginx frontend ke Gateway. Gateway meneruskan request ke Campaign Service atau Notification Service melalui jaringan internal Railway.

> Catatan: URL di atas adalah domain deployment yang diberikan untuk proyek ini. Ketersediaan endpoint harus diverifikasi kembali setelah setiap deployment. Swagger UI memuat library dari CDN, sehingga browser memerlukan akses internet untuk mengambil aset Swagger.

### Lokal

| Kegunaan | URL |
|---|---|
| Frontend dan API melalui Nginx | `http://localhost:5173` |
| GraphQL | `http://localhost:5173/graphql` |
| Swagger UI | `http://localhost:5173/docs/` |
| OpenAPI YAML | `http://localhost:5173/openapi.yaml` |
| Gateway langsung (opsional) | `http://localhost:8000` |
| Health Gateway langsung | `http://localhost:8000/health` |
| Registry Gateway langsung | `http://localhost:8000/registry` |
| WebSocket | `ws://localhost:5173/ws/donasi` |

Untuk penggunaan browser, utamakan origin frontend `http://localhost:5173`. Port internal Campaign Service dan Notification Service tidak perlu dibuka ke publik.

---

## 2. Ringkasan aplikasi dan fitur

Donasi Bersama adalah platform untuk melihat campaign penggalangan dana, mengajukan campaign, berdonasi, melihat perkembangan donasi, mengirim pertanyaan kepada pengelola campaign, serta melihat laporan penggunaan dana.

Fitur utama:

- Registrasi dan login menggunakan email/password.
- Login menggunakan Google OAuth 2.0 / OpenID Connect.
- Autentikasi aplikasi menggunakan JWT.
- Pengajuan campaign beserta foto dan video.
- Persetujuan campaign oleh ADMIN.
- Query dan mutation data campaign melalui GraphQL.
- Pertanyaan donatur dan jawaban pengelola campaign.
- Informasi donatur secara anonim, jumlah donatur, dan laporan penggunaan dana.
- Upload bukti penggunaan dana.
- Pembayaran donasi melalui Midtrans Snap Sandbox jika `MIDTRANS_SERVER_KEY` dikonfigurasi.
- Webhook Midtrans dan webhook simulasi.
- Notifikasi aktivitas melalui WebSocket dan RabbitMQ.
- API Gateway, health check, service registry, dan load balancing instance Campaign Service.

### Peran pengguna

| Role | Hak akses utama |
|---|---|
| `DONATUR` | Melihat campaign, mengirim pertanyaan, membuat donasi, melihat riwayat donasi. Registrasi publik dan akun Google baru selalu memperoleh role ini. |
| `PENGELOLA` | Mengelola campaign miliknya sesuai aturan backend, menjawab pertanyaan, dan mencatat penggunaan dana untuk campaign miliknya. |
| `ADMIN` | Melihat campaign yang menunggu, menyetujui campaign, dan menjalankan fungsi administrasi yang dibatasi backend. |

Role istimewa tidak dapat dipilih melalui formulir registrasi. Penetapan role `PENGELOLA` atau `ADMIN` dilakukan oleh administrator yang berwenang melalui proses administrasi database, bukan dengan mengirim nilai role dari browser.

---

## 3. Arsitektur dan teknologi

```text
Browser
  |
  v
Frontend React + Vite (Nginx)
  |  satu origin: /auth, /graphql, /campaigns, /donations, /webhook, /ws
  v
API Gateway (Go, port 8000)
  |------------------------------|
  v                              v
Campaign Service (Go)       Notification Service (Go)
  |       |                     |
  |       +---- GraphQL         +---- WebSocket
  |       +---- REST            |
  |       +---- Google OAuth    v
  |       +---- Midtrans       RabbitMQ
  v
PostgreSQL
```

| Komponen | Teknologi dan fungsi |
|---|---|
| Frontend | React + Vite; disajikan melalui Nginx. Nginx juga menjadi reverse proxy ke Gateway. |
| API Gateway | Go; reverse proxy, autentikasi service OAuth2 Client Credentials, health check, registry, dan round-robin ke Campaign Service yang sehat. |
| Campaign Service | Go; registrasi, login, Google OAuth, JWT, GraphQL, campaign, media, donasi, Midtrans, webhook, dan transactional outbox. |
| Notification Service | Go; menerima event dari RabbitMQ dan menyebarkan notifikasi melalui WebSocket. |
| Database | PostgreSQL 16. |
| Message broker | RabbitMQ. |
| Deployment lokal | Docker Compose; mTLS antar-service diaktifkan. |
| Deployment Railway | Service terpisah di private network; `MTLS_ENABLED=false` digunakan untuk koneksi HTTP internal Railway sesuai konfigurasi deployment. Autentikasi service OAuth2 tetap aktif. |

Docker Compose menjalankan dua instance Campaign Service (`campaign-service` dan `campaign-service-2`). Gateway memeriksa kesehatan backend dan memilih instance Campaign Service yang sehat. Pada Railway, satu instance Campaign Service dapat digunakan terlebih dahulu; replika tambahan bersifat opsional.

---

## 4. Struktur repository

```text
uas_paknanang-main/
├── campaign-service/       # Backend campaign, auth, GraphQL, donasi, Midtrans
├── db/
│   ├── init.sql             # Skema dasar PostgreSQL untuk database baru
│   └── migrations/          # Perubahan skema bertahap
├── frontend/
│   ├── public/
│   │   ├── docs/index.html  # Swagger UI
│   │   └── openapi.yaml     # OpenAPI 3.1.0, 20 operasi HTTP
│   ├── src/                 # React UI
│   ├── nginx.conf           # Static hosting dan reverse proxy
│   └── Dockerfile
├── gateway/                 # API Gateway Go
├── notification-service/   # WebSocket dan RabbitMQ
├── scripts/                 # Pembuatan sertifikat development
├── .env.example             # Template environment tanpa secret asli
├── .gitignore
├── docker-compose.yml
└── README.md                # Dokumentasi utama proyek
```

File `.env`, folder `certs/`, `frontend/node_modules/`, dan `frontend/dist/` merupakan file/artefak lokal dan tidak boleh di-commit.

---

## 5. Persiapan dan menjalankan secara lokal (Windows)

### Prasyarat

- Windows 10/11.
- Docker Desktop dengan Docker Compose v2.
- PowerShell.
- Git (jika mengambil source dari GitHub).
- Koneksi internet untuk mengunduh Docker image dan dependency.

### Langkah instalasi

1. Ekstrak source code atau clone repository, lalu buka PowerShell di folder root `uas_paknanang-main`.
2. Buat file `.env` dari template:

```powershell
Copy-Item .env.example .env
```

3. Buka `.env` dan ganti placeholder secret. Gunakan nilai acak yang kuat. Secret minimal 32 karakter jika disebutkan sebagai syarat pada tabel environment. Untuk password database dan RabbitMQ lokal, gunakan karakter alfanumerik agar aman sebagai bagian URL koneksi.
4. Buat sertifikat mTLS development:

```powershell
.\scripts\generate-dev-certs.ps1
```

5. Build dan jalankan semua service:

```powershell
docker compose up -d --build
```

6. Periksa status container:

```powershell
docker compose ps
```

7. Buka aplikasi:

```text
http://localhost:5173
```

8. Periksa Gateway:

```powershell
Invoke-RestMethod http://localhost:8000/health
Invoke-RestMethod http://localhost:8000/registry
```

Jika container Campaign Service gagal sehat, periksa log terlebih dahulu:

```powershell
docker compose logs --tail=100 campaign-service campaign-service-2
```

### Menghentikan aplikasi

```powershell
docker compose down
```

Perintah tersebut menghentikan container tetapi mempertahankan volume PostgreSQL. **Jangan gunakan `docker compose down -v` kecuali memang berniat menghapus seluruh data database lokal.** Jangan menjalankan perintah reset atau `TRUNCATE` pada database Railway/produksi.

### Catatan database lokal lama

Jika folder/volume `pgdata` sudah pernah dibuat dengan password berbeda dari `DB_PASSWORD` yang sekarang ada di `.env`, mengubah `.env` saja tidak mengubah password role PostgreSQL di volume lama. Sinkronkan password role database dengan nilai `DB_PASSWORD` atau gunakan database baru yang memang boleh dihapus. Jangan menghapus volume yang berisi data penting.

---

## 6. Konfigurasi environment variable

`.env.example` adalah template konfigurasi. Semua nilai di dalamnya adalah placeholder atau nilai lokal non-rahasia; isi secret sebenarnya hanya pada `.env` lokal atau halaman Variables di Railway. Jangan commit `.env` dan jangan menaruh Client Secret Google, password database, JWT secret, atau Midtrans Server Key di source code.

### Variabel utama aplikasi

| Variabel | Wajib / opsional | Fungsi |
|---|---|---|
| `DB_PASSWORD` | Wajib untuk PostgreSQL Docker Compose | Password user database lokal `crowdfunding`. |
| `DATABASE_URL` | Opsional lokal; wajib pada backend jika memakai database eksternal | Connection string PostgreSQL. Jika kosong pada Compose, dibentuk dari `DB_PASSWORD` dan service `db`. |
| `JWT_SECRET` | Wajib, minimal 32 karakter | Kunci penandatanganan JWT pengguna. |
| `PAYMENT_WEBHOOK_SECRET` | Wajib, minimal 32 karakter | Secret HMAC-SHA256 untuk webhook pembayaran simulasi `/webhook/payment`. |
| `MIDTRANS_SERVER_KEY` | Opsional untuk fitur pembayaran | Server Key Midtrans Sandbox. Jangan gunakan key produksi untuk pengujian. |
| `FRONTEND_URL` | Wajib disetel sesuai origin frontend | URL tujuan redirect setelah Google OAuth dan URL callback selesai pembayaran. |
| `OAUTH_CLIENT_ID` | Wajib | Identitas Gateway untuk autentikasi antar-service. Bukan Google Client ID. |
| `OAUTH_CLIENT_SECRET` | Wajib, minimal 32 karakter | Secret autentikasi antar-service. Bukan Google Client Secret. |
| `GOOGLE_CLIENT_ID` | Opsional jika login Google diaktifkan | OAuth Client ID tipe Web application dari Google Cloud Console. |
| `GOOGLE_CLIENT_SECRET` | Opsional jika login Google diaktifkan | OAuth Client Secret; hanya disimpan di Campaign Service. |
| `GOOGLE_REDIRECT_URL` | Wajib jika login Google diaktifkan | Callback URL yang harus sama persis dengan Authorized redirect URI di Google Cloud. |
| `GOOGLE_COOKIE_SECURE` | `false` lokal; `true` HTTPS | Mengaktifkan atribut Secure pada cookie state OAuth. |
| `RABBITMQ_USER` | Wajib untuk Compose | Username RabbitMQ lokal. |
| `RABBITMQ_PASSWORD` | Wajib untuk Compose | Password RabbitMQ lokal. |
| `CORS_ALLOWED_ORIGINS` | Opsional sesuai topologi | Daftar origin frontend yang diizinkan. Pada konfigurasi same-origin, isi dengan domain frontend. |

### Variabel internal service

| Variabel | Digunakan oleh | Fungsi |
|---|---|---|
| `PORT` | Semua service | Port HTTP yang didengarkan service. Railway dapat menyediakan nilai `PORT` sendiri untuk service publik. |
| `RABBITMQ_URL` | Campaign dan Notification Service | URI koneksi RabbitMQ. Pada Compose dibentuk otomatis dari `RABBITMQ_USER` dan `RABBITMQ_PASSWORD`. |
| `MTLS_ENABLED` | Gateway dan backend | `true` untuk mTLS pada Docker Compose lokal; `false` untuk koneksi HTTP private network Railway sesuai konfigurasi ini. |
| `TLS_CA_FILE` | Gateway dan backend lokal | Lokasi CA development. |
| `TLS_CERT_FILE` | Gateway dan backend lokal | Lokasi sertifikat client/server development. |
| `TLS_KEY_FILE` | Gateway dan backend lokal | Lokasi private key sertifikat development. |
| `CAMPAIGN_SERVICES` | Gateway | Daftar alamat Campaign Service, dipisahkan koma. |
| `NOTIFICATION_SERVICE` | Gateway | Alamat Notification Service. |
| `GATEWAY_HOST` | Frontend Nginx | Host dan port Gateway yang diteruskan Nginx. |

Jangan membagikan isi `.env`. `.env.example` boleh dibagikan karena hanya berisi nama variabel dan placeholder.

---

## 7. Google OAuth 2.0 / Login dengan Google

### Membuat OAuth Client di Google Cloud Console

1. Buka <https://console.cloud.google.com/> dan pilih project Google Cloud.
2. Buka **Google Auth Platform**. Lengkapi Branding, Audience, dan Test Users sesuai kebutuhan aplikasi.
3. Pada menu **Clients**, pilih **Create Client**.
4. Application type: **Web application**.
5. Tambahkan Authorized JavaScript origins dan Authorized redirect URIs.

**Untuk lokal:**

Authorized JavaScript origins:

```text
http://localhost:5173
```

Authorized redirect URIs:

```text
http://localhost:5173/auth/google/callback
```

**Untuk Railway:**

Authorized JavaScript origins:

```text
https://frontend-production-666ac5dd.up.railway.app
```

Authorized redirect URIs:

```text
https://frontend-production-666ac5dd.up.railway.app/auth/google/callback
```

6. Salin Client ID dan Client Secret. Simpan keduanya sebagai environment variable pada Campaign Service, bukan pada frontend.

### Environment Google OAuth

Lokal (`.env`):

```env
GOOGLE_CLIENT_ID=ISI_CLIENT_ID_DARI_GOOGLE
GOOGLE_CLIENT_SECRET=ISI_CLIENT_SECRET_DARI_GOOGLE
GOOGLE_REDIRECT_URL=http://localhost:5173/auth/google/callback
GOOGLE_COOKIE_SECURE=false
FRONTEND_URL=http://localhost:5173
```

Railway → service `campaign-service` → **Variables**:

```env
GOOGLE_CLIENT_ID=ISI_CLIENT_ID_DARI_GOOGLE
GOOGLE_CLIENT_SECRET=ISI_CLIENT_SECRET_DARI_GOOGLE
GOOGLE_REDIRECT_URL=https://frontend-production-666ac5dd.up.railway.app/auth/google/callback
GOOGLE_COOKIE_SECURE=true
FRONTEND_URL=https://frontend-production-666ac5dd.up.railway.app
```

Setelah menyimpan variable, lakukan redeploy Campaign Service.

### Alur login

1. Frontend membuka `GET /auth/google`.
2. Campaign Service membuat `state` acak dan cookie HttpOnly, lalu mengarahkan pengguna ke Google.
3. Google mengembalikan browser ke `GET /auth/google/callback`.
4. Backend memvalidasi `state`, menukar authorization code, mengambil profil OpenID Connect, dan memeriksa email terverifikasi.
5. Akun Google baru dibuat sebagai `DONATUR`. Jika email sudah ada, identitas Google dapat ditautkan sesuai pemeriksaan backend.
6. Backend menerbitkan JWT aplikasi dan mengembalikan pengguna ke frontend.

Endpoint untuk memulai login adalah `/auth/google`. Jangan membuka `/auth/google/callback` secara manual; callback hanya dipanggil oleh alur redirect dari Google. Jika `/auth/google` menghasilkan 404 di Railway, periksa apakah versi kode terbaru sudah dideploy dan Nginx frontend meneruskan prefix `/auth/` ke Gateway. Jika konfigurasi OAuth belum lengkap, backend mengarahkan kembali ke frontend dengan status login gagal.

---

## 8. API REST dan endpoint publik

Semua endpoint di bawah menggunakan base URL origin frontend. Untuk endpoint yang memerlukan autentikasi, kirim header:

```http
Authorization: Bearer <JWT_ACCESS_TOKEN>
```

| Method | Path | Akses | Keterangan |
|---|---|---|---|
| `GET` | `/health` | Publik | Health check API Gateway. |
| `GET` | `/registry` | Publik | Status health Campaign Service dan Notification Service. |
| `POST` | `/auth/register` | Publik | Registrasi; role selalu `DONATUR`. |
| `POST` | `/auth/login` | Publik | Login email/password dan penerbitan JWT. |
| `GET` | `/auth/google` | Publik | Memulai Google OAuth 2.0. |
| `GET` | `/auth/google/callback` | Callback Google | Memproses authorization code Google. |
| `GET` / `POST` | `/graphql` | Bergantung operasi | GraphQL; GET hanya untuk query, POST untuk query dan mutation. |
| `POST` | `/campaigns` | Login | Membuat campaign dengan foto dan video melalui multipart form. |
| `POST` | `/campaigns/{campaignID}/media` | Pemilik atau ADMIN | Menambahkan media ke campaign yang masih menunggu. |
| `GET` | `/campaigns/{campaignID}/media/{mediaID}` | Publik atau pemilik/ADMIN | Mengambil foto/video campaign sesuai status dan hak akses. |
| `POST` | `/campaigns/{campaignID}/fund-updates` | PENGELOLA pemilik atau ADMIN | Mencatat penggunaan dana; foto bukti opsional. |
| `GET` | `/campaigns/{campaignID}/fund-updates/{updateID}/evidence` | Publik atau pemilik/ADMIN | Mengambil foto bukti penggunaan dana sesuai hak akses. |
| `POST` | `/donations` | DONATUR | Membuat pembayaran Midtrans Snap Sandbox. |
| `GET` | `/donations/mine` | DONATUR | Riwayat donasi dan progres milik pengguna. |
| `GET` | `/donations/pending?campaign_id={id}` | DONATUR | Daftar checkout pending milik pengguna untuk campaign. |
| `GET` | `/donations/{reference}/status` | Pemilik transaksi | Memeriksa status pembayaran. |
| `POST` | `/donations/{reference}/cancel` | Pemilik transaksi | Membatalkan checkout pending. |
| `POST` | `/donations/simulate` | DONATUR | Endpoint simulasi donasi untuk pengujian. |
| `POST` | `/webhook/midtrans` | Signature Midtrans | Notifikasi transaksi Midtrans Sandbox. |
| `POST` | `/webhook/payment` | HMAC `X-Signature` | Webhook pembayaran simulasi/asinkron. |
| `GET` WebSocket | `/ws/donasi` | Token internal dari Gateway | Kanal notifikasi real-time. |

Spesifikasi `frontend/public/openapi.yaml` adalah OpenAPI **3.1.0**. Delapan belas operasi yang diperiksa penguji dipertahankan; dua operasi Google OAuth (`GET /auth/google` dan `GET /auth/google/callback`) ditambahkan pada revisi ini, sehingga versi paket ini mendokumentasikan 20 operasi HTTP. GraphQL GET dan WebSocket dijelaskan terpisah karena karakteristiknya berbeda dari operasi REST biasa. Endpoint `POST /oauth/token` dan `POST /internal/broadcast` merupakan endpoint internal antar-service dan tidak ditujukan untuk browser publik.

### Contoh registrasi

```powershell
$body = @{
  name = 'Donatur Uji'
  email = 'donatur@example.com'
  password = 'DonaturUji123'
} | ConvertTo-Json

Invoke-RestMethod -Method Post `
  -Uri 'http://localhost:5173/auth/register' `
  -ContentType 'application/json' `
  -Body $body
```

### Contoh login

```powershell
$body = @{ email = 'donatur@example.com'; password = 'DonaturUji123' } | ConvertTo-Json
$auth = Invoke-RestMethod -Method Post `
  -Uri 'http://localhost:5173/auth/login' `
  -ContentType 'application/json' `
  -Body $body
$token = $auth.token
```

### Upload campaign

`POST /campaigns` menggunakan `multipart/form-data` dengan field `title`, `description`, `target`, satu sampai lima `photos`, dan satu `video` opsional. Foto harus JPG/PNG/WebP; video harus MP4. Total upload dibatasi sekitar 50 MiB. Campaign baru berstatus `MENUNGGU` sampai disetujui ADMIN.

### Webhook Midtrans

Set `MIDTRANS_SERVER_KEY` dengan Server Key Sandbox dan konfigurasi Notification URL pada dashboard Midtrans Sandbox:

```text
https://frontend-production-666ac5dd.up.railway.app/webhook/midtrans
```

Backend memverifikasi signature notifikasi dan memeriksa status transaksi ke Midtrans sebelum memperbarui data donasi. Jangan menerima nominal atau status pembayaran hanya dari browser.

### Webhook simulasi

`POST /webhook/payment` adalah endpoint simulasi internal. Header `X-Signature` harus berisi HMAC-SHA256 hex dari raw request body menggunakan `PAYMENT_WEBHOOK_SECRET`. Endpoint ini berbeda dari webhook Midtrans.

---

## 9. GraphQL: endpoint, skema final, dan contoh

### Endpoint dan metode HTTP

- Lokal: `http://localhost:5173/graphql`
- Railway: `https://frontend-production-666ac5dd.up.railway.app/graphql`
- `POST /graphql`: digunakan untuk query dan mutation.
- `GET /graphql?query=...`: hanya untuk query read-only.
- Mutation melalui GET akan ditolak dengan HTTP `405 Method Not Allowed`.
- Body POST menggunakan `Content-Type: application/json` dan bentuk `{ "query": "...", "variables": {} }`.
- Operasi yang memerlukan login harus menyertakan `Authorization: Bearer <JWT>`.
- GraphQL dapat mengembalikan HTTP 200 dengan properti `errors`; selalu periksa isi response, bukan hanya status HTTP.

### Skema GraphQL final

Skema berikut mencakup tipe yang terdaftar pada server, termasuk `CampaignQuestion`, `CampaignFundUpdate`, dan `CampaignDonor`.

```graphql
type Query {
  campaigns: [Campaign]
  campaign(id: Int!): Campaign
  pendingCampaigns: [Campaign]
  pendingCampaign(id: Int!): Campaign
}

type Mutation {
  createCampaign(title: String!, description: String, target: Int!): Campaign
  approveCampaign(id: Int!): Campaign
  askCampaignQuestion(campaignID: Int!, question: String!): CampaignQuestion
  answerCampaignQuestion(questionID: Int!, answer: String!): CampaignQuestion
}

type Campaign {
  id: Int
  title: String
  description: String
  target: Int
  collected: Int
  status: String
  submittedByName: String
  submittedByEmail: String
  createdAt: String
  media: [CampaignMedia]
  canManage: Boolean
  canAnswerQuestions: Boolean
  questions: [CampaignQuestion]
  fundUpdates: [CampaignFundUpdate]
  donors: [CampaignDonor]
  donorCount: Int
}

type CampaignMedia {
  id: ID
  mediaType: String
  contentType: String
}

type CampaignQuestion {
  id: Int
  question: String
  askedByName: String
  createdAt: String
  answer: String
  answeredByName: String
  answeredAt: String
}

type CampaignFundUpdate {
  id: Int
  amount: Int
  purpose: String
  description: String
  createdByName: String
  createdAt: String
  hasEvidence: Boolean
  evidenceContentType: String
}

type CampaignDonor {
  name: String
  amount: Int
  createdAt: String
}
```

### Query campaign publik

```graphql
query {
  campaigns {
    id
    title
    description
    target
    collected
    status
    donorCount
  }
}
```

Query `campaigns` hanya menampilkan campaign dengan status `AKTIF` atau `SELESAI`.

### Query detail campaign dan engagement

```graphql
query DetailCampaign($id: Int!) {
  campaign(id: $id) {
    id
    title
    description
    target
    collected
    status
    canManage
    canAnswerQuestions
    donorCount
    media { id mediaType contentType }
    questions {
      id question askedByName createdAt
      answer answeredByName answeredAt
    }
    fundUpdates {
      id amount purpose description createdByName createdAt
      hasEvidence evidenceContentType
    }
    donors { name amount createdAt }
  }
}
```

Variables:

```json
{ "id": 1 }
```

Nama donatur pada field `donors` dan nama penanya pada `askedByName` disamarkan menjadi `Donatur`; data kontak pribadi tidak ditampilkan.

### Query campaign menunggu persetujuan (ADMIN)

```graphql
query {
  pendingCampaigns {
    id title description target collected status
    submittedByName submittedByEmail createdAt
    media { id mediaType contentType }
  }
}
```

### Query satu campaign menunggu (ADMIN)

```graphql
query {
  pendingCampaign(id: 1) {
    id title description target collected status
    submittedByName submittedByEmail createdAt
  }
}
```

Kedua query `pendingCampaigns` dan `pendingCampaign` memerlukan JWT dengan role `ADMIN`.

### Mutation membuat campaign (pengguna login)

```graphql
mutation {
  createCampaign(
    title: "Bantuan Banjir"
    description: "Bantuan kebutuhan warga terdampak banjir."
    target: 25000000
  ) {
    id title status target
  }
}
```

Campaign yang dibuat berstatus `MENUNGGU`.

### Mutation menyetujui campaign (ADMIN)

```graphql
mutation {
  approveCampaign(id: 1) {
    id title status
  }
}
```

### Mutation bertanya tentang campaign (DONATUR)

```graphql
mutation {
  askCampaignQuestion(
    campaignID: 1
    question: "Bagaimana rencana penggunaan dana campaign ini?"
  ) {
    id question askedByName createdAt
  }
}
```

Pertanyaan harus 10–2.000 karakter dan campaign harus aktif.

### Mutation menjawab pertanyaan (pemilik campaign atau ADMIN)

```graphql
mutation {
  answerCampaignQuestion(
    questionID: 1
    answer: "Dana akan digunakan untuk kebutuhan yang dijelaskan dalam campaign."
  ) {
    id answer answeredByName answeredAt
  }
}
```

Pertanyaan hanya dapat dijawab satu kali. Pengguna harus merupakan pemilik campaign atau ADMIN.

### Catatan penggunaan dana

Pencatatan penggunaan dana dilakukan melalui REST `POST /campaigns/{campaignID}/fund-updates`, bukan mutation GraphQL. Request menggunakan `multipart/form-data` dengan field `amount`, `purpose`, `description`, dan `evidence` (foto opsional). Query GraphQL menampilkan hasilnya melalui `campaign { fundUpdates { ... } }`.

### Tes GraphQL melalui PowerShell

Lokal:

```powershell
$body = @{ query = '{ campaigns { id title status } }' } | ConvertTo-Json -Compress
Invoke-RestMethod -Method Post `
  -Uri 'http://localhost:5173/graphql' `
  -ContentType 'application/json' `
  -Body $body
```

Railway:

```powershell
$body = @{ query = '{ campaigns { id title status } }' } | ConvertTo-Json -Compress
Invoke-RestMethod -Method Post `
  -Uri 'https://frontend-production-666ac5dd.up.railway.app/graphql' `
  -ContentType 'application/json' `
  -Body $body
```

Jika database belum memiliki campaign aktif, hasil `campaigns` berupa array kosong adalah respons yang valid. Pastikan tidak ada `errors` pada hasil GraphQL.

---

## 10. OpenAPI dan Swagger UI

File spesifikasi berada di:

```text
frontend/public/openapi.yaml
```

Versi spesifikasi: **OpenAPI 3.1.0**. File ini mempertahankan 18 operasi yang diperiksa penguji dan menambahkan dua endpoint Google OAuth, sehingga paket revisi ini mendokumentasikan 20 operasi HTTP. File memuat skema request/response REST, bearer JWT, dan contoh request GraphQL POST.

URL yang harus dapat diakses setelah frontend dideploy:

- Swagger UI: `/docs/`
- YAML mentah: `/openapi.yaml`

Nginx harus menyajikan file tersebut sebagai static assets, bukan mengembalikan `index.html` frontend. Konfigurasi Nginx pada paket ini memiliki route khusus untuk `/docs/` dan `/openapi.yaml`. Jika URL tersebut masih mengembalikan halaman frontend atau HTTP 404 setelah deploy, pastikan deployment menggunakan versi repository terbaru dan folder `frontend/public/` ikut masuk ke image frontend.

Tes sederhana:

```powershell
Invoke-WebRequest 'https://frontend-production-666ac5dd.up.railway.app/docs/' -Method Get
Invoke-WebRequest 'https://frontend-production-666ac5dd.up.railway.app/openapi.yaml' -Method Get
```

Untuk `/docs/`, respons harus berupa halaman HTML Swagger UI. Untuk `/openapi.yaml`, respons harus berisi teks YAML yang dimulai dengan `openapi: 3.1.0`, bukan markup HTML frontend.

---

## 11. WebSocket dan notifikasi real-time

Endpoint WebSocket:

- Lokal: `ws://localhost:5173/ws/donasi`
- Railway: `wss://frontend-production-666ac5dd.up.railway.app/ws/donasi`

Frontend terhubung melalui Nginx dan Gateway ke Notification Service. Notification Service menerima event dari RabbitMQ dan membroadcast JSON ke client yang terhubung.

Event yang digunakan:

| Event | Isi umum |
|---|---|
| `CAMPAIGN_CREATED` | Informasi campaign baru. |
| `CAMPAIGN_APPROVED` | Informasi campaign yang disetujui. |
| `NEW_DONATION` | `campaign_id`, `donor_name`, dan `amount`. |

Catatan implementasi: broadcast dikirim ke seluruh client WebSocket yang terhubung, bukan hanya client pada satu campaign tertentu. Implementasi saat ini belum menyediakan mekanisme reconnect otomatis, replay pesan yang terlewat, atau polling fallback. Karena itu, hal tersebut perlu diperhitungkan dalam pengujian.

---

## 12. Deployment Railway

Aplikasi menggunakan satu repository dengan beberapa service Railway. Semua service harus berada dalam project dan environment Railway yang sama. Buat domain publik hanya untuk frontend; Gateway dan backend sebaiknya tetap private.

### Service dan root directory

| Service Railway | Root Directory / Image | Akses | Port internal |
|---|---|---|---:|
| `frontend` | `/frontend` | Public domain | Mengikuti `PORT` Railway |
| `gateway` | `/gateway` | Private | `8000` |
| `campaign-service` | `/campaign-service` | Private | `8443` |
| `notification-service` | `/notification-service` | Private | `8443` |
| `Postgres` | Plugin PostgreSQL atau database eksternal | Private | Sesuai provider |
| `rabbitmq` | Image `rabbitmq:3-management-alpine` | Private | `5672` |

### Urutan deployment

1. Hubungkan repository GitHub ke project Railway.
2. Buat service untuk frontend, gateway, campaign-service, dan notification-service dengan Root Directory sesuai tabel.
3. Tambahkan PostgreSQL Railway atau gunakan database PostgreSQL eksternal yang memang disiapkan untuk aplikasi. Set `DATABASE_URL` pada Campaign Service.
4. Tambahkan RabbitMQ private service. Atur username/password dan volume persisten untuk `/var/lib/rabbitmq`.
5. Isi variables untuk masing-masing service seperti tabel berikut.
6. Pastikan service backend dapat berkomunikasi melalui private domain Railway.
7. Deploy frontend, lalu buat/aktifkan public domain.
8. Isi Google OAuth redirect URI sesuai domain frontend, lalu redeploy Campaign Service.
9. Tes `/health`, `/registry`, `/graphql`, `/docs/`, dan `/openapi.yaml`.

### Variabel per service Railway

Nama referensi `${{Service.VARIABLE}}` harus disesuaikan dengan nama service dan nama variable yang tampil pada project Railway milik deployment.

| Service | Variables utama |
|---|---|
| `campaign-service` | `PORT=8443`; `MTLS_ENABLED=false`; `DATABASE_URL=${{Postgres.DATABASE_URL}}`; `JWT_SECRET`; `PAYMENT_WEBHOOK_SECRET`; `OAUTH_CLIENT_ID=crowdfunding-gateway`; `OAUTH_CLIENT_SECRET`; `FRONTEND_URL=https://frontend-production-666ac5dd.up.railway.app`; `GOOGLE_CLIENT_ID`; `GOOGLE_CLIENT_SECRET`; `GOOGLE_REDIRECT_URL=https://frontend-production-666ac5dd.up.railway.app/auth/google/callback`; `GOOGLE_COOKIE_SECURE=true`; `RABBITMQ_URL`; `MIDTRANS_SERVER_KEY` jika pembayaran Sandbox digunakan. |
| `notification-service` | `PORT=8443`; `MTLS_ENABLED=false`; `JWT_SECRET` sama dengan Campaign Service; `OAUTH_CLIENT_ID=crowdfunding-gateway`; `RABBITMQ_URL`; `CORS_ALLOWED_ORIGINS=https://frontend-production-666ac5dd.up.railway.app`. |
| `gateway` | `PORT=8000`; `MTLS_ENABLED=false`; `OAUTH_CLIENT_ID=crowdfunding-gateway`; `OAUTH_CLIENT_SECRET` sama dengan Campaign Service; `CAMPAIGN_SERVICES=http://${{campaign-service.RAILWAY_PRIVATE_DOMAIN}}:8443`; `NOTIFICATION_SERVICE=http://${{notification-service.RAILWAY_PRIVATE_DOMAIN}}:8443`; `CORS_ALLOWED_ORIGINS=https://frontend-production-666ac5dd.up.railway.app`. |
| `frontend` | `GATEWAY_HOST=${{gateway.RAILWAY_PRIVATE_DOMAIN}}:8000`; Railway menyediakan `PORT`. |
| `rabbitmq` | `RABBITMQ_DEFAULT_USER`; `RABBITMQ_DEFAULT_PASS`; volume persisten pada `/var/lib/rabbitmq`. |

Jika nama service database bukan `Postgres`, sesuaikan referensi `DATABASE_URL` dengan nama service yang sebenarnya. Jika memakai Neon atau database eksternal, gunakan connection string lengkap yang valid dan SSL sesuai persyaratan provider.

Gunakan nilai acak untuk `JWT_SECRET`, `OAUTH_CLIENT_SECRET`, `PAYMENT_WEBHOOK_SECRET`, dan password RabbitMQ. Untuk URI RabbitMQ, password alfanumerik menghindari masalah karakter khusus pada URL. Jangan mengisi variable Railway dengan isi file `.env` secara mentah; masukkan setiap variable pada service yang membutuhkannya.

### Google OAuth di Railway

Google Cloud Console harus memuat:

- Authorized JavaScript origin: `https://frontend-production-666ac5dd.up.railway.app`
- Authorized redirect URI: `https://frontend-production-666ac5dd.up.railway.app/auth/google/callback`

Nilai `GOOGLE_REDIRECT_URL` di Campaign Service harus sama persis dengan Authorized redirect URI.

### Midtrans Sandbox di Railway

Jika pembayaran diaktifkan, isi `MIDTRANS_SERVER_KEY` pada Campaign Service menggunakan Server Key Sandbox dan atur Notification URL Midtrans ke:

```text
https://frontend-production-666ac5dd.up.railway.app/webhook/midtrans
```

Gunakan akun dan key Sandbox untuk pengujian. Jangan melakukan transaksi sungguhan sebelum konfigurasi produksi dan prosedur verifikasi diselesaikan.

---

## 13. Database, migration, dan data pengujian

### Skema database

`db/init.sql` digunakan untuk inisialisasi database PostgreSQL baru. Migration yang tersedia:

| File | Tujuan |
|---|---|
| `002_campaign_media.sql` | Tabel media campaign. |
| `003_donation_contact.sql` | Kolom kontak donatur. |
| `004_campaign_engagement.sql` | Tabel pertanyaan campaign dan penggunaan dana. |
| `005_midtrans_payments.sql` | Kolom pembayaran dan identitas donatur pada transaksi. |
| `006_google_oauth.sql` | Kolom `google_sub` dan unique index untuk identitas Google. |

Campaign Service juga memastikan beberapa tabel/kolom pendukung tersedia saat startup. Untuk database baru, gunakan `db/init.sql`. Untuk database lama, jalankan migration yang belum diterapkan secara berurutan setelah backup. Jangan mengulang migration yang tidak idempotent pada database yang telah berisi data tanpa memeriksa kondisinya.

### Reset data pengujian

- Gunakan database sementara/disposable untuk seed dan reset data pengujian.
- Skrip seed/reset dari paket pengujian hanya boleh dijalankan pada database sementara yang memang disediakan untuk pengujian.
- Jangan menjalankan `TRUNCATE`, reset test data, atau `docker compose down -v` pada database Railway atau database yang berisi data pengguna.
- Jika memakai PostgreSQL Railway atau Neon, perintah `docker compose exec db ...` tidak akan mengarah ke database eksternal tersebut.
- Formulir penguji mencatat akun uji yang dibuat pada database Compose lokal: `smoke-20261116-164141@example.test`, `admin-smoke-20261116-170650@example.test`, `admin-smoke-20261116-170803@example.test`, serta dua akun bernama `API Check` dan `JWT Check` (alamat email kedua akun ini tidak dicantumkan). Penguji meminta akun-akun tersebut dibersihkan setelah pengujian.
- Sebelum menghapus, periksa hanya database Compose lokal dan cocokkan identitas akun. Contoh pemeriksaan: `docker compose exec db psql -U crowdfunding -d crowdfunding -c "SELECT id,name,email,role FROM users WHERE email IN ('smoke-20261116-164141@example.test','admin-smoke-20261116-170650@example.test','admin-smoke-20261116-170803@example.test') OR name IN ('API Check','JWT Check');"`. Hapus hanya baris yang sudah dipastikan merupakan akun uji penguji; jangan jalankan query ini pada database Railway/produksi.

---

## 14. Pengujian dan hasil pemeriksaan serah-terima

Berdasarkan Formulir Serah-Terima Code Freeze tanggal 16 November 2026 dari Kelompok A:

### Pemeriksaan yang dinyatakan lulus

- Frontend Railway merespons HTTP 200.
- `/health`, `/registry`, dan POST `/graphql` merespons melalui domain Railway.
- Contoh endpoint pada README berhasil dijalankan oleh penguji.
- Pengujian lokal JWT DONATUR, penolakan akses tanpa token, pembatasan role ADMIN, dan validasi HMAC webhook simulasi berhasil.
- Skrip seed/reset pada database PostgreSQL sementara dinyatakan idempoten dan dapat dijalankan kembali setelah reset.

### Catatan perbaikan dokumentasi

- Skema GraphQL dan contoh operasi harus mencantumkan `CampaignQuestion`, `CampaignFundUpdate`, `CampaignDonor`, `pendingCampaigns`, `askCampaignQuestion`, dan `answerCampaignQuestion`.
- URL deployment publik harus ditulis secara eksplisit.
- README tidak boleh mencantumkan password/kredensial admin.
- `/docs/` harus menyajikan Swagger UI dan `/openapi.yaml` harus menyajikan file YAML, bukan halaman SPA frontend.
- Dokumentasi WebSocket dan batasan implementasinya perlu jelas.
- Cara reset database eksternal/Railway tidak boleh disamakan dengan reset database lokal.

### Pengujian yang belum termasuk dalam smoke test penguji

Checkout Midtrans, `/donations/simulate`, `approveCampaign`, uji beban, pengujian mendalam TLS/mTLS, serta login/registrasi dan webhook pada Railway belum dinyatakan teruji oleh penguji. Jangan menuliskan fitur-fitur tersebut sebagai telah lulus pengujian jika belum diuji kembali.

---

## 15. Keamanan dan pengelolaan Git

- `.env` tidak boleh masuk repository. Gunakan `.env.example` sebagai template.
- Jika `.env` pernah masuk commit publik, anggap seluruh secret di dalamnya telah terekspos. Rotasi password PostgreSQL, RabbitMQ, `JWT_SECRET`, `PAYMENT_WEBHOOK_SECRET`, `OAUTH_CLIENT_SECRET`, dan `MIDTRANS_SERVER_KEY` yang pernah aktif.
- Setelah rotasi, bersihkan `.env` dari seluruh Git history menggunakan `git-filter-repo` pada clone yang telah di-backup. Penulisan ulang history memerlukan force-push dan koordinasi dengan kolaborator.
- Jangan force-push sebelum memeriksa remote, branch, tag, dan backup repository.
- Jangan memasukkan Client Secret Google atau token ke frontend, screenshot, dokumentasi publik, atau commit.
- Jangan memberikan kredensial admin produksi kepada penguji. Buat akun uji khusus dengan izin minimum dan cabut setelah pengujian.
- `frontend/node_modules/`, `frontend/dist/`, `certs/`, dan file `.env` harus tetap diabaikan Git.

### Pemeriksaan sebelum commit

```powershell
git status --short
git ls-files .env
git check-ignore -v .env
git diff --cached --check
```

`git ls-files .env` harus kosong. `git check-ignore -v .env` harus menunjukkan aturan `.gitignore`.

### Pembersihan `.env` dari riwayat Git

Lakukan hanya setelah backup clone dan rotasi secret. Dari root repository:

```powershell
py -m pip install git-filter-repo
& "$env:LOCALAPPDATA\Python\pythoncore-3.14-64\Scripts\git-filter-repo.exe" --path .env --invert-paths --force
```

Setelah proses, periksa `git remote -v`. Jika `origin` dilepas oleh `git-filter-repo`, tambahkan kembali:

```powershell
git remote add origin https://github.com/Litaa2407/uas_paknanang.git
```

Verifikasi sebelum push:

```powershell
git log --all -- .env
git ls-files .env
git check-ignore -v .env
```

Dua perintah pertama seharusnya tidak menunjukkan file `.env` atau riwayatnya. Setelah memastikan backup, branch, dan tag sudah benar, barulah riwayat yang telah dibersihkan dapat dikirim ke remote dengan prosedur force-push yang disepakati tim. Semua kolaborator perlu membuat clone baru setelah riwayat ditulis ulang.

---

## 16. Batasan implementasi yang perlu diketahui

- GraphQL membatasi kompleksitas query secara sederhana melalui jumlah kurung kurawal; ini bukan pengganti query-cost analyzer.
- WebSocket membroadcast event ke semua client yang tersambung; belum ada filter per campaign, reconnect otomatis, atau replay pesan.
- Endpoint `/donations/simulate` dan `/webhook/payment` adalah jalur simulasi, berbeda dari integrasi Midtrans Sandbox.
- Pengujian pembayaran Midtrans bergantung pada konfigurasi Sandbox dan callback provider.
- Sertifikat mTLS yang dibuat script hanya untuk development lokal; jangan digunakan sebagai sertifikat produksi.
- Untuk produksi, gunakan secret manager, sertifikat/CA tepercaya, rate limiting, audit log, pemantauan, backup database, dan pengujian keamanan tambahan.

---

## 17. Referensi teknis

- Google Cloud Console: <https://console.cloud.google.com/>
- Dokumentasi Google OAuth: <https://developers.google.com/identity/protocols/oauth2>
- Railway Deployments: <https://docs.railway.com/deployments/monorepo>
- Railway Private Networking: <https://docs.railway.com/networking/private-networking>
- Railway Variables: <https://docs.railway.com/variables/reference>
- OpenAPI Specification: <https://spec.openapis.org/oas/v3.1.0>
- Midtrans Sandbox: <https://dashboard.sandbox.midtrans.com/>

---

**Dokumentasi utama proyek Donasi Bersama — Kelompok E.**
