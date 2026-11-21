param([switch]$ProbeOnly)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$root = Split-Path -Parent $PSScriptRoot
$runtime = Join-Path $root 'runtime\setup-v1'
$cache = Join-Path $runtime 'downloads'
$logs = Join-Path $root 'logs'
$logPath = Join-Path $logs ("setup-{0}-{1}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'), [guid]::NewGuid().ToString('N'))
$writer = $null
$lock = $null
$createdReady = $false
$exitCode = 1

function Log([string]$message) {
    $ErrorActionPreference = 'Stop'
    Write-Host $message
    $script:writer.WriteLine($message)
    $script:writer.Flush()
}

function Run([string]$exe, [string[]]$arguments) {
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & $exe @arguments 2>&1 | ForEach-Object { Log ([string]$_) }
        $code = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previous
    }
    if ($code -ne 0) { throw "$exe a esuat cu codul $code." }
}

function Download([string]$url, [string]$path, [string]$sha256) {
    if ($sha256 -notmatch '^[a-fA-F0-9]{64}$') { throw "Checksum SHA-256 invalid pentru $url." }
    if (Test-Path -LiteralPath $path) {
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -eq $sha256) {
            Log "Folosesc fisierul verificat: $path"
            return
        }
        throw "Fisierul existent nu corespunde checksum-ului: $path. Nu a fost suprascris."
    }
    $part = "$path.$([guid]::NewGuid().ToString('N')).part"
    try {
        Log "Descarc: $url"
        Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $part
        $actual = (Get-FileHash -LiteralPath $part -Algorithm SHA256).Hash
        if ($actual -ne $sha256) { throw "Checksum diferit pentru $url. Asteptat: $sha256; obtinut: $actual." }
        Move-Item -LiteralPath $part -Destination $path
    } finally {
        if (Test-Path -LiteralPath $part) { Remove-Item -LiteralPath $part -Force }
    }
}

function Expand-VerifiedZip([string]$archive, [string]$destination, [string]$required) {
    if (Test-Path -LiteralPath $destination) {
        if (Test-Path -LiteralPath (Join-Path $destination $required)) { return }
        throw "Director incomplet sau necunoscut: $destination. Nu a fost modificat."
    }
    $stage = "$destination.$([guid]::NewGuid().ToString('N')).tmp"
    try {
        New-Item -ItemType Directory -Path $stage | Out-Null
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead($archive)
        try {
            $prefix = [IO.Path]::GetFullPath($stage).TrimEnd('\') + '\'
            foreach ($entry in $zip.Entries) {
                $target = [IO.Path]::GetFullPath((Join-Path $stage $entry.FullName))
                if (-not $target.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
                    throw "Cale nesigura in arhiva: $($entry.FullName)"
                }
            }
        } finally { $zip.Dispose() }
        Expand-Archive -LiteralPath $archive -DestinationPath $stage
        if (-not (Test-Path -LiteralPath (Join-Path $stage $required))) { throw "Arhiva nu contine $required." }
        Move-Item -LiteralPath $stage -Destination $destination
    } finally {
        if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
    }
}

function Get-ModelMetadata([string]$repo) {
    $uri = "https://huggingface.co/api/models/$repo`?blobs=true"
    Log "Verific metadatele oficiale: $uri"
    try {
        $response = Invoke-RestMethod -Uri $uri -Headers @{'User-Agent'='instsum-setup'} -TimeoutSec 30
    } catch {
        throw "Nu pot accesa metadatele modelului $repo : $($_.Exception.Message)"
    }
    if ($response.sha -notmatch '^[a-f0-9]{40}$') { throw "Revizie invalida pentru $repo." }
    return $response
}

function Select-ModelFile($metadata, [string]$pattern, [string]$description) {
    $matches = @($metadata.siblings | Where-Object { $_.rfilename -match $pattern })
    if ($matches.Count -ne 1) { throw "Nu pot selecta $description : am gasit $($matches.Count) fisiere potrivite." }
    $item = $matches[0]
    if ($item.lfs.sha256 -notmatch '^[a-fA-F0-9]{64}$' -or [long]$item.size -le 0) {
        throw "Lipsesc SHA-256 sau dimensiunea pentru $description ($($item.rfilename))."
    }
    return [pscustomobject]@{ name = $item.rfilename; sha256 = $item.lfs.sha256; size = [long]$item.size }
}

function ModelUrl([string]$repo, [string]$revision, [string]$name) {
    $escaped = ($name.Split('/') | ForEach-Object { [Uri]::EscapeDataString($_) }) -join '/'
    return "https://huggingface.co/$repo/resolve/$revision/$escaped"
}

function Get-PrimaryGpuMemoryMB([string[]]$lines) {
    if ($lines.Count -eq 0) { throw 'nvidia-smi nu a raportat niciun GPU.' }
    $parts = $lines[0].Split(',')
    if ($parts.Count -lt 3) { throw 'Nu pot interpreta datele NVIDIA.' }
    return [double]::Parse($parts[1].Trim(), [Globalization.CultureInfo]::InvariantCulture) * 1.048576
}

try {
    New-Item -ItemType Directory -Force -Path $logs | Out-Null
    $writer = [IO.StreamWriter]::new($logPath, $false, [Text.UTF8Encoding]::new($false))
} catch {
    [Console]::Error.WriteLine("Nu se poate crea jurnalul setup: $($_.Exception.Message)")
    exit 1
}

try {
    Set-Location -LiteralPath $root
    Log ("Setup experimental - {0}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'))
    Log "Jurnal: $logPath"
    Log 'Fisierele pot contine cai personale. Verifica jurnalul inainte de a-l trimite.'
    if (-not [Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
        throw 'Este necesar Windows x64.'
    }
    $os = Get-CimInstance Win32_OperatingSystem
    $availableGB = [math]::Round($os.FreePhysicalMemory / 1MB, 2)
    Log "RAM disponibil aproximativ: $availableGB GiB."
    $nvidia = Get-Command nvidia-smi.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $nvidia) { throw 'Driverul NVIDIA sau nvidia-smi nu este disponibil. Instaleaza driverul NVIDIA pentru placa video.' }
    $gpuLines = @(& $nvidia.Source --query-gpu=name,memory.total,driver_version --format=csv,noheader,nounits 2>&1)
    if ($LASTEXITCODE -ne 0) { throw "nvidia-smi a esuat: $gpuLines" }
    foreach ($line in $gpuLines) { Log "GPU: $line" }
    $gpuMB = Get-PrimaryGpuMemoryMB $gpuLines
    if ($gpuMB -lt 9000) { throw "Primul GPU are $([math]::Round($gpuMB)) MB; bugetul configurat este 9000 MB." }

    $visionRepo = 'ggml-org/Qwen2.5-VL-7B-Instruct-GGUF'
    $whisperRepo = 'ggerganov/whisper.cpp'
    $vision = Get-ModelMetadata $visionRepo
    $whisper = Get-ModelMetadata $whisperRepo
    $model = Select-ModelFile $vision '(?i)(^|/)Qwen2\.5-VL-7B-Instruct-Q4_K_M\.gguf$' 'modelul vision Q4_K_M'
    $projector = Select-ModelFile $vision '(?i)(^|/)mmproj.*f16\.gguf$' 'projectorul vision F16'
    $speech = Select-ModelFile $whisper '(^|/)ggml-large-v3-turbo\.bin$' 'modelul Whisper'
    foreach ($item in @($model, $projector, $speech)) {
        Log ("Model: {0}; {1:N2} GiB; SHA-256 {2}" -f $item.name, ($item.size / 1GB), $item.sha256)
    }
    Log "Revizii Hugging Face: vision=$($vision.sha); whisper=$($whisper.sha)"
    $requiredBytes = $model.size + $projector.size + $speech.size + 10GB
    $driveRoot = [IO.Path]::GetPathRoot($root)
    $freeBytes = ([IO.DriveInfo]::new($driveRoot)).AvailableFreeSpace
    Log ("Spatiu liber: {0:N2} GiB; necesar estimat inclusiv temporare: {1:N2} GiB." -f ($freeBytes / 1GB), ($requiredBytes / 1GB))
    if ($freeBytes -lt $requiredBytes) { throw 'Spatiu insuficient pe discul proiectului.' }
    if ($ProbeOnly) {
        Log 'Verificarea preliminara a reusit; nu a fost instalat nimic.'
        $exitCode = 0
    } else {
        Log 'Vor fi descarcate modele de mai multi GB, instrumente si pachete din GitHub, conda-forge, PyPI si Hugging Face.'
        Log 'Verifica licentele modelelor si faptul ca ai permisiunea de a descarca si utiliza aceste resurse.'
        Write-Host 'Continui instalarea locala? Scrie DA si apasa Enter:'
        $answer = [Console]::ReadLine()
        if ($null -eq $answer -or $answer.Trim() -ne 'DA') { throw 'Instalarea a fost anulata; nu s-au descarcat fisiere.' }
        if ((Test-Path -LiteralPath $runtime) -and -not (Test-Path -LiteralPath (Join-Path $runtime '.instsum-owned'))) {
            throw "Directorul $runtime exista si nu este identificat ca instalare instsum. Nu a fost modificat."
        }
        New-Item -ItemType Directory -Force -Path $runtime, $cache | Out-Null
        $owner = Join-Path $runtime '.instsum-owned'
        if (-not (Test-Path -LiteralPath $owner)) { [IO.File]::WriteAllText($owner, 'instsum setup-v1') }
        $lock = [IO.File]::Open((Join-Path $runtime 'setup.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        $ready = Join-Path $runtime 'ready'
        if (Test-Path -LiteralPath $ready) { Remove-Item -LiteralPath $ready -Force }

        $assets = @(
            @{ name='go.zip'; url='https://go.dev/dl/go1.23.12.windows-amd64.zip'; hash='07c35866cdd864b81bb6f1cfbf25ac7f87ddc3a976ede1bf5112acbb12dfe6dc' },
            @{ name='micromamba.exe'; url='https://github.com/mamba-org/micromamba-releases/releases/download/2.10.0-0/micromamba-win-64.exe'; hash='f8b4284bd4eb1134122eaf93b3ea6c7be567e596de5829e1a6f478e34e41974a' },
            @{ name='llama.zip'; url='https://github.com/ggml-org/llama.cpp/releases/download/b7766/llama-b7766-bin-win-cuda-12.4-x64.zip'; hash='4d6ed33e128ec49a8f2c542d6a12b535dd3e7e477fd3c29db0882b063bc05f6d' },
            @{ name='cudart.zip'; url='https://github.com/ggml-org/llama.cpp/releases/download/b7766/cudart-llama-bin-win-cuda-12.4-x64.zip'; hash='8c79a9b226de4b3cacfd1f83d24f962d0773be79f1e7b75c6af4ded7e32ae1d6' },
            @{ name='whisper.zip'; url='https://github.com/ggml-org/whisper.cpp/releases/download/v1.7.6/whisper-bin-x64.zip'; hash='0d2eca299c248f965bd0341bcb219db4b433c7f0c0ce2200d4df85765e8156a9' },
            @{ name='gallery_dl-1.32.20-py3-none-any.whl'; url='https://files.pythonhosted.org/packages/8f/f7/a80d1aa77cabd8a96aacd1c63c530774e15795fa92e9383757b3161af3d2/gallery_dl-1.32.20-py3-none-any.whl'; hash='e09729c8f24f788ac64a663268603377fbf51d4b97818d49e2150c5d78067709' },
            @{ name='eng.traineddata'; url='https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/87416418657359cb625c412a48b6e1d6d41c29bd/eng.traineddata'; hash='7d4322bd2a7749724879683fc3912cb542f19906c83bcc1a52132556427170b2' },
            @{ name='ron.traineddata'; url='https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/87416418657359cb625c412a48b6e1d6d41c29bd/ron.traineddata'; hash='9adfde6b51ba4b97efd10ea37c3070fd3fc2bad7815e81f5c3c198cd96216cc9' }
        )
        foreach ($asset in $assets) { Download $asset.url (Join-Path $cache $asset.name) $asset.hash }
        $models = Join-Path $runtime 'models'
        New-Item -ItemType Directory -Force -Path $models | Out-Null
        $modelPath = Join-Path $models 'vision.gguf'
        $projectorPath = Join-Path $models 'mmproj.gguf'
        $speechPath = Join-Path $models 'whisper.bin'
        Download (ModelUrl $visionRepo $vision.sha $model.name) $modelPath $model.sha256
        Download (ModelUrl $visionRepo $vision.sha $projector.name) $projectorPath $projector.sha256
        Download (ModelUrl $whisperRepo $whisper.sha $speech.name) $speechPath $speech.sha256

        Expand-VerifiedZip (Join-Path $cache 'go.zip') (Join-Path $runtime 'toolchain') 'go\bin\go.exe'
        Expand-VerifiedZip (Join-Path $cache 'whisper.zip') (Join-Path $runtime 'whisper') 'Release\whisper-cli.exe'
        Expand-VerifiedZip (Join-Path $cache 'llama.zip') (Join-Path $runtime 'llama') 'llama-server.exe'
        Expand-VerifiedZip (Join-Path $cache 'cudart.zip') (Join-Path $runtime 'cudart') 'cudart64_12.dll'

        $mamba = Join-Path $cache 'micromamba.exe'
        $env:MAMBA_ROOT_PREFIX = Join-Path $runtime 'mamba'
        $env:CONDA_PKGS_DIRS = Join-Path $runtime 'mamba\pkgs'
        $prefix = Join-Path $runtime 'env'
        $history = Join-Path $prefix 'conda-meta\history'
        $packages = @('python=3.12', 'pip', 'pyyaml', 'requests', 'ffmpeg', 'tesseract', 'yt-dlp=2026.10.26', 'instaloader=4.14.1')
        if (Test-Path -LiteralPath $history) {
            Log 'Verific si completez mediul local existent.'
            Run $mamba (@('install', '-y', '-p', $prefix, '-c', 'conda-forge', '--strict-channel-priority') + $packages)
        } elseif (Test-Path -LiteralPath $prefix) {
            throw "Mediul local este incomplet: $prefix. Elimina acest director dupa verificare si reporneste setup."
        } else {
            Run $mamba (@('create', '-y', '-p', $prefix, '-c', 'conda-forge', '--strict-channel-priority') + $packages)
        }
        $env:PATH = (Join-Path $prefix 'Library\bin') + ';' + (Join-Path $prefix 'Scripts') + ';' + $prefix + ';' + (Join-Path $runtime 'llama') + ';' + (Join-Path $runtime 'cudart') + ';' + (Join-Path $runtime 'whisper\Release') + ';' + $env:PATH
        $python = Join-Path $prefix 'python.exe'
        Run $python @('-m', 'pip', 'install', '--no-index', '--no-deps', (Join-Path $cache 'gallery_dl-1.32.20-py3-none-any.whl'))
        $tessdata = Join-Path $runtime 'tessdata'
        New-Item -ItemType Directory -Force -Path $tessdata | Out-Null
        foreach ($lang in @('eng', 'ron')) {
            $source = Join-Path $cache "$lang.traineddata"
            $target = Join-Path $tessdata "$lang.traineddata"
            if (-not (Test-Path -LiteralPath $target)) { Copy-Item -LiteralPath $source -Destination $target }
            if ((Get-FileHash $target -Algorithm SHA256).Hash -ne (Get-FileHash $source -Algorithm SHA256).Hash) {
                throw "Date OCR existente diferite: $target. Nu au fost suprascrise."
            }
        }
        $env:TESSDATA_PREFIX = $tessdata
        foreach ($tool in @('ffmpeg.exe', 'ffprobe.exe', 'tesseract.exe', 'yt-dlp.exe', 'gallery-dl.exe', 'instaloader.exe')) {
            $command = Get-Command $tool -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
            if (-not $command -or -not $command.Source.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Instrumentul local $tool lipseste din $prefix."
            }
        }
        Run (Join-Path $prefix 'Library\bin\tesseract.exe') @('--list-langs')
        $go = Join-Path $runtime 'toolchain\go\bin\go.exe'
        $env:GOPATH = Join-Path $runtime 'go-work'
        $env:GOCACHE = Join-Path $runtime 'go-build-cache'
        $env:GOENV = 'off'
        $env:GOWORK = 'off'
        $env:GOTOOLCHAIN = 'local'
        $installedExe = Join-Path $runtime 'instsum.exe'
        $tmpExe = Join-Path $runtime ("instsum-" + [guid]::NewGuid().ToString('N') + '.tmp.exe')
        try {
            Run $go @('build', '-o', $tmpExe, '.\cmd\instsum')
            Move-Item -LiteralPath $tmpExe -Destination $installedExe -Force
        } finally { if (Test-Path -LiteralPath $tmpExe) { Remove-Item -LiteralPath $tmpExe -Force } }
        $config = Join-Path $runtime ("config-check-" + [guid]::NewGuid().ToString('N') + '.yaml')
        try {
            Run $python @((Join-Path $PSScriptRoot 'setup-config.py'), (Join-Path $root 'configs\config.yaml'), $config, $runtime)
        } finally { if (Test-Path -LiteralPath $config) { Remove-Item -LiteralPath $config -Force } }
        $manifest = @{
            vision_revision=$vision.sha; vision=$model; projector=$projector
            whisper_revision=$whisper.sha; speech=$speech
        } | ConvertTo-Json -Depth 5
        $manifestPath = Join-Path $runtime 'model-manifest.json'
        if (-not (Test-Path -LiteralPath $manifestPath)) { [IO.File]::WriteAllText($manifestPath, $manifest, [Text.UTF8Encoding]::new($false)) }
        [IO.File]::WriteAllText($ready, 'setup-v1')
        $createdReady = $true
        Log 'Componentele au fost instalate local. Poti porni start.bat.'
        Log 'Atentie: modelele, CUDA si procesarea unui reel real nu au fost validate automat.'
        $exitCode = 0
    }
} catch {
    $message = "EROARE setup: $($_.Exception.Message)"
    [Console]::Error.WriteLine($message)
    try { if ($writer) { $writer.WriteLine($message); $writer.Flush() } } catch {
        [Console]::Error.WriteLine("Nu se poate scrie in jurnal: $($_.Exception.Message)")
    }
    $exitCode = 1
} finally {
    try { Log ("Incheiere: {0}; cod: {1}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'), $exitCode) } catch {
        [Console]::Error.WriteLine("Nu se poate finaliza jurnalul: $($_.Exception.Message)")
        $exitCode = 1
    }
    if ($createdReady -and $exitCode -ne 0) {
        try { Remove-Item -LiteralPath $ready -Force } catch {
            [Console]::Error.WriteLine("Nu se poate invalida markerul setup: $($_.Exception.Message)")
        }
    }
    if ($lock) { $lock.Dispose() }
    if ($writer) { $writer.Dispose() }
}
exit $exitCode
