@echo off
:: ════════════════════════════════════════════════════════════
::  joe-openRedirect — Go Installer & Bot Builder
::  Run this ONCE to install Go and build the bot
:: ════════════════════════════════════════════════════════════
title Installing Go + Building Bot

echo.
echo  ╔══════════════════════════════════════════════════════╗
echo  ║  joe-openRedirect — Setup Script                     ║
echo  ║  This will install Go and build the bot              ║
echo  ╚══════════════════════════════════════════════════════╝
echo.

cd /d "%~dp0"

:: ── Check if Go already installed ─────────────────────────
set "GOEXE="
if exist "C:\Program Files\Go\bin\go.exe" set "GOEXE=C:\Program Files\Go\bin\go.exe"
if exist "C:\Go\bin\go.exe" set "GOEXE=C:\Go\bin\go.exe"

where go >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    echo [+] Go is already installed!
    go version
    goto BUILD
)

if not "%GOEXE%"=="" (
    echo [+] Go found at %GOEXE%
    set PATH=%PATH%;C:\Program Files\Go\bin;C:\Go\bin
    goto BUILD
)

:: ── Download Go installer ──────────────────────────────────
echo [*] Go not found. Downloading Go 1.22.5...
powershell -Command "Invoke-WebRequest -Uri 'https://go.dev/dl/go1.22.5.windows-amd64.msi' -OutFile '%TEMP%\go-installer.msi' -UseBasicParsing"

if not exist "%TEMP%\go-installer.msi" (
    echo [ERROR] Download failed! Get Go manually from https://go.dev/dl/
    pause
    exit /b 1
)

echo [*] Installing Go silently...
msiexec /i "%TEMP%\go-installer.msi" /quiet /norestart

:: Refresh PATH
set PATH=%PATH%;C:\Program Files\Go\bin
echo [+] Go installed!

:BUILD
:: ── Verify Go ─────────────────────────────────────────────
"C:\Program Files\Go\bin\go.exe" version 2>nul || go version 2>nul || (
    echo [ERROR] Go still not found after install.
    echo Please restart this script after restarting your PC.
    pause
    exit /b 1
)

:: ── Set credentials ───────────────────────────────────────
set TELEGRAM_BOT_TOKEN=YOUR_BOT_TOKEN_HERE
set TELEGRAM_CHAT_ID=YOUR_CHAT_ID_HERE
set GH_TOKEN=YOUR_GH_TOKEN_HERE
set GITHUB_REPO=Yosef-ahmed13/joe-openRedirect

:: ── Download dependencies ─────────────────────────────────
echo [*] Downloading Go modules...
"C:\Program Files\Go\bin\go.exe" mod tidy 2>nul || go mod tidy

:: ── Build ─────────────────────────────────────────────────
echo [*] Building bot binary...
"C:\Program Files\Go\bin\go.exe" build -ldflags="-s -w" -o joe-openredirect-bot.exe . 2>nul || go build -o joe-openredirect-bot.exe .

if exist "joe-openredirect-bot.exe" (
    echo.
    echo  ╔══════════════════════════════════════════════╗
    echo  ║  BUILD SUCCESSFUL!                           ║
    echo  ║  Starting bot...                             ║
    echo  ╚══════════════════════════════════════════════╝
    echo.
    joe-openredirect-bot.exe
) else (
    echo [ERROR] Build failed! Check main.go for errors.
    pause
    exit /b 1
)

pause
