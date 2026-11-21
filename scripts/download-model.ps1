param(
    [Parameter(Mandatory=$true)]
    [ValidateSet('hf','local','mirror-url')]
    [string]$Source,
    [Parameter(Mandatory=$true)]
    [string]$From,
    [Parameter(Mandatory=$true)]
    [string]$Destination,
    [string]$Sha256 = ''
)
$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath $Destination) { throw "Destination already exists: $Destination" }
if ($Source -eq 'local') {
    Copy-Item -LiteralPath $From -Destination $Destination
} else {
    if (!$Sha256) { throw 'Remote downloads require the expected SHA-256 via -Sha256.' }
    if ($Source -eq 'hf') {
        # -From is repository/resolve/revision/path, e.g. org/model/resolve/main/model.gguf.
        $uri = "https://huggingface.co/$From"
    } else {
        $uri = $From
    }
    if (-not $uri.StartsWith('https://')) { throw 'Remote downloads must use HTTPS.' }
    Invoke-WebRequest -Uri $uri -OutFile $Destination
}
if ($Sha256 -and (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash -ne $Sha256) {
    throw "SHA-256 mismatch for $Destination. The downloaded file was left in place for inspection; do not use it."
}
Write-Host "Model saved to $Destination; update the matching models path in configs/config.yaml."
