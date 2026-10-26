package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// testJPEG builds a JPEG and splices a fake EXIF (APP1) segment in after the SOI marker,
// standing in for camera metadata such as GPS.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("Exif\x00\x00"), bytes.Repeat([]byte{0x42}, 64)...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func TestRenderVariantsSizesAndStripsMetadata(t *testing.T) {
	original := testJPEG(t, 3000, 2000)
	if !bytes.Contains(original, []byte("Exif\x00\x00")) {
		t.Fatal("fixture should contain an EXIF segment")
	}

	out, size, err := RenderVariants(original)
	if err != nil {
		t.Fatal(err)
	}
	if size.X != 3000 || size.Y != 2000 {
		t.Fatalf("original size = %v", size)
	}

	want := map[string][2]int{"thumb": {400, 266}, "web": {1600, 1066}} // Fit rounds down
	for _, r := range out {
		if got := [2]int{r.Width, r.Height}; got != want[r.Name] {
			t.Errorf("%s size = %v, want %v", r.Name, got, want[r.Name])
		}
		if bytes.Contains(r.Body, []byte("Exif")) {
			t.Errorf("%s still contains EXIF metadata", r.Name)
		}
	}
}

func TestRenderVariantsNeverUpscales(t *testing.T) {
	out, _, err := RenderVariants(testJPEG(t, 300, 200))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out {
		if r.Width != 300 || r.Height != 200 {
			t.Errorf("%s upscaled to %dx%d", r.Name, r.Width, r.Height)
		}
	}
}

func TestRenderVariantsRejectsNonImage(t *testing.T) {
	if _, _, err := RenderVariants([]byte("not a jpeg")); err == nil {
		t.Fatal("expected an error for non-image input")
	}
}

func TestExtractMetadataToleratesBrokenExif(t *testing.T) {
	m := ExtractMetadata(testJPEG(t, 10, 10))
	if m.HadGPS {
		t.Error("broken EXIF must not report GPS")
	}
}

func TestPhotoIDIsDeterministic(t *testing.T) {
	a, b := PhotoID("bkt", "album/x.jpg"), PhotoID("bkt", "album/x.jpg")
	if a != b || len(a) != 16 {
		t.Fatalf("ids %q %q", a, b)
	}
	if a == PhotoID("bkt", "album/y.jpg") {
		t.Fatal("different keys must give different ids")
	}
}

func TestAlbum(t *testing.T) {
	cases := map[string]string{
		"2026-10-thueringer-wald/IMG_1.jpg": "2026-10-thueringer-wald",
		"birds/kingfisher/IMG_2.jpg":        "birds",
		"IMG_3.jpg":                         "unsorted",
	}
	for key, want := range cases {
		if got := Album(key); got != want {
			t.Errorf("Album(%q) = %q, want %q", key, got, want)
		}
	}
}

// gpsExif builds a minimal but valid EXIF block: Make = "TestCam" plus a GPS IFD with
// latitude/longitude, the same structure a camera or phone writes.
func gpsExif() []byte {
	le := binary.LittleEndian
	tiff := make([]byte, 148)
	copy(tiff, "II*\x00")
	le.PutUint32(tiff[4:], 8) // IFD0 offset

	entry := func(at int, tag, typ uint16, count, value uint32) {
		le.PutUint16(tiff[at:], tag)
		le.PutUint16(tiff[at+2:], typ)
		le.PutUint32(tiff[at+4:], count)
		le.PutUint32(tiff[at+8:], value)
	}
	const ascii, long, rational = 2, 4, 5

	le.PutUint16(tiff[8:], 2)       // IFD0: 2 entries
	entry(10, 0x010F, ascii, 8, 92) // Make -> "TestCam\0" at 92
	entry(22, 0x8825, long, 1, 38)  // GPS IFD pointer -> 38
	le.PutUint32(tiff[34:], 0)      // no next IFD

	le.PutUint16(tiff[38:], 4)     // GPS IFD: 4 entries
	entry(40, 0x0001, ascii, 2, 0) // GPSLatitudeRef "N"
	tiff[48] = 'N'
	entry(52, 0x0002, rational, 3, 100) // GPSLatitude -> 100
	entry(64, 0x0003, ascii, 2, 0)      // GPSLongitudeRef "E"
	tiff[72] = 'E'
	entry(76, 0x0004, rational, 3, 124) // GPSLongitude -> 124
	le.PutUint32(tiff[88:], 0)

	copy(tiff[92:], "TestCam\x00")
	for i, v := range []uint32{50, 1, 41, 1, 0, 1, 10, 1, 56, 1, 0, 1} { // 50°41'N 10°56'E
		le.PutUint32(tiff[100+4*i:], v)
	}
	return append([]byte("Exif\x00\x00"), tiff...)
}

func jpegWithApp1(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 800, 600)), nil); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	return append(append(append(append([]byte{}, b[:2]...), seg...), payload...), b[2:]...)
}

func TestGPSIsDetectedAndNeverPublished(t *testing.T) {
	original := jpegWithApp1(t, gpsExif())

	m := ExtractMetadata(original)
	if !m.HadGPS {
		t.Fatal("expected GPS to be detected in the original")
	}
	if m.Camera != "TestCam" {
		t.Errorf("camera = %q, want TestCam", m.Camera)
	}

	out, _, err := RenderVariants(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out {
		if ExtractMetadata(r.Body).HadGPS {
			t.Errorf("%s still has GPS", r.Name)
		}
		if bytes.Contains(r.Body, []byte("Exif")) || bytes.Contains(r.Body, []byte("TestCam")) {
			t.Errorf("%s still carries EXIF metadata", r.Name)
		}
	}
}
