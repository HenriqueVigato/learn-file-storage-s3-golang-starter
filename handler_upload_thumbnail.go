package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse the thumbnail", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get the thumbnail", err)
		return
	}
	defer file.Close()

	mediatype, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "some error", err)
		return
	}

	allowedTypes := []string{"image/jpeg", "image/png"}
	if !slices.Contains(allowedTypes, mediatype) {
		respondWithError(w, http.StatusBadRequest, "Wrong type of media", nil)
		return
	}

	thumbnailVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video id", err)
		return
	}
	if thumbnailVideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User is not authorized", err)
		return
	}
	fileExtention := strings.Split(mediatype, "/")[1]

	fileName := fmt.Sprintf("%s.%s", videoID.String(), fileExtention)
	thumbnailPath := filepath.Join(cfg.assetsRoot, fileName)

	osFile, err := os.Create(thumbnailPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't create file", err)
		return
	}

	defer osFile.Close()

	_, err = io.Copy(osFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't save file", err)
		return
	}

	dataURL := fmt.Sprintf("http://localhost:8091/assets/%s", fileName)
	thumbnailVideo.ThumbnailURL = &dataURL

	err = cfg.db.UpdateVideo(thumbnailVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Erro a atualizar o video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, thumbnailVideo)
}
