package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"math/big"
	"path"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

// Variant is one published size. Published images are re-encoded from pixels only,
// so they carry no EXIF at all: GPS coordinates never leave the private originals bucket.
type Variant struct {
	Name    string
	MaxEdge int
	Quality int
}

var variants = []Variant{
	{Name: "thumb", MaxEdge: 400, Quality: 78},
	{Name: "web", MaxEdge: 1600, Quality: 82},
}

// Rendered is a variant ready to upload.
type Rendered struct {
	Name   string
	Body   []byte
	Width  int
	Height int
}

// Metadata is what we keep about a photo. HadGPS records only that a location existed,
// never the coordinates.
type Metadata struct {
	Camera       string  `dynamodbav:"camera,omitempty"`
	Lens         string  `dynamodbav:"lens,omitempty"`
	FocalLength  float64 `dynamodbav:"focalLengthMm,omitempty"`
	Aperture     float64 `dynamodbav:"aperture,omitempty"`
	ExposureTime string  `dynamodbav:"exposureTime,omitempty"`
	ISO          int     `dynamodbav:"iso,omitempty"`
	TakenAt      string  `dynamodbav:"takenAt,omitempty"`
	HadGPS       bool    `dynamodbav:"hadGps"`
}

// PhotoID is deterministic, so a duplicate S3 event overwrites the same objects and item
// instead of creating copies (S3 event delivery is at-least-once).
func PhotoID(bucket, key string) string {
	sum := sha256.Sum256([]byte(bucket + "/" + key))
	return hex.EncodeToString(sum[:8])
}

// Album is the first folder of the key: "2026-10-thueringer-wald/IMG_1234.jpg" -> "2026-10-thueringer-wald".
func Album(key string) string {
	dir := path.Dir(key)
	if dir == "." || dir == "/" || dir == "" {
		return "unsorted"
	}
	return strings.SplitN(dir, "/", 2)[0]
}

// ExtractMetadata is best effort: a photo with missing or broken EXIF is still published.
func ExtractMetadata(original []byte) Metadata {
	var m Metadata
	x, err := exif.Decode(bytes.NewReader(original))
	if err != nil {
		return m
	}

	str := func(f exif.FieldName) string {
		t, err := x.Get(f)
		if err != nil {
			return ""
		}
		s, err := t.StringVal()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.Trim(s, "\x00"))
	}
	rat := func(f exif.FieldName) *big.Rat {
		t, err := x.Get(f)
		if err != nil || t.Format() != tiff.RatVal {
			return nil
		}
		r, err := t.Rat(0)
		if err != nil {
			return nil
		}
		return r
	}

	// Some cameras repeat the maker in the model ("Canon" + "Canon EOS R7"), some don't
	// ("NIKON CORPORATION" + "NIKON Z 6_2"), and some write only one of the two.
	maker, model := str(exif.Make), str(exif.Model)
	switch {
	case model == "":
		m.Camera = maker
	case maker == "" || strings.HasPrefix(model, strings.Fields(maker)[0]):
		m.Camera = model
	default:
		m.Camera = maker + " " + model
	}
	m.Lens = str(exif.LensModel)

	if r := rat(exif.FocalLength); r != nil {
		f, _ := r.Float64()
		m.FocalLength = f
	}
	if r := rat(exif.FNumber); r != nil {
		f, _ := r.Float64()
		m.Aperture = f
	}
	if r := rat(exif.ExposureTime); r != nil {
		if r.Cmp(big.NewRat(1, 1)) < 0 && r.Num().Int64() > 0 {
			m.ExposureTime = fmt.Sprintf("1/%d", r.Denom().Int64()/r.Num().Int64())
		} else {
			f, _ := r.Float64()
			m.ExposureTime = fmt.Sprintf("%gs", f)
		}
	}
	if t, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if v, err := t.Int(0); err == nil {
			m.ISO = v
		}
	}
	if dt, err := x.DateTime(); err == nil {
		m.TakenAt = dt.Format(time.RFC3339)
	}
	if _, _, err := x.LatLong(); err == nil {
		m.HadGPS = true
	}
	return m
}

// RenderVariants decodes the original (applying EXIF orientation) and produces each
// published size. It never upscales. It returns the original's pixel size too.
func RenderVariants(original []byte) ([]Rendered, image.Point, error) {
	img, err := imaging.Decode(bytes.NewReader(original), imaging.AutoOrientation(true))
	if err != nil {
		return nil, image.Point{}, fmt.Errorf("decode image: %w", err)
	}
	size := img.Bounds().Size()

	out := make([]Rendered, 0, len(variants))
	for _, v := range variants {
		resized := imaging.Fit(img, v.MaxEdge, v.MaxEdge, imaging.Lanczos)
		var buf bytes.Buffer
		if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(v.Quality)); err != nil {
			return nil, size, fmt.Errorf("encode %s: %w", v.Name, err)
		}
		b := resized.Bounds().Size()
		out = append(out, Rendered{Name: v.Name, Body: buf.Bytes(), Width: b.X, Height: b.Y})
	}
	return out, size, nil
}
