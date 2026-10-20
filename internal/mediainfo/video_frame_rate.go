package mediainfo

import (
	"fmt"
	"math"
	"strconv"
)

// normalizeVideoFrameRates applies the CLI's standard-rate rounding after
// container measurements, refreshes derived facts, and copies the first video's
// rate to General, even when a later video has a different or nonempty rate.
func normalizeVideoFrameRates(general *Stream, streams []Stream) {
	primaryVideo := -1
	for i := range streams {
		stream := &streams[i]
		if stream.Kind != StreamVideo {
			continue
		}
		if primaryVideo < 0 {
			primaryVideo = i
		}
		previousRate, _ := canonicalSeedValue(*stream, "FrameRate")
		nominal, _ := canonicalSeedValue(*stream, "FrameRate_Nominal")
		rate32, _ := strconv.ParseFloat(previousRate, 32)
		nominal32, _ := strconv.ParseFloat(nominal, 32)
		changed := nominal32 > rate32*0.9995 && nominal32 < rate32*1.0005
		if changed {
			clearCanonicalSeedField(stream, "FrameRate_Nominal", "Nominal frame rate")
			display, _ := canonicalSeedTextValue(*stream, "Frame rate")
			if display != "" {
				display = nominal + " FPS"
			}
			replaceCanonicalSeedFill(stream, "FrameRate", nominal, "Frame rate", display)
		}
		originalNum, _ := canonicalSeedValue(*stream, "FrameRate_Original_Num")
		for _, field := range []struct {
			name  fieldName
			label string
		}{
			{"FrameRate", "Frame rate"},
			{"FrameRate_Nominal", "Nominal frame rate"},
			{"FrameRate_Original", "Original frame rate"},
		} {
			if originalNum != "" {
				break
			}
			value, _ := canonicalSeedValue(*stream, field.name)
			rate, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			rounded := roundVideoFrameRate(rate)
			if math.Abs(rounded-rate) >= 0.000999999 {
				value = fmt.Sprintf("%.3f", rounded)
				display, _ := canonicalSeedTextValue(*stream, field.label)
				if display != "" {
					display = value + " FPS"
				}
				replaceCanonicalSeedFill(stream, field.name, value, field.label, display)
				changed = true
			}
			if actual, _ := canonicalSeedValue(*stream, "FrameRate"); field.name != "FrameRate" && value != "" && value == actual {
				clearCanonicalSeedField(stream, field.name, field.label)
				changed = true
			}
		}
		rate, _ := canonicalSeedValue(*stream, "FrameRate")
		if rate != previousRate {
			fps, _ := strconv.ParseFloat(rate, 64)
			num, den := matroskaFrameRateRatio(fps)
			if num > 0 && den > 0 {
				replaceCanonicalSeedFill(stream, "FrameRate_Num", strconv.Itoa(num), "", "")
				replaceCanonicalSeedFill(stream, "FrameRate_Den", strconv.Itoa(den), "", "")
			} else {
				clearCanonicalSeedField(stream, "FrameRate_Num", "")
				clearCanonicalSeedField(stream, "FrameRate_Den", "")
			}
			if display, _ := canonicalSeedTextValue(*stream, "Frame rate"); display != "" {
				display = formatFrameRate(fps)
				if den > 1 {
					display = fmt.Sprintf("%s (%d/%d) FPS", rate, num, den)
				}
				replaceCanonicalSeedText(stream, "Frame rate", display)
			}
			bitRate, _ := canonicalSeedValue(*stream, "BitRate")
			bps, _ := strconv.ParseFloat(bitRate, 64)
			if bps == 0 {
				bitRate, _ = canonicalSeedValue(*stream, "BitRate_Nominal")
				bps, _ = strconv.ParseFloat(bitRate, 64)
			}
			width, _ := canonicalSeedValue(*stream, "Width")
			height, _ := canonicalSeedValue(*stream, "Height")
			w, _ := strconv.ParseUint(width, 10, 64)
			h, _ := strconv.ParseUint(height, 10, 64)
			if density := formatBitsPerPixelFrame(bps, w, h, fps); density != "" {
				if value, _ := canonicalSeedValue(*stream, "Bits-(Pixel*Frame)"); value != "" {
					replaceCanonicalSeedFill(stream, "Bits-(Pixel*Frame)", density, "Bits/(Pixel*Frame)", density)
				} else {
					replaceCanonicalSeedText(stream, "Bits/(Pixel*Frame)", density)
				}
			}
		}
		if changed {
			refreshCanonicalCompatibilitySnapshot(stream)
		}
	}
	if general == nil || primaryVideo < 0 {
		return
	}
	rate, _ := canonicalSeedValue(streams[primaryVideo], "FrameRate")
	clearCanonicalSeedField(general, "FrameRate", "Frame rate")
	fields := general.Fields[:0]
	for _, field := range general.Fields {
		if field.Name != "Frame rate" {
			fields = append(fields, field)
		}
	}
	general.Fields = fields
	if rate != "" {
		display := rate + " FPS"
		replaceCanonicalSeedFill(general, "FrameRate", rate, "Frame rate", display)
		general.Fields = append(general.Fields, Field{Name: "Frame rate", Value: display})
	}
}

// roundVideoFrameRate mirrors MediaInfoLib's Video_FrameRate_Rounded windows
// and minimum correction, using the already stored decimal rate as input.
func roundVideoFrameRate(rate float64) float64 {
	rounded := rate
	switch {
	case rate > 9.990 && rate <= 10.010:
		rounded = 10
	case rate > 11.984 && rate <= 11.994:
		rounded = 11.988
	case rate > 11.994 && rate <= 12.010:
		rounded = 12
	case rate > 14.980 && rate <= 14.990:
		rounded = 14.985
	case rate > 14.990 && rate <= 15.010:
		rounded = 15
	case rate > 23.952 && rate <= 23.988:
		rounded = 23.976
	case rate > 23.988 && rate <= 24.024:
		rounded = 24
	case rate > 24.975 && rate <= 25.025:
		rounded = 25
	case rate > 29.940 && rate <= 29.985:
		rounded = 29.970
	case rate > 29.970 && rate <= 30.030:
		rounded = 30
	case rate > 23.952*2 && rate <= 23.988*2:
		rounded = 23.976 * 2
	case rate > 23.988*2 && rate <= 24.024*2:
		rounded = 24 * 2
	case rate > 24.975*2 && rate <= 25.025*2:
		rounded = 25 * 2
	case rate > 29.940*2 && rate <= 29.985*2:
		rounded = 29.970 * 2
	case rate > 29.970*2 && rate <= 30.030*2:
		rounded = 30 * 2
	}
	if math.Abs(rounded-rate) >= 0.000999999 {
		return rounded
	}
	return rate
}
