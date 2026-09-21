package anekbot

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/text/encoding/charmap"
)

const (
	anekTrigger    = "анек!"
	defaultBaseURL = "http://rzhunemogu.ru"

	// http://rzhunemogu.ru/FAQ.aspx
	anekTypeNormal = 1
	anekType18Plus = 11

	inlineSuggestionCount = 3
	inlineTitleMaxRunes   = 60

	inlineAITitlePrefix  = "Сгенерировать анек с помощью ИИ на тему "
	inlineAITextMaxRunes = 100
	inlineAICacheSeconds = 120
	aiJokeResultID       = "ai-joke"

	// aiJokePendingCallbackData is attached to a placeholder button on the AI-joke
	// result. Telegram only assigns an inline_message_id (needed to edit the message
	// once the joke is ready) to messages sent with an inline keyboard attached, so
	// the button exists purely for that; HandleCallback just acks taps on it.
	aiJokePendingButtonText   = "⏳"
	aiJokePendingCallbackData = "ai-joke-pending"
)

const aiJokePromptTemplate = "Придумай короткий анекдот на русском языке на тему: %s. " +
	"Ответь только текстом анекдота, без вступлений, пояснений и кавычек."

const aiJokeGeneratingMessage = "Генерирую анек с помощью ИИ, подождите немного…"

type AnekHandler struct {
	client    *http.Client
	baseURL   string
	randFloat func() float64
	promos    *Promotions
}

func NewAnekHandler() *AnekHandler {
	return &AnekHandler{
		client:    &http.Client{Timeout: 10 * time.Second},
		baseURL:   defaultBaseURL,
		randFloat: rand.Float64,
	}
}

func (h *AnekHandler) SetPromotions(p *Promotions) {
	h.promos = p
}

func (h *AnekHandler) Name() string {
	return "anek"
}

func (h *AnekHandler) Handle(ctx context.Context, sender Sender, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	msg := update.Message

	if !strings.Contains(strings.ToLower(msg.Text), anekTrigger) {
		return
	}
	logDebugf("anek handler: matched trigger in chat_id=%d", msg.Chat.ID)

	joke, err := h.fetchJoke(ctx)
	if err != nil {
		logWarnf("anek handler: fetch joke: %v", err)
		return
	}

	if _, err := sender.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            joke,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	}); err != nil {
		logWarnf("anek handler: send message: %v", err)
	}
}

func (h *AnekHandler) HandleInline(ctx context.Context, sender Sender, update *models.Update) {
	if update.InlineQuery == nil {
		return
	}
	query := update.InlineQuery

	topic := strings.Join(strings.Fields(query.Query), " ")
	if topic != "" {
		h.answerAIJokePlaceholderInline(ctx, sender, query, topic)
		return
	}

	jokes := make([]string, inlineSuggestionCount)
	var wg sync.WaitGroup
	wg.Add(inlineSuggestionCount)
	for i := range jokes {
		go func(i int) {
			defer wg.Done()
			joke, err := h.fetchJoke(ctx)
			if err != nil {
				logWarnf("anek handler: fetch joke for inline query: %v", err)
				return
			}
			jokes[i] = joke
		}(i)
	}
	wg.Wait()

	results := make([]models.InlineQueryResult, 0, inlineSuggestionCount)
	for i, joke := range jokes {
		if joke == "" {
			continue
		}
		results = append(results, &models.InlineQueryResultArticle{
			ID:                  strconv.Itoa(i),
			Title:               inlineTitle(joke),
			InputMessageContent: models.InputTextMessageContent{MessageText: joke},
			ReplyMarkup:         h.promos.Keyboard(),
		})
	}

	logDebugf("anek handler: answering inline query with %d results", len(results))
	if _, err := sender.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: query.ID,
		Results:       results,
		CacheTime:     1, // 0 is indistinguishable from unset and gets dropped
	}); err != nil {
		logWarnf("anek handler: answer inline query: %v", err)
	}
}

// answerAIJokePlaceholderInline answers a non-empty inline query with a single result
// carrying placeholder text. The actual joke is generated once Telegram reports (via
// a chosen_inline_result update) that the user picked this result and it was sent,
// at which point HandleChosenInlineResult edits the message in place with the joke.
func (h *AnekHandler) answerAIJokePlaceholderInline(ctx context.Context, sender Sender, query *models.InlineQuery, topic string) {
	title := truncateToRunes(inlineAITitlePrefix+topic, inlineAITextMaxRunes)

	result := &models.InlineQueryResultArticle{
		ID:                  aiJokeResultID,
		Title:               title,
		InputMessageContent: models.InputTextMessageContent{MessageText: aiJokeGeneratingMessage},
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: aiJokePendingButtonText, CallbackData: aiJokePendingCallbackData}},
			},
		},
	}

	logDebugf("anek handler: answering inline query with AI-generate placeholder for topic %q", topic)
	if _, err := sender.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: query.ID,
		Results:       []models.InlineQueryResult{result},
		CacheTime:     inlineAICacheSeconds,
	}); err != nil {
		logWarnf("anek handler: answer inline query: %v", err)
	}
}

// HandleChosenInlineResult generates the AI joke once the user has actually sent the
// placeholder result from answerAIJokePlaceholderInline, then edits it in place.
// This relies on Telegram delivering chosen_inline_result updates, which requires
// inline feedback to be enabled for the bot via BotFather's /setinlinefeedback.
func (h *AnekHandler) HandleChosenInlineResult(ctx context.Context, sender Sender, update *models.Update, llm *LLMHandler) {
	if update.ChosenInlineResult == nil {
		return
	}
	chosen := update.ChosenInlineResult

	if chosen.ResultID != aiJokeResultID || chosen.InlineMessageID == "" {
		return
	}
	topic := strings.Join(strings.Fields(chosen.Query), " ")
	if topic == "" {
		return
	}

	if llm == nil {
		h.editInlineMessage(ctx, sender, chosen.InlineMessageID, llmUnavailableMessage, false)
		return
	}

	logDebugf("anek handler: generating AI joke for topic %q", topic)
	joke, _, err := llm.Ask(ctx, fmt.Sprintf(aiJokePromptTemplate, topic))
	if err != nil {
		logWarnf("anek handler: generate AI joke: %v", err)
		h.editInlineMessage(ctx, sender, chosen.InlineMessageID, llmUnavailableMessage, false)
		return
	}

	h.editInlineMessage(ctx, sender, chosen.InlineMessageID, truncateToRunes(joke, telegramMessageMaxRunes), true)
}

func (h *AnekHandler) editInlineMessage(ctx context.Context, sender Sender, inlineMessageID, text string, tryHTML bool) {
	markup := h.promos.Keyboard()
	if markup == nil {
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
	}
	params := &bot.EditMessageTextParams{
		InlineMessageID: inlineMessageID,
		Text:            text,
		ReplyMarkup:     markup,
	}
	if tryHTML {
		params.ParseMode = models.ParseModeHTML
	}

	if _, err := sender.EditMessageText(ctx, params); err != nil {
		logWarnf("anek handler: edit message text: %v", err)
		if tryHTML {
			plain := *params
			plain.ParseMode = ""
			if _, err := sender.EditMessageText(ctx, &plain); err != nil {
				logWarnf("anek handler: edit message text plain-text fallback: %v", err)
			}
		}
	}
}

// HandleCallback acks taps on the transient pending-generation button and on link-less promotion buttons so the
// Telegram client doesn't leave the user staring at a stuck loading spinner;
// the actual joke arrives via HandleChosenInlineResult regardless of any tap.
func (h *AnekHandler) HandleCallback(ctx context.Context, sender Sender, update *models.Update) {
	if update.CallbackQuery == nil ||
		(update.CallbackQuery.Data != aiJokePendingCallbackData && update.CallbackQuery.Data != promotionCallbackData) {
		return
	}
	if _, err := sender.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	}); err != nil {
		logWarnf("anek handler: answer callback query: %v", err)
	}
}

func inlineTitle(joke string) string {
	preview := strings.Join(strings.Fields(joke), " ")
	runes := []rune(preview)
	if len(runes) <= inlineTitleMaxRunes {
		return preview
	}
	return string(runes[:inlineTitleMaxRunes]) + "…"
}

func (h *AnekHandler) fetchJoke(ctx context.Context) (string, error) {
	anekType := anekTypeNormal
	if h.randFloat() > 0.85 {
		anekType = anekType18Plus
	}

	url := fmt.Sprintf("%s/RandJSON.aspx?CType=%d", h.baseURL, anekType)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// rzhunemogu.ru serves windows-1251-encoded bytes,
	// but Telegram rejects non-UTF-8 text
	utf8Body, err := charmap.Windows1251.NewDecoder().Bytes(body)
	if err != nil {
		return "", fmt.Errorf("decode windows-1251 response: %w", err)
	}

	// The response isn't valid JSON
	joke := strings.TrimPrefix(string(utf8Body), `{"content":"`)
	joke = strings.TrimSuffix(joke, `"}`)
	return joke, nil
}
