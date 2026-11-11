package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MaxFaviconSizeBytes = 524288 // 512 KB
	SettingFaviconData  = "company_favicon_data"
	SettingFaviconMime  = "company_favicon_mime"
	SettingFaviconTime  = "company_favicon_updated_at"
)

// GetSystemFavicon godoc
// @Summary Get system favicon
// @Description Serves active favicon (custom favicon or fallback to corporate logo thumbnail)
// @Tags system
// @Produce image/x-icon
// @Produce image/svg+xml
// @Produce image/png
// @Success 200 {string} string "Binary favicon content"
// @Success 304 {string} string "Not modified"
// @Router /api/system/favicon [get]
// @Router /favicon.ico [get]
func (h *Handler) GetSystemFavicon(w http.ResponseWriter, r *http.Request) {
	dataBytes, mimeType, etag, _, _ := h.getEffectiveFavicon(r.Context())

	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && ifNoneMatch == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dataBytes)
}

// UploadFaviconRequestJSON defines optional JSON payload for uploading favicon.
type UploadFaviconRequestJSON struct {
	Data     string `json:"data"`      // raw SVG or base64 data URL / raw base64
	MimeType string `json:"mime_type"` // optional MIME type hint
}

// UploadFavicon godoc
// @Summary Upload custom favicon
// @Description Admin endpoint to upload custom favicon (ICO, SVG, PNG)
// @Tags admin
// @Accept multipart/form-data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param favicon formData file false "Favicon image file (ICO, SVG, PNG, max 512KB)"
// @Param body body UploadFaviconRequestJSON false "JSON payload with raw SVG or base64 favicon data"
// @Success 200 {object} MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/admin/settings/favicon [post]
func (h *Handler) UploadFavicon(w http.ResponseWriter, r *http.Request) {
	var rawData []byte
	var mimeType string

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Limit multipart stream reading to MaxFaviconSizeBytes + 4KB margin for headers
		if err := r.ParseMultipartForm(MaxFaviconSizeBytes + 4096); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid form data or file too large (max 512 KB)")
			return
		}

		file, header, err := r.FormFile("favicon")
		if err != nil {
			file, header, err = r.FormFile("file")
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing favicon file in multipart form (key 'favicon' or 'file')")
			return
		}
		defer file.Close()

		if header.Size > MaxFaviconSizeBytes {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("File size %d exceeds maximum limit of 512 KB", header.Size))
			return
		}

		buf := new(bytes.Buffer)
		lr := io.LimitReader(file, MaxFaviconSizeBytes+1)
		n, err := io.Copy(buf, lr)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed reading uploaded file")
			return
		}
		if n > MaxFaviconSizeBytes {
			writeJSONError(w, http.StatusBadRequest, "File size exceeds maximum limit of 512 KB")
			return
		}

		rawData = buf.Bytes()
		mimeType = header.Header.Get("Content-Type")
	} else {
		// JSON payload
		var req UploadFaviconRequestJSON
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		reqData := strings.TrimSpace(req.Data)
		if reqData == "" {
			writeJSONError(w, http.StatusBadRequest, "Favicon data cannot be empty")
			return
		}

		if strings.HasPrefix(reqData, "data:") {
			// Extract data URI: data:<mime>;base64,<encoded>
			parts := strings.SplitN(reqData, ",", 2)
			if len(parts) == 2 {
				headerPart := parts[0]
				if semi := strings.Index(headerPart, ";"); semi > 5 {
					mimeType = headerPart[5:semi]
				}
				decoded, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					writeJSONError(w, http.StatusBadRequest, "Invalid base64 encoding in data URI")
					return
				}
				rawData = decoded
			}
		} else if strings.HasPrefix(reqData, "<svg") || strings.HasPrefix(reqData, "<?xml") {
			rawData = []byte(reqData)
			mimeType = "image/svg+xml"
		} else {
			// Try decoding as raw base64
			decoded, err := base64.StdEncoding.DecodeString(reqData)
			if err == nil && len(decoded) > 0 {
				rawData = decoded
			} else {
				rawData = []byte(reqData)
			}
		}

		if req.MimeType != "" {
			mimeType = req.MimeType
		}
	}

	if len(rawData) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Favicon data is empty")
		return
	}

	if len(rawData) > MaxFaviconSizeBytes {
		writeJSONError(w, http.StatusBadRequest, "Favicon data exceeds 512 KB limit")
		return
	}

	// Validate & normalize MIME type
	trimmed := strings.TrimSpace(string(rawData))
	detectedMime := http.DetectContentType(rawData)

	if strings.HasPrefix(trimmed, "<svg") || strings.HasPrefix(trimmed, "<?xml") {
		mimeType = "image/svg+xml"
	} else if len(rawData) >= 4 && rawData[0] == 0 && rawData[1] == 0 && rawData[2] == 1 && rawData[3] == 0 {
		// Windows Icon format (.ico) magic number: 0x00000100
		mimeType = "image/x-icon"
	} else if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectedMime
	}

	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	validMimes := map[string]bool{
		"image/svg+xml":             true,
		"image/x-icon":              true,
		"image/vnd.microsoft.icon":   true,
		"image/png":                 true,
		"image/jpeg":                true,
		"image/webp":                true,
	}

	if !validMimes[mimeType] {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported image format '%s'. Supported formats: SVG, ICO, PNG, JPEG, WebP", mimeType))
		return
	}

	// SVG security sanitization check
	if mimeType == "image/svg+xml" {
		if err := validateSVG(rawData); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid or unsafe SVG: %v", err))
			return
		}
	}

	// Prepare data string for storage (raw SVG text if SVG, otherwise base64 encoded)
	var storedData string
	if mimeType == "image/svg+xml" {
		storedData = string(rawData)
	} else {
		storedData = base64.StdEncoding.EncodeToString(rawData)
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)

	// 1. Save to SQLite Store
	if h.store != nil {
		if err := h.store.SetSetting(r.Context(), SettingFaviconData, storedData); err != nil {
			log.Printf("Failed to save favicon data to store: %v", err)
		}
		if err := h.store.SetSetting(r.Context(), SettingFaviconMime, mimeType); err != nil {
			log.Printf("Failed to save favicon mime to store: %v", err)
		}
		if err := h.store.SetSetting(r.Context(), SettingFaviconTime, updatedAt); err != nil {
			log.Printf("Failed to save favicon timestamp to store: %v", err)
		}
	}

	// 2. Save / update Kubernetes ConfigMap if client is active
	cmUpdated := false
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.FaviconConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.FaviconConfigMapName

		cm, err := h.k8sClient.CoreV1().ConfigMaps(ns).Get(r.Context(), cmName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				newCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: ns,
						Labels: map[string]string{
							"app.kubernetes.io/name":      "kncc",
							"app.kubernetes.io/component": "branding",
						},
					},
					Data: map[string]string{
						"favicon.data":      storedData,
						"favicon.mime_type": mimeType,
						"updated_at":        updatedAt,
					},
				}
				if _, createErr := h.k8sClient.CoreV1().ConfigMaps(ns).Create(r.Context(), newCM, metav1.CreateOptions{}); createErr == nil {
					cmUpdated = true
				} else {
					log.Printf("Notice: could not create favicon ConfigMap %s/%s: %v", ns, cmName, createErr)
				}
			} else {
				log.Printf("Notice: error reading favicon ConfigMap %s/%s: %v", ns, cmName, err)
			}
		} else {
			if cm.Data == nil {
				cm.Data = make(map[string]string)
			}
			cm.Data["favicon.data"] = storedData
			cm.Data["favicon.mime_type"] = mimeType
			cm.Data["updated_at"] = updatedAt
			if _, updateErr := h.k8sClient.CoreV1().ConfigMaps(ns).Update(r.Context(), cm, metav1.UpdateOptions{}); updateErr == nil {
				cmUpdated = true
			} else {
				log.Printf("Notice: could not update favicon ConfigMap %s/%s: %v", ns, cmName, updateErr)
			}
		}
	}

	// Compute new ETag
	hash := md5.Sum(rawData)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"favicon_url":       fmt.Sprintf("/api/system/favicon?v=%s", hex.EncodeToString(hash[:])),
		"etag":              etag,
		"mime_type":         mimeType,
		"configmap_updated": cmUpdated,
		"message":           "Favicon updated successfully",
	})
}

// ResetFavicon godoc
// @Summary Reset favicon to default
// @Description Admin endpoint to reset favicon back to corporate logo thumbnail default
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MessageResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/admin/settings/favicon [delete]
func (h *Handler) ResetFavicon(w http.ResponseWriter, r *http.Request) {
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	// 1. Clear in SQLite store
	if h.store != nil {
		_ = h.store.SetSetting(r.Context(), SettingFaviconData, "")
		_ = h.store.SetSetting(r.Context(), SettingFaviconMime, "")
		_ = h.store.SetSetting(r.Context(), SettingFaviconTime, updatedAt)
	}

	// 2. Clear in Kubernetes ConfigMap
	cmUpdated := false
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.FaviconConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.FaviconConfigMapName

		cm, err := h.k8sClient.CoreV1().ConfigMaps(ns).Get(r.Context(), cmName, metav1.GetOptions{})
		if err == nil && cm.Data != nil {
			cm.Data["favicon.data"] = ""
			cm.Data["favicon.mime_type"] = "image/svg+xml"
			cm.Data["updated_at"] = updatedAt
			if _, updateErr := h.k8sClient.CoreV1().ConfigMaps(ns).Update(r.Context(), cm, metav1.UpdateOptions{}); updateErr == nil {
				cmUpdated = true
			}
		}
	}

	// Fallback to active logo
	dataBytes, _, etag, _, _ := h.getEffectiveLogo(r.Context())
	hash := md5.Sum(dataBytes)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"favicon_url":       fmt.Sprintf("/api/system/favicon?v=%s", hex.EncodeToString(hash[:])),
		"etag":              etag,
		"configmap_updated": cmUpdated,
		"message":           "Favicon reset to corporate logo thumbnail",
	})
}

// getEffectiveFavicon resolves the active favicon and its metadata.
// When no custom favicon is configured, it defaults to the thumbnail of the corporate logo.
func (h *Handler) getEffectiveFavicon(ctx context.Context) (dataBytes []byte, mimeType string, etag string, isCustom bool, updatedAt string) {
	var storedData string
	var storedMime string

	// 1. Try resolving from Kubernetes ConfigMap
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.FaviconConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.FaviconConfigMapName
		if cm, err := h.k8sClient.CoreV1().ConfigMaps(ns).Get(ctx, cmName, metav1.GetOptions{}); err == nil && cm.Data != nil {
			if val := cm.Data["favicon.data"]; val != "" {
				storedData = val
				storedMime = cm.Data["favicon.mime_type"]
				updatedAt = cm.Data["updated_at"]
			}
		}
	}

	// 2. If not found in ConfigMap, try SQLite Store
	if storedData == "" && h.store != nil {
		if val, err := h.store.GetSetting(ctx, SettingFaviconData); err == nil && val != "" {
			storedData = val
			storedMime, _ = h.store.GetSetting(ctx, SettingFaviconMime)
			updatedAt, _ = h.store.GetSetting(ctx, SettingFaviconTime)
		}
	}

	// 3. Process stored favicon data if available
	if storedData != "" {
		if storedMime == "" {
			storedMime = "image/svg+xml"
		}

		if storedMime == "image/svg+xml" {
			dataBytes = []byte(storedData)
			isCustom = true
		} else {
			// Base64 encoded binary image
			raw := strings.TrimSpace(storedData)
			if strings.HasPrefix(raw, "data:") {
				if idx := strings.Index(raw, ","); idx != -1 {
					raw = raw[idx+1:]
				}
			}
			decoded, err := base64.StdEncoding.DecodeString(raw)
			if err == nil && len(decoded) > 0 {
				dataBytes = decoded
				isCustom = true
			}
		}
	}

	// 4. Default / Fallback: corporate logo thumbnail!
	if len(dataBytes) == 0 {
		logoBytes, logoMime, logoEtag, _, logoUpdatedAt := h.getEffectiveLogo(ctx)
		return logoBytes, logoMime, logoEtag, false, logoUpdatedAt
	}

	hash := md5.Sum(dataBytes)
	etag = fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))
	return dataBytes, storedMime, etag, isCustom, updatedAt
}
