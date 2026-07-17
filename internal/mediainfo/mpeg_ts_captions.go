package mediainfo

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// appendTSCaptionStreams appends canonical EIA-608 and EIA-708 streams derived
// from one transport-stream video PID.
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
	presentationDuration := false
	if len(video.captionServices) > 0 && video.pts.has() && video.mpeg2PresentationEndPTS > video.pts.min {
		duration = float64(video.mpeg2PresentationEndPTS-video.pts.min) / 90000.0
		presentationDuration = true
	}
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
	if fps > 0 && !presentationDuration {
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
		descriptor, descriptorPresent := video.captionServices["CC1"]
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, fps, "EIA-608", "CC1", &video.ccOdd, descriptor, descriptorPresent, emitLinesCount))
	}
	if shouldEmitTSCC3(video) {
		descriptor, descriptorPresent := video.captionServices["CC3"]
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, fps, "EIA-608", "CC3", &video.ccEven, descriptor, descriptorPresent, emitLinesCount))
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
			service := strconv.Itoa(svc)
			descriptor, descriptorPresent := video.captionServices[service]
			*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, fps, "EIA-708", service, nil, descriptor, descriptorPresent, emitLinesCount))
		}
	}
}

// shouldSuppressEarlyService2Only rejects the short-start service-2 outlier
// emitted without a timed CC1 stream.
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

// shouldEmitTSCC3 reports whether field-two captions have command timing or a
// supported no-DTVCC display fallback.
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

// shouldEmitTSCC1 reports whether field-one captions have command timing
// sufficient for a canonical CC1 stream.
func shouldEmitTSCC1(video *tsStream) bool {
	if video == nil || !video.ccOdd.found {
		return false
	}
	return video.ccOdd.firstCommandPTS != 0 || video.ccOdd.firstCommandFrame > 0
}

// buildTSCaptionStream constructs one canonical caption stream from its
// service identity and measured timing.
func buildTSCaptionStream(videoPID uint16, programNumber uint16, delaySeconds float64, duration float64, fps float64, format string, service string, track *ccTrack, descriptor tsCaptionService, descriptorPresent bool, emitLinesCount bool) Stream {
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
	streamFacts := &mpegTSStructuredFacts{}
	startCommand := tsCaptionCommandStart(track, delaySeconds, fps)
	if track != nil && track.firstContentPTS > 0 {
		start := mediaInfoCaptionTimestamp(track.firstContentPTS, fps, -1)
		end := mediaInfoCaptionTimestamp(track.lastContentPTS, fps, -1)
		endCommandFrameOffset := 1
		if track.commandDuplicated {
			endCommandFrameOffset = -1
		}
		endCommand := mediaInfoCaptionTimestamp(track.lastCommandPTS, fps, endCommandFrameOffset)
		if end == 0 {
			end = endCommand
		}
		if endCommand == 0 {
			endCommand = end
		}
		visible := 0.0
		if end > start {
			visible = math.Round((end-start)*1000) / 1000
			fields = append(fields, Field{Name: "Duration of the visible content", Value: formatDuration(visible)})
		}
		fields = append(fields, Field{Name: "Start time", Value: formatDuration(start)})
		if end > 0 {
			fields = append(fields, Field{Name: "End time", Value: formatDuration(end)})
		}
		if track.firstDisplayFrame >= 0 {
			fields = append(fields, Field{Name: "Count of frames before first event", Value: strconv.Itoa(track.firstDisplayFrame)})
		}
		if track.firstType != "" {
			fields = append(fields, Field{Name: "Type of the first event", Value: track.firstType})
		}

		if visible > 0 {
			streamFacts.Set("Duration_Start2End", formatJSONSeconds6(visible))
		}
		streamFacts.Set("Duration_Start", formatJSONSeconds6(start))
		if end > 0 {
			streamFacts.Set("Duration_End", formatJSONSeconds6(end))
		}
		if endCommand > 0 {
			streamFacts.Set("Duration_End_Command", formatJSONSeconds6(endCommand))
		}
		if track.firstDisplayFrame >= 0 {
			streamFacts.Set("FirstDisplay_Delay_Frames", strconv.Itoa(track.firstDisplayFrame))
		}
		if track.firstType != "" {
			streamFacts.Set("FirstDisplay_Type", track.firstType)
		}
	}

	streamFacts.Set("ID", jsonID)
	streamFacts.Set("StreamOrder", "0-0")
	streamFacts.Set("Duration", formatJSONSeconds(duration))
	streamFacts.Set("StreamSize", "0")
	streamFacts.Set("Video_Delay", "0.000")
	if emitLinesCount && format == "EIA-608" {
		streamFacts.Set("Lines_Count", "0")
	}
	if programNumber > 0 {
		streamFacts.Set("MenuID", strconv.FormatUint(uint64(programNumber), 10))
	}
	if delaySeconds > 0 {
		streamFacts.Set("Delay", fmt.Sprintf("%.9f", delaySeconds))
		streamFacts.Set("Delay_Source", "Container")
	}
	if format == "EIA-608" && startCommand > 0 {
		streamFacts.Set("Duration_Start_Command", formatJSONSeconds6(startCommand))
	}
	if descriptor.Language != "" {
		fields = append(fields, Field{Name: "Language", Value: descriptor.Language})
		streamFacts.Set("Language", descriptor.Language)
	}
	descriptorValue := "No"
	if descriptorPresent {
		descriptorValue = "Yes"
	}
	extra := structuredObjectFromKVs([]jsonKV{
		{Key: "CaptionServiceName", Val: service},
		{Key: "CaptionServiceContent_IsPresent", Val: "Yes"},
		{Key: "CaptionServiceDescriptor_IsPresent", Val: descriptorValue},
	})
	return buildCanonicalMPEGTSStream(StreamText, nil, fields, streamFacts, &extra, false)
}

// tsCaptionCommandStart aligns the first CEA-608 command to MediaInfo's
// zero-based video-frame timestamp.
func tsCaptionCommandStart(track *ccTrack, delay, fps float64) float64 {
	if track == nil || fps <= 0 {
		return 0
	}
	if track.firstCommandPTS != 0 {
		return mediaInfoCaptionTimestamp(track.firstCommandPTS, fps, -1)
	}
	if track.firstCommandFrame > 0 {
		return delay + float64(track.firstCommandFrame)/fps
	}
	return 0
}

// mediaInfoCaptionTimestamp converts a GA94 picture PTS to the synthesized
// EIA-608 parser timestamp. MediaInfo stores this value as float32
// milliseconds, which matters for large absolute transport timestamps.
func mediaInfoCaptionTimestamp(pts uint64, fps float64, frameOffset int) float64 {
	if pts == 0 || fps <= 0 {
		return 0
	}
	seconds := float64(pts)/90000 + float64(frameOffset)/fps
	return float64(float32(seconds*1000)) / 1000
}
