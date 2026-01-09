package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

const maxMemory = 10 << 20

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

	mediaType := handler.Header.Get("Content-Type")

	dat, err := io.ReadAll(file)

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "DB - GetVideo Error", err)
	    return
	}

	if userID != videoMetaData.UserID {
		respondWithError(w, http.StatusUnauthorized, "User Validation Error", err)
	    return
	}

	imageString := base64.StdEncoding.EncodeToString(dat)

	thumbnailURL := fmt.Sprintf("data:%s;base64,%s", mediaType, imageString)

	videoMetaData.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "DB - UpdateVideo Error", err)
	    return
	}

	fmt.Println("Successful thumbnail upload for video", videoID, "by user", userID)
	
	respondWithJSON(w, http.StatusOK, videoMetaData)
}
