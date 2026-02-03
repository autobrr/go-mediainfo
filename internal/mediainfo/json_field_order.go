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
	"FirstPacketOrder":                  2,
	"ID":                                3,
	"MenuID":                            4,
	"UniqueID":                          5,
	"Format":                            6,
	"Format_Version":                    7,
	"Format_Profile":                    8,
	"Format_Level":                      9,
	"Format_Settings_CABAC":             10,
	"Format_Settings_RefFrames":         11,
	"Format_Settings_BVOP":              12,
	"Format_Settings_Matrix":            13,
	"Format_Settings_Matrix_Data":       14,
	"Format_Settings_GOP":               15,
	"CodecID":                           16,
	"Duration":                          17,
	"BitRate_Mode":                      18,
	"BitRate":                           19,
	"BitRate_Nominal":                   20,
	"BitRate_Maximum":                   21,
	"Width":                             22,
	"Height":                            23,
	"Sampled_Width":                     24,
	"Sampled_Height":                    25,
	"PixelAspectRatio":                  26,
	"DisplayAspectRatio":                27,
	"Rotation":                          28,
	"FrameRate_Mode":                    29,
	"FrameRate_Mode_Original":           30,
	"FrameRate":                         31,
	"FrameRate_Num":                     32,
	"FrameRate_Den":                     33,
	"FrameCount":                        34,
	"Standard":                          35,
	"ColorSpace":                        36,
	"ChromaSubsampling":                 37,
	"BitDepth":                          38,
	"ScanType":                          39,
	"Compression_Mode":                  40,
	"Delay":                             41,
	"Delay_DropFrame":                   42,
	"Delay_Source":                      43,
	"Delay_Original":                    44,
	"Delay_Original_DropFrame":          45,
	"Delay_Original_Source":             46,
	"TimeCode_FirstFrame":               47,
	"TimeCode_Source":                   48,
	"Gop_OpenClosed":                    49,
	"Gop_OpenClosed_FirstFrame":         50,
	"StreamSize":                        51,
	"BufferSize":                        52,
	"Encoded_Library":                   53,
	"Encoded_Library_Name":              54,
	"Encoded_Library_Version":           55,
	"Encoded_Library_Settings":          56,
	"Default":                           57,
	"Forced":                            58,
	"colour_description_present":        59,
	"colour_description_present_Source": 60,
	"colour_range":                      61,
	"colour_range_Source":               62,
	"List_StreamKind":                   63,
	"List_StreamPos":                    64,
	"ServiceName":                       65,
	"ServiceProvider":                   66,
	"ServiceType":                       67,
	"extra":                             68,
}

var jsonAudioFieldOrder = map[string]int{
	"@type":                      0,
	"StreamOrder":                1,
	"FirstPacketOrder":           2,
	"ID":                         3,
	"MenuID":                     4,
	"UniqueID":                   5,
	"Format":                     6,
	"Format_Commercial_IfAny":    7,
	"Format_Settings_Endianness": 8,
	"Format_Version":             9,
	"Format_Settings_SBR":        10,
	"Format_AdditionalFeatures":  11,
	"MuxingMode":                 12,
	"CodecID":                    13,
	"Duration":                   14,
	"Source_Duration":            15,
	"Source_Duration_LastFrame":  16,
	"BitRate_Mode":               17,
	"BitRate":                    18,
	"BitRate_Maximum":            19,
	"Channels":                   20,
	"ChannelPositions":           21,
	"ChannelLayout":              22,
	"SamplesPerFrame":            23,
	"SamplingRate":               24,
	"SamplingCount":              25,
	"FrameRate":                  26,
	"FrameCount":                 27,
	"Source_FrameCount":          28,
	"Compression_Mode":           29,
	"Delay":                      30,
	"Delay_Source":               31,
	"Video_Delay":                32,
	"Encoded_Library":            33,
	"StreamSize":                 34,
	"Source_StreamSize":          35,
	"Default":                    36,
	"Forced":                     37,
	"ServiceKind":                38,
	"AlternateGroup":             39,
	"extra":                      40,
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
