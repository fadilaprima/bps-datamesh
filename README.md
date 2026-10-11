# Data Mesh DTSEN

API arsitektur **Data Mesh** berbasis **microservices** (Go/GoFiber, PostgreSQL) dengan pendekatan **Domain-Driven Design** dan **contract-first design (OpenAPI 3.0.3)**. Sistem ini memecah `wide table` monolitik Data Tunggal Sosial Ekonomi Nasional (DTSEN) BPS RI menjadi **9 domain data otonom**.

Dikembangkan sebagai skripsi Program Studi D-IV Komputasi Statistik, Politeknik Statistika STIS.

- **Judul:** Perancangan dan Implementasi Arsitektur Data Mesh Pendekatan Domain-Driven Design pada Microservices (Studi Kasus: DTSEN BPS RI)
- **Pengembang:** Fadila Prima Rahayu


## Latar Belakang Singkat

DTSEN mengelola data dalam `wide table` terpusat berisi ratusan variabel heterogen. Struktur ini menimbulkan *tight coupling* pada level data, *bottleneck* integrasi, dan *single point of failure*. Repositori ini memecahnya menjadi domain data otonom, masing-masing dengan basis data, layanan, dan kontrak API sendiri.

## Domain

| Subdomain | Domain |
|---|---|
| Generic | Wilayah, Kependudukan |
| Supporting | Kesejahteraan, Hunian, Energi, Kesehatan, Pendidikan, Ketenagakerjaan |
| Core | Data Sosial Ekonomi (menggabungkan data delapan domain lewat API / *data stitching*) |

Prinsip utama: satu server basis data per layanan (*database server per microservice*), relasi lintas domain lewat *logical foreign key* (NIK dan Nomor KK), dan komunikasi antar-layanan melalui REST API sesuai kontrak OpenAPI.

## Memulai Cepat

```bash
git clone https://github.com/fadilaprima/bps-datamesh.git
cd bps-datamesh

# siapkan kredensial (lihat halaman Wiki "Konfigurasi")
cp db.env.example db.env

chmod +x start-all.sh
./start-all.sh
```

Skema basis data dibuat otomatis (*auto-migrate*) saat layanan pertama kali berjalan. Panduan lengkap ada di Wiki.

## Dokumentasi

Panduan lengkap tersedia di **[Wiki](../../wiki)**:

- [Instalasi dan Deployment](../../wiki/Instalasi-dan-Deployment)
- [Konfigurasi](../../wiki/Konfigurasi)
- [Panduan API](../../wiki/Panduan-API)
- [Tata Kelola Data](../../wiki/Tata-Kelola-Data)
- [Pengujian](../../wiki/Pengujian)
- [Troubleshooting](../../wiki/Troubleshooting)
- [Menambah Domain Baru](../../wiki/Menambah-Domain-Baru)

## Hasil Pengujian

| Jenis | Cakupan | Hasil |
|---|---|---|
| Black-box | 268 skenario, 9 domain | 100% lolos |
| Integration | rantai 7 langkah API dan *data stitching* 8 domain | seluruhnya lolos |
| API contract (OAS 3.0.3) | 105 endpoint, 420 assertion | 100% lolos |

## Keterbatasan

Komunikasi antar-layanan masih REST sinkron, belum ada *load testing*, dan pola seperti *messaging*, *saga*, *circuit breaker*, dan *API gateway* belum diterapkan. Detail ada di skripsi.
