package mediainfo

import "fmt"

func buildCCTextStream(entry *psStream, videoDelay float64, videoDuration float64, frameRate float64) *Stream {
	if entry == nil || !entry.ccFound {
		return nil
	}
	service := ccServiceName(entry.ccService)
	idLabel := fmt.Sprintf("%s-%s", formatID(uint64(entry.id)), service)
	fields := []Field{
		{Name: "ID", Value: idLabel},
		{Name: "Format", Value: "EIA-608"},
		{Name: "Muxing mode, more info", Value: "Muxed in Video #1"},
	}

	if videoDuration > 0 {
		fields = append(fields, Field{Name: "Duration", Value: formatDuration(videoDuration)})
	}

	start := ccPTSSeconds(entry.ccFirstDisplayPTS)
	if start == 0 {
		start = ccPTSSeconds(entry.ccFirstPTS)
	}
	if start == 0 && entry.ccFirstFrame > 0 && frameRate > 0 {
		start = float64(entry.ccFirstFrame) / frameRate
	}
	end := ccPTSSeconds(entry.ccLastPTS)
	if end == 0 {
		end = start
	}
	visible := 0.0
	if end > start {
		visible = end - start
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
	framesBefore := entry.ccFirstFrame
	if framesBefore < 0 {
		framesBefore = 0
	}
	fields = append(fields, Field{Name: "Count of frames before first event", Value: fmt.Sprintf("%d", framesBefore)})
	firstType := entry.ccFirstType
	if firstType == "" {
		firstType = "PopOn"
	}
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
		stream.JSON["FirstPacketOrder"] = fmt.Sprintf("%d", entry.firstPacketOrder)
	}
	if videoDuration > 0 {
		stream.JSON["Duration"] = formatJSONSeconds(videoDuration)
	}
	if visible > 0 {
		stream.JSON["Duration_Start2End"] = formatJSONSeconds(visible)
	}
	startCommand := ccPTSSeconds(entry.ccFirstCommandPTS)
	if startCommand == 0 {
		startCommand = start
	}
	if startCommand > 0 {
		stream.JSON["Duration_Start_Command"] = formatJSONSeconds(startCommand)
	}
	if start > 0 {
		stream.JSON["Duration_Start"] = formatJSONSeconds(start)
	}
	if end > 0 {
		stream.JSON["Duration_End"] = formatJSONSeconds(end)
		stream.JSON["Duration_End_Command"] = formatJSONSeconds(end)
	}
	stream.JSON["BitRate_Mode"] = "CBR"
	if videoDelay > 0 {
		stream.JSON["Delay"] = fmt.Sprintf("%.9f", videoDelay)
	}
	stream.JSON["Video_Delay"] = "0.000"
	stream.JSON["StreamSize"] = "0"
	stream.JSON["FirstDisplay_Delay_Frames"] = fmt.Sprintf("%d", framesBefore)
	stream.JSON["FirstDisplay_Type"] = firstType
	stream.JSONRaw["extra"] = renderJSONObject([]jsonKV{{Key: "CaptionServiceName", Val: service}}, false)
	return &stream
}

func ccPTSSeconds(value uint64) float64 {
	if value == 0 {
		return 0
	}
	return float64(value) / 90000.0
}
