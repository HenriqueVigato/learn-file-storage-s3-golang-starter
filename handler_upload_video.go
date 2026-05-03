package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid id", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldnt' find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldnt' validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "to AWS by user:", userID)

	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	videoMetaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	if videoMetaData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "only the video owner can upload", err)
		return
	}

	multiFile, multiHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get the video", err)
		return
	}
	defer multiFile.Close()

	videoType, _, err := mime.ParseMediaType(multiHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse the type of media", err)
		return
	}
	if videoType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Wrong type of media", nil)
		return
	}

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create a temp file", err)
		return
	}

	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, multiFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save the video", err)
		return
	}

	tmpFile.Seek(0, io.SeekStart)

	tmpFileAspectRatio, err := getVideoAspectRation(tmpFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get the aspect ratio from video %v", err)
		return
	}
	var prefix string
	switch tmpFileAspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "portrait"
	case "other":
		prefix = "other"
	}

	cryptoName := make([]byte, 32)
	rand.Read(cryptoName)
	videoName := fmt.Sprintf("%s/%x.mp4", prefix, cryptoName)

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(videoName),
		Body:        tmpFile,
		ContentType: aws.String("video/mp4"),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't upload to S3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, videoName)
	videoMetaData.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetaData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update the video in database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetaData)
}

type Stream struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type FFProbeOutput struct {
	Streams []Stream `json:"Streams"`
}

func getVideoAspectRation(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var videoInfo bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &videoInfo
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %v, stderr: %s", err, stderr.String())
	}

	var output FFProbeOutput
	if err := json.Unmarshal(videoInfo.Bytes(), &output); err != nil {
		return "", err
	}

	ratio := float64(output.Streams[0].Width) / float64(output.Streams[0].Height)
	switch {
	case ratio >= 1.7 && ratio <= 1.8:
		return "16:9", nil
	case ratio >= 0.5 && ratio <= 0.6:
		return "9:16", nil
	default:
		return "other", nil
	}
}
