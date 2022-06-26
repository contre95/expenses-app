package http

import (
	"expenses-app/pkg/app/querying"
	"fmt"
	"net/http"

	fiber "github.com/gofiber/fiber/v2"
)

type Category struct {
	ID   string
	Name string
}

func getCategories(s querying.CategoryGetter) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		resp, err := s.Get()
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(&fiber.Map{
				"success": false,
				"err":     fmt.Sprintf("%v", err),
				"dat":     nil,
			})
		}
		categories := []Category{}
		for id, name := range resp.Categories {
			categories = append(categories, Category{id, name})
		}
		return c.Status(http.StatusAccepted).JSON(&fiber.Map{
			"success": true,
			"err":     nil,
			"data":    categories,
		})
	}
}
