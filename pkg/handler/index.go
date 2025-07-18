package handler

import (
	"net/http"

	"github.com/gattma/sealed-secrets-web/pkg/config"
	"github.com/gattma/sealed-secrets-web/pkg/seal"
	"github.com/gattma/sealed-secrets-web/pkg/version"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	sealer    seal.Sealer
	indexHTML string
	filter    *config.FieldFilter
	cfg       *config.Config
}

func New(indexHTML string, sealer seal.Sealer, cfg *config.Config) *Handler {
	return &Handler{
		sealer:    sealer,
		indexHTML: indexHTML,
		cfg:       cfg,
		filter:    cfg.FieldFilter,
	}
}

func (h *Handler) Index(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, h.indexHTML)
}

func (h *Handler) RedirectToIndex(context string) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		ctx.Redirect(http.StatusMovedPermanently, context)
		ctx.Abort()
	}
}

func (h *Handler) Version(c *gin.Context) {
	c.JSONP(http.StatusOK, map[string]string{"version": version.Version, "build": version.Build})
}
