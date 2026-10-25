# Turns the Bluetooth radio off and on again (same as the toggle in Windows
# quick settings). Clears a stuck Windows BLE stack without rebooting the PC.
# Run in Windows PowerShell 5.1 (powershell.exe), no admin rights needed:
#   powershell -ExecutionPolicy Bypass -File .\bt-reset.ps1

Add-Type -AssemblyName System.Runtime.WindowsRuntime
$asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
})[0]

function Await($op, [Type]$type) {
    $task = $asTask.MakeGenericMethod($type).Invoke($null, @($op))
    $task.Wait(-1) | Out-Null
    $task.Result
}

[Windows.Devices.Radios.Radio, Windows.System.Devices, ContentType = WindowsRuntime] | Out-Null
[Windows.Devices.Radios.RadioAccessStatus, Windows.System.Devices, ContentType = WindowsRuntime] | Out-Null

Await ([Windows.Devices.Radios.Radio]::RequestAccessAsync()) ([Windows.Devices.Radios.RadioAccessStatus]) | Out-Null
$radios = Await ([Windows.Devices.Radios.Radio]::GetRadiosAsync()) ([System.Collections.Generic.IReadOnlyList[Windows.Devices.Radios.Radio]])
$bt = $radios | Where-Object { $_.Kind -eq 'Bluetooth' } | Select-Object -First 1
if (-not $bt) { Write-Error "No Bluetooth radio found"; exit 1 }

Write-Host "Bluetooth off..."
Await ($bt.SetStateAsync('Off')) ([Windows.Devices.Radios.RadioAccessStatus]) | Out-Null
Start-Sleep -Seconds 3
Write-Host "Bluetooth on..."
Await ($bt.SetStateAsync('On')) ([Windows.Devices.Radios.RadioAccessStatus]) | Out-Null
Write-Host "Done. TouchyTails will reconnect by itself within a few seconds."
