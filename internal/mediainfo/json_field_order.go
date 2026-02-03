package mediainfo

import "sort"

var jsonGeneralFieldOrder = map[string]int{
	"@type":                    0,
	"ID":                       1,
	"UniqueID":                 1,
	"VideoCount":               2,
	"AudioCount":               3,
	"TextCount":                4,
	"ImageCount":               5,
	"MenuCount":                6,
	"FileExtension":            7,
	"Format":                   8,
	"Format_Version":           9,
	"Format_Profile":           10,
	"CodecID":                  11,
	"CodecID_Compatible":       12,
	"FileSize":                 13,
	"Duration":                 14,
	"OverallBitRate_Mode":      15,
	"OverallBitRate":           16,
	"FrameRate":                17,
	"FrameCount":               18,
	"StreamSize":               19,
	"HeaderSize":               20,
	"DataSize":                 21,
	"FooterSize":               22,
	"IsStreamable":             23,
	"File_Created_Date":        24,
	"File_Created_Date_Local":  25,
	"File_Modified_Date":       26,
	"File_Modified_Date_Local": 27,
	"Encoded_Application":      28,
	"Encoded_Library":          29,
	"extra":                    30,
}

var jsonVideoFieldOrder = map[string]int{
	"@type":                             0,
	"StreamOrder":                       1,
	"ID":                                2,
	"MenuID":                            3,
	"UniqueID":                          3,
	"Format":                            4,
	"Format_Profile":                    5,
	"Format_Level":                      6,
	"Format_Settings_CABAC":             7,
	"Format_Settings_RefFrames":         8,
	"CodecID":                           9,
	"Duration":                          10,
	"BitRate":                           11,
	"BitRate_Nominal":                   12,
	"BitRate_Maximum":                   13,
	"Width":                             14,
	"Height":                            15,
	"Sampled_Width":                     16,
	"Sampled_Height":                    17,
	"PixelAspectRatio":                  18,
	"DisplayAspectRatio":                19,
	"Rotation":                          20,
	"FrameRate_Mode":                    21,
	"FrameRate_Mode_Original":           22,
	"FrameRate":                         23,
	"FrameRate_Num":                     24,
	"FrameRate_Den":                     25,
	"FrameCount":                        26,
	"ChromaSubsampling":                 27,
	"BitDepth":                          28,
	"ScanType":                          29,
	"Delay":                             30,
	"Delay_Source":                      31,
	"StreamSize":                        32,
	"Encoded_Library":                   33,
	"Encoded_Library_Name":              34,
	"Encoded_Library_Version":           35,
	"Encoded_Library_Settings":          36,
	"Default":                           37,
	"Forced":                            38,
	"colour_description_present":        39,
	"colour_description_present_Source": 40,
	"colour_range":                      41,
	"colour_range_Source":               42,
	"List_StreamKind":                   43,
	"List_StreamPos":                    44,
	"ServiceName":                       45,
	"ServiceProvider":                   46,
	"ServiceType":                       47,
	"extra":                             48,
}

var jsonAudioFieldOrder = map[string]int{
	"@type":                     0,
	"StreamOrder":               1,
	"ID":                        2,
	"MenuID":                    3,
	"UniqueID":                  3,
	"Format":                    4,
	"Format_Version":            5,
	"Format_Settings_SBR":       5,
	"Format_AdditionalFeatures": 6,
	"MuxingMode":                7,
	"CodecID":                   7,
	"Duration":                  8,
	"Source_Duration":           9,
	"Source_Duration_LastFrame": 10,
	"BitRate_Mode":              11,
	"BitRate":                   12,
	"BitRate_Maximum":           13,
	"Channels":                  14,
	"ChannelPositions":          15,
	"ChannelLayout":             16,
	"SamplesPerFrame":           17,
	"SamplingRate":              18,
	"SamplingCount":             19,
	"FrameRate":                 20,
	"FrameCount":                21,
	"Source_FrameCount":         22,
	"Compression_Mode":          23,
	"Delay":                     24,
	"Delay_Source":              25,
	"Video_Delay":               26,
	"Encoded_Library":           27,
	"StreamSize":                28,
	"Source_StreamSize":         29,
	"Default":                   30,
	"Forced":                    31,
	"AlternateGroup":            32,
	"extra":                     33,
}

func sortJSONFields(kind StreamKind, fields []jsonKV) []jsonKV {
	order := jsonVideoFieldOrder
	switch kind {
	case StreamGeneral:
		order = jsonGeneralFieldOrder
	case StreamAudio:
		order = jsonAudioFieldOrder
	case StreamVideo:
		order = jsonVideoFieldOrder
	}
	positions := map[string]int{}
	for i, field := range fields {
		positions[field.Key] = i
	}
	sort.SliceStable(fields, func(i, j int) bool {
		ai, aok := order[fields[i].Key]
		aj, bok := order[fields[j].Key]
		switch {
		case aok && bok:
			return ai < aj
		case aok:
			return true
		case bok:
			return false
		default:
			return positions[fields[i].Key] < positions[fields[j].Key]
		}
	})
	return fields
}
