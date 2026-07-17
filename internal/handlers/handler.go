package handlers

import (
	"Saveit./internal/cache"
)

type Handler struct {
	Cache *cache.Cache
}

func New(c *cache.Cache) *Handler {
	return &Handler{Cache: c}
}
