package handler

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"webscraper/internal/features/model"
	"webscraper/internal/features/scraper"
)

// Handler holds all HTTP handlers and their shared state.
type Handler struct {
	engine    *scraper.Engine
	historyMu sync.RWMutex
	history   []model.ScrapeResult
	sessionMu sync.Mutex
	sessions  map[string]context.CancelFunc
}

// New returns a Handler wired to the given scraper engine.
func New(engine *scraper.Engine) *Handler {
	return &Handler{
		engine:   engine,
		sessions: make(map[string]context.CancelFunc),
	}
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// Scrape handles POST /api/scrape
func (h *Handler) Scrape(c *fiber.Ctx) error {
	var req model.ScrapeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON: " + err.Error()})
	}
	if len(req.Companies) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no companies provided"})
	}

	logging.Logger.Info("scrape request received",
		zap.Int("count", len(req.Companies)),
		zap.String("time", time.Now().Format(time.RFC3339)),
	)

	results := h.engine.Run(req.Companies)

	h.historyMu.Lock()
	h.history = append(h.history, results...)
	if len(h.history) > 500 {
		h.history = h.history[len(h.history)-500:]
	}
	h.historyMu.Unlock()

	return c.JSON(model.ScrapeResponse{Results: results})
}

// ScrapeStream handles POST /api/scrape/stream
// It streams results as Server-Sent Events (SSE) so the client receives each
// company result the moment it is scraped rather than waiting for all.
func (h *Handler) ScrapeStream(c *fiber.Ctx) error {
	var req model.ScrapeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON: " + err.Error()})
	}
	if len(req.Companies) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no companies provided"})
	}

	logging.Logger.Info("stream scrape request received",
		zap.Int("count", len(req.Companies)),
		zap.String("time", time.Now().Format(time.RFC3339)),
	)

	sessionID := newSessionID()
	ctx, cancel := context.WithCancel(context.Background())

	h.sessionMu.Lock()
	h.sessions[sessionID] = cancel
	h.sessionMu.Unlock()

	resultCh := make(chan model.ScrapeResult, 50)
	go h.engine.RunStream(ctx, req.Companies, resultCh)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			cancel()
			h.sessionMu.Lock()
			delete(h.sessions, sessionID)
			h.sessionMu.Unlock()
		}()

		// Send session ID so the client can call /stop/:sessionId.
		fmt.Fprintf(w, "data: {\"type\":\"session\",\"sessionId\":\"%s\"}\n\n", sessionID)
		w.Flush()

		// Send total count so the client can initialise the progress bar.
		fmt.Fprintf(w, "data: {\"type\":\"total\",\"total\":%d}\n\n", len(req.Companies))
		w.Flush()

		var collected []model.ScrapeResult
		for result := range resultCh {
			collected = append(collected, result)
			data, _ := json.Marshal(result)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.Flush()
		}

		// Persist to history.
		h.historyMu.Lock()
		h.history = append(h.history, collected...)
		if len(h.history) > 500 {
			h.history = h.history[len(h.history)-500:]
		}
		h.historyMu.Unlock()

		fmt.Fprintf(w, "event: done\ndata: {}\n\n")
		w.Flush()
	})

	return nil
}

// StopScrape handles POST /api/scrape/stop/:sessionId
// It cancels the context for the given session, stopping new work from being dispatched.
func (h *Handler) StopScrape(c *fiber.Ctx) error {
	sessionID := c.Params("sessionId")

	h.sessionMu.Lock()
	cancel, ok := h.sessions[sessionID]
	h.sessionMu.Unlock()

	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
	}

	cancel()
	logging.Logger.Info("scrape session stopped", zap.String("sessionId", sessionID))
	return c.JSON(fiber.Map{"ok": true})
}

// History handles GET /api/scrape/history
func (h *Handler) History(c *fiber.Ctx) error {
	h.historyMu.RLock()
	snap := make([]model.ScrapeResult, len(h.history))
	copy(snap, h.history)
	h.historyMu.RUnlock()

	return c.JSON(snap)
}
