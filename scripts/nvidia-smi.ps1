function Enable-NvidiaSmi {
    if (-not [string]::IsNullOrWhiteSpace($env:IGX_NVIDIA_SMI_PATH)) {
        $explicit = $env:IGX_NVIDIA_SMI_PATH
        if (-not [IO.Path]::IsPathRooted($explicit) -or
            [IO.Path]::GetFileName($explicit) -ine 'nvidia-smi.exe' -or
            -not (Test-Path -LiteralPath $explicit -PathType Leaf)) {
            throw "IGX_NVIDIA_SMI_PATH trebuie sa indice calea absoluta catre nvidia-smi.exe: $explicit"
        }
        $env:PATH = (Split-Path -Parent $explicit) + ';' + $env:PATH
        return $explicit
    }
    $existing = Get-Command nvidia-smi.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($existing) { return $existing.Source }
    $candidates = @()
    if ($env:SystemRoot) { $candidates += (Join-Path $env:SystemRoot 'System32\nvidia-smi.exe') }
    if ($env:ProgramFiles) { $candidates += (Join-Path $env:ProgramFiles 'NVIDIA Corporation\NVSMI\nvidia-smi.exe') }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            $env:PATH = (Split-Path -Parent $candidate) + ';' + $env:PATH
            return $candidate
        }
    }
    return $null
}
