$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$acceptanceArguments = @($args)

$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$outputDirectory = Join-Path $repositoryRoot 'artifacts\.acceptance-runner'
$runnerPath = Join-Path $outputDirectory 'acceptance.exe'
$image = 'new-api-pilot-go-test:latest'

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'docker is required to build the acceptance harness'
}

docker image inspect $image *> $null
if ($LASTEXITCODE -ne 0) {
    throw "required acceptance build image is unavailable: $image"
}

[System.IO.Directory]::CreateDirectory($outputDirectory) | Out-Null
$buildArguments = @(
    'run', '--rm',
    '--mount', "type=bind,source=$repositoryRoot,target=/workspace,readonly",
    '--mount', "type=bind,source=$outputDirectory,target=/out",
    '-w', '/workspace',
    '-e', 'CGO_ENABLED=0',
    '-e', 'GOOS=windows',
    '-e', 'GOARCH=amd64',
    $image,
    'go', 'build', '-trimpath', '-o', '/out/acceptance.exe', './scripts/acceptance'
)
& docker @buildArguments
if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $runnerPath -PathType Leaf)) {
    throw 'build acceptance harness in Docker failed'
}

if ($acceptanceArguments.Count -eq 0) {
    throw 'acceptance arguments are required'
}
& $runnerPath @acceptanceArguments
exit $LASTEXITCODE
