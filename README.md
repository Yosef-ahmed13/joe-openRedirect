# 🎯 joe-openRedirect

> **Automated Open Redirect Scanner** — Telegram Bot ↔ GitHub Actions  
> Language: **Go** | Scanner: **Nuclei** | Template: coffinxp/openRedirect

---

## 🏗️ Architecture

```
You (Telegram)
    │
    │  /openRedirect target.com
    │  or .txt file upload
    ▼
[Go Bot — Local PC]
    │  Splits into chunks of 50
    │  Triggers GitHub via API
    ▼
[GitHub Actions — joe-openRedirect.yml]
    │  GAU + Waybackurls → collect URLs
    │  GF filter → redirect params
    │  Nuclei + openRedirect.yaml → scan
    │  Parse + score results
    ▼
[Telegram]
    📄 Report file (per batch)
    🔗 URL • Description • Score/10
```

---

## 🚀 Quick Start

### 1. Clone the repo
```bash
git clone https://github.com/Yosef-ahmed13/joe-openRedirect
cd joe-openRedirect
```

### 2. Add GitHub Secrets
Go to: **Settings → Secrets → Actions** and add:

| Secret | Value |
|--------|-------|
| `TELEGRAM_BOT_TOKEN` | Your bot token |
| `TELEGRAM_CHAT_ID` | Your chat ID |

### 3. Run the Bot (Windows)
```
Double-click: start_bot.cmd
```

---

## 📱 Telegram Commands

| Command | Description |
|---------|-------------|
| `/openRedirect target.com` | Scan single domain |
| `/openRedirect a.com b.com c.com` | Scan multiple domains |
| `/openRedirect` + send `.txt` file | Scan from file |
| Paste domain list as text | Auto-detected & scanned |
| `/status` | Check latest GitHub Actions run |
| `/help` | Show help |

---

## 📦 How Batching Works

1. You send 200 domains → Bot splits into **4 batches × 50 domains**
2. Each batch triggers **1 GitHub Actions run** concurrently
3. After each batch finishes, a **report file** is sent to Telegram
4. Report contains: `URL • Description • Severity Score (1-10)`

---

## 📄 Report Format

```
╔══════════════════════════════════════════════════════╗
║        joe-openRedirect — Scan Report                ║
╚══════════════════════════════════════════════════════╝

📅 Date     : 2025-01-15 14:30:00 UTC
📦 Batch    : 1/3
🎯 Targets  : target.com, evil.org +48 more
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🔗 URL         : https://target.com/redirect?url=https://evil.com
🎭 Template    : open-redirect
📝 Description : Open Redirect vulnerability allows attackers to redirect...
⚠️  Severity    : HIGH
🔢 Score       : 8 / 10
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 TOTAL FINDINGS: 1
```

---

## ⚙️ Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CHUNK_SIZE` | `50` | Domains per GitHub Actions batch |
| `DISPATCH_DELAY` | `2s` | Delay between batch triggers |
| `GO_VERSION` | `1.22` | Go version in Actions |

---

## 🔧 Nuclei Template

Uses: [`coffinxp/nuclei-templates/openRedirect.yaml`](https://github.com/coffinxp/nuclei-templates/blob/main/openRedirect.yaml)

Downloaded automatically during each scan run.

---

## 🛡️ Security Notes

- Bot only responds to your `TELEGRAM_CHAT_ID`
- Credentials stored as GitHub Secrets (never in code)
- `start_bot.cmd` contains credentials for local testing only
