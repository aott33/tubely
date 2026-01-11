package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

type Streams struct {
	Width	int `json:"width"`
	Height	int `json:"height"`
}

type ffprobeOutput struct {
	Streams	[]Streams `json:"streams"`
}

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

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get aspect ratio", err)
		return
	}

	fmt.Println("Aspect Ration", aspectRatio)

	ratioType := "other"

	if aspectRatio == "16:9" {
		ratioType = "landscape"
	} else if aspectRatio == "9:16" {
		ratioType = "portrait"
	}

	key := make([]byte, 32)
	rand.Read(key)
	randFilename := hex.EncodeToString(key)
	fileExtension := strings.Split(mediaType, "/")[1]
	filenameWithExt := fmt.Sprintf("%s/%s.%s", ratioType, randFilename, fileExtension)

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

func getVideoAspectRatio(filePath string) (string, error) {
	var ratioString string
	tolerance := 10

	var b bytes.Buffer
	cmd := exec.Command(
		"ffprobe",
		"-v",
		"error",
		"-print_format",
		"json",
		"-show_streams",
		filePath,
	)

	cmd.Stdout = &b

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	var ffprobeOutput ffprobeOutput

	err = json.Unmarshal(b.Bytes(), &ffprobeOutput)
	if err != nil {
		return "", err
	}

	if len(ffprobeOutput.Streams) == 0 {
		return "", fmt.Errorf("No Streams")
	}

	width := ffprobeOutput.Streams[0].Width
	height := ffprobeOutput.Streams[0].Height
	
	w9 := width * 9
	h16 := height * 16
	
	w16 := width * 16
	h9 := height * 9

	diff16x9 := w9 - h16
	if diff16x9 < 0 {
    	diff16x9 = -diff16x9
	}

	diff9x16 := w16 - h9
	if diff9x16 < 0 {
    	diff9x16 = -diff9x16
	}

	if diff16x9 < tolerance {
    	ratioString = "16:9"
	} else if diff9x16 < tolerance {
    	ratioString = "9:16"
	} else {
    	ratioString = "other"
	}

	return ratioString, nil
}
