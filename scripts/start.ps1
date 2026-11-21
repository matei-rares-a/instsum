$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$logDir = Join-Path $root 'logs'
$logPath = Join-Path $logDir ("instsum-{0}-{1}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'), [guid]::NewGuid().ToString('N'))
$writer = $null
$buildDir = $null
$configDir = $null
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
    $urlFile = Join-Path $root 'start-url.txt'
    $inputUrl = $null
    if (Test-Path -LiteralPath $urlFile -PathType Leaf) {
        $inputUrl = [IO.File]::ReadAllText($urlFile).Trim()
        if (-not [string]::IsNullOrWhiteSpace($inputUrl)) {
            Write-LogLine 'URL incarcat din start-url.txt.'
        }
    }
    if ([string]::IsNullOrWhiteSpace($inputUrl)) {
        Write-Host 'Lipeste URL-ul postarii sau reelului Instagram si apasa Enter:'
        $inputUrl = [Console]::ReadLine()
    }
    if ([string]::IsNullOrWhiteSpace($inputUrl)) {
        Write-LogLine 'Eroare: nu ai introdus un URL.'
        $exitCode = 64
    } else {
        $runtime = Join-Path $root 'runtime\setup-v1'
        $ready = Join-Path $runtime 'ready'
        $args = @($inputUrl.Trim())
        $exe = Join-Path $root 'instsum.exe'
        if (Test-Path -LiteralPath $ready -PathType Leaf) {
            if (([IO.File]::ReadAllText($ready)).Trim() -ne 'setup-v1') {
                throw "Marker setup invalid: $ready. Ruleaza setup.bat din nou."
            }
            $exe = Join-Path $runtime 'instsum.exe'
            $prefix = Join-Path $runtime 'env'
            $python = Join-Path $prefix 'python.exe'
            $helper = Join-Path $PSScriptRoot 'setup-config.py'
            foreach ($file in @($exe, $python, $helper, (Join-Path $root 'configs\config.yaml'))) {
                if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
                    throw "Instalare locala incompleta: lipseste $file. Ruleaza setup.bat din nou."
                }
            }
            $env:PATH = (Join-Path $prefix 'Library\bin') + ';' + (Join-Path $prefix 'Scripts') + ';' + $prefix + ';' + (Join-Path $runtime 'llama') + ';' + (Join-Path $runtime 'cudart') + ';' + (Join-Path $runtime 'whisper\Release') + ';' + $env:PATH
            $env:TESSDATA_PREFIX = Join-Path $runtime 'tessdata'
            $configDir = Join-Path ([System.IO.Path]::GetTempPath()) ("instsum-config-" + [guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path $configDir | Out-Null
            $config = Join-Path $configDir 'config.yaml'
            Write-LogLine 'Se pregateste configuratia locala din configs\config.yaml...'
            $configExit = Invoke-LoggedCommand $python @($helper, (Join-Path $root 'configs\config.yaml'), $config, $runtime)
            if ($configExit -ne 0 -or -not (Test-Path -LiteralPath $config -PathType Leaf)) {
                throw "Generarea configuratiei locale a esuat. Cod: $configExit"
            }
            $args = @('--config', $config, $inputUrl.Trim())
        } elseif (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
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
            $exitCode = Invoke-LoggedCommand $exe $args
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
    if ($configDir -and (Test-Path -LiteralPath $configDir)) {
        try { Remove-Item -LiteralPath $configDir -Recurse -Force } catch {
            [Console]::Error.WriteLine("Nu s-a putut elimina configuratia temporara $configDir : $($_.Exception.Message)")
            try { Write-LogLine "Eroare la curatarea configuratiei temporare: $($_.Exception.Message)" } catch {
                [Console]::Error.WriteLine("Nu se poate scrie in jurnal: $($_.Exception.Message)")
            }
            $exitCode = 1
        }
    }
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
