param(
  [string]$ExePath = "dist/cervterm-screenshot.exe",
  [string]$OutPath = "docs/assets/cervterm-preview.png",
  [int]$WaitSeconds = 3,
  [string[]]$ExeArgs = @()
 )

$ErrorActionPreference = "Stop"

if (!(Test-Path $ExePath)) {
  throw "Executable not found: $ExePath"
}

$outDir = Split-Path -Parent $OutPath
if ($outDir) {
  New-Item -ItemType Directory -Force -Path $outDir | Out-Null
}

$proc = Start-Process -FilePath (Resolve-Path $ExePath) -WorkingDirectory (Resolve-Path ".") -ArgumentList (@("--log-file", "-") + $ExeArgs) -PassThru
try {
  $deadline = (Get-Date).AddSeconds($WaitSeconds)
  do {
    Start-Sleep -Milliseconds 200
    $proc.Refresh()
  } while ($proc.MainWindowHandle -eq 0 -and (Get-Date) -lt $deadline)

  if ($proc.MainWindowHandle -eq 0) {
    throw "CervTerm window handle was not available for PID $($proc.Id)"
  }

  Add-Type -AssemblyName System.Drawing
  Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class Win32Capture {
  [DllImport("user32.dll")]
  public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
  [DllImport("user32.dll")]
  public static extern bool PrintWindow(IntPtr hWnd, IntPtr hdcBlt, int nFlags);
  public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
}
"@

  $rect = New-Object Win32Capture+RECT
  if (![Win32Capture]::GetWindowRect($proc.MainWindowHandle, [ref]$rect)) {
    throw "GetWindowRect failed"
  }

  $width = [Math]::Max(1, $rect.Right - $rect.Left)
  $height = [Math]::Max(1, $rect.Bottom - $rect.Top)
  $bitmap = New-Object System.Drawing.Bitmap $width, $height
  $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
  try {
    $hdc = $graphics.GetHdc()
    try {
      if (![Win32Capture]::PrintWindow($proc.MainWindowHandle, $hdc, 0)) {
        throw "PrintWindow failed"
      }
    } finally {
      $graphics.ReleaseHdc($hdc)
    }
    $bitmap.Save((Resolve-Path $outDir).Path + [System.IO.Path]::DirectorySeparatorChar + (Split-Path -Leaf $OutPath), [System.Drawing.Imaging.ImageFormat]::Png)
  } finally {
    $graphics.Dispose()
    $bitmap.Dispose()
  }

  Write-Host "Wrote $OutPath from PID $($proc.Id)"
} finally {
  if (!$proc.HasExited) {
    Stop-Process -Id $proc.Id -Force
  }
}
