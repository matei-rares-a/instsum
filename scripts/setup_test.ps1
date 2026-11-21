$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSHOME 'Modules\Microsoft.PowerShell.Utility\Microsoft.PowerShell.Utility.psd1')
$setup = Join-Path $PSScriptRoot 'setup.ps1'
$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($setup, [ref]$tokens, [ref]$errors)
if ($errors.Count -ne 0) { throw "Setup parser errors: $errors" }
foreach ($name in @('Download', 'Expand-VerifiedZip', 'Select-ModelFile', 'Select-SetupGpu', 'Run')) {
    $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    if (-not $definition) { throw "Missing function $name" }
    Invoke-Expression $definition.Extent.Text
}
function Log([string]$message) { }
function Assert-Fails([scriptblock]$action) {
    $failed = $false
    try { & $action } catch { $failed = $true }
    if (-not $failed) { throw 'Expected an explicit failure.' }
}
$temp = Join-Path ([IO.Path]::GetTempPath()) ("instsum-setup-test-" + [guid]::NewGuid().ToString('N'))
try {
    New-Item -ItemType Directory -Path $temp | Out-Null
    $file = Join-Path $temp 'existing file'
    [IO.File]::WriteAllText($file, 'verified')
    $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash
    Download 'https://invalid.example/file' $file $hash
    Assert-Fails { Download 'https://invalid.example/file' $file ('0' * 64) }
    Assert-Fails { Download 'https://invalid.example/file' $file 'invalid' }
    if ([IO.File]::ReadAllText($file) -ne 'verified') { throw 'Existing file was changed.' }

    $metadata = [pscustomobject]@{ siblings = @([pscustomobject]@{ rfilename='model.gguf'; size=123; lfs=@{sha256=('a' * 64)} }) }
    if ((Select-ModelFile $metadata 'model\.gguf$' 'model').size -ne 123) { throw 'Incorrect model selection.' }
    Assert-Fails { Select-ModelFile $metadata 'missing' 'model' }
    $metadata.siblings += $metadata.siblings[0]
    Assert-Fails { Select-ModelFile $metadata 'model' 'model' }
    $metadata.siblings = @([pscustomobject]@{ rfilename='model.gguf'; size=123; lfs=@{} })
    Assert-Fails { Select-ModelFile $metadata 'model' 'model' }
    $gpus = @('0, GTX 1650, GPU-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa, 4096, 555.1', '1, RTX 3080, GPU-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb, 10240, 555.1')
    if ((Select-SetupGpu $gpus '' 9000).index -ne 1) { throw 'Wrong GPU selected.' }
    if ((Select-SetupGpu $gpus '1' 9000).index -ne 1) { throw 'Explicit selection failed.' }
    Assert-Fails { Select-SetupGpu $gpus '0' 9000 }
    Assert-Fails { Select-SetupGpu $gpus '2' 9000 }
    Assert-Fails { Select-SetupGpu @('not a valid GPU line') '' 9000 }
    Assert-Fails { Select-SetupGpu @($gpus[0]) '' 9000 }
    Assert-Fails { Select-SetupGpu @($gpus[1], $gpus[1]) '' 9000 }
    Run $env:ComSpec @('/d', '/c', 'exit', '0')
    Assert-Fails { Run $env:ComSpec @('/d', '/c', 'exit', '7') }

    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zipPath = Join-Path $temp 'unsafe.zip'
    $zip = [IO.Compression.ZipFile]::Open($zipPath, [IO.Compression.ZipArchiveMode]::Create)
    try { [void]$zip.CreateEntry('../escape.txt') } finally { $zip.Dispose() }
    Assert-Fails { Expand-VerifiedZip $zipPath (Join-Path $temp 'extract') 'escape.txt' }
    if (Test-Path -LiteralPath (Join-Path $temp 'escape.txt')) { throw 'Unsafe ZIP escaped extraction.' }
    Write-Host 'Setup helper tests passed.'
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}
