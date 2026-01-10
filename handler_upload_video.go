package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find jwt", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate jwt", err)
		return
	}

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}

	if userID != videoMetaData.UserID {
		respondWithError(w, http.StatusUnauthorized, "User not authorized", nil)
		return
	}

	fmt.Println("uploading file for video", videoID, "by user", userID)
	
	file, handler, err := r.FormFile("video")
	if err != nil {
	    respondWithError(w, http.StatusBadRequest, "Form File error", err)
	    return
	}

	defer file.Close()

	contentTypeHeader := handler.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	defer os.Remove(tempFile.Name())

	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset temp file pointer", err)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)
	randFilename := hex.EncodeToString(key)
	fileExtension := strings.Split(mediaType, "/")[1]
	filenameWithExt := fmt.Sprintf("%s.%s", randFilename, fileExtension)

	_, err = cfg.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: 		&cfg.s3Bucket,
		Key:			&filenameWithExt,
		Body:			tempFile,
		ContentType: 	&mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't add object to S3 bucket", err)
		return
	}	

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, filenameWithExt)

	videoMetaData.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "DB - UpdateVideo Error", err)
	    return
	}
	
	respondWithJSON(w, http.StatusOK, videoMetaData)

}
