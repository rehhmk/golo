package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/enzotriches/golo/internal/signals"
)

const termsVersion = "beta-2026-07"

type Store interface {
	ListActiveSubscribers() ([]signals.Subscriber, error)
	ListSignals(limit int) ([]signals.Decision, error)
	RedeemInvitation(code string, subscriber signals.Subscriber) error
	SaveDelivery(signals.Delivery) error
}

type Service struct {
	token         string
	webhookSecret string
	store         Store
	client        *http.Client
	baseURL       string
}

func New(token, webhookSecret string, store Store) *Service {
	return &Service{
		token: token, webhookSecret: webhookSecret, store: store,
		client:  &http.Client{Timeout: 8 * time.Second},
		baseURL: "https://api.telegram.org",
	}
}

func (s *Service) Configured() bool { return s.token != "" && s.store != nil }

func (s *Service) SendTest(ctx context.Context, chatID int64) error {
	_, err := s.send(ctx, chatID,
		"✅ <b>Teste do Golo concluído.</b>\nO canal de alertas está configurado.\n\n<b>18+</b> · Ministério da Fazenda adverte: Aposta não é investimento.",
		"", nil)
	return err
}

func (s *Service) SendSignal(ctx context.Context, decision signals.Decision) error {
	text := signalMessage(decision)
	return s.broadcast(ctx, decision.ID, "signal", text, decision.Quote.DeepLink)
}

func (s *Service) SendResolution(ctx context.Context, decision signals.Decision) error {
	icon, label := "⚪", "ANULADO"
	switch decision.Status {
	case signals.StatusWon:
		icon, label = "✅", "ACERTO"
	case signals.StatusLost:
		icon, label = "❌", "ERRO"
	}
	performanceText := ""
	if decisions, err := s.store.ListSignals(1000); err == nil {
		performance := signals.ComputePerformance(decisions)
		performanceText = fmt.Sprintf("\nHistórico: %d acertos · %d erros · ROI teórico %.1f%% (1 unidade fixa)\n",
			performance.Wins, performance.Losses, performance.ROIPct)
	}
	text := fmt.Sprintf(
		"%s <b>%s — sinal resolvido</b>\n%s x %s\nMercado: Over %.1f · alvo: +%d gol(s)\n\n"+
			"Este resultado permanece no histórico completo do Golo.%s\n"+
			"<b>18+</b> · Ministério da Fazenda adverte: Aposta não é investimento.",
		icon, label, esc(decision.HomeTeam), esc(decision.AwayTeam), decision.MarketLine, decision.AdditionalGoals, performanceText,
	)
	return s.broadcast(ctx, decision.ID, "resolution", text, "")
}

func signalMessage(d signals.Decision) string {
	reasons := make([]string, 0, 3)
	for i, contribution := range d.Prediction.Contributions {
		if i == 3 {
			break
		}
		direction := "aumenta"
		if contribution.Contribution < 0 {
			direction = "reduz"
		}
		reasons = append(reasons, fmt.Sprintf("• %s %s a intensidade (%+.3f)",
			esc(humanFeature(contribution.Name)), direction, contribution.Contribution))
	}
	reasonText := "• Tempo restante e taxa-base do modelo"
	if len(reasons) > 0 {
		reasonText = strings.Join(reasons, "\n")
	}
	return fmt.Sprintf(
		"⚽ <b>GOLO PROVÁVEL · sinal analítico</b>\n"+
			"<b>%s %d–%d %s</b> · %d'\n%s\n\n"+
			"<b>Mercado:</b> Over %.1f · %s\n"+
			"<b>Odd:</b> %.2f · atualizada %s\n"+
			"<b>Golo:</b> %.1f%% · mercado sem margem %.1f%%\n"+
			"<b>Diferença:</b> +%.1f pp\n\n"+
			"<b>Evidência histórica:</b> %.1f%% (IC 95%% %.1f–%.1f%%)\n"+
			"%d partidas · odd conservadora mínima %.2f\n\n"+
			"<b>Por quê:</b>\n%s\n\n"+
			"Probabilidade não é garantia. A cotação pode mudar antes da abertura.\n"+
			"<b>18+</b> · Ministério da Fazenda adverte: Aposta não é investimento.",
		esc(d.HomeTeam), d.State.Score.Home, d.State.Score.Away, esc(d.AwayTeam), d.MatchSecond/60,
		esc(d.Competition), d.MarketLine, esc(d.Quote.Bookmaker), d.Quote.Over,
		d.Quote.UpdatedAt.Format("15:04:05"),
		d.ModelProbability*100, d.MarketProbability*100, d.Edge*100,
		d.Qualification.Holdout.HitRate*100, d.Qualification.Holdout.RateLow*100,
		d.Qualification.Holdout.RateHigh*100, d.Qualification.Holdout.MatchCount,
		d.Qualification.Holdout.OddsHigh, reasonText,
	)
}

func (s *Service) broadcast(ctx context.Context, signalID, kind, text, link string) error {
	if !s.Configured() {
		return fmt.Errorf("telegram is not configured")
	}
	subscribers, err := s.store.ListActiveSubscribers()
	if err != nil {
		return err
	}
	var firstErr error
	for _, subscriber := range subscribers {
		delivery := signals.Delivery{
			ID: randomID(), SignalID: signalID, SubscriberID: subscriber.ID,
			Kind: kind, Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		var deliveryErr error
		for attempt := 1; attempt <= 3; attempt++ {
			delivery.Attempts = attempt
			messageID, sendErr := s.send(ctx, subscriber.TelegramChatID, text, link, nil)
			if sendErr == nil {
				delivery.Status, delivery.MessageID, delivery.Error = "sent", messageID, ""
				deliveryErr = nil
				break
			}
			deliveryErr = sendErr
			delivery.Status, delivery.Error = "failed", sendErr.Error()
		}
		if deliveryErr != nil && firstErr == nil {
			firstErr = deliveryErr
		}
		delivery.UpdatedAt = time.Now()
		_ = s.store.SaveDelivery(delivery)
	}
	return firstErr
}

type Update struct {
	Message *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
	} `json:"message"`
	Callback *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Message struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func (s *Service) HandleWebhook(ctx context.Context, secret string, update Update) error {
	if s.webhookSecret == "" || secret != s.webhookSecret {
		return fmt.Errorf("invalid Telegram webhook secret")
	}
	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/start ") {
		code := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/start "))
		text := "<b>Golo Beta 18+</b>\n\nAlertas probabilísticos não garantem resultados. " +
			"Ao continuar, você confirma ter 18 anos ou mais e aceita que todos os acertos e erros serão registrados."
		button := map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "Confirmo 18+ e aceito", "callback_data": "accept:" + code},
		}}}
		_, err := s.send(ctx, update.Message.Chat.ID, text, "", button)
		return err
	}
	if update.Callback != nil && strings.HasPrefix(update.Callback.Data, "accept:") {
		code := strings.TrimPrefix(update.Callback.Data, "accept:")
		name := strings.TrimSpace(update.Callback.From.FirstName + " @" + update.Callback.From.Username)
		subscriber := signals.Subscriber{
			ID: randomID(), TelegramChatID: update.Callback.Message.Chat.ID,
			DisplayName: name, Active: true, AdultConfirmed: true,
			TermsVersion: termsVersion, CreatedAt: time.Now(),
		}
		if err := s.store.RedeemInvitation(code, subscriber); err != nil {
			_, notifyErr := s.send(ctx, subscriber.TelegramChatID, "Convite inválido, expirado ou já utilizado.", "", nil)
			return notifyErr
		}
		_, err := s.send(ctx, subscriber.TelegramChatID,
			"✅ <b>Acesso ativado.</b>\nVocê receberá sinais qualificados e também todos os resultados — acertos, erros e anulados.", "", nil)
		return err
	}
	return nil
}

func (s *Service) send(ctx context.Context, chatID int64, text, link string, replyMarkup any) (int, error) {
	payload := map[string]any{
		"chat_id": chatID, "text": text, "parse_mode": "HTML",
		"disable_web_page_preview": true,
	}
	if link != "" {
		replyMarkup = map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "Abrir mercado", "url": link},
		}}}
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/bot%s/sendMessage", s.baseURL, s.token), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("telegram status %d: %s", response.StatusCode, raw)
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, err
	}
	if !result.OK {
		return 0, fmt.Errorf("telegram rejected message")
	}
	return result.Result.MessageID, nil
}

func esc(value string) string { return html.EscapeString(value) }

func humanFeature(value string) string {
	labels := map[string]string{
		"abs_score_diff": "diferença no placar", "goals_so_far": "gols já marcados",
		"abs_red_diff": "desequilíbrio de cartões", "shots_10m_total": "chutes nos últimos 10 min",
		"shots_on_target_10m_total":   "chutes no alvo nos últimos 10 min",
		"corners_10m_total":           "escanteios nos últimos 10 min",
		"dangerous_attacks_10m_total": "ataques perigosos nos últimos 10 min",
	}
	if label := labels[value]; label != "" {
		return label
	}
	return value
}

func randomID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// ParseChatID is used by the admin test endpoint.
func ParseChatID(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) }
