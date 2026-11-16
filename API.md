# API Documentation — Platform Crowdfunding dan Donasi

**Versi:** Code Freeze Week 12  
**Base URL lokal:** `http://localhost:8000` (API Gateway)  
**Format:** JSON untuk HTTP; GraphQL untuk data campaign; WebSocket untuk notifikasi.

Spesifikasi OpenAPI tersedia di [`frontend/public/openapi.yaml`](./frontend/public/openapi.yaml). Swagger UI interaktif disajikan frontend pada `/docs/` setelah frontend dibangun dan dideploy.

Semua request browser diarahkan melalui API Gateway. Campaign Service dan Notification Service tidak mempublikasikan port ke host. Gateway berkomunikasi dengan keduanya menggunakan mTLS dan OAuth2 Client Credentials.

## 1. Ringkasan endpoint publik

| Method | Path | Auth pengguna | Deskripsi |
|---|---|---|---|
| GET | `/health` | Tidak | Health check Gateway |
| GET | `/registry` | Tidak | Status health service backend |
| POST | `/auth/register` | Tidak | Registrasi akun DONATUR |
| POST | `/auth/login` | Tidak | Login dan penerbitan JWT pengguna |
| POST | `/graphql` | Bergantung operasi | Query/mutation campaign |
| POST | `/campaigns` | Wajib | Mengajukan campaign dengan foto dan video (multipart legacy) |
| POST | `/campaigns/{campaignID}/media` | Wajib | Menambahkan foto/video ke campaign yang masih menunggu persetujuan |
| GET | `/campaigns/{campaignID}/media/{mediaID}` | Opsional | Mengambil media campaign aktif atau selesai |
| POST | `/campaigns/{campaignID}/fund-updates` | PENGELOLA pemilik campaign atau ADMIN | Mencatat penggunaan dana, dengan foto bukti opsional |
| GET | `/campaigns/{campaignID}/fund-updates/{updateID}/evidence` | Opsional | Mengambil foto bukti update campaign aktif atau selesai |
| POST | `/donations` | DONATUR | Membuat pembayaran Midtrans Snap Sandbox |
| GET | `/donations/mine` | DONATUR | Melihat riwayat donasi dan progres campaign milik sendiri |
| GET | `/donations/pending?campaign_id={id}` | DONATUR | Mengambil checkout pending milik donatur untuk dilanjutkan |
| POST | `/donations/{reference}/cancel` | Pemilik transaksi | Membatalkan checkout pending di Midtrans |
| GET | `/donations/{reference}/status` | Pemilik transaksi | Memeriksa dan menyinkronkan status pembayaran dari Midtrans |
| POST | `/donations/simulate` | DONATUR | Mencatat donasi simulasi |
| POST | `/webhook/midtrans` | Signature Midtrans | Notifikasi transaksi Midtrans |
| POST | `/webhook/payment` | HMAC signature | Notifikasi pembayaran simulasi |
| GET | `/ws/donasi` | Internal Gateway token | WebSocket notifikasi real-time |

Header `X-Internal-Token` tidak boleh dikirim atau dibuat oleh browser. Header tersebut dibuat oleh Gateway setelah memperoleh OAuth2 access token dan hanya digunakan pada jaringan internal.

## 2. Health check dan service registry

### `GET /health`

Memeriksa proses Gateway.

Response `200 OK`:

```json
{
  "status": "ok",
  "service": "gateway"
}
```

### `GET /registry`

Menampilkan status health Campaign Service dan Notification Service. Health check dilakukan Gateway setiap lima detik.

Contoh response:

```json
{
  "campaign": {
    "https://campaign-service:8443": true,
    "https://campaign-service-2:8443": true
  },
  "notification": {
    "https://notification-service:8443": true
  }
}
```

Nilai `true` berarti endpoint `/health` service merespons HTTP 200. Gateway hanya memilih instance Campaign Service yang sehat.

## 3. Registrasi

### `POST /auth/register`

Membuat akun pengguna baru. Role dari request tidak digunakan; seluruh registrasi publik selalu menjadi `DONATUR`.

Request:

```json
{
  "name": "Donatur Uji",
  "email": "donatur@example.com",
  "password": "DonaturUji123"
}
```

Validasi utama:

- Nama 2–100 karakter.
- Email valid dan maksimal 150 karakter.
- Password 10–72 karakter (batas maksimum mengikuti bcrypt).
- Role tidak dapat dinaikkan melalui payload registrasi.

Response `200 OK`:

```json
{
  "token": "<JWT_ACCESS_TOKEN>",
  "token_type": "Bearer",
  "expires_in": 43200,
  "user": {
    "id": 1,
    "name": "Donatur Uji",
    "email": "donatur@example.com",
    "role": "DONATUR"
  }
}
```

Status error umum: `400 Bad Request` untuk data tidak valid, `409 Conflict` untuk email duplikat atau data yang ditolak database.

## 4. Login

### `POST /auth/login`

Request:

```json
{
  "email": "donatur@example.com",
  "password": "DonaturUji123"
}
```

Response sukses memiliki bentuk yang sama dengan registrasi. JWT pengguna berlaku 12 jam dan ditandatangani menggunakan HS256. Frontend mengakhiri sesi dan meminta login kembali saat JWT kedaluwarsa, termasuk ketika aplikasi dibuka kembali setelah masa berlaku token habis. Password disimpan sebagai bcrypt hash.

Status error umum: `400 Bad Request` untuk JSON tidak valid dan `401 Unauthorized` untuk kredensial yang salah.

Untuk operasi GraphQL yang membutuhkan autentikasi, kirim:

```http
Authorization: Bearer <JWT_ACCESS_TOKEN>
```

## 5. GraphQL

### `POST /graphql`

Content-Type: `application/json`

Request body:

```json
{
  "query": "query { campaigns { id title description target collected status } }"
}
```

### Query: campaign publik

```graphql
query {
  campaigns {
    id
    title
    description
    target
    collected
    status
  }
}
```

Hanya campaign dengan status `AKTIF` yang ditampilkan.

### Query: satu campaign publik

```graphql
query {
  campaign(id: 1) {
    id
    title
    description
    target
    collected
    status
  }
}
```

Mengembalikan `null` jika campaign tidak ditemukan atau belum aktif.

Detail campaign juga menyediakan `questions`, `fundUpdates`, `donors`, `donorCount`, dan `canManage`. Daftar donatur tidak membuka nama maupun kontak; setiap entri ditampilkan sebagai “Donatur”.

```graphql
query {
  campaign(id: 1) {
    canManage
    donorCount
    questions { id question askedByName createdAt answer answeredByName answeredAt }
    fundUpdates { id amount purpose description createdByName createdAt hasEvidence evidenceContentType }
    donors { name amount createdAt }
  }
}
```

### Mutation: bertanya tentang campaign (DONATUR)

```graphql
mutation {
  askCampaignQuestion(campaignID: 1, question: "Bagaimana rencana penggunaan dana campaign ini?") {
    id
    question
    createdAt
  }
}
```

Wajib JWT role `DONATUR`; pertanyaan berisi 10–2.000 karakter. Nama dan kontak penanya tidak dipublikasikan.

### Mutation: menjawab pertanyaan (PENGELOLA pemilik campaign atau ADMIN)

```graphql
mutation {
  answerCampaignQuestion(questionID: 1, answer: "Dana akan digunakan untuk kebutuhan yang dijelaskan dalam campaign.") {
    id
    answer
    answeredByName
    answeredAt
  }
}
```

Setiap pertanyaan hanya dapat dijawab satu kali.

### Mencatat penggunaan dana (PENGELOLA pemilik campaign atau ADMIN)

Kirim `multipart/form-data` ke `POST /campaigns/{campaignID}/fund-updates` dengan JWT pengguna. Field `amount` berupa bilangan positif; `purpose` berisi 3–120 karakter dan `description` 5–2.000 karakter. Field `evidence` opsional berupa satu foto JPG, PNG, atau WebP, maksimal 5 MiB. Total request maksimal 6 MiB. Nominal update tidak boleh melebihi dana terkumpul yang belum dilaporkan penggunaannya. Update hanya dapat ditambahkan ke campaign aktif.

### Query: campaign menunggu persetujuan (ADMIN)

```graphql
query {
  pendingCampaigns {
    id
    title
    description
    target
    collected
    status
    submittedByName
    submittedByEmail
    createdAt
  }
}
```

Wajib menyertakan JWT pengguna dengan role `ADMIN`.

### Query: detail campaign menunggu persetujuan (ADMIN)

```graphql
query {
  pendingCampaign(id: 1) {
    id
    title
    description
    target
    collected
    status
    submittedByName
    submittedByEmail
    createdAt
    media { id mediaType contentType }
  }
}
```

Query ini hanya mengembalikan campaign berstatus `MENUNGGU` dan hanya dapat diakses oleh ADMIN. Halaman detail menampilkan cerita lengkap, media, identitas pengaju, waktu pengajuan, dan target donasi.

### Mutation: membuat campaign (pengguna login)

Sesuai aturan proyek saat ini, semua pengguna yang sudah login dapat mengajukan campaign. Campaign baru disimpan dengan status `MENUNGGU`.

```graphql
mutation {
  createCampaign(
    title: "Bantuan Banjir"
    description: "Bantuan untuk warga terdampak"
    target: 25000000
  ) {
    id
    title
    status
  }
}
```

Header:

```http
Authorization: Bearer <JWT_ACCESS_TOKEN>
```

Frontend mengirim mutation GraphQL ini terlebih dahulu, lalu mengunggah media ke `POST /campaigns/{campaignID}/media` menggunakan `multipart/form-data` dan JWT yang sama. Pilih minimal satu foto; maksimal lima foto (JPG, PNG, atau WebP, masing-masing maksimal 5 MB) dan satu video MP4 opsional (maksimal 25 MB). Total request maksimal 50 MB. Media hanya dapat ditambahkan oleh pemilik campaign atau ADMIN selama status campaign masih `MENUNGGU`.

### Mutation: menyetujui campaign (ADMIN)

```graphql
mutation {
  approveCampaign(id: 1) {
    id
    title
    status
  }
}
```

Wajib JWT role `ADMIN`. Campaign berubah dari `MENUNGGU` menjadi `AKTIF`.

### Catatan GraphQL

- GraphQL mengembalikan HTTP `200` untuk hasil eksekusi yang dapat berisi properti `errors`; client tetap harus memeriksa `errors`.
- Body request dibatasi maksimal 1 MiB.
- Pemeriksaan kompleksitas dasar menolak query dengan lebih dari 12 pembuka selection set.
- Query `campaigns` dan `campaign` bersifat publik; `pendingCampaigns` dan mutation memiliki pemeriksaan autentikasi/role sesuai aturan di atas.
- Tabel pertanyaan dan update penggunaan dana disiapkan otomatis secara idempoten saat Campaign Service mulai. SQL yang sama tersedia di `db/migrations/004_campaign_engagement.sql`.

## 6. Pengajuan campaign dengan media

### `POST /campaigns`

Frontend mengirim `multipart/form-data` dengan JWT pengguna:

```http
Authorization: Bearer <JWT_ACCESS_TOKEN>
Content-Type: multipart/form-data
```

Field form:

| Field | Bentuk | Aturan |
|---|---|---|
| `title` | teks | 3–100 karakter |
| `description` | teks | Maksimal 10.000 karakter |
| `target` | angka | Bilangan positif |
| `photos` | file berulang | Wajib 1–5 file JPG, PNG, atau WebP; maksimal 5 MiB per file |
| `video` | file opsional | Maksimal 1 file MP4; maksimal 25 MiB |

Total upload maksimal 50 MiB. Campaign baru berstatus `MENUNGGU`. Response `201 Created` berisi object campaign (`id`, `title`, `description`, `target`, `collected`, `status`). Event `CAMPAIGN_CREATED` diterbitkan melalui transactional outbox.

### `GET /campaigns/{campaignID}/media/{mediaID}`

Media campaign `AKTIF` dapat diakses publik dan dikirim inline dengan Content-Type yang tersimpan. Media campaign yang belum aktif hanya tersedia bagi pemilik atau ADMIN dengan JWT; selain itu endpoint mengembalikan `404`.

## 7. Pembayaran donasi

### `POST /donations`

Membuat transaksi Midtrans Snap Sandbox. Hanya akun `DONATUR` yang dapat membuat pembayaran. Server Key disimpan di Campaign Service melalui `MIDTRANS_SERVER_KEY` dan tidak pernah dikirim ke browser.

Request memakai JWT dan format yang sama dengan endpoint simulasi:

```json
{
  "campaign_id": 1,
  "amount": 25000,
  "donor_name": "Donatur Uji",
  "donor_email": "donatur@example.com",
  "donor_phone": "+6281234567890"
}
```

Nominal minimum Rp 1.000; campaign harus berstatus `AKTIF`. Transaksi dicatat sebagai `PENDING` sebelum request ke Midtrans. Response `201 Created` berisi referensi dan `redirect_url` Snap Sandbox. Frontend mengarahkan donatur ke halaman pembayaran Midtrans; Snap menyediakan metode pembayaran yang aktif pada akun merchant, termasuk QRIS dan Virtual Account.

```json
{
  "status": "PENDING",
  "reference": "DON-<random>",
  "redirect_url": "https://app.sandbox.midtrans.com/snap/v2/..."
}
```

### `GET /donations/{reference}/status`

Hanya pemilik transaksi yang terautentikasi sebagai `DONATUR` yang dapat memeriksa status. Campaign Service mengambil status terbaru dari Midtrans Sandbox dan memvalidasi order serta nominal sebelum memperbarui database. Progres campaign bertambah satu kali saja setelah pembayaran terkonfirmasi. Browser tidak dapat menetapkan status transaksi sendiri.

### `GET /donations/pending?campaign_id={id}`

Mengembalikan maksimal lima checkout berstatus `PENDING` milik donatur yang sedang login untuk campaign tersebut. Endpoint tidak mengembalikan transaksi pengguna lain. Frontend menampilkan tombol **Lanjutkan pembayaran** yang membuka kembali Snap checkout menggunakan tautan transaksi yang sama. Tautan disimpan hanya di backend dan hanya dikembalikan ke pemilik pembayaran.

### `GET /donations/mine`

Mengembalikan riwayat transaksi milik Donatur yang sedang login beserta nominal, status, tanggal, dan progres terbaru campaign (dana terkumpul dan target). Endpoint ini hanya membaca data transaksi yang terikat dengan ID pengguna dari token.

### `POST /donations/{reference}/cancel`

Hanya pemilik transaksi yang terautentikasi sebagai `DONATUR` yang dapat membatalkan pembayaran. Campaign Service memeriksa status transaksi ke Midtrans, meminta pembatalan untuk transaksi yang masih `PENDING`, lalu memverifikasi statusnya kembali sebelum menandainya `FAILED`. Jika pembayaran sudah berhasil, status `SETTLEMENT` tetap dipertahankan dan donasi tetap tercatat. Pembatalan tidak menghapus riwayat transaksi.

### `POST /webhook/midtrans`

Endpoint notifikasi Midtrans. Validasi `signature_key` menggunakan SHA-512 atas gabungan `order_id + status_code + gross_amount + MIDTRANS_SERVER_KEY`, kemudian Campaign Service mengambil status langsung dari API Midtrans sebelum mengubah transaksi. Pada pengembangan lokal, endpoint ini tidak dapat menerima notifikasi internet tanpa URL publik; halaman kembali tetap memeriksa status lewat API status server.

Gunakan Sandbox Server Key dari dashboard Midtrans. Tambahkan nilainya secara lokal ke `.env` sebagai `MIDTRANS_SERVER_KEY` (jangan commit atau kirim key ke chat), lalu rebuild service:

```powershell
docker compose up -d --build --force-recreate campaign-service campaign-service-2 frontend gateway
```

Jangan gunakan Server Key Sandbox untuk pembayaran uang sungguhan. Integrasi ini diarahkan ke endpoint Sandbox.

### `POST /donations/simulate`

Hanya akun `DONATUR` yang dapat mencatat dukungan simulasi. Kirim JWT pengguna:

```http
Authorization: Bearer <JWT_ACCESS_TOKEN>
Content-Type: application/json
```

Request:

```json
{
  "campaign_id": 1,
  "amount": 25000,
  "donor_name": "Donatur Uji",
  "donor_email": "donatur@example.com",
  "donor_phone": "+6281234567890"
}
```

Campaign harus berstatus `AKTIF`; nominal minimum Rp 1.000. Ketika dana terkumpul mencapai target, status campaign berubah menjadi `SELESAI` dan campaign tidak menerima donasi baru. Nama, email, dan nomor telepon divalidasi dan disimpan bersama catatan donasi, tetapi tidak disebarkan pada event publik. Response `200 OK` berisi `{"status":"simulated","reference":"SIM-..."}`. Progres campaign dan event `SIMULATED_DONATION` dicatat secara transaksional.

## 8. Webhook simulasi

### `POST /webhook/payment`

Endpoint simulasi untuk menerima notifikasi pembayaran. Request harus memiliki header `X-Signature`, yaitu HMAC-SHA256 hex dari raw request body menggunakan `PAYMENT_WEBHOOK_SECRET`.

Request:

```json
{
  "campaign_id": 1,
  "donor_name": "Anonim",
  "amount": 50000,
  "reference": "PAY-2026-00001",
  "status": "SETTLEMENT"
}
```

Status yang diterima: `PENDING`, `SETTLEMENT`, atau `FAILED`. `reference` harus unik. Jika status `SETTLEMENT`, nilai `campaigns.collected` ditambahkan dalam transaksi database yang sama dengan pencatatan donasi dan outbox event.

Contoh membuat signature untuk body yang disimpan pada `payload.json`:

```bash
openssl dgst -sha256 -hmac "$PAYMENT_WEBHOOK_SECRET" payload.json
```

Gunakan nilai hex setelah teks `=` sebagai header `X-Signature`.

Response sukses `202 Accepted`:

```json
{
  "status": "accepted",
  "message": "event akan diproses secara asinkron"
}
```

Status error umum: `400 Bad Request` untuk payload/signature tidak valid, `401 Unauthorized` untuk HMAC tidak valid, `404 Not Found` jika campaign settlement tidak ditemukan, dan `409 Conflict` untuk reference duplikat atau data yang melanggar constraint.

Endpoint ini hanya untuk simulasi pengujian internal; pembayaran Midtrans memakai `POST /webhook/midtrans`.

## 9. WebSocket notifikasi

### `GET /ws/donasi`

URL browser lokal:

```javascript
const socketURL = new URL('/ws/donasi', window.location.href);
socketURL.protocol = socketURL.protocol === 'https:' ? 'wss:' : 'ws:';
const socket = new WebSocket(socketURL);
socket.onmessage = (message) => {
  const event = JSON.parse(message.data);
  console.log(event);
};
```

Origin browser harus tercantum di `CORS_ALLOWED_ORIGINS`. Gateway melakukan upgrade WebSocket dan meneruskan koneksi ke Notification Service. Gateway menambahkan token service internal dan menggunakan mTLS pada koneksi upstream.

Contoh pesan campaign dibuat:

```json
{
  "event_id": 10,
  "event": "CAMPAIGN_CREATED",
  "campaign": {
    "id": 10,
    "title": "Bantuan Banjir",
    "description": "Bantuan untuk warga terdampak",
    "target": 25000000,
    "collected": 0,
    "status": "MENUNGGU"
  }
}
```

Contoh pesan donasi:

```json
{
  "event_id": 11,
  "event": "NEW_DONATION",
  "campaign_id": 1,
  "donor_name": "Anonim",
  "amount": 50000
}
```

Donasi simulasi yang dikirim dari frontend menggunakan `SIMULATED_DONATION` dan tidak menyertakan data kontak:

```json
{
  "event_id": 12,
  "event": "SIMULATED_DONATION",
  "campaign_id": 1,
  "donor_name": "Donatur",
  "amount": 25000
}
```

WebSocket hanya mengirim notifikasi aktivitas kepada client yang sedang tersambung. Event history tersimpan pada tabel outbox untuk status publish, bukan sebagai API riwayat notifikasi.

## 10. OAuth2 internal (Gateway ke backend)

OAuth2 Client Credentials digunakan untuk autentikasi service-to-service, bukan login pengguna.

Token endpoint internal:

```http
POST https://campaign-service:8443/oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic <base64(client_id:client_secret)>

grant_type=client_credentials&scope=service%3Aproxy
```

Endpoint ini tidak diekspos melalui route publik Gateway dan mensyaratkan sertifikat client mTLS yang diterbitkan CA lokal. Response:

```json
{
  "access_token": "<INTERNAL_ACCESS_TOKEN>",
  "token_type": "Bearer",
  "expires_in": 900,
  "scope": "service:proxy"
}
```

Gateway menyimpan token hingga mendekati waktu kedaluwarsa dan memperbaruinya otomatis. Untuk request internal, Gateway mengirim token pada header `X-Internal-Token` agar header `Authorization: Bearer <user-jwt>` milik pengguna dapat diteruskan tanpa ditimpa.

Backend memeriksa signature HS256, issuer, audience, subject client, scope, dan masa berlaku token. Endpoint backend selain `/health` dan `/oauth/token` juga mensyaratkan sertifikat client mTLS dengan Common Name `gateway` ketika `MTLS_ENABLED=true`.

## 11. Message queue dan transactional outbox

Campaign Service menyimpan perubahan data dan event outbox dalam transaksi PostgreSQL yang sama. Worker membaca event berdasarkan ID dengan advisory transaction lock PostgreSQL agar satu publisher memproses urutan event, lalu memublikasikannya ke exchange RabbitMQ `crowdfunding.events` dengan delivery mode persistent dan publisher confirmation.

| Event | Routing key | Field `event` |
|---|---|---|
| Campaign dibuat | `campaign.created` | `CAMPAIGN_CREATED` |
| Campaign disetujui | `campaign.approved` | `CAMPAIGN_APPROVED` |
| Donasi dicatat | `donation.created` | `NEW_DONATION` |

Notification Service membuat queue durable `notification.events`, binding `#`, mengonsumsi pesan dengan manual acknowledgement, lalu broadcast payload JSON ke WebSocket. Pesan membawa `event_id` untuk membantu deteksi duplikasi. Pola pengiriman adalah at-least-once, bukan exactly-once.

## 12. Status code

| Status | Makna umum |
|---|---|
| `200 OK` | Request berhasil |
| `202 Accepted` | Webhook diterima dan event disiapkan untuk diproses asinkron |
| `400 Bad Request` | JSON, form, atau payload tidak valid |
| `401 Unauthorized` | Kredensial, signature, atau token tidak valid |
| `403 Forbidden` | Sertifikat client tidak diizinkan; penolakan role pada GraphQL dikembalikan di properti `errors` dengan HTTP 200 |
| `404 Not Found` | Resource tidak ditemukan |
| `409 Conflict` | Email/reference duplikat atau constraint konflik |
| `500 Internal Server Error` | Kegagalan internal atau database |
| `503 Service Unavailable` | Upstream tidak sehat atau autentikasi service tidak tersedia |

## 13. Catatan keamanan

- Jangan mengirim `JWT_SECRET`, `PAYMENT_WEBHOOK_SECRET`, `OAUTH_CLIENT_SECRET`, password DB, atau RabbitMQ credentials ke browser.
- Jangan memanggil port backend secara langsung dari client. Gunakan Gateway.
- Jangan commit `.env` atau folder `certs/`.
- Rotasi secret yang pernah masuk ke repository publik.
- Sertifikat dan CA yang dihasilkan script hanya untuk development lokal.
- Koneksi RabbitMQ pada konfigurasi lokal menggunakan AMQP di jaringan internal Docker dengan autentikasi username/password, belum AMQPS.
- Pada deployment publik, gunakan HTTPS untuk akses browser, AMQPS untuk broker, secret manager, CA tepercaya, rate limiting, audit log, dan konfigurasi webhook resmi payment provider.
