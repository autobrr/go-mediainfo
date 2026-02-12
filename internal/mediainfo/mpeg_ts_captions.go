package mediainfo

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

func appendTSCaptionStreams(out *[]Stream, video *tsStream) {
	if out == nil || video == nil || video.kind != StreamVideo {
		return
	}
	has608 := video.hasValidCEA608()
	has708 := len(video.dtvccServices) > 0
	if !has608 && !has708 {
		return
	}
	duration := ptsDuration(video.pts)
	if duration <= 0 {
		return
	}
	delay := 0.0
	if video.pts.has() {
		delay = float64(video.pts.min) / 90000.0
	}
	fps := 0.0
	if video.hasMPEG2Info {
		if video.mpeg2Info.FrameRateNumer > 0 && video.mpeg2Info.FrameRateDenom > 0 {
			fps = float64(video.mpeg2Info.FrameRateNumer) / float64(video.mpeg2Info.FrameRateDenom)
		} else if video.mpeg2Info.FrameRate > 0 {
			fps = video.mpeg2Info.FrameRate
		}
	}
	if fps > 0 {
		// PTS deltas cover (N-1) frame intervals; official mediainfo reports full duration (N intervals).
		duration += 1.0 / fps
		duration = math.Round(duration*1000) / 1000
	}
	if shouldSuppressEarlyService2Only(video, delay, fps) {
		return
	}
	menuID := video.programNumber
	videoPID := video.pid
	// MediaInfoLib suppresses Text_Lines_Count when it has jumped/unsynched during parsing.
	// With the default CLI ParseSpeed (0.5), this tends to happen on longer TS where scanning
	// is bounded (e.g. ~30s). Heuristic: only emit Lines_Count for short streams.
	emitLinesCount := duration > 0 && duration <= 30.0

	if shouldEmitTSCC1(video) {
		startCommand := 0.0
		if fps > 0 && video.ccOdd.firstCommandPTS != 0 {
			// MediaInfoLib tracks command time from FrameInfo.DTS; align to the nearest frame time.
			ptsSec := float64(video.ccOdd.firstCommandPTS) / 90000.0
			frame := int64(math.Round((ptsSec-delay)*fps)) - 1
			if frame < 0 {
				frame = 0
			}
			startCommand = delay + float64(frame)/fps
		} else if fps > 0 && video.ccOdd.firstCommandFrame > 0 {
			startCommand = delay + float64(video.ccOdd.firstCommandFrame)/fps
		}
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-608", "CC1", startCommand, emitLinesCount))
	}
	if shouldEmitTSCC3(video) {
		startCommand := 0.0
		if fps > 0 && video.ccEven.firstCommandPTS != 0 {
			ptsSec := float64(video.ccEven.firstCommandPTS) / 90000.0
			frame := int64(math.Round((ptsSec-delay)*fps)) - 1
			if frame < 0 {
				frame = 0
			}
			startCommand = delay + float64(frame)/fps
		} else if fps > 0 && video.ccEven.firstCommandFrame > 0 {
			startCommand = delay + float64(video.ccEven.firstCommandFrame)/fps
		}
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-608", "CC3", startCommand, emitLinesCount))
	}
	if len(video.dtvccServices) > 0 {
		services := make([]int, 0, len(video.dtvccServices))
		for svc := range video.dtvccServices {
			services = append(services, svc)
		}
		sort.Ints(services)
		for _, svc := range services {
			if svc <= 0 {
				continue
			}
			*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-708", strconv.Itoa(svc), 0, emitLinesCount))
		}
	}
}

func shouldSuppressEarlyService2Only(video *tsStream, delay, fps float64) bool {
	if video == nil || shouldEmitTSCC1(video) || !video.ccEven.found {
		return false
	}
	if len(video.dtvccServices) != 1 {
		return false
	}
	if _, ok := video.dtvccServices[2]; !ok {
		return false
	}
	start := 0.0
	if fps > 0 && video.ccEven.firstCommandPTS != 0 {
		ptsSec := float64(video.ccEven.firstCommandPTS) / 90000.0
		frame := int64(math.Round((ptsSec-delay)*fps)) - 1
		if frame < 0 {
			frame = 0
		}
		start = delay + float64(frame)/fps
	} else if fps > 0 && video.ccEven.firstCommandFrame > 0 {
		start = delay + float64(video.ccEven.firstCommandFrame)/fps
	}
	return start > 0 && start < 0.5
}

func shouldEmitTSCC3(video *tsStream) bool {
	if video == nil || !video.ccEven.found {
		return false
	}
	if video.ccEven.firstCommandPTS != 0 || video.ccEven.firstCommandFrame > 0 {
		return true
	}
	// Without any DTVCC services, keep legacy CC3 fallback when a display type was detected.
	if len(video.dtvccServices) == 0 && video.ccEven.firstType != "" {
		return true
	}
	return false
}

func shouldEmitTSCC1(video *tsStream) bool {
	if video == nil || !video.ccOdd.found {
		return false
	}
	return video.ccOdd.firstCommandPTS != 0 || video.ccOdd.firstCommandFrame > 0
}

func buildTSCaptionStream(videoPID uint16, programNumber uint16, delaySeconds float64, duration float64, format string, service string, startCommandSeconds float64, emitLinesCount bool) Stream {
	idLabel := fmt.Sprintf("%s-%s", formatID(uint64(videoPID)), service)
	jsonID := fmt.Sprintf("%d-%s", videoPID, service)
	fields := []Field{
		{Name: "ID", Value: idLabel},
	}
	if programNumber > 0 {
		fields = append(fields, Field{Name: "Menu ID", Value: formatID(uint64(programNumber))})
	}
	fields = append(fields,
		Field{Name: "Format", Value: format},
		Field{Name: "Muxing mode", Value: "A/53 / DTVCC Transport"},
		Field{Name: "Muxing mode, more info", Value: "Muxed in Video #1"},
		Field{Name: "Duration", Value: formatDuration(duration)},
	)
	fields = append(fields,
		Field{Name: "Bit rate mode", Value: "Constant"},
		Field{Name: "Stream size", Value: "0.00 Byte (0%)"},
	)

	jsonExtras := map[string]string{
		"ID":          jsonID,
		"StreamOrder": "0-0",
		"Duration":    formatJSONSeconds(duration),
		"StreamSize":  "0",
		"Video_Delay": "0.000",
	}
	if emitLinesCount && format == "EIA-608" {
		jsonExtras["Lines_Count"] = "0"
	}
	if programNumber > 0 {
		jsonExtras["MenuID"] = strconv.FormatUint(uint64(programNumber), 10)
	}
	if delaySeconds > 0 {
		jsonExtras["Delay"] = fmt.Sprintf("%.9f", delaySeconds)
		jsonExtras["Delay_Source"] = "Container"
	}
	if format == "EIA-608" && startCommandSeconds > 0 {
		jsonExtras["Duration_Start_Command"] = formatJSONSeconds6(startCommandSeconds)
	}
	jsonRaw := map[string]string{
		"extra": renderJSONObject([]jsonKV{
			{Key: "CaptionServiceDescriptor_IsPresent", Val: "No"},
			{Key: "CaptionServiceName", Val: service},
		}, false),
	}
	return Stream{Kind: StreamText, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw}
}
