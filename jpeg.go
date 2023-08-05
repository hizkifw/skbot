package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
)

func TransformImage(img image.Image) *image.NRGBA {
	return imaging.FlipH(imaging.Rotate90(img))
}

func TransformJpeg(w io.Writer, r io.Reader) error {
	im, err := jpeg.Decode(r)
	if err != nil {
		return fmt.Errorf("error decoding jpeg: %w", err)
	}

	tim := TransformImage(im)

	err = jpeg.Encode(w, tim, nil)
	if err != nil {
		return fmt.Errorf("error encoding jpeg: %w", err)
	}

	return nil
}
