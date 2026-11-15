# Panduan Proyek Crowdfunding — Kelompok E

Panduan instalasi lama yang menjalankan service secara manual dan menggunakan Neon sudah tidak berlaku untuk konfigurasi Code Freeze Week 12.

Gunakan dokumentasi berikut sebagai acuan:

- [`README.md`](README.md) — instalasi, konfigurasi `.env`, pembuatan sertifikat mTLS, Docker Compose, dan deployment lokal.
- [`API.md`](API.md) — dokumentasi endpoint REST, GraphQL, OAuth2 internal, webhook, dan WebSocket.

Seluruh backend pada konfigurasi Code Freeze dijalankan melalui Docker Compose. Jangan menjalankan service secara manual bersamaan dengan container karena dapat menyebabkan konflik port atau konfigurasi yang berbeda.
