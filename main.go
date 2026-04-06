package main

// ═══════════════════════════════════════════════════════════════════
//   joe-openRedirect Bot — Telegram ↔ GitHub Actions Bridge
//   Language : Go  |  High-Performance | Multi-threaded dispatch
//   Command  : /openRedirect <domain>  |  file upload  |  paste list
// ═══════════════════════════════════════════════════════════════════

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ─── CONFIG ────────────────────────────────────────────────────────
const (
	CHUNK_SIZE     = 50
	DISPATCH_DELAY = 2 * time.Second // delay between batch triggers
	GH_API_BASE    = "https://api.github.com"
	WORKFLOW_EVENT = "open-redirect"
)

var (
	TELEGRAM_TOKEN = mustEnv("TELEGRAM_BOT_TOKEN")
	CHAT_ID_STR    = mustEnv("TELEGRAM_CHAT_ID")
	GITHUB_TOKEN   = mustEnv("GH_TOKEN")
	GITHUB_REPO    = getEnv("GITHUB_REPO", "Yosef-ahmed13/joe-openRedirect")

	CHAT_ID int64
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("❌ Missing required environment variable: %s", key)
	}
	return v
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── GITHUB DISPATCH ───────────────────────────────────────────────
type DispatchPayload struct {
	EventType     string         `json:"event_type"`
	ClientPayload map[string]any `json:"client_payload"`
}

func triggerWorkflow(domains []string, batchID, totalBatches int) (int, error) {
	domainsStr := strings.Join(domains, ",")
	payload := DispatchPayload{
		EventType: WORKFLOW_EVENT,
		ClientPayload: map[string]any{
			"domains":       domainsStr,
			"batch_id":      fmt.Sprintf("%d", batchID),
			"total_batches": fmt.Sprintf("%d", totalBatches),
			"triggered_at":  time.Now().UTC().Format(time.RFC3339),
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/repos/%s/dispatches", GH_API_BASE, GITHUB_REPO),
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+GITHUB_TOKEN)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// ─── BOT HELPERS ───────────────────────────────────────────────────
func sendHTML(bot *tgbotapi.BotAPI, text string) {
	msg := tgbotapi.NewMessage(CHAT_ID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	if _, err := bot.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func isAuthorized(update tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}
	return update.Message.Chat.ID == CHAT_ID
}

// ─── DOMAIN HELPERS ────────────────────────────────────────────────
var domainRe = regexp.MustCompile(`(?i)^(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func parseDomains(raw string) []string {
	var domains []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(
		strings.ReplaceAll(raw, ",", "\n"),
	))
	for scanner.Scan() {
		d := strings.TrimSpace(scanner.Text())
		d = strings.ToLower(d)
		// strip protocol prefix if user pastes URLs
		d = strings.TrimPrefix(d, "https://")
		d = strings.TrimPrefix(d, "http://")
		d = strings.TrimPrefix(d, "www.")
		// strip trailing path
		if idx := strings.Index(d, "/"); idx != -1 {
			d = d[:idx]
		}
		if d == "" || strings.HasPrefix(d, "#") {
			continue
		}
		if domainRe.MatchString(d) && !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	return domains
}

func chunkDomains(domains []string, size int) [][]string {
	total := len(domains)
	numChunks := int(math.Ceil(float64(total) / float64(size)))
	chunks := make([][]string, 0, numChunks)
	for i := 0; i < total; i += size {
		end := i + size
		if end > total {
			end = total
		}
		chunks = append(chunks, domains[i:end])
	}
	return chunks
}

// ─── DISPATCH PIPELINE ─────────────────────────────────────────────
func dispatchDomains(bot *tgbotapi.BotAPI, domains []string) {
	total := len(domains)
	chunks := chunkDomains(domains, CHUNK_SIZE)
	totalBatches := len(chunks)

	// Preview
	preview := domains[0]
	if total > 1 {
		preview = fmt.Sprintf("%s +%d more", domains[0], total-1)
	}

	sendHTML(bot, fmt.Sprintf(
		"🎯 <b>Open Redirect Scan Started!</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━━━\n"+
			"🌐 Target(s): <code>%s</code>\n"+
			"📋 Total domains: <b>%d</b>\n"+
			"📦 Batches (50/each): <b>%d</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━━━\n"+
			"⏳ Triggering GitHub Actions...\n"+
			"📲 Results will be sent here automatically!",
		preview, total, totalBatches,
	))

	var (
		successCount int64
		failCount    int64
		wg           sync.WaitGroup
	)

	// Sequential dispatch with delay to avoid GH rate-limit
	for i, chunk := range chunks {
		wg.Add(1)
		batchNum := i + 1
		currentChunk := chunk

		go func() {
			defer wg.Done()
			// stagger launches
			time.Sleep(time.Duration(i) * DISPATCH_DELAY)

			status, err := triggerWorkflow(currentChunk, batchNum, totalBatches)
			if err != nil || status != 204 {
				atomic.AddInt64(&failCount, 1)
				sendHTML(bot, fmt.Sprintf(
					"❌ <b>Batch %d/%d failed!</b>\n"+
						"HTTP %d — Check GH Token / Secrets.",
					batchNum, totalBatches, status,
				))
				return
			}

			atomic.AddInt64(&successCount, 1)
			batchPreview := currentChunk[0]
			if len(currentChunk) > 3 {
				batchPreview = fmt.Sprintf("%s, %s, %s +%d",
					currentChunk[0], currentChunk[1], currentChunk[2],
					len(currentChunk)-3)
			} else if len(currentChunk) > 1 {
				batchPreview = strings.Join(currentChunk, ", ")
			}
			sendHTML(bot, fmt.Sprintf(
				"✅ <b>Batch %d/%d</b> triggered!\n"+
					"   <code>%s</code>",
				batchNum, totalBatches, batchPreview,
			))
		}()
	}

	wg.Wait()

	sendHTML(bot, fmt.Sprintf(
		"🏁 <b>All batches dispatched!</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━━━\n"+
			"✅ Success: %d/%d\n"+
			"❌ Failed:  %d/%d\n"+
			"━━━━━━━━━━━━━━━━━━━━━━\n"+
			"👀 <a href='https://github.com/%s/actions'>Watch on GitHub Actions</a>\n"+
			"📲 Results file will arrive here per batch!",
		successCount, totalBatches,
		failCount, totalBatches,
		GITHUB_REPO,
	))
}

// ─── COMMAND: /openRedirect ────────────────────────────────────────
func handleOpenRedirect(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	args := strings.TrimSpace(msg.CommandArguments())

	if args == "" {
		sendHTML(bot,
			"⚠️ <b>Usage:</b>\n\n"+
				"  <code>/openRedirect target.com</code>\n"+
				"  <code>/openRedirect a.com b.com c.com</code>\n\n"+
				"Or send a <b>.txt file</b> with one domain per line.\n"+
				"Or <b>paste</b> a list of domains as plain text.",
		)
		return
	}

	domains := parseDomains(args)
	if len(domains) == 0 {
		sendHTML(bot, "❌ No valid domains found in your input.")
		return
	}

	go dispatchDomains(bot, domains)
}

// ─── FILE HANDLER ──────────────────────────────────────────────────
func handleFile(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	doc := msg.Document
	if doc == nil {
		return
	}
	if !strings.HasSuffix(strings.ToLower(doc.FileName), ".txt") {
		sendHTML(bot, "⚠️ Please send a <b>.txt</b> file with one domain per line.")
		return
	}

	sendHTML(bot, "📥 File received! Processing...")

	fileURL, err := bot.GetFileDirectURL(doc.FileID)
	if err != nil {
		sendHTML(bot, "❌ Failed to get file URL: "+err.Error())
		return
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		sendHTML(bot, "❌ Failed to download file: "+err.Error())
		return
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		sendHTML(bot, "❌ Failed to read file: "+err.Error())
		return
	}

	domains := parseDomains(string(content))
	if len(domains) == 0 {
		sendHTML(bot, "❌ File is empty or contains no valid domains.")
		return
	}

	go dispatchDomains(bot, domains)
}

// ─── TEXT HANDLER (paste list) ─────────────────────────────────────
func handleText(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	domains := parseDomains(text)
	if len(domains) > 0 {
		go dispatchDomains(bot, domains)
	} else {
		sendHTML(bot,
			"💬 Unknown input. Try:\n"+
				"  <code>/openRedirect example.com</code>\n"+
				"  Or send a <b>.txt file</b>",
		)
	}
}

// ─── COMMAND: /status ──────────────────────────────────────────────
func handleStatus(bot *tgbotapi.BotAPI) {
	url := fmt.Sprintf("%s/repos/%s/actions/runs?per_page=1&event=repository_dispatch", GH_API_BASE, GITHUB_REPO)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+GITHUB_TOKEN)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		sendHTML(bot, "❌ GitHub API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var result struct {
		WorkflowRuns []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			CreatedAt  string `json:"created_at"`
			HTMLURL    string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.WorkflowRuns) == 0 {
		sendHTML(bot, "❌ No workflow runs found.")
		return
	}

	run := result.WorkflowRuns[0]
	emoji := map[string]string{
		"completed":   "✅",
		"in_progress": "🔄",
		"queued":      "⏳",
		"waiting":     "⏳",
	}
	statusEmoji := emoji[run.Status]
	if statusEmoji == "" {
		statusEmoji = "❓"
	}
	if run.Status == "completed" && run.Conclusion != "success" {
		statusEmoji = "❌"
	}

	created := strings.Replace(run.CreatedAt[:16], "T", " ", 1)
	sendHTML(bot, fmt.Sprintf(
		"%s <b>Latest Run:</b> #%d\n"+
			"📋 Status: <code>%s</code>\n"+
			"🏁 Result: <code>%s</code>\n"+
			"⏰ Started: %s UTC\n"+
			"🔗 <a href='%s'>View on GitHub</a>",
		statusEmoji, run.ID, run.Status, run.Conclusion, created, run.HTMLURL,
	))
}

// ─── COMMAND: /help ────────────────────────────────────────────────
func handleHelp(bot *tgbotapi.BotAPI) {
	sendHTML(bot,
		"🛡️ <b>joe-openRedirect Bot — Help</b>\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━\n\n"+
			"<b>1. Single domain:</b>\n"+
			"   <code>/openRedirect google.com</code>\n\n"+
			"<b>2. Multiple domains:</b>\n"+
			"   <code>/openRedirect a.com b.com c.com</code>\n\n"+
			"<b>3. Paste a list:</b>\n"+
			"   Just paste domains (one per line) as text\n\n"+
			"<b>4. Upload a file:</b>\n"+
			"   Send a <code>.txt</code> file (one domain per line)\n\n"+
			"<b>5. Check status:</b>\n"+
			"   <code>/status</code>\n\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"🔧 <b>Scanner:</b> Nuclei + openRedirect template\n"+
			"📦 <b>Batch size:</b> 50 domains per GitHub Action\n"+
			"⚡ <b>Mode:</b> High-performance concurrent dispatch",
	)
}

// ─── MAIN ──────────────────────────────────────────────────────────
func main() {
	// Parse CHAT_ID
	fmt.Sscanf(CHAT_ID_STR, "%d", &CHAT_ID)

	fmt.Println(`
╔══════════════════════════════════════════════╗
║  joe-openRedirect — Telegram Bot             ║
║  Telegram ↔ GitHub Actions (Go Edition)     ║
╚══════════════════════════════════════════════╝`)
	fmt.Printf("  Bot Token : %s...\n", TELEGRAM_TOKEN[:20])
	fmt.Printf("  Chat ID   : %d\n", CHAT_ID)
	fmt.Printf("  GitHub    : %s\n", GITHUB_REPO)
	fmt.Printf("  Chunk Size: %d domains\n\n", CHUNK_SIZE)
	fmt.Println("🟢 Bot running — Send /openRedirect to Telegram!\n")

	bot, err := tgbotapi.NewBotAPI(TELEGRAM_TOKEN)
	if err != nil {
		log.Fatalf("Bot init error: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if !isAuthorized(update) {
			continue
		}
		msg := update.Message
		if msg == nil {
			continue
		}

		switch {
		case msg.Command() == "openRedirect" || msg.Command() == "openredirect":
			go handleOpenRedirect(bot, msg)
		case msg.Command() == "status":
			go handleStatus(bot)
		case msg.Command() == "help" || msg.Command() == "start":
			go handleHelp(bot)
		case msg.Document != nil:
			go handleFile(bot, msg)
		case msg.Text != "" && !strings.HasPrefix(msg.Text, "/"):
			go handleText(bot, msg)
		}
	}
}
