package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

const maxMemory = 10 << 20

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	fmt.Println("File upload starting")

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

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
    	respondWithError(w, http.StatusInternalServerError, "Parse error", err)
    	return
	}

	file, handler, err := r.FormFile("thumbnail")
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "Form File error", err)
	    return
	}

	defer file.Close()

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "DB - GetVideo Error", err)
	    return
	}

	if userID != videoMetaData.UserID {
		respondWithError(w, http.StatusUnauthorized, "User Validation Error", nil)
	    return
	}

	contentTypeHeader := handler.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "Parse media type error", err)
	    return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusInternalServerError, "Incorrect media type error", nil)
	    return	
	}

	key := make([]byte, 32)
	rand.Read(key)
	randFilename := base64.URLEncoding.EncodeToString(key)

	fileExtension := strings.Split(mediaType, "/")[1]
	filenameWithExt := fmt.Sprintf("%s.%s", randFilename, fileExtension)
	filepathString := filepath.Join(cfg.assetsRoot, filenameWithExt)

	thumbnailFile, err := os.Create(filepathString)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "File creation error", err)
	    return
	}

	_, err = io.Copy(thumbnailFile, file)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "File copy error", err)
	    return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, filenameWithExt)

	videoMetaData.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "DB - UpdateVideo Error", err)
	    return
	}

	fmt.Println("Successful thumbnail upload for video", videoID, "by user", userID)
	
	respondWithJSON(w, http.StatusOK, videoMetaData)
}
