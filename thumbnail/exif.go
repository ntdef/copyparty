package thumbnail

import (
	"image"
	"io"
	"os"

	"github.com/rwcarlsen/goexif/exif"
)

// Orientation represents EXIF orientation values
type Orientation int

const (
	OrientationUnspecified Orientation = 0
	OrientationNormal      Orientation = 1
	OrientationFlipH       Orientation = 2
	OrientationRotate180   Orientation = 3
	OrientationFlipV       Orientation = 4
	OrientationTranspose   Orientation = 5
	OrientationRotate270   Orientation = 6
	OrientationTransverse  Orientation = 7
	OrientationRotate90    Orientation = 8
)

// getOrientation reads EXIF orientation from an image file
func getOrientation(reader io.Reader) Orientation {
	x, err := exif.Decode(reader)
	if err != nil {
		return OrientationNormal
	}

	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return OrientationNormal
	}

	orient, err := tag.Int(0)
	if err != nil {
		return OrientationNormal
	}

	return Orientation(orient)
}

// getOrientationFromFile reads EXIF orientation from a file path
func getOrientationFromFile(path string) Orientation {
	f, err := os.Open(path)
	if err != nil {
		return OrientationNormal
	}
	defer f.Close()

	return getOrientation(f)
}

// needsRotation returns true if the orientation requires rotation
func (o Orientation) needsRotation() bool {
	return o == OrientationRotate90 || o == OrientationRotate180 || o == OrientationRotate270
}

// rotationAngle returns the rotation angle in degrees for the orientation
func (o Orientation) rotationAngle() int {
	switch o {
	case OrientationRotate90:
		return 90
	case OrientationRotate180:
		return 180
	case OrientationRotate270:
		return 270
	default:
		return 0
	}
}

// shouldTranspose returns true if width/height should be swapped
func (o Orientation) shouldTranspose() bool {
	return o == OrientationRotate90 || o == OrientationRotate270 ||
		o == OrientationTranspose || o == OrientationTransverse
}

// transformBounds adjusts image bounds based on orientation
func (o Orientation) transformBounds(bounds image.Rectangle) image.Rectangle {
	if o.shouldTranspose() {
		// Swap width and height
		return image.Rect(0, 0, bounds.Dy(), bounds.Dx())
	}
	return bounds
}
