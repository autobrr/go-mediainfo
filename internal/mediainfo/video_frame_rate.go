package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// normalizeVideoFrameRates applies the CLI's standard-rate rounding after
// container measurements and updates General facts copied from the old rate.
func normalizeVideoFrameRates(general *Stream, streams []Stream) {
	primaryVideo := -1
	previousRate := ""
	for i := range streams {
		stream := &streams[i]
		if stream.Kind != StreamVideo {
			continue
		}
		if primaryVideo < 0 {
			display, _ := canonicalSeedTextValue(*stream, "Frame rate")
			if display == "" {
				display = findField(stream.Fields, "Frame rate")
			}
			if display != "" {
				primaryVideo = i
				previousRate = extractLeadingNumber(display)
			}
		}
		if originalNum, _ := canonicalSeedValue(*stream, "FrameRate_Original_Num"); originalNum != "" {
			continue
		}
		changed := false
		for _, field := range []struct {
			name  fieldName
			label string
		}{
			{"FrameRate", "Frame rate"},
			{"FrameRate_Nominal", "Nominal frame rate"},
			{"FrameRate_Original", "Original frame rate"},
		} {
			value, _ := canonicalSeedValue(*stream, field.name)
			rate, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			rounded := roundVideoFrameRate(rate)
			if !(math.Abs(rounded-rate) >= 0.000999999) {
				continue
			}
			raw := fmt.Sprintf("%.3f", rounded)
			display, _ := canonicalSeedTextValue(*stream, field.label)
			if display != "" {
				// Retain any explicit rational suffix and its existing visibility.
				_, suffix, found := strings.Cut(display, " ")
				display = raw
				if found {
					display += " " + suffix
				}
			}
			replaceCanonicalSeedFill(stream, field.name, raw, field.label, display)
			changed = true
		}
		if changed {
			refreshCanonicalCompatibilitySnapshot(stream)
		}
	}
	if general == nil || primaryVideo < 0 || previousRate == "" {
		return
	}
	rate, _ := canonicalSeedValue(streams[primaryVideo], "FrameRate")
	if rate == "" || rate == previousRate {
		return
	}
	// Containers can copy General facts before this normalization. Update
	// matching copies independently; retain other values and missing fields.
	if value, _ := canonicalSeedValue(*general, "FrameRate"); value == previousRate {
		replaceCanonicalSeedFill(general, "FrameRate", rate, "", "")
	}
	if display, _ := canonicalSeedTextValue(*general, "Frame rate"); extractLeadingNumber(display) == previousRate {
		replaceCanonicalSeedText(general, "Frame rate", strings.Replace(display, previousRate, rate, 1))
	}
	for i := range general.Fields {
		field := &general.Fields[i]
		if field.Name == "Frame rate" && extractLeadingNumber(field.Value) == previousRate {
			field.Value = strings.Replace(field.Value, previousRate, rate, 1)
		}
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
