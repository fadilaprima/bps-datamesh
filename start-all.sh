#!/bin/bash

echo "=========================================="
echo " MENYALAKAN SELURUH DATA MESH DTSEN BPS RI "
echo "=========================================="

# 1. Daftar 8 Domain + 1 Stitching
SERVICES=("kependudukan" "pendidikan" "kesehatan" "ketenagakerjaan" "kesejahteraan" "wilayah" "hunian" "energi" "data-sosial-ekonomi")

for s in "${SERVICES[@]}"; do
    echo "[+] Menyalakan service: $s ..."
    cd ~/bps-datamesh/services/$s

    # salin db.env 
    if [ -f "../../db.env" ]; then
        cp ../../db.env .env
        set -a
        source .env
        set +a
    fi

    # Jalan di background
    nohup go run main.go > output.log 2>&1 &

    cd ~/bps-datamesh
done

echo "=========================================="
echo " SEMUA LAYANAN SUKSES DIJALANKAN DI AZURE! "
echo "=========================================="