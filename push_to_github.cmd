@echo off
:: ════════════════════════════════════════════════════════
::  joe-openRedirect — GitHub Push Script
::  Pushes all project files to GitHub remote
:: ════════════════════════════════════════════════════════
title Pushing joe-openRedirect to GitHub

echo.
echo  ╔══════════════════════════════════════════════╗
echo  ║  Pushing joe-openRedirect to GitHub...       ║
echo  ╚══════════════════════════════════════════════╝
echo.

cd /d "%~dp0"

:: Check git
where git >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] git not found! Install from https://git-scm.com/
    pause
    exit /b 1
)

:: Init repo if needed
if not exist ".git" (
    echo [*] Initializing git repo...
    git init
    git remote add origin https://github.com/Yosef-ahmed13/joe-openRedirect.git
)

:: Stage all files
echo [*] Staging files...
git add -A

:: Commit
set MSG=feat: joe-openRedirect automation system - Telegram bot + GitHub Actions
git commit -m "%MSG%" 2>nul || echo [*] Nothing new to commit

:: Push
echo [*] Pushing to GitHub...
git branch -M main
git push -u origin main

if %ERRORLEVEL% EQU 0 (
    echo.
    echo [+] SUCCESS! Pushed to GitHub.
    echo [+] Now go to Settings -> Secrets -> Actions and add:
    echo     TELEGRAM_BOT_TOKEN = 8757533394:AAHmg0kdTTDfQ-8fjwF74sghN3pyyJVAWpY
    echo     TELEGRAM_CHAT_ID   = 5966836890
    echo.
) else (
    echo.
    echo [ERROR] Push failed. Check your GitHub token/remote URL.
)

pause
