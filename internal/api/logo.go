package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"kncc/internal/assets"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MaxLogoSizeBytes = 1048576 // 1 MB
	SettingLogoData  = "company_logo_data"
	SettingLogoMime  = "company_logo_mime"
	SettingLogoTime  = "company_logo_updated_at"
)

// GetSystemLogo godoc
// @Summary Get system logo
// @Description Serves active logo (custom or default shipbuilding logo) with HTTP caching
// @Tags system
// @Produce image/svg+xml
// @Produce image/png
// @Produce image/jpeg
// @Produce image/webp
// @Success 200 {string} string "Binary image content"
// @Success 304 {string} string "Not modified"
// @Router /api/system/logo [get]
func (h *Handler) GetSystemLogo(w http.ResponseWriter, r *http.Request) {
	dataBytes, mimeType, etag, _, _ := h.getEffectiveLogo(r.Context())

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

// UploadLogoRequestJSON defines optional JSON payload for uploading logo.
type UploadLogoRequestJSON struct {
	Data     string `json:"data"`      // raw SVG or base64 data URL
	MimeType string `json:"mime_type"` // optional MIME type hint
}

// UploadLogo godoc
// @Summary Upload corporate logo
// @Description Admin endpoint to upload custom corporate SVG/PNG/JPEG logo
// @Tags admin
// @Accept multipart/form-data
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param logo formData file false "Logo image file (SVG, PNG, JPEG, WebP, max 1MB)"
// @Param body body UploadLogoRequestJSON false "JSON payload with raw SVG or base64 image data"
// @Success 200 {object} MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/admin/settings/logo [post]
func (h *Handler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	var rawData []byte
	var mimeType string

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Limit multipart stream reading to MaxLogoSizeBytes + 4KB margin for headers
		if err := r.ParseMultipartForm(MaxLogoSizeBytes + 4096); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid form data or file too large (max 1 MB)")
			return
		}

		file, header, err := r.FormFile("logo")
		if err != nil {
			file, header, err = r.FormFile("file")
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing logo file in multipart form (key 'logo' or 'file')")
			return
		}
		defer file.Close()

		if header.Size > MaxLogoSizeBytes {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("File size %d exceeds maximum limit of 1 MB", header.Size))
			return
		}

		buf := new(bytes.Buffer)
		lr := io.LimitReader(file, MaxLogoSizeBytes+1)
		n, err := io.Copy(buf, lr)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed reading uploaded file")
			return
		}
		if n > MaxLogoSizeBytes {
			writeJSONError(w, http.StatusBadRequest, "File size exceeds maximum limit of 1 MB")
			return
		}

		rawData = buf.Bytes()
		mimeType = header.Header.Get("Content-Type")
	} else {
		// JSON payload
		var req UploadLogoRequestJSON
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		reqData := strings.TrimSpace(req.Data)
		if reqData == "" {
			writeJSONError(w, http.StatusBadRequest, "Logo data cannot be empty")
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
		writeJSONError(w, http.StatusBadRequest, "Logo data is empty")
		return
	}

	if len(rawData) > MaxLogoSizeBytes {
		writeJSONError(w, http.StatusBadRequest, "Logo data exceeds 1 MB limit")
		return
	}

	// Validate & normalize MIME type
	detectedMime := http.DetectContentType(rawData)
	if strings.HasPrefix(strings.TrimSpace(string(rawData)), "<svg") ||
		strings.HasPrefix(strings.TrimSpace(string(rawData)), "<?xml") {
		mimeType = "image/svg+xml"
	} else if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectedMime
	}

	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	validMimes := map[string]bool{
		"image/svg+xml": true,
		"image/png":     true,
		"image/jpeg":    true,
		"image/webp":    true,
	}

	if !validMimes[mimeType] {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Unsupported image format '%s'. Supported formats: SVG, PNG, JPEG, WebP", mimeType))
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
		if err := h.store.SetSetting(r.Context(), SettingLogoData, storedData); err != nil {
			log.Printf("Failed to save logo data to store: %v", err)
		}
		if err := h.store.SetSetting(r.Context(), SettingLogoMime, mimeType); err != nil {
			log.Printf("Failed to save logo mime to store: %v", err)
		}
		if err := h.store.SetSetting(r.Context(), SettingLogoTime, updatedAt); err != nil {
			log.Printf("Failed to save logo timestamp to store: %v", err)
		}
	}

	// 2. Save / update Kubernetes ConfigMap if client is active
	cmUpdated := false
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.LogoConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.LogoConfigMapName

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
						"logo.data":      storedData,
						"logo.mime_type": mimeType,
						"updated_at":     updatedAt,
					},
				}
				if _, createErr := h.k8sClient.CoreV1().ConfigMaps(ns).Create(r.Context(), newCM, metav1.CreateOptions{}); createErr == nil {
					cmUpdated = true
				} else {
					log.Printf("Notice: could not create logo ConfigMap %s/%s: %v", ns, cmName, createErr)
				}
			} else {
				log.Printf("Notice: error reading logo ConfigMap %s/%s: %v", ns, cmName, err)
			}
		} else {
			if cm.Data == nil {
				cm.Data = make(map[string]string)
			}
			cm.Data["logo.data"] = storedData
			cm.Data["logo.mime_type"] = mimeType
			cm.Data["updated_at"] = updatedAt
			if _, updateErr := h.k8sClient.CoreV1().ConfigMaps(ns).Update(r.Context(), cm, metav1.UpdateOptions{}); updateErr == nil {
				cmUpdated = true
			} else {
				log.Printf("Notice: could not update logo ConfigMap %s/%s: %v", ns, cmName, updateErr)
			}
		}
	}

	// Compute new ETag
	hash := md5.Sum(rawData)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"logo_url":          fmt.Sprintf("/api/system/logo?v=%s", hex.EncodeToString(hash[:])),
		"etag":              etag,
		"mime_type":         mimeType,
		"configmap_updated": cmUpdated,
		"message":           "Корпоративный логотип успешно сохранен",
	})
}

// ResetLogo godoc
// @Summary Reset corporate logo to default
// @Description Admin endpoint to reset logo back to default shipbuilding vector
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MessageResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/admin/settings/logo [delete]
func (h *Handler) ResetLogo(w http.ResponseWriter, r *http.Request) {
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	// 1. Clear in SQLite store
	if h.store != nil {
		_ = h.store.SetSetting(r.Context(), SettingLogoData, "")
		_ = h.store.SetSetting(r.Context(), SettingLogoMime, "")
		_ = h.store.SetSetting(r.Context(), SettingLogoTime, updatedAt)
	}

	// 2. Clear in Kubernetes ConfigMap
	cmUpdated := false
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.LogoConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.LogoConfigMapName

		cm, err := h.k8sClient.CoreV1().ConfigMaps(ns).Get(r.Context(), cmName, metav1.GetOptions{})
		if err == nil {
			if cm.Data != nil {
				cm.Data["logo.data"] = ""
				cm.Data["logo.mime_type"] = "image/svg+xml"
				cm.Data["updated_at"] = updatedAt
				if _, updateErr := h.k8sClient.CoreV1().ConfigMaps(ns).Update(r.Context(), cm, metav1.UpdateOptions{}); updateErr == nil {
					cmUpdated = true
				}
			}
		}
	}

	defaultBytes := []byte(assets.DefaultShipbuildingLogoSVG)
	hash := md5.Sum(defaultBytes)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"logo_url":          fmt.Sprintf("/api/system/logo?v=%s", hex.EncodeToString(hash[:])),
		"configmap_updated": cmUpdated,
		"message":           "Логотип сброшен на значение по умолчанию (судостроение)",
	})
}

// getEffectiveLogo resolves the active logo and its metadata.
func (h *Handler) getEffectiveLogo(ctx context.Context) (dataBytes []byte, mimeType string, etag string, isCustom bool, updatedAt string) {
	var storedData string
	var storedMime string

	// 1. Try resolving from Kubernetes ConfigMap
	if !isNilK8s(h.k8sClient) && h.cfg.K8s.LogoConfigMapName != "" {
		ns := h.cfg.K8s.Namespace
		cmName := h.cfg.K8s.LogoConfigMapName
		if cm, err := h.k8sClient.CoreV1().ConfigMaps(ns).Get(ctx, cmName, metav1.GetOptions{}); err == nil && cm.Data != nil {
			if val := cm.Data["logo.data"]; val != "" {
				storedData = val
				storedMime = cm.Data["logo.mime_type"]
				updatedAt = cm.Data["updated_at"]
			}
		}
	}

	// 2. If not found in ConfigMap, try SQLite Store
	if storedData == "" && h.store != nil {
		if val, err := h.store.GetSetting(ctx, SettingLogoData); err == nil && val != "" {
			storedData = val
			storedMime, _ = h.store.GetSetting(ctx, SettingLogoMime)
			updatedAt, _ = h.store.GetSetting(ctx, SettingLogoTime)
		}
	}

	// 3. Process stored logo data if available
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

	// 4. Fallback to default shipbuilding vector logo
	if len(dataBytes) == 0 {
		dataBytes = []byte(assets.DefaultShipbuildingLogoSVG)
		mimeType = "image/svg+xml"
		isCustom = false
	} else {
		mimeType = storedMime
	}

	hash := md5.Sum(dataBytes)
	etag = fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))
	return dataBytes, mimeType, etag, isCustom, updatedAt
}

// validateSVG checks that the SVG is valid XML and contains no scripts or malicious attributes.
func validateSVG(data []byte) error {
	str := strings.ToLower(string(data))

	// Dangerous tags and attributes
	forbidden := []string{
		"<script",
		"</script",
		"javascript:",
		"onload=",
		"onerror=",
		"onclick=",
		"onmouseover=",
		"onfocus=",
		"<iframe",
		"<object",
		"<embed",
		"<foreignobject",
	}

	for _, token := range forbidden {
		if strings.Contains(str, token) {
			return fmt.Errorf("SVG contains forbidden or potentially unsafe token '%s'", token)
		}
	}

	// Ensure well-formed XML
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		t, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("malformed XML: %w", err)
		}
		if se, ok := t.(xml.StartElement); ok {
			for _, attr := range se.Attr {
				val := strings.ToLower(strings.TrimSpace(attr.Value))
				if strings.HasPrefix(val, "javascript:") || strings.HasPrefix(val, "data:text/html") {
					return fmt.Errorf("SVG attribute %s contains unsafe URI", attr.Name.Local)
				}
			}
		}
	}

	return nil
}

func isNilK8s(client any) bool {
	if client == nil {
		return true
	}
	v := reflect.ValueOf(client)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return true
	}
	return false
}
