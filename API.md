# API Documentation — Platform Crowdfunding dan Donasi

**Versi:** Code Freeze Week 12  
**Base URL lokal:** `http://localhost:8000` (API Gateway)  
**Format:** JSON untuk HTTP; GraphQL untuk data campaign; WebSocket untuk notifikasi.

Semua request browser diarahkan melalui API Gateway. Campaign Service dan Notification Service tidak mempublikasikan port ke host. Gateway berkomunikasi dengan keduanya menggunakan mTLS dan OAuth2 Client Credentials.

## 1. Ringkasan endpoint publik

| Method | Path | Auth pengguna | Deskripsi |
|---|---|---|---|
| GET | `/health` | Tidak | Health check Gateway |
| GET | `/registry` | Tidak | Status health service backend |
| POST | `/auth/register` | Tidak | Registrasi akun DONATUR |
| POST | `/auth/login` | Tidak | Login dan penerbitan JWT pengguna |
| POST | `/graphql` | Bergantung operasi | Query/mutation campaign |
| POST | `/webhook/payment` | HMAC signature | Penerimaan event pembayaran |
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

Response sukses memiliki bentuk yang sama dengan registrasi. JWT pengguna berlaku 12 jam dan ditandatangani menggunakan HS256. Password disimpan sebagai bcrypt hash.

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
  }
}
```

Wajib menyertakan JWT pengguna dengan role `ADMIN`.

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
- Query `campaigns` dan `campaign` bersifat publik; `pendingCampaigns` dan mutation memiliki pemeriksaan role sesuai aturan di atas.

## 6. Payment webhook

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

Webhook saat ini adalah simulasi. Integrasi payment gateway sesungguhnya harus mengikuti format signature, retry, dan payload resmi provider terkait.

## 7. WebSocket notifikasi

### `GET /ws/donasi`

URL browser lokal:

```javascript
const socket = new WebSocket('ws://localhost:8000/ws/donasi');
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

WebSocket hanya mengirim notifikasi aktivitas kepada client yang sedang tersambung. Event history tersimpan pada tabel outbox untuk status publish, bukan sebagai API riwayat notifikasi.

## 8. OAuth2 internal (Gateway ke backend)

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

## 9. Message queue dan transactional outbox

Campaign Service menyimpan perubahan data dan event outbox dalam transaksi PostgreSQL yang sama. Worker membaca event berdasarkan ID dengan advisory transaction lock PostgreSQL agar satu publisher memproses urutan event, lalu memublikasikannya ke exchange RabbitMQ `crowdfunding.events` dengan delivery mode persistent dan publisher confirmation.

| Event | Routing key | Field `event` |
|---|---|---|
| Campaign dibuat | `campaign.created` | `CAMPAIGN_CREATED` |
| Campaign disetujui | `campaign.approved` | `CAMPAIGN_APPROVED` |
| Donasi dicatat | `donation.created` | `NEW_DONATION` |

Notification Service membuat queue durable `notification.events`, binding `#`, mengonsumsi pesan dengan manual acknowledgement, lalu broadcast payload JSON ke WebSocket. Pesan membawa `event_id` untuk membantu deteksi duplikasi. Pola pengiriman adalah at-least-once, bukan exactly-once.

## 10. Status code

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

## 11. Catatan keamanan

- Jangan mengirim `JWT_SECRET`, `PAYMENT_WEBHOOK_SECRET`, `OAUTH_CLIENT_SECRET`, password DB, atau RabbitMQ credentials ke browser.
- Jangan memanggil port backend secara langsung dari client. Gunakan Gateway.
- Jangan commit `.env` atau folder `certs/`.
- Rotasi secret yang pernah masuk ke repository publik.
- Sertifikat dan CA yang dihasilkan script hanya untuk development lokal.
- Koneksi RabbitMQ pada konfigurasi lokal menggunakan AMQP di jaringan internal Docker dengan autentikasi username/password, belum AMQPS.
- Pada deployment publik, gunakan HTTPS untuk akses browser, AMQPS untuk broker, secret manager, CA tepercaya, rate limiting, audit log, dan konfigurasi webhook resmi payment provider.
