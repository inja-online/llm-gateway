# Windows PowerShell smoke against a running gateway.
# Usage:
#   $env:KEY = "sk-..."
#   $env:MODEL = "deepseek/deepseek-chat"   # optional
#   .\examples\curl-openai.ps1

$ErrorActionPreference = "Stop"
$Gateway = if ($env:GATEWAY) { $env:GATEWAY } else { "http://localhost:8787" }
$Model = if ($env:MODEL) { $env:MODEL } else { "deepseek/deepseek-chat" }
if (-not $env:KEY) { throw "set `$env:KEY to an api key valid for the target provider" }

Write-Host "== non-streaming =="
Invoke-RestMethod -Method Post -Uri "$Gateway/v1/chat/completions" `
  -Headers @{ Authorization = "Bearer $($env:KEY)" } `
  -ContentType "application/json" `
  -Body (@{
    model = $Model
    messages = @(@{ role = "user"; content = "Say hi in one word." })
  } | ConvertTo-Json -Depth 5)

Write-Host "`n== healthz =="
Invoke-RestMethod -Uri "$Gateway/healthz"
