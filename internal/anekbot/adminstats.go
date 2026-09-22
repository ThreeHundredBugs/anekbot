package anekbot

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ThreeHundredBugs/anekbot/internal/logging"
	"github.com/ThreeHundredBugs/anekbot/internal/stats"
)

const (
	statsCommand  = "/stats"
	statsTopUsers = 10
)

type StatsHandler struct {
	stats  *stats.Stats
	admins map[string]struct{}
}

func NewStatsHandler(s *stats.Stats, adminUsernames []string) *StatsHandler {
	admins := make(map[string]struct{}, len(adminUsernames))
	for _, u := range adminUsernames {
		u = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u), "@"))
		if u != "" {
			admins[u] = struct{}{}
		}
	}
	return &StatsHandler{stats: s, admins: admins}
}

func (h *StatsHandler) Name() string {
	return "stats"
}

func (h *StatsHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || strings.TrimSpace(update.Message.Text) != statsCommand {
		return
	}
	msg := update.Message

	// chat.ID == from.ID confirms this is truly a private chat with the sender, not a
	// spoofed chat type on a forwarded/group message.
	if msg.Chat.Type != models.ChatTypePrivate || msg.From == nil || msg.Chat.ID != msg.From.ID {
		return
	}
	if !h.isAdmin(msg.From.Username) {
		return
	}

	snap := h.stats.Snapshot(statsTopUsers)
	logging.Debugf("stats handler: replying to /stats for admin @%s", msg.From.Username)

	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   formatStats(snap),
	}); err != nil {
		logging.Warnf("stats handler: send message: %v", err)
	}
}

func (h *StatsHandler) isAdmin(username string) bool {
	if username == "" {
		return false
	}
	_, ok := h.admins[strings.ToLower(username)]
	return ok
}

func formatStats(snap stats.Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Всего анеков: %d\nИИ-анеков: %d\nПользователей: %d\n", snap.TotalAneks, snap.TotalAIAneks, snap.TotalUsers)

	fmt.Fprintf(&b, "\nВопросы к ИИ: %d\nРеакции на мат: %d\nПромо показано: %d\n",
		snap.QuestionsAnswered, snap.SwearingReactions, snap.PromotionsShown)

	fmt.Fprintf(&b, "\nЗапросы к ИИ: %d успешно, %d с ошибкой\nFallback-провайдер сработал: %d раз\nСейчас выполняется: %d\nОтказано по лимиту: %d на пользователя, %d по параллелизму\n",
		snap.LLMRequestsOK, snap.LLMRequestsError, snap.LLMFallbacks, snap.LLMConcurrencyInUse,
		snap.RateLimitRejectionsPerUser, snap.RateLimitRejectionsConcurrency)

	if len(snap.TopUsers) == 0 {
		b.WriteString("\nТоп пользователей: пока нет данных.")
		return b.String()
	}

	b.WriteString("\nТоп пользователей:")
	for i, u := range snap.TopUsers {
		name := "id:" + fmt.Sprint(u.UserID)
		if u.Username != "" {
			name = "@" + u.Username
		}
		fmt.Fprintf(&b, "\n%d. %s — %d", i+1, name, u.Count)
	}
	return b.String()
}
