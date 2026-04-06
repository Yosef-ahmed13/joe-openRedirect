package api

import (
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
	GH_API_BASE    = "https://api.github.com"
	WORKFLOW_EVENT = "open-redirect"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getChatID() int64 {
	var id int64
	fmt.Sscanf(os.Getenv("TELEGRAM_CHAT_ID"), "%d", &id)
	return id
}

// ─── GITHUB DISPATCH ───────────────────────────────────────────────
type DispatchPayload struct {
	EventType     string         `json:"event_type"`
	ClientPayload map[string]any `json:"client_payload"`
}

func triggerWorkflow(domains []string, batchID, totalBatches int) (int, error) {
	token := os.Getenv("GH_TOKEN")
	repo := getEnv("GITHUB_REPO", "Yosef-ahmed13/joe-openRedirect")

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
		fmt.Sprintf("%s/repos/%s/dispatches", GH_API_BASE, repo),
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+token)
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
	msg := tgbotapi.NewMessage(getChatID(), text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	bot.Send(msg)
}

func isAuthorized(update *tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}
	return update.Message.Chat.ID == getChatID()
}

// ─── DOMAIN HELPERS ────────────────────────────────────────────────
var domainRe = regexp.MustCompile(`(?i)^(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func parseDomains(raw string) []string {
	var domains []string
	seen := map[string]bool{}
	for _, rawLine := range strings.Split(strings.ReplaceAll(raw, ",", "\n"), "\n") {
		d := strings.TrimSpace(rawLine)
		d = strings.ToLower(d)
		d = strings.TrimPrefix(d, "https://")
		d = strings.TrimPrefix(d, "http://")
		d = strings.TrimPrefix(d, "www.")
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

func dispatchDomains(bot *tgbotapi.BotAPI, domains []string) {
	total := len(domains)
	chunks := chunkDomains(domains, CHUNK_SIZE)
	totalBatches := len(chunks)

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
			"⏳ Triggering GitHub Actions...",
		preview, total, totalBatches,
	))

	var successCount, failCount int64
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		batchNum := i + 1
		currentChunk := chunk

		go func(idx int, bNum int, cChunk []string) {
			defer wg.Done()
			time.Sleep(time.Duration(idx) * 2 * time.Second)

			status, err := triggerWorkflow(cChunk, bNum, totalBatches)
			if err != nil || status != 204 {
				atomic.AddInt64(&failCount, 1)
				sendHTML(bot, fmt.Sprintf("❌ <b>Batch %d/%d failed!</b> HTTP %d", bNum, totalBatches, status))
				return
			}
			atomic.AddInt64(&successCount, 1)

			bp := cChunk[0]
			if len(cChunk) > 3 {
				bp = fmt.Sprintf("%s, %s, %s +%d", cChunk[0], cChunk[1], cChunk[2], len(cChunk)-3)
			} else if len(cChunk) > 1 {
				bp = strings.Join(cChunk, ", ")
			}
			sendHTML(bot, fmt.Sprintf("✅ <b>Batch %d/%d</b> triggered!\n<code>%s</code>", bNum, totalBatches, bp))
		}(i, batchNum, currentChunk)
	}

	wg.Wait()
	sendHTML(bot, fmt.Sprintf(
		"🏁 <b>All batches dispatched!</b>\n"+
			"✅ Success: %d/%d\n"+
			"❌ Failed:  %d/%d\n"+
			"👀 <a href='https://github.com/%s/actions'>Watch on GitHub Actions</a>",
		successCount, totalBatches, failCount, totalBatches, getEnv("GITHUB_REPO", "Yosef-ahmed13/joe-openRedirect"),
	))
}

// ─── COMMAND HANDLERS ──────────────────────────────────────────────
func handleTextOrCommand(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	cmd := strings.ToLower(msg.Command())
	text := strings.TrimSpace(msg.Text)
	args := strings.TrimSpace(msg.CommandArguments())

	if cmd == "openredirect" {
		domains := parseDomains(args)
		if len(domains) > 0 {
			go dispatchDomains(bot, domains)
		} else {
			sendHTML(bot, "⚠️ <b>Usage:</b>\n<code>/openRedirect target.com</code>")
		}
		return
	}

	if cmd == "status" {
		go func() {
			url := fmt.Sprintf("%s/repos/%s/actions/runs?per_page=1&event=repository_dispatch", GH_API_BASE, getEnv("GITHUB_REPO", ""))
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", "token "+os.Getenv("GH_TOKEN"))
			
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode != 200 {
				sendHTML(bot, "❌ GitHub API error")
				return
			}
			defer resp.Body.Close()

			var result struct {
				WorkflowRuns []struct {
					ID         int64  `json:"id"`
					Status     string `json:"status"`
					Conclusion string `json:"conclusion"`
					CreatedAt  string `json:"created_at"`
					HTMLURL    string `json:"html_url"`
				} `json:"workflow_runs"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			if len(result.WorkflowRuns) == 0 {
				sendHTML(bot, "❌ No workflow runs found.")
				return
			}

			run := result.WorkflowRuns[0]
			sendHTML(bot, fmt.Sprintf("<b>Latest Run:</b> #%d\n📋 Status: %s\n🏁 Result: %s\n🔗 <a href='%s'>View on GitHub</a>", run.ID, run.Status, run.Conclusion, run.HTMLURL))
		}()
		return
	}

	if cmd == "help" || cmd == "start" {
		sendHTML(bot, "🛡️ <b>joe-openRedirect Bot</b>\n\n- <code>/openRedirect target.com</code>\n- Send a `.txt` file\n- Paste domains")
		return
	}

	// Just raw domains
	domains := parseDomains(text)
	if len(domains) > 0 {
		go dispatchDomains(bot, domains)
	} else {
		sendHTML(bot, "💬 Send /openRedirect <domain> or a .txt file.")
	}
}

func handleFile(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	if !strings.HasSuffix(strings.ToLower(msg.Document.FileName), ".txt") {
		sendHTML(bot, "⚠️ Please send a <b>.txt</b> file.")
		return
	}

	sendHTML(bot, "📥 File received! Processing...")
	go func() {
		fileURL, err := bot.GetFileDirectURL(msg.Document.FileID)
		if err != nil {
			sendHTML(bot, "❌ Failed to get file URL")
			return
		}
		resp, err := http.Get(fileURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		content, _ := io.ReadAll(resp.Body)
		domains := parseDomains(string(content))
		if len(domains) > 0 {
			dispatchDomains(bot, domains)
		} else {
			sendHTML(bot, "❌ File contains no valid domains.")
		}
	}()
}

// ─── VERCEL WEBHOOK HANDLER ────────────────────────────────────────
func Handler(w http.ResponseWriter, r *http.Request) {
	// 1. Initialize Bot
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		http.Error(w, "TELEGRAM_BOT_TOKEN not set", http.StatusInternalServerError)
		return
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Printf("Bot init error: %v", err)
		http.Error(w, "Bot init error", http.StatusInternalServerError)
		return
	}

	// 2. Parse Incoming Webhook Update
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read body", http.StatusBadRequest)
		return
	}

	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		log.Printf("Error unmarshaling json: %v", err)
		http.Error(w, "Invalid json", http.StatusBadRequest)
		return
	}

	// 3. Process the Update
	if !isAuthorized(&update) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Unauthorized or not a message"))
		return
	}

	msg := update.Message
	if msg.Document != nil {
		handleFile(bot, msg)
	} else if msg.Text != "" {
		handleTextOrCommand(bot, msg)
	}

	// 4. Return 200 OK immediately so Telegram knows we received it
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
