package mediainfo

import (
	"fmt"
	"math"
	"strconv"
)

func buildCCTextStream(entry *psStream, videoDelay float64, videoDuration float64, frameRate float64) *Stream {
	track, service := selectCCTrack(entry)
	if track == nil {
		return nil
	}
	service = ccServiceName(service)
	idLabel := fmt.Sprintf("%s-%s", formatID(uint64(entry.id)), service)
	fields := []Field{
		{Name: "ID", Value: idLabel},
		{Name: "Format", Value: "EIA-608"},
		{Name: "Muxing mode, more info", Value: "Muxed in Video #1"},
	}

	if videoDuration > 0 {
		fields = append(fields, Field{Name: "Duration", Value: formatDuration(videoDuration)})
	}

	start := ccPTSSeconds(track.firstDisplayPTS)
	if start == 0 {
		start = ccPTSSeconds(track.firstPTS)
	}
	if start == 0 && track.firstFrame > 0 && frameRate > 0 {
		start = float64(track.firstFrame) / frameRate
	}
	end := ccPTSSeconds(track.lastPTS)
	firstType := track.firstType
	if firstType == "" {
		firstType = "PopOn"
	}
	ccRate := ccFrameRateForVideo(frameRate)
	commandOffset := 0.0
	displayOffset := 0.0
	if ccRate > 0 {
		commandOffset = 1.0 / ccRate
		if firstType == "PopOn" {
			displayOffset = 4.0 / ccRate
		}
	}
	if start > 0 && displayOffset > 0 {
		start -= displayOffset
		if start < 0 {
			start = 0
		}
	}
	if end > 0 && displayOffset > 0 {
		end -= displayOffset
		if end < 0 {
			end = 0
		}
	}
	if end == 0 {
		end = start
	}
	visible := 0.0
	visibleBase := 0.0
	if end > start {
		visibleBase = end - start
	}
	if ccRate > 0 && visibleBase > 0 {
		visible = math.Round(visibleBase*2000) / 2000
	} else {
		visible = visibleBase
	}
	if visible > 0 {
		fields = append(fields, Field{Name: "Duration of the visible content", Value: formatDuration(visible)})
	}
	if start > 0 {
		fields = append(fields, Field{Name: "Start time", Value: formatDuration(start)})
	}
	if end > 0 {
		fields = append(fields, Field{Name: "End time", Value: formatDuration(end)})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
	fields = append(fields, Field{Name: "Stream size", Value: "0.00 Byte (0%)"})
	framesBefore := max(track.firstFrame, 0)
	fields = append(fields, Field{Name: "Count of frames before first event", Value: strconv.Itoa(framesBefore)})
	fields = append(fields, Field{Name: "Type of the first event", Value: firstType})
	fields = append(fields, Field{Name: "Caption service name", Value: service})

	stream := Stream{
		Kind:                StreamText,
		Fields:              fields,
		JSON:                map[string]string{},
		JSONRaw:             map[string]string{},
		JSONSkipStreamOrder: true,
		JSONSkipComputed:    true,
	}
	stream.JSON["ID"] = fmt.Sprintf("%d-%s", entry.id, service)
	if entry.firstPacketOrder >= 0 {
		stream.JSON["FirstPacketOrder"] = strconv.Itoa(entry.firstPacketOrder)
	}
	if videoDuration > 0 {
		stream.JSON["Duration"] = formatJSONSeconds(videoDuration)
	}
	if visible > 0 {
		stream.JSON["Duration_Start2End"] = formatJSONSeconds6(visible)
	}
	startCommand := ccPTSSeconds(track.firstCommandPTS)
	if startCommand > 0 && commandOffset > 0 {
		startCommand -= commandOffset
		if startCommand < 0 {
			startCommand = 0
		}
	}
	if startCommand == 0 {
		startCommand = start
	}
	if startCommand > 0 {
		stream.JSON["Duration_Start_Command"] = formatJSONSeconds6(startCommand)
	}
	if start > 0 {
		stream.JSON["Duration_Start"] = formatJSONSeconds6(start)
	}
	if end > 0 {
		stream.JSON["Duration_End"] = formatJSONSeconds6(end)
		stream.JSON["Duration_End_Command"] = formatJSONSeconds6(end)
	}
	stream.JSON["BitRate_Mode"] = "CBR"
	if videoDelay > 0 {
		stream.JSON["Delay"] = fmt.Sprintf("%.9f", videoDelay)
	}
	stream.JSON["Video_Delay"] = "0.000"
	stream.JSON["StreamSize"] = "0"
	stream.JSON["FirstDisplay_Delay_Frames"] = strconv.Itoa(framesBefore)
	stream.JSON["FirstDisplay_Type"] = firstType
	stream.JSONRaw["extra"] = renderJSONObject([]jsonKV{{Key: "CaptionServiceName", Val: service}}, false)
	return &stream
}

func selectCCTrack(entry *psStream) (*ccTrack, string) {
	if entry == nil || !entry.ccFound {
		return nil, ""
	}
	if entry.ccEven.found {
		return &entry.ccEven, "CC3"
	}
	if entry.ccOdd.found {
		return &entry.ccOdd, "CC1"
	}
	return nil, ""
}

func ccPTSSeconds(value uint64) float64 {
	if value == 0 {
		return 0
	}
	return float64(value) / 90000.0
}

func ccFrameRateForVideo(frameRate float64) float64 {
	if frameRate <= 0 {
		return 0
	}
	if frameRate > 23 && frameRate < 24.5 {
		return 29.97
	}
	if frameRate > 29 && frameRate < 30.5 {
		return 29.97
	}
	if frameRate > 24.5 && frameRate < 25.5 {
		return 25.0
	}
	return frameRate
}
