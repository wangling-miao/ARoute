package media

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type contextKey string

const ctxKeyUserID contextKey = "auth_user_id"

type mediaResponse struct {
	Data interface{} `json:"data"`
	Meta interface{} `json:"meta"`
}

type mediaErrorsEnvelope struct {
	Errors []mediaErrItem `json:"errors"`
}

type mediaErrItem struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

type mediaPageMeta struct {
	TotalCount int64 `json:"total_count"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalPages int   `json:"total_pages"`
}

func writeMediaJSON(w http.ResponseWriter, status int, data interface{}) {
	resp := mediaResponse{Data: data, Meta: map[string]interface{}{}}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeMediaJSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta mediaPageMeta) {
	resp := mediaResponse{Data: data, Meta: meta}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Total-Count", strconv.FormatInt(meta.TotalCount, 10))
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeMediaError(w http.ResponseWriter, status int, code, message string) {
	envelope := mediaErrorsEnvelope{
		Errors: []mediaErrItem{
			{Code: code, Message: message, Details: map[string]interface{}{}},
		},
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(envelope)
}

type uploadResponse struct {
	*interfaces.MediaFile
	URL string `json:"url"`
}

func (p *Plugin) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		writeMediaError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to parse multipart form: "+err.Error())
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeMediaError(w, http.StatusBadRequest, "VALIDATION_ERROR", "no file uploaded; field name must be \"file\"")
		return
	}

	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		writeMediaError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to open uploaded file")
		return
	}
	defer file.Close()

	mf, err := p.service.Upload(r.Context(), file, fileHeader, extractUploaderID(r))
	if err != nil {
		if errors.Is(err, interfaces.ErrValidation) {
			writeMediaError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		p.ctx.Logger().Error("upload failed", "error", err)
		writeMediaError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "file upload failed")
		return
	}

	url, urlErr := p.service.GetURL(r.Context(), mf.ID)
	if urlErr != nil {
		p.ctx.Logger().Warn("failed to generate file URL", "id", mf.ID, "error", urlErr)
		url = ""
	}

	writeMediaJSON(w, http.StatusCreated, uploadResponse{
		MediaFile: mf,
		URL:       url,
	})
}

func (p *Plugin) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	sort := q.Get("sort")
	order := strings.ToLower(q.Get("order"))
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	search := q.Get("search")

	query := &interfaces.ListQuery{
		Page:    page,
		PerPage: perPage,
		Sort:    sort,
		Order:   order,
		Search:  search,
	}

	result, err := p.service.List(r.Context(), query)
	if err != nil {
		p.ctx.Logger().Error("list media failed", "error", err)
		writeMediaError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list media files")
		return
	}

	items, ok := result.Data.([]*interfaces.MediaFile)
	if !ok {
		writeMediaJSON(w, http.StatusOK, result.Data)
		return
	}

	type listItem struct {
		*interfaces.MediaFile
		URL string `json:"url"`
	}

	list := make([]listItem, 0, len(items))
	for _, mf := range items {
		url, urlErr := p.service.GetURL(r.Context(), mf.ID)
		if urlErr != nil {
			p.ctx.Logger().Warn("failed to generate file URL", "id", mf.ID, "error", urlErr)
			url = ""
		}
		list = append(list, listItem{
			MediaFile: mf,
			URL:       url,
		})
	}

	writeMediaJSONWithMeta(w, http.StatusOK, list, mediaPageMeta{
		TotalCount: result.Meta.Total,
		Page:       result.Meta.Page,
		PerPage:    result.Meta.PerPage,
		TotalPages: result.Meta.TotalPages,
	})
}

func (p *Plugin) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeMediaError(w, http.StatusBadRequest, "VALIDATION_ERROR", "missing media ID")
		return
	}

	if err := p.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeMediaError(w, http.StatusNotFound, "NOT_FOUND", "media file not found")
			return
		}
		p.ctx.Logger().Error("delete media failed", "id", id, "error", err)
		writeMediaError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete media file")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func extractUploaderID(r *http.Request) string {
	if uid, ok := r.Context().Value(ctxKeyUserID).(string); ok && uid != "" {
		return uid
	}
	return ""
}
