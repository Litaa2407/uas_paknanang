$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent $PSScriptRoot
$CertDir = Join-Path $RootDir "certs"
if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    throw "OpenSSL belum ditemukan. Install OpenSSL lalu buka PowerShell baru."
}

New-Item -ItemType Directory -Force -Path $CertDir | Out-Null
@("ca.key", "ca.crt", "ca.srl", "campaign.*", "notification.*", "gateway.*") | ForEach-Object {
    Get-ChildItem -Path (Join-Path $CertDir $_) -ErrorAction SilentlyContinue | Remove-Item -Force -Recurse
}

openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes `
    -keyout (Join-Path $CertDir "ca.key") `
    -out (Join-Path $CertDir "ca.crt") `
    -subj "/CN=Crowdfunding Local Development CA"
if ($LASTEXITCODE -ne 0) { throw "Gagal membuat CA certificate." }

function New-SignedCertificate([string]$Name, [string]$CommonName, [string]$Extensions) {
    $key = Join-Path $CertDir "$Name.key"
    $csr = Join-Path $CertDir "$Name.csr"
    $crt = Join-Path $CertDir "$Name.crt"
    $ext = Join-Path $CertDir "$Name.ext"
    Set-Content -Path $ext -Value $Extensions -Encoding ascii

    openssl req -new -newkey rsa:2048 -nodes -keyout $key -out $csr -subj "/CN=$CommonName"
    if ($LASTEXITCODE -ne 0) { throw "Gagal membuat CSR $Name." }
    openssl x509 -req -in $csr `
        -CA (Join-Path $CertDir "ca.crt") `
        -CAkey (Join-Path $CertDir "ca.key") `
        -CAcreateserial -out $crt -days 825 -sha256 -extfile $ext
    if ($LASTEXITCODE -ne 0) { throw "Gagal menandatangani certificate $Name." }
    Remove-Item $csr, $ext -Force
}

New-SignedCertificate "campaign" "campaign-service" @"
subjectAltName=DNS:campaign-service,DNS:campaign-service-2,DNS:localhost,IP:127.0.0.1
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
"@
New-SignedCertificate "notification" "notification-service" @"
subjectAltName=DNS:notification-service,DNS:localhost,IP:127.0.0.1
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
"@
New-SignedCertificate "gateway" "gateway" @"
subjectAltName=DNS:gateway
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
"@

Remove-Item (Join-Path $CertDir "*.srl") -Force -ErrorAction SilentlyContinue
Write-Host "Sertifikat development dibuat di: $CertDir"
Write-Warning "Jangan commit folder certs/ atau gunakan sertifikat ini untuk production."
