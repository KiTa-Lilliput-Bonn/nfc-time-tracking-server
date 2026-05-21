' NFC Time Tracking Server — start without a visible console window.
' Deploy next to nfc-time-tracker-server.exe (same folder as config.yaml and data/).
' Autostart: create a shortcut to this .vbs in the user Startup folder, not to the .exe.

Option Explicit

Dim fso, shell, baseDir, exePath
Set fso = CreateObject("Scripting.FileSystemObject")
Set shell = CreateObject("WScript.Shell")

baseDir = fso.GetParentFolderName(WScript.ScriptFullName)
shell.CurrentDirectory = baseDir

exePath = baseDir & "\nfc-time-tracker-server.exe"
If Not fso.FileExists(exePath) Then
  WScript.Quit 1
End If

' Window style 0 = hidden; bWaitOnReturn False = do not block on server process.
shell.Run Chr(34) & exePath & Chr(34), 0, False
