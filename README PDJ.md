# Jawaban PDJ — Test Plan Crowdfunding

Dokumen ini menjawab **setiap pertanyaan secara terpisah** sesuai urutan pada dokumen `Jawaban_Pertanyaan_Test_Plan_Crowdfunding.docx`. Teks pertanyaan dipertahankan, termasuk semua pertanyaan pada bagian Approval. Jawaban didasarkan pada source code repository; hal yang belum tersedia atau belum diputuskan dinyatakan secara eksplisit.

> **Status saat ini:** tersedia registrasi/login, pengajuan dan approval campaign, query campaign publik, webhook pembayaran simulasi, serta broadcast aktivitas WebSocket. Belum ada checkout/donasi dari UI atau integrasi payment provider seperti GoPay.

## 01. Test Plan Identifier

#### 1. Apa nama resmi aplikasi atau platform crowdfunding/donasi yang kalian buat?

**Jawaban:** Nama kerja pada repository adalah **Platform Crowdfunding dan Donasi**, dengan identitas UI **Donasi Bersama**. Nama produk resmi masih perlu disepakati kelompok.

#### 2. Siapa saja anggota kelompok E dan apa pembagian tanggung jawab masing-masing?

**Jawaban:** Dokumen sumber mencantumkan Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas sebagai anggota Kelompok 5 (E). Pembagian PIC per komponen belum tercantum dan harus ditetapkan kelompok.

#### 3. Apakah sistem memiliki versi atau tahapan pengembangan tertentu?

**Jawaban:** Repository menyebut implementasi sampai Sesi 10. Nomor versi/build rilis belum ditetapkan. README menyebut tahapan berikutnya (Sesi 11–14), tetapi status dan tanggal tiap tahap perlu dikonfirmasi.

#### 4. Apakah terdapat dokumentasi resmi proyek yang bisa diberikan kepada kelompok penguji?

**Jawaban:** Dokumen yang ada di repository meliputi README proyek, konfigurasi Docker Compose, source code, dan skema database. SRS formal, OpenAPI/Swagger, dan laporan hasil pengujian belum tersedia.

#### 5. Apakah terdapat perubahan fitur atau spesifikasi yang masih direncanakan hingga waktu code freeze minggu ke-12?

**Jawaban:** Rencana perubahan fitur sampai code freeze minggu ke-12 belum dicatat di repository. Konfirmasi daftar perubahan dan tanggal freeze kepada kelompok; setelah freeze, ikuti proses persetujuan perubahan yang disepakati.

## 02. Introduction

#### 6. Bisa dijelaskan secara keseluruhan bagaimana platform crowdfunding/donasi yang kalian buat bekerja?

**Jawaban:** Pengguna mendaftar/login, melihat campaign aktif, dan dapat mengajukan campaign. Pengajuan menunggu persetujuan Admin. Campaign Service menyediakan GraphQL dan webhook simulasi; Notification Service membroadcast event melalui WebSocket. Aplikasi belum menyediakan alur donasi dari UI.

#### 7. Apa tujuan utama sistem ini dan permasalahan apa yang ingin diselesaikan?

**Jawaban:** Tujuan implementasi saat ini adalah menyediakan platform pengajuan dan publikasi campaign dengan autentikasi/otorisasi dasar, serta menunjukkan komunikasi antarservice dan notifikasi aktivitas. Donasi provider sungguhan belum menjadi fitur yang tersedia.

#### 8. Siapa saja pengguna sistem? Apakah terdapat donatur, pengelola kampanye, dan admin?

**Jawaban:** Database mengenal role `DONATUR`, `PENGELOLA`, dan `ADMIN`. Registrasi publik selalu menghasilkan `DONATUR`; setiap pengguna login saat ini dapat mengajukan campaign. Operasi daftar campaign tertunda dan approval hanya untuk `ADMIN`. `PENGELOLA` belum memiliki aturan khusus pada resolver.

#### 9. Bagaimana alur pengguna mulai dari melihat kampanye hingga berhasil melakukan donasi?

**Jawaban:** Alur yang benar-benar tersedia berakhir pada melihat campaign aktif dan mengajukan campaign baru untuk ditinjau. Memilih campaign, memasukkan nominal donasi, checkout, dan pembayaran provider belum tersedia pada frontend.

#### 10. Bagaimana proses pengelola kampanye membuat dan mengelola kampanye donasi?

**Jawaban:** Pengguna yang login mengisi judul, cerita, target, minimal 1 hingga maksimal 5 foto, dan opsional satu video MP4. Frontend mengirim form multipart ke `POST /campaigns`; backend memvalidasi file dan menyimpan campaign beserta media dalam satu transaksi berstatus `MENUNGGU`. Admin dapat menyetujui campaign hingga menjadi `AKTIF`; campaign aktif dapat dibuka pada halaman detail yang menampilkan cerita, foto/video, target, dan progres dana. Edit/hapus serta donasi melalui frontend belum tersedia.

#### 11. Bagaimana sistem menampilkan jumlah dana yang sudah terkumpul?

**Jawaban:** Campaign aktif menyimpan nilai `collected` dan `target`; frontend menampilkan keduanya serta menghitung persentase. Nilai `collected` bertambah dari webhook simulasi hanya untuk status `SETTLEMENT` dengan signature valid.

#### 12. Apakah perubahan jumlah donasi langsung ditampilkan kepada seluruh pengguna yang sedang membuka halaman kampanye?

**Jawaban:** Tidak sepenuhnya. WebSocket mengirim event ke daftar aktivitas, tetapi frontend tidak memuat ulang campaign atau memperbarui angka progres setelah menerima event. Pengguna perlu memuat ulang halaman untuk mengambil nilai campaign terkini.

#### 13. Apa alasan kalian menggunakan WebSocket untuk memperbarui progres donasi secara real-time?

**Jawaban:** WebSocket menyediakan kanal push agar aktivitas dari service dapat diterima browser tanpa polling berkala. Implementasi saat ini membroadcast event campaign dan webhook donasi simulasi, bukan sistem update progres yang terjamin.

#### 14. Apa tujuan utama integrasi payment dalam sistem?

**Jawaban:** Dalam implementasi sekarang belum ada integrasi payment provider. Webhook simulasi memperagakan penerimaan status pembayaran dan penambahan total campaign setelah verifikasi signature.

#### 15. Payment gateway atau penyedia layanan pembayaran apa yang digunakan?

**Jawaban:** Belum ada payment gateway yang terintegrasi. GoPay disebut di dokumen rancangan awal saja; source code tidak menghubungi GoPay, Midtrans, Xendit, atau provider lain.

#### 16. Apakah sistem memiliki dokumentasi kebutuhan (SRS), desain sistem, diagram arsitektur, atau dokumentasi API yang bisa dijadikan referensi pengujian?

**Jawaban:** Belum tersedia SRS formal, OpenAPI/Swagger, atau paket diagram lengkap di repository. README, Docker Compose, source code, `db/init.sql`, dan migration `db/migrations/002_campaign_media.sql` dapat dipakai untuk memahami implementasi yang ada.

## 03. Test Items

### A. Arsitektur dan Service

#### 17. Service apa saja yang terdapat dalam sistem (misalnya authentication, campaign, donation, payment, notification)?

**Jawaban:** Service yang ada adalah API Gateway, Campaign Service (auth, GraphQL campaign, webhook simulasi), Notification Service (WebSocket), PostgreSQL, dan frontend React/Vite. Tidak ada service pembayaran atau autentikasi mandiri.

#### 18. Bagaimana arsitektur sistem yang digunakan, monolit atau microservices?

**Jawaban:** Aplikasi menggunakan beberapa service: Gateway, Campaign Service, dan Notification Service, dengan dua instance Campaign Service pada Docker Compose. Jadi bukan satu backend monolitik, meskipun beberapa domain seperti auth dan campaign berada bersama dalam Campaign Service.

#### 19. Bagaimana hubungan antarservice dalam proses donasi?

**Jawaban:** Alur donasi provider belum tersedia. Untuk alur yang ada, browser menuju Gateway; Gateway merutekan request ke Campaign Service; Campaign Service memakai PostgreSQL dan mengirim event ke Notification Service untuk broadcast WebSocket.

#### 20. Apakah terdapat API Gateway atau mekanisme komunikasi khusus antarservice?

**Jawaban:** Ya. API Gateway Go mendengarkan port 8000, menjadi reverse proxy, menyediakan `/health` dan `/registry`, serta melakukan health check dan round-robin ke instance Campaign Service yang sehat.

#### 21. Bisa diberikan daftar endpoint beserta metode HTTP-nya?

**Jawaban:** Endpoint publik melalui Gateway: `GET /health`, `GET /registry`, `POST /auth/register`, `POST /auth/login`, `POST /graphql`, `POST /campaigns` (autentikasi; multipart campaign dan media), `GET /campaigns/{campaignID}/media/{mediaID}`, `POST /webhook/payment`, dan WebSocket `/ws/donasi`. Endpoint media hanya menampilkan campaign aktif ke publik; pemilik/Admin dapat melihat media campaign yang menunggu. Endpoint internal Notification Service adalah `POST /internal/broadcast`.

#### 22. Apakah tersedia dokumentasi OpenAPI/Swagger?

**Jawaban:** Tidak. File OpenAPI/Swagger tidak ditemukan pada repository. Daftar route dan GraphQL schema harus dirujuk dari source code.

#### 23. Apakah terdapat dokumentasi mengenai endpoint WebSocket dan event yang dikirimkan?

**Jawaban:** Route WebSocket adalah `/ws/donasi`. Event yang dihasilkan: `CAMPAIGN_CREATED`, `CAMPAIGN_APPROVED`, dan `NEW_DONATION`. Tidak ada dokumen protokol terpisah; payload aktual dibentuk pada Campaign Service.

### B. WebSocket – Real-time Progress Update

#### 24. Pada bagian mana saja WebSocket diterapkan dalam sistem?

**Jawaban:** Frontend membuka koneksi WebSocket ke Gateway pada `/ws/donasi` saat aplikasi dimuat, sebelum login. Notification Service mendaftarkan koneksi dan mengirim pesan broadcast.

#### 25. Apakah WebSocket digunakan untuk memperbarui jumlah donasi, persentase progres kampanye, atau status donasi?

**Jawaban:** WebSocket mengirim objek event aktivitas, bukan mekanisme khusus yang menyinkronkan persentase. `NEW_DONATION` dipicu settlement simulasi; frontend hanya menampilkan event dan tidak memperbarui nilai campaign otomatis.

#### 26. Kapan koneksi WebSocket dibuat, apakah saat pengguna membuka halaman kampanye atau setelah login?

**Jawaban:** Di frontend saat halaman aplikasi dimuat, tanpa menunggu login. Koneksi ditutup ketika komponen dilepas; tidak ada implementasi reconnect otomatis.

#### 27. Bagaimana sistem mengirimkan pembaruan ketika donasi berhasil dilakukan?

**Jawaban:** Webhook simulasi dengan signature valid dan status `SETTLEMENT` menyimpan donasi, menambah `collected`, lalu Campaign Service mengirim event `NEW_DONATION`. Tidak ada callback provider sungguhan.

#### 28. Apakah perubahan progres dikirimkan ke semua pengguna yang sedang membuka kampanye yang sama?

**Jawaban:** Tidak ada pembukaan kanal per campaign. Notification Service membroadcast setiap pesan ke seluruh client WebSocket yang tersambung, termasuk client yang tidak sedang membuka campaign tersebut.

#### 29. Informasi apa saja yang dikirimkan melalui WebSocket?

**Jawaban:** `CAMPAIGN_CREATED` dan `CAMPAIGN_APPROVED` membawa objek campaign; `NEW_DONATION` membawa `campaign_id`, `donor_name`, dan `amount`. Pesan dikirim sebagai JSON.

#### 30. Apakah WebSocket juga digunakan untuk notifikasi selain pembaruan progres donasi?

**Jawaban:** Ya. Selain event donasi simulasi, WebSocket membawa event pembuatan dan persetujuan campaign.

#### 31. Apakah terdapat mekanisme fallback apabila WebSocket tidak dapat digunakan, misalnya polling HTTP?

**Jawaban:** Tidak ada fallback polling untuk notifikasi WebSocket. Campaign hanya dimuat dari GraphQL pada pemuatan halaman atau setelah aksi tertentu seperti approval.

### C. Payment Integration

#### 32. Payment gateway apa yang digunakan (misalnya Midtrans, Xendit, atau lainnya)?

**Jawaban:** Belum ada payment gateway yang terintegrasi. GoPay disebut di dokumen rancangan awal saja; source code tidak menghubungi GoPay, Midtrans, Xendit, atau provider lain.

#### 33. Metode pembayaran apa saja yang didukung?

**Jawaban:** Belum ada metode pembayaran yang didukung karena provider dan checkout belum diintegrasikan.

#### 34. Bagaimana alur integrasi antara sistem kalian dan payment gateway?

**Jawaban:** Belum ada alur integrasi payment provider. Yang tersedia hanya endpoint internal simulasi: pengirim membuat payload, menghitung HMAC-SHA256 menggunakan secret lokal, lalu mengirimnya ke webhook.

#### 35. Apakah sistem menggunakan redirect, payment API, atau hosted checkout?

**Jawaban:** Tidak ada redirect, hosted checkout, atau payment API provider pada frontend/backend saat ini.

#### 36. Bagaimana sistem menerima informasi bahwa pembayaran berhasil atau gagal?

**Jawaban:** Untuk simulasi, Campaign Service menerima status pada payload webhook. Status hanya diterima jika signature dan payload valid; hanya `SETTLEMENT` yang menambah total campaign.

#### 37. Apakah payment gateway menggunakan webhook atau callback untuk mengirimkan status pembayaran?

**Jawaban:** Ada endpoint webhook simulasi milik Campaign Service; tidak menerima callback dari payment gateway sungguhan.

#### 38. Endpoint mana yang digunakan untuk menerima callback atau webhook pembayaran?

**Jawaban:** Endpointnya `POST /webhook/payment` melalui Gateway, diteruskan ke Campaign Service.

#### 39. Apakah terdapat proses verifikasi sebelum status donasi diperbarui menjadi berhasil?

**Jawaban:** Ya, untuk simulasi: HMAC-SHA256 header `X-Signature` harus cocok dengan raw body. Payload juga divalidasi. Belum ada verifikasi transaksi, nominal order, atau status melalui provider.

## 04. Features to be Tested / Not Tested

### A. Features to be Tested

#### 40. Apakah koneksi WebSocket akan diuji ketika pengguna membuka halaman kampanye?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 41. Apakah perlu diuji pembaruan progres ketika donasi berhasil dilakukan?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 42. Apakah perlu diuji ketika banyak pengguna membuka kampanye yang sama secara bersamaan?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 43. Apakah perlu diuji ketika beberapa donasi berhasil masuk dalam waktu hampir bersamaan?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 44. Apakah perlu diuji ketika koneksi WebSocket terputus secara tiba-tiba?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 45. Apakah perlu diuji kemampuan sistem menghubungkan kembali pengguna setelah koneksi terputus?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 46. Apakah perlu diuji apakah pengguna mendapatkan pembaruan terbaru setelah berhasil reconnect?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 47. Apakah perlu diuji ketika server WebSocket mengalami gangguan?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 48. Apakah perlu diuji apakah pengguna dapat menerima pembaruan dari kampanye yang tidak sedang diakses atau tidak memiliki izin untuk dilihat?

**Jawaban:** Ya, sebagai pengujian fungsi WebSocket pada lingkungan lokal yang disetujui. Catat bahwa broadcast saat ini global, tidak memfilter campaign, dan tidak menyediakan reconnect atau replay otomatis. Load test harus mendapat persetujuan terlebih dahulu.

#### 49. Apakah perlu diuji proses pembayaran berhasil, gagal, pending, dan kedaluwarsa?

**Jawaban:** Uji status `PENDING`, `SETTLEMENT`, dan `FAILED` pada webhook simulasi. Status kedaluwarsa belum didukung. Jangan menyebut skenario ini sebagai pengujian status GoPay/provider.

#### 50. Apakah perlu diuji callback atau webhook yang dikirim oleh payment gateway?

**Jawaban:** Ya, untuk webhook simulasi di lokal: verifikasi signature invalid ditolak, referensi identik kedua ditolak oleh unique constraint, dan payload valid diproses sesuai status. Provider callback berulang/terlambat belum terintegrasi.

#### 51. Apakah perlu diuji callback yang dikirim berulang kali?

**Jawaban:** Ya, untuk webhook simulasi di lokal: verifikasi signature invalid ditolak, referensi identik kedua ditolak oleh unique constraint, dan payload valid diproses sesuai status. Provider callback berulang/terlambat belum terintegrasi.

#### 52. Apakah perlu diuji callback dengan signature atau token yang tidak valid?

**Jawaban:** Ya, untuk webhook simulasi di lokal: verifikasi signature invalid ditolak, referensi identik kedua ditolak oleh unique constraint, dan payload valid diproses sesuai status. Provider callback berulang/terlambat belum terintegrasi.

#### 53. Apakah perlu diuji manipulasi nominal donasi dari sisi pengguna?

**Jawaban:** Belum dapat menguji manipulasi terhadap nominal checkout karena checkout/order tidak ada. Uji batas amount positif dan payload webhook simulasi hanya pada lokal serta dengan persetujuan developer; webhook belum membandingkan nominal terhadap order sebelumnya.

#### 54. Apakah perlu diuji ketika nominal pembayaran tidak sesuai dengan nominal donasi?

**Jawaban:** Belum dapat menguji manipulasi terhadap nominal checkout karena checkout/order tidak ada. Uji batas amount positif dan payload webhook simulasi hanya pada lokal serta dengan persetujuan developer; webhook belum membandingkan nominal terhadap order sebelumnya.

#### 55. Apakah perlu diuji ketika callback diterima lebih dari satu kali?

**Jawaban:** Ya, untuk webhook simulasi di lokal: verifikasi signature invalid ditolak, referensi identik kedua ditolak oleh unique constraint, dan payload valid diproses sesuai status. Provider callback berulang/terlambat belum terintegrasi.

#### 56. Apakah perlu diuji ketika callback datang terlambat atau tidak berurutan?

**Jawaban:** Ya, untuk webhook simulasi di lokal: verifikasi signature invalid ditolak, referensi identik kedua ditolak oleh unique constraint, dan payload valid diproses sesuai status. Provider callback berulang/terlambat belum terintegrasi.

#### 57. Apakah perlu diuji ketika payment gateway tidak dapat diakses?

**Jawaban:** Tidak sebagai alur provider, sebab provider belum terhubung. Pengujian kegagalan service dapat dilakukan terpisah pada environment lokal yang disetujui, tetapi tidak membuktikan penanganan callback/rekonsiliasi provider.

#### 58. Apakah perlu diuji ketika pembayaran berhasil di payment gateway tetapi sistem belum memperbarui status donasi?

**Jawaban:** Tidak sebagai alur provider, sebab provider belum terhubung. Pengujian kegagalan service dapat dilakukan terpisah pada environment lokal yang disetujui, tetapi tidak membuktikan penanganan callback/rekonsiliasi provider.

### B. Features Not to be Tested

#### 59. Apakah ada fitur yang berada di luar cakupan pengujian kelompok kami?

**Jawaban:** Ya. Di luar cakupan saat ini adalah transaksi uang sungguhan, integrasi provider/GoPay, refund, fitur yang belum diimplementasikan, dan pengujian production atau sistem eksternal tanpa izin.

#### 60. Apakah transaksi pembayaran sungguhan tidak diperbolehkan dan harus menggunakan sandbox?

**Jawaban:** Ya. Pengujian hanya pada database Neon khusus testing dan webhook simulasi; transaksi uang sungguhan tidak dilakukan. Sandbox provider belum tersedia karena belum ada integrasi provider. Jangan gunakan database production untuk pengujian.

#### 61. Apakah ada layanan eksternal yang tidak dapat kami akses untuk pengujian?

**Jawaban:** Tidak ada provider eksternal yang digunakan oleh source code saat ini. Akses ke staging/provider di luar repository belum ditentukan; minta izin dan kredensial uji dari pemilik sistem bila nantinya diperlukan.

#### 62. Apakah ada fitur yang membutuhkan izin khusus dari developer sebelum diuji?

**Jawaban:** Ya. Load test, pengujian security/payload, reset database, dan pengujian eksternal harus mendapat persetujuan developer/koordinator dan memakai environment lokal atau staging yang disetujui.

## 05. Approach / Test Strategy

### A. Functional Testing

#### 63. Bagaimana sistem menentukan bahwa donasi berhasil dilakukan?

**Jawaban:** Pada implementasi saat ini, hanya webhook simulasi dengan signature valid dan status `SETTLEMENT` yang diperlakukan sebagai settlement dan menambah total. Tidak ada verifikasi melalui gateway pembayaran.

#### 64. Apakah jumlah dana kampanye otomatis bertambah setelah pembayaran dinyatakan berhasil?

**Jawaban:** Ya, ketika webhook simulasi `SETTLEMENT` valid berhasil disimpan, `collected` campaign bertambah dalam transaksi database. `PENDING` dan `FAILED` tidak menambahnya.

#### 65. Apakah progres donasi hanya diperbarui setelah pembayaran berhasil atau juga ketika pembayaran masih pending?

**Jawaban:** Total hanya bertambah pada status `SETTLEMENT`; `PENDING` tidak menambah. Frontend belum otomatis menyegarkan angka setelah menerima WebSocket.

#### 66. Apa yang ditampilkan sistem ketika pembayaran gagal?

**Jawaban:** Webhook `FAILED` tersimpan sebagai status donasi gagal dan tidak menambah `collected`. UI pembayaran gagal belum ada.

#### 67. Bagaimana sistem menangani donasi yang sudah kedaluwarsa?

**Jawaban:** Status `EXPIRED`/kedaluwarsa belum didukung oleh schema atau handler. Pengujian kedaluwarsa belum dapat dilakukan sebagai fitur berjalan.

#### 68. Apakah status donasi dan jumlah dana kampanye selalu diperbarui secara konsisten?

**Jawaban:** Handler menyimpan donasi dan perubahan total dalam transaksi database untuk settlement simulasi. Konsistensi dengan provider, retry/rekonsiliasi, dan pembaruan UI real-time belum tersedia.

#### 69. Bagaimana sistem menangani pembatalan pembayaran?

**Jawaban:** Status pembatalan terpisah belum didefinisikan. Status webhook yang diterima hanya `PENDING`, `SETTLEMENT`, dan `FAILED`; kebijakan cancel belum ada.

#### 70. Apakah sistem memiliki fitur pengembalian dana (refund)?

**Jawaban:** Tidak. Fitur refund tidak tersedia.

### B. WebSocket Testing

#### 71. Bagaimana sistem menentukan pengguna yang berhak menerima update progres kampanye?

**Jawaban:** WebSocket tidak memeriksa hak akses campaign atau identitas pengguna. Setelah origin lolos pemeriksaan, event dibroadcast ke semua client; jangan gunakan untuk data yang memerlukan otorisasi per campaign.

#### 72. Apakah koneksi WebSocket memerlukan autentikasi?

**Jawaban:** Tidak. Koneksi WebSocket tidak memvalidasi JWT atau memerlukan login.

#### 73. Apakah pengguna yang belum login tetap dapat menerima update progres kampanye?

**Jawaban:** Ya. Koneksi dibuat sebelum login dan tidak meminta token, sehingga pengguna anonim dapat menerima broadcast.

#### 74. Apakah setiap perubahan jumlah donasi harus langsung dikirim melalui WebSocket?

**Jawaban:** Belum ada persyaratan delivery real-time formal. Implementasi mengirim event hasil operasi campaign dan settlement simulasi, tetapi tidak menjamin pengiriman ketika client terputus.

#### 75. Berapa lama waktu yang diharapkan antara pembayaran berhasil dan progres donasi diperbarui?

**Jawaban:** SLA waktu belum ditetapkan dan belum diukur. Broadcast dilakukan setelah transaksi settlement commit dengan request internal timeout 2 detik, tetapi itu bukan jaminan waktu sampai browser menerima atau menampilkan update.

#### 76. Apakah sistem menjamin pembaruan dikirim sesuai urutan transaksi?

**Jawaban:** Tidak ada jaminan urutan end-to-end atau event ID/sequence pada implementasi. Jangan jadikan urutan kedatangan sebagai sumber kebenaran saldo.

#### 77. Bagaimana sistem menangani dua atau lebih donasi yang berhasil pada waktu bersamaan?

**Jawaban:** Update database settlement dilakukan dengan operasi penambahan SQL di dalam transaksi; referensi unik menolak referensi yang sama. Load test bersamaan belum pernah didokumentasikan.

#### 78. Bagaimana sistem menangani pengguna yang kehilangan koneksi internet?

**Jawaban:** Koneksi yang putus ditutup oleh server/client. Frontend tidak memiliki reconnect otomatis atau sinkronisasi ulang khusus setelah reconnect.

#### 79. Apakah pengguna otomatis terhubung kembali setelah koneksi terputus?

**Jawaban:** Tidak. Frontend tidak mengimplementasikan reconnect otomatis.

#### 80. Setelah reconnect, apakah pengguna menerima kondisi progres terbaru atau hanya update yang terjadi setelah koneksi tersambung kembali?

**Jawaban:** Tidak ada replay/snapshot otomatis saat reconnect. Setelah memuat ulang halaman, frontend mengambil daftar campaign lagi melalui GraphQL.

#### 81. Bagaimana sistem menangani pesan WebSocket yang gagal dikirim?

**Jawaban:** Server menutup dan menghapus koneksi yang gagal saat `WriteJSON`; tidak ada antrean atau pengiriman ulang event.

#### 82. Apakah ada mekanisme heartbeat atau ping-pong untuk mendeteksi koneksi yang sudah tidak aktif?

**Jawaban:** Tidak ditemukan heartbeat ping/pong yang dikonfigurasi secara eksplisit di service. Perilaku library standar bukan SLA aplikasi dan belum diuji sebagai fitur.

#### 83. Apakah terdapat batas maksimal koneksi WebSocket secara bersamaan?

**Jawaban:** Batas maksimal koneksi belum ditetapkan atau diuji. Kapasitas perlu diukur pada environment terisolasi sebelum memberi angka.

### C. Performance dan Load Testing

#### 84. Berapa jumlah pengguna yang diperkirakan membuka halaman kampanye secara bersamaan?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

#### 85. Berapa jumlah koneksi WebSocket bersamaan yang ditargetkan dapat ditangani?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

#### 86. Berapa banyak update donasi yang diperkirakan terjadi dalam satu detik atau satu menit?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

#### 87. Berapa waktu maksimal yang diharapkan dari pembayaran berhasil sampai progres kampanye diperbarui?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

#### 88. Apakah terdapat target response time untuk API pembayaran?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

#### 89. Apakah sistem sudah pernah diuji dengan banyak koneksi WebSocket secara bersamaan?

**Jawaban:** Belum ada bukti load test atau uji banyak koneksi pada repository.

#### 90. Apakah sistem memiliki target jumlah transaksi donasi yang dapat diproses dalam periode tertentu?

**Jawaban:** Belum ditentukan dan belum diukur. Tetapkan target pengguna/koneksi/event, latency, serta throughput bersama tim lalu uji bertahap pada environment yang disetujui.

### D. Security Testing – WebSocket

#### 91. Bagaimana sistem melakukan autentikasi koneksi WebSocket?

**Jawaban:** Tidak ada autentikasi atau validasi token pada endpoint WebSocket saat handshake. Akses dibatasi hanya melalui pengecekan Origin yang diizinkan.

#### 92. Apakah sistem melakukan validasi token ketika koneksi WebSocket dibuat?

**Jawaban:** Tidak ada autentikasi atau validasi token pada endpoint WebSocket saat handshake. Akses dibatasi hanya melalui pengecekan Origin yang diizinkan.

#### 93. Bagaimana sistem mencegah pengguna berlangganan atau subscribe ke kanal kampanye yang tidak seharusnya dapat diakses?

**Jawaban:** Tidak ada subscribe/channel campaign maupun pemeriksaan izin per campaign. Semua pesan dibroadcast global; pengguna tidak dapat memilih kanal melalui protokol aplikasi, tetapi tetap dapat menerima seluruh broadcast.

#### 94. Apakah sistem membatasi asal koneksi menggunakan konfigurasi Origin atau mekanisme keamanan lainnya?

**Jawaban:** Ya. Notification Service mengizinkan Origin `http://localhost:5173` dan `http://localhost:8000`. Konfigurasi ini khusus lokal dan perlu ditinjau untuk deployment.

#### 95. Apakah pesan yang diterima melalui WebSocket divalidasi sebelum diproses?

**Jawaban:** Server membaca frame dari client hanya untuk menjaga loop koneksi; tidak ada pemrosesan pesan client untuk mengubah data campaign. Endpoint internal `/internal/broadcast` memvalidasi JSON dan membatasi body 64 KiB.

#### 96. Apakah informasi sensitif seperti data pribadi donatur atau informasi pembayaran ikut dikirim melalui WebSocket?

**Jawaban:** Event `NEW_DONATION` memang mengirim nama donatur dan nominal ke semua client WebSocket. Ini data yang perlu perlindungan; belum ada filter, autentikasi, atau penyamaran nama.

#### 97. Bagaimana sistem mencegah pengguna mengirimkan pesan palsu untuk memanipulasi progres donasi?

**Jawaban:** Pesan dari browser tidak mengubah progres: update jumlah hanya dilakukan oleh Campaign Service setelah verifikasi signature webhook simulasi. Namun WebSocket tidak menggunakan autentikasi, dan keamanan endpoint internal broadcast perlu dibatasi pada jaringan layanan.

### E. Security Testing – Payment Integration

#### 98. Bagaimana sistem memverifikasi keaslian callback atau webhook dari payment gateway?

**Jawaban:** Keaslian webhook simulasi diverifikasi dengan HMAC-SHA256 atas raw request body dan secret `PAYMENT_WEBHOOK_SECRET`, dibandingkan dengan header `X-Signature` menggunakan perbandingan konstan. Ini bukan mekanisme resmi provider.

#### 99. Apakah sistem menggunakan signature verification atau mekanisme autentikasi khusus?

**Jawaban:** Keaslian webhook simulasi diverifikasi dengan HMAC-SHA256 atas raw request body dan secret `PAYMENT_WEBHOOK_SECRET`, dibandingkan dengan header `X-Signature` menggunakan perbandingan konstan. Ini bukan mekanisme resmi provider.

#### 100. Bagaimana sistem menangani callback yang memiliki signature tidak valid?

**Jawaban:** Signature yang tidak cocok ditolak dengan HTTP 401 sebelum payload diproses atau database diperbarui.

#### 101. Apakah nominal pembayaran diverifikasi kembali di sisi server?

**Jawaban:** Amount divalidasi agar positif dan database membatasi amount positif, tetapi tidak dibandingkan dengan order/nominal yang dibuat sebelumnya karena fitur checkout tidak ada. Jadi perlindungan terhadap perubahan amount relatif terhadap transaksi provider belum diimplementasikan.

#### 102. Apakah sistem memastikan nominal donasi tidak dapat diubah hanya dengan memodifikasi request dari sisi pengguna?

**Jawaban:** Amount divalidasi agar positif dan database membatasi amount positif, tetapi tidak dibandingkan dengan order/nominal yang dibuat sebelumnya karena fitur checkout tidak ada. Jadi perlindungan terhadap perubahan amount relatif terhadap transaksi provider belum diimplementasikan.

#### 103. Bagaimana sistem mencegah callback palsu yang mengklaim pembayaran telah berhasil?

**Jawaban:** Webhook palsu tanpa HMAC yang benar ditolak. Siapa pun yang memiliki secret dapat membuat signature; secret harus dijaga, dan implementasi tidak memeriksa transaksi terhadap provider.

#### 104. Apakah sistem menggunakan idempotency key untuk mencegah satu transaksi diproses berkali-kali?

**Jawaban:** Tidak ada kolom idempotency key khusus. `payment_reference` memiliki constraint UNIQUE, sehingga pengulangan referensi yang sama ditolak; referensi baru untuk pembayaran yang sama belum dapat dikenali sebagai duplikat.

#### 105. Bagaimana sistem menangani callback yang dikirim berulang kali oleh payment gateway?

**Jawaban:** Referensi yang sama ditolak oleh unique constraint. Tidak ada mekanisme retry provider atau pemrosesan idempotent untuk referensi baru yang mewakili transaksi sama.

#### 106. Bagaimana sistem memastikan transaksi pembayaran tidak tercatat sebagai berhasil sebelum diverifikasi?

**Jawaban:** Untuk webhook simulasi, verifikasi HMAC dan validasi payload dilakukan sebelum insert; hanya `SETTLEMENT` valid yang menaikkan total. Tidak ada validasi status terhadap provider eksternal.

#### 107. Apakah data sensitif pembayaran disimpan oleh sistem atau sepenuhnya dikelola oleh payment gateway?

**Jawaban:** Tidak ada data kartu atau kredensial pembayaran yang disimpan karena provider belum terintegrasi. Database menyimpan nama donor, amount, reference, status, dan campaign ID untuk simulasi.

#### 108. Apakah komunikasi dengan payment gateway menggunakan HTTPS/TLS?

**Jawaban:** Tidak ada komunikasi dengan payment gateway. Deployment compose tidak mengonfigurasi TLS; akses HTTPS harus disediakan oleh reverse proxy/infrastruktur sebelum deployment.

#### 109. Bagaimana sistem menangani kebocoran atau penyalahgunaan API key dan secret key payment gateway?

**Jawaban:** API key payment provider belum digunakan. Secret webhook dan JWT dibaca dari environment; gunakan secret lokal yang kuat dan jangan commit atau bagikan nilainya. Prosedur rotasi/incident response belum didokumentasikan.

## 06. Pass / Fail Criteria

### A. WebSocket

#### 110. Berapa target waktu maksimal pembaruan progres donasi setelah pembayaran berhasil?

**Jawaban:** Target waktu belum ditetapkan atau diuji. WebSocket mengirim event setelah commit, tetapi tidak ada SLA delivery ke browser.

#### 111. Apakah semua pengguna yang membuka kampanye harus mendapatkan pembaruan yang sama?

**Jawaban:** Saat ini semua client yang tersambung menerima broadcast yang sama, bukan hanya pengguna yang membuka campaign tersebut. Ini perilaku aktual, bukan desain isolasi per campaign.

#### 112. Apakah jumlah dana yang ditampilkan harus selalu sama dengan jumlah donasi yang berhasil?

**Jawaban:** Sebagai aturan bisnis target, jumlah terkumpul seharusnya sama dengan penjumlahan settlement yang tercatat. Implementasi menambah total saat settlement simulasi; audit/reconciliation otomatis belum ada.

#### 113. Apakah pembaruan harus diterima secara berurutan berdasarkan transaksi?

**Jawaban:** Belum ada jaminan urutan transaksi pada event WebSocket. Gunakan nilai database sebagai sumber kebenaran dan jangan menyimpulkan urutan dari event.

#### 114. Berapa jumlah maksimal koneksi WebSocket bersamaan yang harus mampu ditangani sistem?

**Jawaban:** Angka maksimum koneksi belum ditentukan atau dites.

#### 115. Berapa persentase keberhasilan pengiriman update yang ditargetkan?

**Jawaban:** Target persentase delivery belum ditetapkan; tidak ada metrik delivery atau acknowledgement event.

#### 116. Apakah koneksi yang terputus harus dapat tersambung kembali secara otomatis?

**Jawaban:** Reconnect otomatis belum tersedia di frontend.

#### 117. Apa kriteria keberhasilan ketika pengguna melakukan reconnect?

**Jawaban:** Reconnect belum diimplementasikan. Kriteria yang dapat diuji saat ini hanya koneksi baru dapat dibuat kembali oleh browser/client; pemulihan event yang terlewat tidak dijamin.

#### 118. Apakah sistem dianggap gagal jika ada update yang hilang, terlambat, atau diterima lebih dari satu kali?

**Jawaban:** Untuk transport saat ini, kehilangan event atau tidak adanya replay merupakan keterbatasan; duplikat settlement dengan reference sama ditolak. Target toleransi loss/latency/duplikasi harus disepakati sebelum menetapkan pass/fail.

### B. Payment Integration

#### 119. Apa indikator bahwa transaksi pembayaran dinyatakan berhasil?

**Jawaban:** Untuk simulasi, indikatornya payload webhook valid, HMAC valid, dan status `SETTLEMENT` berhasil disimpan. Ini tidak menunjukkan pembayaran provider sungguhan.

#### 120. Apakah transaksi hanya boleh berubah menjadi berhasil setelah callback atau verifikasi payment gateway valid?

**Jawaban:** Untuk webhook simulasi ya: signature valid diperlukan sebelum status diproses. Namun tidak ada callback/provider verification sehingga kriteria provider belum dapat diuji.

#### 121. Bagaimana hasil yang diharapkan ketika callback memiliki signature tidak valid?

**Jawaban:** Harus ditolak HTTP 401 dan tidak boleh membuat record atau menaikkan total; ini perilaku handler yang dapat diuji.

#### 122. Apakah callback duplikat harus menghasilkan satu transaksi donasi saja?

**Jawaban:** Referensi duplikat yang sama harus menghasilkan satu row karena unique constraint. Request berikutnya ditolak; API belum memberi respons idempotent sukses.

#### 123. Apakah nominal pembayaran yang berbeda dari nominal transaksi harus ditolak?

**Jawaban:** Nominal hanya diperiksa positif, bukan dibandingkan dengan intent/nominal order. Perbedaan dari order tidak dapat diuji karena order tidak ada.

#### 124. Apa yang harus terjadi ketika pembayaran berstatus pending?

**Jawaban:** Status `PENDING` tersimpan tetapi tidak menambah total campaign.

#### 125. Berapa lama waktu maksimal yang diharapkan untuk memperbarui status donasi setelah callback diterima?

**Jawaban:** Belum ada target waktu. Handler merespons setelah transaksi database diproses; waktu end-to-end perlu diukur dan disepakati.

#### 126. Bagaimana sistem menentukan bahwa transaksi yang sudah berhasil tidak boleh diproses kembali?

**Jawaban:** Reference unik menolak reference yang sama. Tidak ada idempotency key atau aturan untuk transaksi sama dengan reference berbeda.

#### 127. Apakah terdapat standar keamanan atau persyaratan dari payment gateway yang wajib dipenuhi?

**Jawaban:** Belum ada persyaratan provider karena provider belum dipilih/integrasi. Untuk webhook simulasi, implementasi memakai HMAC-SHA256 dan secret environment; standar provider harus ditambahkan jika integrasi dipilih.

### C. Kriteria Keseluruhan

#### 128. Apakah kelompok E memiliki target performa tertentu untuk sistem?

**Jawaban:** Belum ditetapkan. Tetapkan target performa, response time, pass rate, dan toleransi error sebelum pengujian; jangan mengasumsikan angka tertentu.

#### 129. Berapa response time yang dianggap memenuhi standar?

**Jawaban:** Belum ditetapkan. Tetapkan target performa, response time, pass rate, dan toleransi error sebelum pengujian; jangan mengasumsikan angka tertentu.

#### 130. Berapa persentase test case yang harus lulus?

**Jawaban:** Belum ditetapkan. Tetapkan target performa, response time, pass rate, dan toleransi error sebelum pengujian; jangan mengasumsikan angka tertentu.

#### 131. Apakah terdapat toleransi terhadap error selama pengujian beban?

**Jawaban:** Belum ditetapkan. Tetapkan target performa, response time, pass rate, dan toleransi error sebelum pengujian; jangan mengasumsikan angka tertentu.

#### 132. Apakah terdapat standar keamanan tertentu yang menjadi acuan pengujian?

**Jawaban:** Kontrol yang terlihat mencakup bcrypt, JWT HS256, role check, parameterized SQL, batas ukuran request, HMAC webhook simulasi, dan beberapa security header. Tidak ada standar kepatuhan formal yang dinyatakan di repository.

## 07. Suspension & Resumption Criteria

#### 133. Dalam kondisi apa pengujian harus dihentikan sementara?

**Jawaban:** Hentikan sementara bila service/data uji tidak stabil, total campaign tidak sesuai settlement simulasi, ditemukan akses/aktivitas tidak sah, atau pengujian berisiko mengenai sistem di luar cakupan/izin.

#### 134. Apakah pengujian harus dihentikan ketika server WebSocket mengalami gangguan?

**Jawaban:** Untuk pengujian WebSocket, hentikan atau suspend bila Notification Service tidak sehat atau gangguan mengganggu layanan lain. Catat kondisi; tidak ada failover/replay yang menjamin kontinuitas.

#### 135. Apa yang dilakukan jika koneksi WebSocket gagal secara massal ketika load testing?

**Jawaban:** Hentikan load test, simpan metrik dan waktu kejadian, periksa health service dan resource, lalu lanjutkan hanya setelah developer menyetujui dan kapasitas aman.

#### 136. Apakah pengujian harus dihentikan ketika payment gateway sandbox mengalami gangguan?

**Jawaban:** Tidak ada sandbox provider. Jika kelak sandbox yang disetujui bermasalah, suspend skenario payment dan jangan mengganti dengan production.

#### 137. Bagaimana jika callback pembayaran tidak diterima atau tidak dapat diverifikasi?

**Jawaban:** Webhook simulasi yang gagal verifikasi tidak mengubah data. Catat request uji tanpa secret; jangan menandai pembayaran berhasil. Mekanisme retry/rekonsiliasi provider belum tersedia.

#### 138. Bagaimana jika terjadi ketidaksesuaian antara status transaksi dan jumlah dana kampanye?

**Jawaban:** Suspend skenario terkait, bandingkan rows donations berstatus `SETTLEMENT` dengan nilai `collected` pada database Neon testing, simpan bukti aman, dan minta developer menyelidiki sebelum retest. Jangan menghapus atau me-reset data tanpa persetujuan.

#### 139. Apakah pengujian harus dihentikan jika ditemukan indikasi manipulasi transaksi?

**Jawaban:** Ya. Hentikan skenario terkait, pertahankan bukti yang tidak memuat secret/data sensitif, laporkan kepada PIC, dan jangan lanjutkan payload manipulatif tanpa izin.

#### 140. Siapa yang berwenang memutuskan bahwa pengujian harus dihentikan?

**Jawaban:** Nama pemegang kewenangan belum ditetapkan. Tentukan koordinator Kelompok 5 (E) bersama developer/pemilik service sebelum mulai test.

#### 141. Apa persyaratan agar pengujian dapat dilanjutkan kembali?

**Jawaban:** Lanjutkan setelah penyebab teridentifikasi atau diperbaiki, service sehat, data uji tervalidasi/reset dengan aman, dan koordinator serta developer menyetujui.

#### 142. Apakah data donasi, transaksi, dan progres kampanye harus di-reset sebelum pengujian diulang?

**Jawaban:** Gunakan project/branch atau database Neon khusus testing. Siapkan backup dan prosedur reset terkontrol sebelum pengulangan; jangan menjalankan reset/drop pada database yang berisi data penting atau production.

## 08. Test Deliverables

#### 143. Apakah kelompok E memiliki dokumentasi kebutuhan sistem yang dapat diberikan kepada kelompok penguji?

**Jawaban:** README dan source code tersedia. SRS formal serta dokumen requirement terpisah belum ditemukan.

#### 144. Apakah tersedia dokumentasi alur donasi dari awal hingga pembayaran berhasil?

**Jawaban:** Belum ada alur donasi end-to-end karena frontend tidak menyediakan checkout dan provider belum diintegrasikan. README ini mendokumentasikan alur webhook simulasi.

#### 145. Apakah tersedia dokumentasi arsitektur WebSocket?

**Jawaban:** Belum ada dokumen arsitektur WebSocket khusus. Implementasinya dapat ditelusuri pada `notification-service/main.go` dan pemakaian pada Campaign Service/frontend.

#### 146. Apakah tersedia dokumentasi event atau pesan yang dikirim melalui WebSocket?

**Jawaban:** Belum ada dokumen event terpisah. Event dan payload aktual dirangkum pada README PDJ dan didefinisikan pada source code.

#### 147. Apakah tersedia dokumentasi API dan endpoint pembayaran?

**Jawaban:** OpenAPI/Swagger tidak ada. Endpoint webhook simulasi adalah `POST /webhook/payment`; spesifikasinya dapat ditinjau pada handler Campaign Service.

#### 148. Apakah tersedia dokumentasi integrasi payment gateway?

**Jawaban:** Tidak ada dokumen integrasi payment provider karena provider belum dipilih/diimplementasikan.

#### 149. Apakah tersedia dokumentasi format callback atau webhook beserta contoh payload-nya?

**Jawaban:** Dokumentasi formal belum ada. Handler simulasi mengharapkan JSON dengan `campaign_id`, `donor_name`, `amount`, `reference`, dan `status`, serta `X-Signature` HMAC-SHA256 dari raw body.

#### 150. Apakah tersedia informasi status transaksi yang digunakan sistem?

**Jawaban:** Status tabel `donations` adalah `PENDING`, `SETTLEMENT`, atau `FAILED`. `SETTLEMENT` yang valid menambah `collected`; status `EXPIRED` tidak tersedia.

#### 151. Apakah tersedia akun uji dan data kampanye untuk pengujian?

**Jawaban:** Tidak ada akun/data seed yang disertakan. Siapkan data uji pada database Neon yang dikonfigurasi untuk testing, jalankan migration media jika schema lama sudah ada, dan buat role admin melalui prosedur administrator yang disetujui.

#### 152. Apakah tersedia dokumentasi hasil pengujian sebelumnya?

**Jawaban:** Laporan hasil pengujian sebelumnya tidak ditemukan di repository.

#### 153. Apakah tersedia log koneksi WebSocket dan aktivitas callback pembayaran untuk membantu investigasi bug?

**Jawaban:** Tidak ada dashboard/riwayat event persisten. Gateway menulis routing/error ke stdout; detail log aplikasi dan hasil verifikasi callback tidak disimpan sebagai audit log.

#### 154. Apakah kelompok E memiliki format pelaporan bug tertentu?

**Jawaban:** Format bug standar perlu ditetapkan kelompok. Minimal catat ID, judul, environment/versi, prasyarat, langkah reproduksi, hasil aktual/harapan, severity, bukti aman, dan status.

## 09. Environmental Needs

### A. Lingkungan Sistem

#### 155. Apa URL lingkungan testing atau staging yang dapat digunakan?

**Jawaban:** Environment demo tersedia secara lokal di `http://localhost:5173` (frontend) dan gateway di `http://localhost:8000`. Campaign Service menggunakan Neon sesuai `DATABASE_URL` pada `.env`; URL staging terpisah belum diberikan.

#### 156. Bagaimana cara mengakses lingkungan pengujian, apakah publik, VPN, atau localhost?

**Jawaban:** Jalankan service dengan Docker Compose di komputer lokal. Akses database memakai connection string Neon pada `.env`; URL publik, VPN, atau staging belum ditetapkan.

#### 157. Apakah sistem sudah dapat dijalankan tanpa bantuan developer?

**Jawaban:** Repository menyediakan Docker Compose. Siapkan `.env` berisi `DATABASE_URL` Neon, `JWT_SECRET`, dan `PAYMENT_WEBHOOK_SECRET`. Pada Neon yang sudah berisi schema lama, jalankan `db/migrations/002_campaign_media.sql`; pada database kosong, jalankan `db/init.sql`. Kesiapan di semua mesin belum divalidasi; ikuti README dan minta bantuan bila setup gagal.

#### 158. Apakah tersedia lingkungan testing yang terpisah dari production?

**Jawaban:** Tidak ada staging terpisah yang dikonfigurasi di repository. Docker Compose berjalan lokal dan terhubung ke database pada `DATABASE_URL`; pastikan URL menunjuk ke project/branch Neon testing yang terpisah, bukan production.

#### 159. Apakah tersedia endpoint khusus untuk pengujian WebSocket?

**Jawaban:** Ya, WebSocket melalui Gateway tersedia pada `ws://localhost:8000/ws/donasi` ketika dijalankan secara lokal.

#### 160. Apakah tersedia endpoint untuk simulasi callback atau webhook pembayaran?

**Jawaban:** Ya, endpoint simulasi tersedia di `POST http://localhost:8000/webhook/payment`; request memerlukan HMAC pada `X-Signature` dan secret lokal.

### B. Tools Pengujian

#### 161. Apakah sistem dapat diuji menggunakan Postman?

**Jawaban:** Bisa. Postman atau klien HTTP sejenis dapat menguji REST endpoint dan mengirim body JSON GraphQL ke `/graphql`. Untuk registrasi/login gunakan JSON; untuk webhook hitung `X-Signature` dari body persis menggunakan secret lokal.

#### 162. Tools apa yang direkomendasikan untuk menguji koneksi WebSocket?

**Jawaban:** Gunakan klien WebSocket yang dapat terkoneksi ke `ws://localhost:8000/ws/donasi` seperti Postman WebSocket, `wscat`, atau alat setara. Tool yang dipakai bukan bagian dari konfigurasi repository.

#### 163. Apakah pengujian WebSocket dapat dilakukan menggunakan k6, JMeter, atau tools lainnya?

**Jawaban:** Secara umum bisa dengan tool yang mendukung WebSocket, tetapi load test hanya setelah target beban, environment, dan izin developer disepakati. Belum ada konfigurasi k6/JMeter di repository.

#### 164. Apakah tersedia dashboard untuk memantau koneksi WebSocket yang aktif?

**Jawaban:** Tidak ada dashboard atau log transaksi/event terpusat di repository. Observasi terbatas pada pesan yang diterima client dan log stdout service; hasil verifikasi webhook tidak dicatat sebagai audit trail.

#### 165. Apakah tersedia log untuk melihat pesan WebSocket yang dikirim dan diterima?

**Jawaban:** Tidak ada dashboard atau log transaksi/event terpusat di repository. Observasi terbatas pada pesan yang diterima client dan log stdout service; hasil verifikasi webhook tidak dicatat sebagai audit trail.

#### 166. Apakah tersedia dashboard atau log transaksi pembayaran?

**Jawaban:** Tidak ada dashboard atau log transaksi/event terpusat di repository. Observasi terbatas pada pesan yang diterima client dan log stdout service; hasil verifikasi webhook tidak dicatat sebagai audit trail.

#### 167. Apakah tersedia fitur untuk melihat hasil verifikasi callback?

**Jawaban:** Tidak ada dashboard atau log transaksi/event terpusat di repository. Observasi terbatas pada pesan yang diterima client dan log stdout service; hasil verifikasi webhook tidak dicatat sebagai audit trail.

### C. Akun dan Data Pengujian

#### 168. Apakah tersedia akun uji untuk setiap role, seperti donatur, pengelola kampanye, dan admin?

**Jawaban:** Tidak ada akun uji bawaan untuk setiap role. Registrasi publik hanya membuat `DONATUR`; `ADMIN`/`PENGELOLA` harus dibuat atau diubah oleh administrator database untuk demo.

#### 169. Bagaimana cara kelompok penguji mendapatkan akun untuk setiap role?

**Jawaban:** Minta akun uji kepada PIC yang ditetapkan kelompok. Jika belum ada, buat data hanya pada database Neon testing melalui prosedur admin; jangan berbagi kredensial atau memakai akun production.

#### 170. Apakah tersedia data kampanye dengan target dan nominal donasi tertentu?

**Jawaban:** Tidak ada seed campaign. Campaign dapat dibuat dengan akun login melalui GraphQL; pengajuan awal berstatus `MENUNGGU` dan perlu di-approve Admin agar tampil publik.

#### 171. Apakah data donasi dan transaksi dapat di-reset di antara pengujian?

**Jawaban:** Tidak ada fitur reset data dalam aplikasi. Reset hanya melalui prosedur database Neon testing yang disetujui dan setelah memastikan project/branch benar-benar khusus pengujian.

#### 172. Apakah tersedia metode pembayaran sandbox?

**Jawaban:** Tidak ada metode sandbox provider. Webhook internal simulasi bukan sandbox payment gateway.

#### 173. Apakah kelompok penguji diperbolehkan melakukan simulasi pembayaran berulang kali?

**Jawaban:** Pengulangan webhook simulasi hanya pada environment lokal, dengan reference baru untuk transaksi baru. Reference sama ditolak. Dapatkan persetujuan developer untuk uji callback berulang/negatif.

#### 174. Apakah terdapat batas jumlah transaksi yang boleh dilakukan selama pengujian?

**Jawaban:** Batas jumlah transaksi tidak ditentukan. Batasi pada data uji lokal; tetapkan limit dan izin sebelum load test.

### D. Payment Security

#### 175. Apakah payment gateway menggunakan mode sandbox atau mock payment?

**Jawaban:** Tidak ada mode sandbox payment gateway. Yang ada hanya endpoint webhook simulasi dengan HMAC.

#### 176. Apakah tersedia dokumentasi cara mengirim simulasi callback?

**Jawaban:** Panduan formal belum tersedia. Untuk pengujian lokal, kirim JSON payload yang sesuai schema ke `/webhook/payment` dan isi `X-Signature` dengan HMAC-SHA256 dari raw body memakai `PAYMENT_WEBHOOK_SECRET` lokal.

#### 177. Apakah tersedia contoh callback yang valid dan tidak valid?

**Jawaban:** Contoh payload valid mengikuti field `campaign_id`, `donor_name`, `amount`, `reference`, `status`; status harus salah satu `PENDING`, `SETTLEMENT`, atau `FAILED`, campaign harus ada, dan signature harus benar. Contoh invalid dapat berupa signature yang salah atau field/status invalid. Lakukan hanya di lokal.

#### 178. Apakah kelompok penguji dapat melakukan simulasi callback berulang?

**Jawaban:** Ya, di lokal dengan izin. Mengulang reference sama harus ditolak karena constraint unik; jangan lakukan ke provider/production.

#### 179. Apakah terdapat batasan untuk pengujian manipulasi payload callback?

**Jawaban:** Manipulasi payload hanya di environment lokal dan dengan persetujuan developer. Jangan kirim payload uji ke endpoint eksternal; jangan gunakan secret nyata.

#### 180. Apakah pengujian dapat dilakukan tanpa menggunakan data kartu atau informasi pembayaran sungguhan?

**Jawaban:** Ya. Webhook simulasi tidak meminta data kartu atau informasi payment method. Gunakan data donor dummy, bukan data pribadi nyata.

## 10. Staffing & Responsibilities

#### 181. Siapa anggota kelompok E yang bertanggung jawab terhadap implementasi WebSocket?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 182. Siapa yang bertanggung jawab terhadap integrasi payment gateway?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 183. Siapa yang memahami keseluruhan alur transaksi donasi?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 184. Siapa yang dapat menjelaskan mekanisme autentikasi dan keamanan sistem?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 185. Siapa yang bertanggung jawab memberikan akses aplikasi dan dokumentasi?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 186. Siapa yang dapat membantu ketika ditemukan masalah pada koneksi WebSocket?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 187. Siapa yang dapat membantu ketika ditemukan masalah pada integrasi pembayaran?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 188. Siapa yang bertanggung jawab melakukan perbaikan bug?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

#### 189. Siapa kontak utama yang bisa dihubungi oleh kelompok penguji selama proses pengujian?

**Jawaban:** Nama PIC untuk tanggung jawab ini belum ditetapkan. Anggota yang tercantum adalah Elizabeth Yulita Abadi, Audrey Reymond, Matthew Jeremy, dan Andi Andreas; kelompok perlu menunjuk PIC spesifik dan kontak utama sebelum pengujian.

## 11. Schedule

#### 190. Kapan sistem diperkirakan siap untuk diuji?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 191. Kapan dokumentasi sistem akan diserahkan kepada kelompok penguji?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 192. Kapan akses ke lingkungan testing dapat diberikan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 193. Kapan akun dan data pengujian dapat disediakan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 194. Kapan pengujian fungsional WebSocket dapat dilakukan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 195. Kapan pengujian performa dan koneksi WebSocket secara bersamaan dapat dilakukan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 196. Kapan pengujian keamanan integrasi payment dapat dilakukan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 197. Berapa lama waktu yang disediakan untuk melakukan pengujian?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 198. Kapan hasil pengujian harus diserahkan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 199. Kapan kelompok E dapat mulai melakukan perbaikan bug?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 200. Berapa lama waktu yang disepakati untuk perbaikan bug menjelang minggu ke-14?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

#### 201. Kapan jadwal retest dapat dilakukan?

**Jawaban:** Tanggal/jangka waktu untuk hal ini belum dicantumkan di repository. Tetapkan bersama kelompok dan penguji, lalu dokumentasikan deadline, akses, durasi, serta jadwal retest sebelum test execution.

## 12. Risks & Contingencies

### A. Risiko WebSocket

#### 202. Apa yang terjadi jika koneksi WebSocket terputus ketika pengguna sedang membuka halaman kampanye?

**Jawaban:** Server menghapus dan menutup koneksi yang putus. Frontend tidak mencoba reconnect otomatis dan event selama offline tidak direplay.

#### 203. Apakah ada kemungkinan pembaruan progres donasi tidak diterima oleh pengguna?

**Jawaban:** Ya. Client yang tidak tersambung saat broadcast dapat melewatkan event; tidak ada mekanisme acknowledgement, queue, atau replay.

#### 204. Bagaimana jika pengguna menerima update yang sama lebih dari satu kali?

**Jawaban:** Tidak ada ID event atau deduplikasi client. Sistem tidak menjamin penanganan event duplikat secara end-to-end.

#### 205. Bagaimana jika update diterima dalam urutan yang tidak sesuai?

**Jawaban:** Urutan event tidak diberi sequence number atau jaminan pemrosesan global. Gunakan query database sebagai sumber kebenaran, bukan akumulasi event di client.

#### 206. Apa yang terjadi jika jumlah koneksi WebSocket meningkat secara drastis?

**Jawaban:** Kapasitas dan perilaku overload belum diuji/ditentukan. Batasi pengujian pada environment lokal yang disetujui; pantau resource dan hentikan jika service tidak sehat.

#### 207. Bagaimana jika server WebSocket mengalami overload?

**Jawaban:** Kapasitas dan perilaku overload belum diuji/ditentukan. Batasi pengujian pada environment lokal yang disetujui; pantau resource dan hentikan jika service tidak sehat.

#### 208. Apakah terdapat kemungkinan pengguna yang tidak berwenang mengakses informasi kampanye atau donasi?

**Jawaban:** Campaign publik memang dapat dilihat tanpa login. Namun WebSocket saat ini menyiarkan semua event, termasuk nama dan jumlah donasi, tanpa autentikasi/filter. Ini risiko privasi yang harus dicatat; jangan menganggap akses event dibatasi.

#### 209. Bagaimana jika pengguna kehilangan koneksi dan melakukan reconnect setelah beberapa transaksi donasi terjadi?

**Jawaban:** Tidak ada reconnect otomatis atau replay. Client dapat memuat ulang halaman untuk memperoleh daftar campaign terkini, tetapi event yang terlewat tidak dipulihkan dan angka tidak otomatis sinkron setelah reconnect.

### B. Risiko Payment Integration

#### 210. Apa yang terjadi jika payment gateway tidak dapat diakses?

**Jawaban:** Tidak berlaku untuk provider karena tidak ada koneksi provider. Jika Gateway atau Campaign Service tidak tersedia, request lokal gagal; health/registry dapat dipakai untuk pemeriksaan service.

#### 211. Bagaimana jika pembayaran berhasil di payment gateway tetapi callback tidak sampai ke sistem?

**Jawaban:** Belum ada skenario provider/callback eksternal. Untuk webhook simulasi yang tidak sampai, tidak ada retry/reconciliation otomatis; operator perlu mengirim ulang secara aman setelah memeriksa data dan reference.

#### 212. Bagaimana jika callback dikirimkan lebih dari satu kali?

**Jawaban:** Reference yang sama ditolak oleh unique constraint; tidak ada respons idempotent atau retry provider.

#### 213. Bagaimana jika callback palsu dikirimkan oleh pihak yang tidak berwenang?

**Jawaban:** Webhook dengan signature tidak valid ditolak. Pihak yang memiliki secret webhook dapat menghasilkan HMAC valid; jaga secret dan batasi akses endpoint.

#### 214. Bagaimana jika signature callback tidak valid?

**Jawaban:** Handler membandingkan signature dan mengembalikan HTTP 401 bila tidak valid; tidak ada perubahan database untuk request tersebut.

#### 215. Bagaimana jika nominal transaksi diubah oleh pengguna?

**Jawaban:** Amount diperiksa positif, tetapi tidak dibandingkan dengan order yang telah dibuat. Karena checkout tidak ada, perlindungan terhadap nominal berbeda dari order belum terimplementasi.

#### 216. Bagaimana jika callback diterima sebelum sistem menyimpan data transaksi?

**Jawaban:** Tidak ada payment intent/order yang perlu disimpan dahulu; handler langsung mencoba insert donation dengan reference unik dan foreign key campaign. Callback provider out-of-order belum ditangani.

#### 217. Bagaimana jika status pembayaran di payment gateway berbeda dengan status donasi di aplikasi?

**Jawaban:** Tidak ada provider untuk dibandingkan dan tidak ada rekonsiliasi status. Status di aplikasi berasal dari webhook simulasi yang ditandatangani.

#### 218. Apakah ada risiko data sensitif pembayaran terekspos melalui log atau komunikasi API?

**Jawaban:** Payload WebSocket mencakup nama donor dan nominal; gateway dan aplikasi belum memiliki audit log terpusat. Gunakan data dummy, batasi akses, dan jangan menaruh secret/token di log atau bukti.

#### 219. Bagaimana jika API key atau secret key payment gateway bocor?

**Jawaban:** API key provider belum digunakan. Jika secret webhook/JWT bocor, hentikan pengujian, beri tahu PIC, rotasi secret melalui environment, lalu validasi konfigurasi sebelum melanjutkan. Prosedur rotasi formal perlu disepakati.

### C. Risiko Keseluruhan Sistem

#### 220. Apa risiko terbesar ketika banyak pengguna melakukan donasi secara bersamaan?

**Jawaban:** Risiko utama adalah beban database/service dan race/update UI yang belum diuji. Settlement diperbarui dengan operasi SQL transaksional, tetapi tidak ada bukti kapasitas concurrent/load test.

#### 221. Apakah pernah ditemukan masalah ketika sistem diuji dengan banyak koneksi WebSocket?

**Jawaban:** Belum ada hasil atau bukti pengujian banyak koneksi WebSocket dalam repository.

#### 222. Apakah pernah terjadi ketidaksesuaian antara total donasi dengan jumlah transaksi berhasil?

**Jawaban:** Belum ada laporan pengujian konsistensi historis. Untuk simulasi, verifikasi total campaign terhadap record `SETTLEMENT` di database Neon testing.

#### 223. Apakah terdapat mekanisme backup dan recovery apabila terjadi kegagalan sistem?

**Jawaban:** Backup dan uji recovery Neon belum didokumentasikan dalam repository. Pastikan fitur backup/restore sesuai paket Neon yang digunakan dan uji pemulihan di database testing; konfigurasi aplikasi tidak dengan sendirinya menyediakan backup.

#### 224. Bagaimana cara memastikan pengujian tidak memengaruhi data transaksi yang sebenarnya?

**Jawaban:** Jalankan hanya terhadap database/project Neon khusus testing yang sudah dikonfirmasi, gunakan akun/payload dummy, pisahkan secret pengujian, dan jangan mengarahkan tool atau reset ke production.

## 13. Approvals

#### 225. Siapa yang berwenang memberikan persetujuan bahwa sistem siap diuji?

**Jawaban:** Pemberi persetujuan belum ditetapkan. Kelompok harus menunjuk pemilik aplikasi/developer dan koordinator penguji yang menyatakan environment serta cakupan siap.

#### 226. Apakah pengujian WebSocket dan payment security memerlukan persetujuan khusus dari developer?

**Jawaban:** Ya. Minta persetujuan developer untuk pengujian WebSocket/security yang melibatkan payload negatif atau beban. Pengujian fitur normal pada lokal tetap harus mengikuti aturan kelompok.

#### 227. Apakah kelompok penguji diperbolehkan melakukan load testing pada koneksi WebSocket?

**Jawaban:** diperbolehkan

#### 228. Apakah kelompok penguji diperbolehkan melakukan simulasi callback palsu dan callback berulang di lingkungan sandbox?

**Jawaban:** Hanya pada endpoint simulasi lokal, setelah persetujuan developer. Jangan mengirim callback palsu/berulang ke payment provider atau production.

#### 229. Apakah terdapat batasan pengujian keamanan yang harus disetujui terlebih dahulu?

**Jawaban:** Ya. Batas environment, data, volume trafik, manipulasi payload, perlindungan secret, dan prosedur penghentian harus disetujui sebelum pengujian keamanan.

#### 230. Apakah Test Plan harus dikonfirmasi oleh kelompok E sebelum pengujian dilakukan?

**Jawaban:** Ya. Kelompok E perlu meninjau dan mengonfirmasi cakupan serta kriteria Test Plan sebelum test execution, terutama karena beberapa skenario provider belum didukung.

#### 231. Siapa yang akan memvalidasi bahwa bug yang ditemukan sudah diperbaiki?

**Jawaban:** tentunya salah satu dari kelompok kami.

#### 232. Siapa yang memberikan persetujuan bahwa pengujian telah selesai?

**Jawaban:** Seluruh anggota kelompok

## Catatan penutup

Informasi PIC, jadwal, target performa, persetujuan, serta hasil pengujian sebelumnya tidak ditetapkan oleh source code dan harus diisi/disahkan oleh kelompok. Jawaban ini menjelaskan implementasi saat ini, bukan bukti bahwa seluruh test case sudah dijalankan.
