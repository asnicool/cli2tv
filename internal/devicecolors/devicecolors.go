package devicecolors

import (
	"image/color"

	"cli2tv/devices"
)

type Palette struct {
	Fill   color.NRGBA
	Stroke color.NRGBA
	Text   color.NRGBA
}

func LightPalette(deviceType string) Palette {
	switch deviceType {
	case devices.DeviceTypeChromecast:
		return Palette{
			Fill:   color.NRGBA{R: 0xda, G: 0xee, B: 0xe7, A: 0xff},
			Stroke: color.NRGBA{R: 0xda, G: 0xee, B: 0xe7, A: 0xff},
			Text:   color.NRGBA{R: 0x34, G: 0x6d, B: 0x5f, A: 0xff},
		}
	case devices.DeviceTypeDLNA:
		return Palette{
			Fill:   color.NRGBA{R: 0xe2, G: 0xe8, B: 0xfa, A: 0xff},
			Stroke: color.NRGBA{R: 0xe2, G: 0xe8, B: 0xfa, A: 0xff},
			Text:   color.NRGBA{R: 0x46, G: 0x5d, B: 0x96, A: 0xff},
		}
	default:
		return Palette{
			Fill:   color.NRGBA{R: 0xea, G: 0xea, B: 0xea, A: 0xff},
			Stroke: color.NRGBA{R: 0xea, G: 0xea, B: 0xea, A: 0xff},
			Text:   color.NRGBA{R: 0x5f, G: 0x5f, B: 0x5f, A: 0xff},
		}
	}
}

func DarkPalette(deviceType string) Palette {
	switch deviceType {
	case devices.DeviceTypeChromecast:
		return Palette{
			Fill:   color.NRGBA{R: 0x36, G: 0x47, B: 0x42, A: 0xff},
			Stroke: color.NRGBA{R: 0x36, G: 0x47, B: 0x42, A: 0xff},
			Text:   color.NRGBA{R: 0xbd, G: 0xd4, B: 0xcb, A: 0xff},
		}
	case devices.DeviceTypeDLNA:
		return Palette{
			Fill:   color.NRGBA{R: 0x3a, G: 0x40, B: 0x52, A: 0xff},
			Stroke: color.NRGBA{R: 0x3a, G: 0x40, B: 0x52, A: 0xff},
			Text:   color.NRGBA{R: 0xc3, G: 0xcd, B: 0xe8, A: 0xff},
		}
	default:
		return Palette{
			Fill:   color.NRGBA{R: 0x3a, G: 0x3a, B: 0x3a, A: 0xff},
			Stroke: color.NRGBA{R: 0x3a, G: 0x3a, B: 0x3a, A: 0xff},
			Text:   color.NRGBA{R: 0xbe, G: 0xbe, B: 0xbe, A: 0xff},
		}
	}
}

func AudioOnlyLightPalette() Palette {
	return Palette{
		Fill:   color.NRGBA{R: 0xf6, G: 0xea, B: 0xd6, A: 0xff},
		Stroke: color.NRGBA{R: 0xf6, G: 0xea, B: 0xd6, A: 0xff},
		Text:   color.NRGBA{R: 0x83, G: 0x64, B: 0x2f, A: 0xff},
	}
}

func AudioOnlyDarkPalette() Palette {
	return Palette{
		Fill:   color.NRGBA{R: 0x48, G: 0x3c, B: 0x2d, A: 0xff},
		Stroke: color.NRGBA{R: 0x48, G: 0x3c, B: 0x2d, A: 0xff},
		Text:   color.NRGBA{R: 0xe1, G: 0xc0, B: 0x92, A: 0xff},
	}
}

func Hex(c color.NRGBA) string {
	const digits = "0123456789abcdef"
	buf := []byte{'#', 0, 0, 0, 0, 0, 0}
	buf[1] = digits[c.R>>4]
	buf[2] = digits[c.R&0x0f]
	buf[3] = digits[c.G>>4]
	buf[4] = digits[c.G&0x0f]
	buf[5] = digits[c.B>>4]
	buf[6] = digits[c.B&0x0f]
	return string(buf)
}
