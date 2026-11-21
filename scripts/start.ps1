$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$logDir = Join-Path $root 'logs'
$logPath = Join-Path $logDir ("instsum-{0}-{1}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'), [guid]::NewGuid().ToString('N'))
$writer = $null
$buildDir = $null
$exitCode = 1

function Write-LogLine([string]$message) {
    $ErrorActionPreference = 'Stop'
    Write-Host $message
    $script:writer.WriteLine($message)
    $script:writer.Flush()
}

function Invoke-LoggedCommand([string]$program, [string[]]$arguments) {
    $previousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & $program @arguments 2>&1 | ForEach-Object {
            Write-LogLine ([string]$_)
        }
        return $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousPreference
    }
}

try {
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    $writer = [System.IO.StreamWriter]::new($logPath, $false, [System.Text.UTF8Encoding]::new($false))
} catch {
    [Console]::Error.WriteLine("Nu se poate crea jurnalul in $logDir : $($_.Exception.Message)")
    exit 1
}

try {
    Set-Location -LiteralPath $root
    Write-LogLine ("Pornire: {0}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'))
    Write-LogLine "Jurnal: $logPath"
    Write-Host 'Lipeste URL-ul postarii sau reelului Instagram si apasa Enter:'
    $inputUrl = [Console]::ReadLine()
    if ([string]::IsNullOrWhiteSpace($inputUrl)) {
        Write-LogLine 'Eroare: nu ai introdus un URL.'
        $exitCode = 64
    } else {
        $exe = Join-Path $root 'instsum.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
            $go = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
            if (-not $go) {
                Write-LogLine 'Eroare: instsum.exe nu exista si Go nu este disponibil in PATH. Instaleaza Go 1.23+ sau construieste instsum.exe.'
            } else {
                $buildDir = Join-Path ([System.IO.Path]::GetTempPath()) ("instsum-build-" + [guid]::NewGuid().ToString('N'))
                New-Item -ItemType Directory -Path $buildDir | Out-Null
                $exe = Join-Path $buildDir 'instsum.exe'
                Write-LogLine 'Se construieste aplicatia cu Go...'
                $buildExit = Invoke-LoggedCommand $go.Source @('build', '-o', $exe, '.\cmd\instsum')
                if ($buildExit -ne 0) {
                    Write-LogLine "Eroare la compilare. Cod: $buildExit"
                    $exe = $null
                }
            }
        }
        if ($exe -and (Test-Path -LiteralPath $exe -PathType Leaf)) {
            Write-LogLine 'Se proceseaza postarea...'
            $exitCode = Invoke-LoggedCommand $exe @($inputUrl.Trim())
            switch ($exitCode) {
                0 { Write-LogLine 'Procesare completa.' }
                2 { Write-LogLine 'Rezultatul este partial. Verifica avertismentele din jurnal si rezultatul exportat.' }
                default { Write-LogLine "Procesarea a esuat. Cod: $exitCode" }
            }
        }
    }
} catch {
    $exitCode = 1
    $message = "Eroare la pornire sau jurnalizare: $($_.Exception.Message)"
    [Console]::Error.WriteLine($message)
    try {
        if ($writer) {
            $writer.WriteLine($message)
            $writer.Flush()
        }
    } catch {
        [Console]::Error.WriteLine("Nu se poate scrie in jurnal: $($_.Exception.Message)")
    }
} finally {
    if ($buildDir -and (Test-Path -LiteralPath $buildDir)) {
        try {
            Remove-Item -LiteralPath $buildDir -Recurse -Force
        } catch {
            [Console]::Error.WriteLine("Nu s-a putut elimina compilarea temporara $buildDir : $($_.Exception.Message)")
            try { Write-LogLine "Eroare la curatarea compilarii temporare: $($_.Exception.Message)" } catch {
                [Console]::Error.WriteLine("Nu se poate scrie in jurnal: $($_.Exception.Message)")
            }
            $exitCode = 1
        }
    }
    try {
        Write-LogLine ("Incheiere: {0}; cod: {1}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'), $exitCode)
    } catch {
        [Console]::Error.WriteLine("Nu se poate finaliza jurnalul: $($_.Exception.Message)")
        $exitCode = 1
    }
    if ($writer) { $writer.Dispose() }
}
exit $exitCode
