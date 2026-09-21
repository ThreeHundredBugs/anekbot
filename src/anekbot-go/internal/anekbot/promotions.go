package anekbot

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"

	"github.com/go-telegram/bot/models"
)

const promotionCallbackData = "promotion"

type Promotion struct {
	Message string  `json:"message"`
	Weight  float64 `json:"weight"`
	Link    string  `json:"link,omitempty"`
}

type promotionsFile struct {
	Promotions struct {
		Frequency float64     `json:"frequency"`
		Items     []Promotion `json:"items"`
	} `json:"promotions"`
}

type Promotions struct {
	frequency   float64
	promotions  []Promotion
	totalWeight float64
	randFloat   func() float64
}

func LoadPromotions(path string) (*Promotions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read promotions file: %w", err)
	}
	return ParsePromotions(data)
}

func ParsePromotions(data []byte) (*Promotions, error) {
	var f promotionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse promotions: %w", err)
	}
	cfg := f.Promotions
	if cfg.Frequency < 0 || cfg.Frequency > 1 {
		return nil, fmt.Errorf("promotions.frequency %v out of range 0-1", cfg.Frequency)
	}

	p := &Promotions{frequency: cfg.Frequency, promotions: cfg.Items, randFloat: rand.Float64}
	for i, promo := range cfg.Items {
		if promo.Message == "" {
			return nil, fmt.Errorf("promotions.items[%d]: message is required", i)
		}
		if promo.Weight <= 0 {
			return nil, fmt.Errorf("promotions.items[%d]: weight must be positive", i)
		}
		if promo.Link != "" {
			u, err := url.Parse(promo.Link)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "tg") {
				return nil, fmt.Errorf("promotions.items[%d]: invalid link %q", i, promo.Link)
			}
		}
		p.totalWeight += promo.Weight
	}
	return p, nil
}

func (p *Promotions) Pick() (Promotion, bool) {
	if p == nil || len(p.promotions) == 0 || p.randFloat() >= p.frequency {
		return Promotion{}, false
	}
	target := p.randFloat() * p.totalWeight
	for _, promo := range p.promotions {
		target -= promo.Weight
		if target < 0 {
			return promo, true
		}
	}
	return p.promotions[len(p.promotions)-1], true
}

func (p *Promotions) Keyboard() *models.InlineKeyboardMarkup {
	promo, ok := p.Pick()
	if !ok {
		return nil
	}
	button := models.InlineKeyboardButton{Text: promo.Message}
	if promo.Link != "" {
		button.URL = promo.Link
	} else {
		button.CallbackData = promotionCallbackData
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{button}}}
}
