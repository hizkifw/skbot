package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var camera *Camera
var capturesFolder string = "captures"

func main() {
	var err error
	camera, err = NewCamera("")
	if err != nil {
		panic(fmt.Sprintf("%s: %s", "Failed to connect to camera, make sure it's around!", err))
	}
	log.Printf("Camera connected")

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Signal received, exiting")
		camera.Cleanup()
		os.Exit(0)
	}()

	os.MkdirAll(capturesFolder, 0o755)

	http.HandleFunc("/api/preview", previewHandler)
	http.HandleFunc("/api/capture", captureHandler)
	http.HandleFunc("/api/capture/auto", autoCaptureHandler)
	http.HandleFunc("/api/iso", isoHandler)
	http.Handle("/captures", http.FileServer(http.Dir(capturesFolder)))
	http.Handle("/", http.FileServer(http.Dir("public")))
	http.ListenAndServe(":8080", nil)
	camera.Cleanup()
}

func isoHandler(w http.ResponseWriter, r *http.Request) {
	newVal := r.URL.Query().Get("iso")
	if newVal == "" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(camera.originalIso))
		return
	}
	camera.originalIso = newVal
	w.Write([]byte(camera.originalIso))
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Starting preview")
	ticker := time.NewTicker(time.Millisecond * 30)
	mimeWriter := multipart.NewWriter(w)
	w.Header().Add("Content-Type", "multipart/x-mixed-replace; boundary="+mimeWriter.Boundary())
	defer camera.Exit()

	for {
		partWriter, err := mimeWriter.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"image/jpeg"},
		})

		if err != nil {
			log.Printf("Error creating part: %v\n", err)
			return
		}

		buf := new(bytes.Buffer)
		if err := camera.CapturePreview(buf); err != nil {
			log.Printf("Error capturing image: %v\n", err)
			return
		}

		if err := TransformJpeg(partWriter, buf); err != nil {
			return
		}

		select {
		case <-ticker.C:
		case <-r.Context().Done():
			log.Println("Closing preview")
			return
		}
	}
}

func captureAndSave(wJpg io.Writer) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	fnameRaw := fmt.Sprintf("%s/%s.cr2", capturesFolder, timestamp)
	fnameJpg := fmt.Sprintf("%s/%s.jpg", capturesFolder, timestamp)

	fRaw, err := os.Create(fnameRaw)
	if err != nil {
		return "", fmt.Errorf("error creating output raw file: %w", err)
	}
	fJpg, err := os.Create(fnameJpg)
	if err != nil {
		return "", fmt.Errorf("error creating output jpg file: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := camera.CaptureDownloadMulti(fRaw, buf, true); err != nil {
		return "", fmt.Errorf("error capturing image: %w", err)
	}
	log.Printf("Saved %s\n", fnameRaw)

	bufReader := bytes.NewReader(buf.Bytes())
	io.Copy(wJpg, bufReader)
	bufReader.Seek(0, 0)
	io.Copy(fJpg, bufReader)
	log.Printf("Saved %s\n", fnameJpg)

	return fnameRaw, nil
}

func captureHandler(w http.ResponseWriter, r *http.Request) {
	buf := new(bytes.Buffer)

	if _, err := captureAndSave(buf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := TransformJpeg(w, buf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func autoCaptureHandler(w http.ResponseWriter, r *http.Request) {
	ticker := time.NewTicker(time.Second * 10)
	mimeWriter := multipart.NewWriter(w)
	w.Header().Add("Content-Type", "multipart/x-mixed-replace; boundary="+mimeWriter.Boundary())

	for {
		partWriter, err := mimeWriter.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"image/jpeg"},
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		buf := new(bytes.Buffer)
		if _, err := captureAndSave(buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := TransformJpeg(partWriter, buf); err != nil {
			return
		}

		select {
		case <-ticker.C:
		case <-r.Context().Done():
			return
		}
	}
}
