package mediainfo

import (
	"fmt"
	"sort"
	"strconv"
)

func appendTSCaptionStreams(out *[]Stream, video *tsStream) {
	if out == nil || video == nil || video.kind != StreamVideo {
		return
	}
	has608 := video.ccOdd.found || video.ccEven.found
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
	menuID := video.programNumber
	videoPID := video.pid

	if video.ccOdd.found {
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-608", "CC1", video.ccOdd.firstCommandPTS))
	}
	if video.ccEven.found {
		*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-608", "CC3", video.ccEven.firstCommandPTS))
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
			*out = append(*out, buildTSCaptionStream(videoPID, menuID, delay, duration, "EIA-708", strconv.Itoa(svc), 0))
		}
	}
}

func buildTSCaptionStream(videoPID uint16, programNumber uint16, delaySeconds float64, duration float64, format string, service string, firstCommandPTS uint64) Stream {
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
	if format == "EIA-608" && firstCommandPTS > 0 {
		start := float64(firstCommandPTS) / 90000.0
		fields = append(fields, Field{Name: "Start time (commands)", Value: formatDuration(start)})
	}
	fields = append(fields,
		Field{Name: "Bit rate mode", Value: "Constant"},
		Field{Name: "Stream size", Value: "0.00 Byte (0%)"},
	)

	jsonExtras := map[string]string{
		"ID":          jsonID,
		"StreamOrder": "0-0",
		"StreamSize":  "0",
		"Video_Delay": "0.000",
	}
	if programNumber > 0 {
		jsonExtras["MenuID"] = strconv.FormatUint(uint64(programNumber), 10)
	}
	if delaySeconds > 0 {
		jsonExtras["Delay"] = fmt.Sprintf("%.9f", delaySeconds)
		jsonExtras["Delay_Source"] = "Container"
	}
	if format == "EIA-608" && firstCommandPTS > 0 {
		jsonExtras["Duration_Start_Command"] = formatJSONSeconds6(float64(firstCommandPTS) / 90000.0)
	}
	jsonRaw := map[string]string{
		"extra": renderJSONObject([]jsonKV{
			{Key: "CaptionServiceDescriptor_IsPresent", Val: "No"},
			{Key: "CaptionServiceName", Val: service},
		}, false),
	}
	return Stream{Kind: StreamText, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw}
}
