package api

import (
	"net/http"
	"strings"

	"github.com/rs/cors"
)

func (a *API) corsMiddleware() func(http.Handler) http.Handler {
	var allowedOrigins []string
	if a.env == LOCAL {
		allowedOrigins = []string{"http://localhost:*"}
	} else {
		allowedOrigins = []string{"https://icaa.world", "https://*.icaa.world"}
	}

	return cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
		AllowOriginFunc: func(origin string) bool {
			if a.env == LOCAL && strings.HasPrefix(origin, "http://localhost") {
				return true
			}
			return false
		},
	}).Handler
}
