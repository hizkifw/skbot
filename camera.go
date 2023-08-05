package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/jonmol/gphoto2"
	"github.com/nf/cr2"
)

type Camera struct {
	lock *sync.Mutex
	cam  *gphoto2.Camera

	originalIso string
}

func NewCamera(name string) (*Camera, error) {
	cam, err := gphoto2.NewCamera(name)
	if err != nil {
		return nil, err
	}

	cam.LoadWidgets()
	isoStr, err := cam.Settings.Find("imgsettings").Find("iso").Get()
	if err != nil {
		return nil, fmt.Errorf("error reading ISO value: %w", err)
	}
	log.Printf("Current ISO is %s\n", isoStr)

	c := &Camera{
		lock:        &sync.Mutex{},
		cam:         cam,
		originalIso: isoStr.(string),
	}
	c.setAuto(true)

	return c, nil
}

func (c *Camera) setAuto(auto bool) {
	newIso := "Auto"
	if !auto {
		newIso = c.originalIso
	}

	log.Printf("Set ISO to %s", newIso)
	c.cam.Settings.Find("imgsettings").Find("iso").Set(newIso)
}

func (c *Camera) Exit() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cam.Exit()
}

func (c *Camera) Cleanup() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.setAuto(false)
	c.cam.Exit()
	c.cam.Free()
}

func (c *Camera) CapturePreview(buffer io.Writer) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.cam.CapturePreview(buffer)
}

func (c *Camera) CaptureDownloadMulti(bufRaw io.Writer, bufJpeg io.Writer, leaveOnCamera bool) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.setAuto(false)
	defer c.setAuto(true)

	c.cam.Exit()

	cim, err := c.cam.CaptureImage()
	if err != nil {
		return fmt.Errorf("error capturing image: %w", err)
	}

	if strings.HasSuffix(cim.Name, ".jpg") {
		return cim.DownloadImage(bufJpeg, leaveOnCamera)
	}

	buf := new(bytes.Buffer)
	if err := cim.DownloadImage(buf, leaveOnCamera); err != nil {
		return fmt.Errorf("error downloading image from camera: %w", err)
	}

	rawReader := bytes.NewReader(buf.Bytes())
	if _, err := io.Copy(bufRaw, rawReader); err != nil {
		return fmt.Errorf("error writing raw file: %w", err)
	}

	rawReader.Seek(0, 0)
	im, err := cr2.Decode(rawReader)
	if err != nil {
		return fmt.Errorf("error decoding CR2 image: %w", err)
	}

	if err := jpeg.Encode(bufJpeg, im, nil); err != nil {
		return fmt.Errorf("error encoding JPEG image: %w", err)
	}

	return nil
}
