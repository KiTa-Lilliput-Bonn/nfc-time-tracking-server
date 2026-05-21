@echo off
REM NFC Time Tracking Server — console mode for support / debugging (visible logs on stderr).
REM Do NOT add this batch file to Windows Startup. For autostart without a console window,
REM use start-nfc-time-tracking.vbs instead.

cd /d "%~dp0"
nfc-time-tracker-server.exe
