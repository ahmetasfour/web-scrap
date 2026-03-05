package handler

import "github.com/gofiber/fiber/v2"

// RegisterRoutes registers all routes on the fiber app.
func RegisterRoutes(app *fiber.App, h *Handler) {
	api := app.Group("/api")
	api.Post("/scrape", h.Scrape)
	api.Post("/scrape/stream", h.ScrapeStream)
	api.Get("/scrape/history", h.History)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
}
